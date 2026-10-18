-- Milestone 1: demo data.
--
-- Clearly-labelled demo rows so the dashboard can show something on a fresh
-- database. No real client names, no credentials, no production anywhere.
-- The operator can delete this workspace like any other.

INSERT INTO operators (name, email)
VALUES ('Operator', 'operator@local.example')
ON CONFLICT (email) DO NOTHING;

INSERT INTO workspaces (slug, name, website_url)
VALUES
  ('demo-studio', 'Demo Studio', 'https://demo-studio.example'),
  ('demo-cafe', 'Demo Cafe', 'https://demo-cafe.example')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO brand_profiles (workspace_id, business_description, audience, tone)
SELECT s.id, name || ': sample business for development.', 'Sample audience', 'Friendly, factual'
FROM workspaces s
WHERE s.slug IN ('demo-studio', 'demo-cafe')
ON CONFLICT (workspace_id) DO NOTHING;
