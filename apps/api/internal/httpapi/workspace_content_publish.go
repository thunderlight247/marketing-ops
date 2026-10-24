package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"marketingops/api/internal/integration"
	"marketingops/api/internal/secretcipher"
	"marketingops/api/internal/store"
)

// pubResponse is an alias for readability in the response assembly.
type pubResponse = publishResponse

const maxPublishPayloadBytes = 8 * 1024

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

// buildPublisher decrypts credentials server-side and delegates provider
// configuration to the registered publishing adapter. Secret values are never
// returned to the caller except inside the publisher implementation.
func buildPublisher(ctx context.Context, ws WorkspaceStore, integrationRow store.Integration, key []byte, registry *integration.Registry, apiBase string, httpClient *http.Client) (integration.Publisher, error) {
	if ws == nil {
		return nil, errors.New("workspace store is unavailable")
	}
	if registry == nil {
		return nil, errors.New("publishing adapters are unavailable")
	}
	adapter, ok := registry.Publisher(integrationRow.AdapterKind)
	if !ok {
		return nil, fmt.Errorf("%s integrations do not support publishing yet", integrationRow.AdapterKind)
	}
	if _, ok := registry.Get(integrationRow.AdapterKind); !ok {
		return nil, fmt.Errorf("%s integrations are not registered", integrationRow.AdapterKind)
	}

	credentials := map[string]string{}
	credSet, err := ws.GetCredentialSet(ctx, integrationRow.ID)
	if err != nil || len(key) == 0 {
		return nil, integration.ErrPublisherCredentialsMissing
	}
	opened, err := secretcipher.Decrypt(key, credSet.Ciphertext)
	// Decrypt/parse failure is NOT "missing": credentials are stored but
	// unreadable with the current server key (it may have changed). The
	// distinct sentinel tells the operator to re-enter them.
	if err != nil {
		return nil, integration.ErrPublisherCredentialsUnreadable
	}
	if json.Unmarshal(opened, &credentials) != nil {
		return nil, integration.ErrPublisherCredentialsUnreadable
	}
	publisher, err := adapter.NewPublisher(integrationRow.Config, credentials, apiBase, httpClient)
	if errors.Is(err, integration.ErrPublisherNotConfigured) {
		return nil, err
	}
	if errors.Is(err, integration.ErrPublisherCredentialsMissing) {
		return nil, err
	}
	if err != nil {
		return nil, errors.New("publishing adapter could not be configured")
	}
	return publisher, nil
}

// versionFacts packages the immutable facts needed by any publisher.
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
	mainImage      *integration.PayloadMainImage
	channelCopyStr map[string]string
	videoURL       string
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
	// An uploaded hero image is stored in the version metadata:
	// {"mainImage": {"ref": "image-...", "src": "https://...", "alt": "..."}}.
	// The asset reference and alt text travel to all publishers; the public
	// CDN URL only matters to destinations that fetch the image themselves
	// (Instagram), and it never includes credentials.
	if raw, ok := version.Metadata["mainImage"].(map[string]any); ok {
		if ref, ok := raw["ref"].(string); ok && ref != "" {
			alt, _ := raw["alt"].(string)
			src, _ := raw["src"].(string)
			f.mainImage = &integration.PayloadMainImage{Ref: ref, URL: src, Alt: alt}
		}
	}
	// A video destination (YouTube) points at a publicly fetchable video
	// file through version metadata: {"videoUrl": "https://.../clip.mp4"}.
	// Only the URL travels to publishers; the file is downloaded
	// server-side at publish time and never stored here.
	if raw, ok := version.Metadata["videoUrl"].(string); ok {
		f.videoURL = strings.TrimSpace(raw)
	}
	return f
}

// markReconnectionNeeded flips an integration into the error lifecycle and
// records the publish capability as failed when the destination rejected the
// saved credentials, so the dashboard can prompt reconnection instead of
// leaving a dead integration looking connected (Milestone 10). Silent best
// effort: the publish failure itself is still reported to the caller.
func markReconnectionNeeded(ctx context.Context, ws WorkspaceStore, workspaceID, integrationID string, opsErr error) {
	if ws == nil || !integration.PublisherNeedsReconnection(opsErr) {
		return
	}
	if statusErr := ws.SetIntegrationStatus(ctx, workspaceID, integrationID, integration.LifecycleError); statusErr != nil {
		return
	}
	detail := safePublisherErrorText(opsErr)
	_ = ws.UpsertCapability(ctx, integrationID, store.IntegrationCapability{
		IntegrationID: integrationID,
		Capability:    integration.CapPublish,
		Status:        integration.StatusFailed,
		Details:       &detail,
	})
}

// publisherPayload assembles one immutable approved content snapshot for any adapter.
func publisherPayload(f versionFacts) integration.ContentPayload {
	return integration.ContentPayload{
		ItemID: f.itemID, Version: f.versionNumber, Title: f.title, Slug: f.slug,
		Excerpt: f.excerpt, Body: f.body, CallToAction: f.callToAction,
		ChannelCopy: f.channelCopyStr, MainImage: f.mainImage,
		VideoURL: f.videoURL,
	}
}

// POST /api/v1/workspaces/{id}/content/{itemId}/publish
//
// Body: {"integrationId": "<workspace integration>", "mode": "draft"|"publish"}.
// Draft writes are destination-specific and do not change public content;
// publishing is approval-gated by the store and records the destination artifact.
func publishWorkspaceContentHandler(ws WorkspaceStore, key []byte, registry *integration.Registry, apiBase string, httpClient *http.Client) http.HandlerFunc {
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
		var req publishRequest
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "request body is not valid JSON")
			return
		}
		scheduledAt, hasSchedule, scheduleProblems := parsePublishingTime(req.ScheduledAt)
		if len(scheduleProblems) > 0 {
			writeJSONError(w, http.StatusBadRequest, strings.Join(scheduleProblems, "; "))
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
		publisher, setupErr := buildPublisher(r.Context(), ws, integrationRow, key, registry, apiBase, httpClient)
		if setupErr != nil {
			if errors.Is(setupErr, integration.ErrPublisherNotConfigured) || errors.Is(setupErr, integration.ErrPublisherCredentialsMissing) || errors.Is(setupErr, integration.ErrPublisherCredentialsUnreadable) {
				writeJSONError(w, http.StatusConflict, setupErr.Error())
			} else {
				writeJSONError(w, http.StatusBadRequest, setupErr.Error())
			}
			return
		}

		facts := resolveVersionFacts(item, *version)
		payload := publisherPayload(facts)

		if mode == "draft" {
			draftID, txID, opsErr := publisher.CreateDraft(r.Context(), payload)
			if opsErr != nil {
				markReconnectionNeeded(r.Context(), ws, workspaceID, integrationID, opsErr)
				writePublisherFailure(w, opsErr)
				return
			}
			audit(r.Context(), ws, store.AuditEvent{
				WorkspaceID: &workspaceID,
				Action:      "content.publisher_draft_created",
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
				Message:       "Destination draft created or replaced. It is not visible publicly; publish still requires an approved version.",
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
		case keyErr == nil && item.Status != "approved":
			writeJSONError(w, http.StatusConflict,
				"the newest version must still be approved before a failed publish can be retried")
			return
		case keyErr == nil && existing.Status == "running":
			writeJSONError(w, http.StatusConflict,
				"a publish attempt for this exact version is already running")
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
				ScheduledAt:      scheduleTimePointer(scheduledAt, hasSchedule),
				NextAttemptAt:    scheduleTimePointer(scheduledAt, hasSchedule),
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

		// A future schedule queues the job for the background worker instead of
		// publishing inline right now. A past/absent schedule keeps the previous
		// synchronous behavior. The scheduled request still creates a durable
		// job — restarting the process cannot lose it (Milestone 9 acceptance).
		if hasSchedule && scheduledAt.After(time.Now().UTC()) {
			// No inline "running" claim is made, so the worker still owns it.
			audit(r.Context(), ws, store.AuditEvent{
				WorkspaceID: &workspaceID,
				Action:      "content.publish_scheduled",
				EntityType:  "content_item",
				EntityID:    &facts.itemID,
				Details: map[string]any{
					"id":       facts.itemID,
					"jobId":    job.ID,
					"version":  facts.versionNumber,
					"schedule": scheduledAt.UTC().Format(time.RFC3339),
				},
			})
			result := publishResponse{
				Item:    toContentItemJSON(item),
				Mode:    mode,
				JobID:   &job.ID,
				Message: "The publish is queued; the background worker will run it at the scheduled time.",
			}
			writeJSON(w, http.StatusAccepted, result)
			return
		}

		if err := ws.SetPublishingJobStatus(r.Context(), workspaceID, job.ID, "running", ""); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "publishing job bookkeeping failed")
			return
		}

		// Refresh the destination draft from the exact latest approved version,
		// then publish that same immutable backend snapshot.
		_, _, publishErr := publisher.CreateDraft(r.Context(), payload)
		var outcome integration.PublishOutcome
		if publishErr == nil {
			outcome, publishErr = publisher.Publish(r.Context(), payload)
		}
		if publishErr != nil {
			_ = ws.SetPublishingJobStatus(r.Context(), workspaceID, job.ID, "failed", safePublisherErrorText(publishErr))
			markReconnectionNeeded(r.Context(), ws, workspaceID, integrationID, publishErr)
			writePublisherFailure(w, publishErr)
			return
		}

		// Verify the resulting URL read-only and store it with the artifact
		// even if the destination site is not reachable yet.
		verification := integration.URLVerification{}
		if website := derefString(workspace.WebsiteURL); website != "" {
			verification = publisher.VerifyPublicURL(r.Context(), website, facts.slug)
		}
		publicURL, ok, status := verification.URL, verification.Verified, verification.HTTPStatus
		publishedID, txID := outcome.ExternalID, outcome.TransactionID

		if err := ws.SetPublishingJobStatus(r.Context(), workspaceID, job.ID, "succeeded", ""); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "publishing succeeded but job bookkeeping failed; the artifact was not recorded")
			return
		}
		var externalURL *string
		if publicURL != "" {
			externalURL = &publicURL
		}
		if err := ws.RecordPublishedArtifact(r.Context(), job.ID, integrationID, &publishedID, externalURL); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "publishing succeeded but the artifact record failed")
			return
		}
		publishedItem, err := ws.MarkContentPublished(r.Context(), workspaceID, facts.itemID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "publishing succeeded but the item status update failed")
			return
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

// scheduleTimePointer returns the write schema for a scheduled enqueue: both
// scheduled_at and next_attempt_at carry the same value so the due worker scan
// and the history view agree.
func scheduleTimePointer(when time.Time, scheduled bool) *time.Time {
	if !scheduled {
		return nil
	}
	copied := when.UTC()
	return &copied
}

func message(ok bool) string {
	if ok {
		return "Published: the destination accepted the approved content and the public URL answered successfully."
	}
	return "Published: the destination accepted the approved content. The public URL could not be verified from here — open it in a browser to confirm."
}

// derefString returns "" for nil pointers (workspace website URL).
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// writePublisherFailure returns only errors that explicitly declare their
// message safe; unknown adapter errors are never echoed to clients.
func writePublisherFailure(w http.ResponseWriter, err error) {
	var safeErr integration.SafePublisherError
	if errors.As(err, &safeErr) {
		writeJSONError(w, http.StatusBadGateway, safeErr.SafeMessage())
		return
	}
	writeJSONError(w, http.StatusBadGateway, "the publishing destination failed; see server-side diagnostics")
}

func safePublisherErrorText(err error) string {
	var safeErr integration.SafePublisherError
	if errors.As(err, &safeErr) {
		return safeErr.SafeMessage()
	}
	return "the publishing destination failed"
}
