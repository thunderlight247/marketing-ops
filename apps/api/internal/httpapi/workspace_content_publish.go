package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"marketingops/api/internal/integration"
	"marketingops/api/internal/secretcipher"
	"marketingops/api/internal/store"
)

// pubResponse is an alias for readability in the response assembly.
type pubResponse = publishResponse

const maxPublishPayloadBytes = 8 * 1024

// Sentinel setup errors distinguishable inside this handler.
var (
	errSanityNotConfigured = errors.New("the sanity integration has no projectId/dataset configured")
	errSanityNoCredentials = errors.New("the sanity integration has no saved write token")
)

type publishResponse struct {
	Item          contentItemJSON `json:"item"`
	Mode          string          `json:"mode"`
	JobID         *string         `json:"jobId,omitempty"`
	TransactionID *string         `json:"transactionId,omitempty"`
	DraftID       *string         `json:"draftId,omitempty"`
	PublicURL     *string         `json:"publicUrl,omitempty"`
	URLVerified   *bool           `json:"urlVerified,omitempty"`
	URLHTTP       *int            `json:"urlHttp,omitempty"`
	Message       string          `json:"message"`
}

// sanitySetup bundles the facts the Sanity ops client needs, resolved from the
// integration row and its decrypted credentials.
type sanitySetup struct {
	ops        integration.SanityOps
	docType    string
	urlPrefix  string
	apiVersion string
}

// buildSanitySetup decrypts credentials server-side and validates config. The
// token is returned only inside the ops struct and never logged.
func buildSanitySetup(ctx context.Context, ws WorkspaceStore, integrationRow store.Integration, key []byte, opsBase string, httpClient *http.Client) (sanitySetup, error) {
	projectID, _ := integrationRow.Config["projectId"].(string)
	dataset, _ := integrationRow.Config["dataset"].(string)
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(dataset) == "" {
		return sanitySetup{}, errSanityNotConfigured
	}

	creds := map[string]string{}
	credSet, err := ws.GetCredentialSet(ctx, integrationRow.ID)
	if err != nil {
		return sanitySetup{}, errSanityNoCredentials
	}
	if len(key) == 0 {
		return sanitySetup{}, errSanityNoCredentials
	}
	opened, err := secretcipher.Decrypt(key, credSet.Ciphertext)
	if err != nil {
		return sanitySetup{}, errSanityNoCredentials
	}
	if err := json.Unmarshal(opened, &creds); err != nil {
		return sanitySetup{}, errSanityNoCredentials
	}
	token := strings.TrimSpace(creds["token"])
	if token == "" {
		return sanitySetup{}, errSanityNoCredentials
	}

	configuredAPIVersion, _ := integrationRow.Config["apiVersion"].(string)
	docType, _ := integrationRow.Config["docType"].(string)
	urlPrefix, _ := integrationRow.Config["publicUrlPrefix"].(string)

	return sanitySetup{
		ops: integration.SanityOps{
			Creds: integration.SanityCredentials{
				ProjectID:  strings.TrimSpace(projectID),
				Dataset:    strings.TrimSpace(dataset),
				Token:      token,
				APIVersion: strings.TrimSpace(configuredAPIVersion),
			},
			Base:   opsBase,
			Client: httpClient,
		},
		docType:   strings.TrimSpace(docType),
		urlPrefix: strings.TrimSpace(urlPrefix),
	}, nil
}

// versionFacts packages what the publish flows need about the item being pushed.
type versionFacts struct {
	itemID         string
	item           store.ContentItem
	versionID      string
	versionNumber  int
	title          string
	slug           string
	excerpt        string
	body           string
	callToAction   string
	channelCopyStr map[string]string
}

// resolveVersionFacts gathers the newest version's content for the Sanity post.
func resolveVersionFacts(item store.ContentItem, version store.ContentVersion) versionFacts {
	f := versionFacts{
		itemID: item.ID, item: item,
		versionID: version.ID, versionNumber: version.VersionNumber,
		title: version.Title,
	}
	if item.Slug != nil {
		f.slug = *item.Slug
	}
	if version.Excerpt != nil {
		f.excerpt = *version.Excerpt
	}
	f.body = version.Body
	if version.CallToAction != nil {
		f.callToAction = *version.CallToAction
	}
	f.channelCopyStr = map[string]string{}
	for name, value := range version.ChannelCopy {
		if s, ok := value.(string); ok {
			f.channelCopyStr[strings.ToLower(strings.TrimSpace(name))] = s
		}
	}
	return f
}

// sanityPostFromFacts assembles the Sanity document for one version.
func sanityPostFromFacts(setup sanitySetup, f versionFacts) integration.SanityPost {
	extra := map[string]any{}
	if f.callToAction != "" {
		extra["callToAction"] = f.callToAction
	}
	if len(f.channelCopyStr) > 0 {
		extra["marketingOpsChannels"] = f.channelCopyStr
	}
	return integration.SanityPost{
		DocType:    setup.docType,
		Title:      f.title,
		Slug:       f.slug,
		Excerpt:    f.excerpt,
		BodyBlocks: integration.ConvertBodyToBlocks(f.body),
		Extra:      extra,
	}
}

// POST /api/v1/workspaces/{id}/content/{itemId}/publish
//
// Body: {"integrationId": "<sanity integration>", "mode": "draft"|"publish"}.
// mode=draft writes (or idempotently replaces) a Sanity DRAFT for the newest
// version — invisible on the public site, no approval required. mode=publish
// is approval-gated by the store and records the destination artifact.
func publishWorkspaceContentHandler(ws WorkspaceStore, key []byte, opsBase string, httpClient *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		workspaceID, itemID := r.PathValue("id"), r.PathValue("itemId")

		workspace, err := ws.GetWorkspaceByID(r.Context(), workspaceID)
		if err != nil {
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

		body, err := io.ReadAll(io.LimitReader(r.Body, maxPublishPayloadBytes+1))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "could not read the request body")
			return
		}
		var req struct {
			IntegrationID *string `json:"integrationId"`
			Mode          *string `json:"mode"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "request body is not valid JSON")
			return
		}
		mode := strings.ToLower(trim(req.Mode))
		if mode != "draft" && mode != "publish" {
			writeJSONError(w, http.StatusBadRequest, "mode must be draft or publish")
			return
		}
		if version == nil {
			writeJSONError(w, http.StatusConflict, "this item has no saved content yet; save the draft once before publishing steps")
			return
		}
		integrationID := trim(req.IntegrationID)
		if integrationID == "" {
			writeJSONError(w, http.StatusBadRequest, "integrationId is required")
			return
		}
		integrationRow, err := ws.GetIntegrationInWorkspace(r.Context(), workspaceID, integrationID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "no such integration in this workspace")
			} else {
				writeJSONError(w, http.StatusInternalServerError, "integration lookup failed")
			}
			return
		}
		if integrationRow.AdapterKind != "sanity" {
			writeJSONError(w, http.StatusBadRequest, "sanity publishing is only wired for the sanity adapter in this milestone")
			return
		}

		setup, setupErr := buildSanitySetup(r.Context(), ws, integrationRow, key, opsBase, httpClient)
		if setupErr != nil {
			writeJSONError(w, http.StatusConflict, setupErr.Error())
			return
		}

		facts := resolveVersionFacts(item, *version)
		ops := setup.ops

		if mode == "draft" {
			draftID, txID, opsErr := ops.CreateDraft(r.Context(), facts.itemID, facts.versionNumber,
				sanityPostFromFacts(setup, facts))
			if opsErr != nil {
				writeSanityFailure(w, opsErr)
				return
			}
			audit(r.Context(), ws, store.AuditEvent{
				WorkspaceID: &workspaceID,
				Action:      "content.sanity_draft_created",
				EntityType:  "content_item",
				EntityID:    &facts.itemID,
				Details: map[string]any{
					"id":      facts.itemID,
					"draftId": draftID,
					"version": facts.versionNumber,
				},
			})
			resp := publishResponse{
				Item:          toContentItemJSON(item),
				Mode:          mode,
				DraftID:       &draftID,
				TransactionID: &txID,
				Message:       "Sanity draft created (or replaced) for this version. It is not visible publicly; publish still requires the publish step on the approved version.",
			}
			writeJSON(w, http.StatusCreated, resp)
			return
		}

		// ---- publish mode: approval-gated and idempotent ----------------
		if facts.slug == "" {
			writeJSONError(w, http.StatusBadRequest, "the item has no slug; set one before publishing")
			return
		}
		idempotencyKey := fmt.Sprintf("%s:%s:%s:%s",
			workspace.Slug, facts.itemID, facts.versionID, integrationID)

		// A job whose key matches this exact request is RESUMED: succeeded means
		// conflict (duplicate publish), anything else re-runs as another attempt
		// instead of tripping the unique key with a duplicate insert.
		var job *store.PublishingJob
		existing, keyErr := ws.GetPublishingJobByIdempotencyKey(r.Context(), workspaceID, idempotencyKey)
		switch {
		case keyErr == nil && existing.Status == "succeeded":
			writeJSONError(w, http.StatusConflict,
				"this exact version was already published through this integration; it is recorded in the publishing history")
			return
		case keyErr == nil:
			resume := existing
			job = &resume
		case errors.Is(keyErr, store.ErrNotFound):
			created, insertErr := ws.InsertPublishingJob(r.Context(), workspaceID, store.PublishingJob{
				ContentItemID:    facts.itemID,
				ContentVersionID: facts.versionID,
				IntegrationID:    integrationID,
				IdempotencyKey:   idempotencyKey,
			})
			switch {
			case insertErr == nil:
				job = &created
			case errors.Is(insertErr, store.ErrNotApproved):
				writeJSONError(w, http.StatusConflict,
					"the newest version must be approved before it can be published (or it changed after approval)")
				return
			default:
				writeJSONError(w, http.StatusInternalServerError, "publishing job creation failed")
				return
			}
		default:
			writeJSONError(w, http.StatusInternalServerError, "publishing job lookup failed")
			return
		}

		if err := ws.SetPublishingJobStatus(r.Context(), workspaceID, job.ID, "running", ""); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "publishing job bookkeeping failed")
			return
		}

		publishedID, txID, opsErr := ops.Publish(r.Context(), facts.itemID, facts.versionNumber)
		var sanityErr *integration.SanityWriteError
		if errors.As(opsErr, &sanityErr) && sanityErr.Kind == integration.SanityFailDraftGone {
			// The draft was never created (or was deleted): recreate it
			// deterministically, then publish. Still approval-gated above.
			if _, _, createErr := ops.CreateDraft(r.Context(), facts.itemID, facts.versionNumber,
				sanityPostFromFacts(setup, facts)); createErr == nil {
				publishedID, txID, opsErr = ops.Publish(r.Context(), facts.itemID, facts.versionNumber)
			}
		}
		if opsErr != nil {
			_ = ws.SetPublishingJobStatus(r.Context(), workspaceID, job.ID, "failed", safeText(opsErr))
			writeSanityFailure(w, opsErr)
			return
		}

		if err := ws.SetPublishingJobStatus(r.Context(), workspaceID, job.ID, "succeeded", ""); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "publishing succeeded but job bookkeeping failed; the artifact was not recorded")
			return
		}
		if err := ws.RecordPublishedArtifact(r.Context(), job.ID, integrationID, &publishedID, nil); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "publishing succeeded but the artifact record failed")
			return
		}
		publishedItem, err := ws.MarkContentPublished(r.Context(), workspaceID, facts.itemID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "publishing succeeded but the item status update failed")
			return
		}

		var publicURL string
		var ok bool
		var status int
		if website := derefString(workspace.WebsiteURL); website != "" {
			publicURL, ok, status = ops.VerifyPublicURL(r.Context(), website, setup.urlPrefix, facts.slug)
		}

		audit(r.Context(), ws, store.AuditEvent{
			WorkspaceID: &workspaceID,
			Action:      "content.published",
			EntityType:  "content_item",
			EntityID:    &facts.itemID,
			Details: map[string]any{
				"id":          facts.itemID,
				"jobId":       job.ID,
				"publishedId": publishedID,
				"version":     facts.versionNumber,
				"url":         publicURL,
				"verified":    ok,
			},
		})

		resp := publishResponse{
			Item:          toContentItemJSON(publishedItem),
			Mode:          mode,
			JobID:         &job.ID,
			TransactionID: &txID,
			PublicURL:     &publicURL,
			URLVerified:   &ok,
			URLHTTP:       &status,
			Message:       message(ok),
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func message(ok bool) string {
	if ok {
		return "Published: the Sanity transaction succeeded and the public URL answered 200."
	}
	return "Published: the Sanity transaction succeeded. The public URL could not be verified from here — open it in a browser to confirm."
}

// derefString returns "" for nil pointers (workspace website URL).
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// safeText keeps only the classified error text for storage.
func safeText(err error) string {
	return err.Error()
}

// writeSanityFailure maps classified Sanity failures to a 502. Texts are
// credential-free by construction.
func writeSanityFailure(w http.ResponseWriter, err error) {
	writeJSONError(w, http.StatusBadGateway, err.Error())
}
