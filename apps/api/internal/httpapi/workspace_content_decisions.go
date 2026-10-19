package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"marketingops/api/internal/store"
)

const maxDecisionPayloadBytes = 32 * 1024

// OperatorProvider resolves the acting operator for unauthenticated (D16)
// single-operator mode. The real store finds or seeds the singleton row; the
// fake records calls. When authentication lands (Milestone 13), this becomes
// the session identity instead of a lookup.
type OperatorProvider interface {
	EnsureOperator(ctx context.Context) (operatorID string, err error)
}

// decisionRecord is the JSON face of a recorded decision.
type decisionRecord struct {
	ID               string  `json:"id"`
	ContentItemID    string  `json:"contentItemId"`
	ContentVersionID string  `json:"contentVersionId"`
	Decision         string  `json:"decision"`
	Note             *string `json:"note"`
	DecidedAt        string  `json:"decidedAt"`
}

func toDecisionJSON(a store.Approval) decisionRecord {
	return decisionRecord{
		ID:               a.ID,
		ContentItemID:    a.ContentItemID,
		ContentVersionID: a.ContentVersionID,
		Decision:         a.Decision,
		Note:             a.Note,
		DecidedAt:        a.DecidedAt.UTC().Format(time.RFC3339),
	}
}

// POST /api/v1/workspaces/{id}/content/{itemId}/decision
//
// Body: {"decision": "approve"|"reject", "note"?: string}. The decided
// version is always the item's newest, so a superseded snapshot can never
// regain approval. One audit row records every decision.
func decideWorkspaceContentHandler(ws WorkspaceStore, operators OperatorProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		if operators == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		workspaceID, itemID := r.PathValue("id"), r.PathValue("itemId")
		if _, err := ws.GetWorkspaceByID(r.Context(), workspaceID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "no such workspace")
			} else {
				writeJSONError(w, http.StatusInternalServerError, "workspace lookup failed")
			}
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, maxDecisionPayloadBytes+1))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "could not read the request body")
			return
		}
		if len(body) > maxDecisionPayloadBytes {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request body is too large")
			return
		}
		var req struct {
			Decision *string `json:"decision"`
			Note     *string `json:"note"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "request body is not valid JSON")
			return
		}

		decision := strings.ToLower(trim(req.Decision))
		if decision != "approve" && decision != "reject" {
			writeJSONError(w, http.StatusBadRequest, "decision must be approve or reject")
			return
		}
		note := trim(req.Note)
		if runeCount(note) > 2000 {
			writeJSONError(w, http.StatusBadRequest, "note is too long (max 2000 characters)")
			return
		}

		operatorID, err := operators.EnsureOperator(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "operator lookup failed")
			return
		}

		item, approval, err := ws.DecideApproval(r.Context(), workspaceID, itemID, store.ApprovalDecisionInput{
			Decision:   decision,
			OperatorID: operatorID,
			Note:       emptyToNil(note),
		})
		switch {
		case err == nil:
			audit(r.Context(), ws, store.AuditEvent{
				WorkspaceID: &workspaceID,
				Action:      "content.approval_" + decision + "_recorded",
				EntityType:  "content_item",
				EntityID:    &item.ID,
				Details: map[string]any{
					"id":       item.ID,
					"decision": approval.Decision,
					"status":   item.Status,
				},
			})
			writeJSON(w, http.StatusOK, struct {
				contentItemJSON
				Decision decisionRecord `json:"decision"`
			}{toContentItemJSON(item), toDecisionJSON(approval)})
		case errors.Is(err, store.ErrNotFound):
			writeJSONError(w, http.StatusNotFound, "no such content item")
		case errors.Is(err, store.ErrVersionNotLatest):
			writeJSONError(w, http.StatusConflict, "there is no draft content to decide on yet")
		case errors.Is(err, store.ErrAlreadyDecided):
			writeJSONError(w, http.StatusConflict, "the newest version already has a decision; edit the draft to submit it again")
		case errors.Is(err, store.ErrInvalidStatus):
			writeJSONError(w, http.StatusConflict, "this item's status does not allow an approval decision here")
		default:
			writeJSONError(w, http.StatusInternalServerError, "approval decision failed")
		}
	}
}

// POST /api/v1/workspaces/{id}/content/{itemId}/status
//
// Body: {"status": "review"|"draft"}. Only the two manually-driven moves are
// exposed; approve/reject decisions go through the decision endpoint and the
// scheduler owns everything queued/publishing and beyond.
func transitionWorkspaceContentHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		workspaceID, itemID := r.PathValue("id"), r.PathValue("itemId")
		if _, err := ws.GetWorkspaceByID(r.Context(), workspaceID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "no such workspace")
			} else {
				writeJSONError(w, http.StatusInternalServerError, "workspace lookup failed")
			}
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 4*1024+1))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "could not read the request body")
			return
		}
		var req struct {
			Status *string `json:"status"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "request body is not valid JSON")
			return
		}
		to := strings.ToLower(trim(req.Status))
		if to != "review" && to != "draft" {
			writeJSONError(w, http.StatusBadRequest, "status must be review or draft")
			return
		}

		item, err := ws.TransitionContentItemStatus(r.Context(), workspaceID, itemID, to)
		switch {
		case err == nil:
			audit(r.Context(), ws, store.AuditEvent{
				WorkspaceID: &workspaceID,
				Action:      "content.status_changed",
				EntityType:  "content_item",
				EntityID:    &item.ID,
				Details: map[string]any{
					"id":     item.ID,
					"status": item.Status,
				},
			})
			writeJSON(w, http.StatusOK, toContentItemJSON(item))
		case errors.Is(err, store.ErrNotFound):
			writeJSONError(w, http.StatusNotFound, "no such content item")
		case errors.Is(err, store.ErrInvalidStatus):
			writeJSONError(w, http.StatusConflict, "that status change is not allowed from the item's current status")
		default:
			writeJSONError(w, http.StatusInternalServerError, "status change failed")
		}
	}
}

// GET /api/v1/workspaces/{id}/content/{itemId}/history returns every version
// of the item plus its decision records, newest first.
func getWorkspaceContentHistoryHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		workspaceID, itemID := r.PathValue("id"), r.PathValue("itemId")
		if _, _, err := ws.GetContentItemDetail(r.Context(), workspaceID, itemID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "no such content item")
			} else {
				writeJSONError(w, http.StatusInternalServerError, "content lookup failed")
			}
			return
		}

		versions, err := ws.ListContentVersionsInWorkspace(r.Context(), workspaceID, itemID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "version listing failed")
			return
		}
		approvals, err := ws.ListApprovalsByItem(r.Context(), workspaceID, itemID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "approval listing failed")
			return
		}

		out := make([]struct {
			contentVersionJSON
			Decision *decisionRecord `json:"decision"`
		}, 0, len(versions))
		decisions := map[string]decisionRecord{}
		for _, a := range approvals {
			decisions[a.ContentVersionID] = toDecisionJSON(a)
		}
		for _, v := range versions {
			var decision *decisionRecord
			if d, ok := decisions[v.ID]; ok {
				decision = &d
			}
			out = append(out, struct {
				contentVersionJSON
				Decision *decisionRecord `json:"decision"`
			}{toContentVersionJSON(v), decision})
		}
		writeJSON(w, http.StatusOK, map[string]any{"versions": out})
	}
}
