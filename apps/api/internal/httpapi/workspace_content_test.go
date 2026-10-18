package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"marketingops/api/internal/store"
)

func TestListWorkspaceContent(t *testing.T) {
	wsID := "11111111-1111-1111-1111-111111111111"

	t.Run("returns 501 when store is nil", func(t *testing.T) {
		h := NewRouter(Deps{Workspaces: nil, Logger: testLogger})
		rec := doJSON(t, h, http.MethodGet, "/api/v1/workspaces/"+wsID+"/content", "")
		if rec.Code != http.StatusNotImplemented {
			t.Fatalf("status = %d, want 501", rec.Code)
		}
	})

	t.Run("returns 404 when workspace does not exist", func(t *testing.T) {
		h := NewRouter(Deps{
			Logger: testLogger,
			Workspaces: &fakeWorkspaceStore{
				byID: map[string]store.Workspace{},
			},
		})
		rec := doJSON(t, h, http.MethodGet, "/api/v1/workspaces/"+wsID+"/content", "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})

	t.Run("returns 200 with empty items list", func(t *testing.T) {
		h := NewRouter(Deps{
			Logger: testLogger,
			Workspaces: &fakeWorkspaceStore{
				byID: map[string]store.Workspace{
					wsID: {ID: wsID, Slug: "demo", Name: "Demo"},
				},
				listContentResult: []store.ContentItem{},
			},
		})
		rec := doJSON(t, h, http.MethodGet, "/api/v1/workspaces/"+wsID+"/content", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		var body struct {
			Items []contentItemJSON `json:"items"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if len(body.Items) != 0 {
			t.Fatalf("expected 0 items, got %d", len(body.Items))
		}
	})

	t.Run("returns 200 with content items", func(t *testing.T) {
		slug := "first-post"
		h := NewRouter(Deps{
			Logger: testLogger,
			Workspaces: &fakeWorkspaceStore{
				byID: map[string]store.Workspace{
					wsID: {ID: wsID, Slug: "demo", Name: "Demo"},
				},
				listContentResult: []store.ContentItem{{
					ID:          "content-1",
					WorkspaceID: wsID,
					Kind:        "blog_article",
					Status:      "draft",
					Title:       "First Post",
					Slug:        &slug,
					CreatedAt:   time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
					UpdatedAt:   time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
				}},
			},
		})
		rec := doJSON(t, h, http.MethodGet, "/api/v1/workspaces/"+wsID+"/content", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		var body struct {
			Items []contentItemJSON `json:"items"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if len(body.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(body.Items))
		}
		if body.Items[0].Title != "First Post" || *body.Items[0].Slug != "first-post" {
			t.Errorf("unexpected item: %+v", body.Items[0])
		}
	})
}

func TestCreateWorkspaceContent(t *testing.T) {
	wsID := "11111111-1111-1111-1111-111111111111"

	t.Run("returns 501 when store is nil", func(t *testing.T) {
		h := NewRouter(Deps{Workspaces: nil, Logger: testLogger})
		rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+wsID+"/content", `{"title":"Test","kind":"blog_article"}`)
		if rec.Code != http.StatusNotImplemented {
			t.Fatalf("status = %d, want 501", rec.Code)
		}
	})

	t.Run("returns 404 when workspace does not exist", func(t *testing.T) {
		h := NewRouter(Deps{
			Logger: testLogger,
			Workspaces: &fakeWorkspaceStore{
				byID: map[string]store.Workspace{},
			},
		})
		rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+wsID+"/content", `{"title":"Test","kind":"blog_article"}`)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})

	t.Run("rejects invalid JSON", func(t *testing.T) {
		h := NewRouter(Deps{
			Logger: testLogger,
			Workspaces: &fakeWorkspaceStore{
				byID: map[string]store.Workspace{
					wsID: {ID: wsID, Slug: "demo", Name: "Demo"},
				},
			},
		})
		rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+wsID+"/content", `not json`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("rejects missing kind and missing title", func(t *testing.T) {
		h := NewRouter(Deps{
			Logger: testLogger,
			Workspaces: &fakeWorkspaceStore{
				byID: map[string]store.Workspace{
					wsID: {ID: wsID, Slug: "demo", Name: "Demo"},
				},
			},
		})
		rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+wsID+"/content", `{}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "kind is required") || !strings.Contains(rec.Body.String(), "title is required") {
			t.Errorf("expected validation errors, got: %s", rec.Body.String())
		}
	})

	t.Run("rejects unsupported kind", func(t *testing.T) {
		h := NewRouter(Deps{
			Logger: testLogger,
			Workspaces: &fakeWorkspaceStore{
				byID: map[string]store.Workspace{
					wsID: {ID: wsID, Slug: "demo", Name: "Demo"},
				},
			},
		})
		rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+wsID+"/content", `{"title":"Hello","kind":"podcast"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "kind must be blog_article, social_post, or snippet") {
			t.Errorf("expected kind error, got: %s", rec.Body.String())
		}
	})

	t.Run("rejects invalid slug format", func(t *testing.T) {
		h := NewRouter(Deps{
			Logger: testLogger,
			Workspaces: &fakeWorkspaceStore{
				byID: map[string]store.Workspace{
					wsID: {ID: wsID, Slug: "demo", Name: "Demo"},
				},
			},
		})
		rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+wsID+"/content", `{"title":"Hello","kind":"social_post","slug":"Invalid Slug!"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "slug must be lowercase letters") {
			t.Errorf("expected slug format error, got: %s", rec.Body.String())
		}
	})

	t.Run("rejects duplicate slug with 409 Conflict", func(t *testing.T) {
		h := NewRouter(Deps{
			Logger: testLogger,
			Workspaces: &fakeWorkspaceStore{
				byID: map[string]store.Workspace{
					wsID: {ID: wsID, Slug: "demo", Name: "Demo"},
				},
				insertContentErr: store.ErrSlugTaken,
			},
		})
		rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+wsID+"/content", `{"title":"Hello","kind":"blog_article","slug":"duplicate"}`)
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409", rec.Code)
		}
	})

	t.Run("creates draft content item and writes audit event", func(t *testing.T) {
		fake := &fakeWorkspaceStore{
			byID: map[string]store.Workspace{
				wsID: {ID: wsID, Slug: "demo", Name: "Demo"},
			},
		}
		h := NewRouter(Deps{Logger: testLogger, Workspaces: fake})
		rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+wsID+"/content", `{"title":"Healthy Smoothie Recipe","kind":"blog_article","slug":"healthy-smoothie"}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body: %s", rec.Code, rec.Body.String())
		}
		var created contentItemJSON
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if created.Status != "draft" {
			t.Errorf("expected draft status, got %s", created.Status)
		}
		if created.Title != "Healthy Smoothie Recipe" || *created.Slug != "healthy-smoothie" || created.Kind != "blog_article" {
			t.Errorf("unexpected created item: %+v", created)
		}

		if len(fake.insertedContent) != 1 {
			t.Fatalf("expected 1 inserted item, got %d", len(fake.insertedContent))
		}
		if len(fake.auditEvents) != 1 {
			t.Fatalf("expected 1 audit event, got %d", len(fake.auditEvents))
		}
		if fake.auditEvents[0].Action != "content.created" {
			t.Errorf("audit action = %q, want content.created", fake.auditEvents[0].Action)
		}
	})
}
