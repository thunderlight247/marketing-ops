// Email drip sequence API handlers. Sequences are workspace-scoped and
// editable by any authenticated user of that workspace. The steps are
// stored as JSONB; validation is done here before persisting.
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"marketingops/api/internal/store"
)

const (
	maxSequenceNameRunes = 200
	maxSequenceSteps     = 20
	maxStepSubjectRunes  = 200
	maxStepBodyRunes     = 10000
)

// emailSequenceStore is the slice of WorkspaceStore that the sequence
// handlers need. Declared separately so the main fakeWorkspaceStore in
// tests can stub it.
type emailSequenceStore interface {
	ListEmailSequences(ctx context.Context, workspaceID string) ([]store.EmailSequence, error)
	CreateEmailSequence(ctx context.Context, in store.EmailSequence) (store.EmailSequence, error)
	UpdateEmailSequence(ctx context.Context, in store.EmailSequence) (store.EmailSequence, error)
	DeleteEmailSequence(ctx context.Context, workspaceID, id string) error
}

func emailSequenceStoreOf(ws WorkspaceStore) (emailSequenceStore, bool) {
	s, ok := ws.(emailSequenceStore)
	return s, ok
}

// ─── JSON shapes ──────────────────────────────────────────────────────────────

type emailSequenceStepJSON struct {
	DelayDays int    `json:"delayDays"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
}

type emailSequenceJSON struct {
	ID          string                  `json:"id"`
	WorkspaceID string                  `json:"workspaceId"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Steps       []emailSequenceStepJSON `json:"steps"`
	CreatedAt   string                  `json:"createdAt"`
	UpdatedAt   string                  `json:"updatedAt"`
}

func toEmailSequenceJSON(s store.EmailSequence) emailSequenceJSON {
	steps := make([]emailSequenceStepJSON, len(s.Steps))
	for i, st := range s.Steps {
		steps[i] = emailSequenceStepJSON{
			DelayDays: st.DelayDays,
			Subject:   st.Subject,
			Body:      st.Body,
		}
	}
	return emailSequenceJSON{
		ID:          s.ID,
		WorkspaceID: s.WorkspaceID,
		Name:        s.Name,
		Description: s.Description,
		Steps:       steps,
		CreatedAt:   s.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   s.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// ─── Validation ───────────────────────────────────────────────────────────────

type sequenceInput struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Steps       []emailSequenceStepJSON `json:"steps"`
}

func validateSequenceInput(in sequenceInput) []string {
	var problems []string
	name := strings.TrimSpace(in.Name)
	if name == "" {
		problems = append(problems, "name is required")
	} else if len([]rune(name)) > maxSequenceNameRunes {
		problems = append(problems, "name must be 200 characters or fewer")
	}
	if len(in.Steps) > maxSequenceSteps {
		problems = append(problems, "a sequence may have at most 20 steps")
	}
	for i, st := range in.Steps {
		if st.DelayDays < 0 {
			problems = append(problems, "step delay must be 0 or more days")
		}
		if len([]rune(st.Subject)) > maxStepSubjectRunes {
			problems = append(problems, "step subject must be 200 characters or fewer")
		}
		if len([]rune(st.Body)) > maxStepBodyRunes {
			problems = append(problems, "step body must be 10 000 characters or fewer")
		}
		_ = i
	}
	return problems
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

// GET /api/v1/workspaces/{id}/email-sequences
func listEmailSequencesHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seqStore, ok := emailSequenceStoreOf(ws)
		if !ok {
			http.Error(w, `{"error":"email sequences not supported"}`, http.StatusNotImplemented)
			return
		}
		workspaceID := r.PathValue("id")
		seqs, err := seqStore.ListEmailSequences(r.Context(), workspaceID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		out := make([]emailSequenceJSON, len(seqs))
		for i, s := range seqs {
			out[i] = toEmailSequenceJSON(s)
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// POST /api/v1/workspaces/{id}/email-sequences
func createEmailSequenceHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seqStore, ok := emailSequenceStoreOf(ws)
		if !ok {
			http.Error(w, `{"error":"email sequences not supported"}`, http.StatusNotImplemented)
			return
		}
		workspaceID := r.PathValue("id")

		var in sequenceInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if problems := validateSequenceInput(in); len(problems) > 0 {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"problems": problems})
			return
		}

		steps := make([]store.EmailSequenceStep, len(in.Steps))
		for i, st := range in.Steps {
			steps[i] = store.EmailSequenceStep{
				DelayDays: st.DelayDays,
				Subject:   st.Subject,
				Body:      st.Body,
			}
		}
		created, err := seqStore.CreateEmailSequence(r.Context(), store.EmailSequence{
			WorkspaceID: workspaceID,
			Name:        strings.TrimSpace(in.Name),
			Description: strings.TrimSpace(in.Description),
			Steps:       steps,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, toEmailSequenceJSON(created))
	}
}

// PUT /api/v1/workspaces/{id}/email-sequences/{seqId}
func updateEmailSequenceHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seqStore, ok := emailSequenceStoreOf(ws)
		if !ok {
			http.Error(w, `{"error":"email sequences not supported"}`, http.StatusNotImplemented)
			return
		}
		workspaceID := r.PathValue("id")
		seqID := r.PathValue("seqId")

		var in sequenceInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if problems := validateSequenceInput(in); len(problems) > 0 {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"problems": problems})
			return
		}

		steps := make([]store.EmailSequenceStep, len(in.Steps))
		for i, st := range in.Steps {
			steps[i] = store.EmailSequenceStep{
				DelayDays: st.DelayDays,
				Subject:   st.Subject,
				Body:      st.Body,
			}
		}
		updated, err := seqStore.UpdateEmailSequence(r.Context(), store.EmailSequence{
			ID:          seqID,
			WorkspaceID: workspaceID,
			Name:        strings.TrimSpace(in.Name),
			Description: strings.TrimSpace(in.Description),
			Steps:       steps,
		})
		if err != nil {
			if err == store.ErrNotFound {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "sequence not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, toEmailSequenceJSON(updated))
	}
}

// DELETE /api/v1/workspaces/{id}/email-sequences/{seqId}
func deleteEmailSequenceHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seqStore, ok := emailSequenceStoreOf(ws)
		if !ok {
			http.Error(w, `{"error":"email sequences not supported"}`, http.StatusNotImplemented)
			return
		}
		workspaceID := r.PathValue("id")
		seqID := r.PathValue("seqId")
		err := seqStore.DeleteEmailSequence(r.Context(), workspaceID, seqID)
		if err != nil {
			if err == store.ErrNotFound {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "sequence not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
