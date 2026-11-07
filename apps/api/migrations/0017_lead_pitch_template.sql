-- Workspace-authored outreach message. When set, the lead engine sends this
-- exact text per lead (with {{placeholders}} merged in) instead of generating
-- a pitch with the AI provider.
ALTER TABLE workspace_lead_configs
    ADD COLUMN pitch_template text NOT NULL DEFAULT '';
