package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestContentDraftEditing covers the Milestone 4 draft flow: detail reads,
// versioned saves with planning facts, the draft-only rule, and isolation.
func TestContentDraftEditing(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()

	wsA := uniqueWorkspace(t, s, "draft-a")
	wsB := uniqueWorkspace(t, s, "draft-b")

	item, err := s.InsertContentItem(ctx, ContentItem{
		WorkspaceID: wsA.ID, Kind: "blog_article", Title: "Draft article",
	})
	if err != nil {
		t.Fatalf("InsertContentItem: %v", err)
	}

	t.Run("a fresh item has no version yet", func(t *testing.T) {
		got, version, err := s.GetContentItemDetail(ctx, wsA.ID, item.ID)
		if err != nil {
			t.Fatalf("GetContentItemDetail: %v", err)
		}
		if got.Title != "Draft article" || version != nil {
			t.Fatalf("expected no version on a fresh item, got %+v / %+v", got, version)
		}
	})

	t.Run("the first save creates version 1 with brief and planning facts", func(t *testing.T) {
		when := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
		slug := "draft-article"
		excerpt := "A short summary"
		body := "Body of version one"
		got, version, err := s.SaveContentDraft(ctx, wsA.ID, item.ID, ContentDraftInput{
			Title: item.Title, Slug: &slug, PlannedPublishAt: &when,
			Excerpt: &excerpt, Body: &body,
			CallToAction: strPtr("Book a table"),
			ChannelCopy:  map[string]any{"instagram": "New post!"},
			Metadata:     map[string]any{"source": "ai-draft", "model": "test"},
		})
		if err != nil {
			t.Fatalf("SaveContentDraft: %v", err)
		}
		if version.VersionNumber != 1 || version.Body != body ||
			version.CallToAction == nil || *version.CallToAction != "Book a table" {
			t.Fatalf("version 1 wrong: %+v", version)
		}
		if version.ChannelCopy["instagram"] != "New post!" || version.Metadata["source"] != "ai-draft" {
			t.Fatalf("channel copy or metadata lost: %+v", version)
		}
		if got.Slug == nil || *got.Slug != slug {
			t.Fatalf("slug not saved: %+v", got)
		}
		if got.PlannedPublishAt == nil || !got.PlannedPublishAt.Equal(when) {
			t.Fatalf("planned publish not saved: %+v", got.PlannedPublishAt)
		}
		if got.UpdatedAt.Before(item.CreatedAt) {
			t.Fatalf("updatedAt should be refreshed: %+v", got)
		}
	})

	t.Run("the second save creates version 2 and keeps the brief when omitted", func(t *testing.T) {
		excerpt := "Revised excerpt"
		body := "Body of version two"
		_, version, err := s.SaveContentDraft(ctx, wsA.ID, item.ID, ContentDraftInput{
			Title: "Draft article (renamed)", Excerpt: &excerpt, Body: &body,
		})
		if err != nil {
			t.Fatalf("SaveContentDraft: %v", err)
		}
		if version.VersionNumber != 2 || version.Body != body {
			t.Fatalf("version 2 wrong: %+v", version)
		}
		if version.CallToAction == nil || *version.CallToAction != "Book a table" {
			t.Fatalf("omitted CTA should be carried from version 1, got %+v", version.CallToAction)
		}
		if len(version.ChannelCopy) != 1 {
			t.Fatalf("omitted channel copy should be carried from version 1, got %+v", version.ChannelCopy)
		}

		got, latest, err := s.GetContentItemDetail(ctx, wsA.ID, item.ID)
		if err != nil {
			t.Fatalf("GetContentItemDetail: %v", err)
		}
		if got.Title != "Draft article (renamed)" {
			t.Fatalf("title not renamed: %+v", got)
		}
		if latest == nil || latest.VersionNumber != 2 {
			t.Fatalf("detail must return the newest version, got %+v", latest)
		}

		// A version list preserves both snapshots (history by construction).
		history, err := s.ListContentVersionsInWorkspace(ctx, wsA.ID, item.ID)
		if err != nil || len(history) != 2 {
			t.Fatalf("expected 2 stored versions, got %d err=%v", len(history), err)
		}
		if history[0].Body != "Body of version one" || history[1].Body != "Body of version two" {
			t.Fatalf("history not immutable: %+v", history)
		}
	})

	t.Run("clearing the slug and planned slot works on drafts", func(t *testing.T) {
		empty := ""
		got, _, err := s.SaveContentDraft(ctx, wsA.ID, item.ID, ContentDraftInput{
			Title: item.Title, Slug: &empty, ClearPlannedPublishAt: true,
		})
		if err != nil {
			t.Fatalf("SaveContentDraft: %v", err)
		}
		if got.Slug != nil || got.PlannedPublishAt != nil {
			t.Fatalf("slug/planned slot should be cleared: %+v", got)
		}
	})

	t.Run("a replacement slug colliding in the workspace is ErrSlugTaken", func(t *testing.T) {
		taken := "occupied-slug"
		other, err := s.InsertContentItem(ctx, ContentItem{
			WorkspaceID: wsA.ID, Kind: "snippet", Title: "With slug",
		})
		if err != nil {
			t.Fatalf("InsertContentItem: %v", err)
		}
		if _, _, err := s.SaveContentDraft(ctx, wsA.ID, other.ID, ContentDraftInput{
			Title: other.Title, Slug: &taken,
		}); err != nil {
			t.Fatalf("first save must succeed: %v", err)
		}
		clash, err := s.InsertContentItem(ctx, ContentItem{
			WorkspaceID: wsA.ID, Kind: "blog_article", Title: "Clash",
		})
		if err != nil {
			t.Fatalf("InsertContentItem (no slug): %v", err)
		}
		if _, _, err := s.SaveContentDraft(ctx, wsA.ID, clash.ID, ContentDraftInput{
			Title: clash.Title, Slug: &taken,
		}); !errors.Is(err, ErrSlugTaken) {
			t.Fatalf("expected ErrSlugTaken, got %v", err)
		}
	})

	t.Run("published content is frozen and reports ErrNotEditable", func(t *testing.T) {
		frozen, err := s.InsertContentItem(ctx, ContentItem{
			WorkspaceID: wsB.ID, Kind: "social_post", Title: "Frozen",
		})
		if err != nil {
			t.Fatalf("InsertContentItem: %v", err)
		}
		if err := s.setContentStatus(t, frozen.ID, "published"); err != nil {
			t.Fatalf("force status: %v", err)
		}
		if _, _, err := s.SaveContentDraft(ctx, wsB.ID, frozen.ID, ContentDraftInput{
			Title: frozen.Title, Body: strPtr("ignored"),
		}); !errors.Is(err, ErrNotEditable) {
			t.Fatalf("expected ErrNotEditable, got %v", err)
		}
	})

	t.Run("a version cannot be attached from the wrong workspace", func(t *testing.T) {
		if _, _, err := s.SaveContentDraft(ctx, wsB.ID, item.ID, ContentDraftInput{
			Title: "Hijack", Body: strPtr("cross workspace"),
		}); !errors.Is(err, ErrNotFound) {
			t.Fatalf("cross-workspace save must be ErrNotFound, got %v", err)
		}
		if _, _, err := s.GetContentItemDetail(ctx, wsB.ID, item.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("cross-workspace detail must be ErrNotFound, got %v", err)
		}
	})
}

func strPtr(s string) *string { return &s }

// setContentStatus forces a status directly so tests can arrange frozen rows.
func (s *Store) setContentStatus(t *testing.T, itemID, status string) error {
	t.Helper()
	_, err := s.pool.Exec(context.Background(),
		`UPDATE content_items SET status = $2 WHERE id = $1`, itemID, status)
	return err
}
