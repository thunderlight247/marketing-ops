package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !rl.Allow("1.2.3.4") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)
	rl.Allow("1.2.3.4")
	rl.Allow("1.2.3.4")
	if rl.Allow("1.2.3.4") {
		t.Fatal("third request should be blocked")
	}
}

func TestRateLimiter_SeparateIPsAreIndependent(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	if !rl.Allow("1.1.1.1") {
		t.Fatal("first IP first request should be allowed")
	}
	if !rl.Allow("2.2.2.2") {
		t.Fatal("second IP first request should be allowed")
	}
	if rl.Allow("1.1.1.1") {
		t.Fatal("first IP second request should be blocked")
	}
}

func TestRateLimiter_WindowResets(t *testing.T) {
	rl := NewRateLimiter(1, 10*time.Millisecond)
	rl.Allow("1.2.3.4")
	time.Sleep(15 * time.Millisecond)
	if !rl.Allow("1.2.3.4") {
		t.Fatal("after window reset, request should be allowed")
	}
}

func TestRateLimiter_Middleware429(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := rl.Middleware(inner)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/x/content/generate", nil)
	req.RemoteAddr = "10.0.0.1:12345"

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first request: want 200, got %d", w.Code)
	}

	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req)
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: want 429, got %d", w2.Code)
	}
	if w2.Header().Get("Retry-After") == "" {
		t.Error("want Retry-After header in 429 response")
	}
}
