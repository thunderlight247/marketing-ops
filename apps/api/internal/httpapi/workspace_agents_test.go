package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"marketingops/api/internal/agents"
	"marketingops/api/internal/aiprovider"
	"marketingops/api/internal/store"
)

const agentWsID = "11111111-1111-1111-1111-111111111111"

// fakeAgentRunner records the prompts it is asked to transport.
type fakeAgentRunner struct {
	text string
	fail error

	calls []aiprovider.RunPromptRequest
}

func (f *fakeAgentRunner) RunPrompt(ctx context.Context, req aiprovider.RunPromptRequest) (aiprovider.PromptReply, error) {
	f.calls = append(f.calls, req)
	if f.fail != nil {
		return aiprovider.PromptReply{}, f.fail
	}
	return aiprovider.PromptReply{Provider: "fake", Model: "fake-1", Text: f.text}, nil
}

// fakeGSCLoader answers with a canned snapshot and records the workspace ids
// it was asked about.
type fakeGSCLoader struct {
	snapshot *agents.GSCSnapshot
	note     string
	err      error
	calls    []string
}

func (f *fakeGSCLoader) LoadSnapshot(ctx context.Context, workspaceID string) (*agents.GSCSnapshot, string, error) {
	f.calls = append(f.calls, workspaceID)
	return f.snapshot, f.note, f.err
}

// advisorReplyJSON builds a report JSON as a model would reply.
func advisorReplyJSON(dataSource string, checked bool) string {
	checkedText := "false"
	if checked {
		checkedText = "true"
	}
	return `{"dataSource":"` + dataSource + `","cannibalizationChecked":` + checkedText +
		`,"summary":"s","gaps":[],"recommendations":[]}`
}

func gscDataMarkerNone() string { return "\nGSC_DATA_AVAILABILITY: none" }
func gscDataMarkerGSC() string  { return "\nGSC_DATA_AVAILABILITY: search_console" }

// agentRunPath is the single generic endpoint every specialist answers.
func agentRunPath(wsID, agentID string) string {
	return "/api/v1/workspaces/" + wsID + "/agents/" + agentID + "/run"
}

func postAdvisory(h http.Handler, wsID, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, agentRunPath(wsID, agents.SEOAdvisorID), strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// gscLoaderDeps maps a GSC loader to the declared data-need key, as cmd
// wiring does for the real loader.
func gscLoaderDeps(loader GSCLoader) map[string]AgentSnapshotLoader {
	return map[string]AgentSnapshotLoader{agents.DataNeedSearchConsole: AdaptGSCLoader(loader)}
}

func TestAgentRunRunnerNotConfigured(t *testing.T) {
	h := NewRouter(Deps{Logger: testLogger, Workspaces: workspaceDeps(nil, nil)})
	rec := postAdvisory(h, agentWsID, "")
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 without a runner, got %d", rec.Code)
	}
}

func TestAgentRunUnknownWorkspace(t *testing.T) {
	h := NewRouter(Deps{Logger: testLogger, Workspaces: workspaceDeps(nil, nil), AgentRunner: &fakeAgentRunner{}})
	rec := postAdvisory(h, "00000000-0000-0000-0000-000000000000", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// TestAgentRunUnknownAgent verifies agents outside the registry never run:
// the route cannot be pointed at a nonexistent profile.
func TestAgentRunUnknownAgent(t *testing.T) {
	h := NewRouter(Deps{Logger: testLogger, Workspaces: workspaceDeps(nil, nil), AgentRunner: &fakeAgentRunner{}})
	req := httptest.NewRequest(http.MethodPost, agentRunPath(agentWsID, "not_registered"), strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown agent, got %d", rec.Code)
	}
	rec = httptest.NewRecorder() // legacy per-specialist path must also be gone
	req = httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+agentWsID+"/agents/content-creator/draft", strings.NewReader("{}"))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("legacy agent route still answers: %d", rec.Code)
	}
}

// gapWriterReplyJSON builds a search-gap-writer plan reply as a model would.
func gapWriterReplyJSON(targetQuery string, checked bool) string {
	_ = checked // no cannibalization gate here; the plan schema has no such field
	return `
{"title":"Winter maintenance guide","slug":"winter-maintenance-guide","excerpt":"Prep for winter.",` +
		`"body":"## Winter maintenance\n\nPlan ahead.","callToAction":"Contact us.",` +
		`"keyPoints":["Pick a window"],"targetQuery":"` + targetQuery + `"}`
}
func TestAgentDescriptorsEndpoint(t *testing.T) {
	h := NewRouter(Deps{Logger: testLogger})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var parsed struct {
		Agents []agents.Descriptor `json:"agents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("body is not the descriptor shape: %v", err)
	}
	if len(parsed.Agents) != 6 {
		t.Fatalf("expected the six built-in specialists, got %d: %+v", len(parsed.Agents), parsed.Agents)
	}
	byID := map[string]agents.Descriptor{}
	for _, d := range parsed.Agents {
		byID[d.ID] = d
	}
	if byID[agents.SEOAdvisorID].OutputContract != agents.ContractAdvisory ||
		byID[agents.SEOAdvisorID].Version == "" || len(byID[agents.SEOAdvisorID].DataNeeds) != 1 {
		t.Fatalf("unexpected seo descriptor: %+v", byID[agents.SEOAdvisorID])
	}
	if byID[agents.ContentCreatorID].OutputContract != agents.ContractDrafting ||
		len(byID[agents.ContentCreatorID].Actions) != 1 ||
		byID[agents.ContentCreatorID].Actions[0] != agents.ActionFileDraft {
		t.Fatalf("unexpected creator descriptor: %+v", byID[agents.ContentCreatorID])
	}
	// The pluggable layer: the gap writer appeared through registration only,
	// with no handler, route, loader, or client of its own.
	if byID[agents.SearchGapWriterID].OutputContract != agents.ContractDrafting ||
		len(byID[agents.SearchGapWriterID].DataNeeds) != 1 ||
		byID[agents.SearchGapWriterID].DataNeeds[0] != agents.DataNeedSearchConsole ||
		len(byID[agents.SearchGapWriterID].Actions) != 1 {
		t.Fatalf("unexpected gap-writer descriptor: %+v", byID[agents.SearchGapWriterID])
	}
	// The fourth specialist brings the second executor class: the proposal
	// contract with the create_campaign action and no data needs.
	if byID[agents.CampaignStrategistID].OutputContract != agents.ContractProposal ||
		byID[agents.CampaignStrategistID].Version != agents.CampaignStrategistVersion ||
		len(byID[agents.CampaignStrategistID].DataNeeds) != 0 ||
		len(byID[agents.CampaignStrategistID].Actions) != 1 ||
		byID[agents.CampaignStrategistID].Actions[0] != agents.ActionCreateCampaign {
		t.Fatalf("unexpected strategist descriptor: %+v", byID[agents.CampaignStrategistID])
	}
	// The fifth specialist brings the third executor class: the channel-copy
	// contract with the file_channel_copy action and no data needs.
	if byID[agents.ChannelCopyWriterID].OutputContract != agents.ContractChannelCopy ||
		byID[agents.ChannelCopyWriterID].Version != agents.ChannelCopyWriterVersion ||
		len(byID[agents.ChannelCopyWriterID].DataNeeds) != 0 ||
		len(byID[agents.ChannelCopyWriterID].Actions) != 1 ||
		byID[agents.ChannelCopyWriterID].Actions[0] != agents.ActionFileChannelCopy {
		t.Fatalf("unexpected channel-copy descriptor: %+v", byID[agents.ChannelCopyWriterID])
	}
	// The sixth specialist brings the fourth executor class: the lead-reply
	// contract with the file_lead_reply action and no data needs.
	if byID[agents.LeadResponderID].OutputContract != agents.ContractLeadReply ||
		byID[agents.LeadResponderID].Version != agents.LeadResponderVersion ||
		len(byID[agents.LeadResponderID].DataNeeds) != 0 ||
		len(byID[agents.LeadResponderID].Actions) != 1 ||
		byID[agents.LeadResponderID].Actions[0] != agents.ActionFileLeadReply {
		t.Fatalf("unexpected lead-responder descriptor: %+v", byID[agents.LeadResponderID])
	}
}

// TestSearchGapWriterRunThroughGenericRoute proves the add-one-agent claim:
// the third specialist runs (and files a draft) through the same generic
// route, loader wiring, and audit pipeline with zero new httpapi code.
func TestSearchGapWriterRunThroughGenericRoute(t *testing.T) {
	ws := workspaceDeps(nil, nil)
	// No loaders configured: the run must degrade to no-data honestly, and
	// the reply therefore must not name any target query.
	runner := &fakeAgentRunner{text: creatorReply}
	h := NewRouter(Deps{Logger: testLogger, Workspaces: ws, AgentRunner: runner})
	rec := doJSON(t, h, http.MethodPost, agentRunPath(agentWsID, agents.SearchGapWriterID),
		`{"kind":"blog_article","focus":"Fix seasonal gaps"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	req := runner.calls[0]
	if !strings.HasSuffix(req.User, gscDataMarkerNone()) {
		t.Fatalf("missing honest no-data marker:\n%s", req.User)
	}
	if !strings.Contains(req.User, "Brief for this piece: Content kind for this piece: blog_article") {
		t.Fatalf("drafting brief missing:\n%s", req.User)
	}
	if len(ws.auditEvents) != 1 || ws.auditEvents[0].Action != "agent.action.file_draft.completed" {
		t.Fatalf("unexpected audit: %+v", ws.auditEvents)
	}

	// The offline stub must also work end to end (dev mode without a key).
	h = NewRouter(Deps{Logger: testLogger, Workspaces: workspaceDeps(nil, nil), AgentRunner: aiprovider.StubProvider{}})
	rec = doJSON(t, h, http.MethodPost, agentRunPath(agentWsID, agents.SearchGapWriterID), `{}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("offline stub status %d: %s", rec.Code, rec.Body.String())
	}
}

// TestSearchGapWriterDataHonesty verifies the target query can only be one
// of the rows actually provided to the run, enforced when data was loaded.
func TestSearchGapWriterDataHonesty(t *testing.T) {
	rows := &agents.GSCSnapshot{
		Property: "sc-domain:demo.example", StartDate: "2026-09-24", EndDate: "2026-10-22",
		PageQueries: []agents.GSCPageQueryRow{{
			Page: "/a", Query: "coffee", Clicks: 1, Impressions: 200, Ctr: 0.005, Position: 12,
		}},
	}
	for _, tc := range []struct {
		name, reply string
		invented    bool
	}{
		{"verbatim provided query", gapWriterReplyJSON("coffee", true), false},
		{"reworded query", gapWriterReplyJSON("Coffee", true), true},
		{"fabricated query", gapWriterReplyJSON("espresso machine sales", true), true},
		{"missing with rows", `{"title":"t","body":"b"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := agents.SearchGapWriter{}
			_, err := w.Run(context.Background(), &fakeAgentRunner{text: tc.reply}, agents.RunInput{
				Data: map[string]any{agents.DataNeedSearchConsole: rows},
			})
			if tc.invented && err == nil {
				t.Fatal("a target query outside the provided rows must be rejected")
			}
			if !tc.invented && err != nil {
				t.Fatalf("valid reply rejected: %v", err)
			}
		})
	}
	// With no rows, a reply naming any query must be rejected...
	w := agents.SearchGapWriter{}
	if _, err := w.Run(context.Background(), &fakeAgentRunner{text: gapWriterReplyJSON("coffee", true)}, agents.RunInput{}); err == nil {
		t.Fatal("query named without rows must be rejected")
	}
	// ...and a no-data reply without a query must pass with the first key
	// point untouched.
	out, err := w.Run(context.Background(), &fakeAgentRunner{text: creatorReply}, agents.RunInput{})
	if err != nil {
		t.Fatalf("no-data plan rejected: %v", err)
	}
	if strings.Contains(strings.Join(out.Plan.KeyPoints, "|"), agents.TargetQueryKeyPointPrefix) {
		t.Fatalf("no-data plan must not gain a target-query marker: %+v", out.Plan.KeyPoints)
	}
	if out.Plan.AgentID != agents.SearchGapWriterID {
		t.Fatalf("plan identity missing: %+v", out.Plan)
	}

	// Rows + verbatim query: the marker lands as the first key point.
	rec, err := w.Run(context.Background(), &fakeAgentRunner{text: gapWriterReplyJSON("coffee", true)}, agents.RunInput{
		Data: map[string]any{agents.DataNeedSearchConsole: rows},
	})
	if err != nil {
		t.Fatalf("data-informed plan rejected: %v", err)
	}
	if len(rec.Plan.KeyPoints) == 0 || rec.Plan.KeyPoints[0] != agents.TargetQueryKeyPointPrefix+"coffee" {
		t.Fatalf("target-query evidence point missing: %+v", rec.Plan.KeyPoints)
	}
}

// strategistReplyJSON builds a campaign-strategist proposal reply as a
// model would.
func strategistReplyJSON(name, utm string) string {
	return `{"name":"` + name + `","utmCampaign":"` + utm + `",` +
		`"notes":"Slow-season reminder angle. [to confirm: schedule]","keyPoints":["Reminder angle"]}`
}

// TestCampaignStrategistRunThroughGenericRoute proves the second executor
// class: one generic route, one pipeline, and the create_campaign action
// filing a planned campaign with the forced status and its own audit event.
func TestCampaignStrategistRunThroughGenericRoute(t *testing.T) {
	fake := workspaceDeps(nil, nil)
	runner := &fakeAgentRunner{text: strategistReplyJSON("Winter service reminder", "winter-service-2026")}
	h := NewRouter(Deps{Logger: testLogger, Workspaces: fake, AgentRunner: runner})
	rec := doJSON(t, h, http.MethodPost, agentRunPath(agentWsID, agents.CampaignStrategistID),
		`{"focus":"Plan for the slow season"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	// The executor filed exactly one campaign, scoped to the workspace and
	// forced to "planned", with the validated proposal fields.
	if len(fake.insertedCampaigns) != 1 {
		t.Fatalf("campaigns filed: %d", len(fake.insertedCampaigns))
	}
	filed := fake.insertedCampaigns[0]
	if filed.WorkspaceID != agentWsID || filed.Status != "planned" ||
		filed.Name != "Winter service reminder" || filed.UtmCampaign != "winter-service-2026" {
		t.Fatalf("unexpected filed campaign: %+v", filed)
	}
	if !strings.Contains(filed.Notes, "[to confirm: schedule]") {
		t.Fatalf("rationale lost: %q", filed.Notes)
	}

	// The reply envelope matches the proposal contract's dashboard shape.
	var parsed struct {
		Campaign   campaignJSON `json:"campaign"`
		Provider   string       `json:"provider"`
		AgentID    string       `json:"agentId"`
		KeyPoints  []string     `json:"keyPoints"`
		DurationMS int64        `json:"durationMs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("body is not the proposal envelope: %v", err)
	}
	if parsed.Campaign.ID == "" || parsed.Campaign.Status != "planned" || parsed.AgentID != agents.CampaignStrategistID {
		t.Fatalf("unexpected envelope: %+v", parsed)
	}
	if parsed.Provider != "fake" {
		t.Fatalf("provider lost: %q", parsed.Provider)
	}

	// Task history: one completed action event with the forced status on it.
	if len(fake.auditEvents) != 1 {
		t.Fatalf("expected one audit event, got %d", len(fake.auditEvents))
	}
	audit := fake.auditEvents[0]
	if audit.Action != "agent.action.create_campaign.completed" ||
		audit.EntityType != "campaign" || audit.EntityID == nil {
		t.Fatalf("unexpected audit event: %+v", audit)
	}
	if audit.Details["status"] != "planned" || audit.Details["agent_id"] != agents.CampaignStrategistID {
		t.Fatalf("audit details lost the proposal facts: %+v", audit.Details)
	}

	// The offline stub must also work end to end (dev mode without a key).		h = NewRouter(Deps{Logger: testLogger, Workspaces: workspaceDeps(nil, nil), AgentRunner: aiprovider.StubProvider{}})
	rec = doJSON(t, h, http.MethodPost, agentRunPath(agentWsID, agents.CampaignStrategistID), `{}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("offline stub status %d: %s", rec.Code, rec.Body.String())
	}
}

// TestCampaignStrategistFilingErrors maps the store disagreements onto the
// same responses the manual campaign form produces.
func TestCampaignStrategistFilingErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		errors error
		status int
	}{{"duplicate name", store.ErrSlugTaken, http.StatusConflict},
		{"name rejected", store.ErrNameRequired, http.StatusBadGateway},
		{"store failure", errors.New("boom"), http.StatusInternalServerError}} {
		t.Run(tc.name, func(t *testing.T) {
			fake := workspaceDeps(nil, nil)
			fake.insertCampaignErr = tc.errors
			runner := &fakeAgentRunner{text: strategistReplyJSON("Any name", "")}
			h := NewRouter(Deps{Logger: testLogger, Workspaces: fake, AgentRunner: runner})
			rec := doJSON(t, h, http.MethodPost, agentRunPath(agentWsID, agents.CampaignStrategistID), `{}`)
			if rec.Code != tc.status {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if tc.status == http.StatusInternalServerError && strings.Contains(rec.Body.String(), "boom") {
				t.Fatal("internal error echoed raw store text")
			}
		})
	}
}

// channelCopyReplyJSON builds a channel-copy plan reply as a model would.
func channelCopyReplyJSON() string {
	return `{"channelCopy":{"facebook":"Open all winter. Book November service now.","x":"Winter service. Book today."}}`
}

// channelCopyFixture is a draft item with a written body, the shape the
// channel-copy specialist always targets.
func channelCopyFixture(fake *fakeWorkspaceStore) string {
	itemID := "22222222-2222-2222-2222-222222222222"
	fake.contentItemsByID = map[string]store.ContentItem{
		itemID: {ID: itemID, WorkspaceID: agentWsID, Kind: "social_post", Title: "Winter service", Status: "draft"},
	}
	fake.contentVersionsByItem = map[string][]store.ContentVersion{
		itemID: {{ID: "44444444-4444-4444-4444-444444444444", ContentItemID: itemID, VersionNumber: 1,
			Title: "Winter service", Body: "Booking is now open for November furnace checks.",
			ChannelCopy: map[string]any{"youtube": "Existing channel description."},
			Metadata:    map[string]any{"previous": "version"}}},
	}
	return itemID
}

// TestChannelCopyWriterRunThroughGenericRoute proves the third executor
// class: one generic route, the pre-run item validation, and the
// file_channel_copy action filing into the target item's newest version
// with its own audit event — never creating an item.
func TestChannelCopyWriterRunThroughGenericRoute(t *testing.T) {
	fake := workspaceDeps(nil, nil)
	itemID := channelCopyFixture(fake)
	runner := &fakeAgentRunner{text: channelCopyReplyJSON()}
	h := NewRouter(Deps{Logger: testLogger, Workspaces: fake, AgentRunner: runner})
	rec := doJSON(t, h, http.MethodPost, agentRunPath(agentWsID, agents.ChannelCopyWriterID),
		`{"contentItemId":"`+itemID+`","focus":"Emphasize the booking deadline"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	// The specialist saw the item's proven text, capped, plus the focus —
	// and no other tool surface.
	req := runner.calls[0]
	if !strings.Contains(req.User, "Draft to adapt — Title: Winter service") {
		t.Fatalf("brief lost the item title:\n%s", req.User)
	}
	if !strings.Contains(req.User, "Body excerpt: Booking is now open for November furnace checks.") {
		t.Fatalf("brief lost the body excerpt:\n%s", req.User)
	}
	if !strings.Contains(req.User, "Emphasize the booking deadline") {
		t.Fatalf("brief lost the focus:\n%s", req.User)
	}

	// Exactly one draft-save call on the target item, no new items created,
	// with the copy map and agent provenance — and the item's proven text
	// carried forward (nil excerpt/body/CTA, title kept).
	if len(fake.savedDrafts) != 1 || len(fake.insertedContent) != 0 {
		t.Fatalf("unexpected filing calls: saves %d, inserts %d", len(fake.savedDrafts), len(fake.insertedContent))
	}
	save := fake.savedDrafts[0]
	if save.ItemID != itemID || save.Input.Title != "Winter service" ||
		save.Input.Excerpt != nil || save.Input.Body != nil || save.Input.CallToAction != nil {
		t.Fatalf("unexpected save input: %+v", save.Input)
	}
	if len(save.Input.ChannelCopy) != 3 || save.Input.ChannelCopy["x"] != "Winter service. Book today." {
		t.Fatalf("channel copy wrong: %+v", save.Input.ChannelCopy)
	}
	if save.Input.ChannelCopy["youtube"] != "Existing channel description." {
		t.Fatalf("previous channel copy must merge, never drop: %+v", save.Input.ChannelCopy)
	}
	if save.Input.Metadata["source"] != "agent" || save.Input.Metadata["agentId"] != agents.ChannelCopyWriterID {
		t.Fatalf("provenance lost: %+v", save.Input.Metadata)
	}

	// The reply envelope matches the channel-copy contract's dashboard shape.
	var parsed struct {
		Item     contentItemJSON    `json:"item"`
		Version  contentVersionJSON `json:"version"`
		Provider string             `json:"provider"`
		AgentID  string             `json:"agentId"`
		Channels map[string]string  `json:"channels"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("body is not the channel-copy envelope: %v", err)
	}
	if parsed.AgentID != agents.ChannelCopyWriterID || parsed.Provider != "fake" ||
		parsed.Version.ID == "" || len(parsed.Channels) != 2 {
		t.Fatalf("unexpected envelope: %+v / channels %+v", parsed, parsed.Channels)
	}

	// Task history: one completed action event scoped to the target item.
	if len(fake.auditEvents) != 1 {
		t.Fatalf("expected one audit event, got %d", len(fake.auditEvents))
	}
	audit := fake.auditEvents[0]
	if audit.Action != "agent.action.file_channel_copy.completed" ||
		audit.EntityType != "content_item" || audit.EntityID == nil || *audit.EntityID != itemID {
		t.Fatalf("unexpected audit event: %+v", audit)
	}
	if audit.Details["channels"] != 2 || audit.Details["agent_id"] != agents.ChannelCopyWriterID { // new copy only, merged set is 3
		t.Fatalf("audit details lost the run facts: %+v", audit.Details)
	}

	// The offline stub must also work end to end (dev mode without a key).
	h = NewRouter(Deps{Logger: testLogger, Workspaces: fake, AgentRunner: aiprovider.StubProvider{}})
	rec = doJSON(t, h, http.MethodPost, agentRunPath(agentWsID, agents.ChannelCopyWriterID),
		`{"contentItemId":"`+itemID+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("offline stub status %d: %s", rec.Code, rec.Body.String())
	}
}

// TestChannelCopyWriterPreRunValidation pins the before-any-model-call
// failures: a required target (400), an unknown or foreign item (404), and
// a frozen item (409).
func TestChannelCopyWriterPreRunValidation(t *testing.T) {
	for _, tc := range []struct {
		name        string
		body        string
		setupItem   bool
		itemStatus  string
		wantStatus  int
		noModelCall bool
	}{
		{"missing contentItemId", `{}`, true, "draft", http.StatusBadRequest, true},
		{"unknown item", `{"contentItemId":"22222222-2222-2222-2222-222222222222"}`, false, "", http.StatusNotFound, true},
		{"review item", `{"contentItemId":"22222222-2222-2222-2222-222222222222"}`, true, "review", http.StatusConflict, true},
		// Approved (and rejected) items must not be silently reset to draft by
		// a run: the agent files only into drafts.
		{"approved item", `{"contentItemId":"22222222-2222-2222-2222-222222222222"}`, true, "approved", http.StatusConflict, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := workspaceDeps(nil, nil)
			itemID := "22222222-2222-2222-2222-222222222222"
			if tc.setupItem {
				item := store.ContentItem{ID: itemID, WorkspaceID: agentWsID, Kind: "social_post", Title: "Winter service", Status: tc.itemStatus}
				fake.contentItemsByID = map[string]store.ContentItem{itemID: item}
				fake.contentVersionsByItem = map[string][]store.ContentVersion{
					itemID: {{ID: "55555555-5555-5555-5555-555555555555", ContentItemID: itemID, VersionNumber: 1}},
				}
			}
			runner := &fakeAgentRunner{text: channelCopyReplyJSON()}
			h := NewRouter(Deps{Logger: testLogger, Workspaces: fake, AgentRunner: runner})
			rec := doJSON(t, h, http.MethodPost, agentRunPath(agentWsID, agents.ChannelCopyWriterID), tc.body)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if tc.noModelCall && len(runner.calls) != 0 {
				t.Fatalf("a pre-run failure must not reach the model: %d calls", len(runner.calls))
			}
			for _, e := range fake.auditEvents {
				if e.Action != "agent.run.failed" {
					t.Fatalf("unexpected audit event: %+v", e)
				}
			}
		})
	}
	// An item from another workspace is indistinguishable from a missing one.
	fake := workspaceDeps(nil, nil)
	foreign := "33333333-3333-3333-3333-333333333333"
	fake.contentItemsByID = map[string]store.ContentItem{
		foreign: {ID: foreign, WorkspaceID: "99999999-9999-9999-9999-999999999999", Kind: "social_post", Status: "draft"},
	}
	runner := &fakeAgentRunner{text: channelCopyReplyJSON()}
	h := NewRouter(Deps{Logger: testLogger, Workspaces: fake, AgentRunner: runner})
	rec := doJSON(t, h, http.MethodPost, agentRunPath(agentWsID, agents.ChannelCopyWriterID),
		`{"contentItemId":"`+foreign+`"}`)
	if rec.Code != http.StatusNotFound || len(runner.calls) != 0 {
		t.Fatalf("foreign item: status %d, model calls %d", rec.Code, len(runner.calls))
	}
}

// TestChannelCopyWriterFilingErrors maps the store disagreements onto the
// same responses the manual draft editor produces.
func TestChannelCopyWriterFilingErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		errors error
		status int
	}{{"item frozen between load and save", store.ErrNotEditable, http.StatusConflict},
		{"item gone", store.ErrNotFound, http.StatusNotFound},
		{"store failure", errors.New("boom"), http.StatusInternalServerError}} {
		t.Run(tc.name, func(t *testing.T) {
			fake := workspaceDeps(nil, nil)
			itemID := channelCopyFixture(fake)
			fake.saveDraftErr = tc.errors
			runner := &fakeAgentRunner{text: channelCopyReplyJSON()}
			h := NewRouter(Deps{Logger: testLogger, Workspaces: fake, AgentRunner: runner})
			rec := doJSON(t, h, http.MethodPost, agentRunPath(agentWsID, agents.ChannelCopyWriterID),
				`{"contentItemId":"`+itemID+`"}`)
			if rec.Code != tc.status {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if tc.status == http.StatusInternalServerError && strings.Contains(rec.Body.String(), "boom") {
				t.Fatal("internal error echoed raw store text")
			}
		})
	}
}

func TestAgentRunNoDataWorkflow(t *testing.T) {
	ws := workspaceDeps(nil, nil)
	loader := &fakeGSCLoader{}
	runner := &fakeAgentRunner{text: advisorReplyJSON("none", false)}
	h := NewRouter(Deps{Logger: testLogger, Workspaces: ws, AgentRunner: runner, AgentLoaders: gscLoaderDeps(loader)})

	rec := postAdvisory(h, agentWsID, `{"focus":"local pack ideas"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var parsed struct {
		Report     agents.AdvisorReport `json:"report"`
		DataNote   string               `json:"dataNote"`
		DurationMS int64                `json:"durationMs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("body is not the advisory shape: %v", err)
	}
	if parsed.Report.DataSource != "none" {
		t.Fatalf("no loader data must force dataSource none, got %q", parsed.Report.DataSource)
	}
	if parsed.Report.CannibalizationChecked {
		t.Fatalf("no loader data must force cannibalizationChecked=false")
	}
	if parsed.Report.Provider != "fake" || parsed.Report.Model != "fake-1" {
		t.Fatalf("wrong provider metadata: %+v", parsed.Report)
	}
	if parsed.Report.AgentID != agents.SEOAdvisorID || parsed.Report.Version != agents.SEOAdvisorVersion {
		t.Fatalf("missing specialist identity: %+v", parsed.Report)
	}
	if parsed.DurationMS < 0 {
		t.Fatalf("negative duration")
	}

	// The loader saw exactly the requested workspace...
	if len(loader.calls) != 1 || loader.calls[0] != agentWsID {
		t.Fatalf("loader workspace calls: %#v", loader.calls)
	}
	// ...and the prompt told the specialist the truth.
	req := runner.calls[0]
	if !strings.HasSuffix(req.User, gscDataMarkerNone()) {
		t.Fatalf("prompt should end with the no-data marker:\n%s", req.User)
	}
	if !strings.Contains(req.User, "Operator focus for this review: local pack ideas") {
		t.Fatalf("focus missing from the prompt:\n%s", req.User)
	}

	// Task history: exactly one completed audit event, workspace-scoped.
	if len(ws.auditEvents) != 1 {
		t.Fatalf("expected one audit event, got %d", len(ws.auditEvents))
	}
	audit := ws.auditEvents[0]
	if audit.Action != "agent.run.completed" {
		t.Fatalf("wrong audit action: %q", audit.Action)
	}
	if audit.WorkspaceID == nil || *audit.WorkspaceID != agentWsID {
		t.Fatalf("audit not workspace-scoped: %#v", audit)
	}
}

func TestAgentRunPromptContents(t *testing.T) {
	runner := &fakeAgentRunner{text: advisorReplyJSON("none", false)}
	h := NewRouter(Deps{Logger: testLogger, Workspaces: workspaceDeps(nil, nil),
		AgentRunner: runner, AgentLoaders: gscLoaderDeps(&fakeGSCLoader{})})
	rec := postAdvisory(h, agentWsID, `{"focus":"local pack ideas"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one provider call, got %d", len(runner.calls))
	}
	req := runner.calls[0]
	if !strings.HasSuffix(req.User, gscDataMarkerNone()) {
		t.Fatalf("prompt should end with the no-data marker:\n%s", req.User)
	}
	if !strings.Contains(req.User, "Operator focus for this review: local pack ideas") {
		t.Fatalf("focus missing from the prompt:\n%s", req.User)
	}
	if !strings.Contains(req.User, "Business: Demo Cafe (https://demo.example)") {
		t.Fatalf("workspace newline facts missing:\n%s", req.User)
	}
}

func TestAgentRunWithGSCSnapshot(t *testing.T) {
	runner := &fakeAgentRunner{text: advisorReplyJSON("search_console", true)}
	loader := &fakeGSCLoader{snapshot: &agents.GSCSnapshot{
		Property:  "sc-domain:demo.example",
		StartDate: "2026-09-24",
		EndDate:   "2026-10-22",
		PageQueries: []agents.GSCPageQueryRow{{
			Page: "/a", Query: "coffee", Clicks: 5, Impressions: 100, Ctr: 0.05, Position: 11.5,
		}},
		TopPages: []agents.GSCPageRow{{Page: "/a", Clicks: 5, Impressions: 100, Ctr: 0.05, Position: 11.5}},
	}}
	h := NewRouter(Deps{Logger: testLogger, Workspaces: workspaceDeps(nil, nil),
		AgentRunner: runner, AgentLoaders: gscLoaderDeps(loader)})

	rec := postAdvisory(h, agentWsID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	req := runner.calls[0]
	if !strings.HasSuffix(req.User, gscDataMarkerGSC()) {
		t.Fatalf("GSC marker missing:\n%s", req.User)
	}
	if !strings.Contains(req.User, `"query":"coffee"`) || !strings.Contains(req.User, `"page":"/a"`) {
		t.Fatalf("verbatim rows missing from the prompt:\n%s", req.User)
	}
	if !strings.Contains(req.System, "NO INVENTED DATA") || !strings.Contains(req.System, "ADVISORY ONLY") {
		t.Fatalf("guardrail prompt text missing")
	}
}

func TestAgentRunBadFocus(t *testing.T) {
	h := NewRouter(Deps{Logger: testLogger, Workspaces: workspaceDeps(nil, nil),
		AgentRunner: &fakeAgentRunner{}})
	long := strings.Repeat("f", 2001)
	rec := postAdvisory(h, agentWsID, `{"focus":"`+long+`"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for over-long focus, got %d", rec.Code)
	}
	rec = postAdvisory(h, agentWsID, `{invalid`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad JSON, got %d", rec.Code)
	}
}

func TestAgentRunSpecialistFailure(t *testing.T) {
	ws := workspaceDeps(nil, nil)
	runner := &fakeAgentRunner{fail: &aiprovider.ProviderError{Kind: aiprovider.FailTimeout, Cause: "deadline"}}
	h := NewRouter(Deps{Logger: testLogger, Workspaces: ws, AgentRunner: runner})

	rec := postAdvisory(h, agentWsID, "")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 on provider failure, got %d", rec.Code)
	}
	if len(ws.auditEvents) != 1 || ws.auditEvents[0].Action != "agent.run.failed" {
		t.Fatalf("expected a failure audit event: %#v", ws.auditEvents)
	}
	if _, ok := ws.auditEvents[0].Details["reason_kind"]; !ok {
		t.Fatalf("failure audit missing reason kind")
	}
}

// TestLoaderOnlyOwnWorkspaceIntegration exercises the real loader against the
// fake store: a Search Console integration that belongs to another workspace
// must not be visible to this workspace's run.
func TestLoaderOnlyOwnWorkspaceIntegration(t *testing.T) {
	otherWS := "22222222-2222-2222-2222-222222222222"
	otherIntegration := store.Integration{
		ID:          "int-1",
		WorkspaceID: otherWS,
		AdapterKind: "search_console",
		Name:        "Other workspace GSC",
		Status:      "connected",
		Config:      map[string]any{"property": "sc-domain:other.example"},
	}
	for _, tt := range []struct {
		name   string
		result []store.Integration
	}{
		{"no integrations", nil},
		{"only other workspace", []store.Integration{otherIntegration}},
		{"own but not connected", []store.Integration{{
			ID: "int-2", WorkspaceID: agentWsID, AdapterKind: "search_console",
			Name: "G", Status: "limited", Config: map[string]any{"property": "sc-domain:demo.example"},
		}}},
	} {
		fake := &fakeWorkspaceStore{listIntegrationsResult: tt.result}
		loader := NewGSCAdvisoryLoader(fake, []byte("k"))
		snap, note, err := loader.LoadSnapshot(context.Background(), agentWsID)
		if err != nil {
			t.Fatalf("%s: LoadSnapshot: %v", tt.name, err)
		}
		if snap != nil {
			t.Fatalf("%s: snapshot must be nil, got %#v", tt.name, snap)
		}
		if strings.TrimSpace(note) == "" {
			t.Fatalf("%s: expected a no-data note", tt.name)
		}
	}
}

// TestLoaderGarbageCredentialsDegradesSafely verifies that decrypt and Google
// failures degrade to no-data mode with a note, never to presenting an
// analysis the run could not actually perform.
func TestLoaderGarbageCredentialsDegradesSafely(t *testing.T) {
	fake := &fakeWorkspaceStore{
		listIntegrationsResult: []store.Integration{{
			ID: "int-3", WorkspaceID: agentWsID, AdapterKind: "search_console",
			Name: "G", Status: "connected", Config: map[string]any{"property": "sc-domain:demo.example"},
		}},
		credentialSetsByInID: map[string]store.CredentialSet{
			"int-3": {IntegrationID: "int-3", KeyVersion: 1, Ciphertext: []byte("garbage")},
		},
	}
	loader := NewGSCAdvisoryLoader(fake, []byte("k"))
	snap, note, err := loader.LoadSnapshot(context.Background(), agentWsID)
	if err != nil {
		t.Fatalf("LoadSnapshot must not error on store-level shape: %v", err)
	}
	if snap != nil {
		t.Fatalf("garbage credentials must not yield a snapshot: %#v", snap)
	}
	if strings.TrimSpace(note) == "" {
		t.Fatalf("expected a safe note explaining the no-data mode")
	}
}

// ---- Lead responder (sixth specialist, fourth executor) --------------------

// leadReplyFixture seeds one real enquiry in the fake store.
func leadReplyFixture(fake *fakeWorkspaceStore) string {
	const id = "66666666-6666-6666-6666-666666666666"
	name, email := "Jane Doe", "jane@example.com"
	message := "Do you service the north side? I need a furnace check before winter."
	fake.leadsByID = map[string]store.Lead{id: {
		ID: id, WorkspaceID: agentWsID, SourceKind: "form_api", SourceDetail: "contact form",
		Status: "new", ContactName: &name, ContactEmail: &email, Message: &message,
		Notes: "operator notes stay put",
	}}
	return id
}

func leadReplyJSON(subject, body string) string {
	return `{"subject":"` + subject + `","body":"` + body + `"}`
}

// TestLeadResponderRunThroughGenericRoute proves the fourth executor class:
// the same generic route answers one existing lead, and the file_lead_reply
// action stores a reviewable draft on that lead without sending anything or
// moving its status.
func TestLeadResponderRunThroughGenericRoute(t *testing.T) {
	fake := workspaceDeps(nil, nil)
	leadID := leadReplyFixture(fake)
	const subject = "Re: your furnace check enquiry"
	const body = "Hi Jane, thanks for the detailed note about the north side. I'll confirm the visit window and get back to you."
	runner := &fakeAgentRunner{text: leadReplyJSON(subject, body)}
	h := NewRouter(Deps{Logger: testLogger, Workspaces: fake, AgentRunner: runner})
	rec := doJSON(t, h, http.MethodPost, agentRunPath(agentWsID, agents.LeadResponderID),
		`{"leadId":"`+leadID+`","focus":"Mention we answer within one working day"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	// The brief carries the lead's own words and recorded facts, never the
	// operator's other data: no notes, no other leads.
	req := runner.calls[0]
	for _, want := range []string{
		"Reply to this lead",
		"Contact name: Jane Doe",
		"Contact email: jane@example.com",
		"Enquiry (the lead's own words): Do you service the north side?",
		"Mention we answer within one working day",
	} {
		if !strings.Contains(req.User, want) {
			t.Fatalf("brief lost %q:\n%s", want, req.User)
		}
	}
	if strings.Contains(req.User, "operator notes stay put") {
		t.Fatalf("private operator notes leaked into the prompt:\n%s", req.User)
	}

	// Exactly one draft stored on the target lead; nothing created.
	if len(fake.savedLeadReplies) != 1 || len(fake.insertedLeads) != 0 || len(fake.updatedLeads) != 0 {
		t.Fatalf("unexpected filing calls: replies %d, inserts %d, updates %d",
			len(fake.savedLeadReplies), len(fake.insertedLeads), len(fake.updatedLeads))
	}
	saved := fake.savedLeadReplies[0]
	if saved.workspaceID != agentWsID || saved.leadID != leadID || saved.subject != subject || saved.body != body {
		t.Fatalf("unexpected saved reply: %+v", saved)
	}

	var parsed struct {
		Lead    leadJSON `json:"lead"`
		Subject string   `json:"subject"`
		Body    string   `json:"body"`
		AgentID string   `json:"agentId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("body is not the lead-reply envelope: %v", err)
	}
	if parsed.Lead.ID != leadID || parsed.Subject != subject || parsed.Body != body ||
		parsed.AgentID != agents.LeadResponderID {
		t.Fatalf("unexpected envelope: %+v", parsed)
	}
	if parsed.Lead.ReplyDraftSubject == nil || *parsed.Lead.ReplyDraftSubject != subject ||
		parsed.Lead.ReplyDraftBody == nil || *parsed.Lead.ReplyDraftBody != body ||
		parsed.Lead.ReplyDraftedAt == nil {
		t.Fatalf("envelope lead lost the stored draft: %+v", parsed.Lead)
	}
	if parsed.Lead.Status != "new" {
		t.Fatalf("drafting a reply must not move the lead status: %q", parsed.Lead.Status)
	}

	// Task history: one completed action event scoped to the lead.
	if len(fake.auditEvents) != 1 {
		t.Fatalf("expected one audit event, got %d", len(fake.auditEvents))
	}
	audit := fake.auditEvents[0]
	if audit.Action != "agent.action.file_lead_reply.completed" ||
		audit.EntityType != "lead" || audit.EntityID == nil || *audit.EntityID != leadID {
		t.Fatalf("unexpected audit event: %+v", audit)
	}
	if audit.Details["status"] != "new" || audit.Details["agent_id"] != agents.LeadResponderID {
		t.Fatalf("audit details lost the run facts: %+v", audit.Details)
	}

	// The offline stub must also work end to end (dev mode without a key).
	h = NewRouter(Deps{Logger: testLogger, Workspaces: fake, AgentRunner: aiprovider.StubProvider{}})
	rec = doJSON(t, h, http.MethodPost, agentRunPath(agentWsID, agents.LeadResponderID),
		`{"leadId":"`+leadID+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("offline stub status %d: %s", rec.Code, rec.Body.String())
	}
}

// TestLeadResponderPreRunValidation pins the before-any-model-call failures: a
// required target (400), an unknown or foreign lead (404), a decided lead
// (409), and a lead with nothing to answer (400).
func TestLeadResponderPreRunValidation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		body       string
		leadStatus string
		noContent  bool
		wantStatus int
	}{
		{"missing leadId", `{}`, "new", false, http.StatusBadRequest},
		{"unknown lead", `{"leadId":"77777777-7777-7777-7777-777777777777"}`, "", false, http.StatusNotFound},
		{"won lead", `{"leadId":"66666666-6666-6666-6666-666666666666"}`, "won", false, http.StatusConflict},
		{"lost lead", `{"leadId":"66666666-6666-6666-6666-666666666666"}`, "lost", false, http.StatusConflict},
		{"empty lead", `{"leadId":"66666666-6666-6666-6666-666666666666"}`, "new", true, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := workspaceDeps(nil, nil)
			leadID := leadReplyFixture(fake)
			lead := fake.leadsByID[leadID]
			if tc.leadStatus != "" {
				lead.Status = tc.leadStatus
			}
			if tc.noContent {
				lead.ContactName, lead.Message = nil, nil
			}
			fake.leadsByID[leadID] = lead
			runner := &fakeAgentRunner{text: leadReplyJSON("s", "b")}
			h := NewRouter(Deps{Logger: testLogger, Workspaces: fake, AgentRunner: runner})
			rec := doJSON(t, h, http.MethodPost, agentRunPath(agentWsID, agents.LeadResponderID), tc.body)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if len(runner.calls) != 0 {
				t.Fatalf("a pre-run failure must not reach the model: %d calls", len(runner.calls))
			}
			if len(fake.savedLeadReplies) != 0 {
				t.Fatalf("a pre-run failure must not store a reply: %+v", fake.savedLeadReplies)
			}
			for _, e := range fake.auditEvents {
				if e.Action != "agent.run.failed" {
					t.Fatalf("unexpected audit event: %+v", e)
				}
			}
		})
	}
	// A lead from another workspace is indistinguishable from a missing one.
	fake := workspaceDeps(nil, nil)
	foreign := "88888888-8888-8888-8888-888888888888"
	message := "hello"
	fake.leadsByID = map[string]store.Lead{
		foreign: {ID: foreign, WorkspaceID: "99999999-9999-9999-9999-999999999999",
			SourceKind: "form_api", Status: "new", Message: &message},
	}
	runner := &fakeAgentRunner{text: leadReplyJSON("s", "b")}
	h := NewRouter(Deps{Logger: testLogger, Workspaces: fake, AgentRunner: runner})
	rec := doJSON(t, h, http.MethodPost, agentRunPath(agentWsID, agents.LeadResponderID),
		`{"leadId":"`+foreign+`"}`)
	if rec.Code != http.StatusNotFound || len(runner.calls) != 0 {
		t.Fatalf("foreign lead: status %d, model calls %d", rec.Code, len(runner.calls))
	}
}

// TestLeadResponderFilingErrors maps store disagreements onto safe responses
// without echoing raw store text.
func TestLeadResponderFilingErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{"lead gone between load and save", store.ErrNotFound, http.StatusNotFound},
		{"empty drafted reply", store.ErrNameRequired, http.StatusBadGateway},
		{"store failure", errors.New("boom"), http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := workspaceDeps(nil, nil)
			leadID := leadReplyFixture(fake)
			fake.saveLeadReplyErr = tc.err
			runner := &fakeAgentRunner{text: leadReplyJSON("s", "b")}
			h := NewRouter(Deps{Logger: testLogger, Workspaces: fake, AgentRunner: runner})
			rec := doJSON(t, h, http.MethodPost, agentRunPath(agentWsID, agents.LeadResponderID),
				`{"leadId":"`+leadID+`"}`)
			if rec.Code != tc.status {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "boom") {
				t.Fatal("internal error echoed raw store text")
			}
			if len(fake.auditEvents) != 0 {
				t.Fatalf("a failed filing must not record a completion event: %+v", fake.auditEvents)
			}
		})
	}
}
