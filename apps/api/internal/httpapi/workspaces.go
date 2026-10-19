package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"marketingops/api/internal/store"
)

// WorkspaceStore is what the workspace handlers need. *store.Store implements
// all of it; tests use fakes, so new methods must also be added to the fakes.
type WorkspaceStore interface {
	ListWorkspaces(ctx context.Context) ([]store.Workspace, error)
	InsertWorkspace(ctx context.Context, w store.Workspace) (store.Workspace, error)
	GetWorkspaceByID(ctx context.Context, id string) (store.Workspace, error)
	UpdateWorkspace(ctx context.Context, w store.Workspace) (store.Workspace, error)
	DeleteWorkspace(ctx context.Context, id string) error
	GetBrandProfile(ctx context.Context, workspaceID string) (store.BrandProfile, error)
	UpsertBrandProfile(ctx context.Context, b store.BrandProfile) (store.BrandProfile, error)
	ListContentItems(ctx context.Context, workspaceID string) ([]store.ContentItem, error)
	InsertContentItem(ctx context.Context, item store.ContentItem) (store.ContentItem, error)
	GetContentItemDetail(ctx context.Context, workspaceID, itemID string) (store.ContentItem, *store.ContentVersion, error)
	SaveContentDraft(ctx context.Context, workspaceID, itemID string, in store.ContentDraftInput) (store.ContentItem, store.ContentVersion, error)
	DecideApproval(ctx context.Context, workspaceID, itemID string, in store.ApprovalDecisionInput) (store.ContentItem, store.Approval, error)
	ListApprovalsByItem(ctx context.Context, workspaceID, itemID string) ([]store.Approval, error)
	ListContentVersionsInWorkspace(ctx context.Context, workspaceID, itemID string) ([]store.ContentVersion, error)
	TransitionContentItemStatus(ctx context.Context, workspaceID, itemID, toStatus string) (store.ContentItem, error)
	EnsureOperator(ctx context.Context) (string, error)
	InsertPublishingJob(ctx context.Context, workspaceID string, j store.PublishingJob) (store.PublishingJob, error)
	GetPublishingJobByIdempotencyKey(ctx context.Context, workspaceID, key string) (store.PublishingJob, error)
	SetPublishingJobStatus(ctx context.Context, workspaceID, jobID, status, lastErr string) error
	RecordPublishedArtifact(ctx context.Context, jobID, integrationID string, externalID, externalURL *string) error
	MarkContentPublished(ctx context.Context, workspaceID, itemID string) (store.ContentItem, error)
	ListIntegrations(ctx context.Context, workspaceID string) ([]store.Integration, error)
	InsertIntegration(ctx context.Context, in store.Integration) (store.Integration, error)
	GetIntegrationInWorkspace(ctx context.Context, workspaceID, id string) (store.Integration, error)
	SetIntegrationStatus(ctx context.Context, workspaceID, id, status string) error
	DeleteIntegration(ctx context.Context, workspaceID, id string) error
	UpsertCapability(ctx context.Context, integrationID string, c store.IntegrationCapability) error
	ListCapabilities(ctx context.Context, integrationID string) ([]store.IntegrationCapability, error)
	PutCredentialSet(ctx context.Context, c store.CredentialSet) error
	GetCredentialSet(ctx context.Context, integrationID string) (store.CredentialSet, error)
	DeleteCredentialSet(ctx context.Context, integrationID string) error
	AddAuditEvent(ctx context.Context, e store.AuditEvent) error
}

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}[a-z0-9]$|^[a-z0-9]$`)

const maxWorkspacePayloadBytes = 8 * 1024

// workspaceJSON is the wire form of a workspace.
type workspaceJSON struct {
	ID         string  `json:"id"`
	Slug       string  `json:"slug"`
	Name       string  `json:"name"`
	WebsiteURL *string `json:"websiteUrl"`
	Status     string  `json:"status"`
	CreatedAt  string  `json:"createdAt"`
	UpdatedAt  string  `json:"updatedAt"`
}

func toWorkspaceJSON(w store.Workspace) workspaceJSON {
	return workspaceJSON{
		ID:         w.ID,
		Slug:       w.Slug,
		Name:       w.Name,
		WebsiteURL: w.WebsiteURL,
		Status:     w.Status,
		CreatedAt:  w.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:  w.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// listWorkspacesHandler returns every active workspace.
func listWorkspacesHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		workspaces, err := ws.ListWorkspaces(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "workspace listing failed")
			return
		}
		out := make([]workspaceJSON, 0, len(workspaces))
		for _, ws := range workspaces {
			out = append(out, toWorkspaceJSON(ws))
		}
		writeJSON(w, http.StatusOK, map[string]any{"workspaces": out})
	}
}

// createWorkspaceHandler validates and creates a workspace.
func createWorkspaceHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxWorkspacePayloadBytes+1))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "could not read the request body")
			return
		}
		if len(body) > maxWorkspacePayloadBytes {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request body is too large")
			return
		}

		var req struct {
			Slug       *string `json:"slug"`
			Name       *string `json:"name"`
			WebsiteURL *string `json:"websiteUrl"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "request body is not valid JSON")
			return
		}

		problems := []string{}
		websiteURL := ""
		slug := trim(req.Slug)
		name := trim(req.Name)
		if req.WebsiteURL != nil {
			websiteURL = strings.TrimRight(strings.TrimSpace(*req.WebsiteURL), "/")
		}
		if slug == "" {
			problems = append(problems, "slug is required")
		} else if !slugPattern.MatchString(slug) {
			problems = append(problems, "slug must be lowercase letters, digits, or hyphens")
		}
		if name == "" {
			problems = append(problems, "name is required")
		} else if runeCount(name) > 120 {
			problems = append(problems, "name is too long (max 120 characters)")
		}
		if len(problems) > 0 {
			writeJSONError(w, http.StatusBadRequest, strings.Join(problems, "; "))
			return
		}

		created, err := ws.InsertWorkspace(r.Context(), store.Workspace{
			Slug: slug, Name: name, WebsiteURL: emptyToNil(websiteURL),
		})
		switch {
		case err == nil:
			writeJSON(w, http.StatusCreated, toWorkspaceJSON(created))
		case errors.Is(err, store.ErrSlugTaken):
			writeJSONError(w, http.StatusConflict, "a workspace with that slug already exists")
		case errors.Is(err, store.ErrNotFound):
			// InsertWorkspace maps ON CONFLICT to a sentinel already; unreachable
			// here, but treat it like a conflict rather than crashing.
			writeJSONError(w, http.StatusConflict, "a workspace with that slug already exists")
		default:
			writeJSONError(w, http.StatusInternalServerError, "workspace creation failed")
		}
	}
}

func trim(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

func emptyToNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func runeCount(s string) int { return len([]rune(s)) }

const workspaceUnavailable = "workspaces are not available (no database configured)"
