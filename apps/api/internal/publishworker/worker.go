// Package publishworker drains due publishing jobs in the background:
// scheduled publishes run at their due time, failed writes retry with
// exponential backoff up to a bounded attempt count, and anything terminal
// waits for an operator decision. Secrets are handled exactly like the HTTP
// path: credentials are decrypted only in this process and never logged.
package publishworker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"marketingops/api/internal/integration"
	"marketingops/api/internal/secretcipher"
	"marketingops/api/internal/store"
)

// RunStore is the slice of the store the worker needs.
type RunStore interface {
	ClaimDuePublishingJob(ctx context.Context, maxAttempts int, now time.Time) (store.PublishingJobRun, error)
	CompletePublishingJobSucceeded(ctx context.Context, jobID, integrationID string, externalID, externalURL *string) (store.ContentItem, error)
	CompletePublishingJobFailed(ctx context.Context, jobID, lastErr string, retryAfter *time.Time) error
	RecoverStuckPublishingJobs(ctx context.Context, olderThan time.Duration, now time.Time) (int, error)
	GetCredentialSet(ctx context.Context, integrationID string) (store.CredentialSet, error)
	AddAuditEvent(ctx context.Context, e store.AuditEvent) error
	// Reconnection plumbing (Milestone 10): a rejected/expired destination
	// credential must not leave the integration looking connected.
	SetIntegrationStatus(ctx context.Context, workspaceID, integrationID, status string) error
	UpsertCapability(ctx context.Context, integrationID string, c store.IntegrationCapability) error
}

// Defaults for the retry policy; override in Options.
const (
	DefaultInterval    = 15 * time.Second
	DefaultMaxAttempts = 5
	DefaultBaseDelay   = 30 * time.Second
	DefaultMaxDelay    = 30 * time.Minute
	// DefaultStuckAfter bounds how long a 'running' job may sit untouched
	// before the startup sweep requeues it as interrupted (the destination
	// writes are idempotent, so the retry cannot duplicate a post).
	DefaultStuckAfter = 10 * time.Minute
)

// Worker drains due publishing jobs on a ticker.
type Worker struct {
	Store       RunStore
	Registry    *integration.Registry
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

// New fills the defaults the operator did not set.
func New(opt Options) *Worker {
	return &Worker{
		Store:       opt.Store,
		Registry:    opt.Registry,
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

// Options configures a Worker.
type Options struct {
	Store         RunStore
	Registry      *integration.Registry
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

// Run drains until the context is cancelled, ticking every Interval. A store
// outage or a failing job never stops the loop: each tick is best-effort and
// the next tick retries the drain.
func (w *Worker) Run(ctx context.Context) {
	logger := w.logger()
	interval := w.Interval
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	logger.Info("publishing worker started", "interval", interval.String())

	// One post-crash sweep: jobs abandoned mid-attempt by a previous process
	// are requeued for the first drain below instead of sitting in 'running'
	// forever. The sweep never runs again on the tick.
	if recovered, err := w.Store.RecoverStuckPublishingJobs(ctx, w.StuckAfter, w.now()); err != nil {
		logger.Warn("stuck-job sweep failed", "error_class", "recovery-sweep-failed")
	} else if recovered > 0 {
		logger.Info("recovered interrupted publishing jobs", "count", recovered)
	}

	for {
		// Drain once immediately so jobs enqueued before startup run promptly.
		w.once(ctx)
		select {
		case <-ctx.Done():
			logger.Info("publishing worker stopped")
			return
		case <-ticker.C:
			w.once(ctx)
		}
	}
}

// once claims and runs every due job it can find, serially. A claim race with
// another worker is harmless: each row is claimed under SKIP LOCKED, so both
// workers work, on distinct jobs.
func (w *Worker) once(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		run, err := w.Store.ClaimDuePublishingJob(ctx, w.MaxAttempts, w.now())
		if errors.Is(err, store.ErrNotFound) {
			return // nothing due
		}
		if err != nil {
			w.logger().Warn("claiming due publishing job failed", "error_class", "claim-failed")
			return
		}
		w.runOne(ctx, run)
	}
}

func (w *Worker) runOne(ctx context.Context, run store.PublishingJobRun) {
	logger := w.logger()
	logger = logger.With("jobId", run.Job.ID, "workspace", run.Workspace.Slug,
		"item", run.Item.ID, "version", run.Version.VersionNumber)

	publisher, err := w.buildPublisher(ctx, run)
	if err != nil {
		// A missing adapter or unusable credentials will not heal on its own:
		// no retry slot is reserved for setup failures.
		w.failJob(ctx, run, err, logger, false)
		return
	}

	payload := workerPayload(run)
	logger = logger.With("integration", run.Integration.AdapterKind)
	// Draftless destinations (Meta) have no external draft step; the skip is
	// explicit instead of a swallowed error.
	if _, _, draftErr := publisher.CreateDraft(ctx, payload); draftErr != nil && !errors.Is(draftErr, integration.ErrNoDestinationDraft) {
		w.failJob(ctx, run, draftErr, logger, w.retryable(run.Job.Attempts))
		return
	}
	outcome, publishErr := publisher.Publish(ctx, payload)
	if publishErr != nil {
		w.failJob(ctx, run, publishErr, logger, w.retryable(run.Job.Attempts))
		return
	}

	// Read-only URL verification, recorded with the artifact but never blocking
	// success: the destination site may deploy slower than the write.
	verification := integration.URLVerification{}
	if website := derefString(run.Workspace.WebsiteURL); website != "" {
		verification = publisher.VerifyPublicURL(ctx, website, payload.Slug)
	}
	var externalURL *string
	if verification.URL != "" {
		externalURL = &verification.URL
	}
	publishedID := outcome.ExternalID

	if _, err := w.Store.CompletePublishingJobSucceeded(ctx, run.Job.ID, run.Integration.ID, &publishedID, externalURL); err != nil {
		// The external write landed; keep the attempt open for the operator to
		// reconcile instead of re-running the publish automatically.
		logger.Error("succeeded completion failed", "error_class", "job-completion-failed")
		return
	}

	logger.Info("publishing job succeeded", "url", verification.URL,
		"verified", verification.Verified, "attempt", run.Job.Attempts)
	w.audit(ctx, store.AuditEvent{
		WorkspaceID: &run.Workspace.ID,
		Action:      "publishing_job_succeeded",
		EntityType:  "content_item",
		EntityID:    &run.Item.ID,
		Details: map[string]any{
			"jobId":       run.Job.ID,
			"attempt":     run.Job.Attempts,
			"publishedId": publishedID,
			"url":         verification.URL,
			"verified":    verification.Verified,
			"version":     run.Version.VersionNumber,
		},
	})
}

// failJob records the failure and either schedules the next attempt with
// exponential backoff (attempts < MaxAttempts) or leaves the job terminal
// until an operator retries it.
func (w *Worker) failJob(ctx context.Context, run store.PublishingJobRun, cause error, logger *slog.Logger, retryable bool) {
	var retryAfter *time.Time
	if retryable {
		delay := w.backoff(run.Job.Attempts)
		when := w.now().Add(delay)
		retryAfter = &when
		logger.Warn("publishing attempt failed; retrying",
			"attempt", run.Job.Attempts, "nextAttemptIn", delay.String())
	} else {
		logger.Error("publishing job failed terminally",
			"attempt", run.Job.Attempts, "maxAttempts", w.MaxAttempts)
	}
	if err := w.Store.CompletePublishingJobFailed(ctx, run.Job.ID, safeErrorText(cause), retryAfter); err != nil {
		logger.Warn("failed-completion bookkeeping failed", "error_class", "job-completion-failed")
	}
	// A rejected or expired destination credential won't heal by itself: flag
	// the integration so the dashboard prompts reconnection instead of leaving
	// a dead destination looking connected (Milestone 10). Best effort — the
	// job's own failure state above is what the operator consults first.
	if integration.PublisherNeedsReconnection(cause) {
		if err := w.Store.SetIntegrationStatus(ctx, run.Workspace.ID, run.Integration.ID, integration.LifecycleError); err != nil {
			logger.Warn("reconnection-state update failed", "error_class", "integration-status-failed")
		}
		detail := safeErrorText(cause)
		if err := w.Store.UpsertCapability(ctx, run.Integration.ID, store.IntegrationCapability{
			IntegrationID: run.Integration.ID,
			Capability:    integration.CapPublish,
			Status:        integration.StatusFailed,
			Details:       &detail,
		}); err != nil {
			logger.Warn("capability detail update failed", "error_class", "capability-update-failed")
		}
	}
	w.audit(ctx, store.AuditEvent{
		WorkspaceID: &run.Workspace.ID,
		Action:      "publishing_job_failed",
		EntityType:  "content_item",
		EntityID:    &run.Item.ID,
		Details: map[string]any{
			"jobId":   run.Job.ID,
			"attempt": run.Job.Attempts,
			"retry":   retryable,
			"reason":  safeErrorText(cause),
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

// buildPublisher mirrors the HTTP publish path: credentials are decrypted
// server-side and handed to the registered adapter; nothing secret is logged.
func (w *Worker) buildPublisher(ctx context.Context, run store.PublishingJobRun) (integration.Publisher, error) {
	if w.Registry == nil {
		return nil, errors.New("publishing adapters are unavailable")
	}
	adapter, ok := w.Registry.Publisher(run.Integration.AdapterKind)
	if !ok {
		return nil, integration.ErrPublisherUnsupported
	}
	if _, ok := w.Registry.Get(run.Integration.AdapterKind); !ok {
		return nil, integration.ErrPublisherUnsupported
	}
	credRow, err := w.Store.GetCredentialSet(ctx, run.Integration.ID)
	if err != nil || len(w.Key) == 0 {
		return nil, integration.ErrPublisherCredentialsMissing
	}
	credentials := map[string]string{}
	opened, err := secretcipher.Decrypt(w.Key, credRow.Ciphertext)
	if err != nil || json.Unmarshal(opened, &credentials) != nil {
		return nil, integration.ErrPublisherCredentialsMissing
	}
	publisher, err := adapter.NewPublisher(run.Integration.Config, credentials, w.APIBase, w.Client)
	if err != nil {
		if errors.Is(err, integration.ErrPublisherNotConfigured) || errors.Is(err, integration.ErrPublisherCredentialsMissing) {
			return nil, err
		}
		return nil, errors.New("publishing adapter could not be configured")
	}
	return publisher, nil
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

// retryable reports whether another attempt slot is available for the job.
func (w *Worker) retryable(attempts int) bool {
	return attempts < w.MaxAttempts
}

func (w *Worker) now() time.Time {
	if w.Clock != nil {
		return w.Clock()
	}
	return time.Now()
}

// safeErrorText returns only explicitly declared safe provider messages.
func safeErrorText(err error) string {
	var safeErr integration.SafePublisherError
	if errors.As(err, &safeErr) {
		return safeErr.SafeMessage()
	}
	return "the publishing destination failed"
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// workerPayload assembles the immutable approved content snapshot from the
// claimed facts, matching the HTTP publish path's mapping.
func workerPayload(run store.PublishingJobRun) integration.ContentPayload {
	v := run.Version
	item := run.Item
	channels := map[string]string{}
	for name, value := range v.ChannelCopy {
		if s, ok := value.(string); ok {
			channels[strings.ToLower(strings.TrimSpace(name))] = s
		}
	}
	var mainImage *integration.PayloadMainImage
	if raw, ok := v.Metadata["mainImage"].(map[string]any); ok {
		if ref, ok := raw["ref"].(string); ok && ref != "" {
			alt, _ := raw["alt"].(string)
			src, _ := raw["src"].(string)
			mainImage = &integration.PayloadMainImage{Ref: ref, URL: src, Alt: alt}
		}
	}
	payload := integration.ContentPayload{
		ItemID: item.ID, Version: v.VersionNumber, Title: v.Title,
		Excerpt: stringOrNull(v.Excerpt), Body: v.Body,
		CallToAction: stringOrNull(v.CallToAction),
		ChannelCopy:  channels,
		MainImage:    mainImage,
	}
	if item.Slug != nil {
		payload.Slug = *item.Slug
	}
	return payload
}

func stringOrNull(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
