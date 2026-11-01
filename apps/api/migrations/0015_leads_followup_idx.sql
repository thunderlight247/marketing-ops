-- Performance: index for the follow_up_at ordering used by ListLeads.
-- The existing leads_workspace_idx covers (workspace_id, updated_at DESC)
-- but the urgent-first sort also orders by follow_up_at. A partial index
-- on non-terminal leads with a follow-up date avoids a sequential sort.
CREATE INDEX IF NOT EXISTS leads_followup_idx
    ON leads (workspace_id, follow_up_at ASC NULLS LAST)
    WHERE follow_up_at IS NOT NULL
      AND status NOT IN ('won', 'lost', 'blacklisted');
