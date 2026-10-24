package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"marketingops/api/internal/store"
)

// publishJobJSON is the wire form of one publishing job.
type publishJobJSON struct {
	ID               string  `json:"id"`
	ContentItemID    string  `json:"contentItemId"`
	ContentVersionID string  `json:"contentVersionId"`
	IntegrationID    string  `json:"integrationId"`
	Status           string  `json:"status"`
	ScheduledAt      *string `json:"scheduledAt,omitempty"`
	Attempts         int     `json:"attempts"`
	LastError        *string `json:"lastError,omitempty"`
	NextAttemptAt    *string `json:"nextAttemptAt,omitempty"`
	CreatedAt        string  `json:"createdAt"`
	UpdatedAt        string  `json:"updatedAt"`
}

func toPublishJobJSON(j store.PublishingJob) publishJobJSON {
	iso := func(t *time.Time) *string {
		if t == nil {
			return nil
		}
		out := t.UTC().Format(time.RFC3339)
		return &out
	}
	return publishJobJSON{
		ID:               j.ID,
		ContentItemID:    j.ContentItemID,
		ContentVersionID: j.ContentVersionID,
		IntegrationID:    j.IntegrationID,
		Status:           j.Status,
		ScheduledAt:      iso(j.ScheduledAt),
		Attempts:         j.Attempts,
		LastError:        j.LastError,
		NextAttemptAt:    iso(j.NextAttemptAt),
		CreatedAt:        j.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:        j.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// publishRequest is the POST /publish body.
type publishRequest struct {
	IntegrationID *string `json:"integrationId"`
	Mode          *string `json:"mode"`
	ScheduledAt   *string `json:"scheduledAt,omitempty"`
}

// parsePublishingTime validates an optional UTC ISO 8601 schedule.
func parsePublishingTime(raw *string) (time.Time, bool, []string) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return time.Time{}, false, nil
	}
	when, err := time.Parse(time.RFC3339, strings.TrimSpace(*raw))
	if err != nil {
		return time.Time{}, false, []string{"scheduledAt must be a UTC ISO 8601 timestamp"}
	}
	return when, true, nil
}

// listContentJobsHandler returns the publishing history of one content item.
//
// GET /api/v1/workspaces/{id}/content/{itemId}/jobs
func listContentJobsHandler(ws WorkspaceStore) http.HandlerFunc {
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
		jobs, err := ws.ListPublishingJobs(r.Context(), workspaceID, itemID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "publishing job listing failed")
			return
		}
		out := make([]publishJobJSON, 0, len(jobs))
		for _, j := range jobs {
			out = append(out, toPublishJobJSON(j))
		}
		writeJSON(w, http.StatusOK, map[string]any{"jobs": out})
	}
}

// cancelContentJobHandler stops a queued or failed job; running jobs are owned
// by the worker and cannot be cancelled here.
//
// POST /api/v1/workspaces/{id}/content/{itemId}/jobs/{jobId}/cancel
func cancelContentJobHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		workspaceID, itemID, jobID := r.PathValue("id"), r.PathValue("itemId"), r.PathValue("jobId")
		if _, _, err := ws.GetContentItemDetail(r.Context(), workspaceID, itemID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "no such content item")
			} else {
				writeJSONError(w, http.StatusInternalServerError, "content lookup failed")
			}
			return
		}
		job, err := ws.CancelPublishingJob(r.Context(), workspaceID, jobID)
		switch {
		case err == nil:
			audit(r.Context(), ws, store.AuditEvent{
				WorkspaceID: &workspaceID,
				Action:      "publishing_job_cancelled",
				EntityType:  "content_item",
				EntityID:    &itemID,
				Details:     map[string]any{"jobId": job.ID, "attempts": job.Attempts},
			})
			writeJSON(w, http.StatusOK, map[string]any{"job": toPublishJobJSON(job)})
		case errors.Is(err, store.ErrNotFound):
			writeJSONError(w, http.StatusNotFound,
				"no such job in this workspace, or it cannot be cancelled in its current state")
		default:
			writeJSONError(w, http.StatusInternalServerError, "job cancellation failed")
		}
	}
}

// retryContentJobHandler re-queues one failed job for an immediate attempt.
// The approval gate is re-checked inside the store.
//
// POST /api/v1/workspaces/{id}/content/{itemId}/jobs/{jobId}/retry
func retryContentJobHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		workspaceID, itemID, jobID := r.PathValue("id"), r.PathValue("itemId"), r.PathValue("jobId")
		if _, _, err := ws.GetContentItemDetail(r.Context(), workspaceID, itemID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "no such content item")
			} else {
				writeJSONError(w, http.StatusInternalServerError, "content lookup failed")
			}
			return
		}
		job, err := ws.RetryPublishingJob(r.Context(), workspaceID, jobID, time.Now().UTC())
		switch {
		case err == nil:
			audit(r.Context(), ws, store.AuditEvent{
				WorkspaceID: &workspaceID,
				Action:      "publishing_job_retried",
				EntityType:  "content_item",
				EntityID:    &itemID,
				Details:     map[string]any{"jobId": job.ID, "attempts": job.Attempts},
			})
			writeJSON(w, http.StatusOK, map[string]any{"job": toPublishJobJSON(job)})
		case errors.Is(err, store.ErrNotFound):
			writeJSONError(w, http.StatusNotFound, "no such job in this workspace, or it is not retryable")
		case errors.Is(err, store.ErrNotApproved):
			writeJSONError(w, http.StatusConflict,
				"the newest version must be approved again before this job can be retried")
		case errors.Is(err, store.ErrInvalidStatus):
			writeJSONError(w, http.StatusConflict, "only failed jobs can be retried")
		default:
			writeJSONError(w, http.StatusInternalServerError, "job retry failed")
		}
	}
}
