// Business assessment (growth foundation). A GET on the workspace returns a
// deterministic, prioritized list of gaps and next steps built only from data
// the platform already stores: the brand profile and the workspace report.
//
// Honesty rules:
//   - No AI provider is called and no fact is invented. Every finding is
//     labelled with its source: "provided" (a stored brand field is filled in),
//     "missing" (a stored field or record is absent), or "derived" (a count or
//     ratio computed from stored state).
//   - Search visibility and web traffic are NOT assessed here: no verified
//     Search Console or analytics source feeds this endpoint yet, so the
//     response reports them as unknown rather than guessing.
package httpapi

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"marketingops/api/internal/store"
)

// Finding is one assessed item. Source is "provided", "missing" or "derived".
type Finding struct {
	Area           string `json:"area"`
	Source         string `json:"source"`
	Status         string `json:"status"` // "ok", "attention", or "unknown"
	Priority       int    `json:"priority"`
	Summary        string `json:"summary"`
	Recommendation string `json:"recommendation,omitempty"`
	// ActionSegment is the workspace sub-page that resolves the finding, relative
	// to /workspaces/{id}. Empty when no page applies.
	ActionSegment string `json:"actionSegment,omitempty"`
}

// GET /api/v1/workspaces/{id}/assessment
func getWorkspaceAssessmentHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		var profile *store.BrandProfile
		brand, err := ws.GetBrandProfile(r.Context(), workspaceID)
		switch {
		case err == nil:
			profile = &brand
		case err == store.ErrNotFound:
			// No profile yet: every brand check below reports it as missing.
		default:
			writeJSONError(w, http.StatusInternalServerError, "brand lookup failed")
			return
		}
		report, err := ws.WorkspaceReport(r.Context(), workspaceID, store.ReportRange{})
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "assessment failed")
			return
		}
		findings := assessWorkspace(profile, report)
		writeJSON(w, http.StatusOK, map[string]any{
			"workspaceId": workspaceID,
			"findings":    findings,
		})
	}
}

// assessWorkspace turns stored brand and report data into findings, sorted by
// priority (lowest number first). It is a pure function so it can be tested
// without a database.
func assessWorkspace(profile *store.BrandProfile, report store.WorkspaceReport) []Finding {
	var out []Finding

	// Brand profile: the source of truth the rest of the platform writes from.
	text := func(p *string) string {
		if p == nil {
			return ""
		}
		return strings.TrimSpace(*p)
	}
	brandChecks := []struct {
		area, label, rec string
		value            string
		priority         int
	}{
		{"Business description", "description", "Describe what the business does and for whom, in the brand profile.", "", 1},
		{"Services", "services", "List the services or products the business sells.", "", 1},
		{"Target audience", "audience", "Define who the business wants to reach.", "", 2},
		{"Tone of voice", "tone", "Set the tone content should use.", "", 3},
	}
	if profile != nil {
		brandChecks[0].value = text(profile.BusinessDescription)
		brandChecks[1].value = text(profile.Services)
		brandChecks[2].value = text(profile.Audience)
		brandChecks[3].value = text(profile.Tone)
	}
	for _, c := range brandChecks {
		if c.value != "" {
			out = append(out, Finding{
				Area: c.area, Source: "provided", Status: "ok", Priority: 9,
				Summary: fmt.Sprintf("The %s is recorded in the brand profile.", c.label),
			})
		} else {
			out = append(out, Finding{
				Area: c.area, Source: "missing", Status: "attention", Priority: c.priority,
				Summary:        "The brand profile has no " + c.label + " yet.",
				Recommendation: c.rec,
				ActionSegment:  "/settings",
			})
		}
	}

	// Content: is there anything to publish, and is anything waiting on review?
	total := sumCounts(report.ContentStatus)
	published := report.ContentStatus["published"]
	if total == 0 {
		out = append(out, Finding{
			Area: "Content", Source: "derived", Status: "attention", Priority: 2,
			Summary:        "No content items exist yet.",
			Recommendation: "Create a first content draft from the brand profile.",
			ActionSegment:  "/content",
		})
	} else {
		out = append(out, Finding{
			Area: "Content", Source: "derived", Status: "ok", Priority: 9,
			Summary: fmt.Sprintf("%d content item(s); %d published.", total, published),
		})
	}
	if pending := report.ContentStatus["review"]; pending > 0 {
		out = append(out, Finding{
			Area: "Approvals", Source: "derived", Status: "attention", Priority: 3,
			Summary:        fmt.Sprintf("%d item(s) are waiting for review.", pending),
			Recommendation: "Review and approve or reject the waiting items.",
			ActionSegment:  "/content",
		})
	}

	// Publishing failures are the most urgent operational signal.
	if report.FailedCount > 0 {
		out = append(out, Finding{
			Area: "Publishing", Source: "derived", Status: "attention", Priority: 1,
			Summary:        fmt.Sprintf("%d publishing job(s) failed.", report.FailedCount),
			Recommendation: "Inspect the recent failures and retry or fix the connection.",
			ActionSegment:  "/integrations",
		})
	}

	// Leads: whether enquiries arrive and whether they are being followed up.
	leads := sumCounts(report.LeadsByStatus)
	if leads == 0 {
		out = append(out, Finding{
			Area: "Enquiries", Source: "derived", Status: "attention", Priority: 2,
			Summary:        "No leads have been recorded.",
			Recommendation: "Connect a contact form or WhatsApp intake so enquiries are captured.",
			ActionSegment:  "/lead-engine",
		})
	} else {
		newLeads := report.LeadsByStatus["new"]
		status, rec := "ok", ""
		if newLeads > 0 {
			status, rec = "attention", "Follow up on new leads that have not been contacted."
		}
		out = append(out, Finding{
			Area: "Enquiries", Source: "derived", Status: status, Priority: 2,
			Summary:        fmt.Sprintf("%d lead(s) recorded; %d still new.", leads, newLeads),
			Recommendation: rec,
			ActionSegment:  "/lead-engine",
		})
	}

	// Sources the platform cannot see yet are reported as unknown, not as zero.
	out = append(out, Finding{
		Area: "Search visibility", Source: "missing", Status: "unknown", Priority: 4,
		Summary:       "Search visibility is not measured yet: no verified search data source is connected.",
		ActionSegment: "/analytics",
	})
	out = append(out, Finding{
		Area: "Website traffic", Source: "missing", Status: "unknown", Priority: 4,
		Summary:       "Website traffic is not measured yet: no verified analytics source is connected.",
		ActionSegment: "/analytics",
	})

	sort.SliceStable(out, func(i, j int) bool { return out[i].Priority < out[j].Priority })
	return out
}

func sumCounts(c store.ReportCounts) int {
	n := 0
	for _, v := range c {
		n += v
	}
	return n
}
