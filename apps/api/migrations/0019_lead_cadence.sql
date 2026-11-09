-- Milestone: Multi-Touch WhatsApp Follow-up Cadence & Sequence Automation.
--
-- Enables tracking which touch in a multi-step sequence each lead has received,
-- scheduling automated follow-ups for prospects who have not replied, and
-- storing custom follow-up templates and delay intervals per workspace.

ALTER TABLE leads
    ADD COLUMN IF NOT EXISTS sequence_step integer NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS next_followup_at timestamptz;

-- Add partial index for efficient retrieval of due follow-ups
CREATE INDEX IF NOT EXISTS leads_due_followup_idx
    ON leads (workspace_id, next_followup_at ASC)
    WHERE next_followup_at IS NOT NULL
      AND status IN ('sent', 'delivered');

-- Store follow-up sequence templates and interval settings in workspace_lead_configs
ALTER TABLE workspace_lead_configs
    ADD COLUMN IF NOT EXISTS enable_auto_followup boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS followup_1_template text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS followup_1_delay_days integer NOT NULL DEFAULT 3,
    ADD COLUMN IF NOT EXISTS followup_2_template text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS followup_2_delay_days integer NOT NULL DEFAULT 4;
