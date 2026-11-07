// engine.go orchestrates discovery (Google Places scrape + save) and dispatch
// (AI pitch generation + WhatsApp send) for the lead generation engine.
package leadengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"marketingops/api/internal/store"
)

// LeadEngineStore is the subset of store.Store the engine needs.
// It is satisfied by *store.Store and by httpapi.WorkspaceStore.
type LeadEngineStore interface {
	WasSearchedToday(ctx context.Context, workspaceID, dayKey, category, city string) (bool, error)
	RecordDiscoverySearch(ctx context.Context, s store.LeadDiscoverySearch) error
	IsBlacklisted(ctx context.Context, workspaceID, phone string) (bool, error)
	SaveDiscoveredLead(ctx context.Context, workspaceID, placeID, name, phone, category, city, address string, rating float64, reviewCount int, websiteURL string) (store.Lead, error)
	ListLeadEngineLeads(ctx context.Context, workspaceID string) ([]store.Lead, error)
	UpdateLeadPitch(ctx context.Context, workspaceID, leadID, pitch string) error
	UpdateLeadStatus(ctx context.Context, workspaceID, leadID, status string) error
	SaveOutreachLog(ctx context.Context, l store.LeadOutreachLog) (store.LeadOutreachLog, error)
	HasRecentOutboundAttempt(ctx context.Context, workspaceID, leadID string, within time.Duration) (bool, error)
	HasRecentOutboundAttemptByPhone(ctx context.Context, workspaceID, phone string, within time.Duration) (bool, error)
	HasOutboundAttemptByPhone(ctx context.Context, workspaceID, phone string) (bool, error)
	CountOutboundSentToday(ctx context.Context, workspaceID string) (int, error)
	AddToBlacklist(ctx context.Context, workspaceID, phone, reason string) error
	ResetStaleProcessingLeads(ctx context.Context, workspaceID string) (int64, error)
}

// dispatcherClient bounds every dispatcher HTTP call. Without a timeout a
// hung dispatcher leaves the send goroutine blocked forever while holding the
// per-workspace dispatch lock — every later "send pending" trigger is then
// silently skipped and the queue appears frozen. 90s comfortably covers the
// number-check + send round trip; anything longer is a fault.
var dispatcherClient = &http.Client{Timeout: 90 * time.Second}

// ─── discovery ─────────────────────────────────────────────────────────────────

// DiscoveryParams holds everything RunDiscovery needs.
type DiscoveryParams struct {
	WorkspaceID     string
	GooglePlacesKey string
	Categories      []string
	Cities          []string
	DailyLimit      int
}

// RunDiscovery scrapes Google Places for each category+city pair, filters,
// checks blacklist, and persists new leads. It is safe to run in a goroutine.
// Returns the count of newly saved leads and any non-fatal errors.
func RunDiscovery(ctx context.Context, p DiscoveryParams, s LeadEngineStore) (int, []error) {
	dayKey := time.Now().UTC().Format("2006-01-02")
	var errs []error
	total := 0

	cats := p.Categories
	if len(cats) == 0 {
		cats = defaultCategories()
	}
	cities := p.Cities
	if len(cities) == 0 {
		cities = defaultCities()
	}

	for _, cat := range cats {
		for _, city := range cities {
			if total >= p.DailyLimit && p.DailyLimit > 0 {
				break
			}
			already, err := s.WasSearchedToday(ctx, p.WorkspaceID, dayKey, cat, city)
			if err != nil || already {
				continue
			}

			places, err := ScrapeCategoryInCity(ctx, p.GooglePlacesKey, cat, city)
			if err != nil {
				errs = append(errs, fmt.Errorf("scrape %s/%s: %w", cat, city, err))
				_ = s.RecordDiscoverySearch(ctx, store.LeadDiscoverySearch{
					WorkspaceID: p.WorkspaceID,
					DayKey:      dayKey,
					Category:    cat,
					City:        city,
					ResultCount: 0,
					Error:       err.Error(),
				})
				continue
			}

			saved := 0
			for _, pl := range places {
				phone := formatPhone(pl)
				bl, _ := s.IsBlacklisted(ctx, p.WorkspaceID, phone)
				if bl {
					continue
				}
				_, err := s.SaveDiscoveredLead(ctx, p.WorkspaceID,
					pl.PlaceID, pl.Name, phone, cat, city, pl.Address,
					pl.Rating, pl.ReviewCount, pl.WebsiteURL)
				if err == nil {
					saved++
					total++
				}
				// ErrAlreadyContacted: this phone already has a lead row here, so
				// the contact is not new and must not be queued again.

			}
			_ = s.RecordDiscoverySearch(ctx, store.LeadDiscoverySearch{
				WorkspaceID: p.WorkspaceID,
				DayKey:      dayKey,
				Category:    cat,
				City:        city,
				ResultCount: saved,
			})
		}
	}
	return total, errs
}

// ─── dispatch ─────────────────────────────────────────────────────────────────

// DispatchParams holds everything RunDispatch needs.
type DispatchParams struct {
	WorkspaceID      string
	AIKey            string
	AIModel          string
	SenderName       string
	PitchTemplate    string // workspace-authored message; when set, sent verbatim per lead
	BusinessOffering string // from workspace config; drives pitch content
	TargetAudience   string // from workspace config; drives pitch content
	PortfolioURL     string
	MinDelayMs       int
	MaxDelayMs       int
	DailyLimit       int
	DispatchURL      string // WhatsApp dispatcher base URL e.g. http://localhost:8083
	DispatcherSecret string // X-Internal-Secret header value; empty means no auth
}

// RunDispatch generates pitches for discovered leads and sends them via WhatsApp.
// It respects the daily limit and the configured send delays.
// dispatchLocks serializes dispatch per workspace: overlapping runs (a manual
// trigger racing the scheduler, or two clicks) each counted their "own" ~25
// sends, which let far more messages out than the daily limit intends.
var (
	dispatchLocksMu sync.Mutex
	dispatchLocks   = map[string]*sync.Mutex{}
)

// ensureDispatcherConnected reports whether the workspace's WhatsApp session
// is currently connected, starting it and waiting for it when needed. It
// returns an error (and nothing was sent) when the session cannot reach a
// connected state without human action — e.g. the QR code must be scanned.
func ensureDispatcherConnected(ctx context.Context, p DispatchParams) error {
	statusURL := p.DispatchURL + "/sessions/" + p.WorkspaceID + "/status"
	doStatus := func() (string, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
		if err != nil {
			return "", err
		}
		if p.DispatcherSecret != "" {
			req.Header.Set("X-Internal-Secret", p.DispatcherSecret)
		}
		resp, err := dispatcherClient.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if err != nil {
			return "", err
		}
		var parsed struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return "", fmt.Errorf("bad dispatcher status response: %w", err)
		}
		return parsed.Status, nil
	}

	status, err := doStatus()
	if err != nil {
		return fmt.Errorf("WhatsApp dispatcher unreachable: %w", err)
	}
	switch status {
	case "connected":
		return nil // ready to send
	case "qr_ready", "auth_failure":
		return errors.New("WhatsApp needs a QR scan or had an auth failure; open the dashboard WhatsApp panel and scan the code — nothing was sent")
	}

	// disconnected / initializing / error: kick off a start and wait for it.
	startURL := p.DispatchURL + "/sessions/" + p.WorkspaceID + "/start"
	startReq, err := http.NewRequestWithContext(ctx, http.MethodPost, startURL, nil)
	if err != nil {
		return fmt.Errorf("WhatsApp dispatcher unreachable: %w", err)
	}
	startReq.Header.Set("Content-Type", "application/json")
	if p.DispatcherSecret != "" {
		startReq.Header.Set("X-Internal-Secret", p.DispatcherSecret)
	}
	startResp, err := dispatcherClient.Do(startReq)
	if err != nil {
		return fmt.Errorf("could not start WhatsApp session: %w", err)
	}
	startResp.Body.Close()

	deadline := time.Now().Add(100 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
		status, err := doStatus()
		if err != nil {
			continue // transient; keep waiting within the deadline
		}
		switch status {
		case "connected":
			slog.Default().Info("WhatsApp session connected; dispatch proceeding", "workspace_id", p.WorkspaceID)
			return nil
		case "qr_ready", "auth_failure":
			return errors.New("WhatsApp needs a QR scan or had an auth failure; open the dashboard WhatsApp panel and scan the code — nothing was sent")
		}
	}
	return errors.New("WhatsApp session did not connect within 100 seconds; no messages were sent")
}

func dispatchLock(workspaceID string) *sync.Mutex {
	dispatchLocksMu.Lock()
	defer dispatchLocksMu.Unlock()
	l, ok := dispatchLocks[workspaceID]
	if !ok {
		l = &sync.Mutex{}
		dispatchLocks[workspaceID] = l
	}
	return l
}

func RunDispatch(ctx context.Context, p DispatchParams, s LeadEngineStore) error {
	mu := dispatchLock(p.WorkspaceID)
	if !mu.TryLock() {
		slog.Default().Info("dispatch already running for workspace; skipping duplicate trigger",
			"workspace_id", p.WorkspaceID)
		return nil
	}
	defer mu.Unlock()

	// Self-heal: an API restart mid-send leaves leads in 'processing' forever
	// (nothing ever moves them). Recover them so the queue cannot freeze.
	if n, err := s.ResetStaleProcessingLeads(ctx, p.WorkspaceID); err != nil {
		slog.Default().Warn("could not reset stale processing leads", "workspace_id", p.WorkspaceID, "error", err)
	} else if n > 0 {
		slog.Default().Info("reset stale processing leads to failed",
			"workspace_id", p.WorkspaceID, "count", n)
	}

	leads, err := s.ListLeadEngineLeads(ctx, p.WorkspaceID)
	if err != nil {
		return err
	}
	// The sender identity comes from workspace config; nothing business-specific
	// is defaulted here so a half-configured workspace never sends foreign copy.
	senderName := p.SenderName
	portfolioURL := p.PortfolioURL

	// The daily limit is global to the workspace for the whole day: count every
	// message already recorded as sent today before sending anything. Otherwise
	// each run (restart-triggered scheduler pass, manual trigger, UI click)
	// gets a fresh allowance and the real message volume exceeds the cap.
	sentToday := 0
	if p.DailyLimit > 0 {
		sentToday, err = s.CountOutboundSentToday(ctx, p.WorkspaceID)
		if err != nil {
			return err
		}
		if sentToday >= p.DailyLimit {
			slog.Default().Info("daily dispatch limit already reached; skipping run",
				"workspace_id", p.WorkspaceID, "sent_today", sentToday, "daily_limit", p.DailyLimit)
			return nil
		}
	}

	sent := sentToday

	// One message per contact, per run and forever. Legacy kept a UNIQUE phone
	// column, so the same number could never even be queued twice; the DB now
	// backs that up, and this map closes the remaining hole inside a batch
	// (two lead rows carrying the same number).
	claimedPhones := make(map[string]struct{}, len(leads))

	// Pre-flight: never start a batch against a down session. Without this,
	// a dispatcher restart (or a kicked session) turns every attempt into a
	// 409 "not connected" failure — and each failure still burns a lead
	// status and a cooldown slot. If the session is starting up, wait for it
	// to reach connected (or abort with a QR-scan instruction).
	if err := ensureDispatcherConnected(ctx, p); err != nil {
		slog.Default().Warn("dispatch aborted: WhatsApp session not usable",
			"workspace_id", p.WorkspaceID, "reason", err)
		return err
	}

	// Between-send pacing, ported 1:1 from the proven legacy engine that
	// ran this shared account for months without a single restriction:
	//   delay = randomDelay(min, max) — jittered, INCLUSIVE of both bounds
	//   (Node: Math.floor(random * (max - min + 1)) + min);
	//   one delay sits BETWEEN consecutive send attempts — none before the
	//   first message and none after the last of the batch.
	// A dispatcher outage must never collapse the jittered send delay and
	// fire 30+ WhatsApp requests in under two minutes: that burst pattern
	// is exactly what gets accounts restricted.
	attempted := false // a real dispatcher contact happened in this run
	sleepBetweenAttempts := func() error {
		if !attempted {
			return nil // first send of the run goes out at once
		}
		delayMs := p.MinDelayMs
		if p.MaxDelayMs > p.MinDelayMs {
			delayMs = p.MinDelayMs + rand.Intn(p.MaxDelayMs-p.MinDelayMs+1)
		}
		if delayMs <= 0 {
			delayMs = 25000
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(delayMs) * time.Millisecond):
		}
		return nil
	}

	for _, lead := range leads {
		if p.DailyLimit > 0 && sent >= p.DailyLimit {
			break
		}
		// "failed" is retried because a failed send was never delivered —
		// e.g. the WhatsApp session was down at the time.
		if lead.Status != "discovered" && lead.Status != "pending" && lead.Status != "failed" {
			continue
		}
		phone := ""
		if lead.ContactPhone != nil {
			phone = *lead.ContactPhone
		}
		if phone == "" {
			continue
		}

		// The blacklist is per workspace and applies forever: a suppressed
		// number must never be contacted again, even if its lead row has
		// drifted back into a retryable status.
		if suppressed, _ := s.IsBlacklisted(ctx, p.WorkspaceID, phone); suppressed {
			_ = s.UpdateLeadStatus(ctx, p.WorkspaceID, lead.ID, "blacklisted")
			continue
		}
		// Permanent duplicate suppression, exactly as the legacy engine did it:
		// once a phone has ANY outbound attempt in this workspace (sent, or
		// failed with an unknown outcome), it is contacted for good and never
		// selected again — no cooldown, no retry window. Only a send the
		// dispatcher provably refused before accepting it (logged `not_sent:`)
		// stays retryable.
		phoneKey := store.PhoneKey(phone)
		if _, taken := claimedPhones[phoneKey]; taken {
			continue
		}
		if attemptedBefore, _ := s.HasOutboundAttemptByPhone(ctx, p.WorkspaceID, phone); attemptedBefore {
			_ = s.UpdateLeadStatus(ctx, p.WorkspaceID, lead.ID, "sent")
			continue
		}
		claimedPhones[phoneKey] = struct{}{}

		// Generate pitch if not already generated.
		pitch := lead.Pitch
		name := ""
		if lead.ContactName != nil {
			name = *lead.ContactName
		}
		// A workspace-authored message wins: send it verbatim (with
		// placeholders merged in per lead). Empty template = AI-generated pitch.
		if pitch == "" && p.PitchTemplate != "" {
			pitch = applyPitchTemplate(p.PitchTemplate, lead, senderName, portfolioURL)
			_ = s.UpdateLeadPitch(ctx, p.WorkspaceID, lead.ID, pitch)
		} else if pitch == "" {
			rating := 0.0
			if lead.Rating != nil {
				rating = *lead.Rating
			}
			generated, err := GeneratePitch(ctx, PitchParams{
				BusinessName:     name,
				Category:         lead.Category,
				City:             lead.City,
				SenderName:       senderName,
				BusinessOffering: p.BusinessOffering,
				TargetAudience:   p.TargetAudience,
				PortfolioURL:     portfolioURL,
				Rating:           rating,
				ReviewCount:      lead.ReviewCount,
				AIKey:            p.AIKey,
				AIModel:          p.AIModel,
			})
			if err != nil {
				// Failed pitch generation must not let the loop spin freely —
				// space failed attempts with the same anti-burst delay as sends.
				attempted = true
				if paceErr := sleepBetweenAttempts(); paceErr != nil {
					return paceErr
				}
				continue
			}
			pitch = generated
			_ = s.UpdateLeadPitch(ctx, p.WorkspaceID, lead.ID, pitch)
		}

		// Jittered cooldown from the previous real attempt, legacy-style:
		// a delay sits between consecutive sends, never before the first.
		if paceErr := sleepBetweenAttempts(); paceErr != nil {
			return paceErr
		}

		_ = s.UpdateLeadStatus(ctx, p.WorkspaceID, lead.ID, "processing")

		// Send via WhatsApp dispatcher.
		sendURL := p.DispatchURL + "/sessions/" + p.WorkspaceID + "/send"
		payload, _ := json.Marshal(map[string]string{
			"phone":   phone,
			"message": pitch,
		})
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, sendURL, bytes.NewReader(payload))
		if reqErr != nil {
			_ = s.UpdateLeadStatus(ctx, p.WorkspaceID, lead.ID, "failed")
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		if p.DispatcherSecret != "" {
			req.Header.Set("X-Internal-Secret", p.DispatcherSecret)
		}
		resp, err := dispatcherClient.Do(req)
		// Anything that reached the dispatcher consumed a slot in the
		// WhatsApp spam window; the next attempt pays the full jittered gap.
		attempted = true
		if err != nil || resp.StatusCode >= 400 {
			// Record WHY the send failed — the dispatcher's own error text, or
			// unreachability — so a failed batch is never a silent mystery.
			reason := "WhatsApp dispatcher unreachable"
			var body []byte
			if resp != nil {
				body, _ = io.ReadAll(io.LimitReader(resp.Body, 500))
				resp.Body.Close()
				reason = fmt.Sprintf("dispatcher HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			}
			// A 4xx is the dispatcher refusing the request (e.g. session not
			// connected) before it ever reached WhatsApp: nothing was sent, so
			// the attempt may be retried later. Any other failure — timeout, 5xx,
			// transport error — leaves the outcome unknown, and an unknown send
			// must never be repeated (the dispatcher's own rule).
			// notOnWhatsApp is a permanent condition, not a retryable refusal: it
			// is handled (and blacklisted) below, never re-queued.
			if resp != nil && resp.StatusCode >= 400 && resp.StatusCode < 500 &&
				!strings.Contains(string(body), "notOnWhatsApp") {
				reason = "not_sent: " + reason
			}
			slog.Default().Warn("WhatsApp send failed",
				"workspace_id", p.WorkspaceID, "lead_id", lead.ID, "phone", phone, "reason", reason)
			_, _ = s.SaveOutreachLog(ctx, store.LeadOutreachLog{
				WorkspaceID: p.WorkspaceID, LeadID: lead.ID, Direction: "outbound",
				Body: pitch, Status: "failed", Error: reason,
			})
			// "not on WhatsApp" is a PERMANENT condition — the number has no
			// WhatsApp account at all, so retrying it every run only clogs the
			// daily queue and wastes pacing slots. Park the lead as blacklisted
			// (a valid lead-engine status) and suppress the phone so future
			// discovery runs never re-queue it.
			if strings.Contains(string(body), "notOnWhatsApp") {
				_ = s.UpdateLeadStatus(ctx, p.WorkspaceID, lead.ID, "blacklisted")
				_ = s.AddToBlacklist(ctx, p.WorkspaceID, phone, "not_on_whatsapp")
				slog.Default().Info("lead parked: number not registered on WhatsApp",
					"workspace_id", p.WorkspaceID, "lead_id", lead.ID, "phone", phone)
				continue
			}
			_ = s.UpdateLeadStatus(ctx, p.WorkspaceID, lead.ID, "failed")
			continue
		}
		resp.Body.Close()
		_ = s.UpdateLeadStatus(ctx, p.WorkspaceID, lead.ID, "sent")
		_, _ = s.SaveOutreachLog(ctx, store.LeadOutreachLog{
			WorkspaceID: p.WorkspaceID, LeadID: lead.ID, Direction: "outbound",
			Body: pitch, Status: "sent",
		})
		sent++
	}
	return nil
}

// ─── follow-up ────────────────────────────────────────────────────────────────

// FollowUpParams holds one follow-up run: a workspace-authored message and
// the list of previously-messaged contacts the user selected in the dashboard.
type FollowUpParams struct {
	WorkspaceID      string
	Message          string // user-composed follow-up; placeholders merged per contact
	SenderName       string
	PortfolioURL     string
	Contacts         []store.MessagedLead
	MinDelayMs       int
	MaxDelayMs       int
	DispatchURL      string
	DispatcherSecret string
}

// FollowUpResult summarises one follow-up run for the dashboard event feed.
type FollowUpResult struct {
	Total   int `json:"total"`
	Sent    int `json:"sent"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

// TryLockDispatch takes the per-workspace dispatch lock without a long-lived
// goroutine: callers that need to fail fast (e.g. the follow-up trigger wants
// to answer 409 while another run is active) probe it and release at once.
// RunDispatch and RunFollowUp hold the same lock for the whole batch, so a
// follow-up can never race a cold-outreach dispatch on the same session.
func TryLockDispatch(workspaceID string) (func(), bool) {
	mu := dispatchLock(workspaceID)
	if !mu.TryLock() {
		return func() {}, false
	}
	return mu.Unlock, true
}

// RunFollowUp sends the user's follow-up message to the selected previously
// messaged contacts. It is deliberately a second touch the OPERATOR asked
// for, so it bypasses the cold-outreach "sent ever" suppression — but every
// account-safety rule still applies: the per-workspace dispatch lock, the
// same jittered between-send pacing that kept the account restriction-free,
// blacklist (dead numbers) exclusion, and permanent parking of numbers that
// turn out not to be on WhatsApp. Each message is logged as a normal
// outbound entry, so it shows up in the workspace's Daily Activity.
func RunFollowUp(ctx context.Context, p FollowUpParams, s LeadEngineStore) (FollowUpResult, error) {
	res := FollowUpResult{Total: len(p.Contacts)}
	if strings.TrimSpace(p.Message) == "" {
		return res, errors.New("follow-up message is empty")
	}
	// One follow-up per contact per run: two lead rows carrying the same number
	// must not receive the same message twice.
	mu := dispatchLock(p.WorkspaceID)
	if !mu.TryLock() {
		return res, errors.New("another send run is already in progress for this workspace; wait for it to finish and try again")
	}
	defer mu.Unlock()

	// Pre-flight like the cold batch: never send against a down session.
	if err := ensureDispatcherConnected(ctx, DispatchParams{
		WorkspaceID:      p.WorkspaceID,
		DispatchURL:      p.DispatchURL,
		DispatcherSecret: p.DispatcherSecret,
	}); err != nil {
		return res, err
	}

	// Between-send pacing, same legacy rule as RunDispatch: one jittered,
	// bounds-inclusive delay between consecutive real sends — none before
	// the first and none after the last.
	attempted := false
	sleepBetweenAttempts := func() error {
		if !attempted {
			return nil
		}
		delayMs := p.MinDelayMs
		if p.MaxDelayMs > p.MinDelayMs {
			delayMs = p.MinDelayMs + rand.Intn(p.MaxDelayMs-p.MinDelayMs+1)
		}
		if delayMs <= 0 {
			delayMs = 25000
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(delayMs) * time.Millisecond):
		}
		return nil
	}

	claimedPhones := make(map[string]struct{}, len(p.Contacts))
	for _, c := range p.Contacts {
		if strings.TrimSpace(c.Phone) == "" {
			res.Skipped++
			continue
		}
		phoneKey := store.PhoneKey(c.Phone)
		if _, taken := claimedPhones[phoneKey]; taken {
			res.Skipped++
			continue
		}
		// Blacklisted numbers are dead to this workspace: skip and count.
		if suppressed, _ := s.IsBlacklisted(ctx, p.WorkspaceID, c.Phone); suppressed {
			res.Skipped++
			continue
		}
		claimedPhones[phoneKey] = struct{}{}

		if paceErr := sleepBetweenAttempts(); paceErr != nil {
			return res, paceErr
		}

		name := c.Name
		lead := store.Lead{
			ID:          c.ID,
			ContactName: &name,
			Category:    c.Category,
			City:        c.City,
		}
		message := applyPitchTemplate(p.Message, lead, p.SenderName, p.PortfolioURL)

		sendURL := p.DispatchURL + "/sessions/" + p.WorkspaceID + "/send"
		payload, _ := json.Marshal(map[string]string{
			"phone":   c.Phone,
			"message": message,
		})
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, sendURL, bytes.NewReader(payload))
		if reqErr != nil {
			res.Failed++
			_, _ = s.SaveOutreachLog(ctx, store.LeadOutreachLog{
				WorkspaceID: p.WorkspaceID, LeadID: c.ID, Direction: "outbound",
				Body: message, Status: "failed", Error: "could not build dispatcher request: " + reqErr.Error(),
			})
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		if p.DispatcherSecret != "" {
			req.Header.Set("X-Internal-Secret", p.DispatcherSecret)
		}
		resp, err := dispatcherClient.Do(req)
		attempted = true
		if err != nil || resp.StatusCode >= 400 {
			reason := "WhatsApp dispatcher unreachable"
			var body []byte
			if resp != nil {
				body, _ = io.ReadAll(io.LimitReader(resp.Body, 500))
				resp.Body.Close()
				reason = fmt.Sprintf("dispatcher HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			}
			slog.Default().Warn("follow-up send failed",
				"workspace_id", p.WorkspaceID, "lead_id", c.ID, "phone", c.Phone, "reason", reason)
			res.Failed++
			_, _ = s.SaveOutreachLog(ctx, store.LeadOutreachLog{
				WorkspaceID: p.WorkspaceID, LeadID: c.ID, Direction: "outbound",
				Body: message, Status: "failed", Error: reason,
			})
			// Same permanent parking as the cold batch: never retry a
			// number that has no WhatsApp account at all.
			if strings.Contains(string(body), "notOnWhatsApp") {
				_ = s.AddToBlacklist(ctx, p.WorkspaceID, c.Phone, "not_on_whatsapp")
				slog.Default().Info("follow-up contact parked: number not registered on WhatsApp",
					"workspace_id", p.WorkspaceID, "lead_id", c.ID, "phone", c.Phone)
			}
			continue
		}
		resp.Body.Close()
		res.Sent++
		_, _ = s.SaveOutreachLog(ctx, store.LeadOutreachLog{
			WorkspaceID: p.WorkspaceID, LeadID: c.ID, Direction: "outbound",
			Body: message, Status: "sent",
		})
	}
	return res, nil
}

// applyPitchTemplate merges a workspace-authored message with per-lead facts.
// Placeholders are double-braced, lowercase, matched case-insensitively and
// unknown ones are stripped so they never reach a prospect.
func applyPitchTemplate(tpl string, lead store.Lead, senderName, portfolioURL string) string {
	name := ""
	if lead.ContactName != nil {
		name = *lead.ContactName
	}
	ratings := ""
	if lead.Rating != nil {
		ratings = fmt.Sprintf("%.1f", *lead.Rating)
	}
	replacer := strings.NewReplacer(
		"{{business_name}}", name,
		"{{businessname}}", name,
		"{{category}}", lead.Category,
		"{{city}}", lead.City,
		"{{sender_name}}", senderName,
		"{{sendername}}", senderName,
		"{{portfolio_url}}", portfolioURL,
		"{{portfoliourl}}", portfolioURL,
		"{{rating}}", ratings,
	)
	out := strings.TrimSpace(replacer.Replace(tpl))
	// Remove any leftover unknown placeholders.
	out = strings.TrimSpace(unknownPlaceholder.ReplaceAllString(out, ""))
	return out
}

var unknownPlaceholder = regexp.MustCompile(`(?i)\{\{\s*[a-z0-9_]+\s*\}\}`)

// ─── Ghana defaults (mirror of gh-lead-engine/config/index.js) ────────────────

func defaultCategories() []string {
	return []string{
		"school", "academy", "clinic", "hospital", "hotel",
		"guesthouse", "restaurant", "salon", "barbershop",
		"shop", "boutique", "store", "church", "ngo",
		"law firm", "accounting", "consulting",
	}
}

func defaultCities() []string {
	return []string{
		"Accra", "Kumasi", "Tamale", "Cape Coast", "Takoradi",
		"Sunyani", "Koforidua", "Ho", "Bolgatanga", "Wa",
		"Tema", "Kasoa", "Ashaiman",
	}
}
