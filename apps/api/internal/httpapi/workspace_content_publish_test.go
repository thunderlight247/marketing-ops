package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"marketingops/api/internal/integration"
	"marketingops/api/internal/secretcipher"
	"marketingops/api/internal/store"
)

// fakeSanityServer stands in for the Sanity HTTP API at the document level.
// It records every mutation so tests can assert what the handler wrote, and
// can be scripted to fail specific calls.
type fakeSanityServer struct {
	mu       sync.Mutex
	docs     map[string]map[string]any
	txCount  int
	nextCode int  // if set, the next mutate call answers with this status
	publicOK bool // whether /posts/<slug> answers 200
	mutates  []map[string]any
}

func newFakeSanityServer() *fakeSanityServer {
	return &fakeSanityServer{docs: map[string]map[string]any{}, publicOK: true}
}

func (s *fakeSanityServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/data/mutate/") {
			http.NotFound(w, r)
			return
		}
		s.mu.Lock()
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		var env struct {
			Mutations []map[string]any `json:"mutations"`
		}
		_ = json.Unmarshal(body, &env)
		s.mutates = append(s.mutates, env.Mutations...)
		if s.nextCode != 0 {
			code := s.nextCode
			s.nextCode = 0 // fail only this mutation; later retry can succeed
			s.mu.Unlock()
			http.Error(w, "scripted failure", code)
			return
		}
		for _, m := range env.Mutations {
			if doc, ok := m["createOrReplace"].(map[string]any); ok {
				id, _ := doc["_id"].(string)
				if id != "" {
					s.docs[id] = doc
				}
			}
			if del, ok := m["delete"].(map[string]any); ok {
				id, _ := del["id"].(string)
				delete(s.docs, id)
			}
		}
		s.txCount++
		tx := s.txCount
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"transactionId": "fake-tx-" + strconv.Itoa(tx)})
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		path := strings.SplitN(r.URL.Path, "/data/doc/", 2)
		if len(path) != 2 {
			// Public URL verification stand-in: /posts/<slug> -> 200.
			if strings.HasPrefix(r.URL.Path, "/posts/") {
				s.mu.Lock()
				ok := s.publicOK
				s.mu.Unlock()
				if ok {
					w.WriteHeader(http.StatusOK)
				} else {
					w.WriteHeader(http.StatusNotFound)
				}
				return
			}
			http.NotFound(w, r)
			return
		}
		parts := strings.SplitN(strings.Trim(path[1], "/"), "/", 2)
		if len(parts) != 2 {
			http.NotFound(w, r)
			return
		}
		s.mu.Lock()
		doc, ok := s.docs[parts[1]]
		s.mu.Unlock()
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "document not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"documents": []map[string]any{doc}})
	})
	return mux
}

// sanityTestHarness bundles everything one publish test needs.
type sanityTestHarness struct {
	ws        *fakeWorkspaceStore
	sanity    *fakeSanityServer
	sanitySrv *httptest.Server
	key       []byte
	registry  *integration.Registry
	handler   http.Handler
}

func newSanityHarness(t *testing.T) *sanityTestHarness {
	t.Helper()
	key := []byte(strings.Repeat("k", 32))
	ws := &fakeWorkspaceStore{
		byID: map[string]store.Workspace{
			"ws-1": {
				ID: "ws-1", Slug: "probe-ws", Name: "Probe", Status: "active",
				WebsiteURL: strPtr("https://public.example"), CreatedAt: time.Now(), UpdatedAt: time.Now(),
			},
		},
		contentItemsByID: map[string]store.ContentItem{
			"it-1": {
				ID: "it-1", WorkspaceID: "ws-1", Kind: "blog_article", Status: "approved",
				Title: "Probe", Slug: strPtr("probe-slug"),
				CreatedAt: time.Now(), UpdatedAt: time.Now(),
			},
		},
		contentVersionsByItem: map[string][]store.ContentVersion{
			"it-1": {
				{ID: "v-1", ContentItemID: "it-1", VersionNumber: 1, Title: "Probe",
					Excerpt: strPtr("e"), Body: "# Probe\n\nBody **bold**.", CreatedAt: time.Now()},
			},
		},
		decisionsByItem: map[string][]store.Approval{
			"it-1": {{ID: "a-1", ContentItemID: "it-1", ContentVersionID: "v-1", Decision: "approved"}},
		},
		integrationsByID: map[string]store.Integration{
			"in-1": {
				ID: "in-1", WorkspaceID: "ws-1", AdapterKind: "sanity", Name: "sanity probe",
				Status: "connected",
				Config: map[string]any{
					"projectId": "p123", "dataset": "production", "publicUrlPrefix": "/posts/",
				},
			},
		},
	}
	enc, err := secretcipher.Encrypt(key, []byte(`{"token":"test-write-token"}`))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	ws.credentialSetsByInID = map[string]store.CredentialSet{
		"in-1": {IntegrationID: "in-1", KeyVersion: 1, Ciphertext: enc},
	}

	sanity := newFakeSanityServer()
	sanitySrv := httptest.NewServer(sanity.handler())
	t.Cleanup(sanitySrv.Close)

	registry := integration.NewDefaultRegistry()
	h := &sanityTestHarness{
		ws: ws, sanity: sanity, sanitySrv: sanitySrv, key: key, registry: registry,
		handler: publishWorkspaceContentHandler(ws, key, registry, sanitySrv.URL, sanitySrv.Client()),
	}
	return h
}

func (h *sanityTestHarness) postIntegration(t *testing.T, integrationID, mode string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/workspaces/ws-1/content/it-1/publish",
		strings.NewReader(`{"integrationId":"`+integrationID+`","mode":"`+mode+`"}`))
	req.SetPathValue("id", "ws-1")
	req.SetPathValue("itemId", "it-1")
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	return rec
}

func (h *sanityTestHarness) post(t *testing.T, mode string) *httptest.ResponseRecorder {
	return h.postIntegration(t, "in-1", mode)
}

func TestPublishCarriesMainImageFromMetadata(t *testing.T) {
	h := newSanityHarness(t)
	versions := h.ws.contentVersionsByItem["it-1"]
	versions[0].Metadata = map[string]any{
		"mainImage": map[string]any{
			"ref": "image-hero-800x600-jpg",
			"src": "https://cdn.sanity.io/images/p/prod/hero.jpg",
			"alt": "Hero image",
		},
	}
	h.ws.contentVersionsByItem["it-1"] = versions

	rec := h.post(t, "draft")
	if rec.Code != http.StatusCreated {
		t.Fatalf("draft status = %d body %s", rec.Code, rec.Body.String())
	}
	h.sanity.mu.Lock()
	defer h.sanity.mu.Unlock()
	doc, ok := h.sanity.docs["drafts.mktops-it-1"]
	if !ok {
		t.Fatal("draft document missing")
	}
	image, ok := doc["mainImage"].(map[string]any)
	if !ok {
		t.Fatalf("mainImage missing from the written document: %+v", doc)
	}
	asset, ok := image["asset"].(map[string]any)
	if !ok || asset["_ref"] != "image-hero-800x600-jpg" {
		t.Fatalf("mainImage asset ref wrong: %+v", image)
	}
	if image["alt"] != "Hero image" {
		t.Errorf("mainImage alt wrong: %+v", image)
	}
	// The CDN preview URL is browser-only content and must not be written.
	if _, leak := image["src"]; leak {
		t.Errorf("mainImage src must not be sent to Sanity: %+v", image)
	}
}

func TestPublishDraftModeCreatesSanityDraft(t *testing.T) {
	h := newSanityHarness(t)
	rec := h.post(t, "draft")
	if rec.Code != http.StatusCreated {
		t.Fatalf("draft mode status = %d body %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Mode    string  `json:"mode"`
		DraftID *string `json:"draftId"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Mode != "draft" || resp.DraftID == nil {
		t.Fatalf("malformed draft response: %s", rec.Body.String())
	}
	if got := *resp.DraftID; got != "drafts.mktops-it-1" {
		t.Errorf("draftId = %q", got)
	}
	h.sanity.mu.Lock()
	defer h.sanity.mu.Unlock()
	doc, ok := h.sanity.docs["drafts.mktops-it-1"]
	if !ok {
		t.Fatal("the draft document was not written to Sanity")
	}
	if doc["title"] != "Probe" {
		t.Errorf("draft title = %v", doc["title"])
	}
	if _, isPublic := h.sanity.docs["mktops-it-1"]; isPublic {
		t.Error("draft-only mode must not create or replace the public document")
	}
	actions := []string{}
	for _, e := range h.ws.auditEvents {
		actions = append(actions, e.Action)
	}
	if !containsAction(actions, "content.publisher_draft_created") {
		t.Errorf("draft audit missing, got %v", actions)
	}
}

func containsAction(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestPublishRequiresApproval(t *testing.T) {
	h := newSanityHarness(t)
	item := h.ws.contentItemsByID["it-1"]
	item.Status = "draft"
	h.ws.contentItemsByID["it-1"] = item
	rec := h.post(t, "publish")
	if rec.Code != http.StatusConflict {
		t.Fatalf("unapproved publish status = %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "approved") {
		t.Errorf("message should mention approval: %s", rec.Body.String())
	}
	if len(h.ws.insertedJobs) != 0 {
		t.Errorf("no job may be created for unapproved content, got %d", len(h.ws.insertedJobs))
	}
}

func TestPublishSuccessRecordsJobArtifactAndStatus(t *testing.T) {
	h := newSanityHarness(t)
	// Simulate an older draft that does not match the latest approved backend
	// version. Publish must overwrite it from the approved version first while
	// preserving its Sanity creation timestamp.
	h.sanity.docs["drafts.mktops-it-1"] = map[string]any{
		"_id": "drafts.mktops-it-1", "_type": "post",
		"_createdAt": "2026-01-02T03:04:05Z", "title": "stale Studio draft",
		"slug": map[string]any{"_type": "slug", "current": "old-slug"}, "body": []any{},
	}
	rec := h.post(t, "publish")
	if rec.Code != http.StatusOK {
		t.Fatalf("publish status = %d body %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Item      contentItemJSON `json:"item"`
		Mode      string          `json:"mode"`
		JobID     *string         `json:"jobId"`
		PublicURL *string         `json:"publicUrl"`
		URLVerify *bool           `json:"urlVerified"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Mode != "publish" || resp.JobID == nil {
		t.Fatalf("malformed publish response: %s", rec.Body.String())
	}

	// The published document exists in Sanity and the draft is gone.
	h.sanity.mu.Lock()
	publishedDoc, published := h.sanity.docs["mktops-it-1"]
	_, draftGone := h.sanity.docs["drafts.mktops-it-1"]
	envelopes := h.sanity.txCount
	h.sanity.mu.Unlock()
	if !published {
		t.Error("the published document is missing in Sanity")
	} else {
		if publishedDoc["title"] != "Probe" {
			t.Errorf("published stale content instead of approved version: title=%v", publishedDoc["title"])
		}
		if publishedDoc["_createdAt"] != "2026-01-02T03:04:05Z" {
			t.Errorf("published document creation timestamp was not preserved: %v", publishedDoc["_createdAt"])
		}
	}
	if draftGone {
		t.Error("the draft document should be deleted after publish")
	}
	// Draft refresh and the publish transaction each issue one mutation.
	if envelopes != 2 {
		t.Errorf("expected two transactions, got %d", envelopes)
	}

	if len(h.ws.insertedJobs) != 1 {
		t.Fatalf("one job expected, got %d", len(h.ws.insertedJobs))
	}
	jobID := h.ws.insertedJobs[0].ID
	if *resp.JobID != jobID {
		t.Errorf("jobId mismatch: %s vs %s", *resp.JobID, jobID)
	}
	if h.ws.jobsByKey[h.ws.insertedJobs[0].IdempotencyKey].Status != "succeeded" {
		t.Error("the job must end up succeeded")
	}
	if len(h.ws.recordedArtifacts) != 1 || h.ws.recordedArtifacts[0].jobID != jobID {
		t.Errorf("artifact not recorded correctly: %+v", h.ws.recordedArtifacts)
	} else {
		if h.ws.recordedArtifacts[0].externalURL != "https://public.example/posts/probe-slug" {
			t.Errorf("artifact URL not recorded: %+v", h.ws.recordedArtifacts[0])
		}
		if h.ws.recordedArtifacts[0].externalID != "mktops-it-1" {
			t.Errorf("artifact ID should match the stable Sanity document: %+v", h.ws.recordedArtifacts[0])
		}
	}
	if got := h.ws.contentItemsByID["it-1"].Status; got != "published" {
		t.Errorf("item status after publish = %q", got)
	}
	if !containsAction(actionsOf(h.ws), "content.published") {
		t.Error("publish audit missing")
	}
}

func actionsOf(f *fakeWorkspaceStore) []string {
	out := []string{}
	for _, e := range f.auditEvents {
		out = append(out, e.Action)
	}
	return out
}

func TestPublishFailureMarksJobFailedAndRetriesResumeIt(t *testing.T) {
	h := newSanityHarness(t)

	// First attempt: the destination API refuses the mutation.
	h.sanity.mu.Lock()
	h.sanity.nextCode = http.StatusForbidden
	h.sanity.mu.Unlock()
	rec := h.post(t, "publish")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("failing publish status = %d body %s", rec.Code, rec.Body.String())
	}
	if len(h.ws.insertedJobs) != 1 {
		t.Fatalf("one job expected, got %d", len(h.ws.insertedJobs))
	}
	key := h.ws.insertedJobs[0].IdempotencyKey
	stored := h.ws.jobsByKey[key]
	if stored.Status != "failed" || stored.LastError == nil {
		t.Fatalf("job must be failed with last_error, got %+v", stored)
	}
	if strings.Contains(*stored.LastError, "test-write-token") {
		t.Error("last_error must never contain the token")
	}

	// Second attempt (retry): the destination API works again. The SAME job must
	// be resumed, not duplicated.
	h.sanity.mu.Lock()
	h.sanity.nextCode = 0
	h.sanity.mu.Unlock()
	rec2 := h.post(t, "publish")
	if rec2.Code != http.StatusOK {
		t.Fatalf("retry publish status = %d body %s", rec2.Code, rec2.Body.String())
	}
	if len(h.ws.insertedJobs) != 1 {
		t.Errorf("the retry must reuse the existing job, got %d inserted", len(h.ws.insertedJobs))
	}
	if h.ws.jobsByKey[key].Status != "succeeded" {
		t.Error("the resumed job must end up succeeded")
	}
}

func TestPublishTokenExpiryFlagsIntegrationForReconnection(t *testing.T) {
	h := newSanityHarness(t)
	// The Sanity server refuses the write token: HTTP 401 marks a rejected
	// credential (Milestone 10).
	h.sanity.mu.Lock()
	h.sanity.nextCode = http.StatusUnauthorized
	h.sanity.mu.Unlock()

	rec := h.post(t, "publish")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("publish status = %d body %s", rec.Code, rec.Body.String())
	}
	in := h.ws.integrationsByID["in-1"]
	if in.Status != integration.LifecycleError {
		t.Fatalf("the integration must land in the %q lifecycle after a rejected token, got %q", integration.LifecycleError, in.Status)
	}
	caps := h.ws.capabilitiesByInID["in-1"]
	var publishCap *store.IntegrationCapability
	for i := range caps {
		if caps[i].Capability == integration.CapPublish {
			publishCap = &caps[i]
		}
	}
	if publishCap == nil || publishCap.Status != integration.StatusFailed {
		t.Fatalf("expected the publish capability marked failed, got %+v", caps)
	}
	if publishCap.Details == nil || strings.Contains(*publishCap.Details, "test-write-token") {
		t.Fatalf("the capability detail must be safe and non-empty, got %+v", publishCap.Details)
	}
}

func TestPublishNetworkFailureKeepsIntegrationStatus(t *testing.T) {
	h := newSanityHarness(t)
	h.sanity.mu.Lock()
	h.sanity.nextCode = http.StatusServiceUnavailable
	h.sanity.mu.Unlock()

	rec := h.post(t, "publish")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("publish status = %d body %s", rec.Code, rec.Body.String())
	}
	// A temporarily unavailable destination is not a credential problem; the
	// integration stays connected so no reconnection banner appears.
	if in := h.ws.integrationsByID["in-1"]; in.Status != "connected" {
		t.Fatalf("network failures must keep the connected lifecycle, got %q", in.Status)
	}
}

func TestPublishTwiceConflictsOnSucceededJob(t *testing.T) {
	h := newSanityHarness(t)
	if rec := h.post(t, "publish"); rec.Code != http.StatusOK {
		t.Fatalf("first publish status = %d", rec.Code)
	}
	rec := h.post(t, "publish")
	if rec.Code != http.StatusConflict {
		t.Fatalf("second publish status = %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "already published") {
		t.Errorf("conflict message wrong: %s", rec.Body.String())
	}
}

// Publish with a stored credential set that cannot be decrypted must say so
// distinctly from "no credentials": the fix is re-entering, not adding.
func TestPublishRejectsUnreadableCredentials(t *testing.T) {
	h := newSanityHarness(t)
	h.ws.credentialSetsByInID["in-1"] = store.CredentialSet{
		IntegrationID: "in-1", KeyVersion: 9, Ciphertext: []byte("poison-poison-poison-poison"),
	}
	rec := h.postIntegration(t, "in-1", "draft")
	if rec.Code != http.StatusConflict {
		t.Fatalf("unreadable credentials status = %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "re-enter them") {
		t.Errorf("expected the re-enter guidance, got: %s", rec.Body.String())
	}
	if len(h.sanity.mutates) != 0 {
		t.Errorf("unreadable credentials must not reach the destination; got %d mutations", len(h.sanity.mutates))
	}
	if len(h.ws.recordedArtifacts) != 0 {
		t.Errorf("no artifact may be recorded without a successful publish, got %+v", h.ws.recordedArtifacts)
	}
}

type contractTestPublisherAdapter struct {
	publisher *contractTestPublisher
}

func (a contractTestPublisherAdapter) NewPublisher(config map[string]any, credentials map[string]string, _ string, _ *http.Client) (integration.Publisher, error) {
	if config["baseUrl"] != "https://cms.example.test" || credentials["apiKey"] != "encrypted-api-key" {
		return nil, integration.ErrPublisherCredentialsMissing
	}
	return a.publisher, nil
}

type contractTestPublisher struct {
	payload integration.ContentPayload
	drafts  int
	writes  int
}

func (p *contractTestPublisher) CreateDraft(_ context.Context, payload integration.ContentPayload) (string, string, error) {
	p.payload = payload
	p.drafts++
	return "cms-draft-1", "cms-tx-draft", nil
}

func (p *contractTestPublisher) Publish(_ context.Context, payload integration.ContentPayload) (integration.PublishOutcome, error) {
	p.payload = payload
	p.writes++
	return integration.PublishOutcome{ExternalID: "cms-article-1", TransactionID: "cms-tx-publish"}, nil
}

func (p *contractTestPublisher) VerifyPublicURL(_ context.Context, siteURL, slug string) integration.URLVerification {
	return integration.URLVerification{URL: siteURL + "/articles/" + slug, Verified: true, HTTPStatus: http.StatusOK}
}

func TestPublishDispatchesRegisteredAdapter(t *testing.T) {
	h := newSanityHarness(t)
	integrationRow := h.ws.integrationsByID["in-1"]
	integrationRow.AdapterKind = "generic_api"
	integrationRow.Config = map[string]any{"baseUrl": "https://cms.example.test"}
	h.ws.integrationsByID["in-1"] = integrationRow
	credentialCiphertext, err := secretcipher.Encrypt(h.key, []byte(`{"apiKey":"encrypted-api-key"}`))
	if err != nil {
		t.Fatalf("encrypt credentials: %v", err)
	}
	h.ws.credentialSetsByInID["in-1"] = store.CredentialSet{IntegrationID: "in-1", Ciphertext: credentialCiphertext}
	publisher := &contractTestPublisher{}
	h.registry.RegisterPublisher("generic_api", contractTestPublisherAdapter{publisher: publisher})

	rec := h.post(t, "publish")
	if rec.Code != http.StatusOK {
		t.Fatalf("registered publisher status = %d, body %s", rec.Code, rec.Body.String())
	}
	if publisher.drafts != 1 || publisher.writes != 1 {
		t.Fatalf("publisher calls: drafts=%d publishes=%d", publisher.drafts, publisher.writes)
	}
	if publisher.payload.Title != "Probe" || publisher.payload.Slug != "probe-slug" || publisher.payload.Body != "# Probe\n\nBody **bold**." {
		t.Errorf("approved content snapshot was not passed intact: %+v", publisher.payload)
	}
	if len(h.sanity.mutates) != 0 {
		t.Errorf("registered custom publisher must not call Sanity: %d mutations", len(h.sanity.mutates))
	}
	if len(h.ws.recordedArtifacts) != 1 || h.ws.recordedArtifacts[0].externalID != "cms-article-1" || h.ws.recordedArtifacts[0].externalURL != "https://public.example/articles/probe-slug" {
		t.Errorf("custom publisher artifact not recorded: %+v", h.ws.recordedArtifacts)
	}
}

func TestPublishRejectsAdapterWithoutPublisher(t *testing.T) {
	h := newSanityHarness(t)
	integrationRow := h.ws.integrationsByID["in-1"]
	integrationRow.AdapterKind = "generic_api"
	integrationRow.Config = map[string]any{"baseUrl": "http://127.0.0.1:9"}
	h.ws.integrationsByID["in-1"] = integrationRow
	rec := h.postIntegration(t, "in-1", "draft")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unsupported publisher status = %d, body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "do not support publishing yet") {
		t.Fatalf("unsupported publisher should be clear: %s", rec.Body.String())
	}
	if len(h.sanity.mutates) != 0 {
		t.Errorf("unregistered publisher must not make network writes; got %d Sanity mutations", len(h.sanity.mutates))
	}
	if len(h.ws.insertedJobs) != 0 {
		t.Errorf("unregistered publisher must not create a publish job, got %d", len(h.ws.insertedJobs))
	}
}

func TestPublishRejectsUnknownIntegration(t *testing.T) {
	h := newSanityHarness(t)
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/workspaces/ws-1/content/it-1/publish",
		strings.NewReader(`{"integrationId":"nope","mode":"draft"}`))
	req.SetPathValue("id", "ws-1")
	req.SetPathValue("itemId", "it-1")
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown integration status = %d", rec.Code)
	}
}
