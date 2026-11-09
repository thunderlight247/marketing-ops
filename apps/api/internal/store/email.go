// Outbound email storage: one provider connection per workspace plus one row
// per outbound message. The provider API key is stored only as ciphertext and
// is never selected into a dashboard payload.
package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// EmailSettings is the workspace's own provider connection.
type EmailSettings struct {
	WorkspaceID string
	Provider    string
	FromName    string
	FromEmail   string
	// NotifyEmail receives a heads-up when a new lead arrives. Empty disables
	// the alert for this workspace.
	NotifyEmail      string
	APIKeyCiphertext []byte
	APIKeyVersion    int
	// WebhookSecretCiphertext authenticates the provider's delivery callbacks.
	// nil means no webhook is connected for this workspace.
	WebhookSecretCiphertext []byte
	Status                  string // unverified | connected | error
	LastVerifiedAt          *time.Time
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

const emailSettingsColumns = `workspace_id, provider, from_name, from_email, notify_email,
	api_key_ciphertext, api_key_version, webhook_secret_ciphertext, status, last_verified_at,
	created_at, updated_at`

// EmailDelivery is one queued, attempted, or finished outbound message.
type EmailDelivery struct {
	ID          string
	WorkspaceID string
	Kind        string // lead_notification | lead_reply
	LeadID      *string
	ToEmail     string
	Subject     string
	Body        string
	Status      string // queued | sending | sent | failed
	// DeliveryState is what the provider later reported: unknown | delivered |
	// delayed | bounced | complained. It is deliberately separate from Status,
	// because acceptance and delivery are different facts.
	DeliveryState     string
	DeliveryStateAt   *time.Time
	DeliveryDetail    string
	Attempts          int
	NextAttemptAt     *time.Time
	LastError         string
	ProviderMessageID string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	SentAt            *time.Time
}

const emailDeliveryColumns = `id, workspace_id, kind, lead_id, to_email, subject, body,
	status, delivery_state, delivery_state_at, delivery_detail, attempts, next_attempt_at,
	last_error, provider_message_id, created_at, updated_at, sent_at`

// emailDeliveryColumnsD is the same list table-qualified, for the claim query
// whose UPDATE ... FROM makes bare column names ambiguous.
const emailDeliveryColumnsD = `d.id, d.workspace_id, d.kind, d.lead_id, d.to_email, d.subject, d.body,
	d.status, d.delivery_state, d.delivery_state_at, d.delivery_detail, d.attempts,
	d.next_attempt_at, d.last_error, d.provider_message_id, d.created_at, d.updated_at, d.sent_at`

// validEmailStatuses mirrors the CHECK constraint in migration 0010.
var validEmailStatuses = map[string]bool{
	"unverified": true, "connected": true, "error": true,
}

// SaveEmailSettings upserts one workspace's provider connection. A changed API
// key resets the verified state, because the previously proven credential is
// no longer the one stored.
func (s *Store) SaveEmailSettings(ctx context.Context, in EmailSettings) (EmailSettings, error) {
	if !validEmailStatuses[in.Status] {
		return EmailSettings{}, ErrInvalidStatus
	}
	if in.Provider == "" {
		in.Provider = "resend"
	}
	if strings.TrimSpace(in.FromEmail) == "" {
		return EmailSettings{}, ErrNameRequired
	}
	return scanEmailSettings(s.pool.QueryRow(ctx,
		`INSERT INTO workspace_email_settings
			(workspace_id, provider, from_name, from_email, notify_email,
			 api_key_ciphertext, api_key_version, status, last_verified_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (workspace_id) DO UPDATE SET
			provider = EXCLUDED.provider,
			from_name = EXCLUDED.from_name,
			from_email = EXCLUDED.from_email,
			notify_email = EXCLUDED.notify_email,
			api_key_ciphertext = EXCLUDED.api_key_ciphertext,
			api_key_version = EXCLUDED.api_key_version,
			status = EXCLUDED.status,
			last_verified_at = EXCLUDED.last_verified_at,
			updated_at = now()
		 RETURNING `+emailSettingsColumns,
		in.WorkspaceID, in.Provider, in.FromName, in.FromEmail, in.NotifyEmail,
		in.APIKeyCiphertext, in.APIKeyVersion, in.Status, in.LastVerifiedAt))
}

// GetEmailSettings reads one workspace's connection, including the ciphertext
// the worker needs. Callers must never serialize the ciphertext.
func (s *Store) GetEmailSettings(ctx context.Context, workspaceID string) (EmailSettings, error) {
	out, err := scanEmailSettings(s.pool.QueryRow(ctx,
		`SELECT `+emailSettingsColumns+` FROM workspace_email_settings WHERE workspace_id = $1`,
		workspaceID))
	return out, mapNotFound(err)
}

// MarkEmailSettingsStatus records the outcome of a real provider call.
func (s *Store) MarkEmailSettingsStatus(ctx context.Context, workspaceID, status string, verifiedAt *time.Time) error {
	if !validEmailStatuses[status] {
		return ErrInvalidStatus
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE workspace_email_settings
		 SET status = $2, last_verified_at = CASE WHEN $3::timestamptz IS NULL THEN last_verified_at ELSE $3 END,
			updated_at = now()
		 WHERE workspace_id = $1`, workspaceID, status, verifiedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetEmailWebhookSecret stores (or with nil, clears) the signing secret the
// provider uses for this workspace's delivery callbacks. It never changes the
// sending connection or its verified status.
func (s *Store) SetEmailWebhookSecret(ctx context.Context, workspaceID string, ciphertext []byte) (EmailSettings, error) {
	out, err := scanEmailSettings(s.pool.QueryRow(ctx,
		`UPDATE workspace_email_settings
		 SET webhook_secret_ciphertext = $2, updated_at = now()
		 WHERE workspace_id = $1
		 RETURNING `+emailSettingsColumns,
		workspaceID, ciphertext))
	return out, mapNotFound(err)
}

// EmailProviderEventResult reports what a verified provider callback did.
// Found is false when no delivery in this workspace owns that message id;
// Duplicate is true when the provider retried an event already handled.
type EmailProviderEventResult struct {
	DeliveryID string
	Found      bool
	Duplicate  bool
	Updated    bool
}

// EmailProviderEvent is one verified provider callback about one message.
type EmailProviderEvent struct {
	WorkspaceID string
	EventID     string
	EventType   string
	// MessageID is the provider's id for the sent message (not the event id).
	MessageID string
	State     string // delivered | delayed | bounced | complained
	Detail    string
	At        time.Time
}

// ApplyEmailProviderEvent records one verified callback and moves the delivery
// it describes. The provider's own event id is the deduplication key, and a
// state only moves forward in time, so a replayed or out-of-order callback
// cannot overwrite a newer fact.
func (s *Store) ApplyEmailProviderEvent(ctx context.Context, in EmailProviderEvent) (EmailProviderEventResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return EmailProviderEventResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after a successful commit

	var deliveryID string
	err = tx.QueryRow(ctx,
		`SELECT id FROM email_deliveries
		 WHERE workspace_id = $1 AND provider_message_id = $2 AND provider_message_id <> ''
		 ORDER BY created_at DESC
		 LIMIT 1`, in.WorkspaceID, in.MessageID).Scan(&deliveryID)
	if errors.Is(err, pgx.ErrNoRows) {
		return EmailProviderEventResult{Found: false}, nil
	}
	if err != nil {
		return EmailProviderEventResult{}, err
	}

	tag, err := tx.Exec(ctx,
		`INSERT INTO email_provider_events
			(workspace_id, provider_event_id, delivery_id, event_type, received_at)
		 VALUES ($1, $2, $3, $4, now())
		 ON CONFLICT (workspace_id, provider_event_id) DO NOTHING`,
		in.WorkspaceID, in.EventID, deliveryID, in.EventType)
	if err != nil {
		return EmailProviderEventResult{}, err
	}
	if tag.RowsAffected() == 0 {
		return EmailProviderEventResult{DeliveryID: deliveryID, Found: true, Duplicate: true}, nil
	}

	// The newest report wins; an older callback arriving late is recorded but
	// does not rewrite what the provider said afterwards.
	tag, err = tx.Exec(ctx,
		`UPDATE email_deliveries
		 SET delivery_state = $2, delivery_state_at = $3, delivery_detail = $4, updated_at = now()
		 WHERE id = $1
		   AND (delivery_state_at IS NULL OR delivery_state_at <= $3)`, deliveryID, in.State, in.At, in.Detail)
	if err != nil {
		return EmailProviderEventResult{}, err
	}
	updated := tag.RowsAffected() > 0

	if err := tx.Commit(ctx); err != nil {
		return EmailProviderEventResult{}, err
	}
	return EmailProviderEventResult{DeliveryID: deliveryID, Found: true, Updated: updated}, nil
}

// DeleteEmailSettings disconnects a workspace's provider account. Queued
// deliveries are left in place so their history stays readable.
func (s *Store) DeleteEmailSettings(ctx context.Context, workspaceID string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM workspace_email_settings WHERE workspace_id = $1`, workspaceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// EnqueueEmailDelivery records a message to send. It returns created=false when
// the database's uniqueness rules already cover this message: one new-lead
// alert per lead, and at most one live reply per lead.
func (s *Store) EnqueueEmailDelivery(ctx context.Context, in EmailDelivery) (EmailDelivery, bool, error) {
	if in.Kind != "lead_notification" && in.Kind != "lead_reply" {
		return EmailDelivery{}, false, ErrInvalidStatus
	}
	if strings.TrimSpace(in.ToEmail) == "" || strings.TrimSpace(in.Subject) == "" {
		return EmailDelivery{}, false, ErrNameRequired
	}
	// A suppressed address (opted out, or unsubscribed) is never queued. The
	// NOT EXISTS check runs in the same statement as the insert, so a message
	// cannot slip in between the check and the write.
	row := s.pool.QueryRow(ctx,
		`INSERT INTO email_deliveries (workspace_id, kind, lead_id, to_email, subject, body)
		 SELECT $1, $2, $3, $4, $5, $6
		 WHERE NOT EXISTS (
		     SELECT 1 FROM email_suppressions
		      WHERE workspace_id = $1 AND email = lower($4)
		 )
		 ON CONFLICT DO NOTHING
		 RETURNING `+emailDeliveryColumns,
		in.WorkspaceID, in.Kind, in.LeadID, in.ToEmail, in.Subject, in.Body)
	out, err := scanEmailDelivery(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return EmailDelivery{}, false, nil
	}
	if err != nil {
		return EmailDelivery{}, false, err
	}
	return out, true, nil
}

// SuppressEmail records that a workspace must not email an address again. The
// address is stored lowercased so the match ignores case. Re-suppressing an
// address is a no-op.
func (s *Store) SuppressEmail(ctx context.Context, workspaceID, email, reason string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return ErrNameRequired
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO email_suppressions (workspace_id, email, reason)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (workspace_id, email) DO NOTHING`, workspaceID, email, reason)
	return err
}

// GetEmailDeliveryInWorkspace reads one delivery, scoped to its workspace so a
// foreign id can never be read (or retried) through another client's path.
func (s *Store) GetEmailDeliveryInWorkspace(ctx context.Context, workspaceID, id string) (EmailDelivery, error) {
	out, err := scanEmailDelivery(s.pool.QueryRow(ctx,
		`SELECT `+emailDeliveryColumns+` FROM email_deliveries
		 WHERE workspace_id = $1 AND id = $2`, workspaceID, id))
	return out, mapNotFound(err)
}

// ListEmailDeliveries returns a workspace's recent messages, newest first.
// Bodies are included because the operator may need to re-read what was sent.
func (s *Store) ListEmailDeliveries(ctx context.Context, workspaceID string, limit int) ([]EmailDelivery, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := s.pool.Query(ctx,
		`SELECT `+emailDeliveryColumns+` FROM email_deliveries
		 WHERE workspace_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2`, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EmailDelivery{}
	for rows.Next() {
		d, err := scanEmailDelivery(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// EmailDeliveryTotals is the lifetime count of a workspace's outbound
// messages. Analytics needs totals, not the newest page of rows, so it cannot
// undercount once the log grows past a list limit.
type EmailDeliveryTotals struct {
	Total     int
	Sent      int
	Delivered int
}

func (s *Store) EmailDeliveryTotals(ctx context.Context, workspaceID string) (EmailDeliveryTotals, error) {
	var t EmailDeliveryTotals
	err := s.pool.QueryRow(ctx,
		`SELECT count(*),
		        count(*) FILTER (WHERE status = 'sent'),
		        count(*) FILTER (WHERE delivery_state = 'delivered')
		 FROM email_deliveries WHERE workspace_id = $1`,
		workspaceID).Scan(&t.Total, &t.Sent, &t.Delivered)
	return t, err
}

// EmailDeliveryAttention is how many outbound messages in a workspace need a
// human: sends that failed, and messages the provider reported as bounced. A
// failed send and a bounce cannot be the same row, so the two never overlap.
type EmailDeliveryAttention struct {
	Failed  int
	Bounced int
}

// EmailDeliveryAttention lists the workspaces whose delivery log contains a
// failure or a bounce, with the counts. Workspaces with a clean log are omitted
// so the caller reads "has nothing to look at" as an absent map entry.
//
// Problems an operator has already acknowledged (see
// AcknowledgeEmailDeliveryAttention) are not counted: the badge is about work
// still waiting, not about history. The problem's own timestamp is
// GREATEST(updated_at, delivery_state_at) — when the send failed, or when the
// provider reported the bounce — so a problem that arrives after the
// acknowledgement raises the badge again.
func (s *Store) EmailDeliveryAttention(ctx context.Context) (map[string]EmailDeliveryAttention, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT d.workspace_id,
				count(*) FILTER (WHERE d.status = 'failed') AS failed,
				count(*) FILTER (WHERE d.delivery_state = 'bounced') AS bounced
		 FROM email_deliveries d
		 LEFT JOIN email_delivery_acknowledgements a ON a.workspace_id = d.workspace_id
		 WHERE (d.status = 'failed' OR d.delivery_state = 'bounced')
		   AND (a.acknowledged_at IS NULL
		        OR GREATEST(d.updated_at, d.delivery_state_at) > a.acknowledged_at)
		 GROUP BY d.workspace_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]EmailDeliveryAttention{}
	for rows.Next() {
		var workspaceID string
		var a EmailDeliveryAttention
		if err := rows.Scan(&workspaceID, &a.Failed, &a.Bounced); err != nil {
			return nil, err
		}
		out[workspaceID] = a
	}
	return out, rows.Err()
}

// EmailDeliveryAcknowledgements lists when each workspace last acknowledged its
// delivery problems. The dashboard shows it so an operator knows what a cleared
// badge covers: everything recorded before that moment has been seen.
func (s *Store) EmailDeliveryAcknowledgements(ctx context.Context) (map[string]time.Time, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT workspace_id, acknowledged_at FROM email_delivery_acknowledgements`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var workspaceID string
		var at time.Time
		if err := rows.Scan(&workspaceID, &at); err != nil {
			return nil, err
		}
		out[workspaceID] = at
	}
	return out, rows.Err()
}

// AcknowledgedEmailDeliveryProblems lists the failed and bounced messages the
// last acknowledgement covered — the ones an operator dismissed — newest
// problem first. The badge only covers problems recorded after the
// acknowledgement, so together these two lists account for every problem in the
// log. A workspace that was never acknowledged returns nothing.
func (s *Store) AcknowledgedEmailDeliveryProblems(ctx context.Context, workspaceID string, limit int) ([]EmailDelivery, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := s.pool.Query(ctx,
		`SELECT `+emailDeliveryColumnsD+` FROM email_deliveries d
		 JOIN email_delivery_acknowledgements a ON a.workspace_id = d.workspace_id
		 WHERE d.workspace_id = $1
		   AND (d.status = 'failed' OR d.delivery_state = 'bounced')
		   AND GREATEST(d.updated_at, d.delivery_state_at) <= a.acknowledged_at
		 ORDER BY GREATEST(d.updated_at, d.delivery_state_at) DESC
		 LIMIT $2`, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EmailDelivery
	for rows.Next() {
		d, err := scanEmailDelivery(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// EmailDeliveryAcknowledgement returns when a workspace last acknowledged its
// delivery problems, or nil when it never has.
func (s *Store) EmailDeliveryAcknowledgement(ctx context.Context, workspaceID string) (*time.Time, error) {
	var at time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT acknowledged_at FROM email_delivery_acknowledgements WHERE workspace_id = $1`,
		workspaceID).Scan(&at)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &at, nil
}

// AcknowledgeEmailDeliveryAttention records that an operator has seen the
// workspace's current delivery problems. It returns the moment the
// acknowledgement took effect; only problems recorded after it count again.
func (s *Store) AcknowledgeEmailDeliveryAttention(ctx context.Context, workspaceID string) (time.Time, error) {
	var at time.Time
	err := s.pool.QueryRow(ctx,
		`INSERT INTO email_delivery_acknowledgements (workspace_id, acknowledged_at)
		 VALUES ($1, now())
		 ON CONFLICT (workspace_id) DO UPDATE SET acknowledged_at = now()
		 RETURNING acknowledged_at`, workspaceID).Scan(&at)
	if err != nil {
		return time.Time{}, mapNotFound(err)
	}
	return at, nil
}

// ListLeadEmailDeliveries returns every message the platform ever tried to send
// about one lead — the new-enquiry alert and each reply attempt — newest first,
// so the whole contact history of an enquiry reads in one place.
func (s *Store) ListLeadEmailDeliveries(ctx context.Context, workspaceID, leadID string, limit int) ([]EmailDelivery, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := s.pool.Query(ctx,
		`SELECT `+emailDeliveryColumns+` FROM email_deliveries
		 WHERE workspace_id = $1 AND lead_id = $2
		 ORDER BY created_at DESC
		 LIMIT $3`, workspaceID, leadID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EmailDelivery{}
	for rows.Next() {
		d, err := scanEmailDelivery(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ClaimDueEmailDelivery reserves the oldest due message for one worker. The row
// moves to 'sending' with its attempt count incremented, so two workers never
// send the same message.
func (s *Store) ClaimDueEmailDelivery(ctx context.Context, maxAttempts int, now time.Time) (EmailDelivery, error) {
	out, err := scanEmailDelivery(s.pool.QueryRow(ctx,
		`WITH due AS (
			SELECT id FROM email_deliveries
			 WHERE status IN ('queued', 'failed')
			   AND next_attempt_at IS NOT NULL
			   AND next_attempt_at <= $1
			   AND attempts < $2
			 ORDER BY next_attempt_at
			 LIMIT 1
			 FOR UPDATE SKIP LOCKED
		 )
		 UPDATE email_deliveries d
		 SET status = 'sending', attempts = d.attempts + 1, updated_at = now()
		 FROM due
		 WHERE d.id = due.id
		 RETURNING `+emailDeliveryColumnsD,
		now, maxAttempts))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EmailDelivery{}, ErrNotFound
		}
		return EmailDelivery{}, err
	}
	return out, nil
}

// CompleteEmailDeliverySent records provider acceptance. A sent reply also
// records the contact on its lead and moves a 'new' lead to 'contacted' —
// because a person asked for exactly that message to be sent.
func (s *Store) CompleteEmailDeliverySent(ctx context.Context, id, providerMessageID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after a successful commit

	var kind string
	var leadID *string
	var workspaceID string
	err = tx.QueryRow(ctx,
		`UPDATE email_deliveries
		 SET status = 'sent', provider_message_id = $2, last_error = '',
		     next_attempt_at = NULL, sent_at = now(), updated_at = now()
		 WHERE id = $1 AND status = 'sending'
		 RETURNING kind, lead_id, workspace_id`, id, providerMessageID).Scan(&kind, &leadID, &workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if kind == "lead_reply" && leadID != nil {
		if _, err := tx.Exec(ctx,
			`UPDATE leads
			 SET reply_sent_at = now(),
			     status = CASE WHEN status = 'new' THEN 'contacted' ELSE status END,
			     updated_at = now()
			 WHERE id = $1 AND workspace_id = $2`, *leadID, workspaceID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// CompleteEmailDeliveryFailed closes one failed attempt. A non-nil retryAfter
// keeps the message retryable at that time; nil makes it terminal until an
// operator acts.
func (s *Store) CompleteEmailDeliveryFailed(ctx context.Context, id, lastErr string, retryAfter *time.Time) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE email_deliveries
		 SET status = 'failed', last_error = $2, next_attempt_at = $3, updated_at = now()
		 WHERE id = $1 AND status = 'sending'`, id, lastErr, retryAfter)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

// RecoverStuckEmailDeliveries finds rows abandoned mid-attempt by a crashed
// process. Unlike publishing, a send cannot be replayed safely: the provider
// may already have accepted the message, so the row is made terminal with an
// explicit "confirm before resending" note.
func (s *Store) RecoverStuckEmailDeliveries(ctx context.Context, olderThan time.Duration, now time.Time) (int, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE email_deliveries
		 SET status = 'failed',
		     last_error = 'the worker stopped during this attempt; check with the provider before sending again',
		     next_attempt_at = NULL,
		     updated_at = now()
		 WHERE status = 'sending' AND updated_at < $1`,
		now.Add(-olderThan))
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func scanEmailSettings(row pgx.Row) (EmailSettings, error) {
	var s EmailSettings
	if err := row.Scan(&s.WorkspaceID, &s.Provider, &s.FromName, &s.FromEmail, &s.NotifyEmail,
		&s.APIKeyCiphertext, &s.APIKeyVersion, &s.WebhookSecretCiphertext, &s.Status,
		&s.LastVerifiedAt, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return EmailSettings{}, err
	}
	return s, nil
}

func scanEmailDelivery(row pgx.Row) (EmailDelivery, error) {
	var d EmailDelivery
	if err := row.Scan(&d.ID, &d.WorkspaceID, &d.Kind, &d.LeadID, &d.ToEmail, &d.Subject,
		&d.Body, &d.Status, &d.DeliveryState, &d.DeliveryStateAt, &d.DeliveryDetail,
		&d.Attempts, &d.NextAttemptAt, &d.LastError, &d.ProviderMessageID,
		&d.CreatedAt, &d.UpdatedAt, &d.SentAt); err != nil {
		return EmailDelivery{}, err
	}
	return d, nil
}
