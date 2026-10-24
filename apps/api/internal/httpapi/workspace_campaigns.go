// Campaign endpoints (Milestone 11, first increment). Thin handlers over the
// workspace-scoped store; tests in workspace_campaigns_test.go use fakes so
// every new store method must also exist in those fakes.
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

const maxCampaignPayloadBytes = 8 * 1024

var validCampaignStatuses = map[string]bool{
	"planned": true, "active": true, "paused": true, "completed": true,
}

type campaignJSON struct {
	ID          string `json:"id"`
	Workspace   string `json:"workspaceId"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	UtmCampaign string `json:"utmCampaign"`
	Notes       string `json:"notes"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

func toCampaignJSON(c store.Campaign) campaignJSON {
	return campaignJSON{
		ID:          c.ID,
		Workspace:   c.WorkspaceID,
		Name:        c.Name,
		Status:      c.Status,
		UtmCampaign: c.UtmCampaign,
		Notes:       c.Notes, CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: c.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// GET /api/v1/workspaces/{id}/campaigns
func listWorkspaceCampaignsHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		campaigns, err := ws.ListCampaigns(r.Context(), ctx)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "campaign listing failed")
			return
		}
		out := make([]campaignJSON, 0, len(campaigns))
		for _, c := range campaigns {
			out = append(out, toCampaignJSON(c))
		}
		writeJSON(w, http.StatusOK, map[string]any{"campaigns": out})
	}
}

// POST /api/v1/workspaces/{id}/campaigns
func createWorkspaceCampaignHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		body, problems := readJSONBody(w, r, maxCampaignPayloadBytes)
		if problems != nil {
			return
		}
		var req struct {
			Name        *string `json:"name"`
			Status      *string `json:"status"`
			UtmCampaign *string `json:"utmCampaign"`
			Notes       *string `json:"notes"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "request body is not valid JSON")
			return
		}

		name := trim(req.Name)
		if name == "" {
			problems = append(problems, "name is required")
		} else if runeCount(name) > 120 {
			problems = append(problems, "name is too long (max 120 characters)")
		}
		status := trim(req.Status)
		if status != "" && !validCampaignStatuses[status] {
			problems = append(problems, "status must be planned, active, paused, or completed")
		}
		if len(problems) > 0 {
			writeJSONError(w, http.StatusBadRequest, strings.Join(problems, "; "))
			return
		}

		created, err := ws.InsertCampaign(r.Context(), store.Campaign{
			WorkspaceID: workspaceID,
			Name:        name,
			Status:      status,
			UtmCampaign: trim(req.UtmCampaign),
			Notes:       trim(req.Notes),
		})
		switch {
		case err == nil:
			audit(r.Context(), ws, store.AuditEvent{
				WorkspaceID: &workspaceID,
				Action:      "campaign.created",
				EntityType:  "campaign",
				EntityID:    &created.ID,
				Details:     map[string]any{"name": created.Name},
			})
			writeJSON(w, http.StatusCreated, toCampaignJSON(created))
		case errors.Is(err, store.ErrSlugTaken):
			writeJSONError(w, http.StatusConflict, "a campaign with that name already exists in this workspace")
		case errors.Is(err, store.ErrNameRequired):
			writeJSONError(w, http.StatusBadRequest, "name is required")
		case errors.Is(err, store.ErrInvalidStatus):
			writeJSONError(w, http.StatusBadRequest, "unknown campaign status")
		default:
			writeJSONError(w, http.StatusInternalServerError, "campaign creation failed")
		}
	}
}

// GET /api/v1/workspaces/{id}/campaigns/{campaignId}
func getWorkspaceCampaignHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		c, err := ws.GetCampaignInWorkspace(r.Context(), workspaceID, r.PathValue("campaignId"))
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "no such campaign")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "campaign lookup failed")
			return
		}
		writeJSON(w, http.StatusOK, toCampaignJSON(c))
	}
}

// PATCH /api/v1/workspaces/{id}/campaigns/{campaignId} edits name, status,
// utmCampaign, and notes. Omitted fields are unchanged.
func patchWorkspaceCampaignHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		campaignID := r.PathValue("campaignId")
		current, err := ws.GetCampaignInWorkspace(r.Context(), workspaceID, campaignID)
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "no such campaign")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "campaign lookup failed")
			return
		}
		body, problems := readJSONBody(w, r, maxCampaignPayloadBytes)
		if problems != nil {
			return
		}
		var req struct {
			Name        *string `json:"name"`
			Status      *string `json:"status"`
			UtmCampaign *string `json:"utmCampaign"`
			Notes       *string `json:"notes"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "request body is not valid JSON")
			return
		}

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
			if !validCampaignStatuses[status] {
				problems = append(problems, "status must be planned, active, paused, or completed")
			}
		}
		utm := current.UtmCampaign
		if req.UtmCampaign != nil {
			utm = strings.TrimSpace(*req.UtmCampaign)
		}
		notes := current.Notes
		if req.Notes != nil {
			notes = *req.Notes
		}
		if len(problems) > 0 {
			writeJSONError(w, http.StatusBadRequest, strings.Join(problems, "; "))
			return
		}

		updated, err := ws.UpdateCampaign(r.Context(), store.Campaign{
			ID:          current.ID,
			WorkspaceID: workspaceID,
			Name:        name,
			Status:      status,
			UtmCampaign: utm,
			Notes:       notes,
		})
		switch {
		case err == nil:
			audit(r.Context(), ws, store.AuditEvent{
				WorkspaceID: &workspaceID,
				Action:      "campaign.updated",
				EntityType:  "campaign",
				EntityID:    &current.ID,
				Details:     map[string]any{},
			})
			writeJSON(w, http.StatusOK, toCampaignJSON(updated))
		case errors.Is(err, store.ErrNameRequired):
			writeJSONError(w, http.StatusBadRequest, "name cannot be empty")
		case errors.Is(err, store.ErrInvalidStatus):
			writeJSONError(w, http.StatusBadRequest, "unknown campaign status")
		default:
			writeJSONError(w, http.StatusInternalServerError, "saving the campaign failed")
		}
	}
}

// ---- shared small helpers ---------------------------------------------------

// workspaceScope validates the workspace and returns its id; on failure the
// response is already written.
func workspaceScope(w http.ResponseWriter, r *http.Request, ws WorkspaceStore) (string, bool) {
	if ws == nil {
		writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
		return "", false
	}
	workspaceID := r.PathValue("id")
	if _, err := ws.GetWorkspaceByID(r.Context(), workspaceID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "no such workspace")
		} else {
			writeJSONError(w, http.StatusInternalServerError, "workspace lookup failed")
		}
		return "", false
	}
	return workspaceID, true
}

// readJSONBody reads and size-limits the body; on failure the response is
// already written and problems is non-nil.
func readJSONBody(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, []string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "could not read request body")
		return nil, []string{"read failed"}
	}
	if len(body) > int(limit) {
		writeJSONError(w, http.StatusRequestEntityTooLarge, "request body is too large")
		return nil, []string{"too large"}
	}
	return body, nil
}
