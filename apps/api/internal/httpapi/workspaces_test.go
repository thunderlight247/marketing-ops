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
	listResult             []store.Workspace
	listErr                error
	insertErr              error
	inserted               []store.Workspace
	byID                   map[string]store.Workspace
	brand                  *store.BrandProfile // nil means the workspace has none
	upsertErr              error
	upsertedBrand          *store.BrandProfile
	updateErr              error
	updated                []store.Workspace
	deleted                []string
	auditEvents            []store.AuditEvent
	listContentResult      []store.ContentItem
	listContentErr         error
	insertContentErr       error
	insertedContent        []store.ContentItem
	listIntegrationsResult []store.Integration
	listIntegrationsErr    error
	insertIntegrationErr   error
	insertedIntegrations   []store.Integration
	integrationsByID       map[string]store.Integration
	capabilitiesByInID     map[string][]store.IntegrationCapability
	credentialSetsByInID   map[string]store.CredentialSet
}

func (f *fakeWorkspaceStore) ListWorkspaces(ctx context.Context) ([]store.Workspace, error) {
	return f.listResult, f.listErr
}

func (f *fakeWorkspaceStore) GetWorkspaceByID(ctx context.Context, id string) (store.Workspace, error) {
	w, ok := f.byID[id]
	if !ok {
		return store.Workspace{}, store.ErrNotFound
	}
	return w, nil
}

func (f *fakeWorkspaceStore) UpdateWorkspace(ctx context.Context, w store.Workspace) (store.Workspace, error) {
	if f.updateErr != nil {
		return store.Workspace{}, f.updateErr
	}
	w.CreatedAt, w.UpdatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	f.updated = append(f.updated, w)
	return w, nil
}

func (f *fakeWorkspaceStore) DeleteWorkspace(ctx context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func (f *fakeWorkspaceStore) AddAuditEvent(ctx context.Context, e store.AuditEvent) error {
	f.auditEvents = append(f.auditEvents, e)
	return nil
}

func (f *fakeWorkspaceStore) GetBrandProfile(ctx context.Context, workspaceID string) (store.BrandProfile, error) {
	if f.brand == nil {
		return store.BrandProfile{}, store.ErrNotFound
	}
	return *f.brand, nil
}

func (f *fakeWorkspaceStore) UpsertBrandProfile(ctx context.Context, b store.BrandProfile) (store.BrandProfile, error) {
	if f.upsertErr != nil {
		return store.BrandProfile{}, f.upsertErr
	}
	b.UpdatedAt = time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	saved := b
	f.upsertedBrand = &saved
	return b, nil
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

func (f *fakeWorkspaceStore) ListContentItems(ctx context.Context, workspaceID string) ([]store.ContentItem, error) {
	if f.listContentErr != nil {
		return nil, f.listContentErr
	}
	return f.listContentResult, nil
}

func (f *fakeWorkspaceStore) InsertContentItem(ctx context.Context, item store.ContentItem) (store.ContentItem, error) {
	if f.insertContentErr != nil {
		return store.ContentItem{}, f.insertContentErr
	}
	item.ID = "33333333-3333-3333-3333-333333333333"
	item.CreatedAt = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	item.UpdatedAt = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	item.Status = "draft"
	f.insertedContent = append(f.insertedContent, item)
	return item, nil
}

func (f *fakeWorkspaceStore) ListIntegrations(ctx context.Context, workspaceID string) ([]store.Integration, error) {
	if f.listIntegrationsErr != nil {
		return nil, f.listIntegrationsErr
	}
	return f.listIntegrationsResult, nil
}

func (f *fakeWorkspaceStore) InsertIntegration(ctx context.Context, in store.Integration) (store.Integration, error) {
	if f.insertIntegrationErr != nil {
		return store.Integration{}, f.insertIntegrationErr
	}
	in.ID = "44444444-4444-4444-4444-444444444444"
	in.CreatedAt = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	in.UpdatedAt = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	f.insertedIntegrations = append(f.insertedIntegrations, in)
	if f.integrationsByID == nil {
		f.integrationsByID = make(map[string]store.Integration)
	}
	f.integrationsByID[in.ID] = in
	return in, nil
}

func (f *fakeWorkspaceStore) GetIntegrationInWorkspace(ctx context.Context, workspaceID, id string) (store.Integration, error) {
	in, ok := f.integrationsByID[id]
	if !ok || in.WorkspaceID != workspaceID {
		return store.Integration{}, store.ErrNotFound
	}
	return in, nil
}

func (f *fakeWorkspaceStore) SetIntegrationStatus(ctx context.Context, workspaceID, id, status string) error {
	in, ok := f.integrationsByID[id]
	if !ok || in.WorkspaceID != workspaceID {
		return store.ErrNotFound
	}
	in.Status = status
	f.integrationsByID[id] = in
	return nil
}

func (f *fakeWorkspaceStore) DeleteIntegration(ctx context.Context, workspaceID, id string) error {
	in, ok := f.integrationsByID[id]
	if !ok || in.WorkspaceID != workspaceID {
		return store.ErrNotFound
	}
	delete(f.integrationsByID, id)
	return nil
}

func (f *fakeWorkspaceStore) UpsertCapability(ctx context.Context, integrationID string, c store.IntegrationCapability) error {
	if f.capabilitiesByInID == nil {
		f.capabilitiesByInID = make(map[string][]store.IntegrationCapability)
	}
	list := f.capabilitiesByInID[integrationID]
	found := false
	for i, existing := range list {
		if existing.Capability == c.Capability {
			list[i] = c
			found = true
			break
		}
	}
	if !found {
		list = append(list, c)
	}
	f.capabilitiesByInID[integrationID] = list
	return nil
}

func (f *fakeWorkspaceStore) ListCapabilities(ctx context.Context, integrationID string) ([]store.IntegrationCapability, error) {
	if f.capabilitiesByInID == nil {
		return nil, nil
	}
	return f.capabilitiesByInID[integrationID], nil
}

func (f *fakeWorkspaceStore) PutCredentialSet(ctx context.Context, c store.CredentialSet) error {
	if f.credentialSetsByInID == nil {
		f.credentialSetsByInID = make(map[string]store.CredentialSet)
	}
	f.credentialSetsByInID[c.IntegrationID] = c
	return nil
}

func (f *fakeWorkspaceStore) GetCredentialSet(ctx context.Context, integrationID string) (store.CredentialSet, error) {
	if f.credentialSetsByInID == nil {
		return store.CredentialSet{}, store.ErrNotFound
	}
	c, ok := f.credentialSetsByInID[integrationID]
	if !ok {
		return store.CredentialSet{}, store.ErrNotFound
	}
	return c, nil
}

func (f *fakeWorkspaceStore) DeleteCredentialSet(ctx context.Context, integrationID string) error {
	if f.credentialSetsByInID == nil {
		return store.ErrNotFound
	}
	if _, ok := f.credentialSetsByInID[integrationID]; !ok {
		return store.ErrNotFound
	}
	delete(f.credentialSetsByInID, integrationID)
	return nil
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

func detailRouterWith(t *testing.T) (http.Handler, *fakeWorkspaceStore) {
	t.Helper()
	store := &fakeWorkspaceStore{
		byID: map[string]store.Workspace{
			"11111111-1111-1111-1111-111111111111": {
				ID: "11111111-1111-1111-1111-111111111111", Slug: "demo-studio",
				Name: "Demo Studio", Status: "active",
				CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}
	router := NewRouter(Deps{Logger: testLogger, Workspaces: store})
	return router, store
}

func TestWorkspaceDetailEndpoint(t *testing.T) {
	router, _ := detailRouterWith(t)

	rec := doJSON(t, router, http.MethodGet,
		"/api/v1/workspaces/11111111-1111-1111-1111-111111111111", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"brand":null`) {
		t.Errorf("an empty brand must be JSON null: %s", body)
	}
	if !strings.Contains(body, `"slug":"demo-studio"`) {
		t.Errorf("workspace missing from detail: %s", body)
	}

	// Unknown workspace is a 404, not 500.
	rec = doJSON(t, router, http.MethodGet, "/api/v1/workspaces/00000000-0000-0000-0000-000000000000", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown workspace must be 404, got %d", rec.Code)
	}
}

func TestWorkspaceBrandUpsert(t *testing.T) {
	router, fake := detailRouterWith(t)
	id := "11111111-1111-1111-1111-111111111111"

	rec := doJSON(t, router, http.MethodPut, "/api/v1/workspaces/"+id+"/brand",
		`{"businessDescription":" Coffee, baked. ","tone":"  ","prohibitedClaims":["  miracle cure ","", "prices"],"callsToAction":["Book now"],"rules":{"voice":"warm"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	saved := fake.upsertedBrand
	if saved == nil {
		t.Fatal("brand was not saved")
	}
	if saved.WorkspaceID != id {
		t.Errorf("saved profile points at workspace %q", saved.WorkspaceID)
	}
	if saved.BusinessDescription == nil || *saved.BusinessDescription != "Coffee, baked." {
		t.Errorf("businessDescription not trimmed: %+v", saved.BusinessDescription)
	}
	// A whitespace-only text field becomes null rather than an empty string.
	if saved.Tone != nil {
		t.Errorf("blank tone must become null, got %q", *saved.Tone)
	}
	if len(saved.ProhibitedClaims) != 2 || saved.ProhibitedClaims[0] != "miracle cure" {
		t.Errorf("prohibitedClaims not cleaned: %+v", saved.ProhibitedClaims)
	}
	if len(saved.CallsToAction) != 1 || saved.CallsToAction[0] != "Book now" {
		t.Errorf("callsToAction not cleaned: %+v", saved.CallsToAction)
	}
	if saved.Rules["voice"] != "warm" {
		t.Errorf("rules not stored: %+v", saved.Rules)
	}
	if !strings.Contains(rec.Body.String(), `"businessDescription":"Coffee, baked."`) {
		t.Errorf("response misses the saved values: %s", rec.Body.String())
	}
}

func TestWorkspaceBrandUpsertRejects(t *testing.T) {
	router, _ := detailRouterWith(t)
	id := "11111111-1111-1111-1111-111111111111"

	// Unknown workspace is 404.
	rec := doJSON(t, router, http.MethodPut, "/api/v1/workspaces/00000000-0000-0000-0000-000000000000/brand", `{}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown workspace must be 404, got %d", rec.Code)
	}
	// Longer than 20k text field is 400.
	long := strings.Repeat("a", 20_001)
	rec = doJSON(t, router, http.MethodPut, "/api/v1/workspaces/"+id+"/brand",
		`{"businessDescription":"`+long+`"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "too long") {
		t.Errorf("20001-char field must be 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	// Too many list items is 400.
	many := make([]string, 0, 101)
	for i := 0; i < 101; i++ {
		many = append(many, "claim")
	}
	listJSON := `["` + strings.Join(append(many, "")[:101], `","`) + `"]`
	rec = doJSON(t, router, http.MethodPut, "/api/v1/workspaces/"+id+"/brand",
		`{"prohibitedClaims":`+listJSON+`}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("101 list items must be 400, got %d", rec.Code)
	}
}

func TestPatchWorkspaceEndpoint(t *testing.T) {
	router, fake := detailRouterWith(t)
	id := "11111111-1111-1111-1111-111111111111"

	rec := doJSON(t, router, "PATCH", "/api/v1/workspaces/"+id,
		`{"name":" Demo Studio Renamed ","status":"archived"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	saved := fake.updated[0]
	if saved.Name != "Demo Studio Renamed" || saved.Status != "archived" {
		t.Errorf("patch not applied: %+v", saved)
	}
	if len(fake.auditEvents) != 1 || fake.auditEvents[0].Action != "workspace.updated" {
		t.Errorf("audit row missing for patch: %+v", fake.auditEvents)
	}
	if !strings.Contains(rec.Body.String(), `"name":"Demo Studio Renamed"`) {
		t.Errorf("response missing renamed workspace: %s", rec.Body.String())
	}

	// Omitted fields keep their current values.
	fake.updated = nil
	rec = doJSON(t, router, "PATCH", "/api/v1/workspaces/"+id, `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if fake.updated[0].Status != "active" || fake.updated[0].Name != "Demo Studio" {
		t.Errorf("empty patch must keep current values: %+v", fake.updated[0])
	}

	for _, tc := range []struct{ name, body, want string }{
		{"empty name", `{"name":"   "}`, "name cannot be empty"},
		{"bad status", `{"status":"closed"}`, "status must be active or archived"},
		{"not json", `nope`, "not valid JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doJSON(t, router, "PATCH", "/api/v1/workspaces/"+id, tc.body)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tc.want) {
				t.Errorf("got %d body=%s, want 400 containing %q", rec.Code, rec.Body.String(), tc.want)
			}
		})
	}

	// Unknown workspace is 404.
	rec = doJSON(t, router, "PATCH", "/api/v1/workspaces/00000000-0000-0000-0000-000000000000", `{"name":"x"}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown workspace must be 404, got %d", rec.Code)
	}

	// Store-level validation errors are 400 without internal detail.
	errRouter := NewRouter(Deps{Logger: testLogger, Workspaces: &fakeWorkspaceStore{
		byID:      map[string]store.Workspace{id: {}},
		updateErr: store.ErrNameRequired,
	}})
	rec = doJSON(t, errRouter, "PATCH", "/api/v1/workspaces/"+id, `{"name":"x"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("ErrNameRequired must be 400, got %d", rec.Code)
	}

	// Server errors are 500 without leaking internals.
	boom := NewRouter(Deps{Logger: testLogger, Workspaces: &fakeWorkspaceStore{
		byID:      map[string]store.Workspace{id: {}},
		updateErr: errors.New("secret-detail"),
	}})
	rec = doJSON(t, boom, "PATCH", "/api/v1/workspaces/"+id, `{"name":"x"}`)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "secret") {
		t.Errorf("internal errors must be 500 without detail, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeleteWorkspaceEndpoint(t *testing.T) {
	router, fake := detailRouterWith(t)
	id := "11111111-1111-1111-1111-111111111111"

	// Missing or wrong confirmation is refused.
	rec := doJSON(t, router, http.MethodDelete, "/api/v1/workspaces/"+id, `{}`)
	if rec.Code != http.StatusBadRequest || len(fake.deleted) != 0 {
		t.Errorf("delete without confirmation must be 400, got %d", rec.Code)
	}
	rec = doJSON(t, router, http.MethodDelete, "/api/v1/workspaces/"+id, `{"confirmSlug":"wrong"}`)
	if rec.Code != http.StatusBadRequest || len(fake.deleted) != 0 {
		t.Errorf("wrong confirmation must be 400, got %d", rec.Code)
	}

	// Correct confirmation deletes and audits.
	rec = doJSON(t, router, http.MethodDelete, "/api/v1/workspaces/"+id, `{"confirmSlug":"demo-studio"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", rec.Code, rec.Body.String())
	}
	if len(fake.deleted) != 1 || fake.deleted[0] != id {
		t.Errorf("DeleteWorkspace not called correctly: %v", fake.deleted)
	}
	last := fake.auditEvents[len(fake.auditEvents)-1]
	if last.Action != "workspace.deleted" || last.WorkspaceID != nil {
		t.Errorf("deletion audit row wrong: %+v", last)
	}
	if last.Details["slug"] != "demo-studio" {
		t.Errorf("deletion audit must carry the slug: %+v", last.Details)
	}

	// Unknown workspace is 404 and audits nothing.
	auditCount := len(fake.auditEvents)
	rec = doJSON(t, router, http.MethodDelete, "/api/v1/workspaces/00000000-0000-0000-0000-000000000000", `{}`)
	if rec.Code != http.StatusNotFound || len(fake.auditEvents) != auditCount {
		t.Errorf("unknown workspace must be 404 with no audit rows, got %d", rec.Code)
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
