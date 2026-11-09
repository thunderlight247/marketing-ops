package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeUnsubscribeStore struct {
	workspaceID, email, reason string
	calls                      int
}

func (f *fakeUnsubscribeStore) SuppressEmail(_ context.Context, workspaceID, email, reason string) error {
	f.calls++
	f.workspaceID, f.email, f.reason = workspaceID, email, reason
	return nil
}

var unsubKey = []byte("0123456789abcdef0123456789abcdef")

func TestUnsubscribeTokenRoundTrip(t *testing.T) {
	tok := unsubscribeToken(unsubKey, "ws-1", "Owner@Example.com ")
	ws, email, ok := parseUnsubscribeToken(unsubKey, tok)
	if !ok || ws != "ws-1" || email != "owner@example.com" {
		t.Fatalf("parse = %q %q %v", ws, email, ok)
	}
}

func TestUnsubscribeTokenRejectsTamperingAndWrongKey(t *testing.T) {
	tok := unsubscribeToken(unsubKey, "ws-1", "a@example.com")
	if _, _, ok := parseUnsubscribeToken([]byte("another-key-another-key-another!"), tok); ok {
		t.Error("token accepted under a different key")
	}
	if _, _, ok := parseUnsubscribeToken(unsubKey, tok+"x"); ok {
		t.Error("tampered signature accepted")
	}
	if _, _, ok := parseUnsubscribeToken(unsubKey, "garbage"); ok {
		t.Error("malformed token accepted")
	}
}

func TestUnsubscribeEndpointSuppressesValidToken(t *testing.T) {
	fake := &fakeUnsubscribeStore{}
	h := unsubscribeHandler(fake, unsubKey)
	tok := unsubscribeToken(unsubKey, "ws-1", "a@example.com")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/email/unsubscribe?token="+tok, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if fake.calls != 1 || fake.workspaceID != "ws-1" || fake.email != "a@example.com" || fake.reason != "unsubscribe" {
		t.Errorf("suppression = %+v", fake)
	}
}

func TestUnsubscribeEndpointRejectsBadToken(t *testing.T) {
	fake := &fakeUnsubscribeStore{}
	h := unsubscribeHandler(fake, unsubKey)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/email/unsubscribe?token=bogus", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if fake.calls != 0 {
		t.Error("invalid token must not change suppressions")
	}
}
