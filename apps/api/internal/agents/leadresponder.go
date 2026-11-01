package agents

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"marketingops/api/internal/aiprovider"
)

// ---- Lead-responder specialist profile --------------------------------------

// LeadResponderID and LeadResponderVersion identify the curated profile.
const (
	LeadResponderID      = "lead_responder"
	LeadResponderVersion = "1"
)

// Caps for one drafted reply. The body cap keeps a first reply short enough to
// read in one screen (and well inside the run payload and prompt budgets);
// overflow is refused, not truncated, so the operator never reviews silently
// shortened prose.
const (
	maxLeadReplySubjectRunes = 200
	maxLeadReplyBodyRunes    = 1500
)

// emailPattern finds anything that looks like an email address in a drafted
// reply. Only the lead's own address may appear (see leadReplyFromReply).
var emailPattern = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)

// LeadReply is the validated output of one lead-reply run: a subject and body
// stored on an existing lead for operator review. The platform never sends it,
// and storing it never changes the lead's status, source, or attribution.
type LeadReply struct {
	AgentID     string    `json:"agentId"`
	Version     string    `json:"agentVersion"`
	Provider    string    `json:"provider"`
	Model       string    `json:"model"`
	GeneratedAt time.Time `json:"generatedAt"`
	Subject     string    `json:"subject"`
	Body        string    `json:"body"`
}

// LeadResponder drafts a first reply to one real, existing enquiry. It is the
// demand-side counterpart of the drafting specialists: it writes nothing new
// about the business, invents no commitments, and refuses to introduce any
// contact detail the lead did not supply.
type LeadResponder struct{}

// ID and Version report the profile identity recorded in task history.
func (LeadResponder) ID() string      { return LeadResponderID }
func (LeadResponder) Version() string { return LeadResponderVersion }

// DisplayName and Description are the dashboard-facing identity of this
// curated profile.
func (LeadResponder) DisplayName() string { return "Lead responder" }

func (LeadResponder) Description() string {
	return "Drafts a first reply to one recorded lead, stored on that lead for the operator to review and send."
}

// OutputContract, WantsData, and AllowedActions declare the governance facts
// of this specialist: a lead-reply draft filed onto one existing lead via the
// file_lead_reply executor, and no external data needs.
func (LeadResponder) OutputContract() string   { return ContractLeadReply }
func (LeadResponder) WantsData() []string      { return nil }
func (LeadResponder) AllowedActions() []string { return []string{ActionFileLeadReply} }

// LeadReplyInput is the bounded context of one lead-reply run: the workspace
// facts, the route-prepared brief (the lead's own words, source, campaign,
// status), and the lead's own address — the only address a reply may contain.
type LeadReplyInput struct {
	Workspace aiprovider.WorkspaceContext
	Focus     string
	LeadEmail string
}

// Run performs one bounded reply-drafting pass. RunInput.Focus carries the
// target lead's bounded context (its own words, source, campaign, status),
// prepared by the route before any model call; the executor that stores the
// reply runs afterwards, in the route.
func (r LeadResponder) Run(ctx context.Context, run PromptRun, input RunInput) (RunOutput, error) {
	reply, err := r.Draft(ctx, run, LeadReplyInput{
		Workspace: input.Workspace,
		Focus:     input.Focus,
		LeadEmail: input.LeadEmail,
	})
	if err != nil {
		return RunOutput{}, err
	}
	return RunOutput{Reply: &reply}, nil
}

// Draft runs one reply pass: build the bounded prompt, transport it via the
// provider, then validate the reply before it is stored anywhere. The lead's
// own email address is the only address the reply may contain.
func (r LeadResponder) Draft(ctx context.Context, run PromptRun, input LeadReplyInput) (LeadReply, error) {
	reply, err := run.RunPrompt(ctx, aiprovider.RunPromptRequest{
		System:    r.systemPrompt(),
		User:      r.buildUserPrompt(input),
		MaxTokens: 1024,
	})
	if err != nil {
		return LeadReply{}, err
	}
	plan, err := leadReplyFromReply(reply, input.LeadEmail)
	if err != nil {
		return LeadReply{}, err
	}
	plan.AgentID = r.ID()
	plan.Version = r.Version()
	plan.GeneratedAt = time.Now().UTC()
	if plan.Provider == "" {
		plan.Provider = reply.Provider
		plan.Model = reply.Model
	}
	return plan, nil
}

// systemPrompt is the curated, versioned specialist profile for first replies
// to real enquiries.
func (r LeadResponder) systemPrompt() string {
	return strings.Join([]string{
		`You are the internal lead responder of a marketing operations platform. You draft one short first reply to a real enquiry that a human operator has already recorded, so the operator can review, edit, and send it themselves.`,
		`Rules you must never break:`,
		`1. REAL ENQUIRY ONLY. Reply to the lead described below and to nothing else. Never invent a person, a company, an enquiry, or a contact detail. Never write a reply to a lead you were not given.`,
		`2. NO INVENTED COMMITMENTS. You do not know prices, availability, opening hours, dates, staff, certifications, or the outcome of any request. Do not state or imply them. Where a real detail is needed, say that the operator will confirm it: for example "I'll confirm the details for you" rather than a number.`,
		`3. DO NOT CLAIM ACTIONS. You are drafting text, not sending it. Never write that you have already answered, called, booked, refunded, shipped, or scheduled anything, and never claim the business has responded.`,
		`4. NO PERFORMANCE DATA. You are given no analytics, Search Console, or campaign-results rows. Never cite positions, clicks, traffic, conversion rates, or any measured number, and never promise a result.`,
		`5. CONTACT DETAILS. Use only the lead's own email address if you must reference a contact channel; never introduce any other address, phone number, or link that was not provided.`,
		`6. RESPECT THE BUSINESS. Write in the workspace's tone, use only facts from the provided context and the lead's own words, add no claim from the "Do not make these claims" list, and keep it free of marketing exaggeration.`,
		`7. OUTPUT. Answer with a single JSON object and nothing else, with exactly these keys:`,
		`{"subject": string, "body": string}`,
		`"subject" is at most 200 characters with no "Re:" chain or placeholder such as [Name]. "body" is a plain-text reply, at most 1500 characters, opening with a greeting that uses the lead's given name when one was provided, acknowledging what the lead actually wrote, and closing with one clear next step for the operator to confirm. No Markdown headings, no bullet lists, no signature block, no placeholders — every bracket or blank the operator would have to fill in makes the draft unusable.`,
	}, "\n")
}

// buildUserPrompt renders the bounded, non-secret context of this run.
// Credentials, tokens, and any secret-derived data never appear here. The
// route already bounded the lead's own fields into Focus.
func (r LeadResponder) buildUserPrompt(input LeadReplyInput) string {
	var out strings.Builder
	ws := input.Workspace
	if ws.Name != "" {
		out.WriteString("Business: " + ws.Name)
		if ws.WebsiteURL != "" {
			out.WriteString(" (" + ws.WebsiteURL + ")")
		}
		out.WriteString("\n")
	}
	if ws.Description != "" {
		out.WriteString("About: " + ws.Description + "\n")
	}
	if ws.Audience != "" {
		out.WriteString("Audience: " + ws.Audience + "\n")
	}
	if len(ws.Services) > 0 {
		out.WriteString("Services: " + strings.Join(ws.Services, ", ") + "\n")
	}
	if ws.Tone != "" {
		out.WriteString("Tone: " + ws.Tone + "\n")
	}
	if len(ws.ProhibitedClaims) > 0 {
		out.WriteString("Do not make these claims: " + strings.Join(ws.ProhibitedClaims, "; ") + "\n")
	}
	if strings.TrimSpace(input.Focus) != "" {
		out.WriteString(strings.TrimSpace(input.Focus) + "\n")
	}
	return out.String()
}

// leadReplyFromReply turns raw provider text into a validated LeadReply.
// Data honesty is enforced here: the drafted reply must carry a subject and
// body within the caps, must contain no placeholder an operator would have to
// fill in, and must not introduce an email address other than the lead's own
// (an invented address would send a human reply nowhere).
func leadReplyFromReply(reply aiprovider.PromptReply, leadEmail string) (LeadReply, error) {
	var parsed struct {
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if err := json.Unmarshal(extractJSONObject([]byte(reply.Text)), &parsed); err != nil {
		return LeadReply{}, &aiprovider.ProviderError{
			Kind:  aiprovider.FailMalformed,
			Cause: "the lead-reply answer could not be structured into a subject and body",
		}
	}
	subject := strings.TrimSpace(parsed.Subject)
	body := strings.TrimSpace(parsed.Body)
	if subject == "" {
		return LeadReply{}, &aiprovider.ProviderError{
			Kind:  aiprovider.FailMalformed,
			Cause: "the lead-reply answer had no subject",
		}
	}
	if body == "" {
		return LeadReply{}, &aiprovider.ProviderError{
			Kind:  aiprovider.FailMalformed,
			Cause: "the lead-reply answer had no body",
		}
	}
	if runeCount(subject) > maxLeadReplySubjectRunes {
		return LeadReply{}, &aiprovider.ProviderError{
			Kind:  aiprovider.FailMalformed,
			Cause: "the lead-reply subject exceeds the 200-character cap",
		}
	}
	if runeCount(body) > maxLeadReplyBodyRunes {
		return LeadReply{}, &aiprovider.ProviderError{
			Kind:  aiprovider.FailMalformed,
			Cause: "the lead-reply body exceeds the 1500-character cap",
		}
	}
	if email := foreignEmail(subject+"\n"+body, leadEmail); email != "" {
		return LeadReply{}, &aiprovider.ProviderError{
			Kind: aiprovider.FailMalformed,
			Cause: "the lead-reply draft introduced a contact address that was not the lead's own: " +
				email,
		}
	}

	metadata := map[string]any{}
	for key, value := range reply.Metadata {
		if isScalar(value) {
			metadata[key] = value
		}
	}
	return LeadReply{
		Subject:  subject,
		Body:     body,
		Provider: stringFromMetadata(metadata, "provider"),
		Model:    stringFromMetadata(metadata, "model"),
	}, nil
}

// foreignEmail returns the first email address in text that is not the lead's
// own. When the lead supplied no address, any address at all is foreign.
func foreignEmail(text, leadEmail string) string {
	allowed := strings.ToLower(strings.TrimSpace(leadEmail))
	for _, found := range emailPattern.FindAllString(text, -1) {
		if allowed != "" && strings.ToLower(found) == allowed {
			continue
		}
		return found
	}
	return ""
}
