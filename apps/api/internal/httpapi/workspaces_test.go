package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"marketingops/api/internal/store"
)

// testLogger keeps middleware happy without printing anything.
var testLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

// fakeWorkspaceStore records calls and answers from canned data.
type fakeWorkspaceStore struct {
	listResult []store.Workspace
	listErr    error
	insertErr  error
	inserted   []store.Workspace
}

func (f *fakeWorkspaceStore) ListWorkspaces(ctx context.Context) ([]store.Workspace, error) {
	return f.listResult, f.listErr
}

func (f *fakeWorkspaceStore) InsertWorkspace(ctx context.Context, w store.Workspace) (store.Workspace, error) {
	if f.insertErr != nil {
		return store.Workspace{}, f.insertErr
	}
	f.inserted = append(f.inserted, w)
	return store.Workspace{
		ID: "11111111-1111-1111-1111-111111111111", Slug: w.Slug, Name: w.Name,
		WebsiteURL: w.WebsiteURL, Status: "active",
		CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		UpdatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}, nil
}

func workspacesRouter() http.Handler {
	return NewRouter(Deps{
		Health: nil,
		Logger: testLogger,
		Workspaces: &fakeWorkspaceStore{
			listResult: []store.Workspace{{
				ID: "22222222-2222-2222-2222-222222222222", Slug: "demo-studio",
				Name: "Demo Studio", Status: "active",
				CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			}},
		},
	})
}

// post runs a POST against the router with a JSON body and returns the recorders.
func doJSON(t *testing.T, h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestListWorkspacesEndpoint(t *testing.T) {
	rec := doJSON(t, workspacesRouter(), http.MethodGet, "/api/v1/workspaces", "") //nolint:bodyclose — httptest recorder
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	want := `{"workspaces":[{"id":"22222222-2222-2222-2222-222222222222","slug":"demo-studio","name":"Demo Studio","websiteUrl":null,"status":"active","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}]}` + "\n"
	if rec.Body.String() != want {
		t.Errorf("body = %s, want %s", rec.Body.String(), want)
	}
}

func TestListWorkspacesWithoutStore(t *testing.T) {
	h := NewRouter(Deps{Logger: testLogger})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("nil store must give 501, got %d", rec.Code)
	}
}

func TestCreateWorkspaceEndpoint(t *testing.T) {
	store := &fakeWorkspaceStore{insertErr: nil}
	h := NewRouter(Deps{Logger: testLogger, Workspaces: store})

	rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces",
		`{"slug":"demo-cafe","name":"Demo Cafe","websiteUrl":"https://demo-cafe.example/"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if len(store.inserted) != 1 {
		t.Fatalf("store not called: %+v", store.inserted)
	}
	inserted := store.inserted[0]
	if inserted.Slug != "demo-cafe" || inserted.Name != "Demo Cafe" {
		t.Errorf("inserted = %+v", inserted)
	}
	if inserted.WebsiteURL == nil || *inserted.WebsiteURL != "https://demo-cafe.example" {
		t.Errorf("website URL not trimmed: %+v", inserted.WebsiteURL)
	}
	if !strings.Contains(rec.Body.String(), `"slug":"demo-cafe"`) {
		t.Errorf("response body missing slug: %s", rec.Body.String())
	}
}

func TestCreateWorkspaceValidation(t *testing.T) {
	h := NewRouter(Deps{Logger: testLogger, Workspaces: &fakeWorkspaceStore{}})
	cases := []struct {
		name, body, wantErr string
	}{
		{"missing slug", `{"name":"x"}`, "slug is required"},
		{"bad slug", `{"slug":"Nope!","name":"x"}`, "slug must be lowercase"},
		{"missing name", `{"slug":"ok-slug"}`, "name is required"},
		{"emphasis dots", `{"slug":"a.b","name":"x"}`, "slug must be lowercase"},
		{"not json", `nonsense`, "not valid JSON"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), tc.wantErr) {
				t.Errorf("body = %s, want to contain %q", rec.Body.String(), tc.wantErr)
			}
		})
	}
}

func TestCreateWorkspaceConflictAndStoreError(t *testing.T) {
	conflict := NewRouter(Deps{Logger: testLogger, Workspaces: &fakeWorkspaceStore{insertErr: store.ErrSlugTaken}})
	rec := doJSON(t, conflict, http.MethodPost, "/api/v1/workspaces", `{"slug":"taken","name":"x"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("ErrSlugTaken must be 409, got %d", rec.Code)
	}
	internal := NewRouter(Deps{Logger: testLogger, Workspaces: &fakeWorkspaceStore{insertErr: errors.New("boom")}})
	rec = doJSON(t, internal, http.MethodPost, "/api/v1/workspaces", `{"slug":"ok","name":"x"}`)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "boom") {
		t.Errorf("internal errors must be 500 without detail, got %d body=%s", rec.Code, rec.Body.String())
	}
}
