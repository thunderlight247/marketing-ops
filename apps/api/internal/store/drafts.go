package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ContentDraftInput is one save of a draft brief.
//
// Title is always required. Slug and PlannedPublishAt are optional facts of
// the planning record: nil keeps the current value, a non-nil value replaces
// it (an empty slug string means "no slug"), ClearPlannedPublishAt removes a
// set slot. A brief field left nil (excerpt, body, CTA, channel copy,
// metadata) carries forward from the newest version; every save still
// produces one new immutable version.
type ContentDraftInput struct {
	Title                 string
	Slug                  *string
	PlannedPublishAt      *time.Time
	ClearPlannedPublishAt bool
	Excerpt               *string
	Body                  *string
	CallToAction          *string
	ChannelCopy           map[string]any
	Metadata              map[string]any
}

// SaveContentDraft updates the planning record of one item and appends a new
// immutable version of the brief, in one transaction.
//
// The item must be a draft: approved or published content is frozen and its
// re-approval flow arrives with the approval workflow. The UPDATE joins the
// scoping in SQL, so a version cannot be attached outside the workspace
// boundary.
func (s *Store) SaveContentDraft(ctx context.Context, workspaceID, itemID string, in ContentDraftInput) (ContentItem, ContentVersion, error) {
	// The item UPDATE keeps every condition in SQL: id, workspace, and the
	// draft-only rule. planned_publish_at comes from (clear flag, value):
	// clear wins, then an explicit value, else keep the current column.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ContentItem{}, ContentVersion{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after a successful commit

	// Distinguish "no such item in this workspace" from "the item exists but
	// is frozen" by locking and checking the row first.
	var status string
	err = tx.QueryRow(ctx,
		`SELECT status FROM content_items WHERE id = $1 AND workspace_id = $2 FOR UPDATE`,
		itemID, workspaceID).Scan(&status)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ContentItem{}, ContentVersion{}, ErrNotFound
		}
		return ContentItem{}, ContentVersion{}, err
	}
	if status != "draft" {
		return ContentItem{}, ContentVersion{}, ErrNotEditable
	}

	// Omitted brief fields carry forward from the newest version, so a save
	// that only touches one field does not silently blank the others.
	if in.Excerpt == nil || in.Body == nil || in.CallToAction == nil ||
		in.ChannelCopy == nil || in.Metadata == nil {
		var prevExcerpt, prevCTA *string
		var prevBody string
		var prevChannels, prevMetadata map[string]any
		err := tx.QueryRow(ctx,
			`SELECT excerpt, body, call_to_action, channel_copy, metadata
			 FROM content_versions WHERE content_item_id = $1
			 ORDER BY version_number DESC LIMIT 1`, itemID,
		).Scan(&prevExcerpt, &prevBody, &prevCTA, &prevChannels, &prevMetadata)
		if err == nil {
			if in.Excerpt == nil {
				in.Excerpt = prevExcerpt
			}
			if in.Body == nil {
				in.Body = &prevBody
			}
			if in.CallToAction == nil {
				in.CallToAction = prevCTA
			}
			if in.ChannelCopy == nil {
				in.ChannelCopy = orEmptyMap(prevChannels)
			}
			if in.Metadata == nil {
				in.Metadata = orEmptyMap(prevMetadata)
			}
		} else if err != pgx.ErrNoRows {
			return ContentItem{}, ContentVersion{}, err
		}
	}

	// A first save has no previous version; brief fields stay as provided,
	// with JSONB columns normalized to real objects and body defaulting blank.
	in.ChannelCopy = orEmptyMap(in.ChannelCopy)
	in.Metadata = orEmptyMap(in.Metadata)
	if in.Body == nil {
		empty := ""
		in.Body = &empty
	}

	itemRow := tx.QueryRow(ctx,
		`UPDATE content_items ci
		 SET title = $3,
		     slug = CASE
		          WHEN $4::boolean THEN NULL
		          ELSE coalesce($5, ci.slug)
		        END,
		     planned_publish_at = CASE
		          WHEN $6::boolean THEN NULL
		          ELSE coalesce($7, ci.planned_publish_at)
		        END,
		     updated_at = now()
		 WHERE ci.id = $1 AND ci.workspace_id = $2 AND ci.status = 'draft'
		 RETURNING `+contentItemColumns,
		itemID, workspaceID, in.Title,
		in.Slug != nil && *in.Slug == "", // 4: clear the slug
		in.Slug,                          // 5: new slug (nil keeps current)
		in.ClearPlannedPublishAt,         // 6: clear the publish slot
		in.PlannedPublishAt,              // 7: new slot (nil keeps current)
	)
	item, err := scanContentItem(itemRow)
	if err != nil {
		var pgErr *pgconn.PgError
		switch {
		case errors.As(err, &pgErr) && pgErr.Code == "23505":
			return ContentItem{}, ContentVersion{}, ErrSlugTaken
		case err == pgx.ErrNoRows:
			return ContentItem{}, ContentVersion{}, ErrNotFound
		}
		return ContentItem{}, ContentVersion{}, err
	}

	// ON CONFLICT cannot happen for the standard columns, but a dupe slug is
	// signalled by pgx as a unique violation; surface it as the sentinels the
	// other repository methods already use.
	versionRow := tx.QueryRow(ctx,
		`INSERT INTO content_versions (content_item_id, version_number, title, excerpt, body, call_to_action, channel_copy, metadata)
		 SELECT $1, coalesce(max(cv.version_number), 0) + 1, $2, $3, $4, $5, $6, $7
		 FROM content_versions cv
		 WHERE cv.content_item_id = $1
		 RETURNING `+contentVersionColumns,
		itemID, in.Title, in.Excerpt, *in.Body, in.CallToAction, in.ChannelCopy, in.Metadata)
	version, err := scanContentVersion(versionRow)
	if err != nil {
		return ContentItem{}, ContentVersion{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return ContentItem{}, ContentVersion{}, err
	}
	return item, version, nil
}

func orEmptyMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// GetContentItemDetail returns an item of one workspace together with its
// newest version, if any. A freshly created item has no version yet; only the
// first save creates one.
func (s *Store) GetContentItemDetail(ctx context.Context, workspaceID, itemID string) (ContentItem, *ContentVersion, error) {
	item, err := s.GetContentItemInWorkspace(ctx, workspaceID, itemID)
	if err != nil {
		return ContentItem{}, nil, err
	}

	versions, err := s.ListContentVersionsInWorkspace(ctx, workspaceID, itemID)
	if err != nil {
		return ContentItem{}, nil, err
	}
	if len(versions) == 0 {
		return item, nil, nil
	}
	latest := versions[len(versions)-1]
	return item, &latest, nil
}
