-- A drafted first reply for one existing lead, produced by a specialist agent
-- and stored for operator review. It is never sent by the platform, and its
-- presence never changes the lead's status: only a human can record a contact.
ALTER TABLE leads
    ADD COLUMN reply_draft_subject text,
    ADD COLUMN reply_draft_body text,
    ADD COLUMN reply_drafted_at timestamptz;

ALTER TABLE leads ADD CONSTRAINT leads_reply_draft_pair CHECK (
    (reply_draft_subject IS NULL AND reply_draft_body IS NULL AND reply_drafted_at IS NULL)
    OR (reply_draft_subject IS NOT NULL AND reply_draft_body IS NOT NULL AND reply_drafted_at IS NOT NULL)
);
