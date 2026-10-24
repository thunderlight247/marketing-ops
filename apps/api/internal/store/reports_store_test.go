package store

// Integration tests for the Milestone 12 report aggregation. They run only
// when a database is configured (TestMain in store_test.go) and verify both
// the count math and the workspace isolation of every grouped query.

import (
	"context"
	"testing"
	"time"
)

func TestWorkspaceReportAggregation(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()
	ws := uniqueWorkspace(t, s, "reports")
	other := uniqueWorkspace(t, s, "reports-other")

	// Seed other workspace so isolation is proven by count mismatch.
	if _, err := s.InsertCampaign(ctx, Campaign{WorkspaceID: other.ID, Name: "Other campaign"}); err != nil {
		t.Fatalf("seed other campaign: %v", err)
	}
	if _, err := s.InsertLead(ctx, Lead{WorkspaceID: other.ID, SourceKind: "import", Status: "won"}); err != nil {
		t.Fatalf("seed other lead: %v", err)
	}

	if _, err := s.InsertCampaign(ctx, Campaign{WorkspaceID: ws.ID, Name: "Spring push", Status: "active"}); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	if _, err := s.InsertLead(ctx, Lead{WorkspaceID: ws.ID, SourceKind: "manual", Status: "new"}); err != nil {
		t.Fatalf("seed lead manual: %v", err)
	}
	if _, err := s.InsertLead(ctx, Lead{WorkspaceID: ws.ID, SourceKind: "manual", Status: "new"}); err != nil {
		t.Fatalf("seed lead manual 2: %v", err)
	}
	if _, err := s.InsertLead(ctx, Lead{WorkspaceID: ws.ID, SourceKind: "form_api", Status: "won"}); err != nil {
		t.Fatalf("seed lead form: %v", err)
	}
	if _, err := s.InsertContentItem(ctx, ContentItem{WorkspaceID: ws.ID, Kind: "blog_article", Title: "Probe A"}); err != nil {
		t.Fatalf("seed item A: %v", err)
	}
	if _, err := s.InsertContentItem(ctx, ContentItem{WorkspaceID: ws.ID, Kind: "social_post", Title: "Probe B"}); err != nil {
		t.Fatalf("seed item B: %v", err)
	}
	// One item in the other workspace must not appear in this report.
	if _, err := s.InsertContentItem(ctx, ContentItem{WorkspaceID: other.ID, Kind: "blog_article", Title: "Foreign"}); err != nil {
		t.Fatalf("seed foreign item: %v", err)
	}

	rep, err := s.WorkspaceReport(ctx, ws.ID, ReportRange{})
	if err != nil {
		t.Fatalf("WorkspaceReport: %v", err)
	}
	if rep.WorkspaceID != ws.ID {
		t.Fatalf("report workspace mismatch: %s", rep.WorkspaceID)
	}
	if rep.ContentStatus["draft"] != 2 || len(rep.ContentStatus) != 1 {
		t.Errorf("content status counts = %v, want draft:2 only", rep.ContentStatus)
	}
	if rep.CampaignsBy["active"] != 1 {
		t.Errorf("campaign counts = %v", rep.CampaignsBy)
	}
	if rep.LeadsByStatus["new"] != 2 || rep.LeadsByStatus["won"] != 1 {
		t.Errorf("lead status counts = %v", rep.LeadsByStatus)
	}
	if rep.LeadsBySource["manual"] != 2 || rep.LeadsBySource["form_api"] != 1 {
		t.Errorf("lead source counts = %v", rep.LeadsBySource)
	}
	if len(rep.PublishingJobs) != 0 {
		t.Errorf("no publishing jobs seeded yet, got %v", rep.PublishingJobs)
	}
	if len(rep.RecentFailures) != 0 {
		t.Errorf("no failures seeded yet, got %v", rep.RecentFailures)
	}

	// An unbounded range and an inverted-ish small range both parse; the
	// publishing sections stay consistent (zero jobs here either way).
	until := time.Now().UTC().Add(time.Hour)
	since := time.Now().UTC().Add(-time.Hour)
	rep2, err := s.WorkspaceReport(ctx, ws.ID, ReportRange{Since: &since, Until: &until})
	if err != nil {
		t.Fatalf("WorkspaceReport(range): %v", err)
	}
	if rep2.PublishingJobs["succeeded"] != 0 || rep2.PublishedCount != 0 {
		t.Errorf("range report must still show zero publishing jobs, got %v", rep2.PublishingJobs)
	}
}

func TestWorkspaceReportPublishingCounts(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()
	operatorID, err := s.EnsureOperator(ctx)
	if err != nil {
		t.Fatalf("EnsureOperator: %v", err)
	}
	ws, item, version, integ := seedApprovedVersion(t, s)

	job, err := s.InsertPublishingJob(ctx, ws.ID, PublishingJob{
		ContentItemID: item.ID, ContentVersionID: version.ID, IntegrationID: integ.ID,
		IdempotencyKey: ws.Slug + ":reports-test:1",
	})
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	if err := s.SetPublishingJobStatus(ctx, ws.ID, job.ID, "succeeded", ""); err != nil {
		t.Fatalf("succeed job: %v", err)
	}
	_ = operatorID

	since := time.Now().UTC().Add(-time.Hour)
	until := time.Now().UTC().Add(time.Hour)
	rep, err := s.WorkspaceReport(ctx, ws.ID, ReportRange{Since: &since, Until: &until})
	if err != nil {
		t.Fatalf("WorkspaceReport: %v", err)
	}
	if rep.PublishingJobs["succeeded"] != 1 || rep.PublishedCount != 1 {
		t.Errorf("publishing counts = %v", rep.PublishingJobs)
	}

	// Outside the range the same job must not be counted (created_at filter).
	old := time.Now().UTC().Add(-24 * time.Hour)
	later := time.Now().UTC().Add(-23 * time.Hour)
	repOld, err := s.WorkspaceReport(ctx, ws.ID, ReportRange{Since: &old, Until: &later})
	if err != nil {
		t.Fatalf("WorkspaceReport(old range): %v", err)
	}
	if len(repOld.PublishingJobs) != 0 {
		t.Errorf("out-of-range job must be excluded, got %v", repOld.PublishingJobs)
	}
}
