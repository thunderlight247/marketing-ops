// Outbound email endpoints: one provider connection per workspace, a test
// send that proves the connection, replies sent through it, and the delivery
// log. Nothing here sends inline: a request records a delivery row and the
// background worker performs the send, so a slow provider never blocks the
// dashboard and every attempt is traceable.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"marketingops/api/internal/email"
	"marketingops/api/internal/secretcipher"
	"marketingops/api/internal/store"
)

const (
	maxEmailSettingsPayloadBytes = 16 * 1024
	maxEmailSubjectRunes         = 200
	maxEmailBodyRunes            = 20000
)

// emailStore is the slice of the real store these handlers need. It is a local
// interface so the wider WorkspaceStore fakes used elsewhere stay untouched;
// a store that does not implement it answers 501.
type emailStore interface {
	GetEmailSettings(ctx context.Context, workspaceID string) (store.EmailSettings, error)
	SaveEmailSettings(ctx context.Context, in store.EmailSettings) (store.EmailSettings, error)
	DeleteEmailSettings(ctx context.Context, workspaceID string) error
	MarkEmailSettingsStatus(ctx context.Context, workspaceID, status string, verifiedAt *time.Time) error
	EnqueueEmailDelivery(ctx context.Context, in store.EmailDelivery) (store.EmailDelivery, bool, error)
	ListEmailDeliveries(ctx context.Context, workspaceID string, limit int) ([]store.EmailDelivery, error)
	ListLeadEmailDeliveries(ctx context.Context, workspaceID, leadID string, limit int) ([]store.EmailDelivery, error)
	GetEmailDeliveryInWorkspace(ctx context.Context, workspaceID, id string) (store.EmailDelivery, error)
	GetLeadInWorkspace(ctx context.Context, workspaceID, id string) (store.Lead, error)
	SaveLeadReplyDraft(ctx context.Context, workspaceID, leadID, subject, body string) (store.Lead, error)
	GetWorkspaceByID(ctx context.Context, id string) (store.Workspace, error)
	SetEmailWebhookSecret(ctx context.Context, workspaceID string, ciphertext []byte) (store.EmailSettings, error)
	ApplyEmailProviderEvent(ctx context.Context, in store.EmailProviderEvent) (store.EmailProviderEventResult, error)
	AcknowledgeEmailDeliveryAttention(ctx context.Context, workspaceID string) (time.Time, error)
	EmailDeliveryAcknowledgement(ctx context.Context, workspaceID string) (*time.Time, error)
	AcknowledgedEmailDeliveryProblems(ctx context.Context, workspaceID string, limit int) ([]store.EmailDelivery, error)
	AddAuditEvent(ctx context.Context, e store.AuditEvent) error
}

func emailStoreOf(ws WorkspaceStore) (emailStore, bool) {
	s, ok := ws.(emailStore)
	return s, ok && s != nil
}

// emailDeliveryTotalsStore is optional so the wider WorkspaceStore fakes stay
// untouched. A store that implements it adds lifetime counts to the delivery
// list; analytics must never mistake a 25-row page for the whole history.
type emailDeliveryTotalsStore interface {
	EmailDeliveryTotals(ctx context.Context, workspaceID string) (store.EmailDeliveryTotals, error)
}

type emailSettingsJSON struct {
	Provider    string `json:"provider"`
	FromName    string `json:"fromName"`
	FromEmail   string `json:"fromEmail"`
	NotifyEmail string `json:"notifyEmail"`
	Status      string `json:"status"`
	HasAPIKey   bool   `json:"hasApiKey"`
	// WebhookConfigured reports whether a delivery callback secret is stored.
	// WebhookPath is the URL path the provider must call; the dashboard shows
	// it next to the API base it already knows.
	WebhookConfigured bool    `json:"webhookConfigured"`
	WebhookPath       string  `json:"webhookPath"`
	LastVerifiedAt    *string `json:"lastVerifiedAt"`
	UpdatedAt         string  `json:"updatedAt"`
}

func toEmailSettingsJSON(s store.EmailSettings) emailSettingsJSON {
	var verified *string
	if s.LastVerifiedAt != nil {
		v := s.LastVerifiedAt.UTC().Format(time.RFC3339)
		verified = &v
	}
	return emailSettingsJSON{
		Provider:          s.Provider,
		FromName:          s.FromName,
		FromEmail:         s.FromEmail,
		NotifyEmail:       s.NotifyEmail,
		Status:            s.Status,
		HasAPIKey:         len(s.APIKeyCiphertext) > 0,
		WebhookConfigured: len(s.WebhookSecretCiphertext) > 0,
		WebhookPath:       emailWebhookPath(s.WorkspaceID),
		LastVerifiedAt:    verified,
		UpdatedAt:         s.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

type emailDeliveryJSON struct {
	ID                string     `json:"id"`
	Kind              string     `json:"kind"`
	LeadID            *string    `json:"leadId"`
	To                string     `json:"to"`
	Subject           string     `json:"subject"`
	Status            string     `json:"status"`
	DeliveryState     string     `json:"deliveryState"`
	DeliveryStateAt   *time.Time `json:"deliveryStateAt"`
	DeliveryDetail    string     `json:"deliveryDetail"`
	Attempts          int        `json:"attempts"`
	LastError         string     `json:"lastError"`
	ProviderMessageID string     `json:"providerMessageId"`
	CreatedAt         time.Time  `json:"createdAt"`
	SentAt            *time.Time `json:"sentAt"`
}

func toEmailDeliveryJSON(d store.EmailDelivery) emailDeliveryJSON {
	return emailDeliveryJSON{
		ID:                d.ID,
		Kind:              d.Kind,
		LeadID:            d.LeadID,
		To:                d.ToEmail,
		Subject:           d.Subject,
		Status:            d.Status,
		DeliveryState:     d.DeliveryState,
		DeliveryStateAt:   d.DeliveryStateAt,
		DeliveryDetail:    d.DeliveryDetail,
		Attempts:          d.Attempts,
		LastError:         d.LastError,
		ProviderMessageID: d.ProviderMessageID,
		CreatedAt:         d.CreatedAt.UTC(),
		SentAt:            d.SentAt,
	}
}

// GET /api/v1/workspaces/{id}/email-settings
func getEmailSettingsHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		st, ok := emailStoreOf(ws)
		if !ok {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		settings, err := st.GetEmailSettings(r.Context(), workspaceID)
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusOK, map[string]any{"configured": false, "settings": nil})
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "email settings lookup failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"configured": true,
			"settings":   toEmailSettingsJSON(settings),
		})
	}
}

// PUT /api/v1/workspaces/{id}/email-settings connects or updates this
// workspace's own provider account. The key is encrypted before it is stored
// and is never returned; an omitted key keeps the stored one.
func putEmailSettingsHandler(ws WorkspaceStore, key []byte, keyVersion int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		st, ok := emailStoreOf(ws)
		if !ok {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		body, problems := readJSONBody(w, r, maxEmailSettingsPayloadBytes)
		if problems != nil {
			return
		}
		var req struct {
			FromName    *string `json:"fromName"`
			FromEmail   *string `json:"fromEmail"`
			NotifyEmail *string `json:"notifyEmail"`
			APIKey      *string `json:"apiKey"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "the request body is not valid JSON")
			return
		}

		existing, existingErr := st.GetEmailSettings(r.Context(), workspaceID)
		if existingErr != nil && !errors.Is(existingErr, store.ErrNotFound) {
			writeJSONError(w, http.StatusInternalServerError, "email settings lookup failed")
			return
		}
		if existingErr != nil {
			existing = store.EmailSettings{}
		}

		var findProblems []string
		fromName := strings.TrimSpace(derefStringOr(req.FromName, existing.FromName))
		if utf8.RuneCountInString(fromName) > 120 {
			findProblems = append(findProblems, "from name must be 120 characters or fewer")
		}
		if headerTextUnsafe(fromName) {
			findProblems = append(findProblems, "from name must not contain line breaks")
		}
		fromEmail, ok := normalizeEmailAddress(derefStringOr(req.FromEmail, existing.FromEmail))
		if !ok {
			findProblems = append(findProblems, "from email must be a single valid email address")
		}
		notifyRaw := strings.TrimSpace(derefStringOr(req.NotifyEmail, existing.NotifyEmail))
		notifyEmail := ""
		if notifyRaw != "" {
			notifyEmail, ok = normalizeEmailAddress(notifyRaw)
			if !ok {
				findProblems = append(findProblems, "notification email must be a single valid email address")
			}
		}

		newKey := strings.TrimSpace(derefStringOr(req.APIKey, ""))
		if newKey != "" && (utf8.RuneCountInString(newKey) < 8 || utf8.RuneCountInString(newKey) > 256) {
			findProblems = append(findProblems, "API key looks wrong (8 to 256 characters expected)")
		}
		if newKey == "" && existingErr != nil {
			findProblems = append(findProblems, "an API key is required to connect email")
		}
		if len(findProblems) > 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error":    "the email settings are not valid",
				"problems": findProblems,
			})
			return
		}

		status := existing.Status
		if status == "" {
			status = "unverified"
		}
		ciphertext := existing.APIKeyCiphertext
		version := existing.APIKeyVersion
		verifiedAt := existing.LastVerifiedAt
		keyChanged := newKey != ""
		if newKey != "" {
			if len(key) == 0 {
				writeJSONError(w, http.StatusInternalServerError, "credential encryption is not configured")
				return
			}
			sealed, err := secretcipher.Encrypt(key, []byte(newKey))
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "the API key could not be stored")
				return
			}
			ciphertext = sealed
			version = keyVersion
			// A new key has not proven itself: the dashboard asks for a test
			// send before treating the connection as working.
			status = "unverified"
			verifiedAt = nil
		}

		saved, err := st.SaveEmailSettings(r.Context(), store.EmailSettings{
			WorkspaceID:      workspaceID,
			Provider:         "resend",
			FromName:         fromName,
			FromEmail:        fromEmail,
			NotifyEmail:      notifyEmail,
			APIKeyCiphertext: ciphertext,
			APIKeyVersion:    version,
			Status:           status,
			LastVerifiedAt:   verifiedAt,
		})
		if err != nil {
			if errors.Is(err, store.ErrInvalidStatus) {
				writeJSONError(w, http.StatusBadRequest, "the email settings are not valid")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "saving the email settings failed")
			return
		}
		audit(r.Context(), ws, store.AuditEvent{
			WorkspaceID: &workspaceID,
			Action:      "email_settings_saved",
			EntityType:  "workspace",
			EntityID:    &workspaceID,
			Details: map[string]any{
				"provider":    saved.Provider,
				"fromEmail":   saved.FromEmail,
				"notifyEmail": saved.NotifyEmail,
				"keyChanged":  keyChanged,
			},
		})
		// The key itself never reaches the response or the audit trail.
		writeJSON(w, http.StatusOK, map[string]any{
			"configured": true,
			"settings":   toEmailSettingsJSON(saved),
		})
	}
}

// DELETE /api/v1/workspaces/{id}/email-settings disconnects the provider.
func deleteEmailSettingsHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		st, ok := emailStoreOf(ws)
		if !ok {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		err := st.DeleteEmailSettings(r.Context(), workspaceID)
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "email is not connected for this workspace")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "disconnecting email failed")
			return
		}
		audit(r.Context(), ws, store.AuditEvent{
			WorkspaceID: &workspaceID,
			Action:      "email_settings_removed",
			EntityType:  "workspace",
			EntityID:    &workspaceID,
		})
		w.WriteHeader(http.StatusNoContent)
	}
}

// POST /api/v1/workspaces/{id}/email-settings/test sends one real message to
// the notification address. Only a provider acceptance marks the connection
// verified — there is no configuration-shaped shortcut to "connected".
func testEmailSettingsHandler(ws WorkspaceStore, key []byte, apiBase string, client *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		st, ok := emailStoreOf(ws)
		if !ok {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		settings, err := st.GetEmailSettings(r.Context(), workspaceID)
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusConflict, "connect an email account first")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "email settings lookup failed")
			return
		}
		if len(key) == 0 {
			writeJSONError(w, http.StatusInternalServerError, "credential encryption is not configured")
			return
		}
		opened, err := secretcipher.Decrypt(key, settings.APIKeyCiphertext)
		if err != nil || strings.TrimSpace(string(opened)) == "" {
			writeJSONError(w, http.StatusInternalServerError, "the stored API key could not be read; save it again")
			return
		}
		to := settings.NotifyEmail
		if to == "" {
			to = settings.FromEmail
		}
		workspace, wsErr := st.GetWorkspaceByID(r.Context(), workspaceID)
		if wsErr != nil {
			writeJSONError(w, http.StatusInternalServerError, "workspace lookup failed")
			return
		}
		sender := email.Sender{APIBase: apiBase, APIKey: string(opened), Client: client}
		messageID, sendErr := sender.Send(r.Context(), email.Message{
			FromName: settings.FromName,
			From:     settings.FromEmail,
			To:       to,
			Subject:  "Email delivery is connected — " + workspace.Name,
			Text:     testEmailBody(workspace.Name, settings),
		})
		if sendErr != nil {
			// The send failure is the answer to this request; the status flag
			// is best effort bookkeeping.
			_ = st.MarkEmailSettingsStatus(r.Context(), workspaceID, "error", nil)
			writeJSON(w, http.StatusBadGateway, map[string]any{
				"error":  email.SafeMessage(sendErr),
				"reason": email.Reason(sendErr),
			})
			return
		}
		now := time.Now().UTC()
		if err := st.MarkEmailSettingsStatus(r.Context(), workspaceID, "connected", &now); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "recording the verification failed")
			return
		}
		audit(r.Context(), ws, store.AuditEvent{
			WorkspaceID: &workspaceID,
			Action:      "email_settings_verified",
			EntityType:  "workspace",
			EntityID:    &workspaceID,
			Details:     map[string]any{"to": to, "messageId": messageID},
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"verified":          true,
			"sentTo":            to,
			"providerMessageId": messageID,
			"verifiedAt":        now.Format(time.RFC3339),
		})
	}
}

// POST /api/v1/workspaces/{id}/leads/{leadId}/reply-send sends the reply the
// operator approved. The request only queues it; the worker performs the send
// and records what the provider answered.
func sendLeadReplyHandler(ws WorkspaceStore, key []byte, publicAPIURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		st, ok := emailStoreOf(ws)
		if !ok {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		leadID := r.PathValue("leadId")
		lead, err := st.GetLeadInWorkspace(r.Context(), workspaceID, leadID)
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "no such lead")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "lead lookup failed")
			return
		}
		if lead.Status == "won" || lead.Status == "lost" {
			writeJSONError(w, http.StatusConflict, "this lead is closed; a first reply no longer fits its state")
			return
		}
		if lead.ContactEmail == nil || strings.TrimSpace(*lead.ContactEmail) == "" {
			writeJSONError(w, http.StatusBadRequest, "this lead has no email address to reply to")
			return
		}
		to, valid := normalizeEmailAddress(*lead.ContactEmail)
		if !valid {
			writeJSONError(w, http.StatusBadRequest, "this lead's email address is not usable")
			return
		}
		if _, err := st.GetEmailSettings(r.Context(), workspaceID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSONError(w, http.StatusConflict, "connect an email account for this workspace first")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "email settings lookup failed")
			return
		}

		var req struct {
			Subject *string `json:"subject"`
			Body    *string `json:"body"`
		}
		if r.ContentLength > 0 {
			body, problems := readJSONBody(w, r, maxEmailSettingsPayloadBytes)
			if problems != nil {
				return
			}
			if err := json.Unmarshal(body, &req); err != nil {
				writeJSONError(w, http.StatusBadRequest, "the request body is not valid JSON")
				return
			}
		}

		subject := strings.TrimSpace(derefStringOr(req.Subject, ""))
		text := strings.TrimSpace(derefStringOr(req.Body, ""))
		if subject == "" && lead.ReplyDraftSubject != nil {
			subject = strings.TrimSpace(*lead.ReplyDraftSubject)
		}
		if text == "" && lead.ReplyDraftBody != nil {
			text = strings.TrimSpace(*lead.ReplyDraftBody)
		}
		var problems []string
		if subject == "" {
			problems = append(problems, "a subject is required")
		}
		if text == "" {
			problems = append(problems, "a body is required")
		}
		if utf8.RuneCountInString(subject) > maxEmailSubjectRunes {
			problems = append(problems, "the subject is too long (200 characters max)")
		}
		if utf8.RuneCountInString(text) > maxEmailBodyRunes {
			problems = append(problems, "the body is too long (20000 characters max)")
		}
		if len(problems) > 0 {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":    "there is no sendable reply for this lead yet",
				"problems": problems,
			})
			return
		}

		// Store the exact text that is about to go out, so the lead's record
		// and the delivery log agree even if the operator edited the draft.
		if _, err := st.SaveLeadReplyDraft(r.Context(), workspaceID, lead.ID, subject, text); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "storing the reply failed")
			return
		}

		// Outbound lead email carries an unsubscribe link when this API has a
		// public URL configured. The footer is added to the stored body so the
		// delivery log shows exactly what was sent.
		if link := unsubscribeURL(publicAPIURL, key, workspaceID, to); link != "" {
			text += "\n\n---\nTo stop receiving these emails, unsubscribe: " + link
		}

		delivery, created, err := st.EnqueueEmailDelivery(r.Context(), store.EmailDelivery{
			WorkspaceID: workspaceID,
			Kind:        "lead_reply",
			LeadID:      &lead.ID,
			ToEmail:     to,
			Subject:     subject,
			Body:        text,
		})
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "queueing the reply failed")
			return
		}
		if !created {
			writeJSONError(w, http.StatusConflict, "a reply to this lead is already queued or was already sent")
			return
		}
		audit(r.Context(), ws, store.AuditEvent{
			WorkspaceID: &workspaceID,
			Action:      "email_reply_queued",
			EntityType:  "lead",
			EntityID:    &lead.ID,
			Details:     map[string]any{"deliveryId": delivery.ID, "to": to},
		})
		writeJSON(w, http.StatusAccepted, map[string]any{"delivery": toEmailDeliveryJSON(delivery)})
	}
}

// GET /api/v1/workspaces/{id}/email-deliveries lists recent outbound messages
// with their real provider outcome.
func listEmailDeliveriesHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		st, ok := emailStoreOf(ws)
		if !ok {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		limit := 25
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		rows, err := st.ListEmailDeliveries(r.Context(), workspaceID, limit)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "delivery listing failed")
			return
		}
		out := make([]emailDeliveryJSON, 0, len(rows))
		for _, d := range rows {
			out = append(out, toEmailDeliveryJSON(d))
		}
		payload := map[string]any{"deliveries": out}
		if counter, ok := st.(emailDeliveryTotalsStore); ok {
			totals, err := counter.EmailDeliveryTotals(r.Context(), workspaceID)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "delivery summary failed")
				return
			}
			payload["totals"] = map[string]int{
				"total":     totals.Total,
				"sent":      totals.Sent,
				"delivered": totals.Delivered,
			}
		}
		writeJSON(w, http.StatusOK, payload)
	}
}

// GET /api/v1/workspaces/{id}/leads/{leadId}/email-deliveries is the sending
// history of one enquiry: the alert it produced and every reply attempt, in one
// place. The lead is looked up first so an unknown or foreign id is a 404 rather
// than an empty list that hides a wrong link.
func listLeadEmailDeliveriesHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		st, ok := emailStoreOf(ws)
		if !ok {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		leadID := r.PathValue("leadId")
		if _, err := st.GetLeadInWorkspace(r.Context(), workspaceID, leadID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "no such lead")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "lead lookup failed")
			return
		}
		limit := 25
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		rows, err := st.ListLeadEmailDeliveries(r.Context(), workspaceID, leadID, limit)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "delivery listing failed")
			return
		}
		out := make([]emailDeliveryJSON, 0, len(rows))
		for _, d := range rows {
			out = append(out, toEmailDeliveryJSON(d))
		}
		writeJSON(w, http.StatusOK, map[string]any{"deliveries": out})
	}
}

// POST /api/v1/workspaces/{id}/email-deliveries/{deliveryId}/retry queues a
// fresh attempt at a reply that never reached the person: either the send
// failed, or the message was accepted and then bounced. It copies the exact
// text that was logged, never the lead's draft (which the operator may have
// changed since), and the database's one-live-reply rule still decides whether
// a new attempt is allowed — so this can never email a person twice.
func retryEmailDeliveryHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		st, ok := emailStoreOf(ws)
		if !ok {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		original, err := st.GetEmailDeliveryInWorkspace(r.Context(), workspaceID, r.PathValue("deliveryId"))
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "no such delivery")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "delivery lookup failed")
			return
		}
		if original.Kind != "lead_reply" {
			writeJSONError(w, http.StatusConflict, "only a lead reply can be retried; a new-lead alert is sent once per enquiry")
			return
		}
		// A failed send never reached the recipient. A bounced message did reach
		// their server and was refused, so neither one is a contact — and the
		// bounce moved the row out of the live set, exactly so it can be resent.
		bounced := original.Status == "sent" && original.DeliveryState == "bounced"
		if original.Status != "failed" && !bounced {
			writeJSONError(w, http.StatusConflict, "only a delivery that failed or bounced can be retried")
			return
		}
		if original.LeadID == nil {
			writeJSONError(w, http.StatusConflict, "the lead this reply belonged to no longer exists")
			return
		}
		if _, err := st.GetEmailSettings(r.Context(), workspaceID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSONError(w, http.StatusConflict, "connect an email account for this workspace first")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "email settings lookup failed")
			return
		}

		queued, created, err := st.EnqueueEmailDelivery(r.Context(), store.EmailDelivery{
			WorkspaceID: workspaceID,
			Kind:        "lead_reply",
			LeadID:      original.LeadID,
			ToEmail:     original.ToEmail,
			Subject:     original.Subject,
			Body:        original.Body,
		})
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "queueing the retry failed")
			return
		}
		if !created {
			writeJSONError(w, http.StatusConflict, "a reply to this lead is already queued or was already sent")
			return
		}
		audit(r.Context(), ws, store.AuditEvent{
			WorkspaceID: &workspaceID,
			Action:      "email_delivery_retried",
			EntityType:  "email_delivery",
			EntityID:    &queued.ID,
			Details: map[string]any{
				"retriedDeliveryId": original.ID,
				"to":                queued.ToEmail,
				"wasBounced":        bounced,
			},
		})
		writeJSON(w, http.StatusAccepted, map[string]any{"delivery": toEmailDeliveryJSON(queued)})
	}
}

// GET /api/v1/workspaces/{id}/email-deliveries/acknowledged lists the problems
// an operator dismissed: the failed sends and bounces the last acknowledgement
// covered, newest problem first. The badge covers everything after that
// moment, so the two lists together account for every problem in the log.
func listAcknowledgedEmailProblemsHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		st, ok := emailStoreOf(ws)
		if !ok {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		limit := 25
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		at, err := st.EmailDeliveryAcknowledgement(r.Context(), workspaceID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "the acknowledgement lookup failed")
			return
		}
		rows, err := st.AcknowledgedEmailDeliveryProblems(r.Context(), workspaceID, limit)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "acknowledged problem listing failed")
			return
		}
		out := make([]emailDeliveryJSON, 0, len(rows))
		for _, d := range rows {
			out = append(out, toEmailDeliveryJSON(d))
		}
		// acknowledgedAt is null when nothing was ever dismissed, so the panel
		// can tell "never acknowledged" from "acknowledged, nothing to show".
		var acknowledgedAt *string
		if at != nil {
			formatted := at.UTC().Format(time.RFC3339)
			acknowledgedAt = &formatted
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"acknowledgedAt": acknowledgedAt,
			"deliveries":     out,
		})
	}
}

// POST /api/v1/workspaces/{id}/delivery-attention/acknowledge lets an operator
// clear the overview's delivery-problem badge once they have dealt with what the
// log showed. The acknowledgement is a timestamp, not a deletion: the log keeps
// every message, and a failure or bounce recorded after it raises the badge
// again — so dismissing can never hide a new problem.
func acknowledgeDeliveryAttentionHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		st, ok := emailStoreOf(ws)
		if !ok {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		at, err := st.AcknowledgeEmailDeliveryAttention(r.Context(), workspaceID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "acknowledging the delivery problems failed")
			return
		}
		audit(r.Context(), ws, store.AuditEvent{
			WorkspaceID: &workspaceID,
			Action:      "email_delivery_attention_acknowledged",
			EntityType:  "workspace",
			EntityID:    &workspaceID,
			Details:     map[string]any{"acknowledgedAt": at.UTC().Format(time.RFC3339)},
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"acknowledgedAt": at.UTC().Format(time.RFC3339),
		})
	}
}

// queueNewLeadEmail writes the new-lead alert for a workspace that has a
// notification address. It is deliberately best effort and never surfaces an
// error to the lead-capturing request: capturing the enquiry matters more than
// the alert about it. The (workspace, lead) unique index makes it exactly-once
// even when a form retries or two request paths record the same lead.
func queueNewLeadEmail(ctx context.Context, ws WorkspaceStore, workspaceID, leadID string) {
	st, ok := emailStoreOf(ws)
	if !ok {
		return
	}
	settings, err := st.GetEmailSettings(ctx, workspaceID)
	if err != nil || settings.NotifyEmail == "" {
		return
	}
	lead, err := st.GetLeadInWorkspace(ctx, workspaceID, leadID)
	if err != nil {
		return
	}
	workspace, err := st.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		return
	}
	if _, _, err := st.EnqueueEmailDelivery(ctx, store.EmailDelivery{
		WorkspaceID: workspaceID,
		Kind:        "lead_notification",
		LeadID:      &leadID,
		ToEmail:     settings.NotifyEmail,
		Subject:     "New enquiry: " + leadSummaryLabel(lead),
		Body:        newLeadEmailBody(workspace.Name, lead),
	}); err != nil {
		return
	}
	audit(ctx, ws, store.AuditEvent{
		WorkspaceID: &workspaceID,
		Action:      "email_lead_notification_queued",
		EntityType:  "lead",
		EntityID:    &lead.ID,
	})
}

func leadSummaryLabel(lead store.Lead) string {
	if lead.ContactName != nil && strings.TrimSpace(*lead.ContactName) != "" {
		return strings.TrimSpace(*lead.ContactName)
	}
	if lead.ContactEmail != nil && strings.TrimSpace(*lead.ContactEmail) != "" {
		return strings.TrimSpace(*lead.ContactEmail)
	}
	return "unnamed enquiry"
}

func newLeadEmailBody(workspaceName string, lead store.Lead) string {
	var b strings.Builder
	b.WriteString("A new enquiry arrived for ")
	b.WriteString(workspaceName)
	b.WriteString(".\n\n")
	b.WriteString("Name: " + leadSummaryLabel(lead) + "\n")
	if lead.ContactEmail != nil && *lead.ContactEmail != "" {
		b.WriteString("Email: " + *lead.ContactEmail + "\n")
	}
	if lead.ContactPhone != nil && *lead.ContactPhone != "" {
		b.WriteString("Phone: " + *lead.ContactPhone + "\n")
	}
	source := lead.SourceKind
	if lead.SourceDetail != "" {
		source += " (" + lead.SourceDetail + ")"
	}
	b.WriteString("Source: " + source + "\n")
	if lead.UtmCampaign != nil && *lead.UtmCampaign != "" {
		b.WriteString("Campaign: " + *lead.UtmCampaign + "\n")
	}
	b.WriteString("Recorded: " + lead.CreatedAt.UTC().Format("2006-01-02 15:04 UTC") + "\n")
	if lead.Message != nil && strings.TrimSpace(*lead.Message) != "" {
		b.WriteString("\nTheir message:\n")
		b.WriteString(strings.TrimSpace(*lead.Message))
		b.WriteString("\n")
	}
	b.WriteString("\nNothing has been sent to this person. Open the dashboard to review the enquiry and reply.")
	return b.String()
}

// emailWebhookPath is the exact path the provider must call. It is per
// workspace, so the callback itself identifies which client's signing secret
// and delivery table apply.
func emailWebhookPath(workspaceID string) string {
	return "/api/v1/email/webhooks/resend/" + workspaceID
}

// isEmailWebhookRequest exempts only the provider callback from operator auth.
// The callback authenticates itself with the workspace's signing secret, which
// is the whole point: the provider has no operator key.
func isEmailWebhookRequest(r *http.Request) bool {
	return r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/email/webhooks/")
}

// PUT /api/v1/workspaces/{id}/email-settings/webhook stores the signing secret
// for provider delivery callbacks. The secret is encrypted and never returned;
// the URL to paste into the provider travels with the settings payload.
func putEmailWebhookSecretHandler(ws WorkspaceStore, key []byte, keyVersion int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		st, ok := emailStoreOf(ws)
		if !ok {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		body, problems := readJSONBody(w, r, maxEmailSettingsPayloadBytes)
		if problems != nil {
			return
		}
		var req struct {
			WebhookSecret *string `json:"webhookSecret"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "the request body is not valid JSON")
			return
		}
		secret := strings.TrimSpace(derefStringOr(req.WebhookSecret, ""))
		if secret == "" || !email.ValidateWebhookSecret(secret) || len(secret) > 256 {
			writeJSONError(w, http.StatusBadRequest, "webhookSecret must be the signing secret from the provider's webhook page")
			return
		}
		if len(key) == 0 {
			writeJSONError(w, http.StatusInternalServerError, "credential encryption is not configured")
			return
		}
		sealed, err := secretcipher.Encrypt(key, []byte(secret))
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "the webhook secret could not be stored")
			return
		}
		saved, err := st.SetEmailWebhookSecret(r.Context(), workspaceID, sealed)
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusConflict, "connect an email account first")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "storing the webhook secret failed")
			return
		}
		audit(r.Context(), ws, store.AuditEvent{
			WorkspaceID: &workspaceID,
			Action:      "email_webhook_secret_saved",
			EntityType:  "workspace",
			EntityID:    &workspaceID,
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"configured": true,
			"settings":   toEmailSettingsJSON(saved),
		})
	}
}

// DELETE /api/v1/workspaces/{id}/email-settings/webhook removes the callback
// secret. Delivery states already recorded stay as they are.
func deleteEmailWebhookSecretHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		st, ok := emailStoreOf(ws)
		if !ok {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		saved, err := st.SetEmailWebhookSecret(r.Context(), workspaceID, nil)
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "email is not connected for this workspace")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "removing the webhook secret failed")
			return
		}
		audit(r.Context(), ws, store.AuditEvent{
			WorkspaceID: &workspaceID,
			Action:      "email_webhook_secret_removed",
			EntityType:  "workspace",
			EntityID:    &workspaceID,
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"configured": true,
			"settings":   toEmailSettingsJSON(saved),
		})
	}
}

// POST /api/v1/email/webhooks/{provider}/{workspaceID} receives the provider's
// delivery callbacks. A callback is accepted only when its signature verifies
// against that workspace's stored secret and its timestamp is fresh. Unknown
// event types and unknown messages are acknowledged and ignored: answering
// non-2xx would make the provider retry something nothing here will ever use.
func receiveEmailWebhook(ws WorkspaceStore, credentialKey []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("provider") != "resend" {
			writeJSONError(w, http.StatusNotFound, "no such webhook")
			return
		}
		workspaceID := r.PathValue("id")
		st, ok := emailStoreOf(ws)
		if !ok {
			writeJSONError(w, http.StatusNotFound, "no such webhook")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64*1024))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "the webhook payload could not be read")
			return
		}
		settings, err := st.GetEmailSettings(r.Context(), workspaceID)
		if err != nil || len(settings.WebhookSecretCiphertext) == 0 {
			// No connected webhook: nothing to verify against.
			writeJSONError(w, http.StatusNotFound, "no such webhook")
			return
		}
		if len(credentialKey) == 0 {
			writeJSONError(w, http.StatusInternalServerError, "credential encryption is not configured")
			return
		}
		opened, err := secretcipher.Decrypt(credentialKey, settings.WebhookSecretCiphertext)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "the webhook secret could not be read; save it again")
			return
		}
		event, err := email.ParseWebhook(string(opened), r.Header, body, time.Now().UTC(), email.WebhookTolerance)
		if errors.Is(err, email.ErrWebhookUnauthorized) {
			writeJSONError(w, http.StatusUnauthorized, "the webhook signature is not valid")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "the webhook payload is not readable")
			return
		}

		state, known := email.DeliveryStateFor(event.Type)
		if !known {
			writeJSON(w, http.StatusOK, map[string]any{"ignored": true, "reason": "unsupported event type"})
			return
		}
		result, err := st.ApplyEmailProviderEvent(r.Context(), store.EmailProviderEvent{
			WorkspaceID: workspaceID,
			EventID:     event.ID,
			EventType:   event.Type,
			MessageID:   event.MessageID,
			State:       state,
			Detail:      event.Detail,
			At:          event.OccurredAt,
		})
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "recording the delivery update failed")
			return
		}
		if !result.Found {
			writeJSON(w, http.StatusOK, map[string]any{"ignored": true, "reason": "unknown message"})
			return
		}
		if result.Duplicate {
			writeJSON(w, http.StatusOK, map[string]any{"duplicate": true, "deliveryId": result.DeliveryID})
			return
		}
		if result.Updated {
			audit(r.Context(), ws, store.AuditEvent{
				WorkspaceID: &workspaceID,
				Action:      "email_delivery_state_changed",
				EntityType:  "email_delivery",
				EntityID:    &result.DeliveryID,
				Details:     map[string]any{"state": state, "eventType": event.Type},
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"deliveryId":    result.DeliveryID,
			"deliveryState": state,
			"updated":       result.Updated,
		})
	}
}

func testEmailBody(workspaceName string, settings store.EmailSettings) string {
	var b strings.Builder
	b.WriteString("This is a test message from the marketing operations platform.\n\n")
	b.WriteString("Workspace: " + workspaceName + "\n")
	b.WriteString("From: " + settings.FromEmail + "\n")
	b.WriteString("Notification address: " + settings.NotifyEmail + "\n\n")
	b.WriteString("If you can read this, this workspace's email account works: lead alerts and reviewed replies will be sent through it.")
	return b.String()
}

func derefStringOr(v *string, fallback string) string {
	if v == nil {
		return fallback
	}
	return strings.TrimSpace(*v)
}

// normalizeEmailAddress accepts exactly one bare mailbox. A display-name form
// is refused because the display name is a separate field, and a header
// injection cannot survive the check.
func normalizeEmailAddress(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 320 || headerTextUnsafe(raw) {
		return "", false
	}
	addr, err := mail.ParseAddress(raw)
	if err != nil || !strings.EqualFold(addr.Address, raw) {
		return "", false
	}
	return raw, true
}

func headerTextUnsafe(s string) bool {
	return strings.ContainsAny(s, "\r\n\x00")
}
