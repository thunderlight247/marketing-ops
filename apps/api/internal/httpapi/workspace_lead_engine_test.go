package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"marketingops/api/internal/store"
)

const whatsAppTestWsID = "22222222-2222-2222-2222-222222222222"

type whatsAppFake struct {
	*fakeWorkspaceStore
	statusUpdates []string
}

func (f *whatsAppFake) SetWhatsAppStatus(_ context.Context, _, status, _ string) error {
	f.statusUpdates = append(f.statusUpdates, status)
	return nil
}

func newWhatsAppFake(t *testing.T) *whatsAppFake {
	t.Helper()
	return &whatsAppFake{fakeWorkspaceStore: &fakeWorkspaceStore{
		byID: map[string]store.Workspace{
			whatsAppTestWsID: {ID: whatsAppTestWsID, Slug: "demo", Name: "Demo Studio"},
		},
	}}
}

func TestPairWhatsAppForwardsNormalisedOwnNumber(t *testing.T) {
	f := newWhatsAppFake(t)
	var gotPath string
	var gotBody map[string]string
	dispatcher := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":       "qr_ready",
			"qr":           "data:image/png;base64,qr",
			"pairingPhone": "233241234567",
			"pairingCode":  "ABCD1234",
			"source":       "own",
		})
	}))
	defer dispatcher.Close()
	t.Setenv("WHATSAPP_DISPATCHER_URL", dispatcher.URL)
	t.Setenv("WHATSAPP_DISPATCHER_SECRET", "")

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/workspaces/"+whatsAppTestWsID+"/lead-engine/whatsapp/pair?own=1",
		bytes.NewBufferString(`{"phone":"+233 24 123 4567"}`),
	)
	req.SetPathValue("id", whatsAppTestWsID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	pairWhatsAppHandler(f).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("pair status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if gotPath != "/sessions/"+whatsAppTestWsID+"/own/pair" {
		t.Errorf("dispatcher path = %q", gotPath)
	}
	if gotBody["phone"] != "233241234567" {
		t.Errorf("dispatcher phone = %q", gotBody["phone"])
	}
	if !strings.Contains(rec.Body.String(), `"pairingCode":"ABCD1234"`) {
		t.Errorf("pairing code missing from response: %s", rec.Body.String())
	}
	if len(f.statusUpdates) != 1 || f.statusUpdates[0] != "qr_ready" {
		t.Errorf("status updates = %v", f.statusUpdates)
	}
}

func TestPairWhatsAppRejectsAnInvalidNumberBeforeCallingDispatcher(t *testing.T) {
	f := newWhatsAppFake(t)
	called := false
	dispatcher := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer dispatcher.Close()
	t.Setenv("WHATSAPP_DISPATCHER_URL", dispatcher.URL)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/workspaces/"+whatsAppTestWsID+"/lead-engine/whatsapp/pair?own=1",
		bytes.NewBufferString(`{"phone":"123"}`),
	)
	req.SetPathValue("id", whatsAppTestWsID)
	rec := httptest.NewRecorder()

	pairWhatsAppHandler(f).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid number status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if called {
		t.Error("dispatcher must not be called for an invalid number")
	}
}
