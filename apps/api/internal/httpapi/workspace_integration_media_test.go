package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"marketingops/api/internal/secretcipher"
	"marketingops/api/internal/store"
)

// mediaTestHarness prepares the workspace store, credentials, and a stand-in
// Sanity asset endpoint shared by the media upload tests.
type mediaTestHarness struct {
	ws        *fakeWorkspaceStore
	key       []byte
	assetSrv  *httptest.Server
	assetPath string // captured upload path
	assetAuth string // captured upload authorization
	assetCode int    // scripted Sanity status (0 = success)
}

func newMediaHarness(t *testing.T, adapterKind string) *mediaTestHarness {
	t.Helper()
	key := []byte(strings.Repeat("k", 32))
	wsID := "ws-1"
	inID := "in-1"

	ws := &fakeWorkspaceStore{
		byID: map[string]store.Workspace{
			wsID: {ID: wsID, Slug: "probe", Name: "Probe", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
		integrationsByID: map[string]store.Integration{
			inID: {
				ID: inID, WorkspaceID: wsID, AdapterKind: adapterKind, Name: "sanity probe",
				Status: "connected",
				Config: map[string]any{"projectId": "p123", "dataset": "production"},
			},
		},
	}
	if adapterKind == "sanity" {
		enc, err := secretcipher.Encrypt(key, []byte(`{"token":"sk-media-token"}`))
		if err != nil {
			t.Fatalf("encrypt: %v", err)
		}
		ws.credentialSetsByInID = map[string]store.CredentialSet{
			inID: {IntegrationID: inID, KeyVersion: 1, Ciphertext: enc},
		}
	}

	h := &mediaTestHarness{ws: ws, key: key, assetCode: 0}
	h.assetSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.assetPath = r.URL.Path
		h.assetAuth = r.Header.Get("Authorization")
		if h.assetCode != 0 {
			code := h.assetCode
			h.assetCode = 0
			http.Error(w, "scripted asset failure", code)
			return
		}
		_, _ = w.Write([]byte(`{"document":{"_id":"image-abc-800x600-jpg"},"url":"https://cdn.sanity.io/images/p123/production/abc-800x600.png"}`))
	}))
	t.Cleanup(h.assetSrv.Close)
	return h
}

func (h *mediaTestHarness) post(t *testing.T, body []byte, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/ws-1/integrations/in-1/media", bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	req.SetPathValue("id", "ws-1")
	req.SetPathValue("integrationId", "in-1")
	rec := httptest.NewRecorder()
	handler := uploadIntegrationMediaHandler(h.ws, h.key, h.assetSrv.URL, h.assetSrv.Client())
	handler(rec, req)
	return rec
}

func (h *mediaTestHarness) pngUpload(t *testing.T) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "logo.png")
	if err != nil {
		t.Fatalf("form file: %v", err)
	}
	// Real PNG magic so DetectContentType classifies it as an image.
	data := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, []byte("rest-of-png")...)
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return body.Bytes(), writer.FormDataContentType()
}

func TestMediaUploadProxiesToSanity(t *testing.T) {
	h := newMediaHarness(t, "sanity")
	body, contentType := h.pngUpload(t)
	rec := h.post(t, body, contentType)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["ref"] != "image-abc-800x600-jpg" {
		t.Errorf("ref = %q", resp["ref"])
	}
	if h.assetPath != "/v1/assets/images/production" {
		t.Errorf("asset path = %q", h.assetPath)
	}
	if !strings.Contains(h.assetAuth, "Bearer sk-media-token") {
		t.Errorf("asset auth = %q", h.assetAuth)
	}

	if len(h.ws.auditEvents) != 1 || h.ws.auditEvents[0].Action != "integration.media_uploaded" {
		t.Errorf("audit events = %+v", h.ws.auditEvents)
	}
	auditBytes, _ := json.Marshal(h.ws.auditEvents[0].Details)
	if strings.Contains(string(auditBytes), "sk-media-token") {
		t.Error("secret token found in audit details")
	}
	if strings.Contains(rec.Body.String(), "sk-media-token") {
		t.Error("secret token leaked in response")
	}
}

func TestMediaUploadRejectsNonImage(t *testing.T) {
	h := newMediaHarness(t, "sanity")
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "notes.txt")
	_, _ = part.Write([]byte("plain text is not an image"))
	_ = writer.Close()
	rec := h.post(t, body.Bytes(), writer.FormDataContentType())
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body.String())
	}
	if len(h.ws.auditEvents) != 0 {
		t.Errorf("no audit expected, got %+v", h.ws.auditEvents)
	}
}

func TestMediaUploadRejectsNonSanityAdapter(t *testing.T) {
	h := newMediaHarness(t, "generic_api")
	body, contentType := h.pngUpload(t)
	rec := h.post(t, body, contentType)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body.String())
	}
}

func TestMediaUploadRequiresCredentials(t *testing.T) {
	h := newMediaHarness(t, "sanity")
	h.ws.credentialSetsByInID = nil
	body, contentType := h.pngUpload(t)
	rec := h.post(t, body, contentType)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "credentials") {
		t.Errorf("message should mention credentials: %s", rec.Body.String())
	}
}

func TestMediaUploadSurfacesSanityFailureWithoutLeakingBody(t *testing.T) {
	h := newMediaHarness(t, "sanity")
	h.assetCode = http.StatusUnauthorized
	body, contentType := h.pngUpload(t)
	rec := h.post(t, body, contentType)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "scripted asset failure") {
		t.Error("raw provider error text must not reach the client")
	}
}
