package httpapi

import (
	"testing"

	"marketingops/api/internal/store"
)

func findArea(t *testing.T, findings []Finding, area string) Finding {
	t.Helper()
	for _, f := range findings {
		if f.Area == area {
			return f
		}
	}
	t.Fatalf("no finding for area %q in %+v", area, findings)
	return Finding{}
}

func TestAssessWorkspaceWithoutBrandProfileFlagsMissingFacts(t *testing.T) {
	findings := assessWorkspace(nil, store.WorkspaceReport{})

	for _, area := range []string{"Business description", "Services", "Target audience", "Tone of voice"} {
		f := findArea(t, findings, area)
		if f.Source != "missing" || f.Status != "attention" || f.Recommendation == "" {
			t.Errorf("%s = %+v, want a missing attention finding with a recommendation", area, f)
		}
	}
	// Unmeasured sources must be reported as unknown, never as zero.
	if f := findArea(t, findings, "Search visibility"); f.Status != "unknown" {
		t.Errorf("search visibility status = %q, want unknown", f.Status)
	}
	if f := findArea(t, findings, "Website traffic"); f.Status != "unknown" {
		t.Errorf("website traffic status = %q, want unknown", f.Status)
	}
}

func TestAssessWorkspaceLinksActionableFindingsToTheirPage(t *testing.T) {
	findings := assessWorkspace(nil, store.WorkspaceReport{FailedCount: 1, LeadsByStatus: store.ReportCounts{"new": 2}})

	cases := map[string]string{
		"Services":          "/settings",
		"Publishing":        "/integrations",
		"Enquiries":         "/lead-engine",
		"Search visibility": "/analytics",
	}
	for area, want := range cases {
		if got := findArea(t, findings, area).ActionSegment; got != want {
			t.Errorf("%s action = %q, want %q", area, got, want)
		}
	}
}

func TestAssessWorkspaceMarksProvidedFactsAndPrioritizesFailures(t *testing.T) {
	desc := "Nutrition guidance for Accra families."
	blank := "   "
	profile := &store.BrandProfile{BusinessDescription: &desc, Services: &blank}
	report := store.WorkspaceReport{
		ContentStatus: store.ReportCounts{"review": 2, "published": 1},
		LeadsByStatus: store.ReportCounts{"new": 3},
		FailedCount:   1,
	}

	findings := assessWorkspace(profile, report)

	if f := findArea(t, findings, "Business description"); f.Source != "provided" || f.Status != "ok" {
		t.Errorf("description = %+v, want provided ok", f)
	}
	// Whitespace-only text is not a fact.
	if f := findArea(t, findings, "Services"); f.Source != "missing" {
		t.Errorf("services = %+v, want missing", f)
	}
	if f := findArea(t, findings, "Approvals"); f.Status != "attention" {
		t.Errorf("approvals = %+v, want attention", f)
	}
	if f := findArea(t, findings, "Enquiries"); f.Status != "attention" {
		t.Errorf("enquiries = %+v, want attention for untouched new leads", f)
	}
	// Publishing failures are top priority; they must not sort below anything
	// lower-urgency.
	pub := findArea(t, findings, "Publishing")
	if pub.Priority != 1 {
		t.Errorf("publishing priority = %d, want 1", pub.Priority)
	}
	for i := 1; i < len(findings); i++ {
		if findings[i].Priority < findings[i-1].Priority {
			t.Errorf("findings not sorted by priority at %d: %+v", i, findings[i])
		}
	}
}
