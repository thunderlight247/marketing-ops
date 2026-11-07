-- One contact = one lead row per workspace, ported from gh-lead-engine.
--
-- The legacy engine declared `contacted_businesses.phone_number TEXT UNIQUE`
-- and normalised every number to 233XXXXXXXXX before inserting, so the same
-- business discovered again under a different Google place_id simply could not
-- enter the queue a second time (INSERT OR IGNORE). This is the durable half of
-- duplicate suppression: the row itself is the "already contacted" marker.
--
-- phone_key is the Ghana national significant number (the last 9 digits), which
-- is identical for "+233 54 263 5494", "0542635494" and "233542635494".
ALTER TABLE leads
    ADD COLUMN IF NOT EXISTS phone_key text
    GENERATED ALWAYS AS (
        RIGHT(REGEXP_REPLACE(COALESCE(contact_phone, ''), '\D', '', 'g'), 9)
    ) STORED;

-- Enforce one lead-engine row per phone inside a workspace. Leads from other
-- sources (manual, import, form_api) keep their own duplicate rules.
CREATE UNIQUE INDEX IF NOT EXISTS leads_workspace_phone_key_uniq
    ON leads (workspace_id, phone_key)
    WHERE source_kind = 'lead_engine' AND phone_key <> '';

-- The duplicate check joins outreach logs back to leads by phone; keep that
-- lookup cheap as the log grows.
CREATE INDEX IF NOT EXISTS lead_outreach_logs_workspace_direction_idx
    ON lead_outreach_logs (workspace_id, direction, created_at DESC);
