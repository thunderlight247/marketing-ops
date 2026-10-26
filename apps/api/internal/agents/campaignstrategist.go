package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"marketingops/api/internal/aiprovider"
)

// ---- Campaign-strategist specialist profile ---------------------------------

// CampaignStrategistID and CampaignStrategistVersion identify the curated
// profile.
const (
	CampaignStrategistID      = "campaign_strategist"
	CampaignStrategistVersion = "1"
)

// Input caps keep one run and its stored audit tractable, and mirror the
// caps the manual campaign form and its handlers already enforce.
const (
	maxCampaignNameRunes    = 120
	maxCampaignUTMRunes     = 100
	maxCampaignNotesRunes   = 2000
	maxCampaignKeyPoints    = 6
	maxCampaignKeyPointsRun = 200
)

// utmRe bounds the proposed UTM value to characters that are safe in a URL
// query value: lowercase letters, digits, hyphens, and underscores.
var utmRe = regexp.MustCompile(`^[a-z0-9-_]*$`)

// CampaignProposal is the validated output of one proposal-specialist run.
// It becomes one planned campaign via the create_campaign executor; the
// executor always forces status "planned", so no proposal ever arrives
// active, paused, or completed.
type CampaignProposal struct {
	AgentID     string    `json:"agentId"`
	Version     string    `json:"agentVersion"`
	Provider    string    `json:"provider"`
	Model       string    `json:"model"`
	GeneratedAt time.Time `json:"generatedAt"`
	Name        string    `json:"name"`
	UtmCampaign string    `json:"utmCampaign"`
	Notes       string    `json:"notes"`
	KeyPoints   []string  `json:"keyPoints"`
}

// CampaignStrategist proposes one new campaign for a workspace: what to run,
// a suggested UTM value, and a rationale grounded only in the brand context
// and the operator brief. Like every specialist it is bounded: no invented
// business facts, no performance data it was not given, and a proposal that
// stays planned until the operator activates it.
type CampaignStrategist struct{}

// ID and Version report the profile identity recorded in task history.
func (CampaignStrategist) ID() string      { return CampaignStrategistID }
func (CampaignStrategist) Version() string { return CampaignStrategistVersion }

// DisplayName and Description are the dashboard-facing identity of this
// curated profile.
func (CampaignStrategist) DisplayName() string { return "Campaign strategist" }

func (CampaignStrategist) Description() string {
	return "Proposes one planned campaign for the workspace, rationale included, for operator review."
}

// OutputContract, WantsData, and AllowedActions declare the governance facts
// of this specialist: a proposal filed via the create_campaign executor, and
// no external data needs (so it can never cite data it was not given).
func (CampaignStrategist) OutputContract() string   { return ContractProposal }
func (CampaignStrategist) WantsData() []string      { return nil }
func (CampaignStrategist) AllowedActions() []string { return []string{ActionCreateCampaign} }

// Run performs one bounded proposal pass. The create_campaign executor is
// applied after the run, by the route; this method only produces the
// validated proposal.
func (c CampaignStrategist) Run(ctx context.Context, run PromptRun, input RunInput) (RunOutput, error) {
	proposal, err := c.Propose(ctx, run, AgentInput{
		Workspace: input.Workspace,
		Focus:     input.Focus,
	})
	if err != nil {
		return RunOutput{}, err
	}
	return RunOutput{Proposal: &proposal}, nil
}

// Propose runs one proposal pass: build the bounded prompt, transport it via
// the provider, then validate the reply before it is filed anywhere.
func (c CampaignStrategist) Propose(ctx context.Context, run PromptRun, input AgentInput) (CampaignProposal, error) {
	reply, err := run.RunPrompt(ctx, aiprovider.RunPromptRequest{
		System:    c.systemPrompt(),
		User:      c.buildUserPrompt(input),
		MaxTokens: 2048,
	})
	if err != nil {
		return CampaignProposal{}, err
	}
	proposal, err := proposalFromReply(reply)
	if err != nil {
		return CampaignProposal{}, err
	}
	proposal.AgentID = c.ID()
	proposal.Version = c.Version()
	proposal.GeneratedAt = time.Now().UTC()
	if proposal.Provider == "" {
		proposal.Provider = reply.Provider
		proposal.Model = reply.Model
	}
	return proposal, nil
}

// systemPrompt is the curated, versioned specialist profile for campaign
// planning, independent of (and complementary to) the drafting specialists.
func (c CampaignStrategist) systemPrompt() string {
	return strings.Join([]string{
		`You are the internal campaign strategist of a marketing operations platform. You propose exactly one new marketing campaign for a small business workspace, as a planned record a human operator reviews and decides about.`,
		`Rules you must never break:`,
		`1. PROPOSAL ONLY. You propose; you do not activate, launch, publish, schedule, or claim that anything will run. Your proposal always arrives with status "planned", and the operator alone decides whether it becomes active.`,
		`2. FACTS FROM CONTEXT ONLY. Base the proposal on the provided workspace context below. Invent strictly nothing about the business's history, staff, location, pricing, achievements, or audience size. Where a real detail would strengthen the campaign but is missing, write [to confirm: ...] placeholders in "notes" instead of inventing it.`,
		`3. NO PERFORMANCE DATA. You are given no Search Console or analytics rows; never cite positions, clicks, impressions, rankings, traffic, dates, or any measured number. Reason from the business's offer and audience only.`,
		`4. RESPECT THE BRAND. The campaign angle must match the workspace's tone and audience and must not require any claim from the "Do not make these claims" list.`,
		`5. OUTPUT. Answer with a single JSON object and nothing else, with exactly these keys:`,
		`{"name": string, "utmCampaign": string, "notes": string, "keyPoints": [string]}`,
		`"name" is the campaign's display name, at most 120 characters, specific and non-duplicated from the workspace context, and it must not read as if the campaign already running. "utmCampaign" is the suggested UTM campaign value: lowercase letters, digits, hyphens, and underscores only (no spaces), at most 100 characters, or "" to leave attribution unset. "notes" is the rationale: what the campaign is for, the proposed angle, and every [to confirm: ...] placeholder, in plain text, at most 2000 characters. "keyPoints" lists at most 6 short bullets summarizing the proposal, ordered by importance.`,
	}, "\n")
}

// buildUserPrompt renders the bounded, non-secret context of this run.
// Credentials, tokens, and any secret-derived data never appear here.
func (c CampaignStrategist) buildUserPrompt(input AgentInput) string {
	var out strings.Builder
	ws := input.Workspace
	fmt.Fprintf(&out, "Business: %s", fallback(ws.Name, "(unknown business)"))
	if ws.WebsiteURL != "" {
		fmt.Fprintf(&out, " (%s)", ws.WebsiteURL)
	}
	out.WriteString("\n")
	if ws.Description != "" {
		fmt.Fprintf(&out, "About: %s\n", ws.Description)
	}
	if ws.Audience != "" {
		fmt.Fprintf(&out, "Audience: %s\n", ws.Audience)
	}
	if len(ws.Services) > 0 {
		fmt.Fprintf(&out, "Services: %s\n", strings.Join(ws.Services, ", "))
	}
	if ws.CallsToAction != nil && len(ws.CallsToAction) > 0 {
		fmt.Fprintf(&out, "Preferred calls to action: %s\n", strings.Join(ws.CallsToAction, "; "))
	}
	if ws.Tone != "" {
		fmt.Fprintf(&out, "Tone: %s\n", ws.Tone)
	}
	if len(ws.ProhibitedClaims) > 0 {
		fmt.Fprintf(&out, "Do not make these claims: %s\n", strings.Join(ws.ProhibitedClaims, "; "))
	}
	if strings.TrimSpace(input.Focus) != "" {
		fmt.Fprintf(&out, "Operator focus for this campaign: %s\n", strings.TrimSpace(input.Focus))
	}
	return out.String()
}

// strategistReplyShape mirrors the JSON the proposal system prompt demands.
type strategistReplyShape struct {
	Name        string   `json:"name"`
	UtmCampaign string   `json:"utmCampaign"`
	Notes       string   `json:"notes"`
	KeyPoints   []string `json:"keyPoints"`
}

// proposalFromReply turns raw provider text into a validated CampaignProposal
// or a safe failure. Caps mirror the manual campaign form's own limits so an
// agent proposal and an operator draft behave the same downstream.
func proposalFromReply(reply aiprovider.PromptReply) (CampaignProposal, error) {
	var parsed strategistReplyShape
	if err := json.Unmarshal(extractJSONObject([]byte(reply.Text)), &parsed); err != nil {
		return CampaignProposal{}, &aiprovider.ProviderError{
			Kind:  aiprovider.FailMalformed,
			Cause: "the proposal reply could not be structured into a campaign plan",
		}
	}

	name := truncateRunes(strings.TrimSpace(parsed.Name), maxCampaignNameRunes)
	if name == "" {
		return CampaignProposal{}, &aiprovider.ProviderError{
			Kind:  aiprovider.FailMalformed,
			Cause: "the proposal reply was missing the campaign name",
		}
	}

	utm := strings.ToLower(strings.TrimSpace(parsed.UtmCampaign))
	if utm != "" && !utmRe.MatchString(utm) {
		// An unusable UTM value is dropped, not fatal: attribution stays
		// unset, which the store and the panel already handle.
		utm = ""
	}
	utm = truncateRunes(utm, maxCampaignUTMRunes)

	notes := truncateRunes(strings.TrimSpace(parsed.Notes), maxCampaignNotesRunes)

	points := make([]string, 0, min(len(parsed.KeyPoints), maxCampaignKeyPoints))
	for _, p := range parsed.KeyPoints {
		sp := truncateRunes(strings.TrimSpace(p), maxCampaignKeyPointsRun)
		if sp == "" {
			continue
		}
		points = append(points, sp)
		if len(points) == maxCampaignKeyPoints {
			break
		}
	}

	metadata := map[string]any{}
	for key, value := range reply.Metadata {
		if isScalar(value) {
			metadata[key] = value
		}
	}

	return CampaignProposal{
		Provider:    stringFromMetadata(metadata, "provider"),
		Model:       stringFromMetadata(metadata, "model"),
		Name:        name,
		UtmCampaign: utm,
		Notes:       notes,
		KeyPoints:   points,
	}, nil
}
