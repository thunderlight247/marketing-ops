// Package store is the data-access layer for the domain model.
//
// Rule enforced everywhere: a query that touches a client-owned table must
// scope to a workspace. Methods that read client-owned rows take a workspace id
// and filter on it in SQL, so one workspace cannot see another's records even
// if caller code passes a mismatched id. The integration tests in this package
// verify that.
package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Workspace is one client business.
type Workspace struct {
	ID         string
	Slug       string
	Name       string
	WebsiteURL *string
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Sentinel errors so callers do not need the driver package to compare results.
var (
	ErrNotFound          = errors.New("record not found")
	ErrSlugTaken         = errors.New("workspace slug already used")
	ErrNameRequired      = errors.New("workspace name is required")
	ErrInvalidCapability = errors.New("unknown capability or status value")
	ErrInvalidStatus     = errors.New("unknown lifecycle status value")
)

// Store is the repository target. It wraps a pgx pool so the process shares one.
type Store struct {
	pool *pgxpool.Pool
}

// New wraps an existing pool.
func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Pool exposes the shared pool for callers that need queries the typed
// repositories do not cover yet (tests, ad-hoc operator tooling).
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// ---- Workspace repository -------------------------------------------------

const workspaceColumns = `id, slug, name, website_url, status, created_at, updated_at`

// InsertWorkspace creates a workspace and returns the stored row.
func (s *Store) InsertWorkspace(ctx context.Context, w Workspace) (Workspace, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO workspaces (slug, name, website_url, status)
		 VALUES ($1, $2, $3, 'active')
		 ON CONFLICT (slug) DO NOTHING
		 RETURNING `+workspaceColumns,
		w.Slug, w.Name, w.WebsiteURL)
	out, err := scanWorkspace(row)
	if err == pgx.ErrNoRows {
		return Workspace{}, ErrSlugTaken
	}
	return out, err
}

// GetWorkspaceByID finds a workspace by id.
func (s *Store) GetWorkspaceByID(ctx context.Context, id string) (Workspace, error) {
	out, err := scanWorkspace(s.pool.QueryRow(ctx,
		`SELECT `+workspaceColumns+` FROM workspaces WHERE id = $1`, id))
	return out, mapNotFound(err)
}

// GetWorkspaceBySlug finds a workspace by slug (operator-level lookup).
func (s *Store) GetWorkspaceBySlug(ctx context.Context, slug string) (Workspace, error) {
	out, err := scanWorkspace(s.pool.QueryRow(ctx,
		`SELECT `+workspaceColumns+` FROM workspaces WHERE slug = $1`, slug))
	return out, mapNotFound(err)
}

// UpdateWorkspace rewrites the editable facts of one workspace (everything
// except its immutable id and slug).
func (s *Store) UpdateWorkspace(ctx context.Context, w Workspace) (Workspace, error) {
	if strings.TrimSpace(w.Name) == "" {
		return Workspace{}, ErrNameRequired
	}
	out, err := scanWorkspace(s.pool.QueryRow(ctx,
		`UPDATE workspaces SET name = $2, website_url = $3, status = $4, updated_at = now()
		 WHERE id = $1 RETURNING `+workspaceColumns,
		w.ID, w.Name, w.WebsiteURL, w.Status))
	return out, mapNotFound(err)
}

// ListWorkspaces returns every active workspace, ordered by name.
func (s *Store) ListWorkspaces(ctx context.Context) ([]Workspace, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+workspaceColumns+` FROM workspaces WHERE status = 'active' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Workspace{}
	for rows.Next() {
		w, err := scanWorkspace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// DeleteWorkspace removes a workspace and, by cascade, all of its client data.
//
// Published artifacts must be removed explicitly inside this transaction: their
// FK to integrations has no ON DELETE action (an integration disconnect is a
// RESTRICT, preserving the historical record), so the plain cascade would fail
// whenever a workspace has ever published anything.
func (s *Store) DeleteWorkspace(ctx context.Context, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after a successful commit

	// Artifacts whose publishing job belongs to this workspace.
	if _, err := tx.Exec(ctx,
		`DELETE FROM published_artifacts pa
		 USING publishing_jobs pj
		 JOIN content_items ci ON ci.id = pj.content_item_id
		 WHERE pa.publishing_job_id = pj.id AND ci.workspace_id = $1`, id); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

func scanWorkspace(row pgx.Row) (Workspace, error) {
	var w Workspace
	if err := row.Scan(&w.ID, &w.Slug, &w.Name, &w.WebsiteURL, &w.Status, &w.CreatedAt, &w.UpdatedAt); err != nil {
		return Workspace{}, err
	}
	return w, nil
}

func mapNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
