package agents

import (
	"context"
	"errors"
	"strings"
	"testing"

	"marketingops/api/internal/aiprovider"
)

// validChannelCopyReply is provider text the model could produce for a
// channel-copy run over an existing draft item.
const validChannelCopyReply = `Here are the variants: {"channelCopy":{` +
	`"facebook":"Open all winter. Book November service now.","x":"Winter service. Book today."}}`

func TestChannelCopyWriterRunBuildsBoundedPrompt(t *testing.T) {
	runner := &fakeRunner{replies: []aiprovider.PromptReply{{
		Text:     validChannelCopyReply,
		Metadata: map[string]any{"provider": "stub", "model": "stub-1"},
	}}}
	// Run is what the route calls: the target draft's bounded context arrives
	// prepared, inside RunInput.Focus, and no data source is requested.
	if _, err := (ChannelCopyWriter{}).Run(context.Background(), runner, RunInput{
		Workspace: sampleWorkspace,
		Focus:     "Draft to adapt — Title: Winter service. Body excerpt: Booking now for November furnace checks.",
	}); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one prompt call, got %d", len(runner.calls))
	}
	req := runner.calls[0]
	if !strings.Contains(req.User, "Business: Demo Cafe (https://demo.example)") {
		t.Fatalf("user prompt lost the workspace context:\n%s", req.User)
	}
	if !strings.Contains(req.User, "Draft to adapt — Title: Winter service") {
		t.Fatalf("user prompt lost the draft context:\n%s", req.User)
	}
	if strings.Contains(req.User, "Search Console") {
		t.Fatalf("a no-data specialist must not promise data it will not get:\n%s", req.User)
	}
	for _, want := range [...]string{"FILE NOTHING NEW", "NO PERFORMANCE DATA", "JSON"} {
		if !strings.Contains(req.System, want) {
			t.Fatalf("system prompt lost the %q rule:\n%s", want, req.System)
		}
	}
}

func TestChannelCopyWriterClosedChannelSet(t *testing.T) {
	got := validChannelCopyChannels()
	want := []string{"facebook", "instagram", "x", "youtube"}
	if len(got) != len(want) {
		t.Fatalf("closed channel set drifted: %v", got)
	}
	for i, name := range want {
		if got[i] != name || !channelCopyChannels[name] {
			t.Fatalf("closed channel set drifted: %v", got)
		}
	}
}

func TestDraftChannelCopyValidatesReply(t *testing.T) {
	plan, err := ChannelCopyWriter{}.Draft(context.Background(), &fakeRunner{replies: []aiprovider.PromptReply{{
		Text:     validChannelCopyReply,
		Metadata: map[string]any{"provider": "stub", "model": "stub-1"},
	}}}, AgentInput{Workspace: sampleWorkspace, Focus: "Title: Winter service"})
	if err != nil {
		t.Fatalf("Draft failed: %v", err)
	}
	if plan.AgentID != ChannelCopyWriterID || plan.Version != ChannelCopyWriterVersion {
		t.Fatalf("plan lost its identity: %+v", plan)
	}
	if plan.Provider != "stub" || plan.Model != "stub-1" {
		t.Fatalf("provider/model lost: %s/%s", plan.Provider, plan.Model)
	}
	if len(plan.Channels) != 2 ||
		plan.Channels["facebook"] != "Open all winter. Book November service now." ||
		plan.Channels["x"] != "Winter service. Book today." {
		t.Fatalf("bad copy map: %+v", plan.Channels)
	}
}

func TestDraftChannelCopyRejectsMalformedReplies(t *testing.T) {
	cases := map[string]string{
		"no json":         "a chatty paragraph with no object",
		"bad json":        `{"channelCopy":{`,
		"wrong key shape": `{"copy":{"facebook":"x msg"}}`,
		"nothing at all":  `{"channelCopy":{}}`,
		"unknown channel": `{"channelCopy":{"twitter":"msg"}}`,
		"not a string":    `{"channelCopy":{"facebook":42}}`,
	}
	for name, text := range cases {
		_, err := ChannelCopyWriter{}.Draft(context.Background(), &fakeRunner{replies: []aiprovider.PromptReply{{
			Text: text,
		}}}, AgentInput{Workspace: sampleWorkspace})
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

func TestDraftChannelCopyEnforcesMessageCap(t *testing.T) {
	// 281 characters including the trailing space: overflow is refused, not
	// silently truncated.
	reply := aiprovider.PromptReply{
		Text: `{"channelCopy":{"facebook":"` + strings.Repeat("f", 281) + `"}}`,
	}
	_, err := ChannelCopyWriter{}.Draft(context.Background(), &fakeRunner{replies: []aiprovider.PromptReply{reply}},
		AgentInput{Workspace: sampleWorkspace})
	var perr *aiprovider.ProviderError
	if !errors.As(err, &perr) || perr.Kind != aiprovider.FailMalformed {
		t.Fatalf("expected FailMalformed for an over-cap message, got %v", err)
	}
	if !strings.Contains(perr.Cause, "280") {
		t.Fatalf("error cause should name the cap: %q", perr.Cause)
	}
}

func TestDraftChannelCopyTrimsAndDropsBlanks(t *testing.T) {
	reply := aiprovider.PromptReply{
		Text: `{"channelCopy":{"facebook":"  Ready to post text  ","x":"   ","instagram":"Another variant"}}`,
	}
	plan, err := ChannelCopyWriter{}.Draft(context.Background(), &fakeRunner{replies: []aiprovider.PromptReply{reply}},
		AgentInput{Workspace: sampleWorkspace})
	if err != nil {
		t.Fatalf("Draft failed: %v", err)
	}
	if plan.Channels["facebook"] != "Ready to post text" {
		t.Fatalf("message not trimmed: %q", plan.Channels["facebook"])
	}
	if _, ok := plan.Channels["x"]; ok {
		t.Fatalf("blank message must be dropped, not filed: %+v", plan.Channels)
	}
	if plan.Channels["instagram"] != "Another variant" {
		t.Fatalf("unexpected instagram copy: %q", plan.Channels["instagram"])
	}
}
