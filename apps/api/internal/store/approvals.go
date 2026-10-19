// Approval workflow storage (Milestone 5).
//
// The lifecycle rule lives here: every decision is recorded against one exact
// version of one workspace-scoped item, the item status follows the decision,
// and any later edit resets the item to draft so nothing stale can be queued
// for publishing (the M9 scheduler will not queue a stale version anyway; see
// InsertPublishingJob).
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// contentStatusTransitions is the central, single definition of the content
// lifecycle. Every status change of a content item outside this map is
// rejected at the store boundary. queued/publishing/published/failed stay
// controlled by the scheduler arriving in Milestone 9.
var contentStatusTransitions = map[string]map[string]bool{
	"draft":    {"review": true, "approved": true, "rejected": true},
	"review":   {"approved": true, "rejected": true, "draft": true},
	"approved": {"draft": true}, // back to draft via the approval invalidation
	"rejected": {"draft": true, "review": true},
}

// transitionAllowed reports whether from→to is a legal lifecycle move.
func transitionAllowed(from, to string) bool {
	allowed, ok := contentStatusTransitions[from]
	if !ok {
		return false
	}
	return allowed[to]
}

// ApprovalDecisionInput is one operator decision about an item's latest
// version: "approve" or "reject", with an optional free-text note.
type ApprovalDecisionInput struct {
	Decision   string // "approve" | "reject"
	OperatorID string
	Note       *string
}

// DecideApproval records one approval decision about the newest version of
// one workspace-scoped item and moves the item status in the same
// transaction. The version to approve is implicit: always the newest one, so
// an older, superseded snapshot can never regain approval after an edit.
func (s *Store) DecideApproval(ctx context.Context, workspaceID, itemID string, in ApprovalDecisionInput) (ContentItem, Approval, error) {
	decision := map[string]string{"approve": "approved", "reject": "rejected"}[in.Decision]
	if decision == "" {
		return ContentItem{}, Approval{}, errors.New("unknown decision")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ContentItem{}, Approval{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after a successful commit

	// Lock the item and check the scoping + current status in SQL.
	var status string
	if err := tx.QueryRow(ctx,
		`SELECT status FROM content_items WHERE id = $1 AND workspace_id = $2 FOR UPDATE`,
		itemID, workspaceID,
	).Scan(&status); err != nil {
		if err == pgx.ErrNoRows {
			return ContentItem{}, Approval{}, ErrNotFound
		}
		return ContentItem{}, Approval{}, err
	}
	if !transitionAllowed(status, decision) {
		return ContentItem{}, Approval{}, ErrInvalidStatus
	}

	// The implicit target: the newest version. No versions yet means nothing
	// to decide about.
	latest, err := scanContentVersion(tx.QueryRow(ctx,
		`SELECT `+contentVersionColumns+`
		 FROM content_versions cv
		 WHERE cv.content_item_id = $1
		 ORDER BY cv.version_number DESC LIMIT 1`, itemID))
	if err != nil {
		if err == pgx.ErrNoRows {
			return ContentItem{}, Approval{}, ErrVersionNotLatest
		}
		return ContentItem{}, Approval{}, err
	}

	// The approval row is guarded by the join through the workspace, exactly
	// like RecordApproval; the UNIQUE(content_version_id) constraint turns a
	// duplicate decision into a sentinel instead of a driver error.
	approval, err := recordApprovalInsert(ctx, tx, workspaceID, itemID, latest.ID, decision, in.OperatorID, in.Note)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ContentItem{}, Approval{}, ErrAlreadyDecided
		}
		return ContentItem{}, Approval{}, err
	}

	updated, err := scanContentItem(tx.QueryRow(ctx,
		`UPDATE content_items SET status = $3, updated_at = now()
		 WHERE id = $1 AND workspace_id = $2 AND status = $4
		 RETURNING `+contentItemColumns,
		itemID, workspaceID, decision, status))
	if err != nil {
		return ContentItem{}, Approval{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return ContentItem{}, Approval{}, err
	}
	return updated, approval, nil
}

// ListApprovalsByItem returns the recorded decisions of one item, newest
// first, always scoped through the item's workspace.
func (s *Store) ListApprovalsByItem(ctx context.Context, workspaceID, itemID string) ([]Approval, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT a.id, a.content_item_id, a.content_version_id, a.decision,
			a.operator_id, a.note, a.decided_at
		 FROM approvals a
		 JOIN content_items ci ON ci.id = a.content_item_id
		 WHERE ci.workspace_id = $1 AND ci.id = $2
		 ORDER BY a.decided_at DESC`, workspaceID, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Approval{}
	for rows.Next() {
		var a Approval
		if err := rows.Scan(&a.ID, &a.ContentItemID, &a.ContentVersionID, &a.Decision,
			&a.OperatorID, &a.Note, &a.DecidedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
