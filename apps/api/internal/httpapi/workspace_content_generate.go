package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"marketingops/api/internal/aiprovider"
	"marketingops/api/internal/store"
)

// ContentGenerator is what the generate handler needs from the AI side. Both
// *aiprovider.StubProvider and *aiprovider.OpenRouter satisfy it.
type ContentGenerator interface {
	Name() string
	Generate(ctx context.Context, req aiprovider.GenerateRequest) (aiprovider.GenerateResponse, error)
}

const (
	maxGeneratePayloadBytes = 16 * 1024
	maxGenerateBriefRunes   = 2_000
	maxGenerateInstrRunes   = 2_000
	defaultGenerateTimeout  = 90 * time.Second
)

// POST /api/v1/workspaces/{id}/content/generate asks a model for a structured
// draft and stores it as a draft content item with a version snapshot.
//
// The human-approval rule runs through the whole pipeline: nothing generated
// here is ever queued or published automatically — the item stays a draft and
// publishing only ever starts from an approved version (M5+).
func generateWorkspaceContentHandler(ws WorkspaceStore, gen ContentGenerator, timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		if gen == nil {
			writeJSONError(w, http.StatusNotImplemented, "AI generation is not configured on this server")
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
		// A workspace without a saved brand profile still drafts fine; the
		// model just gets less context.
		brand, err := ws.GetBrandProfile(r.Context(), workspaceID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusInternalServerError, "brand profile lookup failed")
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, maxGeneratePayloadBytes+1))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "could not read the request body")
			return
		}
		if len(body) > maxGeneratePayloadBytes {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request body is too large")
			return
		}
		var req struct {
			Kind         string  `json:"kind"`
			Topic        *string `json:"topic"`
			Channel      *string `json:"channel"`
			Instructions *string `json:"instructions"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "request body is not valid JSON")
			return
		}

		var problems []string
		kind := strings.TrimSpace(req.Kind)
		if !validContentKinds[kind] {
			problems = append(problems, "kind must be blog_article, social_post, or snippet")
		}
		topic := trim(req.Topic)
		if topic == "" {
			problems = append(problems, "topic is required")
		} else if runeCount(topic) > maxGenerateBriefRunes {
			problems = append(problems, "topic is too long (max 2000 characters)")
		}
		channel := strings.ToLower(trim(req.Channel))
		if channel != "" && runeCount(channel) > 64 {
			problems = append(problems, "channel is too long (max 64 characters)")
		}
		instructions := trim(req.Instructions)
		if runeCount(instructions) > maxGenerateInstrRunes {
			problems = append(problems, "instructions are too long (max 2000 characters)")
		}
		if len(problems) > 0 {
			writeJSONError(w, http.StatusBadRequest, strings.Join(problems, "; "))
			return
		}

		// Enforce the generation timeout server-side regardless of what the
		// caller's own context does.
		if timeout <= 0 {
			timeout = defaultGenerateTimeout
		}
		genCtx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		result, err := gen.Generate(genCtx, aiprovider.GenerateRequest{
			Kind:         kind,
			Topic:        topic,
			Channel:      channel,
			Instructions: instructions,
			Workspace:    workspaceContext(workspace, brand),
		})
		if err != nil {
			// Only classified provider errors may surface their text; anything
			// else (e.g. a custom provider implementation erroring out) stays
			// generic so no internal detail can leak into the response. The
			// same convention guards the agent routes.
			message := "the AI provider could not be reached"
			var providerErr *aiprovider.ProviderError
			if errors.As(err, &providerErr) {
				message = providerErr.Error()
			}
			writeJSONError(w, http.StatusBadGateway, message)
			return
		}

		created, err := ws.InsertContentItem(r.Context(), store.ContentItem{
			WorkspaceID: workspaceID,
			Kind:        kind,
			Title:       result.Title,
			Slug:        emptyToNil(result.Slug),
		})
		if err != nil {
			if errors.Is(err, store.ErrSlugTaken) {
				writeJSONError(w, http.StatusConflict,
					"the generated slug already exists in this workspace; reword the topic or rename the existing draft")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "storing the generated draft failed")
			return
		}

		metadata := map[string]any{"source": "ai", "topic_length": runeCount(topic)}
		for key, value := range result.Metadata {
			if isScalarMetadata(value) {
				metadata[key] = value
			}
		}

		// InsertContentItem already stored the item; the first save produces
		// version 1 with the full brief. Only drafts are editable, which also
		// guarantees this generated draft cannot be anything but a draft.
		_, version, err := ws.SaveContentDraft(r.Context(), workspaceID, created.ID, store.ContentDraftInput{
			Title:        result.Title,
			Excerpt:      emptyToNil(result.Excerpt),
			Body:         &result.Body,
			CallToAction: emptyToNil(result.CallToAction),
			ChannelCopy:  stringsMapToAny(result.ChannelCopy),
			Metadata:     metadata,
		})
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "storing the generated draft failed")
			return
		}

		audit(r.Context(), ws, store.AuditEvent{
			WorkspaceID: &workspaceID,
			Action:      "content.generated",
			Details: map[string]any{
				"id":           created.ID,
				"kind":         kind,
				"provider":     result.Provider,
				"model":        result.Model,
				"topic_length": runeCount(topic),
			},
		})

		writeJSON(w, http.StatusCreated, struct {
			contentItemJSON
			Version  contentVersionJSON `json:"version"`
			Provider string             `json:"provider"`
			Model    string             `json:"model"`
		}{
			contentItemJSON: toContentItemJSON(created),
			Version:         toContentVersionJSON(version),
			Provider:        result.Provider,
			Model:           result.Model,
		})
	}
}

// workspaceContext assembles the non-secret brand context for the prompt.
func workspaceContext(w store.Workspace, b store.BrandProfile) aiprovider.WorkspaceContext {
	deref := func(s *string) string {
		if s == nil {
			return ""
		}
		return strings.TrimSpace(*s)
	}
	website := ""
	if w.WebsiteURL != nil {
		website = *w.WebsiteURL
	}
	return aiprovider.WorkspaceContext{
		Name:             w.Name,
		WebsiteURL:       website,
		Description:      deref(b.BusinessDescription),
		Audience:         deref(b.Audience),
		Services:         splitLines(b.Services),
		Tone:             deref(b.Tone),
		ProhibitedClaims: b.ProhibitedClaims,
		CallsToAction:    b.CallsToAction,
	}
}

// splitLines turns the single services text field into a clean list.
func splitLines(s *string) []string {
	if s == nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(*s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// isScalarMetadata keeps the stored prompt metadata small and stringifyable.
func isScalarMetadata(value any) bool {
	switch value.(type) {
	case string, bool, int, int64, float64:
		if s, ok := value.(string); ok && len(s) > 500 {
			return false
		}
		return true
	default:
		return false
	}
}

func stringsMapToAny(in map[string]string) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
