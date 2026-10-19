package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// PublishingJob is one attempt to publish one approved version through one
// integration. The workflow that creates and runs jobs arrives in Milestone 9;
// until then the store only guarantees the data layer exists and is scoped.
type PublishingJob struct {
	ID               string
	ContentItemID    string
	ContentVersionID string
	IntegrationID    string
	IdempotencyKey   string
	Status           string
	ScheduledAt      *time.Time
	Attempts         int
	LastError        *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

const publishingJobColumns = `id, content_item_id, content_version_id, integration_id,
	idempotency_key, status, scheduled_at, attempts, last_error, created_at, updated_at`

// ValidJobStatuses mirrors the CHECK constraint in migration 0002.
var validJobStatuses = map[string]bool{
	"pending": true, "running": true, "succeeded": true, "failed": true, "cancelled": true,
}

// InsertPublishingJob records a publishing job whose approved version belongs
// to workspaceID.
//
// The approval gate lives here (Milestone 5 acceptance): the targeted version
// must carry an 'approved' decision AND still be the item's newest version.
// A never-approved version, or one superseded by an edit after approval,
// returns ErrNotApproved — unapproved or changed content cannot be published.
// The keyed uniqueness (idempotency_key) plus the join through workspace-
// scoped tables means a duplicated or cross-workspace request cannot enqueue
// work for another workspace.
func (s *Store) InsertPublishingJob(ctx context.Context, workspaceID string, j PublishingJob) (PublishingJob, error) {
	var approved int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*)
		 FROM approvals a
		 JOIN content_versions cv ON cv.id = a.content_version_id
		 JOIN content_items ci ON ci.id = cv.content_item_id
		 WHERE ci.id = $1 AND ci.workspace_id = $2 AND cv.id = $3
			AND a.decision = 'approved'
			AND cv.version_number = (
				SELECT max(v2.version_number) FROM content_versions v2
				WHERE v2.content_item_id = ci.id)`,
		j.ContentItemID, workspaceID, j.ContentVersionID).Scan(&approved)
	if err != nil {
		return PublishingJob{}, err
	}
	if approved == 0 {
		return PublishingJob{}, ErrNotApproved
	}
	if j.Status == "" {
		j.Status = "pending"
	}
	if !validJobStatuses[j.Status] {
		return PublishingJob{}, ErrInvalidStatus
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO publishing_jobs
			(content_item_id, content_version_id, integration_id, idempotency_key, status, scheduled_at)
		 SELECT ci.id, cv.id, in2.id, $5, $6, $7
		 FROM content_items ci
		 JOIN content_versions cv ON cv.id = $2 AND cv.content_item_id = ci.id
		 JOIN integrations in2 ON in2.id = $3 AND in2.workspace_id = ci.workspace_id
		 WHERE ci.id = $1 AND ci.workspace_id = $4
		 RETURNING `+publishingJobColumns,
		j.ContentItemID, j.ContentVersionID, j.IntegrationID, workspaceID,
		j.IdempotencyKey, j.Status, j.ScheduledAt)
	out, err := scanPublishingJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return PublishingJob{}, ErrNotFound
	}
	return out, err
}

// GetPublishingJobByID reads one job through the workspace of its item.
func (s *Store) GetPublishingJobByID(ctx context.Context, workspaceID, jobID string) (PublishingJob, error) {
	var j PublishingJob
	err := s.pool.QueryRow(ctx,
		`SELECT pj.id, pj.content_item_id, pj.content_version_id, pj.integration_id,
			pj.idempotency_key, pj.status, pj.scheduled_at, pj.attempts, pj.last_error,
			pj.created_at, pj.updated_at
		 FROM publishing_jobs pj
		 JOIN content_items ci ON ci.id = pj.content_item_id
		 WHERE pj.id = $1 AND ci.workspace_id = $2`, jobID, workspaceID,
	).Scan(&j.ID, &j.ContentItemID, &j.ContentVersionID, &j.IntegrationID,
		&j.IdempotencyKey, &j.Status, &j.ScheduledAt, &j.Attempts, &j.LastError,
		&j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return PublishingJob{}, mapNotFound(err)
	}
	return j, nil
}

// GetPublishingJobByIdempotencyKey looks a retry up by its key, scoped to the
// workspace so two workspaces stay independent. Because the stored key column
// is unique platform-wide, keys should be namespaced per workspace (e.g.
// "<workspace-slug>:<key>") by the workflow layer that creates them.
func (s *Store) GetPublishingJobByIdempotencyKey(ctx context.Context, workspaceID, key string) (PublishingJob, error) {
	var j PublishingJob
	err := s.pool.QueryRow(ctx,
		`SELECT pj.id, pj.content_item_id, pj.content_version_id, pj.integration_id,
			pj.idempotency_key, pj.status, pj.scheduled_at, pj.attempts, pj.last_error,
			pj.created_at, pj.updated_at
		 FROM publishing_jobs pj
		 JOIN content_items ci ON ci.id = pj.content_item_id
		 WHERE pj.idempotency_key = $1 AND ci.workspace_id = $2`, key, workspaceID,
	).Scan(&j.ID, &j.ContentItemID, &j.ContentVersionID, &j.IntegrationID,
		&j.IdempotencyKey, &j.Status, &j.ScheduledAt, &j.Attempts, &j.LastError,
		&j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return PublishingJob{}, mapNotFound(err)
	}
	return j, nil
}

func scanPublishingJob(row pgx.Row) (PublishingJob, error) {
	var j PublishingJob
	if err := row.Scan(&j.ID, &j.ContentItemID, &j.ContentVersionID, &j.IntegrationID,
		&j.IdempotencyKey, &j.Status, &j.ScheduledAt, &j.Attempts, &j.LastError,
		&j.CreatedAt, &j.UpdatedAt); err != nil {
		return PublishingJob{}, err
	}
	return j, nil
}

// PublishedArtifact records where an approved version landed.
type PublishedArtifact struct {
	ID              string
	PublishingJobID string
	IntegrationID   string
	ExternalID      *string
	ExternalURL     *string
	PublishedAt     time.Time
}

// RecordPublishedArtifact stores the destination of one publishing job. The
// job must be workspace-scoped by the caller through GetPublishingJobByID.
func (s *Store) RecordPublishedArtifact(ctx context.Context, jobID, integrationID string, externalID, externalURL *string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO published_artifacts (publishing_job_id, integration_id, external_id, external_url)
		 VALUES ($1, $2, $3, $4)`, jobID, integrationID, externalID, externalURL)
	return err
}

// GetPublishedArtifact returns the destination recorded for one job.
func (s *Store) GetPublishedArtifact(ctx context.Context, jobID string) (PublishedArtifact, error) {
	var a PublishedArtifact
	err := s.pool.QueryRow(ctx,
		`SELECT id, publishing_job_id, integration_id, external_id, external_url, published_at
		 FROM published_artifacts WHERE publishing_job_id = $1`, jobID,
	).Scan(&a.ID, &a.PublishingJobID, &a.IntegrationID, &a.ExternalID, &a.ExternalURL, &a.PublishedAt)
	if err != nil {
		return PublishedArtifact{}, mapNotFound(err)
	}
	return a, nil
}
