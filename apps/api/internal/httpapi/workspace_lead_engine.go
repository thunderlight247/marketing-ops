// Lead engine HTTP handlers — config, discovery trigger, outreach logs, blacklist, WA status.
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

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

		// Load existing config to preserve keys when no new key is sent.
		existing, _ := ws.GetLeadConfig(r.Context(), workspaceID)

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
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		url := dispatcherURL() + "/sessions/" + workspaceID + "/start"
		req, err := newDispatcherRequest(r.Context(), http.MethodPost, url, nil, "application/json")
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
		// Update DB status so config page refreshes correctly.
		status, _ := result["status"].(string)
		phone, _ := result["phone"].(string)
		if status == "connected" {
			_ = ws.SetWhatsAppStatus(r.Context(), workspaceID, "connected", phone)
		} else {
			_ = ws.SetWhatsAppStatus(r.Context(), workspaceID, "qr_ready", "")
		}
		writeJSON(w, http.StatusOK, result)
	}
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
			writeJSONError(w, http.StatusInternalServerError, "could not build dispatcher request")
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			// Best-effort: still mark disconnected in DB.
			_ = ws.SetWhatsAppStatus(r.Context(), workspaceID, "disconnected", "")
			writeJSONError(w, http.StatusBadGateway, "WhatsApp dispatcher unreachable")
			return
		}
		resp.Body.Close()
		_ = ws.SetWhatsAppStatus(r.Context(), workspaceID, "disconnected", "")
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

// ─── leads ────────────────────────────────────────────────────────────────────

type leadEngineLeadJSON struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Phone       string   `json:"phone"`
	Category    string   `json:"category"`
	City        string   `json:"city"`
	Address     string   `json:"address"`
	Rating      *float64 `json:"rating"`
	ReviewCount int      `json:"reviewCount"`
	WebsiteURL  string   `json:"websiteUrl"`
	Pitch       string   `json:"pitch"`
	Status      string   `json:"status"`
	PlaceID     *string  `json:"placeId"`
	CreatedAt   string   `json:"createdAt"`
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
	return leadEngineLeadJSON{
		ID:          l.ID,
		Name:        name,
		Phone:       phone,
		Category:    l.Category,
		City:        l.City,
		Address:     l.Address,
		Rating:      l.Rating,
		ReviewCount: l.ReviewCount,
		WebsiteURL:  l.WebsiteURL,
		Pitch:       l.Pitch,
		Status:      l.Status,
		PlaceID:     l.PlaceID,
		CreatedAt:   l.CreatedAt.Format(time.RFC3339),
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
		out := make([]leadEngineLeadJSON, len(leads))
		for i, l := range leads {
			out[i] = toLeadEngineLeadJSON(l)
		}
		writeJSON(w, http.StatusOK, out)
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
				DailyLimit:      cfg.DailyLimit,
			}, ws)
			_ = discovered
			_ = errs
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
			_ = leadengine.RunDispatch(ctx, leadengine.DispatchParams{
				WorkspaceID:      workspaceID,
				AIKey:            string(aiKey),
				AIModel:          cfg.AIModel,
				SenderName:       cfg.SenderName,
				PortfolioURL:     cfg.PortfolioURL,
				MinDelayMs:       cfg.MinDelayMs,
				MaxDelayMs:       cfg.MaxDelayMs,
				DailyLimit:       cfg.DailyLimit,
				DispatchURL:      dispatcherURL(),
				DispatcherSecret: dispatcherSecret(),
			}, ws)
		}()

		writeJSON(w, http.StatusAccepted, map[string]string{"status": "dispatch_started"})
	}
}

// ─── dispatcher proxy helper ──────────────────────────────────────────────────

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
