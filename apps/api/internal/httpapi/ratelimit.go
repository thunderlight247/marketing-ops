// Package httpapi — per-IP rate limiting for expensive endpoints (Milestone 13).
//
// The limiter uses a fixed window counter keyed on the client's IP address.
// It is intentionally simple: no Redis, no external state, no cross-process
// sharing. This is an internal single-operator tool; the goal is to prevent
// accidental runaway AI calls or integration loops, not to defend against a
// distributed attack.
//
// Only expensive endpoints are throttled:
//   - POST .../content/generate  (AI draft generation)
//   - POST .../agents/{agentId}/run  (specialist agent run)
//
// All other endpoints are unaffected.
package httpapi

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// RateLimiter is a simple in-memory fixed-window rate limiter keyed on IP.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rateBucket
	limit   int
	window  time.Duration
}

type rateBucket struct {
	count   int
	resetAt time.Time
}

// NewRateLimiter creates a limiter that allows at most limit requests per
// window per IP address.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*rateBucket),
		limit:   limit,
		window:  window,
	}
}

// Allow returns true if the IP is within its rate limit for the current window.
func (rl *RateLimiter) Allow(ip string) bool {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()
	b, ok := rl.buckets[ip]
	if !ok || now.After(b.resetAt) {
		rl.buckets[ip] = &rateBucket{count: 1, resetAt: now.Add(rl.window)}
		return true
	}
	b.count++
	return b.count <= rl.limit
}

// Middleware returns an http.Handler wrapper that applies this limiter.
// Requests that exceed the limit receive 429 with a Retry-After header.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !rl.Allow(ip) {
			w.Header().Set("Retry-After", "60")
			writeJSONError(w, http.StatusTooManyRequests,
				"rate limit exceeded; please wait before retrying this operation")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP extracts the best-effort IP address from the request. It reads
// X-Forwarded-For only as a fallback: for a local-network internal tool,
// RemoteAddr is usually authoritative.
func clientIP(r *http.Request) string {
	// Prefer RemoteAddr (authoritative for direct connections).
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}
