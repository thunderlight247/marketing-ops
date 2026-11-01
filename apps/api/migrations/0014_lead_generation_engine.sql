-- Milestone: Lead Generation & WhatsApp Outreach Engine (gh-lead-engine integration).
--
-- Each workspace can configure its own Google Places API key, AI provider key,
-- business offering, target categories, target locations, and WhatsApp outreach
-- settings with complete workspace isolation.

CREATE TABLE workspace_lead_configs (
    workspace_id uuid PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
    enabled boolean NOT NULL DEFAULT false,
    google_places_api_key_ciphertext bytea,
    google_places_api_key_version integer NOT NULL DEFAULT 1,
    ai_provider_key_ciphertext bytea,
    ai_provider_key_version integer NOT NULL DEFAULT 1,
    ai_model text NOT NULL DEFAULT 'meta-llama/llama-3.3-70b-instruct:free',
    sender_name text NOT NULL DEFAULT '',
    business_offering text NOT NULL DEFAULT '',
    target_audience text NOT NULL DEFAULT '',
    portfolio_url text NOT NULL DEFAULT '',
    target_categories text[] NOT NULL DEFAULT '{}',
    target_cities text[] NOT NULL DEFAULT '{}',
    daily_limit integer NOT NULL DEFAULT 20,
    min_delay_ms integer NOT NULL DEFAULT 25000,
    max_delay_ms integer NOT NULL DEFAULT 75000,
    whatsapp_status text NOT NULL DEFAULT 'disconnected'
        CHECK (whatsapp_status IN ('disconnected', 'qr_ready', 'connected')),
    whatsapp_phone text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Upgrade leads table to support local business discovery, rating/review criteria,
-- and automated outreach statuses.
ALTER TABLE leads
    ADD COLUMN IF NOT EXISTS place_id text,
    ADD COLUMN IF NOT EXISTS category text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS city text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS address text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS rating real,
    ADD COLUMN IF NOT EXISTS review_count integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS website_url text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS pitch text NOT NULL DEFAULT '';

-- Update source_kind constraint to include 'lead_engine'.
ALTER TABLE leads DROP CONSTRAINT IF EXISTS leads_source_kind_check;
ALTER TABLE leads ADD CONSTRAINT leads_source_kind_check
    CHECK (source_kind IN ('manual', 'form_api', 'import', 'lead_engine', 'unknown'));

-- Update status constraint to include lead engine outreach states.
ALTER TABLE leads DROP CONSTRAINT IF EXISTS leads_status_check;
ALTER TABLE leads ADD CONSTRAINT leads_status_check
    CHECK (status IN (
        'new', 'contacted', 'qualified', 'won', 'lost',
        'discovered', 'pending', 'processing', 'sent', 'delivered', 'replied', 'demo_sent', 'failed', 'blacklisted'
    ));

-- Ensure one lead per Google Place ID within a workspace.
CREATE UNIQUE INDEX IF NOT EXISTS leads_workspace_place_id_idx
    ON leads (workspace_id, place_id)
    WHERE place_id IS NOT NULL;

-- Outreach logs record every outbound and inbound WhatsApp interaction with delivery acks.
CREATE TABLE lead_outreach_logs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    lead_id uuid NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    direction text NOT NULL CHECK (direction IN ('outbound', 'inbound')),
    message_id text NOT NULL DEFAULT '',
    body text NOT NULL,
    status text NOT NULL,
    ack integer,
    error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX lead_outreach_logs_lead_idx ON lead_outreach_logs (lead_id, created_at DESC);
CREATE INDEX lead_outreach_logs_workspace_idx ON lead_outreach_logs (workspace_id, created_at DESC);

-- Phone blacklist per workspace for DND / opt-outs.
CREATE TABLE lead_blacklist (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    phone_number text NOT NULL,
    reason text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, phone_number)
);

-- Daily Google Places search accounting to respect API budget and prevent redundant queries.
CREATE TABLE lead_discovery_searches (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    day_key text NOT NULL,
    category text NOT NULL,
    city text NOT NULL,
    result_count integer NOT NULL DEFAULT 0,
    error text NOT NULL DEFAULT '',
    searched_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, day_key, category, city)
);

CREATE INDEX lead_discovery_searches_day_idx ON lead_discovery_searches (workspace_id, day_key);
