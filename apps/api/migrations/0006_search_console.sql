-- Milestone 10 (first increment): Google Search Console integration.
--
-- The adapter_kind CHECK from 0002 listed only website content destinations.
-- Search Console is an authorized analytics data source (read-only), so it
-- joins the same integrations table with its own kind. The original inline
-- CHECK constraint was unnamed, so the name is resolved dynamically.

DO $$
DECLARE
    constraint_name text;
BEGIN
    SELECT conname INTO constraint_name
    FROM pg_constraint
    WHERE conrelid = 'integrations'::regclass
      AND contype = 'c'
      AND pg_get_constraintdef(oid) LIKE '%adapter_kind%'
    LIMIT 1;
    IF constraint_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE integrations DROP CONSTRAINT %I', constraint_name);
    END IF;
END $$;

ALTER TABLE integrations
    ADD CONSTRAINT integrations_adapter_kind_check
    CHECK (adapter_kind IN
        ('sanity', 'git', 'custom_api', 'manual', 'search_console'));
