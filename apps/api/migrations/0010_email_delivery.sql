-- Per-workspace outbound email. Each workspace connects its own provider
-- account; the API key is stored only as ciphertext under the platform
-- credential key, exactly like integration credentials.
CREATE TABLE workspace_email_settings (
    workspace_id uuid PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
    provider text NOT NULL DEFAULT 'resend',
    from_name text NOT NULL DEFAULT '',
    from_email text NOT NULL,
    notify_email text NOT NULL DEFAULT '',
    api_key_ciphertext bytea NOT NULL,
    api_key_version integer NOT NULL,
    -- 'unverified' until a real test message is accepted by the provider;
    -- 'error' when a send was rejected for credential reasons.
    status text NOT NULL DEFAULT 'unverified'
        CHECK (status IN ('unverified', 'connected', 'error')),
    last_verified_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- One outbound message. Rows are written before any network call so a send is
-- always traceable, and the worker records what the provider answered.
CREATE TABLE email_deliveries (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('lead_notification', 'lead_reply')),
    lead_id uuid REFERENCES leads(id) ON DELETE SET NULL,
    to_email text NOT NULL,
    subject text NOT NULL,
    body text NOT NULL,
    status text NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'sending', 'sent', 'failed')),
    attempts integer NOT NULL DEFAULT 0,
    -- NULL means the delivery is terminal and waits for an operator.
    next_attempt_at timestamptz DEFAULT now(),
    last_error text NOT NULL DEFAULT '',
    provider_message_id text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    sent_at timestamptz
);

CREATE INDEX email_deliveries_due_idx ON email_deliveries (status, next_attempt_at);
CREATE INDEX email_deliveries_workspace_idx ON email_deliveries (workspace_id, created_at DESC);

-- One alert per lead, ever: a duplicated form submission or an intake retry
-- cannot notify the team twice.
CREATE UNIQUE INDEX email_deliveries_lead_notification_once
    ON email_deliveries (workspace_id, lead_id)
    WHERE kind = 'lead_notification';

-- At most one live reply per lead, so a double click cannot email a person
-- twice. A failed attempt can still be retried as a fresh delivery.
CREATE UNIQUE INDEX email_deliveries_lead_reply_live
    ON email_deliveries (lead_id)
    WHERE kind = 'lead_reply' AND status IN ('queued', 'sending', 'sent');

-- Set only when the provider accepted the reply: the platform records the
-- contact it actually made, and does not guess at the lead's outcome.
ALTER TABLE leads ADD COLUMN reply_sent_at timestamptz;
