-- Analytics query optimisation (Phase 4).
--
-- The AnalyticsSummary query joins leads and email_deliveries with GROUP BY
-- workspace_id. Without a covering index those sub-selects require full table
-- scans as the row counts grow. The indexes below let the planner use index
-- scans + hash-aggregate instead, cutting both query time and I/O.

-- Partial index: CRM leads count + won-filter used by the analytics LEFT JOIN.
-- Excludes lead_engine rows so the WHERE in the sub-select matches exactly.
CREATE INDEX IF NOT EXISTS leads_analytics_crm_idx
    ON leads (workspace_id, status)
    WHERE source_kind <> 'lead_engine';

-- Partial index: lead engine rows used by the engine sub-select.
CREATE INDEX IF NOT EXISTS leads_analytics_engine_idx
    ON leads (workspace_id, status)
    WHERE source_kind = 'lead_engine';

-- Covering index: email_deliveries analytics sub-select needs workspace_id,
-- status, and delivery_state for the three COUNT(*) FILTER columns.
CREATE INDEX IF NOT EXISTS email_deliveries_analytics_idx
    ON email_deliveries (workspace_id, status, delivery_state);
