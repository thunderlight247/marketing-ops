// Package httpapi — authentication middleware (Milestone 13).
//
// The platform is a single-operator internal tool. It does not expose a login
// UI or issue session tokens. Instead every request must supply the operator's
// API key in the Authorization header:
//
//	Authorization: Bearer <api-key>
//
// When no key is configured (development without API_KEY set) the middleware
// is a pass-through so local development needs no extra setup. In production
// APP_ENV=production rejects startup if API_KEY is empty (enforced in config).
//
// The key is compared with crypto/subtle.ConstantTimeCompare so the check
// takes the same time whether the prefix, middle, or end of the key differs.
package httpapi

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// authMiddleware returns a middleware that enforces Bearer token auth when
// apiKey is non-empty. Health and ready endpoints are always exempt so load
// balancers and uptime monitors do not need credentials.
func authMiddleware(apiKey string) func(http.Handler) http.Handler {
	// No key configured → pass-through (development mode).
	if apiKey == "" {
		return func(next http.Handler) http.Handler { return next }
	}
	keybytes := []byte(apiKey)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Health and ready probes are always public.
			if r.URL.Path == "/api/v1/health" || r.URL.Path == "/api/v1/ready" {
				next.ServeHTTP(w, r)
				return
			}
			token := extractBearerToken(r)
			if len(token) == 0 || subtle.ConstantTimeCompare([]byte(token), keybytes) != 1 {
				w.Header().Set("WWW-Authenticate", `Bearer realm="marketing-ops"`)
				writeJSONError(w, http.StatusUnauthorized, "authentication required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// extractBearerToken reads the value after "Bearer " from the Authorization
// header. It returns "" if the header is absent or has a different scheme.
func extractBearerToken(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(h[len("Bearer "):])
}
