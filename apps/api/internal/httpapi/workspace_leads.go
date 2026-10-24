// Lead endpoints (Milestone 11, first increment). A lead either has a
// traceable source (manual, form_api, import) or is explicitly marked
// 'unknown'; the source fields are immutable after creation. Contact fields
// are optional (data minimization) and UTM values are stored only when the
// operator actually has them.
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"marketingops/api/internal/store"
)

const maxLeadPayloadBytes = 16 * 1024

var validLeadSources = map[string]bool{
	"manual": true, "form_api": true, "import": true, "unknown": true,
}

var validLeadStatuses = map[string]bool{
	"new": true, "contacted": true, "qualified": true, "won": true, "lost": true,
}

type leadJSON struct {
	ID           string     `json:"id"`
	Workspace    string     `json:"workspaceId"`
	CampaignID   *string    `json:"campaignId"`
	SourceKind   string     `json:"sourceKind"`
	SourceDetail string     `json:"sourceDetail"`
	Status       string     `json:"status"`
	ContactName  *string    `json:"contactName"`
	ContactEmail *string    `json:"contactEmail"`
	ContactPhone *string    `json:"contactPhone"`
	Message      *string    `json:"message"`
	Notes        string     `json:"notes"`
	UtmSource    *string    `json:"utmSource"`
	UtmMedium    *string    `json:"utmMedium"`
	UtmCampaign  *string    `json:"utmCampaign"`
	FollowUpAt   *time.Time `json:"followUpAt"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

func toLeadJSON(l store.Lead) leadJSON {
	return leadJSON{
		ID:           l.ID,
		Workspace:    l.WorkspaceID,
		CampaignID:   l.CampaignID,
		SourceKind:   l.SourceKind,
		SourceDetail: l.SourceDetail,
		Status:       l.Status,
		ContactName:  l.ContactName,
		ContactEmail: l.ContactEmail,
		ContactPhone: l.ContactPhone,
		Message:      l.Message,
		Notes:        l.Notes,
		UtmSource:    l.UtmSource,
		UtmMedium:    l.UtmMedium,
		UtmCampaign:  l.UtmCampaign,
		FollowUpAt:   l.FollowUpAt,
		CreatedAt:    l.CreatedAt.UTC(),
		UpdatedAt:    l.UpdatedAt.UTC(),
	}
}

// GET /api/v1/workspaces/{id}/leads
func listWorkspaceLeadsHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		leads, err := ws.ListLeads(r.Context(), workspaceID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "lead listing failed")
			return
		}
		out := make([]leadJSON, 0, len(leads))
		for _, l := range leads {
			out = append(out, toLeadJSON(l))
		}
		writeJSON(w, http.StatusOK, map[string]any{"leads": out})
	}
}

// POST /api/v1/workspaces/{id}/leads
func createWorkspaceLeadHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		body, problems := readJSONBody(w, r, maxLeadPayloadBytes)
		if problems != nil {
			return
		}
		var req struct {
			CampaignID   *string    `json:"campaignId"`
			SourceKind   *string    `json:"sourceKind"`
			SourceDetail *string    `json:"sourceDetail"`
			ContactName  *string    `json:"contactName"`
			ContactEmail *string    `json:"contactEmail"`
			ContactPhone *string    `json:"contactPhone"`
			Message      *string    `json:"message"`
			Notes        *string    `json:"notes"`
			UtmSource    *string    `json:"utmSource"`
			UtmMedium    *string    `json:"utmMedium"`
			UtmCampaign  *string    `json:"utmCampaign"`
			FollowUpAt   *time.Time `json:"followUpAt"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "request body is not valid JSON")
			return
		}

		sourceKind := strings.TrimSpace(trim(req.SourceKind))
		if sourceKind == "" {
			// Never fabricate an origin: an omitted one is recorded as unknown.
			sourceKind = "unknown"
		}
		if !validLeadSources[sourceKind] {
			problems = append(problems, "sourceKind must be manual, form_api, import, or unknown")
		}
		if len(problems) > 0 {
			writeJSONError(w, http.StatusBadRequest, strings.Join(problems, "; "))
			return
		}

		created, err := ws.InsertLead(r.Context(), store.Lead{
			WorkspaceID:  workspaceID,
			CampaignID:   blankToNil(req.CampaignID),
			SourceKind:   sourceKind,
			SourceDetail: trim(req.SourceDetail),
			ContactName:  blankToNil(req.ContactName),
			ContactEmail: blankToNil(req.ContactEmail),
			ContactPhone: blankToNil(req.ContactPhone),
			Message:      req.Message,
			Notes:        trim(req.Notes),
			UtmSource:    blankToNil(req.UtmSource),
			UtmMedium:    blankToNil(req.UtmMedium),
			UtmCampaign:  blankToNil(req.UtmCampaign),
			FollowUpAt:   req.FollowUpAt,
		})
		switch {
		case err == nil:
			audit(r.Context(), ws, store.AuditEvent{
				WorkspaceID: &workspaceID,
				Action:      "lead.created",
				EntityType:  "lead",
				EntityID:    &created.ID,
				Details:     map[string]any{"sourceKind": created.SourceKind},
			})
			writeJSON(w, http.StatusCreated, toLeadJSON(created))
		case errors.Is(err, store.ErrInvalidStatus):
			writeJSONError(w, http.StatusBadRequest, "unknown sourceKind or status")
		case errors.Is(err, store.ErrNotFound):
			writeJSONError(w, http.StatusBadRequest, "campaignId does not belong to this workspace")
		default:
			writeJSONError(w, http.StatusInternalServerError, "lead creation failed")
		}
	}
}

// PATCH /api/v1/workspaces/{id}/leads/{leadId} edits follow-up fields. The
// source fields are never editable; empty strings mean "keep unchanged" for
// optional values and the explicit flags clear campaign/follow-up.
func patchWorkspaceLeadHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		leadID := r.PathValue("leadId")
		body, problems := readJSONBody(w, r, maxLeadPayloadBytes)
		if problems != nil {
			return
		}
		var req struct {
			CampaignID    *string    `json:"campaignId"`
			ClearCampaign bool       `json:"clearCampaign"`
			Status        *string    `json:"status"`
			ContactName   *string    `json:"contactName"`
			ContactEmail  *string    `json:"contactEmail"`
			ContactPhone  *string    `json:"contactPhone"`
			Message       *string    `json:"message"`
			Notes         *string    `json:"notes"`
			UtmSource     *string    `json:"utmSource"`
			UtmMedium     *string    `json:"utmMedium"`
			UtmCampaign   *string    `json:"utmCampaign"`
			FollowUpAt    *time.Time `json:"followUpAt"`
			ClearFollowUp bool       `json:"clearFollowUp"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "request body is not valid JSON")
			return
		}

		status := trim(req.Status)
		if status != "" && !validLeadStatuses[status] {
			problems = append(problems, "status must be new, contacted, qualified, won, or lost")
		}
		if len(problems) > 0 {
			writeJSONError(w, http.StatusBadRequest, strings.Join(problems, "; "))
			return
		}

		in := store.LeadUpdate{
			ClearCampaign: req.ClearCampaign,
			Status:        status,
			ContactName:   req.ContactName,
			ContactEmail:  req.ContactEmail,
			ContactPhone:  req.ContactPhone,
			Message:       req.Message,
			UtmSource:     req.UtmSource,
			UtmMedium:     req.UtmMedium,
			UtmCampaign:   req.UtmCampaign,
			FollowUpAt:    req.FollowUpAt,
			ClearFollow:   req.ClearFollowUp,
			CampaignID:    req.CampaignID,
		}
		if req.Notes != nil {
			in.Notes, in.KeepNotes = *req.Notes, false
		} else {
			in.KeepNotes = true
		}

		updated, err := ws.UpdateLead(r.Context(), workspaceID, leadID, in)
		switch {
		case err == nil:
			audit(r.Context(), ws, store.AuditEvent{
				WorkspaceID: &workspaceID,
				Action:      "lead.updated",
				EntityType:  "lead",
				EntityID:    &updated.ID,
				Details:     map[string]any{},
			})
			writeJSON(w, http.StatusOK, toLeadJSON(updated))
		case errors.Is(err, store.ErrNotFound):
			writeJSONError(w, http.StatusNotFound, "no such lead (or campaignId is outside this workspace)")
		case errors.Is(err, store.ErrInvalidStatus):
			writeJSONError(w, http.StatusBadRequest, "unknown status")
		default:
			writeJSONError(w, http.StatusInternalServerError, "saving the lead failed")
		}
	}
}

// blankToNil drops empty strings so "sent but blank" never writes an empty
// UTM/contact value: an omitted value stays SQL NULL.
func blankToNil(s *string) *string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	return s
}
