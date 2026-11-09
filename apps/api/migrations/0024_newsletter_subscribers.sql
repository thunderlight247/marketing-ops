-- Newsletter subscriber list, one row per workspace and address.
--
-- An address on the email_suppressions list (for example after an unsubscribe)
-- is never added here, so a subscriber who opted out cannot be re-added by
-- accident through the signup form.
CREATE TABLE IF NOT EXISTS newsletter_subscribers (
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    email        TEXT NOT NULL CHECK (email = lower(email) AND email <> ''),
    name         TEXT NOT NULL DEFAULT '',
    source       TEXT NOT NULL DEFAULT 'manual',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, email)
);
