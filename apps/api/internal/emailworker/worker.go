// Package emailworker drains queued outbound email in the background.
//
// The HTTP path only ever writes a delivery row: a request never waits on a
// provider. This worker decrypts the workspace's own API key, sends, and
// records exactly what happened. Retries are bounded and only ever scheduled
// for failures that could plausibly succeed; a send whose outcome is unknown
// is left for a person to check, never replayed.
package emailworker

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"marketingops/api/internal/email"
	"marketingops/api/internal/secretcipher"
	"marketingops/api/internal/store"
)

// RunStore is the slice of the store the worker needs.
type RunStore interface {
	ClaimDueEmailDelivery(ctx context.Context, maxAttempts int, now time.Time) (store.EmailDelivery, error)
	CompleteEmailDeliverySent(ctx context.Context, id, providerMessageID string) error
	CompleteEmailDeliveryFailed(ctx context.Context, id, lastErr string, retryAfter *time.Time) error
	RecoverStuckEmailDeliveries(ctx context.Context, olderThan time.Duration, now time.Time) (int, error)
	GetEmailSettings(ctx context.Context, workspaceID string) (store.EmailSettings, error)
	MarkEmailSettingsStatus(ctx context.Context, workspaceID, status string, verifiedAt *time.Time) error
	AddAuditEvent(ctx context.Context, e store.AuditEvent) error
}

// Defaults for the retry policy; override in Options.
const (
	DefaultInterval    = 10 * time.Second
	DefaultMaxAttempts = 5
	DefaultBaseDelay   = 30 * time.Second
	DefaultMaxDelay    = 30 * time.Minute
	// DefaultStuckAfter bounds how long a row may sit in 'sending' before the
	// startup sweep treats it as abandoned by a previous process.
	DefaultStuckAfter = 10 * time.Minute
)

// Worker drains due email deliveries on a ticker.
type Worker struct {
	Store       RunStore
	Key         []byte
	APIBase     string
	Client      *http.Client
	Logger      *slog.Logger
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Interval    time.Duration
	StuckAfter  time.Duration
	// Clock is for tests; nil uses time.Now.
	Clock func() time.Time
}

// Options configures a Worker.
type Options struct {
	Store         RunStore
	CredentialKey []byte
	APIBase       string
	HTTPClient    *http.Client
	Logger        *slog.Logger
	Interval      time.Duration
	MaxAttempts   int
	BaseDelay     time.Duration
	MaxDelay      time.Duration
	StuckAfter    time.Duration
	Clock         func() time.Time
}

// New fills the defaults the operator did not set.
func New(opt Options) *Worker {
	return &Worker{
		Store:       opt.Store,
		Key:         opt.CredentialKey,
		APIBase:     opt.APIBase,
		Client:      opt.HTTPClient,
		Logger:      opt.Logger,
		MaxAttempts: firstPositiveInt(opt.MaxAttempts, DefaultMaxAttempts),
		BaseDelay:   firstPositive(opt.BaseDelay, DefaultBaseDelay),
		MaxDelay:    firstPositive(opt.MaxDelay, DefaultMaxDelay),
		Interval:    firstPositive(opt.Interval, DefaultInterval),
		StuckAfter:  firstPositive(opt.StuckAfter, DefaultStuckAfter),
		Clock:       opt.Clock,
	}
}

func firstPositive(d time.Duration, fallback time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return fallback
}

func firstPositiveInt(n, fallback int) int {
	if n > 0 {
		return n
	}
	return fallback
}

// Run drains until the context is cancelled, ticking every Interval.
func (w *Worker) Run(ctx context.Context) {
	logger := w.logger()
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()
	logger.Info("email worker started", "interval", w.Interval.String())

	// One post-crash sweep: rows abandoned mid-attempt become terminal with a
	// "check before resending" note instead of sitting in 'sending' forever.
	if recovered, err := w.Store.RecoverStuckEmailDeliveries(ctx, w.StuckAfter, w.now()); err != nil {
		logger.Warn("stuck-delivery sweep failed", "error_class", "recovery-sweep-failed")
	} else if recovered > 0 {
		logger.Info("closed interrupted email deliveries", "count", recovered)
	}

	for {
		w.once(ctx)
		select {
		case <-ctx.Done():
			logger.Info("email worker stopped")
			return
		case <-ticker.C:
			w.once(ctx)
		}
	}
}

// once claims and runs every due delivery it can find, serially.
func (w *Worker) once(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		d, err := w.Store.ClaimDueEmailDelivery(ctx, w.MaxAttempts, w.now())
		if errors.Is(err, store.ErrNotFound) {
			return // nothing due
		}
		if err != nil {
			w.logger().Warn("claiming due email delivery failed", "error_class", "claim-failed")
			return
		}
		w.runOne(ctx, d)
	}
}

func (w *Worker) runOne(ctx context.Context, d store.EmailDelivery) {
	logger := w.logger().With("deliveryId", d.ID, "kind", d.Kind, "attempt", d.Attempts)

	settings, err := w.Store.GetEmailSettings(ctx, d.WorkspaceID)
	if err != nil {
		w.fail(ctx, d, email.ErrNotConfigured, logger, false)
		return
	}
	apiKey := ""
	if len(w.Key) > 0 {
		if opened, decErr := secretcipher.Decrypt(w.Key, settings.APIKeyCiphertext); decErr == nil {
			apiKey = strings.TrimSpace(string(opened))
		}
	}
	if apiKey == "" {
		w.fail(ctx, d, email.ErrNotConfigured, logger, false)
		return
	}

	sender := email.Sender{APIBase: w.APIBase, APIKey: apiKey, Client: w.Client}
	messageID, sendErr := sender.Send(ctx, email.Message{
		FromName: settings.FromName,
		From:     settings.FromEmail,
		To:       d.ToEmail,
		Subject:  d.Subject,
		Text:     d.Body,
		// A stable key per row means the provider deduplicates a repeated
		// attempt instead of sending twice.
		IdempotencyKey: "delivery-" + d.ID,
	})
	if sendErr != nil {
		retry := email.Retryable(sendErr) && d.Attempts < w.MaxAttempts
		if email.Reason(sendErr) == "credential_rejected" {
			// A rejected key will not heal by itself: flag the connection so
			// the dashboard asks for a new key instead of failing quietly.
			if err := w.Store.MarkEmailSettingsStatus(ctx, d.WorkspaceID, "error", nil); err != nil {
				logger.Warn("email connection status update failed", "error_class", "settings-status-failed")
			}
		}
		w.fail(ctx, d, sendErr, logger, retry)
		return
	}

	if err := w.Store.CompleteEmailDeliverySent(ctx, d.ID, messageID); err != nil {
		// The provider accepted the message; keep the attempt open for a
		// person to reconcile rather than replaying the send.
		logger.Error("sent bookkeeping failed", "error_class", "delivery-completion-failed")
		return
	}
	logger.Info("email delivered", "messageId", messageID)
	w.audit(ctx, store.AuditEvent{
		WorkspaceID: &d.WorkspaceID,
		Action:      "email_delivery_sent",
		EntityType:  "email_delivery",
		EntityID:    &d.ID,
		Details: map[string]any{
			"kind":      d.Kind,
			"attempt":   d.Attempts,
			"messageId": messageID,
		},
	})
}

// fail records a failed attempt and either schedules the next one with
// exponential backoff or makes the delivery terminal.
func (w *Worker) fail(ctx context.Context, d store.EmailDelivery, cause error, logger *slog.Logger, retryable bool) {
	safe := email.SafeMessage(cause)
	var retryAfter *time.Time
	if retryable {
		when := w.now().Add(w.backoff(d.Attempts))
		retryAfter = &when
		logger.Warn("email attempt failed; retrying", "reason", email.Reason(cause))
	} else {
		logger.Error("email delivery failed terminally", "reason", email.Reason(cause))
	}
	if err := w.Store.CompleteEmailDeliveryFailed(ctx, d.ID, safe, retryAfter); err != nil {
		logger.Warn("failed-completion bookkeeping failed", "error_class", "delivery-completion-failed")
	}
	w.audit(ctx, store.AuditEvent{
		WorkspaceID: &d.WorkspaceID,
		Action:      "email_delivery_failed",
		EntityType:  "email_delivery",
		EntityID:    &d.ID,
		Details: map[string]any{
			"kind":    d.Kind,
			"attempt": d.Attempts,
			"retry":   retryable,
			"reason":  safe,
		},
	})
}

// backoff is exponential in the attempt number already spent, capped.
func (w *Worker) backoff(attempts int) time.Duration {
	delay := w.BaseDelay
	for i := 1; i < attempts && delay < w.MaxDelay; i++ {
		delay *= 2
	}
	if delay > w.MaxDelay {
		delay = w.MaxDelay
	}
	return delay
}

func (w *Worker) audit(ctx context.Context, e store.AuditEvent) {
	if err := w.Store.AddAuditEvent(ctx, e); err != nil {
		w.logger().Warn("audit event not recorded", "action", e.Action, "error_class", "audit-write-failed")
	}
}

func (w *Worker) logger() *slog.Logger {
	if w.Logger != nil {
		return w.Logger
	}
	return slog.Default()
}

func (w *Worker) now() time.Time {
	if w.Clock != nil {
		return w.Clock()
	}
	return time.Now()
}
