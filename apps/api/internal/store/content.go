package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// ContentItem is one piece of planned or published content.
type ContentItem struct {
	ID               string
	WorkspaceID      string
	Kind             string
	Status           string
	Title            string
	Slug             *string
	PlannedPublishAt *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

const contentItemColumns = `id, workspace_id, kind, status, title, slug, planned_publish_at, created_at, updated_at`

// InsertContentItem creates a content item inside one workspace. The item is
// always a draft; only the approval workflow can change its status.
func (s *Store) InsertContentItem(ctx context.Context, item ContentItem) (ContentItem, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO content_items (workspace_id, kind, status, title, slug)
		 VALUES ($1, $2, 'draft', $3, $4)
		 ON CONFLICT (workspace_id, slug) DO NOTHING
		 RETURNING `+contentItemColumns,
		item.WorkspaceID, item.Kind, item.Title, item.Slug)
	out, err := scanContentItem(row)
	if err == pgx.ErrNoRows {
		return ContentItem{}, ErrSlugTaken
	}
	return out, mapNotFound(err)
}

// ListContentItems returns the items of exactly one workspace.
func (s *Store) ListContentItems(ctx context.Context, workspaceID string) ([]ContentItem, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+contentItemColumns+` FROM content_items
		 WHERE workspace_id = $1 ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ContentItem{}
	for rows.Next() {
		item, err := scanContentItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// GetContentItemInWorkspace reads one item only if it belongs to workspaceID.
// This is where isolation is encoded: SQL, not application filtering.
func (s *Store) GetContentItemInWorkspace(ctx context.Context, workspaceID, itemID string) (ContentItem, error) {
	out, err := scanContentItem(s.pool.QueryRow(ctx,
		`SELECT `+contentItemColumns+` FROM content_items
		 WHERE id = $1 AND workspace_id = $2`, itemID, workspaceID))
	return out, mapNotFound(err)
}

func scanContentItem(row pgx.Row) (ContentItem, error) {
	var c ContentItem
	if err := row.Scan(&c.ID, &c.WorkspaceID, &c.Kind, &c.Status, &c.Title,
		&c.Slug, &c.PlannedPublishAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return ContentItem{}, err
	}
	return c, nil
}
