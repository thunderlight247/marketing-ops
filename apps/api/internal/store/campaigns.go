// Campaign storage (Milestone 11): workspace-scoped funnel groupings for
// content and enquiries. One campaign belongs to exactly one workspace.
package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Campaign is one marketing campaign inside a workspace.
type Campaign struct {
	ID          string
	WorkspaceID string
	Name        string
	Status      string
	UtmCampaign string
	Notes       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

const campaignColumns = `id, workspace_id, name, status, utm_campaign, notes, created_at, updated_at`

func campaignValidStatus(status string) bool {
	switch status {
	case "planned", "active", "paused", "completed":
		return true
	}
	return false
}

func campaignValidName(name string) bool {
	return strings.TrimSpace(name) != ""
}

// InsertCampaign creates one campaign in a workspace.
func (s *Store) InsertCampaign(ctx context.Context, c Campaign) (Campaign, error) {
	if !campaignValidName(c.Name) {
		return Campaign{}, ErrNameRequired
	}
	if c.Status == "" {
		c.Status = "planned"
	}
	if !campaignValidStatus(c.Status) {
		return Campaign{}, ErrInvalidStatus
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO campaigns (workspace_id, name, status, utm_campaign, notes)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (workspace_id, name) DO NOTHING
		 RETURNING `+campaignColumns,
		c.WorkspaceID, c.Name, c.Status, c.UtmCampaign, c.Notes)
	out, err := scanCampaign(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Campaign{}, ErrSlugTaken // same per-workspace name already exists
	}
	return out, err
}

// ListCampaigns returns the campaigns of exactly one workspace, newest first.
func (s *Store) ListCampaigns(ctx context.Context, workspaceID string) ([]Campaign, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+campaignColumns+` FROM campaigns
		 WHERE workspace_id = $1 ORDER BY updated_at DESC, created_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Campaign{}
	for rows.Next() {
		c, err := scanCampaign(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetCampaignInWorkspace reads one campaign only if both match.
func (s *Store) GetCampaignInWorkspace(ctx context.Context, workspaceID, id string) (Campaign, error) {
	c, err := scanCampaign(s.pool.QueryRow(ctx,
		`SELECT `+campaignColumns+` FROM campaigns
		 WHERE id = $1 AND workspace_id = $2`, id, workspaceID))
	return c, mapNotFound(err)
}

// UpdateCampaign rewrites the editable fields of one campaign, bounded to its
// workspace. Empty status keeps the current value.
func (s *Store) UpdateCampaign(ctx context.Context, c Campaign) (Campaign, error) {
	if !campaignValidName(c.Name) {
		return Campaign{}, ErrNameRequired
	}
	if c.Status != "" && !campaignValidStatus(c.Status) {
		return Campaign{}, ErrInvalidStatus
	}
	out, err := scanCampaign(s.pool.QueryRow(ctx,
		`UPDATE campaigns SET
			name = $2,
			status = CASE WHEN $3 = '' THEN status ELSE $3 END,
			utm_campaign = $4, notes = $5, updated_at = now()
		 WHERE id = $1 AND workspace_id = $6
		 RETURNING `+campaignColumns,
		c.ID, c.Name, c.Status, c.UtmCampaign, c.Notes, c.WorkspaceID))
	return out, mapNotFound(err)
}

func scanCampaign(row pgx.Row) (Campaign, error) {
	var c Campaign
	if err := row.Scan(&c.ID, &c.WorkspaceID, &c.Name, &c.Status, &c.UtmCampaign, &c.Notes,
		&c.CreatedAt, &c.UpdatedAt); err != nil {
		return Campaign{}, err
	}
	return c, nil
}
