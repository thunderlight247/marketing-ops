package httpapi

// router-level tests for the Milestone 12 report endpoint. The store's SQL
// behavior is covered by store integration tests (skipped without a database).

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"marketingops/api/internal/store"
)

var errReportBoom = errors.New("boom")

func TestWorkspaceReportEndpoint(t *testing.T) {
	wsID := "11111111-1111-1111-1111-111111111111"
	workspace := store.Workspace{ID: wsID, Slug: "demo", Name: "Demo"}

	report := store.WorkspaceReport{
		WorkspaceID:    wsID,
		GeneratedAt:    time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC),
		ContentStatus:  store.ReportCounts{"draft": 2, "published": 1},
		CampaignsBy:    store.ReportCounts{"active": 1},
		LeadsByStatus:  store.ReportCounts{"new": 3, "won": 1},
		LeadsBySource:  store.ReportCounts{"manual": 4},
		PublishingJobs: store.ReportCounts{"succeeded": 2, "failed": 1},
		PublishedCount: 2,
		FailedCount:    1,
		RecentFailures: []store.PublishingFailure{{
			JobID: "job-1", ItemID: "it-1", IntegrationID: "in-1",
			Attempts: 2, LastError: "the destination refused the request (HTTP 503)",
			UpdatedAt: time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC),
		}},
	}

	t.Run("returns the report shape with no range", func(t *testing.T) {
		fake := &fakeWorkspaceStore{byID: map[string]store.Workspace{wsID: workspace}, reportResult: report}
		h := NewRouter(Deps{Logger: testLogger, Workspaces: fake})

		rec := doJSON(t, h, "GET", "/api/v1/workspaces/"+wsID+"/reports", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if body["workspaceId"] != wsID {
			t.Errorf("workspaceId = %v", body["workspaceId"])
		}
		rng, ok := body["range"].(map[string]any)
		if !ok || rng["since"] != nil || rng["until"] != nil {
			t.Errorf("no-range report must carry an empty range, got %v", body["range"])
		}
		if len(fake.reportCalls) != 1 || fake.reportCalls[0].workspaceID != wsID {
			t.Errorf("WorkspaceReport not called once with the workspace, got %+v", fake.reportCalls)
		}
		if fake.reportCalls[0].rangeSince != nil || fake.reportCalls[0].rangeUntil != nil {
			t.Errorf("absent since/until must stay nil, got %+v", fake.reportCalls[0])
		}
		content, ok := body["contentStatusNow"].(map[string]any)
		if !ok || content["draft"] == nil || content["published"] == nil {
			t.Errorf("contentStatusNow missing: %v", body["contentStatusNow"])
		}
		if _, ok := body["leadSourcesNow"]; !ok {
			t.Errorf("leadSourcesNow missing from response")
		}
		pr, ok := body["publishingRange"].(map[string]any)
		if !ok {
			t.Fatalf("publishingRange missing")
		}
		if pr["publishedCount"] != float64(2) || pr["failedCount"] != float64(1) {
			t.Errorf("publishingRange counts wrong: %+v", pr)
		}
		failures, ok := pr["recentFailures"].([]any)
		if !ok || len(failures) != 1 {
			t.Fatalf("recentFailures wrong: %+v", pr["recentFailures"])
		}
		f := failures[0].(map[string]any)
		if f["jobId"] != "job-1" || f["lastError"] == nil {
			t.Errorf("recent failure fields wrong: %+v", f)
		}
	})

	t.Run("parses and validates the range", func(t *testing.T) {
		fake := &fakeWorkspaceStore{byID: map[string]store.Workspace{wsID: workspace}, reportResult: report}
		h := NewRouter(Deps{Logger: testLogger, Workspaces: fake})

		rec := doJSON(t, h, "GET",
			"/api/v1/workspaces/"+wsID+
				"/reports?since=2026-10-01T00%3A00%3A00Z&until=2026-10-31T00%3A00%3A00Z", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
		}
		if len(fake.reportCalls) != 1 {
			t.Fatalf("report calls = %d", len(fake.reportCalls))
		}
		wantSince := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		wantUntil := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
		if fake.reportCalls[0].rangeSince == nil || !fake.reportCalls[0].rangeSince.Equal(wantSince) {
			t.Errorf("since = %v, want %v", fake.reportCalls[0].rangeSince, wantSince)
		}
		if fake.reportCalls[0].rangeUntil == nil || !fake.reportCalls[0].rangeUntil.Equal(wantUntil) {
			t.Errorf("until = %v, want %v", fake.reportCalls[0].rangeUntil, wantUntil)
		}

		// Inverted range is a 400; garbage timestamps are 400s.
		bad := [][2]string{
			{"2026-10-31T00%3A00%3A00Z", "2026-10-01T00%3A00%3A00Z"},
			{"not-a-time", ""},
			{"", "also-not-a-time"},
		}
		for _, c := range bad {
			q := "/api/v1/workspaces/" + wsID + "/reports?"
			if c[0] != "" {
				q += "since=" + c[0] + "&"
			}
			if c[1] != "" {
				q += "until=" + c[1]
			}
			rec := doJSON(t, h, "GET", q, "")
			if rec.Code != http.StatusBadRequest {
				t.Errorf("invalid range %q -> status %d, want 400; body=%s", q, rec.Code, rec.Body.String())
			}
		}
		if len(fake.reportCalls) != 1 {
			t.Errorf("no store call may happen for invalid ranges, got %d", len(fake.reportCalls))
		}
	})

	t.Run("store errors become 500 without leaking internals", func(t *testing.T) {
		fake := &fakeWorkspaceStore{byID: map[string]store.Workspace{wsID: workspace}, reportErr: errReportBoom}
		h := NewRouter(Deps{Logger: testLogger, Workspaces: fake})
		rec := doJSON(t, h, "GET", "/api/v1/workspaces/"+wsID+"/reports", "")
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", rec.Code)
		}
		if want := `{"error":"report generation failed"}`; rec.Body.String() != want+"\n" {
			t.Errorf("body = %q, want %q", rec.Body.String(), want)
		}
		if len(fake.reportCalls) != 0 {
			t.Errorf("the recorded call should not have grown past the failure point, got %d", len(fake.reportCalls))
		}
	})

	t.Run("unknown workspace is 404", func(t *testing.T) {
		fake := &fakeWorkspaceStore{byID: map[string]store.Workspace{}}
		h := NewRouter(Deps{Logger: testLogger, Workspaces: fake})
		rec := doJSON(t, h, "GET", "/api/v1/workspaces/"+wsID+"/reports", "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
		if len(fake.reportCalls) != 0 {
			t.Errorf("no report may be generated for a missing workspace")
		}
	})

	t.Run("nil store answers 501", func(t *testing.T) {
		h := NewRouter(Deps{Logger: testLogger})
		rec := doJSON(t, h, "GET", "/api/v1/workspaces/"+wsID+"/reports", "")
		if rec.Code != http.StatusNotImplemented {
			t.Fatalf("status = %d, want 501", rec.Code)
		}
	})
}
