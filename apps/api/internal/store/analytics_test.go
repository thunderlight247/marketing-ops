package store

import (
	"context"
	"testing"
)

// TestAnalyticsSummaryReadOnly exercises the aggregate SQL without seeding or
// mutating rows: it is safe to run against a database that holds real data.
func TestAnalyticsSummaryReadOnly(t *testing.T) {
	s := requireStore(t)
	got, err := s.AnalyticsSummary(context.Background(), "")
	if err != nil {
		t.Fatalf("AnalyticsSummary: %v", err)
	}
	if got.GeneratedAt.IsZero() {
		t.Error("GeneratedAt is zero")
	}
	if got.ByWorkspace == nil {
		t.Error("ByWorkspace must be an empty slice, not nil")
	}
	if got.CRMStatus == nil || got.EngineStatus == nil {
		t.Error("status maps must be non-nil")
	}

	var campaigns, crmLeads, engineLeads, emails int
	for _, w := range got.ByWorkspace {
		campaigns += w.Campaigns
		crmLeads += w.CRMLeads
		engineLeads += w.EngineLeads
		emails += w.EmailsSent
	}
	if got.Totals.Workspaces != len(got.ByWorkspace) {
		t.Errorf("Workspaces = %d, want %d", got.Totals.Workspaces, len(got.ByWorkspace))
	}
	if got.Totals.Campaigns != campaigns ||
		got.Totals.CRMLeads != crmLeads ||
		got.Totals.EngineLeads != engineLeads ||
		got.Totals.EmailsSent != emails {
		t.Errorf("totals do not equal the per-workspace sums: %+v", got.Totals)
	}
}
