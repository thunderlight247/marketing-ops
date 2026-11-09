package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"marketingops/api/internal/store"
)

// newsletterStore is the subset of the Postgres store the newsletter list uses.
type newsletterStore interface {
	AddNewsletterSubscriber(context.Context, string, string, string, string) (bool, error)
	ListNewsletterSubscribers(context.Context, string) ([]store.NewsletterSubscriber, error)
	DeleteNewsletterSubscriber(context.Context, string, string) error
}

func newsletterSubscriberJSON(n store.NewsletterSubscriber) map[string]any {
	return map[string]any{
		"email":     n.Email,
		"name":      n.Name,
		"source":    n.Source,
		"createdAt": n.CreatedAt,
	}
}

// validSubscriberEmail accepts a plain address only, and stores it lowercased.
func validSubscriberEmail(raw string) (string, bool) {
	addr, err := mail.ParseAddress(strings.TrimSpace(raw))
	if err != nil || addr.Address != strings.TrimSpace(raw) {
		return "", false
	}
	return strings.ToLower(addr.Address), true
}

// /api/v1/workspaces/{id}/newsletter/subscribers
// GET lists subscribers, POST adds one (name optional), DELETE removes one
// by ?email=. Operator-only, like the other workspace routes.
func newsletterSubscribersHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		s, ok := ws.(newsletterStore)
		if !ok {
			writeJSONError(w, http.StatusNotImplemented, "newsletter list unavailable")
			return
		}
		switch r.Method {
		case http.MethodGet:
			subs, err := s.ListNewsletterSubscribers(r.Context(), workspaceID)
			if err != nil {
				writeJSONError(w, 500, "could not list subscribers")
				return
			}
			out := make([]map[string]any, 0, len(subs))
			for _, n := range subs {
				out = append(out, newsletterSubscriberJSON(n))
			}
			writeJSON(w, 200, map[string]any{"subscribers": out})
		case http.MethodPost:
			body, problems := readJSONBody(w, r, 4096)
			if problems != nil {
				return
			}
			var input struct {
				Email string `json:"email"`
				Name  string `json:"name"`
			}
			if json.Unmarshal(body, &input) != nil {
				writeJSONError(w, 400, "email is required")
				return
			}
			email, ok := validSubscriberEmail(input.Email)
			if !ok {
				writeJSONError(w, 400, "email must be a valid address")
				return
			}
			name := strings.TrimSpace(input.Name)
			if len([]rune(name)) > 120 {
				writeJSONError(w, 400, "name must be at most 120 characters")
				return
			}
			added, err := s.AddNewsletterSubscriber(r.Context(), workspaceID, email, name, "manual")
			if err != nil {
				writeJSONError(w, 500, "could not add subscriber")
				return
			}
			if !added {
				writeJSONError(w, 409, "this address is already subscribed or has unsubscribed")
				return
			}
			writeJSON(w, 201, map[string]any{"subscriber": map[string]any{"email": email, "name": name, "source": "manual"}})
		case http.MethodDelete:
			email, ok := validSubscriberEmail(r.URL.Query().Get("email"))
			if !ok {
				writeJSONError(w, 400, "email must be a valid address")
				return
			}
			if err := s.DeleteNewsletterSubscriber(r.Context(), workspaceID, email); errors.Is(err, store.ErrNotFound) {
				writeJSONError(w, 404, "no such subscriber")
				return
			} else if err != nil {
				writeJSONError(w, 500, "could not remove subscriber")
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}
