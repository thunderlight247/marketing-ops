package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"marketingops/api/internal/store"
)

func detailDeps(items map[string]store.ContentItem, versions map[string][]store.ContentVersion) *fakeWorkspaceStore {
	return &fakeWorkspaceStore{
		byID: map[string]store.Workspace{
			"11111111-1111-1111-1111-111111111111": {ID: "11111111-1111-1111-1111-111111111111", Slug: "demo", Name: "Demo"},
		},
		contentItemsByID:      items,
		contentVersionsByItem: versions,
	}
}

func draftItem(id, workspaceID string) store.ContentItem {
	return store.ContentItem{
		ID: id, WorkspaceID: workspaceID, Kind: "blog_article", Status: "draft",
		Title: "Old title", CreatedAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestGetWorkspaceContentItem(t *testing.T) {
	wsID := "11111111-1111-1111-1111-111111111111"
	itemID := "33333333-3333-3333-3333-333333333333"

	t.Run("returns 501 when store is nil", func(t *testing.T) {
		h := NewRouter(Deps{Logger: testLogger})
		rec := doJSON(t, h, http.MethodGet, "/api/v1/workspaces/"+wsID+"/content/"+itemID, "")
		if rec.Code != http.StatusNotImplemented {
			t.Fatalf("status = %d, want 501", rec.Code)
		}
	})

	t.Run("returns 404 for unknown item", func(t *testing.T) {
		h := NewRouter(Deps{Logger: testLogger, Workspaces: detailDeps(nil, nil)})
		rec := doJSON(t, h, http.MethodGet, "/api/v1/workspaces/"+wsID+"/content/"+itemID, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})

	t.Run("returns 404 when the item belongs to another workspace", func(t *testing.T) {
		other := draftItem(itemID, "22222222-2222-2222-2222-222222222222")
		h := NewRouter(Deps{Logger: testLogger, Workspaces: detailDeps(map[string]store.ContentItem{itemID: other}, nil)})
		rec := doJSON(t, h, http.MethodGet, "/api/v1/workspaces/"+wsID+"/content/"+itemID, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})

	t.Run("returns the item with a null version before the first save", func(t *testing.T) {
		h := NewRouter(Deps{Logger: testLogger, Workspaces: detailDeps(
			map[string]store.ContentItem{itemID: draftItem(itemID, wsID)}, nil)})
		rec := doJSON(t, h, http.MethodGet, "/api/v1/workspaces/"+wsID+"/content/"+itemID, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		var body struct {
			Title   string              `json:"title"`
			Version *contentVersionJSON `json:"version"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if body.Title != "Old title" || body.Version != nil {
			t.Fatalf("unexpected body: %+v", body)
		}
	})

	t.Run("returns the item with its newest version", func(t *testing.T) {
		first := store.ContentVersion{ID: "v1", ContentItemID: itemID, VersionNumber: 1, Title: "Old title", Body: "first body"}
		second := store.ContentVersion{ID: "v2", ContentItemID: itemID, VersionNumber: 2, Title: "New title", Body: "second body"}
		h := NewRouter(Deps{Logger: testLogger, Workspaces: detailDeps(
			map[string]store.ContentItem{itemID: draftItem(itemID, wsID)},
			map[string][]store.ContentVersion{itemID: {first, second}})})
		rec := doJSON(t, h, http.MethodGet, "/api/v1/workspaces/"+wsID+"/content/"+itemID, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		var body struct {
			Version *struct {
				VersionNumber int    `json:"versionNumber"`
				Body          string `json:"body"`
			} `json:"version"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if body.Version == nil || body.Version.VersionNumber != 2 || body.Version.Body != "second body" {
			t.Fatalf("expected newest version 2 with second body, got %+v", body.Version)
		}
	})
}

func TestUpdateWorkspaceContentItem(t *testing.T) {
	wsID := "11111111-1111-1111-1111-111111111111"
	itemID := "33333333-3333-3333-3333-333333333333"

	t.Run("returns 501 when store is nil", func(t *testing.T) {
		h := NewRouter(Deps{Logger: testLogger})
		rec := doJSON(t, h, http.MethodPatch, "/api/v1/workspaces/"+wsID+"/content/"+itemID, "{}")
		if rec.Code != http.StatusNotImplemented {
			t.Fatalf("status = %d, want 501", rec.Code)
		}
	})

	t.Run("returns 404 for unknown item", func(t *testing.T) {
		h := NewRouter(Deps{Logger: testLogger, Workspaces: detailDeps(nil, nil)})
		rec := doJSON(t, h, http.MethodPatch, "/api/v1/workspaces/"+wsID+"/content/"+itemID, `{"body":"x"}`)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})

	t.Run("rejects invalid JSON", func(t *testing.T) {
		h := NewRouter(Deps{Logger: testLogger, Workspaces: detailDeps(
			map[string]store.ContentItem{itemID: draftItem(itemID, wsID)}, nil)})
		rec := doJSON(t, h, http.MethodPatch, "/api/v1/workspaces/"+wsID+"/content/"+itemID, "not json")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("rejects a blank replacement title and a bad publish timestamp", func(t *testing.T) {
		h := NewRouter(Deps{Logger: testLogger, Workspaces: detailDeps(
			map[string]store.ContentItem{itemID: draftItem(itemID, wsID)}, nil)})
		rec := doJSON(t, h, http.MethodPatch, "/api/v1/workspaces/"+wsID+"/content/"+itemID,
			`{"title":"   ","plannedPublishAt":"not-a-time"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		if !contains(rec.Body.String(), "title cannot be blank") ||
			!contains(rec.Body.String(), "plannedPublishAt must be a UTC ISO 8601 timestamp") {
			t.Fatalf("expected both validation errors, got: %s", rec.Body.String())
		}
	})

	t.Run("rejects an invalid slug and oversized channel copy", func(t *testing.T) {
		h := NewRouter(Deps{Logger: testLogger, Workspaces: detailDeps(
			map[string]store.ContentItem{itemID: draftItem(itemID, wsID)}, nil)})
		rec := doJSON(t, h, http.MethodPatch, "/api/v1/workspaces/"+wsID+"/content/"+itemID,
			`{"slug":"Bad Slug!","channelCopy":{"twitter":"x"}}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		if !contains(rec.Body.String(), "slug must be lowercase letters, digits, or hyphens") {
			t.Fatalf("expected slug error, got: %s", rec.Body.String())
		}
	})

	t.Run("saves a draft, numbering the next version, and writes an audit event", func(t *testing.T) {
		fake := detailDeps(
			map[string]store.ContentItem{
				itemID: mustHaveSlug(draftItem(itemID, wsID), "old-slug"),
			},
			map[string][]store.ContentVersion{
				itemID: {{ID: "v1", ContentItemID: itemID, VersionNumber: 1, Title: "Old title", Body: "old body"}},
			})
		h := NewRouter(Deps{Logger: testLogger, Workspaces: fake})
		rec := doJSON(t, h, http.MethodPatch, "/api/v1/workspaces/"+wsID+"/content/"+itemID,
			`{"title":"New title","body":"Revised body","channelCopy":{"instagram":"Happy hour at 6!"},"plannedPublishAt":"2026-05-01T09:00:00Z"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
		}

		if len(fake.savedDrafts) != 1 {
			t.Fatalf("expected 1 save call, got %d", len(fake.savedDrafts))
		}
		call := fake.savedDrafts[0]
		if call.WorkspaceID != wsID || call.ItemID != itemID {
			t.Fatalf("save call missing scoping: %+v", call)
		}
		in := call.Input
		if in.Title != "New title" || derefString(in.Body) != "Revised body" {
			t.Fatalf("unexpected input: %+v", in)
		}
		// Omitted excerpt/CTA keep the latest version's values; omitted slug keeps the slug.
		if in.Excerpt != nil || in.CallToAction != nil || in.Slug != nil {
			t.Fatalf("omitted fields should be nil/keep, got: %+v", in)
		}
		if in.PlannedPublishAt == nil || in.PlannedPublishAt.UTC() != time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC) {
			t.Fatalf("planned publish time not parsed: %+v", in.PlannedPublishAt)
		}
		if len(in.ChannelCopy) != 1 || in.ChannelCopy["instagram"] != "Happy hour at 6!" {
			t.Fatalf("unexpected channel copy: %+v", in.ChannelCopy)
		}

		var body struct {
			Version struct {
				VersionNumber int `json:"versionNumber"`
			} `json:"version"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if body.Version.VersionNumber != 2 {
			t.Fatalf("expected version 2, got %d", body.Version.VersionNumber)
		}
		if len(fake.auditEvents) != 1 || fake.auditEvents[0].Action != "content.updated" {
			t.Fatalf("expected content.updated audit event, got %+v", fake.auditEvents)
		}
	})

	t.Run("conflicts 409 when the item is not a draft", func(t *testing.T) {
		item := draftItem(itemID, wsID)
		item.Status = "published"
		h := NewRouter(Deps{Logger: testLogger, Workspaces: detailDeps(
			map[string]store.ContentItem{itemID: item}, nil)})
		rec := doJSON(t, h, http.MethodPatch, "/api/v1/workspaces/"+wsID+"/content/"+itemID, `{"body":"x"}`)
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409", rec.Code)
		}
	})

	t.Run("conflicts 409 when replacing the slug with a taken one", func(t *testing.T) {
		fake := detailDeps(map[string]store.ContentItem{itemID: draftItem(itemID, wsID)}, nil)
		fake.saveDraftErr = store.ErrSlugTaken
		h := NewRouter(Deps{Logger: testLogger, Workspaces: fake})
		rec := doJSON(t, h, http.MethodPatch, "/api/v1/workspaces/"+wsID+"/content/"+itemID, `{"slug":"taken"}`)
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409", rec.Code)
		}
	})
}

func mustHaveSlug(item store.ContentItem, slug string) store.ContentItem {
	item.Slug = &slug
	return item
}

func contains(haystack, needle string) bool { return strings.Contains(haystack, needle) }
