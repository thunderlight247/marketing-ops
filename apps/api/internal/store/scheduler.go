package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// PublishingJobRun bundles everything a background publisher needs after it
// claims one job: the job row plus the workspace, integration, and the exact
// content version it targets. Assembled inside the claim transaction so the
// facts cannot drift between claim and run.
type PublishingJobRun struct {
	Job         PublishingJob
	Workspace   Workspace
	Integration Integration
	Item        ContentItem
	Version     ContentVersion
}

// ClaimDuePublishingJob atomically claims one runnable job.
//
// A job is runnable when it is pending or a retryable failed job, its
// next_attempt_at is due (now or earlier), and attempts stays below
// maxAttempts. FOR UPDATE SKIP LOCKED means concurrent workers (or a restarted
// process racing itself) cannot claim the same row twice; each worker picks a
// different due job. ErrNotFound means nothing is due.
//
// The claim also flips the row to status 'running' and increments attempts in
// the same transaction, so no claimed job can be lost or double-run across
// process restarts. Interrupted claims leave a 'running' row behind; the
// operator sees it the publishing history and may retry it.
func (s *Store) ClaimDuePublishingJob(ctx context.Context, maxAttempts int, now time.Time) (PublishingJobRun, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PublishingJobRun{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after a successful commit

	var run PublishingJobRun
	var scanJob PublishingJob
	err = tx.QueryRow(ctx,
		`SELECT `+publishingJobColumns+`
		 FROM publishing_jobs pj
		 WHERE pj.status IN ('pending', 'failed')
		   AND pj.next_attempt_at IS NOT NULL
		   AND pj.next_attempt_at <= $1
		   AND pj.attempts < $2
		 ORDER BY pj.next_attempt_at
		 LIMIT 1
		 FOR UPDATE OF pj SKIP LOCKED`,
		now, maxAttempts,
	).Scan(&scanJob.ID, &scanJob.ContentItemID, &scanJob.ContentVersionID, &scanJob.IntegrationID,
		&scanJob.IdempotencyKey, &scanJob.Status, &scanJob.ScheduledAt, &scanJob.Attempts,
		&scanJob.LastError, &scanJob.NextAttemptAt, &scanJob.CreatedAt, &scanJob.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PublishingJobRun{}, ErrNotFound
		}
		return PublishingJobRun{}, err
	}
	run.Job = scanJob

	var row pgx.Row
	// The version must still be the item's newest AND carry an approval gate —
	// re-checked here because edits after enqueue reset the approval.
	var approved int
	err = tx.QueryRow(ctx,
		`SELECT count(*)
		 FROM approvals a
		 JOIN content_versions cv ON cv.id = a.content_version_id
		 WHERE cv.content_item_id = $1 AND cv.id = $2 AND a.decision = 'approved'
		   AND cv.version_number = (
		     SELECT max(v2.version_number) FROM content_versions v2
		     WHERE v2.content_item_id = $1)`,
		scanJob.ContentItemID, scanJob.ContentVersionID).Scan(&approved)
	if err != nil {
		return PublishingJobRun{}, err
	}
	if approved == 0 {
		// The content changed after the job was queued: send it to terminal
		// failed state so an operator can re-review instead of publishing stale
		// text that was swapped in after approval.
		if _, failErr := tx.Exec(ctx,
			`UPDATE publishing_jobs
			 SET status = 'failed',
			     last_error = 'the approved version changed after the job was queued; approve it again or retry the job',
			     next_attempt_at = NULL,
			     updated_at = now()
			 WHERE id = $1`, scanJob.ID); failErr != nil {
			return PublishingJobRun{}, failErr
		}
		// Commit the terminal-failure write before reporting the sentinel.
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return PublishingJobRun{}, commitErr
		}
		defer func() { _ = tx.Rollback(ctx) }() // disarm the deferred rollback
		return PublishingJobRun{}, ErrNotApproved
	}

	if err := tx.QueryRow(ctx,
		`UPDATE publishing_jobs
		 SET status = 'running', attempts = attempts + 1, updated_at = now()
		 WHERE id = $1
		 RETURNING `+publishingJobColumns,
		scanJob.ID,
	).Scan(&scanJob.ID, &scanJob.ContentItemID, &scanJob.ContentVersionID, &scanJob.IntegrationID,
		&scanJob.IdempotencyKey, &scanJob.Status, &scanJob.ScheduledAt, &scanJob.Attempts,
		&scanJob.LastError, &scanJob.NextAttemptAt, &scanJob.CreatedAt, &scanJob.UpdatedAt); err != nil {
		return PublishingJobRun{}, err
	}
	run.Job = scanJob

	row = tx.QueryRow(ctx,
		`SELECT `+workspaceColumns+` FROM workspaces
		 WHERE id = (SELECT workspace_id FROM content_items WHERE id = $1)`,
		scanJob.ContentItemID)
	run.Workspace, err = scanWorkspace(row)
	if err != nil {
		return PublishingJobRun{}, err
	}

	row = tx.QueryRow(ctx,
		`SELECT `+integrationColumns+` FROM integrations WHERE id = $1`,
		scanJob.IntegrationID)
	run.Integration, err = scanIntegration(row)
	if err != nil {
		return PublishingJobRun{}, err
	}

	row = tx.QueryRow(ctx,
		`SELECT `+contentItemColumns+` FROM content_items WHERE id = $1`,
		scanJob.ContentItemID)
	run.Item, err = scanContentItem(row)
	if err != nil {
		return PublishingJobRun{}, err
	}

	row = tx.QueryRow(ctx,
		`SELECT `+contentVersionColumns+` FROM content_versions WHERE id = $1`,
		scanJob.ContentVersionID)
	run.Version, err = scanContentVersion(row)
	if err != nil {
		return PublishingJobRun{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return PublishingJobRun{}, err
	}
	return run, nil
}

// CompletePublishingJobSucceeded closes one claimed job: status succeeded, the
// published artifact recorded (replaced when a retry re-publishes the same
// job), and the item marked published — in one transaction.
func (s *Store) CompletePublishingJobSucceeded(ctx context.Context, jobID, integrationID string, externalID, externalURL *string) (ContentItem, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ContentItem{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after a successful commit

	tag, err := tx.Exec(ctx,
		`UPDATE publishing_jobs
		 SET status = 'succeeded', last_error = NULL, next_attempt_at = NULL, updated_at = now()
		 WHERE id = $1 AND status = 'running'`, jobID)
	if err != nil {
		return ContentItem{}, err
	}
	if tag.RowsAffected() == 0 {
		return ContentItem{}, ErrConflict
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO published_artifacts (publishing_job_id, integration_id, external_id, external_url)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (publishing_job_id) DO UPDATE
		 SET external_id = EXCLUDED.external_id, external_url = EXCLUDED.external_url,
		     published_at = now()`,
		jobID, integrationID, externalID, externalURL); err != nil {
		return ContentItem{}, err
	}

	// Only an approved item moves to published; a stale version (edited after
	// the claim) must not swallow the newer workflow state.
	var itemRow ContentItem
	out, err := scanContentItem(tx.QueryRow(ctx,
		`UPDATE content_items
		 SET status = 'published', updated_at = now()
		 WHERE id = (SELECT content_item_id FROM publishing_jobs WHERE id = $1)
		   AND status = 'approved'
		 RETURNING `+contentItemColumns, jobID))
	// An already-published or manually-moved item keeps its current status.
	if err == nil {
		itemRow = out
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return ContentItem{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return ContentItem{}, err
	}
	return itemRow, nil
}

// Terminal attempts boundary for retry scheduling is enforced by the claim
// query (attempts < maxAttempts), not here.

// CompletePublishingJobFailed closes one failed attempt. When retryAfter is
// non-nil the job stays retryable with its next attempt due then; nil means
// terminal failure — failed jobs with NULL next_attempt_at wait for an
// operator retry.
func (s *Store) CompletePublishingJobFailed(ctx context.Context, jobID, lastErr string, retryAfter *time.Time) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE publishing_jobs
		 SET status = 'failed',
		     last_error = NULLIF($2, ''),
		     next_attempt_at = CASE WHEN $3::boolean THEN $4::timestamptz ELSE NULL END,
		     updated_at = now()
		 WHERE id = $1 AND status = 'running'`,
		jobID, lastErr, retryAfter != nil, retryAfter)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

// CancelPublishingJob lets an operator stop a queued or failed job. A running
// job cannot be cancelled from here (the worker owns it); 'succeeded' and
// already-'cancelled' jobs are refused.
func (s *Store) CancelPublishingJob(ctx context.Context, workspaceID, jobID string) (PublishingJob, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE publishing_jobs pj
		 SET status = 'cancelled', next_attempt_at = NULL, updated_at = now()
		 FROM content_items ci
		 WHERE pj.content_item_id = ci.id AND pj.id = $1 AND ci.workspace_id = $2
		   AND pj.status IN ('pending', 'failed')
		 RETURNING pj.id, pj.content_item_id, pj.content_version_id, pj.integration_id,
			pj.idempotency_key, pj.status, pj.scheduled_at, pj.attempts, pj.last_error,
			pj.next_attempt_at, pj.created_at, pj.updated_at`,
		jobID, workspaceID)
	out, err := scanPublishingJob(row)
	return out, mapNotFound(err)
}

// RetryPublishingJob re-enables one terminal (or backoff-waiting) failed job
// for an immediate attempt. The version must still be approved and current;
// otherwise ErrNotApproved is returned.
func (s *Store) RetryPublishingJob(ctx context.Context, workspaceID, jobID string, now time.Time) (PublishingJob, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PublishingJob{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after a successful commit

	var itemID, versionID, status string
	err = tx.QueryRow(ctx,
		`SELECT pj.content_item_id, pj.content_version_id, pj.status
		 FROM publishing_jobs pj
		 JOIN content_items ci ON ci.id = pj.content_item_id
		 WHERE pj.id = $1 AND ci.workspace_id = $2
		 FOR UPDATE OF pj`,
		jobID, workspaceID).Scan(&itemID, &versionID, &status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PublishingJob{}, ErrNotFound
		}
		return PublishingJob{}, err
	}
	if status != "failed" {
		return PublishingJob{}, ErrInvalidStatus
	}

	var approved int
	err = tx.QueryRow(ctx,
		`SELECT count(*)
		 FROM approvals a
		 JOIN content_versions cv ON cv.id = a.content_version_id
		 WHERE cv.content_item_id = $1 AND cv.id = $2 AND a.decision = 'approved'
		   AND cv.version_number = (
		     SELECT max(v2.version_number) FROM content_versions v2
		     WHERE v2.content_item_id = $1)`,
		itemID, versionID).Scan(&approved)
	if err != nil {
		return PublishingJob{}, err
	}
	if approved == 0 {
		return PublishingJob{}, ErrNotApproved
	}

	row := tx.QueryRow(ctx,
		`UPDATE publishing_jobs
		 SET status = 'pending', next_attempt_at = $2, updated_at = now()
		 WHERE id = $1
		 RETURNING `+publishingJobColumns,
		jobID, now)
	out, err := scanPublishingJob(row)
	if err != nil {
		return PublishingJob{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PublishingJob{}, err
	}
	return out, nil
}

// ListPublishingJobs returns the publishing history of one item through one
// workspace scope, newest first.
func (s *Store) ListPublishingJobs(ctx context.Context, workspaceID, itemID string) ([]PublishingJob, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT pj.id, pj.content_item_id, pj.content_version_id, pj.integration_id,
			pj.idempotency_key, pj.status, pj.scheduled_at, pj.attempts, pj.last_error,
			pj.next_attempt_at, pj.created_at, pj.updated_at
		 FROM publishing_jobs pj
		 JOIN content_items ci ON ci.id = pj.content_item_id
		 WHERE ci.id = $1 AND ci.workspace_id = $2
		 ORDER BY pj.created_at DESC`,
		itemID, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PublishingJob{}
	for rows.Next() {
		var j PublishingJob
		if err := rows.Scan(&j.ID, &j.ContentItemID, &j.ContentVersionID, &j.IntegrationID,
			&j.IdempotencyKey, &j.Status, &j.ScheduledAt, &j.Attempts, &j.LastError,
			&j.NextAttemptAt, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// ErrConflict signals a lifecycle move that no longer applies (for example
// completing a job that was already closed).
var ErrConflict = errors.New("that lifecycle change no longer applies to the current state")
