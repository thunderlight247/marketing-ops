package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"marketingops/api/internal/agents"
	"marketingops/api/internal/aiprovider"
	"marketingops/api/internal/store"
)

// contentCreatorMaxPayloadBytes bounds one creation request.
const contentCreatorMaxPayloadBytes = 8 * 1024

// POST /api/v1/workspaces/{id}/agents/content-creator/draft runs the
// content-creator specialist for one workspace and files its validated output
// as a draft in the existing approval workflow. It never publishes or
// schedules: the operator reviews the draft and approves it through the
// ordinary content flow.
//
// Without "contentItemId" a new draft content item is created. With one, the
// plan is saved as a new immutable version of that item (only draft, approved,
// or rejected items accept saves — the store enforces it).
func contentCreatorDraftHandler(ws WorkspaceStore, runner agents.PromptRun, timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		if runner == nil {
			writeJSONError(w, http.StatusNotImplemented, "agent specialists are not configured on this server")
			return
		}
		workspaceID := r.PathValue("id")
		workspace, err := ws.GetWorkspaceByID(r.Context(), workspaceID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "no such workspace")
			} else {
				writeJSONError(w, http.StatusInternalServerError, "workspace lookup failed")
			}
			return
		}
		// A workspace without a saved brand profile is still writable; the
		// creator just gets less context.
		brand, err := ws.GetBrandProfile(r.Context(), workspaceID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusInternalServerError, "brand profile lookup failed")
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, contentCreatorMaxPayloadBytes+1))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "could not read the request body")
			return
		}
		if len(body) > contentCreatorMaxPayloadBytes {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request body is too large")
			return
		}
		var req struct {
			Kind          *string `json:"kind"`
			Focus         *string `json:"focus"`
			ContentItemID *string `json:"contentItemId"`
		}
		if len(body) > 0 {
			if err := json.Unmarshal(body, &req); err != nil {
				writeJSONError(w, http.StatusBadRequest, "request body is not valid JSON")
				return
			}
		}

		kind := trim(req.Kind)
		if kind == "" {
			kind = "blog_article"
		} else if !validContentKinds[kind] {
			writeJSONError(w, http.StatusBadRequest, "kind must be blog_article, social_post, or snippet")
			return
		}
		focus := trim(req.Focus)
		if runeCount(focus) > maxGenerateInstrRunes {
			writeJSONError(w, http.StatusBadRequest, "focus is too long (max 2000 characters)")
			return
		}
		// The agent prompt expects one brief line; the content kind is part
		// of the brief, not a separate channel of context.
		brief := "Content kind for this piece: " + kind
		if focus != "" {
			brief += "\n" + focus
		}

		itemID := trim(req.ContentItemID)
		var item store.ContentItem
		if itemID != "" {
			item, _, err = ws.GetContentItemDetail(r.Context(), workspaceID, itemID)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					writeJSONError(w, http.StatusNotFound, "no such content item in this workspace")
				} else {
					writeJSONError(w, http.StatusInternalServerError, "content item lookup failed")
				}
				return
			}
			if item.Status != "draft" && item.Status != "approved" && item.Status != "rejected" {
				writeJSONError(w, http.StatusConflict, "the content item is frozen; its text can no longer be replaced")
				return
			}
			if req.Kind != nil && kind != item.Kind {
				writeJSONError(w, http.StatusBadRequest, "kind must match the existing content item")
				return
			}
			brief = "Content kind for this piece: " + item.Kind + "\n" + focus
		}

		runTimeout := timeout
		if runTimeout <= 0 {
			runTimeout = defaultAgentTimeout
		}
		runCtx, cancel := context.WithTimeout(r.Context(), runTimeout)
		defer cancel()

		creator := agents.ContentCreator{}
		runStarted := time.Now()
		plan, agentErr := creator.Create(runCtx, runner, agents.AgentInput{
			Workspace: workspaceContext(workspace, brand),
			Focus:     brief,
		})
		runDuration := time.Since(runStarted)
		if agentErr != nil {
			audit(r.Context(), ws, store.AuditEvent{
				WorkspaceID: &workspaceID,
				Action:      "agent.content.draft.failed",
				Details: map[string]any{
					"agent_id":      creator.ID(),
					"agent_version": creator.Version(),
					"reason_kind":   safeFailureKind(agentErr),
				},
			})
			message := "the content specialist could not complete the draft"
			var providerErr *aiprovider.ProviderError
			if errors.As(agentErr, &providerErr) {
				message = providerErr.Error()
			}
			writeJSONError(w, http.StatusBadGateway, message)
			return
		}

		if itemID == "" {
			item, err = ws.InsertContentItem(r.Context(), store.ContentItem{
				WorkspaceID: workspaceID, Kind: kind, Title: plan.Title,
			})
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "content creation failed")
				return
			}
		}
		drift := emptyToNil(plan.Slug)

		saved, version, err := ws.SaveContentDraft(r.Context(), workspaceID, item.ID, store.ContentDraftInput{
			Title:        plan.Title,
			Excerpt:      emptyToNil(plan.Excerpt),
			Body:         &plan.Body,
			CallToAction: emptyToNil(plan.CallToAction),
			ChannelCopy:  stringsMapToAny(nil),
			Metadata: map[string]any{
				"source":        "agent",
				"agentId":       plan.AgentID,
				"agentVersion":  plan.Version,
				"provider":      plan.Provider,
				"model":         plan.Model,
				"slug_suggest":  plan.Slug,
				"keyPointCount": len(plan.KeyPoints),
			},
		})
		if err != nil {
			if errors.Is(err, store.ErrNotEditable) {
				writeJSONError(w, http.StatusConflict,
					"the content item is frozen (review, queued, publishing, or published); its text can no longer be replaced")
				return
			}
			if errors.Is(err, store.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "no such content item in this workspace")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "storing the created draft failed")
			return
		}

		audit(r.Context(), ws, store.AuditEvent{
			WorkspaceID: &workspaceID,
			Action:      "agent.content.draft.created",
			Details: map[string]any{
				"id":            item.ID,
				"kind":          item.Kind,
				"agent_id":      plan.AgentID,
				"agent_version": plan.Version,
				"provider":      plan.Provider,
				"model":         plan.Model,
				"body_runes":    runeCount(plan.Body),
			},
		})

		writeJSON(w, http.StatusCreated, struct {
			Item       contentItemJSON    `json:"item"`
			Version    contentVersionJSON `json:"version"`
			Provider   string             `json:"provider"`
			Model      string             `json:"model"`
			AgentID    string             `json:"agentId"`
			KeyPoints  []string           `json:"keyPoints"`
			DurationMs int64              `json:"durationMs"`
			SlugDrift  *string            `json:"slugSuggestion"`
		}{
			Item:       toContentItemJSON(saved),
			Version:    toContentVersionJSON(version),
			Provider:   plan.Provider,
			Model:      plan.Model,
			AgentID:    plan.AgentID,
			KeyPoints:  plan.KeyPoints,
			DurationMs: runDuration.Milliseconds(),
			SlugDrift:  drift,
		})
	}
}
