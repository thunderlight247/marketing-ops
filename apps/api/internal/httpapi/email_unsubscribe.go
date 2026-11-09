// Public unsubscribe endpoint for outbound lead email. A link carries a signed
// token that names the workspace and address. The signature uses the
// credential key, so nobody can forge a link to suppress an arbitrary address.
package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"log/slog"
	"net/http"
	"strings"
)

// UnsubscribeStore records a suppression for one workspace and address.
type UnsubscribeStore interface {
	SuppressEmail(ctx context.Context, workspaceID, email, reason string) error
}

// unsubscribeStore returns ws as an UnsubscribeStore when it supports
// suppression. The store is the Postgres-backed one in production; any other
// implementation gets a nil result and the endpoint answers 501.
func unsubscribeStore(ws any) UnsubscribeStore {
	if st, ok := ws.(UnsubscribeStore); ok {
		return st
	}
	return nil
}

// unsubscribeToken returns workspaceID|email signed with key, base64url encoded.
func unsubscribeToken(key []byte, workspaceID, email string) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(workspaceID + "|" + strings.ToLower(strings.TrimSpace(email))))
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// parseUnsubscribeToken verifies the signature and returns the workspace and address.
func parseUnsubscribeToken(key []byte, token string) (workspaceID, email string, ok bool) {
	payload, sig, found := strings.Cut(token, ".")
	if !found || len(key) == 0 {
		return "", "", false
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(want)) {
		return "", "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return "", "", false
	}
	ws, addr, found := strings.Cut(string(raw), "|")
	if !found || ws == "" || addr == "" {
		return "", "", false
	}
	return ws, addr, true
}

// unsubscribeURL builds the link for a lead email footer. It returns "" when
// no public API base is configured, and the footer is then omitted.
func unsubscribeURL(apiBase string, key []byte, workspaceID, email string) string {
	apiBase = strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if apiBase == "" || len(key) == 0 {
		return ""
	}
	return apiBase + "/api/v1/email/unsubscribe?token=" + unsubscribeToken(key, workspaceID, email)
}

// GET /api/v1/email/unsubscribe?token=...
// Public, so it takes no session. A valid token suppresses the address for
// its workspace; an invalid one gets 400 and changes nothing.
func unsubscribeHandler(st UnsubscribeStore, key []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if st == nil || len(key) == 0 {
			writeJSONError(w, http.StatusNotImplemented, "unsubscribe is not configured")
			return
		}
		workspaceID, email, ok := parseUnsubscribeToken(key, r.URL.Query().Get("token"))
		if !ok {
			writeJSONError(w, http.StatusBadRequest, "this unsubscribe link is invalid")
			return
		}
		if err := st.SuppressEmail(r.Context(), workspaceID, email, "unsubscribe"); err != nil {
			slog.Default().Error("unsubscribe failed", "workspace_id", workspaceID, "err", err)
			writeJSONError(w, http.StatusInternalServerError, "could not record the unsubscribe")
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("You have been unsubscribed and will not receive more emails from this sender.\n"))
	}
}
