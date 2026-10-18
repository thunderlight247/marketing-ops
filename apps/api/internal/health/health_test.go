package health

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

type fakePinger struct {
	err   error
	block bool
}

func (f fakePinger) Ping(ctx context.Context) error {
	if f.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return f.err
}

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newTestService(db Pinger, timeout time.Duration) *Service {
	return NewService(Options{
		Service:     "marketing-ops-api",
		Version:     "test",
		Environment: "test",
		Database:    db,
		Timeout:     timeout,
		Logger:      quietLogger(),
		Now:         func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) },
	})
}

func TestCheckUnconfiguredDatabaseIsDegraded(t *testing.T) {
	r := newTestService(nil, time.Second).Check(context.Background())
	if r.Status != StatusDegraded {
		t.Errorf("Status = %q, want %q", r.Status, StatusDegraded)
	}
	if r.Database.Status != DatabaseUnconfigured {
		t.Errorf("Database.Status = %q, want %q", r.Database.Status, DatabaseUnconfigured)
	}
	if r.Database.LatencyMS != nil {
		t.Error("LatencyMS should be nil when the database is unconfigured")
	}
}

func TestCheckHealthyDatabaseIsOK(t *testing.T) {
	r := newTestService(fakePinger{}, time.Second).Check(context.Background())
	if r.Status != StatusOK || r.Database.Status != DatabaseOK {
		t.Fatalf("unexpected report: %+v", r)
	}
	if r.Database.LatencyMS == nil {
		t.Error("LatencyMS should be set when the database responds")
	}
	if want := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC); !r.CheckedAt.Equal(want) {
		t.Errorf("CheckedAt = %v, want %v", r.CheckedAt, want)
	}
}

func TestCheckFailedPingIsUnavailableAndRedacted(t *testing.T) {
	secretErr := errors.New("failed to connect to host=db user=admin password=hunter2")
	r := newTestService(fakePinger{err: secretErr}, time.Second).Check(context.Background())
	if r.Status != StatusDegraded || r.Database.Status != DatabaseUnavailable {
		t.Fatalf("unexpected report: %+v", r)
	}
	body, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, leaked := range []string{"hunter2", "admin", "host=db"} {
		if strings.Contains(string(body), leaked) {
			t.Errorf("report leaked %q: %s", leaked, body)
		}
	}
}

func TestCheckTimeoutIsUnavailable(t *testing.T) {
	start := time.Now()
	r := newTestService(fakePinger{block: true}, 20*time.Millisecond).Check(context.Background())
	if r.Database.Status != DatabaseUnavailable {
		t.Errorf("Database.Status = %q, want %q", r.Database.Status, DatabaseUnavailable)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Check took %v; the timeout was not applied", elapsed)
	}
}

func TestReportJSONShape(t *testing.T) {
	r := newTestService(nil, time.Second).Check(context.Background())
	body, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"status", "service", "version", "environment", "checkedAt", "database"} {
		if _, ok := got[key]; !ok {
			t.Errorf("JSON is missing key %q: %s", key, body)
		}
	}
}
