-- Milestone 0 baseline.
--
-- This deliberately small table exists to prove the migration pipeline end to
-- end. Business tables (workspaces, integrations, content, ...) are introduced
-- in Milestone 1, not here.
CREATE TABLE app_metadata (
    key        text PRIMARY KEY,
    value      text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO app_metadata (key, value) VALUES ('schema_baseline', 'milestone-0');
