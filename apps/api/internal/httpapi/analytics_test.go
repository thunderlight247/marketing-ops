package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"marketingops/api/internal/store"
)

type fakeAnalyticsStore struct {
	summary       store.AnalyticsSummary
	err           error
	calls         int
	lastWorkspace string
}

func (f *fakeAnalyticsStore) AnalyticsSummary(_ context.Context, workspaceID string) (store.AnalyticsSummary, error) {
	f.calls++
	f.lastWorkspace = workspaceID
	return f.summary, f.err
}

func (f *fakeAnalyticsStore) AnalyticsBreakdown(_ context.Context, workspaceID string, from, to time.Time) (store.AnalyticsBreakdown, error) {
	f.calls++
	f.lastWorkspace = workspaceID
	return store.AnalyticsBreakdown{From: from, To: to}, f.err
}

func TestAnalyticsBreakdownRejectsBadRange(t *testing.T) {
	h := NewRouter(Deps{Logger: testLogger, Analytics: &fakeAnalyticsStore{}})
	for _, path := range []string{
		"/api/v1/analytics/breakdown?from=2026-13-01",
		"/api/v1/analytics/breakdown?from=2026-06-10&to=2026-06-01",
		"/api/v1/analytics/breakdown?from=2020-01-01&to=2026-06-01",
	} {
		rec := doJSON(t, h, "GET", path, "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", path, rec.Code)
		}
	}
}

func TestAnalyticsBreakdownPassesRange(t *testing.T) {
	fake := &fakeAnalyticsStore{}
	h := NewRouter(Deps{Logger: testLogger, Analytics: fake})
	rec := doJSON(t, h, "GET", "/api/v1/analytics/breakdown?workspaceId=w-2&from=2026-06-01&to=2026-06-07", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if fake.lastWorkspace != "w-2" {
		t.Errorf("workspace = %q, want w-2", fake.lastWorkspace)
	}
}

func TestAnalyticsSummaryEndpoint(t *testing.T) {
	fake := &fakeAnalyticsStore{summary: store.AnalyticsSummary{
		GeneratedAt: time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC),
		Totals: store.AnalyticsTotals{
			Workspaces: 1, EngineLeads: 121, EngineSent: 80,
			EngineFailed: 6, EngineBlacklist: 32, EmailsSent: 4,
		},
		ByWorkspace: []store.WorkspaceAnalytics{{
			WorkspaceID: "w1", Slug: "redeem", Name: "Redeem",
			EngineLeads: 121, EmailsSent: 4,
		}},
		CRMStatus:    store.ReportCounts{},
		EngineStatus: store.ReportCounts{"sent": 80, "failed": 6},
	}}
	h := NewRouter(Deps{Logger: testLogger, Analytics: fake})

	rec := doJSON(t, h, "GET", "/api/v1/analytics/summary", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Totals      store.AnalyticsTotals      `json:"totals"`
		ByWorkspace []store.WorkspaceAnalytics `json:"byWorkspace"`
		Engine      map[string]int             `json:"engineLeadsByStatus"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Totals.EngineLeads != 121 || body.Totals.EngineBlacklist != 32 {
		t.Errorf("totals = %+v", body.Totals)
	}
	if len(body.ByWorkspace) != 1 || body.ByWorkspace[0].Name != "Redeem" {
		t.Errorf("byWorkspace = %+v", body.ByWorkspace)
	}
	if fake.calls != 1 {
		t.Errorf("AnalyticsSummary calls = %d, want 1", fake.calls)
	}
	if fake.lastWorkspace != "" {
		t.Errorf("unfiltered request passed workspace %q", fake.lastWorkspace)
	}
}

func TestAnalyticsSummaryEndpointScopesWorkspace(t *testing.T) {
	fake := &fakeAnalyticsStore{}
	h := NewRouter(Deps{Logger: testLogger, Analytics: fake})

	rec := doJSON(t, h, "GET", "/api/v1/analytics/summary?workspaceId=w-1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if fake.lastWorkspace != "w-1" {
		t.Errorf("workspace = %q, want w-1", fake.lastWorkspace)
	}
}

func TestAnalyticsSummaryEndpointUnavailable(t *testing.T) {
	h := NewRouter(Deps{Logger: testLogger})
	rec := doJSON(t, h, "GET", "/api/v1/analytics/summary", "")
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
}

type fakeOperatorProvider struct {
	updateErr error
}

func (f fakeOperatorProvider) EnsureOperator(context.Context) (string, error) {
	return "op-1", nil
}
func (f fakeOperatorProvider) ListOperators(context.Context) ([]store.Operator, error) {
	return nil, nil
}
func (f fakeOperatorProvider) CreateOperator(context.Context, string, string, string) (store.Operator, error) {
	return store.Operator{}, nil
}
func (f fakeOperatorProvider) UpdateOperator(_ context.Context, id, role, status string) (store.Operator, error) {
	if f.updateErr != nil {
		return store.Operator{}, f.updateErr
	}
	return store.Operator{ID: id, Role: role, Status: status}, nil
}

func TestUpdateOperatorLastAdminConflict(t *testing.T) {
	req := httptest.NewRequest("PATCH", "/api/v1/operators/op-1",
		strings.NewReader(`{"role":"editor"}`))
	req.SetPathValue("id", "op-1")
	rec := httptest.NewRecorder()
	updateOperatorHandler(fakeOperatorProvider{updateErr: store.ErrLastAdmin}).ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "at least one active admin") {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestUpdateOperatorNotFound(t *testing.T) {
	req := httptest.NewRequest("PATCH", "/api/v1/operators/missing",
		strings.NewReader(`{"status":"inactive"}`))
	req.SetPathValue("id", "missing")
	rec := httptest.NewRecorder()
	updateOperatorHandler(fakeOperatorProvider{updateErr: store.ErrNotFound}).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestUpdateOperatorInternalError(t *testing.T) {
	req := httptest.NewRequest("PATCH", "/api/v1/operators/op-1",
		strings.NewReader(`{"role":"viewer"}`))
	req.SetPathValue("id", "op-1")
	rec := httptest.NewRecorder()
	updateOperatorHandler(fakeOperatorProvider{updateErr: errors.New("database down")}).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
