package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"marketingops/api/internal/store"
)

func TestDecideWorkspaceContent(t *testing.T) {
	wsID := "11111111-1111-1111-1111-111111111111"
	itemID := "33333333-3333-3333-3333-333333333333"

	url := func() string { return "/api/v1/workspaces/" + wsID + "/content/" + itemID + "/decision" }

	t.Run("returns 501 when store or operator provider is nil", func(t *testing.T) {
		h := NewRouter(Deps{Logger: testLogger, Workspaces: detailDeps(
			map[string]store.ContentItem{itemID: draftItem(itemID, wsID)}, nil)})
		rec := doJSON(t, h, http.MethodPost, url(), `{}`)
		if rec.Code != http.StatusNotImplemented {
			t.Fatalf("status = %d, want 501", rec.Code)
		}
		h2 := NewRouter(Deps{Logger: testLogger, Operators: &fakeWorkspaceStore{}})
		rec = doJSON(t, h2, http.MethodPost, url(), `{}`)
		if rec.Code != http.StatusNotImplemented {
			t.Fatalf("status = %d, want 501 without operators", rec.Code)
		}
	})

	t.Run("rejects a bad decision and oversized note", func(t *testing.T) {
		h := NewRouter(Deps{Logger: testLogger, Workspaces: detailDeps(
			map[string]store.ContentItem{itemID: draftItem(itemID, wsID)}, nil), Operators: &fakeWorkspaceStore{}})
		rec := doJSON(t, h, http.MethodPost, url(), `{"decision":"publish now"}`)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "decision must be approve or reject") {
			t.Fatalf("expected decision validation, got %d %s", rec.Code, rec.Body.String())
		}
		rec = doJSON(t, h, http.MethodPost, url(), `{"decision":"approve","note":"`+strings.Repeat("x", 2001)+`"}`)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "note is too long") {
			t.Fatalf("expected note validation, got %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("maps sentinel errors to 404 and 409s", func(t *testing.T) {
		cases := []struct {
			name string
			err  error
			want int
		}{
			{"no such item", store.ErrNotFound, http.StatusNotFound},
			{"no versions yet", store.ErrVersionNotLatest, http.StatusConflict},
			{"already decided", store.ErrAlreadyDecided, http.StatusConflict},
			{"status refuses decisions", store.ErrInvalidStatus, http.StatusConflict},
		}
		for _, c := range cases {
			fake := detailDeps(nil, nil)
			fake.decisionErr = c.err
			h := NewRouter(Deps{Logger: testLogger, Workspaces: fake, Operators: fake})
			rec := doJSON(t, h, http.MethodPost, url(), `{"decision":"approve"}`)
			if rec.Code != c.want {
				t.Fatalf("%s: status = %d, want %d", c.name, rec.Code, c.want)
			}
		}
	})

	t.Run("approving records the decision with the resolved operator and audits", func(t *testing.T) {
		fake := detailDeps(
			map[string]store.ContentItem{itemID: draftItem(itemID, wsID)},
			map[string][]store.ContentVersion{
				itemID: {{ID: "v1", ContentItemID: itemID, VersionNumber: 1, Title: "T", Body: "b"}},
			})
		h := NewRouter(Deps{Logger: testLogger, Workspaces: fake, Operators: fake})
		rec := doJSON(t, h, http.MethodPost, url(), `{"decision":"approve","note":"ship it"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
		}
		var resp struct {
			Status   string         `json:"status"`
			Decision decisionRecord `json:"decision"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.Status != "approved" || resp.Decision.Decision != "approved" {
			t.Fatalf("unexpected response: %+v", resp)
		}
		if resp.Decision.DecidedAt == "" {
			t.Fatalf("decision timestamp missing: %+v", resp)
		}

		if len(fake.decisions) != 1 {
			t.Fatalf("expected 1 decision call, got %d", len(fake.decisions))
		}
		call := fake.decisions[0]
		if call.WorkspaceID != wsID || call.ItemID != itemID || call.Decision != "approve" || call.Note == nil || *call.Note != "ship it" {
			t.Fatalf("call wrong: %+v", call)
		}
		if call.OperatorID != fake.fakeOperatorID {
			t.Fatalf("operator id not resolved from the provider: %q", call.OperatorID)
		}
		if len(fake.auditEvents) != 1 || fake.auditEvents[0].Action != "content.approval_approve_recorded" {
			t.Fatalf("expected content.approval_approve_recorded audit, got %+v", fake.auditEvents)
		}
	})
}

func TestTransitionWorkspaceContentStatus(t *testing.T) {
	wsID := "11111111-1111-1111-1111-111111111111"
	itemID := "33333333-3333-3333-3333-333333333333"
	url := "/api/v1/workspaces/" + wsID + "/content/" + itemID + "/status"

	t.Run("returns 501 and 404", func(t *testing.T) {
		h := NewRouter(Deps{Logger: testLogger})
		if rec := doJSON(t, h, http.MethodPost, url, `{"status":"review"}`); rec.Code != http.StatusNotImplemented {
			t.Fatalf("status = %d, want 501", rec.Code)
		}
		h2 := NewRouter(Deps{Logger: testLogger, Workspaces: detailDeps(nil, nil)})
		if rec := doJSON(t, h2, http.MethodPost, url, `{"status":"review"}`); rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})

	t.Run("rejects targets outside the exposed subset", func(t *testing.T) {
		for _, target := range []string{"approved", "published", "queued", "failed"} {
			h := NewRouter(Deps{Logger: testLogger, Workspaces: detailDeps(
				map[string]store.ContentItem{itemID: draftItem(itemID, wsID)}, nil)})
			rec := doJSON(t, h, http.MethodPost, url, `{"status":"`+target+`"}`)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("target %q: status = %d, want 400", target, rec.Code)
			}
		}
	})

	t.Run("submits a draft for review and audits it", func(t *testing.T) {
		fake := detailDeps(
			map[string]store.ContentItem{itemID: draftItem(itemID, wsID)},
			map[string][]store.ContentVersion{
				itemID: {{ID: "v1", ContentItemID: itemID, VersionNumber: 1, Title: "T", Body: "b"}},
			})
		h := NewRouter(Deps{Logger: testLogger, Workspaces: fake})
		rec := doJSON(t, h, http.MethodPost, url, `{"status":"review"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
		}
		var resp struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.Status != "review" {
			t.Fatalf("status = %q, want review", resp.Status)
		}
		if len(fake.auditEvents) != 1 || fake.auditEvents[0].Action != "content.status_changed" {
			t.Fatalf("expected content.status_changed audit, got %+v", fake.auditEvents)
		}
	})

	t.Run("conflicts 409 on an illegal move", func(t *testing.T) {
		item := draftItem(itemID, wsID)
		item.Status = "approved"
		fake := detailDeps(map[string]store.ContentItem{itemID: item}, nil)
		h := NewRouter(Deps{Logger: testLogger, Workspaces: fake})
		rec := doJSON(t, h, http.MethodPost, url, `{"status":"review"}`)
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409", rec.Code)
		}
	})
}

func TestGetWorkspaceContentHistory(t *testing.T) {
	wsID := "11111111-1111-1111-1111-111111111111"
	itemID := "33333333-3333-3333-3333-333333333333"
	url := "/api/v1/workspaces/" + wsID + "/content/" + itemID + "/history"

	t.Run("returns 501 and 404", func(t *testing.T) {
		h := NewRouter(Deps{Logger: testLogger})
		if rec := doJSON(t, h, http.MethodGet, url, ""); rec.Code != http.StatusNotImplemented {
			t.Fatalf("status = %d, want 501", rec.Code)
		}
		h2 := NewRouter(Deps{Logger: testLogger, Workspaces: detailDeps(nil, nil)})
		if rec := doJSON(t, h2, http.MethodGet, url, ""); rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})

	t.Run("pairs each version with its decision", func(t *testing.T) {
		approval := store.Approval{
			ID: "a1", ContentItemID: itemID, ContentVersionID: "v2", Decision: "approved",
			OperatorID: "op1", DecidedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		}
		fake := detailDeps(
			map[string]store.ContentItem{itemID: draftItem(itemID, wsID)},
			map[string][]store.ContentVersion{
				itemID: {
					{ID: "v1", ContentItemID: itemID, VersionNumber: 1, Title: "T", Body: "one"},
					{ID: "v2", ContentItemID: itemID, VersionNumber: 2, Title: "T", Body: "two"},
				},
			})
		fake.decisionsByItem = map[string][]store.Approval{itemID: {approval}}
		item := fake.contentItemsByID[itemID]
		item.Status = "approved"
		fake.contentItemsByID[itemID] = item
		h := NewRouter(Deps{Logger: testLogger, Workspaces: fake})
		rec := doJSON(t, h, http.MethodGet, url, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		var body struct {
			Versions []struct {
				VersionNumber int             `json:"versionNumber"`
				Decision      *decisionRecord `json:"decision"`
			} `json:"versions"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(body.Versions) != 2 {
			t.Fatalf("expected 2 versions, got %d", len(body.Versions))
		}
		if body.Versions[0].Decision != nil {
			t.Fatalf("v1 must have no decision")
		}
		if body.Versions[1].Decision == nil || body.Versions[1].Decision.Decision != "approved" {
			t.Fatalf("v2 must carry its approval: %+v", body.Versions[1].Decision)
		}
	})
}
