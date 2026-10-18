package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"marketingops/api/internal/store"
)

// WorkspaceStore is what the workspace handlers need. *store.Store implements
// it; tests use fakes.
type WorkspaceStore interface {
	ListWorkspaces(ctx context.Context) ([]store.Workspace, error)
	InsertWorkspace(ctx context.Context, w store.Workspace) (store.Workspace, error)
}

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}[a-z0-9]$|^[a-z0-9]$`)

const maxWorkspacePayloadBytes = 8 * 1024

// workspaceJSON is the wire form of a workspace.
type workspaceJSON struct {
	ID         string  `json:"id"`
	Slug       string  `json:"slug"`
	Name       string  `json:"name"`
	WebsiteURL *string `json:"websiteUrl"`
	Status     string  `json:"status"`
	CreatedAt  string  `json:"createdAt"`
	UpdatedAt  string  `json:"updatedAt"`
}

func toWorkspaceJSON(w store.Workspace) workspaceJSON {
	return workspaceJSON{
		ID:         w.ID,
		Slug:       w.Slug,
		Name:       w.Name,
		WebsiteURL: w.WebsiteURL,
		Status:     w.Status,
		CreatedAt:  w.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:  w.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// listWorkspacesHandler returns every active workspace.
func listWorkspacesHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		workspaces, err := ws.ListWorkspaces(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "workspace listing failed")
			return
		}
		out := make([]workspaceJSON, 0, len(workspaces))
		for _, ws := range workspaces {
			out = append(out, toWorkspaceJSON(ws))
		}
		writeJSON(w, http.StatusOK, map[string]any{"workspaces": out})
	}
}

// createWorkspaceHandler validates and creates a workspace.
func createWorkspaceHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxWorkspacePayloadBytes+1))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "could not read the request body")
			return
		}
		if len(body) > maxWorkspacePayloadBytes {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request body is too large")
			return
		}

		var req struct {
			Slug       *string `json:"slug"`
			Name       *string `json:"name"`
			WebsiteURL *string `json:"websiteUrl"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "request body is not valid JSON")
			return
		}

		problems := []string{}
		websiteURL := ""
		slug := trim(req.Slug)
		name := trim(req.Name)
		if req.WebsiteURL != nil {
			websiteURL = strings.TrimRight(strings.TrimSpace(*req.WebsiteURL), "/")
		}
		if slug == "" {
			problems = append(problems, "slug is required")
		} else if !slugPattern.MatchString(slug) {
			problems = append(problems, "slug must be lowercase letters, digits, or hyphens")
		}
		if name == "" {
			problems = append(problems, "name is required")
		} else if runeCount(name) > 120 {
			problems = append(problems, "name is too long (max 120 characters)")
		}
		if len(problems) > 0 {
			writeJSONError(w, http.StatusBadRequest, strings.Join(problems, "; "))
			return
		}

		created, err := ws.InsertWorkspace(r.Context(), store.Workspace{
			Slug: slug, Name: name, WebsiteURL: emptyToNil(websiteURL),
		})
		switch {
		case err == nil:
			writeJSON(w, http.StatusCreated, toWorkspaceJSON(created))
		case errors.Is(err, store.ErrSlugTaken):
			writeJSONError(w, http.StatusConflict, "a workspace with that slug already exists")
		case errors.Is(err, store.ErrNotFound):
			// InsertWorkspace maps ON CONFLICT to a sentinel already; unreachable
			// here, but treat it like a conflict rather than crashing.
			writeJSONError(w, http.StatusConflict, "a workspace with that slug already exists")
		default:
			writeJSONError(w, http.StatusInternalServerError, "workspace creation failed")
		}
	}
}

func trim(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

func emptyToNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func runeCount(s string) int { return len([]rune(s)) }

const workspaceUnavailable = "workspaces are not available (no database configured)"
