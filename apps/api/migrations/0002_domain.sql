-- Milestone 1: core domain model.
--
-- Every client-owned row is scoped to a workspace, enforced by a foreign key
-- on workspace_id below. Status/type fields are text with CHECK constraints
-- so new values are a migration, not an accident in application code.
-- Timestamps are timestamptz; ids are uuid.
--
-- Full lifecycle columns for publishing jobs, versioning, and audit rows are
-- introduced now (Milestone 1) even though the workflow code arrives in
-- Milestones 4-5, so that later migrations do not churn these tables.

CREATE TABLE operators (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text NOT NULL,
    email      text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE workspaces (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        text NOT NULL UNIQUE,
    name        text NOT NULL,
    website_url text,
    status      text NOT NULL DEFAULT 'active'
                CHECK (status IN ('active', 'archived')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- One brand profile per workspace (1:1), per the Milestone 2 client profile.
CREATE TABLE brand_profiles (
    workspace_id         uuid PRIMARY KEY REFERENCES workspaces (id) ON DELETE CASCADE,
    business_description text,
    audience             text,
    services             text,
    tone                 text,
    rules                jsonb NOT NULL DEFAULT '{}',
    prohibited_claims    text[] NOT NULL DEFAULT '{}',
    calls_to_action      text[] NOT NULL DEFAULT '{}',
    updated_at           timestamptz NOT NULL DEFAULT now()
);

-- A connected website/CMS/social integration for a workspace. `config` holds
-- only non-secret display values. Secrets live separately (see below).
CREATE TABLE integrations (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    adapter_kind text NOT NULL
                 CHECK (adapter_kind IN ('sanity', 'git', 'custom_api', 'manual')),
    name         text NOT NULL,
    status       text NOT NULL DEFAULT 'unconfigured'
                 CHECK (status IN ('unconfigured', 'connected', 'limited', 'error', 'disconnected')),
    config       jsonb NOT NULL DEFAULT '{}',
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, name)
);

CREATE INDEX integrations_workspace_idx ON integrations (workspace_id);

-- What an integration can actually do, verified by the adapter. Never trust
-- a row whose status is 'unverified' for a write capability.
CREATE TABLE integration_capabilities (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    integration_id uuid NOT NULL REFERENCES integrations (id) ON DELETE CASCADE,
    capability     text NOT NULL CHECK (capability IN
                   ('read_content', 'create_draft', 'publish', 'update_content',
                    'schedule', 'analytics', 'lead_intake')),
    status         text NOT NULL CHECK (status IN
                   ('supported', 'unsupported', 'unverified', 'failed')),
    details        text,
    verified_at    timestamptz,
    UNIQUE (integration_id, capability)
);

CREATE INDEX integration_capabilities_integration_idx ON integration_capabilities (integration_id);

-- Secrets: AES-GCM ciphertext (includes the nonce), never the key.
-- key_version lets the operator rotate without rewriting the raw secret.
CREATE TABLE integration_credential_sets (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    integration_id uuid NOT NULL UNIQUE REFERENCES integrations (id) ON DELETE CASCADE,
    key_version    integer NOT NULL,
    ciphertext     bytea NOT NULL,
    updated_at     timestamptz NOT NULL DEFAULT now(),
    CHECK (key_version >= 1)
);

CREATE TABLE content_items (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id   uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    kind           text NOT NULL
                   CHECK (kind IN ('blog_article', 'social_post', 'snippet')),
    status         text NOT NULL DEFAULT 'draft'
                   CHECK (status IN ('draft', 'review', 'approved', 'rejected',
                                     'queued', 'publishing', 'published', 'failed')),
    title          text NOT NULL,
    slug           text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, slug)
);

CREATE INDEX content_items_workspace_idx ON content_items (workspace_id);
CREATE INDEX content_items_status_idx ON content_items (status);

-- Immutable content snapshots. The published version must exist here and be
-- approved before any publishing job may run.
CREATE TABLE content_versions (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    content_item_id uuid NOT NULL REFERENCES content_items (id) ON DELETE CASCADE,
    version_number  integer NOT NULL,
    title           text NOT NULL,
    excerpt         text,
    body            text NOT NULL,
    metadata        jsonb NOT NULL DEFAULT '{}',
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (content_item_id, version_number),
    CHECK (version_number >= 1)
);

CREATE INDEX content_versions_item_idx ON content_versions (content_item_id);

CREATE TABLE approvals (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    content_item_id    uuid NOT NULL REFERENCES content_items (id) ON DELETE CASCADE,
    content_version_id uuid NOT NULL UNIQUE REFERENCES content_versions (id) ON DELETE CASCADE,
    decision           text NOT NULL CHECK (decision IN ('approved', 'rejected')),
    operator_id        uuid NOT NULL REFERENCES operators (id),
    note               text,
    decided_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX approvals_item_idx ON approvals (content_item_id);

-- A publishing attempt for one approved version. Idempotency keys make safe
-- retries possible in Milestone 9.
CREATE TABLE publishing_jobs (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    content_item_id      uuid NOT NULL REFERENCES content_items (id) ON DELETE CASCADE,
    content_version_id   uuid NOT NULL REFERENCES content_versions (id) ON DELETE CASCADE,
    integration_id       uuid NOT NULL REFERENCES integrations (id) ON DELETE CASCADE,
    idempotency_key      text NOT NULL UNIQUE,
    status               text NOT NULL DEFAULT 'pending'
                         CHECK (status IN ('pending', 'running', 'succeeded',
                                           'failed', 'cancelled')),
    scheduled_at         timestamptz,
    attempts             integer NOT NULL DEFAULT 0,
    last_error           text,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX publishing_jobs_item_idx ON publishing_jobs (content_item_id);
CREATE INDEX publishing_jobs_interface_idx ON publishing_jobs (integration_id);

CREATE TABLE published_artifacts (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    publishing_job_id uuid NOT NULL UNIQUE REFERENCES publishing_jobs (id) ON DELETE CASCADE,
    integration_id uuid NOT NULL REFERENCES integrations (id),
    external_id    text,
    external_url   text,
    published_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX published_artifacts_integration_idx ON published_artifacts (integration_id);

-- Audit trail. details must hold only redacted/secret-free data.
CREATE TABLE audit_events (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid REFERENCES workspaces (id) ON DELETE SET NULL,
    action      text NOT NULL,
    entity_type text NOT NULL,
    entity_id   uuid,
    details     jsonb NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_events_workspace_idx ON audit_events (workspace_id);
CREATE INDEX audit_events_created_idx ON audit_events (created_at);
