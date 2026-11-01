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

	"marketingops/api/internal/agents"
	"marketingops/api/internal/aiprovider"
	"marketingops/api/internal/store"
)

// maxAgentPayloadBytes bounds one agent run request, regardless of contract.
const maxAgentPayloadBytes = 8 * 1024

// maxChannelCopyExcerptRunes is how much of the target draft's body enters
// the channel-copy brief: enough to adapt from, far below the payload and
// prompt caps of the pipeline.
const maxChannelCopyExcerptRunes = 1200

// maxLeadReplyEnquiryRunes is how much of a lead's own message enters the
// lead-reply brief: enough to answer what was asked, far below the prompt and
// payload caps of the pipeline.
const maxLeadReplyEnquiryRunes = 1200

// AgentSnapshotLoader supplies one declared data need for one workspace, or
// (any(nil), note, nil) when no connected data exists. Loaders are read-only
// and see only the requested workspace's rows.
type AgentSnapshotLoader interface {
	// Load returns the bounded snapshot, an operator-facing note when the
	// run must proceed without data, and an error only for load-shape bugs
	// (data failures degrade to the note, per the loaders' contract).
	Load(ctx context.Context, workspaceID string) (any, string, error)
}

// gscSnapshotAdaptor exposes the Search Console loader under the generic
// loader interface its data-need key dispatches through.
type gscSnapshotAdaptor struct{ inner GSCLoader }

// AdaptGSCLoader wraps the legacy GSC loader as a generic snapshot loader.
// Exported for cmd wiring; tests inject the adaptor around fakes.
func AdaptGSCLoader(l GSCLoader) AgentSnapshotLoader {
	return gscSnapshotAdaptor{inner: l}
}

func (a gscSnapshotAdaptor) Load(ctx context.Context, workspaceID string) (any, string, error) {
	snap, note, err := a.inner.LoadSnapshot(ctx, workspaceID)
	return snap, note, err
}

// dataNeedNotes returns the operator-facing notes for one data need so the
// possibilities stay a documented set, not scattered literals.
//   - absent: no loader is configured for this need
//   - failed: the loader errored (degraded, like any other failure)
//   - ready: a load succeeded; "" when the loader supplied no note
func dataNeedNotes(need string) (absent, failed, ready string) {
	switch need {
	case agents.DataNeedSearchConsole:
		return "Search Console data loading is not configured on this server",
			"Search Console data could not be loaded for this workspace",
			"Search Console page+query rows included"
	default:
		need = strings.ReplaceAll(need, "_", " ")
		return fmt.Sprintf("%s data loading is not configured on this server", need),
			fmt.Sprintf("%s data could not be loaded for this workspace", need),
			fmt.Sprintf("%s data included", need)
	}
}

// GET /api/v1/agents lists the dashboard-facing descriptors of every enabled
// specialist. It carries no secrets and needs no workspace scope.
func listAgentsHandler(reg *agents.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var list []agents.Descriptor
		if reg != nil {
			list = reg.ListDescriptors()
		}
		if list == nil {
			list = []agents.Descriptor{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"agents": list})
	}
}

// runRequest is the generic agent run payload. kind and contentItemId are
// read by drafting agents (whose output contract files a draft); every field
// is validated per contract before use.
type runRequest struct {
	Kind          *string `json:"kind"`
	Focus         *string `json:"focus"`
	ContentItemID *string `json:"contentItemId"`
	// LeadID selects the existing lead a lead_reply run answers.
	LeadID *string `json:"leadId"`
}

// POST /api/v1/workspaces/{id}/agents/{agentId}/run executes one enabled
// specialist for one workspace through the single shared pipeline:
// payload/focus caps, the agent's declared data loaders, the run timeout,
// the specialist, then any executor its actions license. Unknown agents are
// 404; nothing here publishes or schedules.
func workspaceAgentRunHandler(
	ws WorkspaceStore,
	reg *agents.Registry,
	runner agents.PromptRun,
	loaders map[string]AgentSnapshotLoader,
	timeout time.Duration,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		if reg == nil || runner == nil {
			writeJSONError(w, http.StatusNotImplemented, "agent specialists are not configured on this server")
			return
		}
		agent, ok := reg.Get(r.PathValue("agentId"))
		if !ok {
			writeJSONError(w, http.StatusNotFound, "no such agent")
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
		// A workspace without a saved brand profile is still workable; the
		// specialist just gets less context.
		brand, err := ws.GetBrandProfile(r.Context(), workspaceID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusInternalServerError, "brand profile lookup failed")
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, maxAgentPayloadBytes+1))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "could not read the request body")
			return
		}
		if len(body) > maxAgentPayloadBytes {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request body is too large")
			return
		}
		var req runRequest
		if len(body) > 0 {
			if err := json.Unmarshal(body, &req); err != nil {
				writeJSONError(w, http.StatusBadRequest, "request body is not valid JSON")
				return
			}
		}
		focus := trim(req.Focus)
		if runeCount(focus) > maxGenerateInstrRunes {
			writeJSONError(w, http.StatusBadRequest, "focus is too long (max 2000 characters)")
			return
		}

		// Drafting agents set up their file_draft input before the run: an
		// invalid kind or an inaccessible/frozen content item must fail
		// before any model call is paid for. The content kind is part of the
		// brief, not a separate channel of context.
		draft := draftFiling{}
		runFocus := focus
		var copyFiling channelCopyFiling
		var leadFiling leadReplyFiling
		switch agent.OutputContract() {
		case agents.ContractDrafting:
			prepared, reqErr := prepareDraftFiling(r, ws, workspaceID, req.Kind, req.ContentItemID)
			if reqErr != nil {
				reqErr.respond(w)
				return
			}
			draft = prepared
			if draft.itemLoaded {
				runFocus = "Content kind for this piece: " + draft.item.Kind + "\n" + focus
			} else {
				runFocus = "Content kind for this piece: " + draft.kind + "\n" + focus
			}
		case agents.ContractChannelCopy:
			prepared, reqErr := prepareChannelCopyFiling(r, ws, workspaceID, req.ContentItemID)
			if reqErr != nil {
				reqErr.respond(w)
				return
			}
			copyFiling = prepared
			runFocus = copyFiling.brief + focus
		case agents.ContractLeadReply:
			prepared, reqErr := prepareLeadReplyFiling(r, ws, workspaceID, req.LeadID)
			if reqErr != nil {
				reqErr.respond(w)
				return
			}
			leadFiling = prepared
			runFocus = leadFiling.brief + focus
		}

		runTimeout := timeout
		if runTimeout <= 0 {
			runTimeout = defaultAgentTimeout
		}
		runCtx, cancel := context.WithTimeout(r.Context(), runTimeout)
		defer cancel()

		data, dataNote := loadDeclaredData(agent.WantsData(), loaders, r.Context(), workspaceID)
		runStarted := time.Now()
		output, agentErr := agent.Run(runCtx, runner, agents.RunInput{
			Workspace: workspaceContext(workspace, brand),
			Focus:     runFocus,
			Data:      data,
			LeadEmail: leadFiling.leadEmail(),
		})
		runDuration := time.Since(runStarted)
		if agentErr != nil {
			audit(r.Context(), ws, store.AuditEvent{
				WorkspaceID: &workspaceID,
				Action:      "agent.run.failed",
				Details: map[string]any{
					"agent_id":      agent.ID(),
					"agent_version": agent.Version(),
					"reason_kind":   safeFailureKind(agentErr),
				},
			})
			message := "the agent could not complete this run"
			var providerErr *aiprovider.ProviderError
			if errors.As(agentErr, &providerErr) {
				message = providerErr.Error()
			}
			writeJSONError(w, http.StatusBadGateway, message)
			return
		}

		// The registry is the guard, but the output is checked against the
		// declared contract too: a specialist bug must fail loudly, not show
		// one contract's body as another's.
		switch out := agent.OutputContract(); out {
		case agents.ContractAdvisory:
			if output.Report == nil {
				writeJSONError(w, http.StatusInternalServerError, "the agent returned an unexpected contract")
				return
			}
			respondAdvisory(w, r, ws, workspaceID, output.Report, dataNote, runDuration)
		case agents.ContractDrafting:
			if output.Plan == nil {
				writeJSONError(w, http.StatusInternalServerError, "the agent returned an unexpected contract")
				return
			}
			if !respondFileDraft(w, r, ws, workspaceID, draft, output.Plan, runDuration) {
				return
			}
		case agents.ContractProposal:
			if output.Proposal == nil {
				writeJSONError(w, http.StatusInternalServerError, "the agent returned an unexpected contract")
				return
			}
			if !respondCreateCampaign(w, r, ws, workspaceID, output.Proposal, runDuration) {
				return
			}
		case agents.ContractChannelCopy:
			if output.Copy == nil {
				writeJSONError(w, http.StatusInternalServerError, "the agent returned an unexpected contract")
				return
			}
			if !respondFileChannelCopy(w, r, ws, workspaceID, copyFiling, output.Copy, runDuration) {
				return
			}
		case agents.ContractLeadReply:
			if output.Reply == nil {
				writeJSONError(w, http.StatusInternalServerError, "the agent returned an unexpected contract")
				return
			}
			if !respondFileLeadReply(w, r, ws, workspaceID, leadFiling, output.Reply, runDuration) {
				return
			}
		default:
			writeJSONError(w, http.StatusInternalServerError, "the agent declared an unknown output contract")
		}
	}
}

// loadDeclaredData fetches one snapshot per declared data need. Missing
// loaders and failed loads both degrade to the no-data pack with a note,
// exactly like a loader's own empty result.
func loadDeclaredData(needs []string, loaders map[string]AgentSnapshotLoader, ctx context.Context, workspaceID string) (map[string]any, string) {
	data := make(map[string]any, len(needs))
	var notes []string
	for _, need := range needs {
		absent, failed, ready := dataNeedNotes(need)
		loader, configured := loaders[need]
		if !configured {
			data[need] = nil
			notes = append(notes, absent)
			continue
		}
		snap, note, err := loader.Load(ctx, workspaceID)
		if err != nil {
			data[need] = nil
			notes = append(notes, failed)
			continue
		}
		data[need] = snap
		if strings.TrimSpace(note) != "" {
			notes = append(notes, note)
		} else {
			notes = append(notes, ready)
		}
	}
	return data, strings.Join(notes, "; ")
}

// respondAdvisory audits and renders the advisory contract's envelope. The
// specialist is advisory-only: it never edits, publishes, or schedules.
func respondAdvisory(
	w http.ResponseWriter,
	r *http.Request,
	ws WorkspaceStore,
	workspaceID string,
	report *agents.AdvisorReport,
	dataNote string,
	runDuration time.Duration,
) {
	if dataNote == "" {
		_, _, ready := dataNeedNotes(agents.DataNeedSearchConsole)
		dataNote = ready
	}
	audit(r.Context(), ws, store.AuditEvent{
		WorkspaceID: &workspaceID,
		Action:      "agent.run.completed",
		Details: map[string]any{
			"agent_id":        report.AgentID,
			"agent_version":   report.Version,
			"provider":        report.Provider,
			"model":           report.Model,
			"data_source":     report.DataSource,
			"recommendations": len(report.Recommendations),
			"gaps":            len(report.Gaps),
		},
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"report":     report,
		"dataNote":   dataNote,
		"durationMs": runDuration.Milliseconds(),
	})
}

// draftFiling is the prepared file_draft input for one drafting run: a
// validated kind and, when the operator targets an existing item, the
// already-authorized item row.
type draftFiling struct {
	kind       string
	itemLoaded bool
	item       store.ContentItem
}

// prepareDraftFiling validates the drafting request's kind and, when set,
// the target content item, before any model call is made.
// Without "contentItemId" a new draft content item is created later. With
// one, the plan becomes a new immutable version of that item (only draft,
// approved, or rejected items accept saves — the store enforces it too).
// The kind goes into the run brief; the rest feeds the file_draft executor.
func prepareDraftFiling(r *http.Request, ws WorkspaceStore, workspaceID string,
	kind *string, contentItemID *string,
) (draftFiling, *requestError) {
	d := draftFiling{}
	d.kind = trim(kind)
	if d.kind == "" {
		d.kind = "blog_article"
	} else if !validContentKinds[d.kind] {
		return d, &requestError{status: http.StatusBadRequest,
			message: "kind must be blog_article, social_post, or snippet"}
	}
	itemID := trim(contentItemID)
	if itemID == "" {
		return d, nil
	}
	item, _, err := ws.GetContentItemDetail(r.Context(), workspaceID, itemID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return d, &requestError{status: http.StatusNotFound,
				message: "no such content item in this workspace"}
		}
		return d, &requestError{status: http.StatusInternalServerError,
			message: "content item lookup failed"}
	}
	if item.Status != "draft" && item.Status != "approved" && item.Status != "rejected" {
		return d, &requestError{status: http.StatusConflict,
			message: "the content item is frozen; its text can no longer be replaced"}
	}
	if kind != nil && d.kind != item.Kind {
		return d, &requestError{status: http.StatusBadRequest,
			message: "kind must match the existing content item"}
	}
	d.itemLoaded, d.item = true, item
	return d, nil
}

// respondFileDraft runs the file_draft action: it saves the validated plan
// as a draft in the existing approval workflow and answers with the same
// envelope the content flow expects. It never publishes or schedules.
func respondFileDraft(
	w http.ResponseWriter,
	r *http.Request,
	ws WorkspaceStore,
	workspaceID string,
	draft draftFiling,
	plan *agents.ContentPlan,
	runDuration time.Duration,
) bool {
	var item store.ContentItem
	if draft.itemLoaded {
		item = draft.item
	} else {
		created, err := ws.InsertContentItem(r.Context(), store.ContentItem{
			WorkspaceID: workspaceID, Kind: draft.kind, Title: plan.Title,
		})
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "content creation failed")
			return false
		}
		item = created
	}

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
			return false
		}
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "no such content item in this workspace")
			return false
		}
		writeJSONError(w, http.StatusInternalServerError, "storing the created draft failed")
		return false
	}

	audit(r.Context(), ws, store.AuditEvent{
		WorkspaceID: &workspaceID,
		Action:      "agent.action.file_draft.completed",
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
		SlugDrift:  emptyToNil(plan.Slug),
	})
	return true
}

// channelCopyFiling is the prepared file_channel_copy input for one run: the
// already-authorized target item, the channels its newest version already
// carries (new copy merges over these, never drops them), and the bounded
// brief the specialist adapts from. The run never creates an item; it always
// targets an existing one.
type channelCopyFiling struct {
	item store.ContentItem
	// existingChannels is the newest version's channel-copy map, including
	// copies written by humans or earlier runs; the executor only adds keys.
	existingChannels map[string]string
	brief            string // title and capped body excerpt, rendered once, before the run
}

// prepareChannelCopyFiling validates the file_channel_copy request before any
// model call: a target item is required, must exist in this workspace, and
// must be in status "draft". This is deliberately stricter than the ordinary
// draft-save rule: filing copy into an approved or rejected item would reset
// it to draft and invalidate a human decision, and no agent run does that —
// a status change stays an operator action.
func prepareChannelCopyFiling(
	r *http.Request, ws WorkspaceStore, workspaceID string, contentItemID *string,
) (channelCopyFiling, *requestError) {
	itemID := trim(contentItemID)
	if itemID == "" {
		return channelCopyFiling{}, &requestError{status: http.StatusBadRequest,
			message: "contentItemId is required for this agent"}
	}
	item, latest, err := ws.GetContentItemDetail(r.Context(), workspaceID, itemID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return channelCopyFiling{}, &requestError{status: http.StatusNotFound,
				message: "no such content item in this workspace"}
		}
		return channelCopyFiling{}, &requestError{status: http.StatusInternalServerError,
			message: "content item lookup failed"}
	}
	if item.Status != "draft" {
		return channelCopyFiling{}, &requestError{status: http.StatusConflict,
			message: "the channel copy files only into draft items; move the item to draft first"}
	}

	// Carried-forward channels from the newest version, when one exists.
	existing := make(map[string]string)
	if latest != nil {
		for name, value := range latest.ChannelCopy {
			if message, ok := value.(string); ok && name != "" {
				existing[name] = message
			}
		}
	}

	// The brief carries the item's proven text only: its title and a capped
	// excerpt of the newest version's body, when one exists.
	var out strings.Builder
	title := strings.TrimSpace(item.Title)
	if title == "" {
		title = "(untitled)"
	}
	fmt.Fprintf(&out, "Draft to adapt — Title: %s\n", title)
	if latest != nil && strings.TrimSpace(latest.Body) != "" {
		excerpt := strings.TrimSpace(latest.Body)
		if runeCount(excerpt) > maxChannelCopyExcerptRunes {
			runes := []rune(excerpt)
			excerpt = string(runes[:maxChannelCopyExcerptRunes]) + "…"
		}
		fmt.Fprintf(&out, "Body excerpt: %s\n", excerpt)
	}
	return channelCopyFiling{item: item, existingChannels: existing, brief: out.String()}, nil
}

// respondFileChannelCopy runs the file_channel_copy action: it saves the
// validated per-channel copy into the target item through the ordinary
// draft-save path (a new immutable version). New copy merges over the
// newest version's existing channels — a run only ever adds, never drops —
// and the item's proven text (title, excerpt, body, call to action) carries
// forward via the store's nil handling, so the run replaces nothing else and
// stamps agent provenance into the version metadata. It never creates
// items, publishes, or schedules.
func respondFileChannelCopy(
	w http.ResponseWriter,
	r *http.Request,
	ws WorkspaceStore,
	workspaceID string,
	filing channelCopyFiling,
	plan *agents.ChannelCopyPlan,
	runDuration time.Duration,
) bool {
	merged := make(map[string]any, len(filing.existingChannels)+len(plan.Channels))
	for name, message := range filing.existingChannels {
		merged[name] = message
	}
	for name, message := range plan.Channels {
		merged[name] = message
	}
	saved, version, err := ws.SaveContentDraft(r.Context(), workspaceID, filing.item.ID, store.ContentDraftInput{
		Title:       filing.item.Title, // carried forward, not agent-authored
		ChannelCopy: merged,
		Metadata: map[string]any{
			"source":       "agent",
			"agentId":      plan.AgentID,
			"agentVersion": plan.Version,
			"provider":     plan.Provider,
			"model":        plan.Model,
		},
	})
	if err != nil {
		if errors.Is(err, store.ErrNotEditable) {
			writeJSONError(w, http.StatusConflict,
				"the content item is frozen (review, queued, publishing, or published); its text can no longer be replaced")
			return false
		}
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "no such content item in this workspace")
			return false
		}
		writeJSONError(w, http.StatusInternalServerError, "storing the channel copy failed")
		return false
	}

	audit(r.Context(), ws, store.AuditEvent{
		WorkspaceID: &workspaceID,
		Action:      "agent.action.file_channel_copy.completed",
		EntityType:  "content_item",
		EntityID:    &filing.item.ID,
		Details: map[string]any{
			"id":            filing.item.ID,
			"kind":          filing.item.Kind,
			"agent_id":      plan.AgentID,
			"agent_version": plan.Version,
			"provider":      plan.Provider,
			"model":         plan.Model,
			"channels":      len(plan.Channels),
		},
	})

	writeJSON(w, http.StatusCreated, struct {
		Item       contentItemJSON    `json:"item"`
		Version    contentVersionJSON `json:"version"`
		Provider   string             `json:"provider"`
		Model      string             `json:"model"`
		AgentID    string             `json:"agentId"`
		Channels   map[string]string  `json:"channels"`
		DurationMs int64              `json:"durationMs"`
	}{
		Item:       toContentItemJSON(saved),
		Version:    toContentVersionJSON(version),
		Provider:   plan.Provider,
		Model:      plan.Model,
		AgentID:    plan.AgentID,
		Channels:   plan.Channels,
		DurationMs: runDuration.Milliseconds(),
	})
	return true
}

// leadReplyFiling is the prepared file_lead_reply input for one run: the
// already-authorized target lead and the bounded brief the specialist answers.
// The run never creates a lead; it always answers an existing enquiry.
type leadReplyFiling struct {
	lead  store.Lead
	brief string // the lead's own bounded facts, rendered once, before the run
}

// leadEmail returns the target lead's own address, or "" when none was
// recorded. Replies may contain that address only.
func (f leadReplyFiling) leadEmail() string {
	if f.lead.ContactEmail == nil {
		return ""
	}
	return *f.lead.ContactEmail
}

// prepareLeadReplyFiling validates the file_lead_reply request before any
// model call: a target lead is required, must exist in this workspace, must
// not be decided yet (won/lost), and must carry something to answer (an
// enquiry or at least a name). Drafting a "first reply" to a decided lead, or
// to a lead with nothing recorded, would produce text grounded in nothing.
func prepareLeadReplyFiling(
	r *http.Request, ws WorkspaceStore, workspaceID string, leadID *string,
) (leadReplyFiling, *requestError) {
	id := trim(leadID)
	if id == "" {
		return leadReplyFiling{}, &requestError{status: http.StatusBadRequest,
			message: "leadId is required for this agent"}
	}
	lead, err := ws.GetLeadInWorkspace(r.Context(), workspaceID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return leadReplyFiling{}, &requestError{status: http.StatusNotFound,
				message: "no such lead in this workspace"}
		}
		return leadReplyFiling{}, &requestError{status: http.StatusInternalServerError,
			message: "lead lookup failed"}
	}
	if lead.Status == "won" || lead.Status == "lost" {
		return leadReplyFiling{}, &requestError{status: http.StatusConflict,
			message: "this lead is already decided (won or lost); move it back to an open status before drafting a reply"}
	}
	message := ""
	if lead.Message != nil {
		message = strings.TrimSpace(*lead.Message)
	}
	name := ""
	if lead.ContactName != nil {
		name = strings.TrimSpace(*lead.ContactName)
	}
	if message == "" && name == "" {
		return leadReplyFiling{}, &requestError{status: http.StatusBadRequest,
			message: "this lead has no enquiry text or contact name to reply to"}
	}

	var out strings.Builder
	fmt.Fprintf(&out, "Reply to this lead\nRecorded status: %s\nSource: %s", lead.Status, lead.SourceKind)
	if strings.TrimSpace(lead.SourceDetail) != "" {
		fmt.Fprintf(&out, " (%s)", strings.TrimSpace(lead.SourceDetail))
	}
	out.WriteString("\n")
	if name != "" {
		fmt.Fprintf(&out, "Contact name: %s\n", name)
	}
	if email := lead.ContactEmail; email != nil && strings.TrimSpace(*email) != "" {
		fmt.Fprintf(&out, "Contact email: %s\n", strings.TrimSpace(*email))
	}
	if message != "" {
		if runeCount(message) > maxLeadReplyEnquiryRunes {
			message = string([]rune(message)[:maxLeadReplyEnquiryRunes]) + "…"
		}
		fmt.Fprintf(&out, "Enquiry (the lead's own words): %s\n", message)
	}
	if lead.FollowUpAt != nil {
		fmt.Fprintf(&out, "Follow-up already scheduled for: %s\n", lead.FollowUpAt.UTC().Format(time.RFC3339))
	}
	return leadReplyFiling{lead: lead, brief: out.String()}, nil
}

// respondFileLeadReply runs the file_lead_reply action: it stores the
// validated draft on the target lead for the operator to review, edit, and
// send themselves. Nothing is sent here, no lead is created or deleted, and
// the lead's status, source, attribution, and notes are untouched — only a
// person can record that a lead was contacted.
func respondFileLeadReply(
	w http.ResponseWriter,
	r *http.Request,
	ws WorkspaceStore,
	workspaceID string,
	filing leadReplyFiling,
	reply *agents.LeadReply,
	runDuration time.Duration,
) bool {
	saved, err := ws.SaveLeadReplyDraft(r.Context(), workspaceID, filing.lead.ID, reply.Subject, reply.Body)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "no such lead in this workspace")
			return false
		}
		if errors.Is(err, store.ErrNameRequired) {
			writeJSONError(w, http.StatusBadGateway, "the drafted reply was missing a subject or body")
			return false
		}
		writeJSONError(w, http.StatusInternalServerError, "storing the drafted reply failed")
		return false
	}

	audit(r.Context(), ws, store.AuditEvent{
		WorkspaceID: &workspaceID,
		Action:      "agent.action.file_lead_reply.completed",
		EntityType:  "lead",
		EntityID:    &filing.lead.ID,
		Details: map[string]any{
			"id":              filing.lead.ID,
			"status":          saved.Status, // unchanged by this action
			"agent_id":        reply.AgentID,
			"agent_version":   reply.Version,
			"provider":        reply.Provider,
			"model":           reply.Model,
			"subject_runes":   runeCount(reply.Subject),
			"body_runes":      runeCount(reply.Body),
			"has_enquiry":     filing.lead.Message != nil && strings.TrimSpace(*filing.lead.Message) != "",
			"campaign_linked": filing.lead.CampaignID != nil,
		},
	})

	writeJSON(w, http.StatusCreated, struct {
		Lead       leadJSON `json:"lead"`
		Subject    string   `json:"subject"`
		Body       string   `json:"body"`
		Provider   string   `json:"provider"`
		Model      string   `json:"model"`
		AgentID    string   `json:"agentId"`
		DurationMs int64    `json:"durationMs"`
	}{
		Lead:       toLeadJSON(saved),
		Subject:    reply.Subject,
		Body:       reply.Body,
		Provider:   reply.Provider,
		Model:      reply.Model,
		AgentID:    reply.AgentID,
		DurationMs: runDuration.Milliseconds(),
	})
	return true
}

// requestError is a pre-run validation failure with its HTTP rendering.
type requestError struct {
	status  int
	message string
}

func (e *requestError) Error() string { return e.message }

func (e *requestError) respond(w http.ResponseWriter) {
	writeJSONError(w, e.status, e.message)
}

// respondCreateCampaign runs the create_campaign action: it files the
// validated proposal as one campaign in the existing campaigns store, always
// with status "planned". The proposal never arrives active, and every later
// activation, edit, or deletion stays an operator action in the ordinary
// flow: nothing here publishes, schedules, or writes anywhere else.
func respondCreateCampaign(
	w http.ResponseWriter,
	r *http.Request,
	ws WorkspaceStore,
	workspaceID string,
	proposal *agents.CampaignProposal,
	runDuration time.Duration,
) bool {
	created, err := ws.InsertCampaign(r.Context(), store.Campaign{
		WorkspaceID: workspaceID,
		Name:        proposal.Name,
		Status:      "planned", // forced: the operator alone activates a proposal
		UtmCampaign: proposal.UtmCampaign,
		Notes:       proposal.Notes,
	})
	switch {
	case err == nil:
		audit(r.Context(), ws, store.AuditEvent{
			WorkspaceID: &workspaceID,
			Action:      "agent.action.create_campaign.completed",
			EntityType:  "campaign",
			EntityID:    &created.ID,
			Details: map[string]any{
				"name":          created.Name,
				"status":        created.Status,
				"agent_id":      proposal.AgentID,
				"agent_version": proposal.Version,
				"provider":      proposal.Provider,
				"model":         proposal.Model,
				"has_utm":       proposal.UtmCampaign != "",
				"keypoints":     len(proposal.KeyPoints),
			},
		})
		writeJSON(w, http.StatusCreated, struct {
			Campaign   campaignJSON `json:"campaign"`
			Provider   string       `json:"provider"`
			Model      string       `json:"model"`
			AgentID    string       `json:"agentId"`
			KeyPoints  []string     `json:"keyPoints"`
			DurationMs int64        `json:"durationMs"`
		}{
			Campaign:   toCampaignJSON(created),
			Provider:   proposal.Provider,
			Model:      proposal.Model,
			AgentID:    proposal.AgentID,
			KeyPoints:  proposal.KeyPoints,
			DurationMs: runDuration.Milliseconds(),
		})
		return true
	case errors.Is(err, store.ErrSlugTaken):
		// Same non-fatal posture as the manual form: the operator can rename
		// and re-run, and no partial record exists to clean up.
		writeJSONError(w, http.StatusConflict,
			"a campaign with that name already exists in this workspace")
	case errors.Is(err, store.ErrNameRequired):
		writeJSONError(w, http.StatusBadGateway, "the proposal was missing a campaign name")
	case errors.Is(err, store.ErrInvalidStatus):
		// Unreachable while the executor forces "planned"; guards a future
		// refactor from silently activating agent proposals.
		writeJSONError(w, http.StatusInternalServerError, "the proposal filing failed an internal status check")
	default:
		writeJSONError(w, http.StatusInternalServerError, "storing the campaign proposal failed")
	}
	return false
}
