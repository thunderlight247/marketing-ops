// Lead engine HTTP handlers — config, discovery trigger, outreach logs, blacklist, WA status.
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"log/slog"

	"marketingops/api/internal/events"
	"marketingops/api/internal/leadengine"
	"marketingops/api/internal/secretcipher"
	"marketingops/api/internal/store"
)

const maxLeadEnginePayload = 32 * 1024

// ─── config ──────────────────────────────────────────────────────────────────

type leadEngineConfigJSON struct {
	WorkspaceID        string   `json:"workspaceId"`
	Enabled            bool     `json:"enabled"`
	HasGooglePlacesKey bool     `json:"hasGooglePlacesKey"`
	HasAIProviderKey   bool     `json:"hasAiProviderKey"`
	AIModel            string   `json:"aiModel"`
	SenderName         string   `json:"senderName"`
	BusinessOffering   string   `json:"businessOffering"`
	TargetAudience     string   `json:"targetAudience"`
	PortfolioURL       string   `json:"portfolioUrl"`
	TargetCategories   []string `json:"targetCategories"`
	TargetCities       []string `json:"targetCities"`
	DailyLimit         int      `json:"dailyLimit"`
	MinDelayMs         int      `json:"minDelayMs"`
	MaxDelayMs         int      `json:"maxDelayMs"`
	WhatsAppStatus     string   `json:"whatsappStatus"`
	WhatsAppPhone      string   `json:"whatsappPhone"`
	PitchTemplate      string   `json:"pitchTemplate"`
	EnableAutoFollowup bool     `json:"enableAutoFollowup"`
	Followup1Template  string   `json:"followup1Template"`
	Followup1DelayDays int      `json:"followup1DelayDays"`
	Followup2Template  string   `json:"followup2Template"`
	Followup2DelayDays int      `json:"followup2DelayDays"`
}

func toLeadEngineConfigJSON(c store.WorkspaceLeadConfig) leadEngineConfigJSON {
	cats := c.TargetCategories
	if cats == nil {
		cats = []string{}
	}
	cities := c.TargetCities
	if cities == nil {
		cities = []string{}
	}
	return leadEngineConfigJSON{
		WorkspaceID:        c.WorkspaceID,
		Enabled:            c.Enabled,
		HasGooglePlacesKey: len(c.GooglePlacesAPIKeyCiphertext) > 0,
		HasAIProviderKey:   len(c.AIProviderKeyCiphertext) > 0,
		AIModel:            c.AIModel,
		SenderName:         c.SenderName,
		BusinessOffering:   c.BusinessOffering,
		TargetAudience:     c.TargetAudience,
		PortfolioURL:       c.PortfolioURL,
		TargetCategories:   cats,
		TargetCities:       cities,
		DailyLimit:         c.DailyLimit,
		MinDelayMs:         c.MinDelayMs,
		MaxDelayMs:         c.MaxDelayMs,
		WhatsAppStatus:     c.WhatsAppStatus,
		WhatsAppPhone:      c.WhatsAppPhone,
		PitchTemplate:      c.PitchTemplate,
		EnableAutoFollowup: c.EnableAutoFollowup,
		Followup1Template:  c.Followup1Template,
		Followup1DelayDays: c.Followup1DelayDays,
		Followup2Template:  c.Followup2Template,
		Followup2DelayDays: c.Followup2DelayDays,
	}
}

// GET /api/v1/workspaces/{id}/lead-engine/config
func getLeadEngineConfigHandler(ws WorkspaceStore, credKey []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		cfg, err := ws.GetLeadConfig(r.Context(), workspaceID)
		if errors.Is(err, store.ErrNotFound) {
			// Return a zeroed config — the workspace simply hasn't configured it yet.
			writeJSON(w, http.StatusOK, leadEngineConfigJSON{
				WorkspaceID:      workspaceID,
				WhatsAppStatus:   "disconnected",
				TargetCategories: []string{},
				TargetCities:     []string{},
			})
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to load lead engine config")
			return
		}
		writeJSON(w, http.StatusOK, toLeadEngineConfigJSON(cfg))
	}
}

// PUT /api/v1/workspaces/{id}/lead-engine/config
func putLeadEngineConfigHandler(ws WorkspaceStore, credKey []byte, credKeyVersion int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxLeadEnginePayload))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "failed to read request body")
			return
		}
		var in struct {
			Enabled            bool     `json:"enabled"`
			GooglePlacesAPIKey string   `json:"googlePlacesApiKey"`
			AIProviderKey      string   `json:"aiProviderKey"`
			AIModel            string   `json:"aiModel"`
			SenderName         string   `json:"senderName"`
			BusinessOffering   string   `json:"businessOffering"`
			TargetAudience     string   `json:"targetAudience"`
			PortfolioURL       string   `json:"portfolioUrl"`
			TargetCategories   []string `json:"targetCategories"`
			TargetCities       []string `json:"targetCities"`
			DailyLimit         int      `json:"dailyLimit"`
			MinDelayMs         int      `json:"minDelayMs"`
			MaxDelayMs         int      `json:"maxDelayMs"`
			PitchTemplate      string   `json:"pitchTemplate"`
			EnableAutoFollowup bool     `json:"enableAutoFollowup"`
			Followup1Template  string   `json:"followup1Template"`
			Followup1DelayDays int      `json:"followup1DelayDays"`
			Followup2Template  string   `json:"followup2Template"`
			Followup2DelayDays int      `json:"followup2DelayDays"`
		}
		if err := json.Unmarshal(body, &in); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		// Validate delay bounds so engine.go's rand.Intn(max-min) never receives a negative argument.
		if in.MinDelayMs > 0 && in.MaxDelayMs > 0 && in.MinDelayMs >= in.MaxDelayMs {
			writeJSONError(w, http.StatusBadRequest, "minDelayMs must be less than maxDelayMs")
			return
		}

		in.GooglePlacesAPIKey = strings.TrimSpace(in.GooglePlacesAPIKey)
		in.AIProviderKey = strings.TrimSpace(in.AIProviderKey)
		// Google Places keys are long and always start with "AIza"; catch a
		// mis-pasted key here instead of letting every discovery search fail.
		if in.GooglePlacesAPIKey != "" && !strings.HasPrefix(in.GooglePlacesAPIKey, "AIza") {
			writeJSONError(w, http.StatusBadRequest, "Google Places API key must start with \"AIza\" — check the paste for typos")
			return
		}

		// Load existing config to preserve keys when no new key is sent.
		existing, _ := ws.GetLeadConfig(r.Context(), workspaceID)
		if existing.WhatsAppStatus == "" {
			// A workspace that never connected WhatsApp is simply "disconnected";
			// the empty string would violate the status check constraint.
			existing.WhatsAppStatus = "disconnected"
		}

		c := store.WorkspaceLeadConfig{
			WorkspaceID:                  workspaceID,
			Enabled:                      in.Enabled,
			GooglePlacesAPIKeyCiphertext: existing.GooglePlacesAPIKeyCiphertext,
			GooglePlacesAPIKeyVersion:    existing.GooglePlacesAPIKeyVersion,
			AIProviderKeyCiphertext:      existing.AIProviderKeyCiphertext,
			AIProviderKeyVersion:         existing.AIProviderKeyVersion,
			AIModel:                      in.AIModel,
			SenderName:                   in.SenderName,
			BusinessOffering:             in.BusinessOffering,
			TargetAudience:               in.TargetAudience,
			PortfolioURL:                 in.PortfolioURL,
			TargetCategories:             in.TargetCategories,
			TargetCities:                 in.TargetCities,
			DailyLimit:                   in.DailyLimit,
			MinDelayMs:                   in.MinDelayMs,
			MaxDelayMs:                   in.MaxDelayMs,
			WhatsAppStatus:               existing.WhatsAppStatus,
			WhatsAppPhone:                existing.WhatsAppPhone,
			PitchTemplate:                in.PitchTemplate,
			EnableAutoFollowup:           in.EnableAutoFollowup,
			Followup1Template:            in.Followup1Template,
			Followup1DelayDays:           in.Followup1DelayDays,
			Followup2Template:            in.Followup2Template,
			Followup2DelayDays:           in.Followup2DelayDays,
		}

		// Encrypt and store Google Places key if a new one was provided.
		if in.GooglePlacesAPIKey != "" {
			ct, err := secretcipher.Encrypt(credKey, []byte(in.GooglePlacesAPIKey))
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "failed to encrypt Google Places key")
				return
			}
			c.GooglePlacesAPIKeyCiphertext = ct
			c.GooglePlacesAPIKeyVersion = credKeyVersion
		}
		// Encrypt and store AI provider key if a new one was provided.
		if in.AIProviderKey != "" {
			ct, err := secretcipher.Encrypt(credKey, []byte(in.AIProviderKey))
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "failed to encrypt AI provider key")
				return
			}
			c.AIProviderKeyCiphertext = ct
			c.AIProviderKeyVersion = credKeyVersion
		}

		saved, err := ws.UpsertLeadConfig(r.Context(), c)
		if err != nil {
			slog.Default().Warn("lead engine config save failed", "workspace_id", workspaceID, "err", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to save lead engine config")
			return
		}
		writeJSON(w, http.StatusOK, toLeadEngineConfigJSON(saved))
	}
}

// ─── WhatsApp connect / disconnect ───────────────────────────────────────────

func dispatcherURL() string {
	u := os.Getenv("WHATSAPP_DISPATCHER_URL")
	if u == "" {
		u = "http://localhost:8083"
	}
	return u
}

// dispatcherSecret returns the shared secret used to authenticate calls to the
// WhatsApp dispatcher. An empty string means no secret is configured and the
// header is omitted, which keeps the existing "no-auth" mode working.
func dispatcherSecret() string {
	return os.Getenv("WHATSAPP_DISPATCHER_SECRET")
}

// newDispatcherRequest builds an *http.Request pre-signed with the dispatcher
// secret header when one is configured.
func newDispatcherRequest(ctx context.Context, method, url string, body io.Reader, contentType string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if s := dispatcherSecret(); s != "" {
		req.Header.Set("X-Internal-Secret", s)
	}
	return req, nil
}

// POST /api/v1/workspaces/{id}/lead-engine/whatsapp/connect
func connectWhatsAppHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		whatsAppSessionAction(w, r, ws, "start", nil)
	}
}

// POST /api/v1/workspaces/{id}/lead-engine/whatsapp/pair
// Alternate login: ask WhatsApp to send a pairing code to the supplied phone
// number. It authenticates the same persisted session as the QR code.
func pairWhatsAppHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Phone string `json:"phone"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, maxLeadEnginePayload)).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid pairing request")
			return
		}
		phone := normalizeWhatsAppPairingPhone(body.Phone)
		if len(phone) < 8 || len(phone) > 15 {
			writeJSONError(w, http.StatusBadRequest, "enter a phone number with its country code")
			return
		}
		whatsAppSessionAction(w, r, ws, "pair", map[string]string{"phone": phone})
	}
}

func normalizeWhatsAppPairingPhone(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// whatsAppSessionAction proxies start/pair for the dispatcher and mirrors the
// resulting status into the workspace row.
func whatsAppSessionAction(w http.ResponseWriter, r *http.Request, ws WorkspaceStore, action string, payload any) {
	workspaceID, ok := workspaceScope(w, r, ws)
	if !ok {
		return
	}
	url := dispatcherURL() + "/sessions/" + workspaceID + "/" + action
	// own=1 links this workspace's OWN WhatsApp number instead of the
	// compatibility fallback account, so workspaces stop sharing a number.
	if r.URL.Query().Get("own") == "1" {
		url = dispatcherURL() + "/sessions/" + workspaceID + "/own/" + action
	}
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "could not build dispatcher request")
			return
		}
		body = bytes.NewReader(encoded)
	}
	req, err := newDispatcherRequest(r.Context(), http.MethodPost, url, body, "application/json")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not build dispatcher request")
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "WhatsApp dispatcher unreachable")
		return
	}
	defer resp.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		writeJSONError(w, http.StatusBadGateway, "bad dispatcher response")
		return
	}
	if resp.StatusCode >= http.StatusBadRequest {
		message, _ := result["error"].(string)
		if message == "" {
			message = "WhatsApp pairing request was rejected"
		}
		writeJSONError(w, resp.StatusCode, message)
		return
	}
	// Update DB status so config page refreshes correctly.
	status, _ := result["status"].(string)
	phone, _ := result["phone"].(string)
	if status == "" {
		status = "disconnected"
	}
	if status == "connected" {
		_ = ws.SetWhatsAppStatus(r.Context(), workspaceID, "connected", phone)
	} else {
		_ = ws.SetWhatsAppStatus(r.Context(), workspaceID, status, "")
	}
	events.DefaultHub.Publish(workspaceID, "whatsapp_status", status)
	writeJSON(w, http.StatusOK, result)
}

// POST /api/v1/workspaces/{id}/lead-engine/whatsapp/disconnect
func disconnectWhatsAppHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		url := dispatcherURL() + "/sessions/" + workspaceID + "/disconnect"
		req, err := newDispatcherRequest(r.Context(), http.MethodPost, url, nil, "application/json")
		if err != nil {
			_ = ws.SetWhatsAppStatus(r.Context(), workspaceID, "disconnected", "")
			events.DefaultHub.Publish(workspaceID, "whatsapp_status", "disconnected")
			writeJSONError(w, http.StatusInternalServerError, "could not build dispatcher request")
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			// Best-effort: still mark disconnected in DB.
			_ = ws.SetWhatsAppStatus(r.Context(), workspaceID, "disconnected", "")
			events.DefaultHub.Publish(workspaceID, "whatsapp_status", "disconnected")
			writeJSONError(w, http.StatusBadGateway, "WhatsApp dispatcher unreachable")
			return
		}
		resp.Body.Close()
		_ = ws.SetWhatsAppStatus(r.Context(), workspaceID, "disconnected", "")
		events.DefaultHub.Publish(workspaceID, "whatsapp_status", "disconnected")
		writeJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
	}
}

// GET /api/v1/workspaces/{id}/lead-engine/whatsapp/status
func getWhatsAppStatusHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		url := dispatcherURL() + "/sessions/" + workspaceID + "/status"
		req, err := newDispatcherRequest(r.Context(), http.MethodGet, url, nil, "")
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "could not build dispatcher request")
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, "WhatsApp dispatcher unreachable")
			return
		}
		defer resp.Body.Close()
		var result map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			writeJSONError(w, http.StatusBadGateway, "bad dispatcher response")
			return
		}
		// Sync DB status.
		if status, ok := result["status"].(string); ok {
			phone, _ := result["phone"].(string)
			_ = ws.SetWhatsAppStatus(r.Context(), workspaceID, status, phone)
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// GET /api/v1/lead-engine/whatsapp/sessions
// Audit view: which workspace's WhatsApp session is paired to which number, so
// an operator can see at a glance which workspaces still borrow the shared
// fallback account.
func listWhatsAppSessionsHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := newDispatcherRequest(r.Context(), http.MethodGet, dispatcherURL()+"/sessions", nil, "")
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "could not build dispatcher request")
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, "WhatsApp dispatcher unreachable")
			return
		}
		defer resp.Body.Close()
		var payload struct {
			Sessions []struct {
				WorkspaceID *string `json:"workspaceId"`
				Source      string  `json:"source"`
				Status      string  `json:"status"`
				Phone       string  `json:"phone"`
				Connected   bool    `json:"connected"`
			} `json:"sessions"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			writeJSONError(w, http.StatusBadGateway, "bad dispatcher response")
			return
		}
		names := map[string]string{}
		if list, err := ws.ListWorkspaces(r.Context()); err == nil {
			for _, item := range list {
				names[item.ID] = item.Name
			}
		}
		type sessionJSON struct {
			WorkspaceID   string `json:"workspaceId"`
			WorkspaceName string `json:"workspaceName"`
			Source        string `json:"source"`
			Status        string `json:"status"`
			Phone         string `json:"phone"`
			Connected     bool   `json:"connected"`
		}
		out := make([]sessionJSON, 0, len(payload.Sessions))
		for _, s := range payload.Sessions {
			id := ""
			if s.WorkspaceID != nil {
				id = *s.WorkspaceID
			}
			name := names[id]
			if id == "" {
				name = "Shared fallback account"
			} else if name == "" {
				name = id
			}
			out = append(out, sessionJSON{
				WorkspaceID:   id,
				WorkspaceName: name,
				Source:        s.Source,
				Status:        s.Status,
				Phone:         s.Phone,
				Connected:     s.Connected,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
	}
}

// ─── leads ────────────────────────────────────────────────────────────────────

type leadEngineLeadJSON struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Phone    string   `json:"phone"`
	Category string   `json:"category"`
	City     string   `json:"city"`
	Address  string   `json:"address"`
	Rating   *float64 `json:"rating"`
	// ReviewCount is 0 for sources that have no reviews (OpenStreetMap). Read
	// it together with SourceDetail: Rating being null is the reliable signal
	// that the business was never rated, not that it was rated badly.
	ReviewCount int     `json:"reviewCount"`
	WebsiteURL  string  `json:"websiteUrl"`
	Pitch       string  `json:"pitch"`
	Status      string  `json:"status"`
	PlaceID     *string `json:"placeId"`
	// SourceDetail is where this lead was discovered ("google_places",
	// "openstreetmap", "csv_import"), so the list can show provenance instead
	// of implying every lead came from the same directory.
	SourceDetail   string  `json:"sourceDetail"`
	SequenceStep   int     `json:"sequenceStep"`
	NextFollowUpAt *string `json:"nextFollowupAt,omitempty"`
	CreatedAt      string  `json:"createdAt"`
	// SkipReason explains why the most recent dispatch did not message this
	// lead (already contacted, opt-out, safety check unavailable, daily cap).
	SkipReason string `json:"skipReason,omitempty"`
	// LastSend* describe the lead's latest outbound attempt as recorded in the
	// outreach log: "sent" means the dispatcher accepted the message, "failed"
	// means it did not go out (LastSendError says why), "sending" means the
	// outcome was never confirmed. Unlike Status, these come from the send
	// log, so the dashboard can prove whether a message left.
	LastSendStatus string `json:"lastSendStatus,omitempty"`
	LastSendAt     string `json:"lastSendAt,omitempty"`
	LastSendError  string `json:"lastSendError,omitempty"`
}

func toLeadEngineLeadJSON(l store.Lead) leadEngineLeadJSON {
	name := ""
	if l.ContactName != nil {
		name = *l.ContactName
	}
	phone := ""
	if l.ContactPhone != nil {
		phone = *l.ContactPhone
	}
	var nextFollowUp *string
	if l.NextFollowUpAt != nil {
		formatted := l.NextFollowUpAt.Format(time.RFC3339)
		nextFollowUp = &formatted
	}
	return leadEngineLeadJSON{
		ID:             l.ID,
		Name:           name,
		Phone:          phone,
		Category:       l.Category,
		City:           l.City,
		Address:        l.Address,
		Rating:         l.Rating,
		ReviewCount:    l.ReviewCount,
		WebsiteURL:     l.WebsiteURL,
		Pitch:          l.Pitch,
		Status:         l.Status,
		PlaceID:        l.PlaceID,
		SequenceStep:   l.SequenceStep,
		NextFollowUpAt: nextFollowUp,
		SourceDetail:   l.SourceDetail,
		CreatedAt:      l.CreatedAt.Format(time.RFC3339),
	}
}

// GET /api/v1/workspaces/{id}/lead-engine/leads
func listLeadEngineLeadsHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		leads, err := ws.ListLeadEngineLeads(r.Context(), workspaceID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to list leads")
			return
		}
		report, _ := leadengine.LastDispatchReport(workspaceID)
		lastByLead, err := ws.ListLatestOutboundByLead(r.Context(), workspaceID)
		if err != nil {
			// The send confirmation is informational: a lookup failure must not
			// take the whole lead list down with it.
			slog.Default().Warn("could not load last send outcomes", "workspace_id", workspaceID, "error", err)
			lastByLead = nil
		}
		out := make([]leadEngineLeadJSON, len(leads))
		for i, l := range leads {
			out[i] = toLeadEngineLeadJSON(l)
			out[i].SkipReason = report.Skips[l.ID]
			if last, ok := lastByLead[l.ID]; ok {
				out[i].LastSendStatus = last.Status
				out[i].LastSendAt = last.CreatedAt.Format(time.RFC3339)
				out[i].LastSendError = last.Error
			}
		}
		writeJSON(w, http.StatusOK, out)
	}
}

type importLeadEngineItem struct {
	Name       string  `json:"name"`
	Phone      string  `json:"phone"`
	Category   string  `json:"category"`
	City       string  `json:"city"`
	Address    string  `json:"address"`
	Rating     float64 `json:"rating"`
	WebsiteURL string  `json:"websiteUrl"`
}

type importLeadEngineRequest struct {
	Leads []importLeadEngineItem `json:"leads"`
}

// POST /api/v1/workspaces/{id}/lead-engine/import
func importLeadEngineLeadsHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		var req importLeadEngineRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 5*1024*1024)).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON payload")
			return
		}
		if len(req.Leads) == 0 {
			writeJSONError(w, http.StatusBadRequest, "no leads provided in import list")
			return
		}
		if len(req.Leads) > 2000 {
			writeJSONError(w, http.StatusBadRequest, "import batch size cannot exceed 2000 leads")
			return
		}

		total := len(req.Leads)
		imported := 0
		duplicates := 0
		failed := 0

		for _, item := range req.Leads {
			phone := strings.TrimSpace(item.Phone)
			if phone == "" {
				failed++
				continue
			}
			name := strings.TrimSpace(item.Name)
			if name == "" {
				name = "Prospective Business"
			}
			category := strings.TrimSpace(item.Category)
			city := strings.TrimSpace(item.City)
			address := strings.TrimSpace(item.Address)
			websiteURL := strings.TrimSpace(item.WebsiteURL)

			// An empty Rating cell arrives as 0, and no directory rates a
			// business 0 (the scale starts at 1), so 0 means "not rated" and is
			// stored as NULL.
			var rating *float64
			if item.Rating > 0 {
				rated := item.Rating
				rating = &rated
			}
			_, err := ws.SaveDiscoveredLead(r.Context(), store.DiscoveredLead{
				WorkspaceID: workspaceID,
				Source:      "csv_import",
				Name:        name,
				Phone:       phone,
				Category:    category,
				City:        city,
				Address:     address,
				Rating:      rating,
				WebsiteURL:  websiteURL,
			})
			if err != nil {
				if errors.Is(err, store.ErrAlreadyContacted) {
					duplicates++
				} else {
					failed++
				}
				continue
			}
			imported++
		}

		writeJSON(w, http.StatusOK, map[string]int{
			"total":      total,
			"imported":   imported,
			"duplicates": duplicates,
			"failed":     failed,
		})
	}
}

// ─── outreach logs ────────────────────────────────────────────────────────────

// GET /api/v1/workspaces/{id}/lead-engine/leads/{leadId}/logs
func listLeadOutreachLogsHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		leadID := r.PathValue("leadId")
		if leadID == "" {
			writeJSONError(w, http.StatusBadRequest, "leadId required")
			return
		}
		logs, err := ws.ListOutreachLogs(r.Context(), workspaceID, leadID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to list logs")
			return
		}
		if logs == nil {
			logs = []store.LeadOutreachLog{}
		}
		writeJSON(w, http.StatusOK, logs)
	}
}

// GET /api/v1/workspaces/{id}/lead-engine/outreach?days=0
// days=0 (default) → since the start of TODAY; days=N → the last N days.
// Backs the daily activity page: each day starts empty, previous days only
// appear when the user explicitly widens the window.
func listDailyOutreachHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		days := 0
		if d := r.URL.Query().Get("days"); d != "" {
			v, err := strconv.Atoi(d)
			if err != nil || v < 0 || v > 90 {
				writeJSONError(w, http.StatusBadRequest, "days must be an integer between 0 and 90")
				return
			}
			days = v
		}
		rows, err := ws.ListRecentOutreach(r.Context(), workspaceID, days)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to list outreach")
			return
		}
		if rows == nil {
			rows = []store.OutreachActivityRow{}
		}
		writeJSON(w, http.StatusOK, rows)
	}
}

// ─── batch progress ───────────────────────────────────────────────────────────

// leadEngineProgressStore is optional: the rail gets an empty list from stores
// that cannot report backlog, and test fakes need not implement it.
type leadEngineProgressStore interface {
	ListLeadEngineProgress(ctx context.Context) ([]store.LeadEngineProgress, error)
}

// GET /api/v1/lead-engine/progress
// One poll for every workspace: queued leads, in-flight sends, last outbound.
func listLeadEngineProgressHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		progress := []store.LeadEngineProgress{}
		if lister, ok := ws.(leadEngineProgressStore); ok {
			rows, err := lister.ListLeadEngineProgress(r.Context())
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "failed to list outreach progress")
				return
			}
			if rows != nil {
				progress = rows
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"progress": progress})
	}
}

// ─── follow-up ─────────────────────────────────────────────────────────────────

// GET /api/v1/workspaces/{id}/lead-engine/contacts
// The pool of contacts this workspace has ALREADY messaged successfully —
// the candidates the Follow-up tab lists for selection.
func listMessagedContactsHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		contacts, err := ws.ListMessagedLeads(r.Context(), workspaceID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to list messaged contacts")
			return
		}
		if contacts == nil {
			contacts = []store.MessagedLead{}
		}
		writeJSON(w, http.StatusOK, contacts)
	}
}

// followUpRequest is the Follow-up tab's click payload.
type followUpRequest struct {
	Message string   `json:"message"`
	LeadIDs []string `json:"leadIds"`
}

// POST /api/v1/workspaces/{id}/lead-engine/followup
// Sends the follow-up message to the selected previously-messaged contacts in
// the background (pacing means a run takes minutes) and reports progress
// through the workspace event feed.
func triggerFollowUpHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		var in followUpRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, maxLeadEnginePayload)).Decode(&in); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if strings.TrimSpace(in.Message) == "" {
			writeJSONError(w, http.StatusBadRequest, "follow-up message is required")
			return
		}
		// A follow-up is a deliberate, bounded batch — cap the selection the
		// same way the daily limit bounds cold outreach.
		if len(in.LeadIDs) == 0 || len(in.LeadIDs) > 200 {
			writeJSONError(w, http.StatusBadRequest, "select between 1 and 200 contacts")
			return
		}
		// Dedupe the selection: the same contact ticked twice must not
		// receive a doubled message.
		seen := make(map[string]struct{}, len(in.LeadIDs))
		uniqueIDs := make([]string, 0, len(in.LeadIDs))
		for _, id := range in.LeadIDs {
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			uniqueIDs = append(uniqueIDs, id)
		}

		cfg, err := ws.GetLeadConfig(r.Context(), workspaceID)
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusConflict, "lead engine not configured for this workspace")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to load config")
			return
		}
		if cfg.WhatsAppStatus != "connected" {
			writeJSONError(w, http.StatusConflict, "WhatsApp is not connected for this workspace")
			return
		}

		contacts, err := ws.ListMessagedLeads(r.Context(), workspaceID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to list messaged contacts")
			return
		}
		selected := make(map[string]store.MessagedLead, len(uniqueIDs))
		for _, c := range contacts {
			if _, want := seen[c.ID]; want {
				selected[c.ID] = c
			}
		}
		if len(selected) == 0 {
			writeJSONError(w, http.StatusBadRequest, "none of the selected contacts were found among messaged contacts")
			return
		}

		selectedList := make([]store.MessagedLead, 0, len(selected))
		for _, id := range uniqueIDs {
			if c, ok := selected[id]; ok {
				selectedList = append(selectedList, c)
			}
		}

		// Fail fast instead of silently queueing a follow-up behind an
		// active cold-dispatch batch.
		if unlock, locked := leadengine.TryLockDispatch(workspaceID); locked {
			unlock() // RunFollowUp re-locks for the whole batch inside its goroutine
		} else {
			writeJSONError(w, http.StatusConflict, "another send run is already in progress for this workspace; try again when it finishes")
			return
		}

		ctx := context.Background()
		go func() {
			res, runErr := leadengine.RunFollowUp(ctx, leadengine.FollowUpParams{
				WorkspaceID:      workspaceID,
				Message:          in.Message,
				SenderName:       cfg.SenderName,
				PortfolioURL:     cfg.PortfolioURL,
				Contacts:         selectedList,
				MinDelayMs:       cfg.MinDelayMs,
				MaxDelayMs:       cfg.MaxDelayMs,
				DispatchURL:      dispatcherURL(),
				DispatcherSecret: dispatcherSecret(),
			}, ws)
			msg := fmt.Sprintf("Follow-up finished: %d sent, %d failed, %d skipped of %d selected",
				res.Sent, res.Failed, res.Skipped, res.Total)
			if runErr != nil {
				msg += " — " + runErr.Error()
			}
			events.DefaultHub.Publish(workspaceID, "pitch_dispatched", msg)
		}()

		writeJSON(w, http.StatusAccepted, map[string]any{
			"status": "followup_started",
			"total":  len(selectedList),
		})
	}
}

// ─── blacklist ────────────────────────────────────────────────────────────────

// GET /api/v1/workspaces/{id}/lead-engine/blacklist
func listBlacklistHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		list, err := ws.ListBlacklist(r.Context(), workspaceID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to list blacklist")
			return
		}
		if list == nil {
			list = []store.LeadBlacklistEntry{}
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// POST /api/v1/workspaces/{id}/lead-engine/blacklist
func addToBlacklistHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		var in struct {
			Phone  string `json:"phone"`
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, maxLeadEnginePayload)).Decode(&in); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if in.Phone == "" {
			writeJSONError(w, http.StatusBadRequest, "phone is required")
			return
		}
		if err := ws.AddToBlacklist(r.Context(), workspaceID, in.Phone, in.Reason); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to add to blacklist")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
	}
}

// ─── discovery trigger ────────────────────────────────────────────────────────

// POST /api/v1/workspaces/{id}/lead-engine/discover
func triggerDiscoveryHandler(ws WorkspaceStore, credKey []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		cfg, err := ws.GetLeadConfig(r.Context(), workspaceID)
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusConflict, "lead engine not configured for this workspace")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to load config")
			return
		}
		if !cfg.Enabled {
			writeJSONError(w, http.StatusConflict, "lead engine is disabled for this workspace")
			return
		}

		// Decrypt Google Places key.
		googleKey, err := secretcipher.Decrypt(credKey, cfg.GooglePlacesAPIKeyCiphertext)
		if err != nil || len(googleKey) == 0 {
			writeJSONError(w, http.StatusConflict, "Google Places API key not configured")
			return
		}

		// Run discovery in background so the HTTP response is immediate.
		ctx := context.Background()
		go func() {
			discovered, errs := leadengine.RunDiscovery(ctx, leadengine.DiscoveryParams{
				WorkspaceID:     workspaceID,
				GooglePlacesKey: string(googleKey),
				Categories:      cfg.TargetCategories,
				Cities:          cfg.TargetCities,
				DailyLimit:      cfg.DailyLimit}, ws)
			msg := fmt.Sprintf("Discovered %d new leads", discovered)
			if len(errs) > 0 {
				// Surface failures instead of leaving an empty lead list unexplained.
				slog.Default().Warn("lead discovery finished with errors",
					"workspace_id", workspaceID, "failed_searches", len(errs), "err", errs[0])
				msg = fmt.Sprintf("Discovered %d new leads; %d search(es) failed — check the API log for the reason", discovered, len(errs))
			}
			events.DefaultHub.Publish(workspaceID, "lead_discovered", msg)
		}()

		writeJSON(w, http.StatusAccepted, map[string]string{"status": "discovery_started"})
	}
}

// ─── dispatch trigger ─────────────────────────────────────────────────────────

// POST /api/v1/workspaces/{id}/lead-engine/dispatch
func triggerDispatchHandler(ws WorkspaceStore, credKey []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		cfg, err := ws.GetLeadConfig(r.Context(), workspaceID)
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusConflict, "lead engine not configured for this workspace")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to load config")
			return
		}
		if cfg.WhatsAppStatus != "connected" {
			writeJSONError(w, http.StatusConflict, "WhatsApp is not connected for this workspace")
			return
		}

		// Decrypt AI key.
		aiKey, err := secretcipher.Decrypt(credKey, cfg.AIProviderKeyCiphertext)
		if err != nil || len(aiKey) == 0 {
			writeJSONError(w, http.StatusConflict, "AI provider key not configured")
			return
		}

		ctx := context.Background()
		go func() {
			err := leadengine.RunDispatch(ctx, leadengine.DispatchParams{
				WorkspaceID: workspaceID,
				AIKey:       string(aiKey),
				AIModel:     cfg.AIModel, SenderName: cfg.SenderName,
				PitchTemplate:      cfg.PitchTemplate,
				BusinessOffering:   cfg.BusinessOffering,
				TargetAudience:     cfg.TargetAudience,
				PortfolioURL:       cfg.PortfolioURL,
				MinDelayMs:         cfg.MinDelayMs,
				MaxDelayMs:         cfg.MaxDelayMs,
				DailyLimit:         cfg.DailyLimit,
				DispatchURL:        dispatcherURL(),
				DispatcherSecret:   dispatcherSecret(),
				EnableAutoFollowup: cfg.EnableAutoFollowup,
				Followup1DelayDays: cfg.Followup1DelayDays,
			}, ws)
			if err != nil {
				events.DefaultHub.Publish(workspaceID, "pitch_dispatched", "Dispatch run ended with error: "+err.Error())
			} else {
				msg := "Outreach dispatch completed"
				if report, ok := leadengine.LastDispatchReport(workspaceID); ok {
					msg = fmt.Sprintf("Outreach dispatch completed: %d sent, %d failed (%d unconfirmed), %d not sent — reasons are shown on each lead",
						report.Sent, report.Failed, report.Unconfirmed, len(report.Skips))
					if report.Unconfirmed > 0 {
						msg += "; unconfirmed sends are listed in Daily Activity"
					}
					if report.Halted != "" {
						msg += " — " + report.Halted
					}
				}
				events.DefaultHub.Publish(workspaceID, "pitch_dispatched", msg)
			}
		}()

		writeJSON(w, http.StatusAccepted, map[string]string{"status": "dispatch_started"})
	}
}

// ─── inbound message webhook & conversational AI auto-reply ────────────────────

type inboundWhatsAppPayload struct {
	Phone     string `json:"phone"`
	Body      string `json:"body"`
	MessageID string `json:"messageId"`
	Timestamp int64  `json:"timestamp"`
}

// POST /api/v1/workspaces/{id}/lead-engine/whatsapp/inbound
func inboundWhatsAppHandler(ws WorkspaceStore, credKey []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Verify internal secret if configured
		if sec := dispatcherSecret(); sec != "" {
			if r.Header.Get("X-Internal-Secret") != sec {
				writeJSONError(w, http.StatusUnauthorized, "invalid internal secret")
				return
			}
		}

		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}

		var in inboundWhatsAppPayload
		if err := json.NewDecoder(io.LimitReader(r.Body, maxLeadEnginePayload)).Decode(&in); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if in.Phone == "" || in.Body == "" {
			writeJSONError(w, http.StatusBadRequest, "phone and body are required")
			return
		}

		handleInboundForWorkspace(r.Context(), ws, credKey, workspaceID, in)
		writeJSON(w, http.StatusOK, map[string]string{"status": "received"})
	}
}

// POST /api/v1/lead-engine/whatsapp/inbound
// Workspace-agnostic inbound webhook: when every workspace shares one WhatsApp
// number, the dispatcher cannot tell which workspace an inbound message belongs
// to. This handler matches the sender's phone against the leads of every
// workspace and routes the message (and the AI auto-reply) to the workspace
// that owns the lead.
func sharedInboundWhatsAppHandler(ws WorkspaceStore, credKey []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Verify internal secret if configured
		if sec := dispatcherSecret(); sec != "" {
			if r.Header.Get("X-Internal-Secret") != sec {
				writeJSONError(w, http.StatusUnauthorized, "invalid internal secret")
				return
			}
		}

		var in inboundWhatsAppPayload
		if err := json.NewDecoder(io.LimitReader(r.Body, maxLeadEnginePayload)).Decode(&in); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if in.Phone == "" || in.Body == "" {
			writeJSONError(w, http.StatusBadRequest, "phone and body are required")
			return
		}

		ctx := r.Context()

		workspaces, err := ws.ListWorkspaces(ctx)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to list workspaces")
			return
		}
		for _, wsp := range workspaces {
			lead, leadErr := ws.FindLeadByPhone(ctx, wsp.ID, in.Phone)
			if leadErr != nil {
				continue
			}
			// Found the workspace that owns this lead.
			_ = ws.UpdateLeadStatus(ctx, wsp.ID, lead.ID, "replied")
			handleInboundForWorkspace(ctx, ws, credKey, wsp.ID, in)
			writeJSON(w, http.StatusOK, map[string]string{"status": "received"})
			return
		}
		// Not a known lead in any workspace: accept and drop, so the
		// dispatcher never retries webhooks for strangers.
		writeJSON(w, http.StatusOK, map[string]string{"status": "no_match"})
	}
}

// isOptOutKeyword reports whether an inbound message is a bare opt-out keyword
// (STOP, UNSUBSCRIBE, PARAR), ignoring case, surrounding spaces and punctuation.
// Only the whole message counts, so "please stop by our shop" is not an opt-out.
func isOptOutKeyword(body string) bool {
	word := strings.ToUpper(strings.Trim(strings.TrimSpace(body), ".!?"))
	switch word {
	case "STOP", "UNSUBSCRIBE", "PARAR":
		return true
	}
	return false
}

// handleInboundForWorkspace records an inbound message against a specific
// workspace (marking the matching replied lead) and fires the AI auto-reply.
func handleInboundForWorkspace(ctx context.Context, ws WorkspaceStore, credKey []byte, workspaceID string, in inboundWhatsAppPayload) {
	// 1. Find matching lead by phone
	lead, leadErr := ws.FindLeadByPhone(ctx, workspaceID, in.Phone)
	leadID := ""
	if leadErr == nil {
		leadID = lead.ID
	}

	// 2. Persist the inbound message in outreach logs
	_, _ = ws.SaveOutreachLog(ctx, store.LeadOutreachLog{
		WorkspaceID: workspaceID,
		LeadID:      leadID,
		Direction:   "inbound",
		MessageID:   in.MessageID,
		Body:        in.Body,
		Status:      "replied",
	})
	events.DefaultHub.Publish(workspaceID, "whatsapp_inbound", "Inbound WhatsApp message from "+in.Phone)

	// A STOP-style keyword is an opt-out: suppress the number for this
	// workspace so no further outreach or AI reply is sent to it.
	if isOptOutKeyword(in.Body) {
		_ = ws.AddToBlacklist(ctx, workspaceID, in.Phone, "opt_out")
		if leadID != "" {
			_ = ws.UpdateLeadStatus(ctx, workspaceID, leadID, "blacklisted")
		}
		events.DefaultHub.Publish(workspaceID, "whatsapp_opt_out", "Opt-out received from "+in.Phone)
		return
	}

	// 3. Autonomous AI Auto-Reply (runs asynchronously so webhook returns HTTP 200 immediately)
	go func() {
		bgCtx := context.Background()

		cfg, err := ws.GetLeadConfig(bgCtx, workspaceID)
		if err != nil || !cfg.Enabled || len(cfg.AIProviderKeyCiphertext) == 0 {
			return
		}

		aiKeyBytes, err := secretcipher.Decrypt(credKey, cfg.AIProviderKeyCiphertext)
		if err != nil || len(aiKeyBytes) == 0 {
			return
		}

		businessName := ""
		if lead.ContactName != nil {
			businessName = *lead.ContactName
		}

		// Generate conversational reply
		reply, err := leadengine.GenerateConversationalReply(bgCtx, leadengine.ReplyParams{
			SenderName:       cfg.SenderName,
			BusinessName:     businessName,
			BusinessOffering: cfg.BusinessOffering,
			PortfolioURL:     cfg.PortfolioURL,
			InitialPitch:     lead.Pitch,
			InboundMessage:   in.Body,
			AIKey:            string(aiKeyBytes),
			AIModel:          cfg.AIModel,
		})
		if err != nil || reply == "" {
			return
		}

		// Send reply via WhatsApp dispatcher
		sendURL := dispatcherURL() + "/sessions/" + workspaceID + "/send"
		payload, _ := json.Marshal(map[string]string{
			"phone":   in.Phone,
			"message": reply,
		})
		req, err := newDispatcherRequest(bgCtx, http.MethodPost, sendURL, bytes.NewReader(payload), "application/json")
		if err != nil {
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode >= 400 {
			if resp != nil {
				resp.Body.Close()
			}
			return
		}
		resp.Body.Close()

		// Log outbound reply
		_, _ = ws.SaveOutreachLog(bgCtx, store.LeadOutreachLog{
			WorkspaceID: workspaceID,
			LeadID:      leadID,
			Direction:   "outbound",
			Body:        reply,
			Status:      "sent",
		})

		if leadID != "" {
			_ = ws.UpdateLeadStatus(bgCtx, workspaceID, leadID, "contacted")
		}

		events.DefaultHub.Publish(workspaceID, "whatsapp_replied", "Conversational AI replied to "+in.Phone)
	}()
}

// ─── dispatcher proxy helper ────────────────────────────────────────────────

// postJSON posts a JSON body to url and decodes the response into dst.
func postJSON(url string, payload any, dst any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if s := dispatcherSecret(); s != "" {
		req.Header.Set("X-Internal-Secret", s)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(dst)
}
