package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---- Content versions --------------------------------------------------------

// ContentVersion is one immutable snapshot of a content item.
type ContentVersion struct {
	ID            string
	ContentItemID string
	VersionNumber int
	Title         string
	Excerpt       *string
	Body          string
	CallToAction  *string
	ChannelCopy   map[string]any
	Metadata      map[string]any
	CreatedAt     time.Time
}

// InsertContentVersion stores the next numbered snapshot of a content item.
// The item must belong to workspaceID; the SQL joins content_items so a
// version handed to the wrong item cannot be attached through isolation gaps.
func (s *Store) InsertContentVersion(ctx context.Context, workspaceID string, v ContentVersion) (ContentVersion, error) {
	if v.Metadata == nil {
		v.Metadata = map[string]any{}
	}
	if v.ChannelCopy == nil {
		v.ChannelCopy = map[string]any{}
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO content_versions (content_item_id, version_number, title, excerpt, body, call_to_action, channel_copy, metadata)
		 SELECT ci.id, coalesce(max(cv.version_number), 0) + 1, $3, $4, $5, $6, $7, $8
		 FROM content_items ci
		 LEFT JOIN content_versions cv ON cv.content_item_id = ci.id
		 WHERE ci.id = $1 AND ci.workspace_id = $2
		 GROUP BY ci.id
		 RETURNING `+contentVersionColumns,
		v.ContentItemID, workspaceID, v.Title, v.Excerpt, v.Body, v.CallToAction, v.ChannelCopy, v.Metadata)
	var out ContentVersion
	if err := row.Scan(&out.ID, &out.ContentItemID, &out.VersionNumber, &out.Title,
		&out.Excerpt, &out.Body, &out.CallToAction, &out.ChannelCopy, &out.Metadata, &out.CreatedAt); err != nil {
		return ContentVersion{}, mapNotFound(err)
	}
	return out, nil
}

// ListContentVersionsInWorkspace returns the snapshots of one item, newest last.
func (s *Store) ListContentVersionsInWorkspace(ctx context.Context, workspaceID, itemID string) ([]ContentVersion, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT cv.id, cv.content_item_id, cv.version_number, cv.title, cv.excerpt, cv.body,
			cv.call_to_action, cv.channel_copy, cv.metadata, cv.created_at
		 FROM content_versions cv
		 JOIN content_items ci ON ci.id = cv.content_item_id
		 WHERE ci.workspace_id = $1 AND cv.content_item_id = $2
		 ORDER BY cv.version_number`, workspaceID, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ContentVersion{}
	for rows.Next() {
		var v ContentVersion
		if err := rows.Scan(&v.ID, &v.ContentItemID, &v.VersionNumber, &v.Title,
			&v.Excerpt, &v.Body, &v.CallToAction, &v.ChannelCopy, &v.Metadata, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetContentVersionInWorkspace reads one snapshot only through its workspace.
func (s *Store) GetContentVersionInWorkspace(ctx context.Context, workspaceID, versionID string) (ContentVersion, error) {
	var v ContentVersion
	err := s.pool.QueryRow(ctx,
		`SELECT cv.id, cv.content_item_id, cv.version_number, cv.title, cv.excerpt, cv.body,
			cv.call_to_action, cv.channel_copy, cv.metadata, cv.created_at
		 FROM content_versions cv
		 JOIN content_items ci ON ci.id = cv.content_item_id
		 WHERE cv.id = $1 AND ci.workspace_id = $2`, versionID, workspaceID,
	).Scan(&v.ID, &v.ContentItemID, &v.VersionNumber, &v.Title, &v.Excerpt, &v.Body,
		&v.CallToAction, &v.ChannelCopy, &v.Metadata, &v.CreatedAt)
	if err != nil {
		return ContentVersion{}, mapNotFound(err)
	}
	return v, nil
}

const contentVersionColumns = `id, content_item_id, version_number, title, excerpt, body, call_to_action, channel_copy, metadata, created_at`

func scanContentVersion(row pgx.Row) (ContentVersion, error) {
	var v ContentVersion
	if err := row.Scan(&v.ID, &v.ContentItemID, &v.VersionNumber, &v.Title,
		&v.Excerpt, &v.Body, &v.CallToAction, &v.ChannelCopy, &v.Metadata, &v.CreatedAt); err != nil {
		return ContentVersion{}, err
	}
	return v, nil
}

// ---- Approvals -------------------------------------------------------------

// Approval records one decision on one exact version of one content item.
type Approval struct {
	ID               string
	ContentItemID    string
	ContentVersionID string
	Decision         string
	OperatorID       string
	Note             *string
	DecidedAt        time.Time
}

// rowQuerier is satisfied by both *pgxpool.Pool and pgx.Tx.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// RecordApproval stores an approval or rejection of one exact version of one
// content item. The insert only succeeds when the version is a snapshot of
// that item in that workspace, so approval rows cannot be forged across the
// isolation boundary.
func (s *Store) RecordApproval(ctx context.Context, workspaceID, itemID, versionID, decision, operatorID string, note *string) (Approval, error) {
	return recordApprovalInsert(ctx, s.pool, workspaceID, itemID, versionID, decision, operatorID, note)
}

// recordApprovalInsert performs the guarded insert on any query carrier
// (pool for standalone writes, a tx for multi-step flows).
func recordApprovalInsert(ctx context.Context, q rowQuerier, workspaceID, itemID, versionID, decision, operatorID string, note *string) (Approval, error) {
	row := q.QueryRow(ctx,
		`INSERT INTO approvals (content_item_id, content_version_id, decision, operator_id, note)
		 SELECT cv.content_item_id, cv.id, $4, $5, $6
		 FROM content_versions cv
		 JOIN content_items ci ON ci.id = cv.content_item_id
		 WHERE cv.id = $3 AND ci.id = $2 AND ci.workspace_id = $1
		 RETURNING id, decided_at`,
		workspaceID, itemID, versionID, decision, operatorID, note)
	var a Approval
	if err := row.Scan(&a.ID, &a.DecidedAt); err != nil {
		return Approval{}, mapNotFound(err)
	}
	a.ContentItemID, a.ContentVersionID, a.Decision, a.OperatorID, a.Note = itemID, versionID, decision, operatorID, note
	return a, nil
}

// ---- Audit events -----------------------------------------------------------

// AuditEvent is one recorded action, secret-free by convention.
type AuditEvent struct {
	ID          string
	WorkspaceID *string
	Action      string
	EntityType  string
	EntityID    *string
	Details     map[string]any
	CreatedAt   time.Time
}

// AddAuditEvent records an operator-visible, secret-free action row.
func (s *Store) AddAuditEvent(ctx context.Context, e AuditEvent) error {
	if e.Details == nil {
		e.Details = map[string]any{}
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO audit_events (workspace_id, action, entity_type, entity_id, details)
		 VALUES ($1, $2, $3, $4, $5)`,
		e.WorkspaceID, e.Action, e.EntityType, e.EntityID, e.Details)
	return err
}

// ListAuditEvents returns the audit trail of one workspace, newest first.
func (s *Store) ListAuditEvents(ctx context.Context, workspaceID string, limit int) ([]AuditEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, workspace_id, action, entity_type, entity_id, details, created_at
		 FROM audit_events WHERE workspace_id = $1
		 ORDER BY created_at DESC LIMIT $2`, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEvent{}
	for rows.Next() {
		var e AuditEvent
		if err := rows.Scan(&e.ID, &e.WorkspaceID, &e.Action, &e.EntityType, &e.EntityID, &e.Details, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
