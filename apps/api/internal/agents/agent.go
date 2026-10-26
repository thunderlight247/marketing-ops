// Package agents defines the internal specialist-agent interface (Agency
// Agents milestone). A specialist is a curated, versioned prompt profile, not
// a runtime with tools: it extends the platform's existing orchestration —
// workspace context, provider calls, approval rules — and never edits
// anything, publishes, or reaches beyond the data handed to it here.
//
// Security bounds enforced by this package:
//   - only the bounded, non-secret workspace facts and the caller-authorized
//     Search Console snapshot are placed into prompts;
//   - replies are validated before use, including that the reply never
//     claims Search Console data it was not given;
//   - output is advisory only: recommendations for human review.
package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"marketingops/api/internal/aiprovider"
)

// PromptRun is what a specialist needs from an AI provider: one bounded
// system+user completion returning raw text. Implemented by
// aiprovider.OpenRouter (network) and aiprovider.StubProvider (offline).
type PromptRun interface {
	RunPrompt(ctx context.Context, req aiprovider.RunPromptRequest) (aiprovider.PromptReply, error)
}

// GSCPageQueryRow is one aggregated page+query row from Search Console.
// Values come from the store's GSC adapter; nothing here is invented.
type GSCPageQueryRow struct {
	Page        string  `json:"page"`
	Query       string  `json:"query"`
	Clicks      float64 `json:"clicks"`
	Impressions float64 `json:"impressions"`
	Ctr         float64 `json:"ctr"`
	Position    float64 `json:"position"`
}

// GSCPageRow is one aggregated per-page row from Search Console.
type GSCPageRow struct {
	Page        string  `json:"page"`
	Clicks      float64 `json:"clicks"`
	Impressions float64 `json:"impressions"`
	Ctr         float64 `json:"ctr"`
	Position    float64 `json:"position"`
}

// GSCSnapshot is the authorized read-only Search Console data handed to one
// specialist run. Period rows are already capped by the loader.
type GSCSnapshot struct {
	Property    string            `json:"property"`
	StartDate   string            `json:"startDate"` // YYYY-MM-DD, inclusive
	EndDate     string            `json:"endDate"`   // YYYY-MM-DD, inclusive
	PageQueries []GSCPageQueryRow `json:"pageQueries"`
	TopPages    []GSCPageRow      `json:"topPages"`
}

// AgentInput is the bounded context of one advisory run.
type AgentInput struct {
	Workspace aiprovider.WorkspaceContext
	GSC       *GSCSnapshot // nil = no connected data; the run stays advisory
	Focus     string       // optional operator focus, bounded
}

// Recommendation is one prioritized advisory suggestion.
type Recommendation struct {
	Priority string `json:"priority"` // high | medium | low
	Area     string `json:"area"`
	Page     string `json:"page,omitempty"`
	Issue    string `json:"issue"`
	Evidence string `json:"evidence"`
	Action   string `json:"action"`
}

// AdvisorReport is the validated advisory output of a specialist run.
type AdvisorReport struct {
	AgentID                string           `json:"agentId"`
	Version                string           `json:"agentVersion"`
	Provider               string           `json:"provider"`
	Model                  string           `json:"model"`
	DataSource             string           `json:"dataSource"` // search_console | none
	GeneratedAt            time.Time        `json:"generatedAt"`
	CannibalizationChecked bool             `json:"cannibalizationChecked"`
	Summary                string           `json:"summary"`
	Gaps                   []string         `json:"gaps"`
	Recommendations        []Recommendation `json:"recommendations"`
}

// validPriorities is the closed set a reply may use.
var validPriorities = map[string]bool{"high": true, "medium": true, "low": true}

// ---- SEO specialist profile -------------------------------------------------

// SEOAdvisorID and SEOAdvisorVersion identify the curated profile.
const (
	SEOAdvisorID      = "seo_specialist"
	SEOAdvisorVersion = "1"
)

// gscDataMarker is the prompt contract withOfflineRunPrompt stubs (see
// aiprovider StubProvider): the user prompt always ends with this exact line
// so an offline, non-reading provider can state the data basis honestly.
const gscDataMarker = "GSC_DATA_AVAILABILITY: "

const gscDataNone = "none"
const gscDataSearchConsole = "search_console"

// Input caps keep one run and its stored audit tractable.
const (
	maxFocusRunes      = 500
	maxPageQueryRows   = 50
	maxTopPagesRows    = 20
	maxRecommendations = 15
	maxSummaryRunes    = 4000
	maxFieldRunes      = 600
	maxGapRunes        = 300
	maxGaps            = 10
)

// SEOAdvisor is the first curated specialist, distilled from the MIT-licensed
// agency-agents marketing/marketing-seo-specialist persona: evidence-based,
// white-hat, user-intent first, E-E-A-T aware, with a mandatory page+query
// cannibalization gate that degrades to no-data instead of guessing.
type SEOAdvisor struct{}

// ID and Version report the profile identity recorded in task history.
func (SEOAdvisor) ID() string      { return SEOAdvisorID }
func (SEOAdvisor) Version() string { return SEOAdvisorVersion }

// DisplayName and Description are the dashboard-facing identity of this
// curated profile.
func (SEOAdvisor) DisplayName() string { return "SEO advisor" }
func (SEOAdvisor) Description() string {
	return "Evidence-based, advisory-only organic search review of one workspace."
}

// OutputContract, WantsData, and AllowedActions declare the governance facts
// of this specialist: advisory-only output, Search Console snapshots when a
// loader is configured, and no executable actions.
func (SEOAdvisor) OutputContract() string   { return ContractAdvisory }
func (SEOAdvisor) WantsData() []string      { return []string{DataNeedSearchConsole} }
func (SEOAdvisor) AllowedActions() []string { return nil }

// Run dispatches one advisory pass, unpacking only the declared data need.
func (s SEOAdvisor) Run(ctx context.Context, run PromptRun, input RunInput) (RunOutput, error) {
	snapshot, _ := input.Data[DataNeedSearchConsole].(*GSCSnapshot)
	report, err := s.Advise(ctx, run, AgentInput{
		Workspace: input.Workspace,
		GSC:       snapshot,
		Focus:     input.Focus,
	})
	if err != nil {
		return RunOutput{}, err
	}
	return RunOutput{Report: &report}, nil
}

// Advise runs one advisory pass: build the bounded prompt, transport it via
// the provider, then validate the reply before it is shown to anyone.
func (s SEOAdvisor) Advise(ctx context.Context, run PromptRun, input AgentInput) (AdvisorReport, error) {
	reply, err := run.RunPrompt(ctx, aiprovider.RunPromptRequest{
		System: s.systemPrompt(),
		User:   s.buildUserPrompt(input),
	})
	if err != nil {
		return AdvisorReport{}, err
	}
	report, err := s.validateReply(reply, input.GSC != nil)
	if err != nil {
		return AdvisorReport{}, err
	}
	report.AgentID = s.ID()
	report.Version = s.Version()
	report.GeneratedAt = time.Now().UTC()
	if report.Provider == "" {
		report.Provider = reply.Provider
		report.Model = reply.Model
	}
	return report, nil
}

// systemPrompt is the curated, versioned specialist profile. It is internal
// platform content derived from the agency-agents persona, not a copy of the
// upstream file.
func (s SEOAdvisor) systemPrompt() string {
	return strings.Join([]string{
		`You are the internal SEO specialist of a marketing operations platform. You analyze one small business workspace and produce a prioritized, evidence-based advisory report for a human operator.`,
		`Rules you must never break:`,
		`1. WHITE-HAT ONLY. Recommend practices that comply with search-engine guidelines. No link schemes, keyword stuffing, doorway pages, or automation that manipulates rankings.`,
		`2. USER INTENT FIRST. Recommendations must state what real searchers want and how the business's content serves them; E-E-A-T (experience, expertise, authority, trust) must be respected.`,
		`3. SEARCH CONSOLE GATE. If page+query Search Console rows are provided below, you MUST review them for keyword cannibalization (multiple pages competing for the same query) before recommending on-page or targeting changes, and set "cannibalizationChecked" to true. If NO Search Console rows are provided, set "dataSource" to "none" and "cannibalizationChecked" to false, and list the missing connected data as a gap in "gaps". Never perform, imply, or claim a completed cannibalization or performance analysis without the rows.`,
		`4. NO INVENTED DATA. Use only numbers present in the provided rows and facts in the provided workspace context. Every "evidence" field must cite either the specific row(s) it relies on or say "based on the provided workspace context only". Never fabricate positions, clicks, impressions, rankings, or dates.`,
		`5. ADVISORY ONLY. You recommend; you do not execute. Never claim to have edited a page, published, removed content, contacted anyone, or changed any setting. Do not propose changes to third-party accounts.`,
		`6. Respect the workspace's prohibited claims; recommend compliant angles.`,
		`7. OUTPUT. Answer with a single JSON object and nothing else, with exactly these keys:`,
		`{"dataSource": "search_console"|"none", "cannibalizationChecked": true|false, "summary": string, "gaps": [string], "recommendations": [{"priority": "high"|"medium"|"low", "area": string, "page": string, "issue": string, "evidence": string, "action": string}]}`,
		`"summary" is at most 3 short sentences. "gaps" lists data or context you lacked. "recommendations" is ordered most impactful first, at most 15 items; "page" may be "" when not page-specific; "area" is a short category like content, technical, cannibalization, internal_links, metadata. "issue" and "action" are concrete and at most a few sentences each.`,
	}, "\n")
}

// buildUserPrompt renders the bounded, non-secret context of this run.
// Credentials, tokens, and any secret-derived data never appear here.
func (s SEOAdvisor) buildUserPrompt(input AgentInput) string {
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
		fmt.Fprintf(&out, "Operator focus for this review: %s\n", strings.TrimSpace(input.Focus))
	}

	if input.GSC != nil {
		if err := writeGSCSection(&out, *input.GSC); err != nil {
			// A snapshot that fails to serialize degrades to no-data mode
			// rather than shipping an unstructured prompt.
			out.Reset()
			fmt.Fprintf(&out, "Business: %s\n", fallback(ws.Name, "(unknown business)"))
			out.WriteString("Search Console rows could not be prepared for this run.\n")
		}
	}
	fmt.Fprintf(&out, "\n%s", gscDataMarker)
	if input.GSC != nil {
		out.WriteString(gscDataSearchConsole)
	} else {
		out.WriteString(gscDataNone)
	}
	return out.String()
}

// writeGSCSection renders the authorized rows exactly as they were fetched.
func writeGSCSection(out *strings.Builder, snap GSCSnapshot) error {
	fmt.Fprintf(out, "\nSearch Console data for property %s, period %s to %s:\n",
		snap.Property, snap.StartDate, snap.EndDate)
	rows := snap.PageQueries
	if len(rows) > maxPageQueryRows {
		rows = rows[:maxPageQueryRows]
	}
	encoded, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "page+query rows (verbatim aggregates):\n%s\n", encoded)
	pages := snap.TopPages
	if len(pages) > maxTopPagesRows {
		pages = pages[:maxTopPagesRows]
	}
	encodedPages, err := json.Marshal(pages)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "per-page rows (verbatim aggregates):\n%s\n", encodedPages)
	return nil
}

// replyShape mirrors the JSON the system prompt demands.
type replyShape struct {
	DataSource             string           `json:"dataSource"`
	CannibalizationChecked bool             `json:"cannibalizationChecked"`
	Summary                string           `json:"summary"`
	Gaps                   []string         `json:"gaps"`
	Recommendations        []Recommendation `json:"recommendations"`
}

// validateReply turns the raw provider text into an AdvisorReport or a safe
// failure. It also enforces the data-honesty rule: the reply may never claim
// Search Console data that was not provided to the run.
func (s SEOAdvisor) validateReply(reply aiprovider.PromptReply, hadGSC bool) (AdvisorReport, error) {
	raw := []byte(reply.Text)
	var parsed replyShape
	if err := json.Unmarshal(extractJSONObject(raw), &parsed); err != nil {
		return AdvisorReport{}, &aiprovider.ProviderError{
			Kind:  aiprovider.FailMalformed,
			Cause: "the specialist reply could not be structured into a report",
		}
	}

	dataSource := parsed.DataSource
	if !hadGSC {
		// The reply was given no rows; claiming otherwise is a factual error.
		dataSource = gscDataNone
	} else if dataSource != gscDataSearchConsole {
		dataSource = gscDataSearchConsole
	} else if !parsed.CannibalizationChecked && len(parsed.Recommendations) > 0 {
		return AdvisorReport{}, &aiprovider.ProviderError{
			Kind:  aiprovider.FailMalformed,
			Cause: "the specialist replied with Search Console data but did not check it",
		}
	}

	summary := strings.TrimSpace(parsed.Summary)
	if runeCount(summary) > maxSummaryRunes {
		summary = truncateRunes(summary, maxSummaryRunes)
	}

	gaps := make([]string, 0, len(parsed.Gaps))
	for _, gap := range parsed.Gaps {
		gap = strings.TrimSpace(gap)
		if gap == "" {
			continue
		}
		if runeCount(gap) > maxGapRunes {
			gap = truncateRunes(gap, maxGapRunes)
		}
		gaps = append(gaps, gap)
		if len(gaps) >= maxGaps {
			break
		}
	}

	recs := make([]Recommendation, 0, len(parsed.Recommendations))
	for _, rec := range parsed.Recommendations {
		issue := strings.TrimSpace(rec.Issue)
		action := strings.TrimSpace(rec.Action)
		area := strings.TrimSpace(rec.Area)
		if issue == "" || action == "" {
			continue
		}
		priority := strings.ToLower(rec.Priority)
		if !validPriorities[priority] {
			priority = "medium"
		}
		rec.Priority = priority
		rec.Issue = truncateRunes(issue, maxFieldRunes)
		rec.Action = truncateRunes(action, maxFieldRunes)
		rec.Area = truncateRunes(area, 80)
		rec.Page = truncateRunes(strings.TrimSpace(rec.Page), 500)
		rec.Evidence = truncateRunes(strings.TrimSpace(rec.Evidence), maxFieldRunes)
		recs = append(recs, rec)
		if len(recs) >= maxRecommendations {
			break
		}
	}

	metadata := map[string]any{}
	for key, value := range reply.Metadata {
		if isScalar(value) {
			metadata[key] = value
		}
	}

	return AdvisorReport{
		Provider:               stringFromMetadata(metadata, "provider"),
		Model:                  stringFromMetadata(metadata, "model"),
		DataSource:             dataSource,
		CannibalizationChecked: parsed.CannibalizationChecked && hadGSC,
		Summary:                summary,
		Gaps:                   gaps,
		Recommendations:        recs,
	}, nil
}

// extractJSONObject pulls the outermost JSON object out of a chatty reply.
func extractJSONObject(text []byte) []byte {
	start := strings.IndexByte(string(text), '{')
	if start < 0 {
		return nil
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(text); i++ {
		c := text[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return text[start : i+1]
			}
		}
	}
	return nil
}

func isScalar(value any) bool {
	switch value.(type) {
	case string, bool, int, int64, float64:
		if s, ok := value.(string); ok && len(s) > 500 {
			return false
		}
		return true
	default:
		return false
	}
}

func stringFromMetadata(metadata map[string]any, key string) string {
	if v, ok := metadata[key].(string); ok {
		return v
	}
	return ""
}

func runeCount(s string) int {
	return len([]rune(s))
}

func truncateRunes(s string, max int) string {
	if runeCount(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max])
}

func fallback(value, def string) string {
	if strings.TrimSpace(value) == "" {
		return def
	}
	return value
}
