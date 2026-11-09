-- Email suppression list (outreach compliance).
--
-- An address listed here for a workspace must never receive another email from
-- that workspace. EnqueueEmailDelivery checks this table in the same statement
-- that records the message, so a suppressed address is never queued at all.
CREATE TABLE IF NOT EXISTS email_suppressions (
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    email        TEXT NOT NULL CHECK (email = lower(email) AND email <> ''),
    reason       TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, email)
);
