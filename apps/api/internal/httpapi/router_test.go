package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"marketingops/api/internal/health"
)

type fakeChecker struct{ report health.Report }

func (f fakeChecker) Check(context.Context) health.Report { return f.report }

func newHandler(status health.Status, dbStatus string) http.Handler {
	return NewRouter(Deps{
		Health: fakeChecker{report: health.Report{
			Status:      status,
			Service:     "marketing-ops-api",
			Version:     "test",
			Environment: "test",
			CheckedAt:   time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			Database:    health.DatabaseReport{Status: dbStatus, Message: "fixed text"},
		}},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		AllowedOrigins: []string{"http://localhost:3000"},
	})
}

func do(h http.Handler, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthReturns200EvenWhenDegraded(t *testing.T) {
	rec := do(newHandler(health.StatusDegraded, health.DatabaseUnconfigured), http.MethodGet, "/api/v1/health", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got health.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if got.Status != health.StatusDegraded || got.Database.Status != health.DatabaseUnconfigured {
		t.Errorf("unexpected body: %+v", got)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("health responses must not be cached")
	}
}

func TestReadyReflectsStatus(t *testing.T) {
	ok := do(newHandler(health.StatusOK, health.DatabaseOK), http.MethodGet, "/api/v1/ready", nil)
	if ok.Code != http.StatusOK {
		t.Errorf("ready with healthy deps = %d, want 200", ok.Code)
	}
	bad := do(newHandler(health.StatusDegraded, health.DatabaseUnavailable), http.MethodGet, "/api/v1/ready", nil)
	if bad.Code != http.StatusServiceUnavailable {
		t.Errorf("ready with degraded deps = %d, want 503", bad.Code)
	}
}

func TestUnknownRouteAndWrongMethod(t *testing.T) {
	h := newHandler(health.StatusOK, health.DatabaseOK)
	if rec := do(h, http.MethodGet, "/api/v1/nope", nil); rec.Code != http.StatusNotFound {
		t.Errorf("unknown route = %d, want 404", rec.Code)
	}
	if rec := do(h, http.MethodPost, "/api/v1/health", nil); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST health = %d, want 405", rec.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	rec := do(newHandler(health.StatusOK, health.DatabaseOK), http.MethodGet, "/api/v1/health", nil)
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing X-Content-Type-Options: nosniff")
	}
}

func TestCORSAllowsOnlyConfiguredOrigins(t *testing.T) {
	h := newHandler(health.StatusOK, health.DatabaseOK)

	allowed := do(h, http.MethodGet, "/api/v1/health", map[string]string{"Origin": "http://localhost:3000"})
	if got := allowed.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Errorf("allowed origin header = %q", got)
	}

	denied := do(h, http.MethodGet, "/api/v1/health", map[string]string{"Origin": "https://evil.example.com"})
	if got := denied.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("disallowed origin received header %q", got)
	}

	none := do(h, http.MethodGet, "/api/v1/health", nil)
	if got := none.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("request without Origin received header %q", got)
	}
}

func TestCORSPreflight(t *testing.T) {
	h := newHandler(health.StatusOK, health.DatabaseOK)
	rec := do(h, http.MethodOptions, "/api/v1/health", map[string]string{
		"Origin":                        "http://localhost:3000",
		"Access-Control-Request-Method": "GET",
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("preflight is missing Access-Control-Allow-Methods")
	}
}

type panicChecker struct{}

func (panicChecker) Check(context.Context) health.Report { panic("boom") }

func TestPanicIsRecovered(t *testing.T) {
	h := NewRouter(Deps{
		Health: panicChecker{},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	rec := do(h, http.MethodGet, "/api/v1/health", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status after panic = %d, want 500", rec.Code)
	}
}
