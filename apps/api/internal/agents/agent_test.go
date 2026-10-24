package agents

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"marketingops/api/internal/aiprovider"
)

// fakeRunner records prompts and answers from canned replies.
type fakeRunner struct {
	replies []aiprovider.PromptReply
	calls   []aiprovider.RunPromptRequest
}

func (f *fakeRunner) RunPrompt(ctx context.Context, req aiprovider.RunPromptRequest) (aiprovider.PromptReply, error) {
	f.calls = append(f.calls, req)
	if len(f.replies) == 0 {
		return aiprovider.PromptReply{}, errors.New("no canned reply")
	}
	reply := f.replies[0]
	f.replies = f.replies[1:]
	return reply, nil
}

// validNoDataReply is a provider text a model could produce in no-data mode.
const validNoDataReply = `Here is your report: {"dataSource":"none","cannibalizationChecked":false,` +
	`"summary":"No connected Search Console data is available yet.","gaps":["insufficient connected GSC data"],` +
	`"recommendations":[{"priority":"HIGH","area":"metadata","page":"","issue":"Missing meta descriptions","evidence":"based on the provided workspace context only","action":"Add meta descriptions to main pages."}]}`

// validGSCReply is a provider text for the data-available mode.
const validGSCReply = `{"dataSource":"search_console","cannibalizationChecked":true,` +
	`"summary":"Two pages compete for the same query.","gaps":[],` +
	`"recommendations":[{"priority":"high","area":"cannibalization","page":"/a","issue":"Two pages rank for one query","evidence":"row: /a, query x, pos 12","action":"Merge the two pages."}]}`

var sampleGSC = &GSCSnapshot{
	Property:  "sc-domain:demo.example",
	StartDate: "2026-09-24",
	EndDate:   "2026-10-22",
	PageQueries: []GSCPageQueryRow{
		{Page: "/a", Query: "coffee", Clicks: 12, Impressions: 900, Ctr: 0.0133, Position: 14.5},
		{Page: "/b", Query: "coffee", Clicks: 3, Impressions: 120, Ctr: 0.025, Position: 21.0},
	},
	TopPages: []GSCPageRow{{Page: "/a", Clicks: 30, Impressions: 1000, Ctr: 0.03, Position: 12.2}},
}

var sampleWorkspace = aiprovider.WorkspaceContext{
	Name:        "Demo Cafe",
	WebsiteURL:  "https://demo.example",
	Description: "A small cafe.",
	Tone:        "friendly",
}

func TestBuildUserPromptNoDataMode(t *testing.T) {
	text := SEOAdvisor{}.buildUserPrompt(AgentInput{Workspace: sampleWorkspace})
	if !strings.Contains(text, "Business: Demo Cafe (https://demo.example)") {
		t.Fatalf("missing business line:\n%s", text)
	}
	if !strings.Contains(text, "About: A small cafe.") {
		t.Fatalf("missing description:\n%s", text)
	}
	if !strings.HasSuffix(text, "\n"+gscDataMarker+gscDataNone) {
		t.Fatalf("prompt must end with the no-data marker, got:\n%s", text)
	}
	if strings.Contains(text, "GSCDataAvail") || strings.Contains(text, "Clicks") {
		t.Fatalf("no-data prompt must not contain fabricated row data:\n%s", text)
	}
}

func TestBuildUserPromptGSCMode(t *testing.T) {
	text := SEOAdvisor{}.buildUserPrompt(AgentInput{Workspace: sampleWorkspace, GSC: sampleGSC})
	if !strings.HasSuffix(text, "\n"+gscDataMarker+gscDataSearchConsole) {
		t.Fatalf("prompt must end with the GSC marker, got:\n%s", text)
	}
	if !strings.Contains(text, "Property: sc-domain:demo.example") && !strings.Contains(text, "sc-domain:demo.example") {
		t.Fatalf("missing property:\n%s", text)
	}
	if !strings.Contains(text, `"query":"coffee"`) || !strings.Contains(text, `"page":"/b"`) {
		t.Fatalf("verbatim rows missing:\n%s", text)
	}
}

// TestBuildUserPromptFocusBounded keeps operator focus in the prompt but
// bounded and secret-free by construction tests.
func TestBuildUserPromptFocusBounded(t *testing.T) {
	text := SEOAdvisor{}.buildUserPrompt(AgentInput{Workspace: sampleWorkspace, Focus: "focus on local SEO"})
	if !strings.Contains(text, "Operator focus for this review: focus on local SEO") {
		t.Fatalf("missing focus:\n%s", text)
	}
}

func TestAdviseNoDataReply(t *testing.T) {
	runner := &fakeRunner{replies: []aiprovider.PromptReply{{
		Provider: "stub", Model: "stub-v1", Text: validNoDataReply,
	}}}
	report, err := SEOAdvisor{}.Advise(context.Background(), runner, AgentInput{Workspace: sampleWorkspace})
	if err != nil {
		t.Fatalf("Advise: %v", err)
	}
	if report.DataSource != "none" || report.CannibalizationChecked {
		t.Fatalf("no-data run must stay no-data: %+v", report)
	}
	if report.AgentID != SEOAdvisorID || report.Version != SEOAdvisorVersion {
		t.Fatalf("missing identity: %+v", report)
	}
	if report.Provider != "stub" || report.Model != "stub-v1" {
		t.Fatalf("missing provider metadata: %+v", report)
	}
	if len(report.Recommendations) != 1 || report.Recommendations[0].Priority != "high" {
		t.Fatalf("priority normalization failed: %+v", report.Recommendations)
	}
	if len(report.Gaps) != 1 {
		t.Fatalf("gaps lost: %+v", report.Gaps)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one provider call, got %d", len(runner.calls))
	}
}

func TestAdviseGSCReply(t *testing.T) {
	runner := &fakeRunner{replies: []aiprovider.PromptReply{{
		Provider: "openrouter", Model: "m1", Text: validGSCReply,
	}}}
	report, err := SEOAdvisor{}.Advise(context.Background(), runner, AgentInput{Workspace: sampleWorkspace, GSC: sampleGSC})
	if err != nil {
		t.Fatalf("Advise: %v", err)
	}
	if report.DataSource != "search_console" || !report.CannibalizationChecked {
		t.Fatalf("GSC run lost its data basis: %+v", report)
	}
}

func TestAdviseRejectsCannibalizationSkippedWithData(t *testing.T) {
	// With GSC rows provided, a reply that skips the mandatory check and
	// still issues recommendations is rejected outright.
	runner := &fakeRunner{replies: []aiprovider.PromptReply{{
		Provider: "fake", Model: "fake-1",
		Text: `{"dataSource":"search_console","cannibalizationChecked":false,"summary":"s","gaps":[],"recommendations":[{"priority":"high","area":"a","issue":"i","evidence":"e","action":"c"}]}`,
	}}}
	_, err := SEOAdvisor{}.Advise(context.Background(), runner, AgentInput{Workspace: sampleWorkspace, GSC: sampleGSC})
	var perr *aiprovider.ProviderError
	if !errors.As(err, &perr) || perr.Kind != aiprovider.FailMalformed {
		t.Fatalf("expected malformed rejection, got %v", err)
	}
}

func TestAdviseRejectsGarbageReply(t *testing.T) {
	runner := &fakeRunner{replies: []aiprovider.PromptReply{{
		Provider: "fake", Model: "fake-1", Text: "not json at all",
	}}}
	_, err := SEOAdvisor{}.Advise(context.Background(), runner, AgentInput{Workspace: sampleWorkspace})
	if !errors.As(err, new(*aiprovider.ProviderError)) {
		t.Fatalf("expected ProviderError, got %v", err)
	}
}

// TestAdvisePromptCarriesReality verifies the provider call actually got the
// workspace facts and the data-availability marker.
func TestAdvisePromptCarriesReality(t *testing.T) {
	claimsWorkspace := sampleWorkspace
	claimsWorkspace.ProhibitedClaims = []string{"no health guarantees"}
	runner := &fakeRunner{replies: []aiprovider.PromptReply{{Provider: "x", Model: "y", Text: validNoDataReply}}}
	_, adviseErr := SEOAdvisor{}.Advise(context.Background(), runner, AgentInput{Workspace: claimsWorkspace})
	if adviseErr != nil {
		t.Fatalf("Advise: %v", adviseErr)
	}
	call := runner.calls[0]
	if !strings.Contains(call.System, "WHITE-HAT ONLY") || !strings.Contains(call.System, "SEARCH CONSOLE GATE") {
		t.Fatalf("system prompt lost specialist guardrails:\n%s", call.System)
	}
	if !strings.Contains(call.User, "Do not make these claims: no health guarantees") {
		t.Fatalf("prohibited claims missing:\n%s", call.User)
	}
	// An empty prohibited list must produce no claims line.
	clean, ok := SEOAdvisor{}.buildUserPrompt(AgentInput{Workspace: sampleWorkspace}), true
	if !ok {
		t.Fatal("unreachable")
	}
	if strings.Contains(clean, "Do not make these claims:") {
		t.Fatalf("empty prohibited list should produce no claims line:\n%s", clean)
	}
}

func TestValidateReplyCapsAndNorms(t *testing.T) {
	var long strings.Builder
	for len(long.String()) < 2000 {
		long.WriteString("word ")
	}
	longIssue := long.String()
	recs := make([]any, 0, maxRecommendations+15)
	for len(recs) < maxRecommendations+15 {
		recs = append(recs, map[string]string{
			"priority": "low", "area": "a", "page": "/x",
			"issue": longIssue, "evidence": "e", "action": "c",
		})
	}
	payload := map[string]any{
		"dataSource":             "none",
		"cannibalizationChecked": false,
		"summary":                strings.Repeat("s", 9000),
		"gaps":                   arrayFilled(maxGaps+15, longIssue),
		"recommendations":        recs,
	}
	raw, _ := json.Marshal(payload)
	report, err := SEOAdvisor{}.validateReply(aiprovider.PromptReply{Text: string(raw)}, false)
	if err != nil {
		t.Fatalf("validateReply: %v", err)
	}
	if runeCount(report.Summary) > maxSummaryRunes {
		t.Fatalf("summary not capped: %d", runeCount(report.Summary))
	}
	if len(report.Gaps) != maxGaps {
		t.Fatalf("gaps not capped: %d", len(report.Gaps))
	}
	if len(report.Recommendations) != maxRecommendations {
		t.Fatalf("recommendations not capped: %d", len(report.Recommendations))
	}
	// An item whose issue text is long but truncated is still usable.
	for _, rec := range report.Recommendations {
		if runeCount(rec.Issue) > maxFieldRunes {
			t.Fatalf("issue not capped: %d", runeCount(rec.Issue))
		}
	}
}

func arrayFilled(n int, template string) []string {
	out := make([]string, 0, n)
	for len(out) < n {
		out = append(out, template)
	}
	return out
}

func TestExtractJSONObjectSkipsChatter(t *testing.T) {
	text := []byte("Sure! {\"a\":{\"b\":1}} hope this helps")
	got := extractJSONObject(text)
	if string(got) != `{"a":{"b":1}}` {
		t.Fatalf("wrong extraction: %q", got)
	}
}

// --- Stub provider RunPrompt -------------------------------------------------

func TestStubRunPromptNoData(t *testing.T) {
	user := SEOAdvisor{}.buildUserPrompt(AgentInput{Workspace: sampleWorkspace})
	reply, err := aiprovider.StubProvider{}.RunPrompt(context.Background(), aiprovider.RunPromptRequest{
		System: SEOAdvisor{}.systemPrompt(), User: user,
	})
	if err != nil {
		t.Fatalf("RunPrompt: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(reply.Text), &payload); err != nil {
		t.Fatalf("stub reply is not JSON: %v\n%s", err, reply.Text)
	}
	if payload["dataSource"] != "none" {
		t.Fatalf("stub must state no-data: %v", payload["dataSource"])
	}
	if payload["cannibalizationChecked"] != false {
		t.Fatalf("stub must not claim a check: %v", payload["cannibalizationChecked"])
	}
	if !strings.Contains(reply.Text, "[Stub") {
		t.Fatalf("stub output must be labeled as stub: %s", reply.Text)
	}
}

func TestStubRunPromptGSCMarker(t *testing.T) {
	user := SEOAdvisor{}.buildUserPrompt(AgentInput{Workspace: sampleWorkspace, GSC: sampleGSC})
	reply, err := aiprovider.StubProvider{}.RunPrompt(context.Background(), aiprovider.RunPromptRequest{
		System: SEOAdvisor{}.systemPrompt(), User: user,
	})
	if err != nil {
		t.Fatalf("RunPrompt: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(reply.Text), &payload); err != nil {
		t.Fatalf("stub reply is not JSON: %v", err)
	}
	if payload["dataSource"] != "search_console" {
		t.Fatalf("stub must echo the available data basis: %v", payload["dataSource"])
	}
	if payload["cannibalizationChecked"] != true {
		t.Fatalf("stub must reflect the gate honestly for the marker: %v", payload["cannibalizationChecked"])
	}
}

func TestStubRunPromptErrors(t *testing.T) {
	_, emptyErr := aiprovider.StubProvider{}.RunPrompt(context.Background(), aiprovider.RunPromptRequest{})
	if emptyErr == nil {
		t.Fatal("expected empty prompt failure")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, cancelErr := aiprovider.StubProvider{}.RunPrompt(ctx, aiprovider.RunPromptRequest{System: "s", User: "u"})
	if cancelErr == nil {
		t.Fatal("expected context failure")
	}
}
