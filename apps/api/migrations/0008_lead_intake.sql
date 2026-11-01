-- A connection authenticates a single website server; only a hash is stored.
CREATE TABLE lead_intake_connections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name text NOT NULL,
    secret_hash bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(workspace_id, name)
);

-- Receipts bind retry keys to their exact payload and resulting lead.
CREATE TABLE lead_intake_receipts (
    connection_id uuid NOT NULL REFERENCES lead_intake_connections(id) ON DELETE CASCADE,
    submission_id text NOT NULL,
    payload_hash bytea NOT NULL,
    lead_id uuid NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    PRIMARY KEY (connection_id, submission_id)
);
