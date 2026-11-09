// Lead Generation Engine store — workspace lead config, discovery, outreach logs, and blacklist.
package store

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ─── phone identity (ported from gh-lead-engine/src/db.js) ───────────────────

var nonPhoneDigits = regexp.MustCompile(`\D`)

// NormalizePhone mirrors the legacy engine's normalizePhone(): keep digits
// only and canonicalise to the Ghanaian 233-prefixed form. That makes
// "+233 54 263 5494", "0542635494" and "233542635494" the same contact
// instead of three different strings.
func NormalizePhone(phone string) string {
	digits := nonPhoneDigits.ReplaceAllString(phone, "")
	if digits == "" {
		return ""
	}
	if !strings.HasPrefix(digits, "233") {
		digits = "233" + strings.TrimLeft(digits, "0")
	}
	return digits
}

// PhoneKey is the duplicate-suppression identity: the last nine digits of the
// number (Ghana's national significant number). It matches the generated
// leads.phone_key column and every SQL comparison in this file, so Go-side and
// database-side dedupe always agree.
func PhoneKey(phone string) string {
	digits := nonPhoneDigits.ReplaceAllString(phone, "")
	if len(digits) < 9 {
		return digits
	}
	return digits[len(digits)-9:]
}

// ─── types ───────────────────────────────────────────────────────────────────

// WorkspaceLeadConfig holds every per-workspace setting for the lead-gen engine.
// API keys are stored only as ciphertext and are never returned to the browser.
type WorkspaceLeadConfig struct {
	WorkspaceID                  string
	Enabled                      bool
	GooglePlacesAPIKeyCiphertext []byte
	GooglePlacesAPIKeyVersion    int
	AIProviderKeyCiphertext      []byte
	AIProviderKeyVersion         int
	AIModel                      string
	SenderName                   string
	BusinessOffering             string
	TargetAudience               string
	PortfolioURL                 string
	TargetCategories             []string
	TargetCities                 []string
	DailyLimit                   int
	MinDelayMs                   int
	MaxDelayMs                   int
	WhatsAppStatus               string // disconnected | qr_ready | connected
	WhatsAppPhone                string
	PitchTemplate                string // optional authored message; empty = AI-generated pitch
	EnableAutoFollowup           bool
	Followup1Template            string
	Followup1DelayDays           int
	Followup2Template            string
	Followup2DelayDays           int
	CreatedAt                    time.Time
	UpdatedAt                    time.Time
}

// LeadOutreachLog records one outbound or inbound WhatsApp message.
type LeadOutreachLog struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspaceId"`
	LeadID      string    `json:"leadId"`
	Direction   string    `json:"direction"` // outbound | inbound
	MessageID   string    `json:"messageId,omitempty"`
	Body        string    `json:"body"`
	Status      string    `json:"status"`
	Ack         *int      `json:"ack,omitempty"`
	Error       string    `json:"error,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

// LeadBlacklistEntry is one suppressed phone number for a workspace.
// JSON tags are required: the dashboard's Blacklist tab keys rows on `id`
// and reads phoneNumber/reason/createdAt — exported Go names alone would
// serialize as "ID" and break React list keys (duplicate `m-bl-undefined`).
type LeadBlacklistEntry struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspaceId"`
	PhoneNumber string    `json:"phoneNumber"`
	Reason      string    `json:"reason"`
	CreatedAt   time.Time `json:"createdAt"`
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
	whatsapp_status, whatsapp_phone, pitch_template,
	enable_auto_followup, followup_1_template, followup_1_delay_days,
	followup_2_template, followup_2_delay_days,
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
		&c.WhatsAppStatus, &c.WhatsAppPhone, &c.PitchTemplate,
		&c.EnableAutoFollowup, &c.Followup1Template, &c.Followup1DelayDays,
		&c.Followup2Template, &c.Followup2DelayDays,
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
		// Proven-safe pacing (legacy gh-lead-engine ran months without a
		// restriction on these values): 2–5 minutes between messages.
		c.MinDelayMs = 120000
	}
	if c.MaxDelayMs <= 0 {
		c.MaxDelayMs = 300000
	}
	if c.Followup1DelayDays <= 0 {
		c.Followup1DelayDays = 3
	}
	if c.Followup2DelayDays <= 0 {
		c.Followup2DelayDays = 4
	}
	return scanLeadConfig(s.pool.QueryRow(ctx,
		`INSERT INTO workspace_lead_configs
			(workspace_id, enabled,
			 google_places_api_key_ciphertext, google_places_api_key_version,
			 ai_provider_key_ciphertext, ai_provider_key_version, ai_model,
			 sender_name, business_offering, target_audience, portfolio_url,
			 target_categories, target_cities,
			 daily_limit, min_delay_ms, max_delay_ms,
			 whatsapp_status, whatsapp_phone, pitch_template,
			 enable_auto_followup, followup_1_template, followup_1_delay_days,
			 followup_2_template, followup_2_delay_days)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)
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
			pitch_template                       = EXCLUDED.pitch_template,
			enable_auto_followup                 = EXCLUDED.enable_auto_followup,
			followup_1_template                  = EXCLUDED.followup_1_template,
			followup_1_delay_days                = EXCLUDED.followup_1_delay_days,
			followup_2_template                  = EXCLUDED.followup_2_template,
			followup_2_delay_days                = EXCLUDED.followup_2_delay_days,
			updated_at                           = now()
		 RETURNING `+leadConfigColumns,
		c.WorkspaceID, c.Enabled,
		c.GooglePlacesAPIKeyCiphertext, c.GooglePlacesAPIKeyVersion,
		c.AIProviderKeyCiphertext, c.AIProviderKeyVersion, c.AIModel,
		c.SenderName, c.BusinessOffering, c.TargetAudience, c.PortfolioURL,
		c.TargetCategories, c.TargetCities,
		c.DailyLimit, c.MinDelayMs, c.MaxDelayMs,
		c.WhatsAppStatus, c.WhatsAppPhone, c.PitchTemplate,
		c.EnableAutoFollowup, c.Followup1Template, c.Followup1DelayDays,
		c.Followup2Template, c.Followup2DelayDays))
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

// HasRecentOutboundAttemptByPhone reports whether ANY lead in the workspace
// with the given phone had an outbound attempt within the window. Dedupe must
// be phone-based, not lead-based: the same business can be saved as several
// lead rows (it can appear under multiple place_ids in Google Places), and a
// lead-level cooldown would then happily send the same pitch twice.
// Phones are compared on their last 9 digits (digits-only), which stays stable
// across +233 / 0 / local formatting differences.
func (s *Store) HasRecentOutboundAttemptByPhone(ctx context.Context, workspaceID, phone string, within time.Duration) (bool, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*)
		 FROM lead_outreach_logs ol
		 JOIN leads l ON l.id = ol.lead_id AND l.workspace_id = ol.workspace_id
		 WHERE ol.workspace_id = $1 AND ol.direction = 'outbound'
		   AND ol.created_at > now() - make_interval(secs => $2)
		   AND RIGHT(REGEXP_REPLACE(l.contact_phone, '\D', '', 'g'), 9)
		       = RIGHT(REGEXP_REPLACE($3, '\D', '', 'g'), 9)`,
		workspaceID, int64(within.Seconds()), phone).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// HasOutboundAttemptByPhone reports whether this phone has EVER been attempted
// in the workspace — sent or failed, with no time window. This is the legacy
// engine's core rule: a contact leaves the PENDING queue on its first attempt
// and is never selected again ("already contacted" is permanent, not a
// cooldown). Only a message that was provably never handed to WhatsApp is
// retryable: those rows are logged with a `not_sent:` error prefix, because the
// dispatcher refused the request before accepting it.
func (s *Store) HasOutboundAttemptByPhone(ctx context.Context, workspaceID, phone string) (bool, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*)
		 FROM lead_outreach_logs ol
		 JOIN leads l ON l.id = ol.lead_id AND l.workspace_id = ol.workspace_id
		 WHERE ol.workspace_id = $1 AND ol.direction = 'outbound'
		   AND ol.error NOT LIKE 'not_sent:%'
		   AND RIGHT(REGEXP_REPLACE(l.contact_phone, '\D', '', 'g'), 9)
		       = RIGHT(REGEXP_REPLACE($2, '\D', '', 'g'), 9)`,
		workspaceID, phone).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// ResetStaleProcessingLeads returns lead rows stuck in 'processing' back to
// 'failed' so they become retryable. An API restart mid-send leaves rows in
// 'processing' forever (the send loop only picks up discovered/pending/failed)
// — without this, a crash during a batch permanently freezes part of the queue.
// Rows older than one hour only, so an in-flight send is never disturbed.
func (s *Store) ResetStaleProcessingLeads(ctx context.Context, workspaceID string) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE leads SET status = 'failed', updated_at = now()
		 WHERE workspace_id = $1 AND source_kind = 'lead_engine'
		   AND status = 'processing' AND updated_at < now() - interval '1 hour'`,
		workspaceID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// MessagedLead is one workspace contact that has already received a
// successful outbound message — the candidate pool for follow-ups.
type MessagedLead struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Phone       string    `json:"phone"`
	Category    string    `json:"category"`
	City        string    `json:"city"`
	LastSentAt  time.Time `json:"lastSentAt"`
	Blacklisted bool      `json:"blacklisted"`
}

// ListMessagedLeads returns every contact in the workspace with at least one
// successfully delivered outbound message, newest send first. Blacklisted
// comes along so the UI and the follow-up runner can skip dead numbers.
//
// Rows are collapsed per PHONE, not per lead: the legacy engine held one row per
// phone number, so a contact that somehow has two lead rows still appears (and
// can be followed up) exactly once.
func (s *Store) ListMessagedLeads(ctx context.Context, workspaceID string) ([]MessagedLead, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, phone, category, city, last_sent_at, blacklisted
		 FROM (
		   SELECT DISTINCT ON (sent.pkey)
		          sent.id, sent.name, sent.phone, sent.category, sent.city, sent.last_sent_at,
		          EXISTS (
		            SELECT 1 FROM lead_blacklist b
		            WHERE b.workspace_id = $1
		              AND RIGHT(REGEXP_REPLACE(b.phone_number, '\D', '', 'g'), 9) = sent.pkey
		          ) AS blacklisted
		   FROM (
		     SELECT l.id,
		            COALESCE(l.contact_name, '') AS name,
		            COALESCE(l.contact_phone, '') AS phone,
		            l.category, l.city,
		            MAX(ol.created_at) AS last_sent_at,
		            RIGHT(REGEXP_REPLACE(l.contact_phone, '\D', '', 'g'), 9) AS pkey
		     FROM lead_outreach_logs ol
		     JOIN leads l ON l.id = ol.lead_id AND l.workspace_id = ol.workspace_id
		     WHERE ol.workspace_id = $1 AND ol.direction = 'outbound' AND ol.status = 'sent'
		     GROUP BY l.id, l.contact_name, l.contact_phone, l.category, l.city
		   ) sent
		   ORDER BY sent.pkey, sent.last_sent_at DESC
		 ) contacts
		 ORDER BY last_sent_at DESC
		 LIMIT 500`,
		workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MessagedLead
	for rows.Next() {
		var m MessagedLead
		if err := rows.Scan(&m.ID, &m.Name, &m.Phone, &m.Category, &m.City,
			&m.LastSentAt, &m.Blacklisted); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// OutreachActivityRow is one outreach log entry enriched with the lead it
// belongs to, for daily activity views.
type OutreachActivityRow struct {
	LeadOutreachLog
	LeadName  string `json:"leadName"`
	LeadPhone string `json:"leadPhone"`
}

// ListRecentOutreach returns the workspace's outreach log entries created
// since the start of `days` days ago (days=0 → since the start of today),
// newest first, joined with the lead's name and phone.
func (s *Store) ListRecentOutreach(ctx context.Context, workspaceID string, days int) ([]OutreachActivityRow, error) {
	if days < 0 {
		days = 0
	}
	rows, err := s.pool.Query(ctx,
		`SELECT ol.id, ol.workspace_id, ol.lead_id, ol.direction, ol.message_id, ol.body, ol.status, ol.ack, ol.error, ol.created_at,
		       COALESCE(l.contact_name, ''), COALESCE(l.contact_phone, '')
		 FROM lead_outreach_logs ol
		 LEFT JOIN leads l ON l.id = ol.lead_id
		 WHERE ol.workspace_id = $1
		   AND ol.created_at >= date_trunc('day', now()) - make_interval(days => $2)
		 ORDER BY ol.created_at DESC
		 LIMIT 500`,
		workspaceID, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OutreachActivityRow
	for rows.Next() {
		var r OutreachActivityRow
		if err := rows.Scan(&r.ID, &r.WorkspaceID, &r.LeadID, &r.Direction, &r.MessageID,
			&r.Body, &r.Status, &r.Ack, &r.Error, &r.CreatedAt, &r.LeadName, &r.LeadPhone); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CountOutboundSentToday reports how many WhatsApp messages were actually
// recorded as sent for the workspace since the start of the current day (UTC).
// Dispatch uses this so the daily limit is global for the workspace across
// separate, concurrent, or restarted dispatch runs — not per run.
func (s *Store) CountOutboundSentToday(ctx context.Context, workspaceID string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM lead_outreach_logs
		 WHERE workspace_id = $1 AND direction = 'outbound' AND status = 'sent'
		   AND created_at >= date_trunc('day', now())`,
		workspaceID).Scan(&n)
	return n, err
}

// HasRecentOutboundAttempt reports whether an outbound WhatsApp attempt was
// logged for the lead within the given window, regardless of whether it was
// recorded as sent or failed. A failed record can still mean the message
// reached WhatsApp (failures can happen while reading the send result), so
// retries must respect the cooldown to avoid duplicate pitches to the same
// business during retry storms.
func (s *Store) HasRecentOutboundAttempt(ctx context.Context, workspaceID, leadID string, within time.Duration) (bool, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM lead_outreach_logs
		 WHERE workspace_id = $1 AND lead_id = $2 AND direction = 'outbound'
		   AND created_at > now() - make_interval(secs => $3)`,
		workspaceID, leadID, int64(within.Seconds())).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
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

// WasSearchedToday returns true if this category+city was already queried
// successfully today. Failed searches (e.g. a rejected API key) do not count,
// so they stay retryable until they succeed or return zero results.
func (s *Store) WasSearchedToday(ctx context.Context, workspaceID, dayKey, category, city string) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM lead_discovery_searches
		 WHERE workspace_id = $1 AND day_key = $2 AND category = $3 AND city = $4 AND error = ''`,
		workspaceID, dayKey, category, city).Scan(&count)
	return count > 0, err
}

// ─── lead engine lead helpers ─────────────────────────────────────────────────

// SaveDiscoveredLead inserts a newly scraped business as a lead (idempotent by
// place_id, and by phone). A phone that already has a lead row in this
// workspace is never inserted again: that is the legacy "INSERT OR IGNORE on a
// UNIQUE phone_number" rule, and it is what stops the same business — which can
// surface under several Google place_ids — from entering the send queue twice.
func (s *Store) SaveDiscoveredLead(ctx context.Context, workspaceID, placeID, name, phone, category, city, address string, rating float64, reviewCount int, websiteURL string) (Lead, error) {
	if key := PhoneKey(phone); key != "" {
		var exists bool
		if err := s.pool.QueryRow(ctx,
			`SELECT EXISTS (
			   SELECT 1 FROM leads
			   WHERE workspace_id = $1 AND source_kind = 'lead_engine' AND phone_key = $2
			 )`, workspaceID, key).Scan(&exists); err != nil {
			return Lead{}, err
		}
		if exists {
			return Lead{}, ErrAlreadyContacted
		}
	}
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
	// A concurrent discovery run can slip past the check above; the unique
	// index is the real guard, so translate its violation into the same
	// sentinel instead of failing the whole search.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Lead{}, ErrAlreadyContacted
	}
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
	var err error
	if status == "replied" || status == "won" || status == "lost" || status == "blacklisted" {
		_, err = s.pool.Exec(ctx,
			`UPDATE leads SET status = $1, next_followup_at = NULL, updated_at = now()
			 WHERE id = $2 AND workspace_id = $3`,
			status, leadID, workspaceID)
	} else {
		_, err = s.pool.Exec(ctx,
			`UPDATE leads SET status = $1, updated_at = now()
			 WHERE id = $2 AND workspace_id = $3`,
			status, leadID, workspaceID)
	}
	return err
}

// AdvanceLeadCadence updates the sequence step and sets the next follow-up timestamp.
func (s *Store) AdvanceLeadCadence(ctx context.Context, workspaceID, leadID string, step int, nextFollowUp *time.Time) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE leads SET sequence_step = $1, next_followup_at = $2, updated_at = now()
		 WHERE id = $3 AND workspace_id = $4`,
		step, nextFollowUp, leadID, workspaceID)
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

// LeadEngineProgress is the outreach backlog of one workspace: how many leads
// are still waiting, how many are in flight, and when the last outbound
// message was actually attempted. It is derived from the leads and outreach
// tables so the API process can answer without the sender process.
type LeadEngineProgress struct {
	WorkspaceID    string     `json:"workspaceId"`
	Queued         int        `json:"queued"`
	InFlight       int        `json:"inFlight"`
	LastOutboundAt *time.Time `json:"lastOutboundAt,omitempty"`
}

// ListLeadEngineProgress reports the backlog of every workspace that has
// outreach leads, in one query, so a single poll can drive a live rail.
func (s *Store) ListLeadEngineProgress(ctx context.Context) ([]LeadEngineProgress, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT l.workspace_id,
		        count(*) FILTER (WHERE l.status IN ('discovered','pending','failed'))::int AS queued,
		        count(*) FILTER (WHERE l.status = 'processing')::int AS in_flight,
		        (SELECT max(g.created_at) FROM lead_outreach_logs g
		          WHERE g.workspace_id = l.workspace_id AND g.direction = 'outbound') AS last_outbound_at
		 FROM leads l
		 WHERE l.source_kind = 'lead_engine'
		 GROUP BY l.workspace_id
		 ORDER BY l.workspace_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LeadEngineProgress{}
	for rows.Next() {
		var p LeadEngineProgress
		if err := rows.Scan(&p.WorkspaceID, &p.Queued, &p.InFlight, &p.LastOutboundAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListEnabledLeadConfigs returns all workspace lead configs where enabled is true.
func (s *Store) ListEnabledLeadConfigs(ctx context.Context) ([]WorkspaceLeadConfig, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+leadConfigColumns+`
		 FROM workspace_lead_configs
		 WHERE enabled = true`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var configs []WorkspaceLeadConfig
	for rows.Next() {
		c, err := scanLeadConfig(rows)
		if err != nil {
			return nil, err
		}
		configs = append(configs, c)
	}
	return configs, rows.Err()
}

// FindLeadByPhone searches for a lead in a workspace matching the given phone number,
// matching both with and without leading '+'. Returns ErrNotFound if no match.
func (s *Store) FindLeadByPhone(ctx context.Context, workspaceID, phone string) (Lead, error) {
	cleanPhone := strings.TrimPrefix(strings.TrimSpace(phone), "+")
	withPlus := "+" + cleanPhone
	l, err := scanLead(s.pool.QueryRow(ctx,
		`SELECT `+leadColumns+`
		 FROM leads
		 WHERE workspace_id = $1
		   AND (contact_phone = $2 OR contact_phone = $3)
		 ORDER BY created_at DESC
		 LIMIT 1`,
		workspaceID, withPlus, cleanPhone))
	if err != nil && errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, ErrNotFound
	}
	return l, err
}
