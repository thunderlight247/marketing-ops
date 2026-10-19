package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"marketingops/api/internal/store"
)

// contentVersionJSON is one immutable brief snapshot. ChannelCopy and
// Metadata are passed through as generic JSON objects.
type contentVersionJSON struct {
	ID            string  `json:"id"`
	ContentItemID string  `json:"contentItemId"`
	VersionNumber int     `json:"versionNumber"`
	Title         string  `json:"title"`
	Excerpt       *string `json:"excerpt"`
	Body          *string `json:"body"`
	CallToAction  *string `json:"callToAction,omitempty"`
	ChannelCopy   any     `json:"channelCopy,omitempty"`
	Metadata      any     `json:"metadata,omitempty"`
	CreatedAt     string  `json:"createdAt"`
}

func toContentVersionJSON(v store.ContentVersion) contentVersionJSON {
	body := v.Body
	return contentVersionJSON{
		ID:            v.ID,
		ContentItemID: v.ContentItemID,
		VersionNumber: v.VersionNumber,
		Title:         v.Title,
		Excerpt:       v.Excerpt,
		Body:          &body,
		CallToAction:  v.CallToAction,
		ChannelCopy:   v.ChannelCopy,
		Metadata:      v.Metadata,
		CreatedAt:     v.CreatedAt.UTC().Format(time.RFC3339),
	}
}

const (
	maxContentItemPatchBytes = 128 * 1024
	maxContentBodyRunes      = 100_000
	maxContentFieldRunes     = 5_000
	maxChannelCopyEntries    = 32
	maxContentMetadataBytes  = 16 * 1024
)

// GET /api/v1/workspaces/{id}/content/{itemId} returns the item plus its
// newest brief version (or version null for a never-edited item).
func getWorkspaceContentItemHandler(ws WorkspaceStore) http.HandlerFunc {
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
		item, version, err := ws.GetContentItemDetail(r.Context(), workspaceID, itemID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "no such content item")
			} else {
				writeJSONError(w, http.StatusInternalServerError, "content lookup failed")
			}
			return
		}

		resp := struct {
			contentItemJSON
			Version *contentVersionJSON `json:"version"`
		}{toContentItemJSON(item), nil}
		if version != nil {
			v := toContentVersionJSON(*version)
			resp.Version = &v
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// draftPatch is the PATCH body. Omitted fields keep the current values:
// planning facts keep the item's column, brief fields keep the latest
// version's values. Explicit null (or "") clears a text field.
type draftPatch struct {
	Title            *string            `json:"title"`
	Slug             *string            `json:"slug"`
	PlannedPublishAt *string            `json:"plannedPublishAt"`
	Excerpt          *string            `json:"excerpt"`
	Body             *string            `json:"body"`
	CallToAction     *string            `json:"callToAction"`
	ChannelCopy      *map[string]string `json:"channelCopy"`
	Metadata         *map[string]any    `json:"metadata"`
}

// PATCH /api/v1/workspaces/{id}/content/{itemId} saves an editable draft:
// it updates the planning record and appends one immutable version. Only
// drafts are editable; approved or published content is frozen until the
// approval workflow (Milestone 5) says otherwise.
func updateWorkspaceContentItemHandler(ws WorkspaceStore) http.HandlerFunc {
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

		body, err := io.ReadAll(io.LimitReader(r.Body, maxContentItemPatchBytes+1))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "could not read the request body")
			return
		}
		if len(body) > maxContentItemPatchBytes {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request body is too large")
			return
		}
		var patch draftPatch
		if err := json.Unmarshal(body, &patch); err != nil {
			writeJSONError(w, http.StatusBadRequest, "request body is not valid JSON")
			return
		}

		item, version, err := ws.GetContentItemDetail(r.Context(), workspaceID, itemID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "no such content item")
			} else {
				writeJSONError(w, http.StatusInternalServerError, "content lookup failed")
			}
			return
		}

		previousStatus := item.Status // "approved" matters for invalidation

		input, problems := buildDraftInput(item, version, patch)
		if len(problems) > 0 {
			writeJSONError(w, http.StatusBadRequest, strings.Join(problems, "; "))
			return
		}

		updated, newVersion, err := ws.SaveContentDraft(r.Context(), workspaceID, itemID, input)
		switch {
		case err == nil:
			audit(r.Context(), ws, store.AuditEvent{
				WorkspaceID: &workspaceID,
				Action:      "content.updated",
				Details: map[string]any{
					"id":            updated.ID,
					"kind":          updated.Kind,
					"title":         updated.Title,
					"versionNumber": newVersion.VersionNumber,
				},
			})
			// Editing approved content invalidates the approval until it is
			// re-decided; the status flip to draft is recorded for the trail.
			if previousStatus == "approved" {
				audit(r.Context(), ws, store.AuditEvent{
					WorkspaceID: &workspaceID,
					Action:      "content.approval_invalidated",
					EntityType:  "content_item",
					EntityID:    &updated.ID,
					Details: map[string]any{
						"id":            updated.ID,
						"versionNumber": newVersion.VersionNumber,
						"reason":        "edited after approval",
					},
				})
			}
			resp := struct {
				contentItemJSON
				Version contentVersionJSON `json:"version"`
			}{toContentItemJSON(updated), toContentVersionJSON(newVersion)}
			writeJSON(w, http.StatusOK, resp)
		case errors.Is(err, store.ErrNotFound):
			writeJSONError(w, http.StatusNotFound, "no such content item")
		case errors.Is(err, store.ErrNotEditable):
			writeJSONError(w, http.StatusConflict, "content that is approved or published is frozen and cannot be edited here")
		case errors.Is(err, store.ErrSlugTaken):
			writeJSONError(w, http.StatusConflict, "a content item with that slug already exists in this workspace")
		default:
			writeJSONError(w, http.StatusInternalServerError, "content update failed")
		}
	}
}

// buildDraftInput merges the PATCH onto the current item and latest version,
// validating everything in one pass.
func buildDraftInput(item store.ContentItem, version *store.ContentVersion, patch draftPatch) (store.ContentDraftInput, []string) {
	in := store.ContentDraftInput{
		Title:       item.Title,
		ChannelCopy: map[string]any{},
		Metadata:    map[string]any{},
	}
	var fromExcerpt, fromBody, fromCTA *string
	var fromChannels map[string]any
	var fromMetadata map[string]any
	if version != nil {
		fromExcerpt, fromBody, fromCTA = version.Excerpt, &version.Body, version.CallToAction
		fromChannels = version.ChannelCopy
		fromMetadata = version.Metadata
	} else {
		empty := ""
		fromBody = &empty
		fromMetadata = map[string]any{}
	}

	var problems []string
	add := func(msg string) { problems = append(problems, msg) }

	// ---- planning record facts -------------------------------------------------
	if patch.Title != nil {
		title := strings.TrimSpace(*patch.Title)
		if title == "" {
			add("title cannot be blank")
		} else if runeCount(title) > maxContentTitleRunes {
			add("title is too long (max 200 characters)")
		} else {
			in.Title = title
		}
	}

	if patch.Slug != nil {
		slug := strings.ToLower(strings.TrimSpace(*patch.Slug))
		if slug == "" {
			empty := ""
			in.Slug = &empty // clears the slug
		} else if !slugPattern.MatchString(slug) {
			add("slug must be lowercase letters, digits, or hyphens")
		} else {
			in.Slug = &slug
		}
	}

	if patch.PlannedPublishAt != nil {
		raw := strings.TrimSpace(*patch.PlannedPublishAt)
		switch {
		case raw == "":
			in.ClearPlannedPublishAt = true
		default:
			when, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				add("plannedPublishAt must be a UTC ISO 8601 timestamp")
			} else {
				in.PlannedPublishAt = &when
			}
		}
	}

	// ---- brief fields (each becomes part of the new version) -------------------
	if patch.Excerpt != nil {
		in.Excerpt = nullableField(*patch.Excerpt, "excerpt", maxContentFieldRunes, &problems)
	} else {
		in.Excerpt = fromExcerpt
	}

	if patch.Body != nil {
		if utf8.RuneCountInString(*patch.Body) > maxContentBodyRunes {
			add("body is too long (max 100000 characters)")
		} else {
			in.Body = patch.Body
		}
	} else {
		in.Body = fromBody
	}

	if patch.CallToAction != nil {
		in.CallToAction = nullableField(*patch.CallToAction, "callToAction", maxContentFieldRunes, &problems)
	} else {
		in.CallToAction = fromCTA
	}

	if patch.ChannelCopy != nil {
		channels := *patch.ChannelCopy
		if len(channels) > maxChannelCopyEntries {
			add("channelCopy has too many channels (max 32)")
		}
		out := map[string]any{}
		for name, copy := range channels {
			name = strings.ToLower(strings.TrimSpace(name))
			if name == "" {
				add("channelCopy channel names cannot be blank")
				continue
			}
			if runeCount(copy) > maxContentFieldRunes {
				add("channelCopy for " + name + " is too long (max 5000 characters)")
				continue
			}
			out[name] = copy
		}
		in.ChannelCopy = out
	} else {
		in.ChannelCopy = fromChannels
	}
	if in.ChannelCopy == nil {
		in.ChannelCopy = map[string]any{}
	}

	// Metadata is for debugging context (prompt/config markers, notes) and
	// must stay a JSON object within a size cap.
	if patch.Metadata != nil {
		raw, err := json.Marshal(*patch.Metadata)
		if err != nil || len(raw) > maxContentMetadataBytes {
			add("metadata is too large (max 16384 bytes as JSON)")
		} else {
			in.Metadata = *patch.Metadata
		}
	} else {
		in.Metadata = fromMetadata
	}
	if in.Metadata == nil {
		in.Metadata = map[string]any{}
	}

	return in, problems
}

// nullableField converts an optional string field: blank clears to NULL,
// otherwise trim and enforce the cap.
func nullableField(value, name string, maxRunes int, problems *[]string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	if runeCount(trimmed) > maxRunes {
		*problems = append(*problems, name+" is too long (max "+strconv.Itoa(maxRunes)+" characters)")
		return nil
	}
	return &trimmed
}
