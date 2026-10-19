package store

import (
	"context"
	"errors"
	"testing"
)

func TestApprovalLifecycleAndPublishingGuard(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()

	wsA := uniqueWorkspace(t, s, "approve-a")
	wsB := uniqueWorkspace(t, s, "approve-b")
	operator := seedOperator(t, s)

	item, err := s.InsertContentItem(ctx, ContentItem{
		WorkspaceID: wsA.ID, Kind: "blog_article", Title: "Approvable",
	})
	if err != nil {
		t.Fatalf("InsertContentItem: %v", err)
	}

	t.Run("deciding with no version yet fails as ErrVersionNotLatest", func(t *testing.T) {
		_, _, err := s.DecideApproval(ctx, wsA.ID, item.ID, ApprovalDecisionInput{
			Decision: "approve", OperatorID: operator,
		})
		if !errors.Is(err, ErrVersionNotLatest) {
			t.Fatalf("expected ErrVersionNotLatest, got %v", err)
		}
	})

	body1 := "version one body"
	if _, _, err := s.SaveContentDraft(ctx, wsA.ID, item.ID, ContentDraftInput{
		Title: item.Title, Body: &body1,
	}); err != nil {
		t.Fatalf("SaveContentDraft: %v", err)
	}

	t.Run("an unknown item is ErrNotFound", func(t *testing.T) {
		_, _, err := s.DecideApproval(ctx, wsB.ID, item.ID, ApprovalDecisionInput{
			Decision: "approve", OperatorID: operator,
		})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("cross-workspace decide must be ErrNotFound, got %v", err)
		}
		if _, _, err := s.DecideApproval(ctx, wsA.ID, "00000000-0000-0000-0000-000000000000",
			ApprovalDecisionInput{Decision: "approve", OperatorID: operator}); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unknown item decide must be ErrNotFound, got %v", err)
		}
	})

	t.Run("approving moves the item to approved and records the exact version", func(t *testing.T) {
		got, approval, err := s.DecideApproval(ctx, wsA.ID, item.ID, ApprovalDecisionInput{
			Decision: "approve", OperatorID: operator, Note: ptrStr("looks good"),
		})
		if err != nil {
			t.Fatalf("DecideApproval: %v", err)
		}
		if got.Status != "approved" {
			t.Fatalf("status = %q, want approved", got.Status)
		}
		if approval.Decision != "approved" || approval.Note == nil || *approval.Note != "looks good" {
			t.Fatalf("approval row wrong: %+v", approval)
		}
		history, err := s.ListApprovalsByItem(ctx, wsA.ID, item.ID)
		if err != nil || len(history) != 1 {
			t.Fatalf("expected 1 recorded decision, got %d err=%v", len(history), err)
		}
	})

	t.Run("re-deciding an approved item is refused centrally", func(t *testing.T) {
		_, _, err := s.DecideApproval(ctx, wsA.ID, item.ID, ApprovalDecisionInput{
			Decision: "approve", OperatorID: operator,
		})
		if !errors.Is(err, ErrInvalidStatus) {
			t.Fatalf("approved status must refuse decisions with ErrInvalidStatus, got %v", err)
		}
	})

	t.Run("editing approved content invalidates the approval back to draft", func(t *testing.T) {
		body2 := "changed body after approval"
		got, version, err := s.SaveContentDraft(ctx, wsA.ID, item.ID, ContentDraftInput{
			Title: item.Title, Body: &body2,
		})
		if err != nil {
			t.Fatalf("SaveContentDraft on approved item: %v", err)
		}
		if got.Status != "draft" {
			t.Fatalf("editing an approved item must reset it to draft, got %q", got.Status)
		}
		if version.VersionNumber != 2 || version.Body != body2 {
			t.Fatalf("invalidating save stored the wrong version: %+v", version)
		}
	})

	t.Run("publishing unapproved or changed content is refused", func(t *testing.T) {
		in, err := s.InsertIntegration(ctx, Integration{
			WorkspaceID: wsA.ID, AdapterKind: "sanity", Name: "publish-guard",
			Status: "unconfigured", Config: map[string]any{},
		})
		if err != nil {
			t.Fatalf("InsertIntegration: %v", err)
		}
		if _, err := s.InsertPublishingJob(ctx, wsA.ID, PublishingJob{
			ContentItemID: item.ID, ContentVersionID: item.ID, IntegrationID: in.ID,
			IdempotencyKey: "guard:not-approved",
		}); !errors.Is(err, ErrNotApproved) {
			t.Fatalf("unapproved version must be ErrNotApproved, got %v", err)
		}

		// Approve version 2, then confirm the OLD version cannot publish.
		_, approval2, err := s.DecideApproval(ctx, wsA.ID, item.ID, ApprovalDecisionInput{
			Decision: "approve", OperatorID: operator,
		})
		if err != nil {
			t.Fatalf("re-approve: %v", err)
		}
		_, err = s.InsertPublishingJob(ctx, wsA.ID, PublishingJob{
			ContentItemID: item.ID, ContentVersionID: approval2.ContentVersionID, IntegrationID: in.ID,
			IdempotencyKey: "guard:current-approved",
		})
		if err != nil {
			t.Fatalf("current approved version must publish, got %v", err)
		}

		// Make a NEWER version: the approved version is now superseded.
		_, _, err = s.SaveContentDraft(ctx, wsA.ID, item.ID, ContentDraftInput{
			Title: item.Title, Body: ptrStr("changed again"),
		})
		if err != nil {
			t.Fatalf("third save: %v", err)
		}
		_, err = s.InsertPublishingJob(ctx, wsA.ID, PublishingJob{
			ContentItemID: item.ID, ContentVersionID: approval2.ContentVersionID, IntegrationID: in.ID,
			IdempotencyKey: "guard:superseded",
		})
		if !errors.Is(err, ErrNotApproved) {
			t.Fatalf("superseded version must be ErrNotApproved, got %v", err)
		}
	})

	t.Run("rejecting needs review-eligible status and records the decision", func(t *testing.T) {
		other, err := s.InsertContentItem(ctx, ContentItem{
			WorkspaceID: wsB.ID, Kind: "social_post", Title: "Rejected flow",
		})
		if err != nil {
			t.Fatalf("InsertContentItem: %v", err)
		}
		if _, _, err := s.SaveContentDraft(ctx, wsB.ID, other.ID, ContentDraftInput{
			Title: other.Title, Body: ptrStr("rejectable"),
		}); err != nil {
			t.Fatalf("SaveContentDraft: %v", err)
		}
		got, approval, err := s.DecideApproval(ctx, wsB.ID, other.ID, ApprovalDecisionInput{
			Decision: "reject", OperatorID: operator, Note: ptrStr("off-brand"),
		})
		if err != nil {
			t.Fatalf("DecideApproval reject: %v", err)
		}
		if got.Status != "rejected" || approval.Decision != "rejected" {
			t.Fatalf("unexpected reject result: %+v / %+v", got.Status, approval.Decision)
		}
		// A rejected item goes back to draft on the next edit (rework flow).
		reworked, _, err := s.SaveContentDraft(ctx, wsB.ID, other.ID, ContentDraftInput{
			Title: other.Title + " (v2)", Body: ptrStr("reworked"),
		})
		if err != nil || reworked.Status != "draft" {
			t.Fatalf("rework must return to draft: %+v err=%v", reworked, err)
		}
	})
}

func TestContentStatusTransitionsAreCentral(t *testing.T) {
	// The map is THE definition; a few structural checks keep it honest.
	cases := []struct {
		from, to string
		allowed  bool
	}{
		{"draft", "review", true},
		{"draft", "approved", true},
		{"draft", "rejected", true},
		{"review", "approved", true},
		{"review", "draft", true},
		{"approved", "draft", true},
		{"rejected", "review", true},
		{"draft", "queued", false},
		{"approved", "review", false},
		{"published", "draft", false},
		{"queued", "published", false},
		{"draft", "failed", false},
	}
	for _, c := range cases {
		if got := transitionAllowed(c.from, c.to); got != c.allowed {
			t.Errorf("transition %s→%s allowed=%v, want %v", c.from, c.to, got, c.allowed)
		}
	}
}

func TestEnsureOperatorSeedsSingleton(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()
	first, err := s.EnsureOperator(ctx)
	if err != nil {
		t.Fatalf("EnsureOperator: %v", err)
	}
	second, err := s.EnsureOperator(ctx)
	if err != nil {
		t.Fatalf("EnsureOperator second: %v", err)
	}
	if first != second {
		t.Fatalf("operator identity must be stable: %q vs %q", first, second)
	}
}
