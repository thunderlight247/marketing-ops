package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"marketingops/api/internal/store"
)

// brandJSON is the wire form of a brand profile.
type brandJSON struct {
	BusinessDescription *string        `json:"businessDescription"`
	Audience            *string        `json:"audience"`
	Services            *string        `json:"services"`
	Tone                *string        `json:"tone"`
	Rules               map[string]any `json:"rules"`
	ProhibitedClaims    []string       `json:"prohibitedClaims"`
	CallsToAction       []string       `json:"callsToAction"`
	UpdatedAt           string         `json:"updatedAt"` // empty in request bodies
}

func toBrandJSON(b store.BrandProfile) brandJSON {
	return brandJSON{
		BusinessDescription: b.BusinessDescription,
		Audience:            b.Audience,
		Services:            b.Services,
		Tone:                b.Tone,
		Rules:               b.Rules,
		ProhibitedClaims:    b.ProhibitedClaims,
		CallsToAction:       b.CallsToAction,
		UpdatedAt:           b.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

const maxBrandPayloadBytes = 32 * 1024

const (
	maxBrandTextField = 20_000
	maxBrandListItems = 100
	maxBrandListItem  = 500
)

// GET /api/v1/workspaces/{id} returns the workspace and, if present, its brand
// profile. The brand is JSON null when nothing has been saved yet.
func getWorkspaceDetailHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		id := r.PathValue("id")
		if id == "" {
			writeJSONError(w, http.StatusBadRequest, "workspace id is required")
			return
		}
		workspace, err := ws.GetWorkspaceByID(r.Context(), id)
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "no such workspace")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "workspace lookup failed")
			return
		}

		var brand any // JSON null by default
		profile, err := ws.GetBrandProfile(r.Context(), workspace.ID)
		switch {
		case err == nil:
			brand = toBrandJSON(profile)
		case errors.Is(err, store.ErrNotFound):
			// Workspace exists, has no brand profile yet: leave brand as null.
		default:
			writeJSONError(w, http.StatusInternalServerError, "brand lookup failed")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"workspace": toWorkspaceJSON(workspace),
			"brand":     brand,
		})
	}
}

// PUT /api/v1/workspaces/{id}/brand creates or replaces the brand profile.
// Ordinary text fields are trimmed; empty strings are stored as JSON null.
func putWorkspaceBrandHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		workspaceID := r.PathValue("id")
		if _, err := ws.GetWorkspaceByID(r.Context(), workspaceID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "no such workspace")
			} else {
				writeJSONError(w, http.StatusInternalServerError, "workspace lookup failed")
			}
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, maxBrandPayloadBytes+1))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "could not read the request body")
			return
		}
		if len(body) > maxBrandPayloadBytes {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request body is too large")
			return
		}
		var req struct {
			BusinessDescription *string        `json:"businessDescription"`
			Audience            *string        `json:"audience"`
			Services            *string        `json:"services"`
			Tone                *string        `json:"tone"`
			Rules               map[string]any `json:"rules"`
			ProhibitedClaims    []string       `json:"prohibitedClaims"`
			CallsToAction       []string       `json:"callsToAction"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "request body is not valid JSON")
			return
		}

		problems := []string{}
		descriptions := map[*string]string{
			req.BusinessDescription: "businessDescription",
			req.Audience:            "audience",
			req.Services:            "services",
			req.Tone:                "tone",
		}
		trimmed := map[*string]*string{}
		for field, label := range descriptions {
			if field == nil {
				continue
			}
			value := strings.TrimSpace(*field)
			if value == "" {
				trimmed[field] = nil
			} else {
				if runeCount(value) > maxBrandTextField {
					problems = append(problems, label+" is too long")
				}
				trimmed[field] = &value
			}
		}
		if rules := req.Rules; rules != nil {
			for key := range rules {
				if key == "" {
					problems = append(problems, "rules keys must not be empty")
				}
			}
		}
		req.ProhibitedClaims = cleanStringList(req.ProhibitedClaims, maxBrandListItems, maxBrandListItem, "prohibitedClaims", &problems)
		req.CallsToAction = cleanStringList(req.CallsToAction, maxBrandListItems, maxBrandListItem, "callsToAction", &problems)
		if len(problems) > 0 {
			writeJSONError(w, http.StatusBadRequest, strings.Join(problems, "; "))
			return
		}

		saved, err := ws.UpsertBrandProfile(r.Context(), store.BrandProfile{
			WorkspaceID:         workspaceID,
			BusinessDescription: trimmed[req.BusinessDescription],
			Audience:            trimmed[req.Audience],
			Services:            trimmed[req.Services],
			Tone:                trimmed[req.Tone],
			Rules:               req.Rules,
			ProhibitedClaims:    req.ProhibitedClaims,
			CallsToAction:       req.CallsToAction,
		})
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "saving the brand profile failed")
			return
		}
		writeJSON(w, http.StatusOK, toBrandJSON(saved))
	}
}

// cleanStringList trims blanks, drops empties, caps the list size, and reports
// per-item length problems.
func cleanStringList(values []string, maxItems, maxItemLen int, label string, problems *[]string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if len([]rune(v)) > maxItemLen {
			*problems = append(*problems, label+" items are too long (max 500 characters)")
			return nil
		}
		out = append(out, v)
	}
	if len(out) > maxItems {
		*problems = append(*problems, label+" has too many items (max 100)")
		return nil
	}
	return out
}
