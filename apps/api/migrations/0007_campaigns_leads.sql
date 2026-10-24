-- Milestone 11 (first increment): lead/enquiry tracking and campaigns.
--
-- Campaigns group content and enquiries under the operator's existing
-- workspace isolation. A unique (workspace_id, name) pair prevents
-- look-alike duplicates without leaking anything cross-workspace.
--
-- Leads carry only optional contact fields (data minimization): whatever
-- the operator actually has. source_kind is NOT NULL so a lead either has a
-- traceable origin (manual, form_api, import) or is explicitly marked
-- 'unknown' - an unset source can never masquerade as a real one. UTM
-- columns are nullable: attribution is recorded only where the operator
-- actually has tracking data, never inferred.
CREATE TABLE campaigns (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    name         text NOT NULL,
    status       text NOT NULL DEFAULT 'planned'
                 CHECK (status IN ('planned', 'active', 'paused', 'completed')),
    utm_campaign text NOT NULL DEFAULT '',
    notes        text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, name)
);

CREATE TABLE leads (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id  uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    campaign_id   uuid REFERENCES campaigns (id) ON DELETE SET NULL,
    source_kind   text NOT NULL
                  CHECK (source_kind IN ('manual', 'form_api', 'import', 'unknown')),
    source_detail text NOT NULL DEFAULT '',
    status        text NOT NULL DEFAULT 'new'
                  CHECK (status IN ('new', 'contacted', 'qualified', 'won', 'lost')),
    contact_name  text,
    contact_email text,
    contact_phone text,
    message       text,
    notes         text NOT NULL DEFAULT '',
    utm_source    text,
    utm_medium    text,
    utm_campaign  text,
    follow_up_at  timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX leads_workspace_idx ON leads (workspace_id, updated_at DESC);
CREATE INDEX leads_campaign_idx ON leads (campaign_id);
CREATE INDEX campaigns_workspace_idx ON campaigns (workspace_id);
