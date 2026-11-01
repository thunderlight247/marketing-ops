-- An operator can clear the delivery-problem badge for a workspace once they
-- have dealt with what the log showed. The timestamp is the boundary: only
-- problems recorded after it raise the badge again, so acknowledging can never
-- hide a failure that arrives later.
CREATE TABLE email_delivery_acknowledgements (
    workspace_id uuid PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
    acknowledged_at timestamptz NOT NULL DEFAULT now()
);
