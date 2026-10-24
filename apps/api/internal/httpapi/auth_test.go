package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddleware_NoKey(t *testing.T) {
	// When no key is configured every request passes through.
	mw := authMiddleware("")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, path := range []string{"/api/v1/health", "/api/v1/workspaces", "/api/v1/whatever"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("path %s: want 200, got %d", path, w.Code)
		}
	}
}

func TestAuthMiddleware_HealthExempt(t *testing.T) {
	// Health and ready endpoints never need a key even when auth is on.
	mw := authMiddleware("secret-key")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, path := range []string{"/api/v1/health", "/api/v1/ready"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("path %s: want 200 without auth, got %d", path, w.Code)
		}
	}
}

func TestAuthMiddleware_MissingToken(t *testing.T) {
	mw := authMiddleware("correct-key")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
	if w.Header().Get("WWW-Authenticate") == "" {
		t.Error("want WWW-Authenticate header in 401")
	}
}

func TestAuthMiddleware_WrongToken(t *testing.T) {
	mw := authMiddleware("correct-key")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_CorrectToken(t *testing.T) {
	mw := authMiddleware("correct-key")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
	req.Header.Set("Authorization", "Bearer correct-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_WrongScheme(t *testing.T) {
	mw := authMiddleware("correct-key")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
	req.Header.Set("Authorization", "Basic correct-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for Basic scheme, got %d", w.Code)
	}
}
