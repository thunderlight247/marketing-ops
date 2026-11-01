-- Provider delivery webhooks. The signing secret is per workspace (each
-- client has its own provider account) and is stored encrypted like the API
-- key. NULL means no webhook is connected yet.
ALTER TABLE workspace_email_settings ADD COLUMN webhook_secret_ciphertext bytea;

-- What the provider later reported about a message. `status` stays the send
-- lifecycle (queued/sending/sent/failed); delivery_state is a separate fact so
-- "accepted by the provider" and "actually delivered" never get conflated.
ALTER TABLE email_deliveries
    ADD COLUMN delivery_state text NOT NULL DEFAULT 'unknown'
        CHECK (delivery_state IN ('unknown', 'delivered', 'delayed', 'bounced', 'complained')),
    ADD COLUMN delivery_state_at timestamptz,
    ADD COLUMN delivery_detail text NOT NULL DEFAULT '';

-- Provider callbacks are deduplicated on the provider's own event id, so a
-- retried delivery of the same webhook cannot move a row twice.
CREATE TABLE email_provider_events (
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    provider_event_id text NOT NULL,
    delivery_id uuid REFERENCES email_deliveries(id) ON DELETE CASCADE,
    event_type text NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, provider_event_id)
);
