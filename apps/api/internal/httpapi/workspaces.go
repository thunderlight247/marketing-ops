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
	SetContentItemPlatforms(ctx context.Context, workspaceID, itemID string, platforms, mediaURLs []string) (store.ContentItem, error)
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
	ListPublishingJobs(ctx context.Context, workspaceID, itemID string) ([]store.PublishingJob, error)
	CancelPublishingJob(ctx context.Context, workspaceID, itemID, jobID string) (store.PublishingJob, error)
	RetryPublishingJob(ctx context.Context, workspaceID, itemID, jobID string, now time.Time) (store.PublishingJob, error)
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
	// Campaigns and leads (Milestone 11).
	InsertCampaign(ctx context.Context, c store.Campaign) (store.Campaign, error)
	ListCampaigns(ctx context.Context, workspaceID string) ([]store.Campaign, error)
	GetCampaignInWorkspace(ctx context.Context, workspaceID, id string) (store.Campaign, error)
	UpdateCampaign(ctx context.Context, c store.Campaign) (store.Campaign, error)
	InsertLead(ctx context.Context, l store.Lead) (store.Lead, error)
	ListLeads(ctx context.Context, workspaceID string) ([]store.Lead, error)
	GetLeadInWorkspace(ctx context.Context, workspaceID, id string) (store.Lead, error)
	UpdateLead(ctx context.Context, workspaceID, leadID string, in store.LeadUpdate) (store.Lead, error)
	SaveLeadReplyDraft(ctx context.Context, workspaceID, leadID, subject, body string) (store.Lead, error)
	// Reports (Milestone 12, first increment).
	WorkspaceReport(ctx context.Context, workspaceID string, rg store.ReportRange) (store.WorkspaceReport, error)
	// Lead generation engine (per-workspace outreach).
	GetLeadConfig(ctx context.Context, workspaceID string) (store.WorkspaceLeadConfig, error)
	UpsertLeadConfig(ctx context.Context, c store.WorkspaceLeadConfig) (store.WorkspaceLeadConfig, error)
	SetWhatsAppStatus(ctx context.Context, workspaceID, status, phone string) error
	ListLeadEngineLeads(ctx context.Context, workspaceID string) ([]store.Lead, error)
	ListOutreachLogs(ctx context.Context, workspaceID, leadID string) ([]store.LeadOutreachLog, error)
	AddToBlacklist(ctx context.Context, workspaceID, phone, reason string) error
	ListBlacklist(ctx context.Context, workspaceID string) ([]store.LeadBlacklistEntry, error)
	CountDiscoverySearchesToday(ctx context.Context, workspaceID, dayKey string) (int, error)
	RecordDiscoverySearch(ctx context.Context, s store.LeadDiscoverySearch) error
	WasSearchedToday(ctx context.Context, workspaceID, dayKey, category, city string) (bool, error)
	SaveDiscoveredLead(ctx context.Context, workspaceID, placeID, name, phone, category, city, address string, rating float64, reviewCount int, websiteURL string) (store.Lead, error)
	UpdateLeadPitch(ctx context.Context, workspaceID, leadID, pitch string) error
	UpdateLeadStatus(ctx context.Context, workspaceID, leadID, status string) error
	AdvanceLeadCadence(ctx context.Context, workspaceID, leadID string, step int, nextFollowUp *time.Time) error
	IsBlacklisted(ctx context.Context, workspaceID, phone string) (bool, error)
	SaveOutreachLog(ctx context.Context, l store.LeadOutreachLog) (store.LeadOutreachLog, error)
	HasRecentOutboundAttempt(ctx context.Context, workspaceID, leadID string, within time.Duration) (bool, error)
	HasRecentOutboundAttemptByPhone(ctx context.Context, workspaceID, phone string, within time.Duration) (bool, error)
	HasOutboundAttemptByPhone(ctx context.Context, workspaceID, phone string) (bool, error)
	ListRecentOutreach(ctx context.Context, workspaceID string, days int) ([]store.OutreachActivityRow, error)
	ListMessagedLeads(ctx context.Context, workspaceID string) ([]store.MessagedLead, error)
	ResetStaleProcessingLeads(ctx context.Context, workspaceID string) (int64, error)
	CountOutboundSentToday(ctx context.Context, workspaceID string) (int, error)
	ListEnabledLeadConfigs(ctx context.Context) ([]store.WorkspaceLeadConfig, error)
	FindLeadByPhone(ctx context.Context, workspaceID, phone string) (store.Lead, error)
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
	// DeliveryAttention is present only when this workspace has a failed send or
	// a bounced message waiting for a human, so the overview can flag it.
	DeliveryAttention *deliveryAttentionJSON `json:"deliveryAttention,omitempty"`
	// DeliveryAcknowledgedAt is when an operator last dismissed this workspace's
	// delivery problems, so the UI can say what the current badge covers.
	DeliveryAcknowledgedAt *string `json:"deliveryAcknowledgedAt,omitempty"`
}

type deliveryAttentionJSON struct {
	Failed  int `json:"failed"`
	Bounced int `json:"bounced"`
}

// deliveryAttentionStore is the optional slice of the store that counts
// delivery problems per workspace. Stores without it simply report no counts.
type deliveryAttentionStore interface {
	EmailDeliveryAttention(ctx context.Context) (map[string]store.EmailDeliveryAttention, error)
}

// deliveryAcknowledgementStore is the optional slice of the store that reports
// when each workspace last acknowledged its delivery problems.
type deliveryAcknowledgementStore interface {
	EmailDeliveryAcknowledgements(ctx context.Context) (map[string]time.Time, error)
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
		// Delivery problems are a convenience banner, not a fact the list owns:
		// a counting failure must not hide the workspaces themselves.
		attention := map[string]store.EmailDeliveryAttention{}
		if counter, ok := ws.(deliveryAttentionStore); ok {
			if counts, err := counter.EmailDeliveryAttention(r.Context()); err == nil {
				attention = counts
			}
		}
		acknowledged := map[string]time.Time{}
		if acker, ok := ws.(deliveryAcknowledgementStore); ok {
			if times, err := acker.EmailDeliveryAcknowledgements(r.Context()); err == nil {
				acknowledged = times
			}
		}
		out := make([]workspaceJSON, 0, len(workspaces))
		for _, row := range workspaces {
			item := toWorkspaceJSON(row)
			if a := attention[row.ID]; a.Failed > 0 || a.Bounced > 0 {
				item.DeliveryAttention = &deliveryAttentionJSON{Failed: a.Failed, Bounced: a.Bounced}
			}
			if at, ok := acknowledged[row.ID]; ok {
				formatted := at.UTC().Format(time.RFC3339)
				item.DeliveryAcknowledgedAt = &formatted
			}
			out = append(out, item)
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
			audit(r.Context(), ws, store.AuditEvent{
				WorkspaceID: &created.ID,
				Action:      "workspace.created",
				EntityType:  "workspace",
				EntityID:    &created.ID,
				Details:     map[string]any{"slug": created.Slug, "name": created.Name},
			})
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
