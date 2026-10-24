package agents

import (
	"context"
	"errors"
	"strings"
	"testing"

	"marketingops/api/internal/aiprovider"
)

// validCreatorReply is a provider text the model could produce for a draft.
const validCreatorReply = `Sure! Here is the piece: {"title":"Winter Maintenance for Real Coffee Machines","slug":"winter-maintenance-real-coffee",` +
	`"excerpt":"Keep the machines running all winter.","body":"## Winter maintenance\n\nDescaling matters in cold months. Publication follows. ","callToAction":"Book a service visit.",` +
	`"keyPoints":["Descaling in winter","Staff training"]}`

func TestCreateBuildsBoundedPrompt(t *testing.T) {
	runner := &fakeRunner{replies: []aiprovider.PromptReply{{
		Text:     validCreatorReply,
		Metadata: map[string]any{"provider": "stub", "model": "stub-1"},
	}}}
	_, err := ContentCreator{}.Create(context.Background(), runner, AgentInput{
		Workspace: sampleWorkspace,
		Focus:     "Content kind for this piece: blog_article\nWrite about winter maintenance",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one prompt call, got %d", len(runner.calls))
	}
	req := runner.calls[0]
	if !strings.Contains(req.User, "Business: Demo Cafe (https://demo.example)") {
		t.Fatalf("user prompt lost the workspace context:\n%s", req.User)
	}
	if !strings.Contains(req.User, "Content kind for this piece: blog_article") {
		t.Fatalf("user prompt lost the brief:\n%s", req.User)
	}
	if !strings.Contains(req.System, "Do not make these claims") || !strings.Contains(req.System, "JSON") {
		t.Fatalf("system prompt lost its rules:\n%s", req.System)
	}
}

func TestCreateValidatesReply(t *testing.T) {
	plan, err := ContentCreator{}.Create(context.Background(), &fakeRunner{replies: []aiprovider.PromptReply{{
		Text:     validCreatorReply,
		Metadata: map[string]any{"provider": "stub", "model": "stub-1"},
	}}}, AgentInput{Workspace: sampleWorkspace})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if plan.AgentID != ContentCreatorID || plan.Version != ContentCreatorVersion {
		t.Fatalf("plan lost its identity: %+v", plan)
	}
	if plan.Title != "Winter Maintenance for Real Coffee Machines" {
		t.Fatalf("bad title: %q", plan.Title)
	}
	if plan.Slug != "winter-maintenance-real-coffee" {
		t.Fatalf("bad slug: %q", plan.Slug)
	}
	if plan.Provider != "stub" || plan.Model != "stub-1" {
		t.Fatalf("provider/model lost: %s/%s", plan.Provider, plan.Model)
	}
	if len(plan.KeyPoints) != 2 {
		t.Fatalf("bad key points: %+v", plan.KeyPoints)
	}
}

func TestCreateRejectsMalformedReplies(t *testing.T) {
	cases := map[string]aiprovider.PromptReply{
		"no json":         {Text: "a chatty paragraph with no object"},
		"bad json":        {Text: `{"title":"x","body":"`},
		"no title":        {Text: `{"title":"","slug":"s","excerpt":"","body":"words","callToAction":"","keyPoints":[]}`},
		"no body":         {Text: `{"title":"t","slug":"s","excerpt":"","body":"","callToAction":"","keyPoints":[]}`},
		"whitespace body": {Text: `{"title":"t","body":"   \n  "}`},
	}
	for name, reply := range cases {
		_, err := ContentCreator{}.Create(context.Background(), &fakeRunner{replies: []aiprovider.PromptReply{reply}}, AgentInput{})
		if err == nil {
			t.Fatalf("%s: expected an error, got a plan", name)
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

func TestCreateSanitizesAndSuggests(t *testing.T) {
	// A bad slug is dropped, not fatal; over-cap fields are trimmed.
	reply := aiprovider.PromptReply{
		Text: `{"title":"` + strings.Repeat("x", 300) + `","slug":"Not A Slug","excerpt":"` + strings.Repeat("e", 900) +
			`","body":"` + strings.Repeat("b", 30000) + `","callToAction":"","keyPoints":["p","",""]}`,
	}
	plan, err := ContentCreator{}.Create(context.Background(), &fakeRunner{replies: []aiprovider.PromptReply{reply}}, AgentInput{})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if runeCount(plan.Title) != maxContentTitleRunesCreator {
		t.Fatalf("title not capped: %d", runeCount(plan.Title))
	}
	if runeCount(plan.Body) != maxContentBodyRunes {
		t.Fatalf("body not capped: %d", runeCount(plan.Body))
	}
	if runeCount(plan.Excerpt) != maxContentExcerptRunes {
		t.Fatalf("excerpt not capped: %d", runeCount(plan.Excerpt))
	}
	if plan.Slug != "" {
		t.Fatalf("bad slug kept: %q", plan.Slug)
	}
	if len(plan.KeyPoints) != 1 {
		t.Fatalf("empty key points not dropped: %+v", plan.KeyPoints)
	}
}
