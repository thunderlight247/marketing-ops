-- Milestone 9 (first increment): background scheduler support.
--
-- `next_attempt_at` is when the worker may next claim a job. It is written at
-- enqueue time (coalesce of an explicit schedule and now), and after a failed
-- attempt with the computed backoff delay. A NULL next_attempt_at on a failed
-- job means the job reached its terminal state and only an operator retry
-- (or a re-approved version) can restart it.

ALTER TABLE publishing_jobs
    ADD COLUMN next_attempt_at timestamptz;

-- Legacy enqueued jobs (before this migration) become immediately runnable.
UPDATE publishing_jobs
SET next_attempt_at = coalesce(scheduled_at, created_at)
WHERE next_attempt_at IS NULL;

-- The worker's due scan touches exactly the pending/failed rows; a partial
-- index keeps the claim query cheap and ignores terminal rows.
CREATE INDEX publishing_jobs_due_idx
    ON publishing_jobs (next_attempt_at)
    WHERE status IN ('pending', 'failed');
