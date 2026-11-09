// Package cadenceworker scans for CRM leads whose follow_up_at has passed and
// broadcasts a "lead_followup_overdue" SSE event to any connected dashboard
// client. It runs on a ticker — no database writes, no side effects other than
// the in-process hub publish.
//
// Design constraints:
//   - Read-only: a scan that finds nothing has zero cost and zero risk.
//   - Idempotent per tick: the same overdue lead will re-notify until the
//     operator acts; deduplication is left to the UI (it debounces by lead ID).
//   - Bounded: at most maxPerWorkspace leads per workspace per tick to keep
//     SSE payloads small.
package cadenceworker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"marketingops/api/internal/events"
	"marketingops/api/internal/store"
)

const (
	DefaultInterval = 5 * time.Minute
	maxPerWorkspace = 20
	eventType       = "lead_followup_overdue"
)

// OverdueStore is the slice of the store the worker needs.
type OverdueStore interface {
	ListOverdueFollowUpLeads(ctx context.Context, now time.Time, limit int) ([]store.OverdueLead, error)
}

// Worker checks for overdue CRM follow-ups and publishes SSE events.
type Worker struct {
	Store    OverdueStore
	Hub      *events.Hub
	Interval time.Duration
	Logger   *slog.Logger
	Clock    func() time.Time
}

// Options configures a Worker.
type Options struct {
	Store    OverdueStore
	Hub      *events.Hub
	Interval time.Duration
	Logger   *slog.Logger
	Clock    func() time.Time
}

// New returns a Worker with defaults applied.
func New(opt Options) *Worker {
	interval := opt.Interval
	if interval <= 0 {
		interval = DefaultInterval
	}
	hub := opt.Hub
	if hub == nil {
		hub = events.DefaultHub
	}
	return &Worker{
		Store:    opt.Store,
		Hub:      hub,
		Interval: interval,
		Logger:   opt.Logger,
		Clock:    opt.Clock,
	}
}

// Run scans for overdue follow-ups every Interval until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	logger := w.logger()
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()
	logger.Info("cadence worker started", "interval", w.Interval.String())

	// Run immediately on startup so the dashboard gets notified right away.
	w.once(ctx)

	for {
		select {
		case <-ctx.Done():
			logger.Info("cadence worker stopped")
			return
		case <-ticker.C:
			w.once(ctx)
		}
	}
}

// once fetches overdue leads and publishes one SSE event per workspace.
func (w *Worker) once(ctx context.Context) {
	now := w.now()
	leads, err := w.Store.ListOverdueFollowUpLeads(ctx, now, maxPerWorkspace*50) // generous cap
	if err != nil {
		w.logger().Warn("cadence overdue scan failed", "err", err)
		return
	}
	if len(leads) == 0 {
		return
	}

	// Group by workspace and cap per workspace.
	grouped := map[string][]store.OverdueLead{}
	for _, l := range leads {
		if len(grouped[l.WorkspaceID]) < maxPerWorkspace {
			grouped[l.WorkspaceID] = append(grouped[l.WorkspaceID], l)
		}
	}

	for wsID, wsLeads := range grouped {
		payload, err := json.Marshal(map[string]any{
			"count": len(wsLeads),
			"leads": wsLeads,
		})
		if err != nil {
			continue
		}
		w.Hub.Publish(wsID, eventType, fmt.Sprintf("%s", payload))
	}
	w.logger().Info("cadence overdue published", "workspaces", len(grouped), "total_leads", len(leads))
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
