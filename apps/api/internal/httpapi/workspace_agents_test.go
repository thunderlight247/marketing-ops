package httpapi

import (
	"context"
	"encoding/json"
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

func postAdvisory(h http.Handler, wsID, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+wsID+"/agents/seo/advisory", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestSeoAdvisoryRunnerNotConfigured(t *testing.T) {
	h := NewRouter(Deps{Logger: testLogger, Workspaces: workspaceDeps(nil, nil)})
	rec := postAdvisory(h, agentWsID, "")
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 without a runner, got %d", rec.Code)
	}
}

func TestSeoAdvisoryUnknownWorkspace(t *testing.T) {
	h := NewRouter(Deps{Logger: testLogger, Workspaces: workspaceDeps(nil, nil), AgentRunner: &fakeAgentRunner{}})
	rec := postAdvisory(h, "00000000-0000-0000-0000-000000000000", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestSeoAdvisoryNoDataWorkflow(t *testing.T) {
	ws := workspaceDeps(nil, nil)
	loader := &fakeGSCLoader{}
	runner := &fakeAgentRunner{text: advisorReplyJSON("none", false)}
	h := NewRouter(Deps{Logger: testLogger, Workspaces: ws, AgentRunner: runner, GSCLoader: loader})

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
	if audit.Action != "agent.seo.advisory.completed" {
		t.Fatalf("wrong audit action: %q", audit.Action)
	}
	if audit.WorkspaceID == nil || *audit.WorkspaceID != agentWsID {
		t.Fatalf("audit not workspace-scoped: %#v", audit)
	}
}

func TestSeoAdvisoryPromptContents(t *testing.T) {
	runner := &fakeAgentRunner{text: advisorReplyJSON("none", false)}
	h := NewRouter(Deps{Logger: testLogger, Workspaces: workspaceDeps(nil, nil),
		AgentRunner: runner, GSCLoader: &fakeGSCLoader{}})
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

func TestSeoAdvisoryWithGSCSnapshot(t *testing.T) {
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
		AgentRunner: runner, GSCLoader: loader})

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

func TestSeoAdvisoryBadFocus(t *testing.T) {
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

func TestSeoAdvisorySpecialistFailure(t *testing.T) {
	ws := workspaceDeps(nil, nil)
	runner := &fakeAgentRunner{fail: &aiprovider.ProviderError{Kind: aiprovider.FailTimeout, Cause: "deadline"}}
	h := NewRouter(Deps{Logger: testLogger, Workspaces: ws, AgentRunner: runner})

	rec := postAdvisory(h, agentWsID, "")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 on provider failure, got %d", rec.Code)
	}
	if len(ws.auditEvents) != 1 || ws.auditEvents[0].Action != "agent.seo.advisory.failed" {
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
