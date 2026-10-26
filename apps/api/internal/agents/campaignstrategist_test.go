package agents

import (
	"context"
	"errors"
	"strings"
	"testing"

	"marketingops/api/internal/aiprovider"
)

// validStrategistReply is a provider text the model could produce for a
// campaign proposal.
const validStrategistReply = `Certainly — here is the plan: {"name":"Winter service reminder campaign",` +
	`"utmCampaign":"winter-service-2026","notes":"A reminder angle built on the winter maintenance service. [to confirm: pricing delta]",` +
	`"keyPoints":["Reminder angle","Runs through winter"]}`

func TestProposeBuildsBoundedPrompt(t *testing.T) {
	runner := &fakeRunner{replies: []aiprovider.PromptReply{{
		Text:     validStrategistReply,
		Metadata: map[string]any{"provider": "stub", "model": "stub-1"},
	}}}
	if _, err := (CampaignStrategist{}).Propose(context.Background(), runner, (AgentInput{
		Workspace: sampleWorkspace,
		Focus:     "Plan for the slow season",
	})); err != nil {
		t.Fatalf("Propose failed: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one prompt call, got %d", len(runner.calls))
	}
	req := runner.calls[0]
	if !strings.Contains(req.User, "Business: Demo Cafe (https://demo.example)") {
		t.Fatalf("user prompt lost the workspace context:\n%s", req.User)
	}
	if !strings.Contains(req.User, "Operator focus for this campaign: Plan for the slow season") {
		t.Fatalf("user prompt lost the focus:\n%s", req.User)
	}
	if strings.Contains(req.User, "Search Console") {
		t.Fatalf("a no-data specialist must not promise data it will not get:\n%s", req.User)
	}
	for _, want := range [...]string{"PROPOSAL ONLY", "NO PERFORMANCE DATA", "JSON"} {
		if !strings.Contains(req.System, want) {
			t.Fatalf("system prompt lost the %q rule:\n%s", want, req.System)
		}
	}
}

func TestProposeValidatesReply(t *testing.T) {
	proposal, err := CampaignStrategist{}.Propose(context.Background(), &fakeRunner{replies: []aiprovider.PromptReply{{
		Text:     validStrategistReply,
		Metadata: map[string]any{"provider": "stub", "model": "stub-1"},
	}}}, AgentInput{Workspace: sampleWorkspace})
	if err != nil {
		t.Fatalf("Propose failed: %v", err)
	}
	if proposal.AgentID != CampaignStrategistID || proposal.Version != CampaignStrategistVersion {
		t.Fatalf("proposal lost its identity: %+v", proposal)
	}
	if proposal.Name != "Winter service reminder campaign" {
		t.Fatalf("bad name: %q", proposal.Name)
	}
	if proposal.UtmCampaign != "winter-service-2026" {
		t.Fatalf("bad utm: %q", proposal.UtmCampaign)
	}
	if proposal.Provider != "stub" || proposal.Model != "stub-1" {
		t.Fatalf("provider/model lost: %s/%s", proposal.Provider, proposal.Model)
	}
	if len(proposal.KeyPoints) != 2 {
		t.Fatalf("bad key points: %+v", proposal.KeyPoints)
	}
}

func TestProposeRejectsMalformedReplies(t *testing.T) {
	cases := map[string]aiprovider.PromptReply{
		"no json":     {Text: "a chatty paragraph with no object"},
		"bad json":    {Text: `{"name":"`},
		"no name":     {Text: `{"name":"","utmCampaign":"x","notes":"n","keyPoints":[]}`},
		"whitespace":  {Text: `{"name":"   ","notes":"n"}`},
		"camel drama": {Text: "an apology and no object at all"},
	}
	for name, reply := range cases {
		_, err := CampaignStrategist{}.Propose(context.Background(), &fakeRunner{replies: []aiprovider.PromptReply{reply}}, AgentInput{})
		if err == nil {
			t.Fatalf("%s: expected an error, got a proposal", name)
		}
		var perr *aiprovider.ProviderError
		if !errors.As(err, &perr) {
			t.Fatalf("%s: expected a ProviderError, got %T", name, err)
		}
		if perr.Kind != aiprovider.FailMalformed {
			t.Fatalf("%s: expected FailMalformed, got %q", name, perr.Kind)
		}
	}
}

func TestProposeSanitizes(t *testing.T) {
	// An unusable UTM value is dropped, not fatal; over-cap fields are trimmed
	// and empty key points are dropped.
	reply := aiprovider.PromptReply{
		Text: `{"name":"` + strings.Repeat("n", 300) + `","utmCampaign":"Not A UTM Value","notes":"` +
			strings.Repeat("x", 3000) + `","keyPoints":["p","", "p2"]}`,
	}
	proposal, err := CampaignStrategist{}.Propose(context.Background(), &fakeRunner{replies: []aiprovider.PromptReply{reply}}, AgentInput{})
	if err != nil {
		t.Fatalf("Propose failed: %v", err)
	}
	if runeCount(proposal.Name) != maxCampaignNameRunes {
		t.Fatalf("name not capped: %d", runeCount(proposal.Name))
	}
	if runeCount(proposal.Notes) != maxCampaignNotesRunes {
		t.Fatalf("notes not capped: %d", runeCount(proposal.Notes))
	}
	if proposal.UtmCampaign != "" {
		t.Fatalf("bad utm kept: %q", proposal.UtmCampaign)
	}
	if len(proposal.KeyPoints) != 2 {
		t.Fatalf("empty key points not dropped: %+v", proposal.KeyPoints)
	}
}
