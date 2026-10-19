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

type contentItemJSON struct {
	ID               string     `json:"id"`
	WorkspaceID      string     `json:"workspaceId"`
	Kind             string     `json:"kind"`
	Status           string     `json:"status"`
	Title            string     `json:"title"`
	Slug             *string    `json:"slug"`
	PlannedPublishAt *time.Time `json:"plannedPublishAt"`
	CreatedAt        string     `json:"createdAt"`
	UpdatedAt        string     `json:"updatedAt"`
}

func toContentItemJSON(c store.ContentItem) contentItemJSON {
	return contentItemJSON{
		ID:               c.ID,
		WorkspaceID:      c.WorkspaceID,
		Kind:             c.Kind,
		Status:           c.Status,
		Title:            c.Title,
		Slug:             c.Slug,
		PlannedPublishAt: c.PlannedPublishAt,
		CreatedAt:        c.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:        c.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

const (
	maxContentPayloadBytes = 16 * 1024
	maxContentTitleRunes   = 200
)

var validContentKinds = map[string]bool{
	"blog_article": true,
	"social_post":  true,
	"snippet":      true,
}

// GET /api/v1/workspaces/{id}/content returns all content items for one workspace.
func listWorkspaceContentHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		workspaceID := r.PathValue("id")
		if workspaceID == "" {
			writeJSONError(w, http.StatusBadRequest, "workspace id is required")
			return
		}
		if _, err := ws.GetWorkspaceByID(r.Context(), workspaceID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "no such workspace")
			} else {
				writeJSONError(w, http.StatusInternalServerError, "workspace lookup failed")
			}
			return
		}

		items, err := ws.ListContentItems(r.Context(), workspaceID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "content listing failed")
			return
		}
		out := make([]contentItemJSON, 0, len(items))
		for _, item := range items {
			out = append(out, toContentItemJSON(item))
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	}
}

// POST /api/v1/workspaces/{id}/content creates a draft content item.
func createWorkspaceContentHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		workspaceID := r.PathValue("id")
		if workspaceID == "" {
			writeJSONError(w, http.StatusBadRequest, "workspace id is required")
			return
		}
		if _, err := ws.GetWorkspaceByID(r.Context(), workspaceID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "no such workspace")
			} else {
				writeJSONError(w, http.StatusInternalServerError, "workspace lookup failed")
			}
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, maxContentPayloadBytes+1))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "could not read the request body")
			return
		}
		if len(body) > maxContentPayloadBytes {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request body is too large")
			return
		}

		var req struct {
			Kind  *string `json:"kind"`
			Title *string `json:"title"`
			Slug  *string `json:"slug"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "request body is not valid JSON")
			return
		}

		var problems []string
		kind := trim(req.Kind)
		if kind == "" {
			problems = append(problems, "kind is required")
		} else if !validContentKinds[kind] {
			problems = append(problems, "kind must be blog_article, social_post, or snippet")
		}

		title := trim(req.Title)
		if title == "" {
			problems = append(problems, "title is required")
		} else if runeCount(title) > maxContentTitleRunes {
			problems = append(problems, "title is too long (max 200 characters)")
		}

		var slug *string
		if req.Slug != nil {
			trimmedSlug := strings.ToLower(strings.TrimSpace(*req.Slug))
			if trimmedSlug != "" {
				if !slugPattern.MatchString(trimmedSlug) {
					problems = append(problems, "slug must be lowercase letters, digits, or hyphens")
				} else {
					slug = &trimmedSlug
				}
			}
		}

		if len(problems) > 0 {
			writeJSONError(w, http.StatusBadRequest, strings.Join(problems, "; "))
			return
		}

		created, err := ws.InsertContentItem(r.Context(), store.ContentItem{
			WorkspaceID: workspaceID,
			Kind:        kind,
			Title:       title,
			Slug:        slug,
		})
		switch {
		case err == nil:
			audit(r.Context(), ws, store.AuditEvent{
				WorkspaceID: &workspaceID,
				Action:      "content.created",
				Details: map[string]any{
					"id":    created.ID,
					"kind":  created.Kind,
					"title": created.Title,
				},
			})
			writeJSON(w, http.StatusCreated, toContentItemJSON(created))
		case errors.Is(err, store.ErrSlugTaken):
			writeJSONError(w, http.StatusConflict, "a content item with that slug already exists in this workspace")
		default:
			writeJSONError(w, http.StatusInternalServerError, "content creation failed")
		}
	}
}
