package agents

import (
	"context"
	"fmt"
	"strings"
	"time"

	"marketingops/api/internal/aiprovider"
)

// ---- Search-gap writer specialist profile -----------------------------------

// SearchGapWriterID and SearchGapWriterVersion identify the curated profile.
const (
	SearchGapWriterID      = "search_gap_writer"
	SearchGapWriterVersion = "1"
)

// SearchGapWriter drafts one original content piece aimed at a real, visible
// demand gap: a Search Console page+query row where searchers saw the
// workspace but did not click. It is the proof of the pluggable registry: it
// is a prompt profile + declared data need + the existing file_draft action —
// no third handler, route, loader, or store ever existed for it.
//
// Data honesty: the piece must target a query verbatim from the provided
// rows (validated in planFromReply using the rows) or, with no rows, draft
// from the workspace context only and record the missing data as a gap.
// Performance numbers are never inventable and never appear in the text.
type SearchGapWriter struct{}

// ID and Version report the profile identity recorded in task history.
func (SearchGapWriter) ID() string      { return SearchGapWriterID }
func (SearchGapWriter) Version() string { return SearchGapWriterVersion }

// DisplayName and Description are the dashboard-facing identity of this
// curated profile.
func (SearchGapWriter) DisplayName() string { return "Search-gap writer" }
func (SearchGapWriter) Description() string {
	return "Drafts one piece aimed at a real Search Console demand gap, for review in the approval workflow."
}

// OutputContract, WantsData, and AllowedActions declare the governance facts
// of this specialist: drafting output filed via file_draft and Search Console
// snapshots when a loader is configured (advisory on the work even without).
func (SearchGapWriter) OutputContract() string   { return ContractDrafting }
func (SearchGapWriter) WantsData() []string      { return []string{DataNeedSearchConsole} }
func (SearchGapWriter) AllowedActions() []string { return []string{ActionFileDraft} }

// Run dispatches one pass, unpacking only the declared data need.
func (s SearchGapWriter) Run(ctx context.Context, run PromptRun, input RunInput) (RunOutput, error) {
	snapshot, _ := input.Data[DataNeedSearchConsole].(*GSCSnapshot)
	reply, err := run.RunPrompt(ctx, aiprovider.RunPromptRequest{
		System:    s.systemPrompt(),
		User:      s.buildUserPrompt(input, snapshot),
		MaxTokens: 4096,
	})
	if err != nil {
		return RunOutput{}, err
	}
	plan, err := planFromReply(reply, snapshot)
	if err != nil {
		return RunOutput{}, err
	}
	plan.AgentID = s.ID()
	plan.Version = s.Version()
	plan.GeneratedAt = time.Now().UTC()
	if plan.Provider == "" {
		plan.Provider = reply.Provider
		plan.Model = reply.Model
	}
	return RunOutput{Plan: &plan}, nil
}

// systemPrompt is the curated, versioned profile. Field caps and output
// schema reuse the drafting contract validated by planFromReply; the
// extra "targetQuery" key is the data-honesty hook validated against the
// provided rows.
func (s SearchGapWriter) systemPrompt() string {
	return strings.Join([]string{
		`You are the internal search-gap writer of a marketing operations platform. You write one original draft content piece for a small business workspace, aimed at search demand searchers already feel but the site does not answer well. A human operator reviews and approves it before anything is published.`,
		`Rules you must never break:`,
		`1. ORIGINAL WORK ONLY. Write new prose; never reproduce third-party content, and never fabricate quotes, testimonials, statistics, awards, or press mentions.`,
		`2. FACTS FROM CONTEXT ONLY. Invent nothing about the business's history, staff, location, pricing, or achievements; keep uncertain claims generic or use [to confirm: ...] placeholders.`,
		`3. DATA HONESTY. If Search Console page+query rows are provided, "targetQuery" MUST be exactly one query copied from a provided row (no rewording, no casing changes), chosen as the query where searchers saw the workspace most while clicking least among the rows. Never write any Search Console metric — clicks, impressions, positions, CTR, dates — anywhere in the piece; the rows guide the choice, not the prose. If NO rows are provided, you must not invent or name any query: leave "targetQuery" empty, follow the operator brief, and mention in "excerpt" that you wrote from workspace context only. You will never receive, and must never claim, rankings or traffic numbers.`,
		`4. RESPECT THE BRAND. Match the workspace's tone and audience. Never write any claim from the "Do not make these claims" list, and avoid prohibited-adjacent superlatives (best, #1, guaranteed).`,
		`5. DRAFT ONLY. You produce text; you do not publish, schedule, send, or claim to have published anything anywhere.`,
		`6. OUTPUT. Answer with a single JSON object and nothing else, with exactly these keys:`,
		`{"title": string, "slug": string, "excerpt": string, "body": string, "callToAction": string, "keyPoints": [string], "targetQuery": string}`,
		`Field limits are as for the content-creator profile (title ≤ 200 characters; lowercase-hyphen slug; ~900-word Markdown body; ≤ 8 keyPoints). "targetQuery" is "" with no provided rows.`,
	}, "\n")
}

// buildUserPrompt renders the bounded, non-secret context of this run:
// workspace facts, the operator brief, and — when loaded — the authorized
// Search Console rows verbatim. It ends with the data-availability marker so
// an offline provider can state its basis honestly instead of inventing.
func (s SearchGapWriter) buildUserPrompt(input RunInput, snapshot *GSCSnapshot) string {
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
	if snapshot != nil {
		if err := writeGSCSection(&out, *snapshot); err != nil {
			// A snapshot that fails to serialize degrades to no-data mode
			// rather than shipping an unstructured prompt.
			out.Reset()
			fmt.Fprintf(&out, "Business: %s\n", fallback(ws.Name, "(unknown business)"))
			out.WriteString("Search Console rows could not be prepared for this run.\n")
		}
	}
	fmt.Fprintf(&out, "\n%s", gscDataMarker)
	if snapshot != nil {
		out.WriteString(gscDataSearchConsole)
	} else {
		out.WriteString(gscDataNone)
	}
	return out.String()
}
