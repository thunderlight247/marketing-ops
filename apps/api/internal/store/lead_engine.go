// Lead Generation Engine store — workspace lead config, discovery, outreach logs, and blacklist.
package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ─── types ───────────────────────────────────────────────────────────────────

// WorkspaceLeadConfig holds every per-workspace setting for the lead-gen engine.
// API keys are stored only as ciphertext and are never returned to the browser.
type WorkspaceLeadConfig struct {
	WorkspaceID                     string
	Enabled                         bool
	GooglePlacesAPIKeyCiphertext    []byte
	GooglePlacesAPIKeyVersion       int
	AIProviderKeyCiphertext         []byte
	AIProviderKeyVersion            int
	AIModel                         string
	SenderName                      string
	BusinessOffering                string
	TargetAudience                  string
	PortfolioURL                    string
	TargetCategories                []string
	TargetCities                    []string
	DailyLimit                      int
	MinDelayMs                      int
	MaxDelayMs                      int
	WhatsAppStatus                  string // disconnected | qr_ready | connected
	WhatsAppPhone                   string
	CreatedAt                       time.Time
	UpdatedAt                       time.Time
}

// LeadOutreachLog records one outbound or inbound WhatsApp message.
type LeadOutreachLog struct {
	ID          string
	WorkspaceID string
	LeadID      string
	Direction   string // outbound | inbound
	MessageID   string
	Body        string
	Status      string
	Ack         *int
	Error       string
	CreatedAt   time.Time
}

// LeadBlacklistEntry is one suppressed phone number for a workspace.
type LeadBlacklistEntry struct {
	ID          string
	WorkspaceID string
	PhoneNumber string
	Reason      string
	CreatedAt   time.Time
}

// LeadDiscoverySearch records one Google Places search run.
type LeadDiscoverySearch struct {
	ID          string
	WorkspaceID string
	DayKey      string
	Category    string
	City        string
	ResultCount int
	Error       string
	SearchedAt  time.Time
}

// ─── workspace_lead_configs ───────────────────────────────────────────────────

const leadConfigColumns = `workspace_id, enabled,
	google_places_api_key_ciphertext, google_places_api_key_version,
	ai_provider_key_ciphertext, ai_provider_key_version, ai_model,
	sender_name, business_offering, target_audience, portfolio_url,
	target_categories, target_cities,
	daily_limit, min_delay_ms, max_delay_ms,
	whatsapp_status, whatsapp_phone,
	created_at, updated_at`

func scanLeadConfig(row pgx.Row) (WorkspaceLeadConfig, error) {
	var c WorkspaceLeadConfig
	err := row.Scan(
		&c.WorkspaceID, &c.Enabled,
		&c.GooglePlacesAPIKeyCiphertext, &c.GooglePlacesAPIKeyVersion,
		&c.AIProviderKeyCiphertext, &c.AIProviderKeyVersion, &c.AIModel,
		&c.SenderName, &c.BusinessOffering, &c.TargetAudience, &c.PortfolioURL,
		&c.TargetCategories, &c.TargetCities,
		&c.DailyLimit, &c.MinDelayMs, &c.MaxDelayMs,
		&c.WhatsAppStatus, &c.WhatsAppPhone,
		&c.CreatedAt, &c.UpdatedAt,
	)
	return c, err
}

// GetLeadConfig returns the lead engine config for a workspace, or ErrNotFound.
func (s *Store) GetLeadConfig(ctx context.Context, workspaceID string) (WorkspaceLeadConfig, error) {
	c, err := scanLeadConfig(s.pool.QueryRow(ctx,
		`SELECT `+leadConfigColumns+`
		 FROM workspace_lead_configs
		 WHERE workspace_id = $1`, workspaceID))
	if err != nil && errors.Is(err, pgx.ErrNoRows) {
		return WorkspaceLeadConfig{}, ErrNotFound
	}
	return c, err
}

// UpsertLeadConfig creates or fully replaces a workspace's lead engine config.
// Callers encrypt API keys before calling; the store only stores the opaque bytes.
func (s *Store) UpsertLeadConfig(ctx context.Context, c WorkspaceLeadConfig) (WorkspaceLeadConfig, error) {
	if c.AIModel == "" {
		c.AIModel = "meta-llama/llama-3.3-70b-instruct:free"
	}
	if c.DailyLimit <= 0 {
		c.DailyLimit = 20
	}
	if c.MinDelayMs <= 0 {
		c.MinDelayMs = 25000
	}
	if c.MaxDelayMs <= 0 {
		c.MaxDelayMs = 75000
	}
	return scanLeadConfig(s.pool.QueryRow(ctx,
		`INSERT INTO workspace_lead_configs
			(workspace_id, enabled,
			 google_places_api_key_ciphertext, google_places_api_key_version,
			 ai_provider_key_ciphertext, ai_provider_key_version, ai_model,
			 sender_name, business_offering, target_audience, portfolio_url,
			 target_categories, target_cities,
			 daily_limit, min_delay_ms, max_delay_ms,
			 whatsapp_status, whatsapp_phone)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		 ON CONFLICT (workspace_id) DO UPDATE SET
			enabled                              = EXCLUDED.enabled,
			google_places_api_key_ciphertext     = CASE WHEN EXCLUDED.google_places_api_key_ciphertext IS NOT NULL THEN EXCLUDED.google_places_api_key_ciphertext ELSE workspace_lead_configs.google_places_api_key_ciphertext END,
			google_places_api_key_version        = CASE WHEN EXCLUDED.google_places_api_key_ciphertext IS NOT NULL THEN EXCLUDED.google_places_api_key_version ELSE workspace_lead_configs.google_places_api_key_version END,
			ai_provider_key_ciphertext           = CASE WHEN EXCLUDED.ai_provider_key_ciphertext IS NOT NULL THEN EXCLUDED.ai_provider_key_ciphertext ELSE workspace_lead_configs.ai_provider_key_ciphertext END,
			ai_provider_key_version              = CASE WHEN EXCLUDED.ai_provider_key_ciphertext IS NOT NULL THEN EXCLUDED.ai_provider_key_version ELSE workspace_lead_configs.ai_provider_key_version END,
			ai_model                             = EXCLUDED.ai_model,
			sender_name                          = EXCLUDED.sender_name,
			business_offering                    = EXCLUDED.business_offering,
			target_audience                      = EXCLUDED.target_audience,
			portfolio_url                        = EXCLUDED.portfolio_url,
			target_categories                    = EXCLUDED.target_categories,
			target_cities                        = EXCLUDED.target_cities,
			daily_limit                          = EXCLUDED.daily_limit,
			min_delay_ms                         = EXCLUDED.min_delay_ms,
			max_delay_ms                         = EXCLUDED.max_delay_ms,
			updated_at                           = now()
		 RETURNING `+leadConfigColumns,
		c.WorkspaceID, c.Enabled,
		c.GooglePlacesAPIKeyCiphertext, c.GooglePlacesAPIKeyVersion,
		c.AIProviderKeyCiphertext, c.AIProviderKeyVersion, c.AIModel,
		c.SenderName, c.BusinessOffering, c.TargetAudience, c.PortfolioURL,
		c.TargetCategories, c.TargetCities,
		c.DailyLimit, c.MinDelayMs, c.MaxDelayMs,
		c.WhatsAppStatus, c.WhatsAppPhone))
}

// SetWhatsAppStatus updates only the WhatsApp session status and phone for a workspace.
func (s *Store) SetWhatsAppStatus(ctx context.Context, workspaceID, status, phone string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE workspace_lead_configs
		 SET whatsapp_status = $1, whatsapp_phone = $2, updated_at = now()
		 WHERE workspace_id = $3`,
		status, phone, workspaceID)
	return err
}

// ─── lead_outreach_logs ───────────────────────────────────────────────────────

// SaveOutreachLog persists one WhatsApp outbound or inbound log entry.
func (s *Store) SaveOutreachLog(ctx context.Context, l LeadOutreachLog) (LeadOutreachLog, error) {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO lead_outreach_logs
			(workspace_id, lead_id, direction, message_id, body, status, ack, error)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 RETURNING id, workspace_id, lead_id, direction, message_id, body, status, ack, error, created_at`,
		l.WorkspaceID, l.LeadID, l.Direction, l.MessageID, l.Body, l.Status, l.Ack, l.Error,
	).Scan(&l.ID, &l.WorkspaceID, &l.LeadID, &l.Direction, &l.MessageID,
		&l.Body, &l.Status, &l.Ack, &l.Error, &l.CreatedAt)
	return l, err
}

// ListOutreachLogs returns all WhatsApp log entries for a specific lead.
func (s *Store) ListOutreachLogs(ctx context.Context, workspaceID, leadID string) ([]LeadOutreachLog, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, workspace_id, lead_id, direction, message_id, body, status, ack, error, created_at
		 FROM lead_outreach_logs
		 WHERE workspace_id = $1 AND lead_id = $2
		 ORDER BY created_at ASC`, workspaceID, leadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []LeadOutreachLog
	for rows.Next() {
		var l LeadOutreachLog
		if err := rows.Scan(&l.ID, &l.WorkspaceID, &l.LeadID, &l.Direction, &l.MessageID,
			&l.Body, &l.Status, &l.Ack, &l.Error, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

// ─── lead_blacklist ───────────────────────────────────────────────────────────

// AddToBlacklist suppresses a phone number for a workspace.
func (s *Store) AddToBlacklist(ctx context.Context, workspaceID, phone, reason string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO lead_blacklist (workspace_id, phone_number, reason)
		 VALUES ($1,$2,$3)
		 ON CONFLICT (workspace_id, phone_number) DO NOTHING`,
		workspaceID, phone, reason)
	return err
}

// IsBlacklisted returns true if the phone is on the workspace's suppress list.
func (s *Store) IsBlacklisted(ctx context.Context, workspaceID, phone string) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM lead_blacklist
		 WHERE workspace_id = $1 AND phone_number = $2`,
		workspaceID, phone).Scan(&count)
	return count > 0, err
}

// ListBlacklist returns all suppressed numbers for a workspace.
func (s *Store) ListBlacklist(ctx context.Context, workspaceID string) ([]LeadBlacklistEntry, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, workspace_id, phone_number, reason, created_at
		 FROM lead_blacklist
		 WHERE workspace_id = $1
		 ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []LeadBlacklistEntry
	for rows.Next() {
		var e LeadBlacklistEntry
		if err := rows.Scan(&e.ID, &e.WorkspaceID, &e.PhoneNumber, &e.Reason, &e.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

// ─── lead_discovery_searches ──────────────────────────────────────────────────

// CountDiscoverySearchesToday returns how many Places searches have been made today for a workspace.
func (s *Store) CountDiscoverySearchesToday(ctx context.Context, workspaceID, dayKey string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM lead_discovery_searches
		 WHERE workspace_id = $1 AND day_key = $2`,
		workspaceID, dayKey).Scan(&count)
	return count, err
}

// RecordDiscoverySearch inserts or updates a Places search result for deduplication.
func (s *Store) RecordDiscoverySearch(ctx context.Context, s2 LeadDiscoverySearch) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO lead_discovery_searches
			(workspace_id, day_key, category, city, result_count, error)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 ON CONFLICT (workspace_id, day_key, category, city) DO UPDATE SET
			result_count = EXCLUDED.result_count,
			error        = EXCLUDED.error,
			searched_at  = now()`,
		s2.WorkspaceID, s2.DayKey, s2.Category, s2.City, s2.ResultCount, s2.Error)
	return err
}

// WasSearchedToday returns true if this category+city was already queried today.
func (s *Store) WasSearchedToday(ctx context.Context, workspaceID, dayKey, category, city string) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM lead_discovery_searches
		 WHERE workspace_id = $1 AND day_key = $2 AND category = $3 AND city = $4`,
		workspaceID, dayKey, category, city).Scan(&count)
	return count > 0, err
}

// ─── lead engine lead helpers ─────────────────────────────────────────────────

// SaveDiscoveredLead inserts a newly scraped business as a lead (idempotent by place_id).
func (s *Store) SaveDiscoveredLead(ctx context.Context, workspaceID, placeID, name, phone, category, city, address string, rating float64, reviewCount int, websiteURL string) (Lead, error) {
	l, err := scanLead(s.pool.QueryRow(ctx,
		`INSERT INTO leads
			(workspace_id, source_kind, source_detail, contact_name, contact_phone,
			 place_id, category, city, address, rating, review_count, website_url, status, notes)
		 VALUES ($1,'lead_engine','google_places',$2,$3,$4,$5,$6,$7,$8,$9,$10,'discovered','')
		 ON CONFLICT (workspace_id, place_id) WHERE place_id IS NOT NULL DO UPDATE SET
			status     = CASE WHEN leads.status = 'discovered' THEN 'discovered' ELSE leads.status END,
			updated_at = now()
		 RETURNING `+leadColumns,
		workspaceID, name, phone,
		placeID, category, city, address, rating, reviewCount, websiteURL,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, ErrNotFound
	}
	return l, err
}

// UpdateLeadPitch saves the generated pitch and transitions the lead to pending.
func (s *Store) UpdateLeadPitch(ctx context.Context, workspaceID, leadID, pitch string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE leads SET pitch = $1, status = 'pending', updated_at = now()
		 WHERE id = $2 AND workspace_id = $3`,
		pitch, leadID, workspaceID)
	return err
}

// UpdateLeadStatus transitions a lead to any valid lead-engine status.
func (s *Store) UpdateLeadStatus(ctx context.Context, workspaceID, leadID, status string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE leads SET status = $1, updated_at = now()
		 WHERE id = $2 AND workspace_id = $3`,
		status, leadID, workspaceID)
	return err
}

// ListLeadEngineLeads returns all lead_engine source leads for a workspace, newest first.
func (s *Store) ListLeadEngineLeads(ctx context.Context, workspaceID string) ([]Lead, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+leadColumns+`
		 FROM leads
		 WHERE workspace_id = $1
		   AND source_kind = 'lead_engine'
		 ORDER BY created_at DESC`,
		workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var leads []Lead
	for rows.Next() {
		l, err := scanLead(rows)
		if err != nil {
			return nil, err
		}
		leads = append(leads, l)
	}
	return leads, rows.Err()
}
