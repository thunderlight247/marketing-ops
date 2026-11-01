-- A bounced reply is finished: the recipient's server accepted the message and
-- then refused it, so it will never reach the person. It stays in the log as
-- `sent` + `bounced` (that is what happened), but it must not block the fresh
-- attempt an operator is allowed to send.
--
-- The "at most one live reply per lead" invariant therefore excludes a bounced
-- accepted message. Everything else is unchanged: a queued, sending, delivered
-- or otherwise accepted reply still prevents a second copy from going out.
DROP INDEX email_deliveries_lead_reply_live;

CREATE UNIQUE INDEX email_deliveries_lead_reply_live
    ON email_deliveries (lead_id)
    WHERE kind = 'lead_reply'
      AND (
        status IN ('queued', 'sending')
        OR (status = 'sent' AND delivery_state <> 'bounced')
      );
