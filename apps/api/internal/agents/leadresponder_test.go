package agents

import (
	"context"
	"errors"
	"strings"
	"testing"

	"marketingops/api/internal/aiprovider"
)

// validLeadReply is provider text a model could produce for a first reply.
const validLeadReply = `Here is your reply: {"subject":"Re: furnace check on the north side",` +
	`"body":"Hi Jane, thanks for the note about the north side and the winter furnace check. ` +
	`I'll confirm the visit window and the details with you before anything is booked."}`

func leadReplyPlan(t *testing.T, reply string, leadEmail string) (LeadReply, error) {
	t.Helper()
	return LeadResponder{}.Draft(context.Background(), &fakeRunner{replies: []aiprovider.PromptReply{{
		Text:     reply,
		Metadata: map[string]any{"provider": "stub", "model": "stub-1"},
	}}}, LeadReplyInput{
		Workspace: sampleWorkspace,
		Focus:     "Reply to this lead\nContact name: Jane\nEnquiry (the lead's own words): Do you service the north side?",
		LeadEmail: leadEmail,
	})
}

func TestLeadReplyBuildsBoundedPrompt(t *testing.T) {
	runner := &fakeRunner{replies: []aiprovider.PromptReply{{Text: validLeadReply}}}
	if _, err := (LeadResponder{}).Draft(context.Background(), runner, LeadReplyInput{
		Workspace: sampleWorkspace,
		Focus:     "Reply to this lead\nEnquiry (the lead's own words): Do you service the north side?",
		LeadEmail: "jane@example.com",
	}); err != nil {
		t.Fatalf("Draft failed: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one prompt call, got %d", len(runner.calls))
	}
	req := runner.calls[0]
	if !strings.HasPrefix(req.System, "You are the internal lead responder ") {
		t.Fatalf("system prompt lost the persona prefix the offline stub matches:\n%s", req.System)
	}
	for _, want := range [...]string{"REAL ENQUIRY ONLY", "NO INVENTED COMMITMENTS", "DO NOT CLAIM ACTIONS", "NO PERFORMANCE DATA", "CONTACT DETAILS"} {
		if !strings.Contains(req.System, want) {
			t.Fatalf("system prompt lost the %q rule:\n%s", want, req.System)
		}
	}
	if !strings.Contains(req.User, "Business: Demo Cafe (https://demo.example)") {
		t.Fatalf("user prompt lost the workspace context:\n%s", req.User)
	}
	if !strings.Contains(req.User, "Do you service the north side?") {
		t.Fatalf("user prompt lost the lead's own words:\n%s", req.User)
	}
	if strings.Contains(req.User, "Search Console") {
		t.Fatalf("a no-data specialist must not promise data it will not get:\n%s", req.User)
	}
}

func TestLeadReplyValidatesReply(t *testing.T) {
	plan, err := leadReplyPlan(t, validLeadReply, "jane@example.com")
	if err != nil {
		t.Fatalf("Draft failed: %v", err)
	}
	if plan.AgentID != LeadResponderID || plan.Version != LeadResponderVersion {
		t.Fatalf("plan lost its identity: %+v", plan)
	}
	if plan.Subject != "Re: furnace check on the north side" ||
		!strings.HasPrefix(plan.Body, "Hi Jane, thanks for the note") {
		t.Fatalf("bad reply: %+v", plan)
	}
	if plan.Provider != "stub" || plan.Model != "stub-1" {
		t.Fatalf("provider/model lost: %s/%s", plan.Provider, plan.Model)
	}
}

// TestLeadReplyDataHonesty pins the rule that matters for a real enquiry: a
// drafted reply may not introduce a contact address the lead never supplied.
func TestLeadReplyDataHonesty(t *testing.T) {
	// The lead's own address may be quoted back.
	if _, err := leadReplyPlan(t,
		`{"subject":"Thanks","body":"I'll reply here: jane@example.com."}`, "jane@example.com"); err != nil {
		t.Fatalf("the lead's own address must be allowed: %v", err)
	}
	cases := map[string]struct {
		reply     string
		leadEmail string
	}{
		"invented address": {
			reply:     `{"subject":"Thanks","body":"Email us at sales@demo.example any time."}`,
			leadEmail: "jane@example.com",
		},
		"address when the lead gave none": {
			reply:     `{"subject":"Thanks","body":"Call sales@demo.example."}`,
			leadEmail: "",
		},
		"second address alongside the real one": {
			reply:     `{"subject":"Thanks","body":"Jane, or reach sales@demo.example."}`,
			leadEmail: "jane@example.com",
		},
	}
	for name, tc := range cases {
		_, err := leadReplyPlan(t, tc.reply, tc.leadEmail)
		if err == nil {
			t.Fatalf("%s: expected a rejection", name)
		}
		var perr *aiprovider.ProviderError
		if !errors.As(err, &perr) || perr.Kind != aiprovider.FailMalformed {
			t.Fatalf("%s: expected FailMalformed, got %v", name, err)
		}
	}
}

func TestLeadReplyRejectsMalformedReplies(t *testing.T) {
	cases := map[string]string{
		"no json":       "a chatty paragraph with no object",
		"bad json":      `{"subject":"`,
		"no subject":    `{"subject":"","body":"Hi"}`,
		"blank subject": `{"subject":"   ","body":"Hi"}`,
		"no body":       `{"subject":"Thanks","body":""}`,
		"over-long subject": `{"subject":"` + strings.Repeat("s", maxLeadReplySubjectRunes+1) +
			`","body":"Hi"}`,
		"over-long body": `{"subject":"Thanks","body":"` + strings.Repeat("b", maxLeadReplyBodyRunes+1) + `"}`,
	}
	for name, reply := range cases {
		_, err := leadReplyPlan(t, reply, "jane@example.com")
		if err == nil {
			t.Fatalf("%s: expected a rejection", name)
		}
		var perr *aiprovider.ProviderError
		if !errors.As(err, &perr) || perr.Kind != aiprovider.FailMalformed {
			t.Fatalf("%s: expected FailMalformed, got %v", name, err)
		}
	}
	// Exactly at the caps is allowed (no silent truncation either way).
	if _, err := leadReplyPlan(t,
		`{"subject":"`+strings.Repeat("s", maxLeadReplySubjectRunes)+`","body":"`+
			strings.Repeat("b", maxLeadReplyBodyRunes)+`"}`, ""); err != nil {
		t.Fatalf("a reply at the caps must be accepted: %v", err)
	}
}

// TestLeadResponderRunWrapsDraft keeps the specialist's contract wiring in
// place: Run returns exactly the lead_reply output and nothing else.
func TestLeadResponderRunWrapsDraft(t *testing.T) {
	out, err := (LeadResponder{}).Run(context.Background(),
		&fakeRunner{replies: []aiprovider.PromptReply{{Text: validLeadReply}}},
		RunInput{Workspace: sampleWorkspace, Focus: "Reply to this lead"})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if out.Reply == nil || out.Report != nil || out.Plan != nil || out.Proposal != nil || out.Copy != nil {
		t.Fatalf("unexpected output union: %+v", out)
	}
	if out.Reply.AgentID != LeadResponderID {
		t.Fatalf("identity lost: %+v", out.Reply)
	}
}
