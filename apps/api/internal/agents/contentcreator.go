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

// ---- Content-creator specialist profile -------------------------------------

// ContentCreatorID and ContentCreatorVersion identify the curated profile.
const (
	ContentCreatorID      = "content_creator"
	ContentCreatorVersion = "1"
)

// Input caps keep one run and its stored audit tractable. The plan body cap
// exists so a rogue or looping model cannot ship megabytes into a draft
// version or the audit trail.
const (
	maxContentTitleRunesCreator = 200
	maxContentBodyRunes         = 20000
	maxContentExcerptRunes      = 600
	maxContentCTARunes          = 300
	maxKeyPoints                = 8
	maxKeyPointRunes            = 200
)

var slugReCreator = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// ContentPlan is the validated output of one drafting-specialist run. It is a
// draft: the platform never publishes from an agent run, it can only create or
// update a draft content item that the existing approval workflow then gates.
type ContentPlan struct {
	AgentID      string    `json:"agentId"`
	Version      string    `json:"agentVersion"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	GeneratedAt  time.Time `json:"generatedAt"`
	Title        string    `json:"title"`
	Slug         string    `json:"slug"`
	Excerpt      string    `json:"excerpt"`
	Body         string    `json:"body"`
	CallToAction string    `json:"callToAction"`
	KeyPoints    []string  `json:"keyPoints"`
}

// targetQueryKeyPointPrefix marks the draft's chosen Search Console query, so
// the operator sees exactly which real query a data-informed piece targets.
const TargetQueryKeyPointPrefix = "Target query: "

// ContentCreator composes original drafts (blog articles, social posts,
// snippets) for one workspace, in the brand's tone, inside the workspace's
// prohibited-claims list. Like every specialist it is bounded: no invented
// business facts, no publishing, and output validation before anyone sees it.
type ContentCreator struct{}

// ID and Version report the profile identity recorded in task history.
func (ContentCreator) ID() string      { return ContentCreatorID }
func (ContentCreator) Version() string { return ContentCreatorVersion }

// DisplayName and Description are the dashboard-facing identity of this
// curated profile.
func (ContentCreator) DisplayName() string { return "Content creator" }
func (ContentCreator) Description() string {
	return "Composes original draft content pieces in the brand's voice, for review in the approval workflow."
}

// OutputContract, WantsData, and AllowedActions declare the governance facts
// of this specialist: drafting output filed via the file_draft executor, and
// no external data needs.
func (ContentCreator) OutputContract() string   { return ContractDrafting }
func (ContentCreator) WantsData() []string      { return nil }
func (ContentCreator) AllowedActions() []string { return []string{ActionFileDraft} }

// Run dispatches one drafting pass. The file_draft executor is applied after
// the run, by the route; this method only produces the validated plan.
func (c ContentCreator) Run(ctx context.Context, run PromptRun, input RunInput) (RunOutput, error) {
	plan, err := c.Create(ctx, run, AgentInput{
		Workspace: input.Workspace,
		Focus:     input.Focus,
	})
	if err != nil {
		return RunOutput{}, err
	}
	return RunOutput{Plan: &plan}, nil
}

// Create runs one drafting pass: build the bounded prompt, transport it via
// the provider, then validate the reply before it reaches an operator.
func (c ContentCreator) Create(ctx context.Context, run PromptRun, input AgentInput) (ContentPlan, error) {
	reply, err := run.RunPrompt(ctx, aiprovider.RunPromptRequest{
		System:    c.systemPrompt(),
		User:      c.buildUserPrompt(input),
		MaxTokens: 4096,
	})
	if err != nil {
		return ContentPlan{}, err
	}
	plan, err := planFromReply(reply, nil)
	if err != nil {
		return ContentPlan{}, err
	}
	plan.AgentID = c.ID()
	plan.Version = c.Version()
	plan.GeneratedAt = time.Now().UTC()
	if plan.Provider == "" {
		plan.Provider = reply.Provider
		plan.Model = reply.Model
	}
	return plan, nil
}

// systemPrompt is the curated, versioned specialist profile for original
// content composition, independent of (and complementary to) the SEO advisor.
func (c ContentCreator) systemPrompt() string {
	return strings.Join([]string{
		`You are the internal content creator of a marketing operations platform. You write one original draft content piece for a small business workspace, for a human operator to review and approve before anything is published.`,
		`Rules you must never break:`,
		`1. ORIGINAL WORK ONLY. Write new prose; never reproduce long excerpts from third-party sources or known articles. Do not fabricate quotes, customer testimonials, statistics, awards, certifications, or press mentions.`,
		`2. FACTS FROM CONTEXT ONLY. Base the piece on the provided workspace context below. Invent strictly nothing about the business's history, staff, location, pricing, or achievements. Where a real detail would strengthen the piece but is missing, mark it as a gap by keeping the claim generic or writing [to confirm: ...] placeholders instead of inventing it.`,
		`3. RESPECT THE BRAND. Match the workspace's tone and audience. Never write any claim from the "Do not make these claims" list, and avoid prohibited-adjacent superlatives (best, #1, guaranteed).`,
		`4. DRAFT ONLY. You produce text; you do not publish, schedule, send, or claim to have published anything anywhere. The operator decides what happens next.`,
		`5. OUTPUT. Answer with a single JSON object and nothing else, with exactly these keys:`,
		`{"title": string, "slug": string, "excerpt": string, "body": string, "callToAction": string, "keyPoints": [string]}`,
		`"title" is at most 200 characters. "slug" is lowercase letters, digits, and single hyphens only (no leading/trailing hyphen). "excerpt" is a one-sentence teaser. "body" is the main content in Markdown: headings (##), short paragraphs, at most 900 words; for social_post or snippet kinds write the post/snippet text itself in "body". "callToAction" is one short closing action for the reader, or "" when the kind does not need one. "keyPoints" lists at most 8 short bullets summarizing the piece's points, ordered by importance.`,
	}, "\n")
}

// buildUserPrompt renders the bounded, non-secret context of this run.
// Credentials, tokens, and any secret-derived data never appear here.
func (c ContentCreator) buildUserPrompt(input AgentInput) string {
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
	if ws.Tone != "" {
		fmt.Fprintf(&out, "Tone: %s\n", ws.Tone)
	}
	if len(ws.ProhibitedClaims) > 0 {
		fmt.Fprintf(&out, "Do not make these claims: %s\n", strings.Join(ws.ProhibitedClaims, "; "))
	}
	if strings.TrimSpace(input.Focus) != "" {
		fmt.Fprintf(&out, "Brief for this piece: %s\n", strings.TrimSpace(input.Focus))
	}
	return out.String()
}

// creatorReplyShape mirrors the JSON the drafting system prompts demand: the
// plan fields plus the optional, honesty-checked target query. Shared by
// every drafting specialist.
type creatorReplyShape struct {
	Title        string   `json:"title"`
	Slug         string   `json:"slug"`
	Excerpt      string   `json:"excerpt"`
	Body         string   `json:"body"`
	CallToAction string   `json:"callToAction"`
	KeyPoints    []string `json:"keyPoints"`
	TargetQuery  string   `json:"targetQuery,omitempty"`
}

// planFromReply turns raw provider text into a validated ContentPlan for any
// drafting specialist.
//
// The data-honesty rule is enforced once, here, not per agent: a run that
// received Search Console rows may name a target query only if it is verbatim
// from those rows (and it is recorded as the first key point so the operator
// sees the basis); a run without rows must not name any query.
func planFromReply(reply aiprovider.PromptReply, rows *GSCSnapshot) (ContentPlan, error) {
	var parsed creatorReplyShape
	if err := json.Unmarshal(extractJSONObject([]byte(reply.Text)), &parsed); err != nil {
		return ContentPlan{}, &aiprovider.ProviderError{
			Kind:  aiprovider.FailMalformed,
			Cause: "the drafting reply could not be structured into a plan",
		}
	}

	target := strings.TrimSpace(parsed.TargetQuery)
	switch {
	case rows == nil && target != "":
		// The reply was given no rows; claiming a query is a factual error.
		return ContentPlan{}, &aiprovider.ProviderError{
			Kind:  aiprovider.FailMalformed,
			Cause: "the drafting reply named a Search Console query that was not provided to the run",
		}
	case rows != nil && !rows.containsQuery(target):
		return ContentPlan{}, &aiprovider.ProviderError{
			Kind:  aiprovider.FailMalformed,
			Cause: "the drafting reply's target query is not among the provided Search Console rows",
		}
	}

	plan := ContentPlan{
		Title:        truncateRunes(strings.TrimSpace(parsed.Title), maxContentTitleRunesCreator),
		Slug:         strings.ToLower(strings.TrimSpace(parsed.Slug)),
		Excerpt:      truncateRunes(strings.TrimSpace(parsed.Excerpt), maxContentExcerptRunes),
		Body:         truncateRunes(strings.TrimSpace(parsed.Body), maxContentBodyRunes),
		CallToAction: truncateRunes(strings.TrimSpace(parsed.CallToAction), maxContentCTARunes),
	}

	if plan.Title == "" || plan.Body == "" {
		return ContentPlan{}, &aiprovider.ProviderError{
			Kind:  aiprovider.FailMalformed,
			Cause: "the drafting reply was missing the draft title or body",
		}
	}
	if plan.Slug != "" && !slugReCreator.MatchString(plan.Slug) {
		plan.Slug = ""
	}

	points := make([]string, 0, min(len(parsed.KeyPoints), maxKeyPoints))
	for _, p := range parsed.KeyPoints {
		sp := truncateRunes(strings.TrimSpace(p), maxKeyPointRunes)
		if sp == "" {
			continue
		}
		points = append(points, sp)
		if len(points) == maxKeyPoints {
			break
		}
	}
	if target != "" {
		// The operator's evidence line comes first; the newest summary point
		// is dropped if the list would exceed its cap.
		marker := truncateRunes(TargetQueryKeyPointPrefix+target, maxKeyPointRunes)
		if len(points) == maxKeyPoints {
			points = points[:maxKeyPoints-1]
		}
		points = append([]string{marker}, points...)
	}
	plan.KeyPoints = points

	metadata := map[string]any{}
	for key, value := range reply.Metadata {
		if isScalar(value) {
			metadata[key] = value
		}
	}
	plan.Provider = stringFromMetadata(metadata, "provider")
	plan.Model = stringFromMetadata(metadata, "model")
	return plan, nil
}

// containsQuery reports whether q appears verbatim among the snapshot's
// page+query rows. Empty queries never match, so a plan cannot claim
// "targeting nothing" as data-informed.
func (s *GSCSnapshot) containsQuery(q string) bool {
	if strings.TrimSpace(q) == "" {
		return false
	}
	for _, row := range s.PageQueries {
		if row.Query == q {
			return true
		}
	}
	return false
}
