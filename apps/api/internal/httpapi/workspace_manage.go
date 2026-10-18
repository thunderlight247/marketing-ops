package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"marketingops/api/internal/store"
)

const maxPatchPayloadBytes = 8 * 1024

var validWorkspaceStatuses = map[string]bool{"active": true, "archived": true}

// audit adds a best-effort audit row. Audit failures are logged as warnings but
// never change request results, so an audit hiccup cannot mask a successful write.
func audit(ctx context.Context, ws WorkspaceStore, e store.AuditEvent) {
	if err := ws.AddAuditEvent(ctx, e); err != nil {
		slog.Default().Warn("audit event not recorded", "action", e.Action, "error_class", "audit-write-failed")
	}
}

// PATCH /api/v1/workspaces/{id} edits name, websiteUrl, and status. Omitted
// fields are left unchanged; explicit null clears the website. The slug and
// id never change.
func patchWorkspaceHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		id := r.PathValue("id")
		current, err := ws.GetWorkspaceByID(r.Context(), id)
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "no such workspace")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "workspace lookup failed")
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, maxPatchPayloadBytes+1))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "could not read the request body")
			return
		}
		if len(body) > maxPatchPayloadBytes {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request body is too large")
			return
		}

		var req struct {
			Name       *string `json:"name"`
			WebsiteURL *string `json:"websiteUrl"`
			Status     *string `json:"status"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "request body is not valid JSON")
			return
		}

		var problems []string
		name := current.Name
		if req.Name != nil {
			name = strings.TrimSpace(*req.Name)
			if name == "" {
				problems = append(problems, "name cannot be empty")
			} else if runeCount(name) > 120 {
				problems = append(problems, "name is too long (max 120 characters)")
			}
		}
		status := current.Status
		if req.Status != nil {
			status = strings.TrimSpace(*req.Status)
			if !validWorkspaceStatuses[status] {
				problems = append(problems, "status must be active or archived")
			}
		}
		websiteURL := current.WebsiteURL
		if req.WebsiteURL != nil {
			trimmed := strings.TrimRight(strings.TrimSpace(*req.WebsiteURL), "/")
			websiteURL = emptyToNil(trimmed)
		}
		if len(problems) > 0 {
			writeJSONError(w, http.StatusBadRequest, strings.Join(problems, "; "))
			return
		}

		updated, err := ws.UpdateWorkspace(r.Context(), store.Workspace{
			ID: current.ID, Slug: current.Slug, Name: name,
			WebsiteURL: websiteURL, Status: status,
		})
		if errors.Is(err, store.ErrNameRequired) {
			writeJSONError(w, http.StatusBadRequest, "name cannot be empty")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "saving the workspace failed")
			return
		}

		audit(r.Context(), ws, store.AuditEvent{
			WorkspaceID: &current.ID,
			Action:      "workspace.updated",
			EntityType:  "workspace",
			EntityID:    &current.ID,
			Details:     map[string]any{},
		})
		writeJSON(w, http.StatusOK, toWorkspaceJSON(updated))
	}
}

// DELETE /api/v1/workspaces/{id} removes the workspace and, by cascade, all of
// its client data. To make accidents expensive it must arrive with the body
// {"confirmSlug": "<slug-of-this-workspace>"}. The audit row survives the
// cascade with a null workspace_id, which preserves the deletion trail.
func deleteWorkspaceHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		id := r.PathValue("id")
		current, err := ws.GetWorkspaceByID(r.Context(), id)
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "no such workspace")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "workspace lookup failed")
			return
		}

		var req struct {
			ConfirmSlug *string `json:"confirmSlug"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, maxPatchPayloadBytes)).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "request body is not valid JSON")
			return
		}
		if req.ConfirmSlug == nil || *req.ConfirmSlug != current.Slug {
			writeJSONError(w, http.StatusBadRequest,
				"confirmSlug must match this workspace's slug ("+current.Slug+")")
			return
		}

		if err := ws.DeleteWorkspace(r.Context(), current.ID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				// Deleted between the lookup and the DELETE: treat as done.
				w.WriteHeader(http.StatusNoContent)
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "workspace deletion failed")
			return
		}

		audit(r.Context(), ws, store.AuditEvent{
			WorkspaceID: nil, // cascade clears the workspace link; the row survives
			Action:      "workspace.deleted",
			EntityType:  "workspace",
			EntityID:    &current.ID,
			Details:     map[string]any{"slug": current.Slug},
		})
		w.WriteHeader(http.StatusNoContent)
	}
}
