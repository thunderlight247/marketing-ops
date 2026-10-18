// Package health produces the API status report. It contains no HTTP code, so
// it can be reused by handlers, CLI tools, and tests.
package health

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// Status is the overall API status.
type Status string

const (
	StatusOK       Status = "ok"
	StatusDegraded Status = "degraded"
)

// Database status values reported to clients.
const (
	DatabaseOK           = "ok"
	DatabaseUnavailable  = "unavailable"
	DatabaseUnconfigured = "unconfigured"
)

// Pinger is the small slice of a database pool that health checks need.
type Pinger interface {
	Ping(ctx context.Context) error
}

// DatabaseReport describes database readiness. Message is fixed text and never
// contains error details, hostnames, or credentials.
type DatabaseReport struct {
	Status    string `json:"status"`
	LatencyMS *int64 `json:"latencyMs,omitempty"`
	Message   string `json:"message"`
}

// Report is the JSON body of GET /api/v1/health.
type Report struct {
	Status      Status         `json:"status"`
	Service     string         `json:"service"`
	Version     string         `json:"version"`
	Environment string         `json:"environment"`
	CheckedAt   time.Time      `json:"checkedAt"`
	Database    DatabaseReport `json:"database"`
}

// Options configures a Service.
type Options struct {
	Service     string
	Version     string
	Environment string
	// Database may be nil when no database is configured. Do not assign a typed
	// nil pointer here; leave the interface itself nil.
	Database Pinger
	Timeout  time.Duration
	Logger   *slog.Logger
	// Now is injectable for tests. Defaults to time.Now.
	Now func() time.Time
}

// Service builds health reports.
type Service struct {
	opts Options
}

// NewService applies defaults and returns a Service.
func NewService(opts Options) *Service {
	if opts.Timeout <= 0 {
		opts.Timeout = 2 * time.Second
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Service{opts: opts}
}

// Check returns the current report. The API is "ok" only when the database
// answers; an unconfigured or unreachable database yields "degraded".
func (s *Service) Check(ctx context.Context) Report {
	db := s.checkDatabase(ctx)
	status := StatusDegraded
	if db.Status == DatabaseOK {
		status = StatusOK
	}
	return Report{
		Status:      status,
		Service:     s.opts.Service,
		Version:     s.opts.Version,
		Environment: s.opts.Environment,
		CheckedAt:   s.opts.Now().UTC(),
		Database:    db,
	}
}

func (s *Service) checkDatabase(ctx context.Context) DatabaseReport {
	if s.opts.Database == nil {
		return DatabaseReport{
			Status:  DatabaseUnconfigured,
			Message: "DATABASE_URL is not set, so database checks are skipped.",
		}
	}

	ctx, cancel := context.WithTimeout(ctx, s.opts.Timeout)
	defer cancel()

	start := time.Now()
	if err := s.opts.Database.Ping(ctx); err != nil {
		// Log only the failure class. Driver errors can include host and user
		// details, which must not reach logs or responses.
		reason := "error"
		if errors.Is(err, context.DeadlineExceeded) {
			reason = "timeout"
		}
		s.opts.Logger.Warn("database ping failed", "reason", reason)
		return DatabaseReport{
			Status:  DatabaseUnavailable,
			Message: "The database did not respond to a ping.",
		}
	}
	latency := time.Since(start).Milliseconds()
	return DatabaseReport{
		Status:    DatabaseOK,
		LatencyMS: &latency,
		Message:   "The database responded to a ping.",
	}
}
