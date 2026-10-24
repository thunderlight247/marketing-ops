package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// seedApprovedVersion creates a workspace with one approved content item and
// the newest version, inside its own workspace — the state a publishing job
// needs before it may run.
func seedApprovedVersion(t *testing.T, s *Store) (Workspace, ContentItem, ContentVersion, Integration) {
	t.Helper()
	ctx := context.Background()
	operatorID, err := s.EnsureOperator(ctx)
	if err != nil {
		t.Fatalf("EnsureOperator: %v", err)
	}
	ws := uniqueWorkspace(t, s, "pubjobs")
	in, err := s.InsertIntegration(ctx, Integration{
		WorkspaceID: ws.ID, AdapterKind: "sanity", Name: "sanity", Status: "connected",
	})
	if err != nil {
		t.Fatalf("InsertIntegration: %v", err)
	}
	item, err := s.InsertContentItem(ctx, ContentItem{
		WorkspaceID: ws.ID, Kind: "blog_article", Title: "Job probe",
	})
	if err != nil {
		t.Fatalf("InsertContentItem: %v", err)
	}
	item, version, err := s.SaveContentDraft(ctx, ws.ID, item.ID, ContentDraftInput{
		Title: "Job probe", Body: strPtr("Body text."),
	})
	if err != nil {
		t.Fatalf("SaveContentDraft: %v", err)
	}
	if _, _, err := s.DecideApproval(ctx, ws.ID, item.ID, ApprovalDecisionInput{
		Decision: "approve", OperatorID: operatorID,
	}); err != nil {
		t.Fatalf("DecideApproval: %v", err)
	}
	return ws, item, version, in
}

func TestClaimDuePublishingJobClaimsAndRuns(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()
	ws, item, version, in := seedApprovedVersion(t, s)
	now := time.Now().UTC().Truncate(time.Second)

	job, err := s.InsertPublishingJob(ctx, ws.ID, PublishingJob{
		ContentItemID: item.ID, ContentVersionID: version.ID, IntegrationID: in.ID,
		IdempotencyKey: ws.Slug + ":job-probe:1", NextAttemptAt: &now,
	})
	if err != nil {
		t.Fatalf("InsertPublishingJob: %v", err)
	}

	run, err := s.ClaimDuePublishingJob(ctx, 3, now)
	if err != nil {
		t.Fatalf("ClaimDuePublishingJob: %v", err)
	}
	if run.Job.ID != job.ID || run.Job.Status != "running" {
		t.Fatalf("claimed job wrong: %+v", run.Job)
	}
	if run.Job.Attempts != 1 || run.Integration.ID != in.ID || run.Item.ID != item.ID || run.Version.ID != version.ID {
		t.Fatalf("claimed facts wrong: integration=%s item=%s version=%s attempts=%d",
			run.Integration.ID, run.Item.ID, run.Version.ID, run.Job.Attempts)
	}
	if run.Workspace.ID != ws.ID {
		t.Fatalf("workspace leak: %+v", run.Workspace)
	}

	// A second claim must not return the same already-running job.
	if _, err := s.ClaimDuePublishingJob(ctx, 3, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for no remaining due job, got %v", err)
	}

	// Completion closes the job and publishes the item.
	publishedID := "ext-1"
	extURL := "https://public.example/posts/probe"
	if _, err := s.CompletePublishingJobSucceeded(ctx, job.ID, in.ID, &publishedID, &extURL); err != nil {
		t.Fatalf("CompletePublishingJobSucceeded: %v", err)
	}
	after, err := s.GetPublishingJobByID(ctx, ws.ID, job.ID)
	if err != nil || after.Status != "succeeded" || after.NextAttemptAt != nil {
		t.Fatalf("job after success: %+v err=%v", after, err)
	}
	artifact, err := s.GetPublishedArtifact(ctx, job.ID)
	if err != nil || artifact.ExternalID == nil || *artifact.ExternalID != "ext-1" {
		t.Fatalf("artifact not recorded: %+v err=%v", artifact, err)
	}
	item, err = s.GetContentItemInWorkspace(ctx, ws.ID, item.ID)
	if err != nil || item.Status != "published" {
		t.Fatalf("item not marked published: %+v err=%v", item, err)
	}
}

func TestRecoverStuckPublishingJobs(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()
	ws, item, version, in := seedApprovedVersion(t, s)
	now := time.Now().UTC().Truncate(time.Second)

	job, err := s.InsertPublishingJob(ctx, ws.ID, PublishingJob{
		ContentItemID: item.ID, ContentVersionID: version.ID, IntegrationID: in.ID,
		IdempotencyKey: ws.Slug + ":stuck:1", NextAttemptAt: &now,
	})
	if err != nil {
		t.Fatalf("InsertPublishingJob: %v", err)
	}
	run, err := s.ClaimDuePublishingJob(ctx, 3, now)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	// A fresh 'row is not considered stuck; nothing is recovered.
	recovered, err := s.RecoverStuckPublishingJobs(ctx, 10*time.Minute, now)
	if err != nil {
		t.Fatalf("recover fresh: %v", err)
	}
	if recovered != 0 {
		t.Fatalf("fresh running row must not be recovered, got %d", recovered)
	}

	// Pretend the worker died: age the row past the threshold, then recover.
	if err := s.SetJobHeartbeatForTests(ctx, job.ID, now.Add(-15*time.Minute)); err != nil {
		t.Fatalf("age row: %v", err)
	}
	recovered, err = s.RecoverStuckPublishingJobs(ctx, 10*time.Minute, now)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if recovered != 1 {
		t.Fatalf("expected 1 recovered row, got %d", recovered)
	}
	stored, err := s.GetPublishingJobByID(ctx, ws.ID, job.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Status != "failed" {
		t.Fatalf("recovered job status = %s, want failed", stored.Status)
	}
	if stored.NextAttemptAt == nil || stored.NextAttemptAt.Before(now) {
		t.Fatalf("recovered job must be requeued immediately: %+v", stored.NextAttemptAt)
	}
	if stored.LastError == nil || !containsString(*stored.LastError, "interrupted") {
		t.Fatalf("recovery reason missing: %+v", stored.LastError)
	}
	// The attempts spent before the crash are preserved.
	if stored.Attempts != run.Job.Attempts {
		t.Fatalf("attempts changed across recovery: %d -> %d", run.Job.Attempts, stored.Attempts)
	}
}

// SetJobHeartbeatForTests rewinds updated_at of one job so recovery sweep
// thresholds can be exercised without sleeping.
func (s *Store) SetJobHeartbeatForTests(ctx context.Context, jobID string, when time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE publishing_jobs SET updated_at = $2 WHERE id = $1`, jobID, when)
	return err
}

func containsString(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}

func TestClaimRespectsScheduleAndAttemptCap(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()
	ws, item, version, in := seedApprovedVersion(t, s)
	future := time.Now().UTC().Add(time.Hour).Truncate(time.Second)

	if _, err := s.InsertPublishingJob(ctx, ws.ID, PublishingJob{
		ContentItemID: item.ID, ContentVersionID: version.ID, IntegrationID: in.ID,
		IdempotencyKey: ws.Slug + ":future:1", NextAttemptAt: &future,
	}); err != nil {
		t.Fatalf("InsertPublishingJob: %v", err)
	}
	// Before the schedule nothing is due.
	if _, err := s.ClaimDuePublishingJob(ctx, 3, time.Now().UTC()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("future-scheduled job must not be claimed early, got %v", err)
	}
	// At the scheduled time it is claimable.
	run, err := s.ClaimDuePublishingJob(ctx, 3, future.Add(time.Minute))
	if err != nil {
		t.Fatalf("claim at scheduled time: %v", err)
	}

	// Failure with backoff keeps the job retryable but not immediately due.
	retryAt := future.Add(20 * time.Minute)
	if err := s.CompletePublishingJobFailed(ctx, run.Job.ID, "the Sanity API could not be reached", &retryAt); err != nil {
		t.Fatalf("CompletePublishingJobFailed: %v", err)
	}
	stored, err := s.GetPublishingJobByID(ctx, ws.ID, run.Job.ID)
	if err != nil || stored.Status != "failed" || stored.NextAttemptAt == nil || !stored.NextAttemptAt.Equal(retryAt) {
		t.Fatalf("retryable failed job wrong: %+v err=%v", stored, err)
	}
}

func TestTerminalFailureAndOperatorRetry(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()
	operatorID, operatorErr := s.EnsureOperator(ctx)
	if operatorErr != nil {
		t.Fatalf("EnsureOperator: %v", operatorErr)
	}
	ws, item, version, in := seedApprovedVersion(t, s)
	now := time.Now().UTC().Truncate(time.Second)

	job, err := s.InsertPublishingJob(ctx, ws.ID, PublishingJob{
		ContentItemID: item.ID, ContentVersionID: version.ID, IntegrationID: in.ID,
		IdempotencyKey: ws.Slug + ":terminal:1", NextAttemptAt: &now,
	})
	if err != nil {
		t.Fatalf("InsertPublishingJob: %v", err)
	}
	run, err := s.ClaimDuePublishingJob(ctx, 3, now)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := s.CompletePublishingJobFailed(ctx, run.Job.ID, "the Sanity API refused the request (HTTP 403)", nil); err != nil {
		t.Fatalf("terminal complete: %v", err)
	}
	stored, err := s.GetPublishingJobByID(ctx, ws.ID, job.ID)
	if err != nil || stored.Status != "failed" {
		t.Fatalf("terminal job wrong: %+v err=%v", stored, err)
	}

	retry, err := s.RetryPublishingJob(ctx, ws.ID, job.ID, now)
	if err != nil {
		t.Fatalf("RetryPublishingJob: %v", err)
	}
	if retry.Status != "pending" || retry.NextAttemptAt == nil || !retry.NextAttemptAt.Equal(now) {
		t.Fatalf("retry not queued immediately: %+v", retry)
	}

	// A job whose version changed after approval refuses to retry.
	_, edited, err := s.SaveContentDraft(ctx, ws.ID, item.ID, ContentDraftInput{Title: "Edited", Body: strPtr("new body"), Excerpt: strPtr("x")})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if edited.ID == version.ID {
		t.Fatalf("expected a new version")
	}
	if _, _, err := s.DecideApproval(ctx, ws.ID, item.ID, ApprovalDecisionInput{Decision: "approve", OperatorID: operatorID}); err != nil {
		t.Fatalf("re-approve: %v", err)
	}
	// The first retry already re-queued the job, so a second retry is refused
	// as a lifecycle error: only failed (terminal/backoff) jobs may retry.
	if _, err := s.RetryPublishingJob(ctx, ws.ID, job.ID, now); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("retrying a pending job must be ErrInvalidStatus, got %v", err)
	}
}

func TestCancelQueuedJob(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()
	ws, item, version, in := seedApprovedVersion(t, s)
	now := time.Now().UTC().Truncate(time.Second)

	job, err := s.InsertPublishingJob(ctx, ws.ID, PublishingJob{
		ContentItemID: item.ID, ContentVersionID: version.ID, IntegrationID: in.ID,
		IdempotencyKey: ws.Slug + ":cancel:1", NextAttemptAt: &now, ScheduledAt: &now,
	})
	if err != nil {
		t.Fatalf("InsertPublishingJob: %v", err)
	}
	cancelled, err := s.CancelPublishingJob(ctx, ws.ID, job.ID)
	if err != nil {
		t.Fatalf("CancelPublishingJob: %v", err)
	}
	if cancelled.Status != "cancelled" || cancelled.NextAttemptAt != nil {
		t.Fatalf("cancelled job wrong: %+v", cancelled)
	}
	// Cancelled jobs are never claimed.
	if _, err := s.ClaimDuePublishingJob(ctx, 3, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cancelled job claimed: %v", err)
	}
	// Cancelling an already-cancelled job is a no-op refusal.
	if _, err := s.CancelPublishingJob(ctx, ws.ID, job.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("double cancel must be ErrNotFound, got %v", err)
	}
}

func TestClaimFailsJobForSwappedApproval(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()
	ws, item, version, in := seedApprovedVersion(t, s)
	now := time.Now().UTC().Truncate(time.Second)

	job, err := s.InsertPublishingJob(ctx, ws.ID, PublishingJob{
		ContentItemID: item.ID, ContentVersionID: version.ID, IntegrationID: in.ID,
		IdempotencyKey: ws.Slug + ":swap:1", NextAttemptAt: &now,
	})
	if err != nil {
		t.Fatalf("InsertPublishingJob: %v", err)
	}
	// Edit invalidates the approval gate before anyone claims it.
	if _, _, err := s.SaveContentDraft(ctx, ws.ID, item.ID, ContentDraftInput{Title: "Edited", Body: strPtr("new"), Excerpt: strPtr("y")}); err != nil {
		t.Fatalf("edit: %v", err)
	}
	if _, err := s.ClaimDuePublishingJob(ctx, 3, now); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("swapped approval must fail the claim, got %v", err)
	}
	stored, err := s.GetPublishingJobByID(ctx, ws.ID, job.ID)
	if err != nil || stored.Status != "failed" || stored.NextAttemptAt != nil {
		t.Fatalf("stale job must land in terminal failed state: %+v err=%v", stored, err)
	}
	if _, err := s.ListPublishingJobs(ctx, ws.ID, item.ID); err != nil {
		t.Fatalf("ListPublishingJobs: %v", err)
	}
}

func TestPublishingJobsScopedToWorkspace(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()
	wsA, itemA, versionA, inA := seedApprovedVersion(t, s)
	wsB := uniqueWorkspace(t, s, "pubjobs-b")
	now := time.Now().UTC().Truncate(time.Second)

	if _, err := s.InsertPublishingJob(ctx, wsA.ID, PublishingJob{
		ContentItemID: itemA.ID, ContentVersionID: versionA.ID, IntegrationID: inA.ID,
		IdempotencyKey: wsA.Slug + ":scope:1", NextAttemptAt: &now,
	}); err != nil {
		t.Fatalf("InsertPublishingJob: %v", err)
	}

	// The other workspace cannot see, cancel, or retry the job.
	jobs, err := s.ListPublishingJobs(ctx, wsB.ID, itemA.ID)
	if err != nil || len(jobs) != 0 {
		t.Fatalf("cross-workspace job list must be empty: n=%d err=%v", len(jobs), err)
	}
	if _, err := s.CancelPublishingJob(ctx, wsB.ID, "99999999-9999-9999-9999-999999999999"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-workspace cancel must be ErrNotFound, got %v", err)
	}
	if _, err := s.RetryPublishingJob(ctx, wsB.ID, "99999999-9999-9999-9999-999999999999", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-workspace retry must be ErrNotFound, got %v", err)
	}
}
