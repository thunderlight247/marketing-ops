package store

// Integration tests for the Milestone 11 campaign and lead repositories.
// They run only when a database is configured (see TestMain in store_test.go)
// and reuse uniqueWorkspace for cross-workspace isolation checks.

import (
	"context"
	"errors"
	"testing"
	"time"
)

func insertTestCampaign(t *testing.T, s *Store, workspaceID, name string) Campaign {
	t.Helper()
	ctx := context.Background()
	c, err := s.InsertCampaign(ctx, Campaign{
		WorkspaceID: workspaceID,
		Name:        name,
		Status:      "active",
		UtmCampaign: "spring-newsletter",
		Notes:       "spring push",
	})
	if err != nil {
		t.Fatalf("InsertCampaign: %v", err)
	}
	return c
}

func TestCampaignLifecycle(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()
	ws := uniqueWorkspace(t, s, "campaigns")

	c := insertTestCampaign(t, s, ws.ID, "Spring newsletter")
	if c.Status != "active" || c.UtmCampaign != "spring-newsletter" {
		t.Fatalf("stored campaign mismatch: %+v", c)
	}

	// Duplicate name in the same workspace is rejected; the same name in a
	// different workspace is fine (workspace isolation).
	if _, err := s.InsertCampaign(ctx, Campaign{WorkspaceID: ws.ID, Name: "Spring newsletter"}); !errors.Is(err, ErrSlugTaken) {
		t.Errorf("duplicate campaign name must be ErrSlugTaken, got %v", err)
	}
	other := uniqueWorkspace(t, s, "campaigns-other")
	if _, err := s.InsertCampaign(ctx, Campaign{WorkspaceID: other.ID, Name: "Spring newsletter"}); err != nil {
		t.Errorf("same name in another workspace must be allowed, got %v", err)
	}

	// Invalid status is rejected before the database.
	if _, err := s.InsertCampaign(ctx, Campaign{WorkspaceID: ws.ID, Name: "Bad status", Status: "viral"}); !errors.Is(err, ErrInvalidStatus) {
		t.Errorf("invalid status must be ErrInvalidStatus, got %v", err)
	}
	if _, err := s.InsertCampaign(ctx, Campaign{WorkspaceID: ws.ID, Name: "   "}); !errors.Is(err, ErrNameRequired) {
		t.Errorf("blank name must be ErrNameRequired, got %v", err)
	}

	// Update rewrites editable fields and keeps id/workspace.
	updated, err := s.UpdateCampaign(ctx, Campaign{
		ID:          c.ID,
		WorkspaceID: ws.ID,
		Name:        "Spring newsletter",
		Status:      "completed",
		UtmCampaign: "spring-newsletter",
		Notes:       "wrapped up",
	})
	if err != nil || updated.Status != "completed" || updated.Notes != "wrapped up" {
		t.Errorf("UpdateCampaign mismatch: %+v err %v", updated, err)
	}

	// Cross-workspace read/update is not possible.
	if _, err := s.GetCampaignInWorkspace(ctx, other.ID, c.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-workspace campaign read must be ErrNotFound, got %v", err)
	}
	if _, err := s.UpdateCampaign(ctx, Campaign{ID: c.ID, WorkspaceID: other.ID, Name: "hijack"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-workspace campaign update must be ErrNotFound, got %v", err)
	}

	if list, err := s.ListCampaigns(ctx, ws.ID); err != nil || len(list) != 1 || list[0].ID != c.ID {
		t.Errorf("ListCampaigns mismatch: list=%d err=%v", len(list), err)
	}
	// The other workspace sees only its own campaign.
	if list, err := s.ListCampaigns(ctx, other.ID); err != nil || len(list) != 1 {
		t.Errorf("other workspace must see only its campaign: %d %v", len(list), err)
	}
}

func TestLeadLifecycle(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()
	ws := uniqueWorkspace(t, s, "leads")
	campaign := insertTestCampaign(t, s, ws.ID, "Lead campaign")

	email := "jane@example.test"
	note := "asked about pricing"
	follow := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Microsecond)

	lead, err := s.InsertLead(ctx, Lead{
		WorkspaceID:  ws.ID,
		CampaignID:   &campaign.ID,
		SourceKind:   "manual",
		SourceDetail: "taken during a phone call",
		ContactEmail: &email,
		Message:      &note,
		Notes:        "warm lead",
		UtmSource:    strPtr("google"),
		UtmCampaign:  strPtr(campaign.UtmCampaign),
		FollowUpAt:   &follow,
	})
	if err != nil {
		t.Fatalf("InsertLead: %v", err)
	}
	if lead.Status != "new" || lead.CampaignID == nil || *lead.CampaignID != campaign.ID {
		t.Fatalf("stored lead mismatch: %+v", lead)
	}

	// Blank source becomes 'unknown' explicitly, never fabricated.
	unknown, err := s.InsertLead(ctx, Lead{WorkspaceID: ws.ID})
	if err != nil || unknown.SourceKind != "unknown" {
		t.Errorf("blank source must default to 'unknown': %+v err %v", unknown, err)
	}
	if _, err := s.InsertLead(ctx, Lead{WorkspaceID: ws.ID, SourceKind: "scraped"}); !errors.Is(err, ErrInvalidStatus) {
		t.Errorf("unknown source kind must be rejected, got %v", err)
	}
	if _, err := s.InsertLead(ctx, Lead{WorkspaceID: ws.ID, Status: "almost"}); !errors.Is(err, ErrInvalidStatus) {
		t.Errorf("unknown lead status must be rejected, got %v", err)
	}

	// A campaign id from another workspace is not accepted.
	other := uniqueWorkspace(t, s, "leads-other")
	if _, err := s.InsertLead(ctx, Lead{WorkspaceID: ws.ID, CampaignID: &other.ID}); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-workspace campaign reference must be ErrNotFound, got %v", err)
	}

	// Update: status move, follow-up set, notes append via explicit write.
	nextFollow := follow.Add(24 * time.Hour).UTC().Truncate(time.Microsecond)
	updated, err := s.UpdateLead(ctx, ws.ID, lead.ID, LeadUpdate{
		Status:     "contacted",
		FollowUpAt: &nextFollow,
		Notes:      "called, send the plan",
	})
	if err != nil {
		t.Fatalf("UpdateLead: %v", err)
	}
	if updated.Status != "contacted" || updated.FollowUpAt == nil || !updated.FollowUpAt.Equal(nextFollow) {
		t.Fatalf("UpdateLead mismatch: %+v", updated)
	}
	if updated.Notes != "called, send the plan" {
		t.Errorf("notes not applied: %q", updated.Notes)
	}
	if updated.SourceKind != "manual" || updated.SourceDetail != "taken during a phone call" {
		t.Errorf("source must be immutable after creation: %+v", updated)
	}

	// Clear the follow-up and detach the campaign.
	cleared, err := s.UpdateLead(ctx, ws.ID, lead.ID, LeadUpdate{ClearFollow: true, ClearCampaign: true})
	if err != nil {
		t.Fatalf("UpdateLead clear: %v", err)
	}
	if cleared.FollowUpAt != nil || cleared.CampaignID != nil {
		t.Errorf("clear flags must empty the fields: %+v", cleared)
	}

	// Workspace isolation for reads.
	if _, err := s.GetLeadInWorkspace(ctx, other.ID, lead.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-workspace lead read must be ErrNotFound, got %v", err)
	}
	A := uniqueWorkspace(t, s, "leads-A")
	leadA, _ := s.InsertLead(ctx, Lead{WorkspaceID: A.ID, SourceKind: "form_api"})
	if list, err := s.ListLeads(ctx, ws.ID); err != nil {
		for _, l := range list {
			if l.WorkspaceID != ws.ID {
				t.Fatalf("ListLeads leaked a row from workspace %s", l.WorkspaceID)
			}
		}
	}
	if _, err := s.GetLeadInWorkspace(ctx, other.ID, leadA.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-workspace lead read must be ErrNotFound, got %v", err)
	}
}
