-- Email drip sequences. Each workspace can have multiple named sequences;
-- each sequence is an ordered list of steps stored in JSONB for simplicity.
-- Steps contain: delay_days (int), subject (text), body (text).
-- A future worker can consume these; for now they are created/edited in the UI.

CREATE TABLE IF NOT EXISTS email_sequences (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid        NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name         text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    description  text        NOT NULL DEFAULT '',
    steps        jsonb       NOT NULL DEFAULT '[]',
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS email_sequences_workspace_id_idx ON email_sequences (workspace_id);
