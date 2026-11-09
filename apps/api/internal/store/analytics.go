// Analytics aggregation across workspaces. The dashboard needs one summary,
// not every lead row: these counts are computed in SQL so the response stays
// small as the lead tables grow. CRM leads exclude lead-engine outreach rows;
// engine figures cover those rows only.
package store

import (
	"context"
	"time"
)

// AnalyticsTotals is the platform-wide total for the active workspaces.
type AnalyticsTotals struct {
	Workspaces      int `json:"workspaces"`
	Campaigns       int `json:"campaigns"`
	CRMLeads        int `json:"crmLeads"`
	CRMWon          int `json:"crmWon"`
	EngineLeads     int `json:"engineLeads"`
	EngineSent      int `json:"engineSent"`
	EngineReplied   int `json:"engineReplied"`
	EngineFailed    int `json:"engineFailed"`
	EngineBlacklist int `json:"engineBlacklisted"`
	EmailsTotal     int `json:"emailsTotal"`
	EmailsSent      int `json:"emailsSent"`
	EmailsDelivered int `json:"emailsDelivered"`
}

// WorkspaceAnalytics is one active workspace's share of the summary.
type WorkspaceAnalytics struct {
	WorkspaceID     string `json:"workspaceId"`
	Slug            string `json:"slug"`
	Name            string `json:"name"`
	Campaigns       int    `json:"campaigns"`
	CRMLeads        int    `json:"crmLeads"`
	CRMWon          int    `json:"crmWon"`
	EngineLeads     int    `json:"engineLeads"`
	EngineSent      int    `json:"engineSent"`
	EngineReplied   int    `json:"engineReplied"`
	EmailsTotal     int    `json:"emailsTotal"`
	EmailsSent      int    `json:"emailsSent"`
	EmailsDelivered int    `json:"emailsDelivered"`
}

// AnalyticsSummary is the whole dashboard payload.
type AnalyticsSummary struct {
	GeneratedAt  time.Time            `json:"generatedAt"`
	Totals       AnalyticsTotals      `json:"totals"`
	ByWorkspace  []WorkspaceAnalytics `json:"byWorkspace"`
	CRMStatus    ReportCounts         `json:"crmLeadsByStatus"`
	EngineStatus ReportCounts         `json:"engineLeadsByStatus"`
}

// AnalyticsSummary computes the summary for every active workspace, or for one
// workspace when workspaceID is non-empty. It uses three aggregate queries
// (workspace rows, CRM statuses, engine statuses) rather than fetching lists
// and counting them in the application.
func (s *Store) AnalyticsSummary(ctx context.Context, workspaceID string) (AnalyticsSummary, error) {
	out := AnalyticsSummary{
		GeneratedAt:  time.Now().UTC(),
		ByWorkspace:  []WorkspaceAnalytics{},
		CRMStatus:    ReportCounts{},
		EngineStatus: ReportCounts{},
	}

	rows, err := s.pool.Query(ctx,
		`SELECT w.id, w.slug, w.name,
		        COALESCE(c.n, 0),
		        COALESCE(l.n, 0), COALESCE(l.won, 0),
		        COALESCE(e.n, 0), COALESCE(e.sent, 0), COALESCE(e.replied, 0),
		        COALESCE(d.n, 0), COALESCE(d.sent, 0), COALESCE(d.delivered, 0)
		 FROM workspaces w
		 LEFT JOIN (
		     SELECT workspace_id, count(*) AS n FROM campaigns GROUP BY workspace_id
		 ) c ON c.workspace_id = w.id
		 LEFT JOIN (
		     SELECT workspace_id,
		            count(*) AS n,
		            count(*) FILTER (WHERE status = 'won') AS won
		     FROM leads
		     WHERE source_kind <> 'lead_engine'
		     GROUP BY workspace_id
		 ) l ON l.workspace_id = w.id
		 LEFT JOIN (
		     SELECT workspace_id,
		            count(*) AS n,
		            count(*) FILTER (WHERE status IN ('sent', 'delivered', 'replied', 'demo_sent')) AS sent,
		            count(*) FILTER (WHERE status IN ('replied', 'demo_sent')) AS replied
		     FROM leads
		     WHERE source_kind = 'lead_engine'
		     GROUP BY workspace_id
		 ) e ON e.workspace_id = w.id
		 LEFT JOIN (
		     SELECT workspace_id,
		            count(*) AS n,
		            count(*) FILTER (WHERE status = 'sent') AS sent,
		            count(*) FILTER (WHERE delivery_state = 'delivered') AS delivered
		     FROM email_deliveries
		     GROUP BY workspace_id
		 ) d ON d.workspace_id = w.id
		 WHERE w.status = 'active'
		   AND ($1 = '' OR w.id::text = $1)
		 ORDER BY w.name`, workspaceID)
	if err != nil {
		return AnalyticsSummary{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var w WorkspaceAnalytics
		if err := rows.Scan(
			&w.WorkspaceID, &w.Slug, &w.Name,
			&w.Campaigns,
			&w.CRMLeads, &w.CRMWon,
			&w.EngineLeads, &w.EngineSent, &w.EngineReplied,
			&w.EmailsTotal, &w.EmailsSent, &w.EmailsDelivered,
		); err != nil {
			return AnalyticsSummary{}, err
		}
		out.ByWorkspace = append(out.ByWorkspace, w)
		out.Totals.Workspaces++
		out.Totals.Campaigns += w.Campaigns
		out.Totals.CRMLeads += w.CRMLeads
		out.Totals.CRMWon += w.CRMWon
		out.Totals.EngineLeads += w.EngineLeads
		out.Totals.EngineSent += w.EngineSent
		out.Totals.EngineReplied += w.EngineReplied
		out.Totals.EmailsTotal += w.EmailsTotal
		out.Totals.EmailsSent += w.EmailsSent
		out.Totals.EmailsDelivered += w.EmailsDelivered
	}
	if err := rows.Err(); err != nil {
		return AnalyticsSummary{}, err
	}

	crmRows, err := s.pool.Query(ctx,
		`SELECT status, count(*) FROM leads
		 WHERE source_kind <> 'lead_engine'
		   AND ($1 = '' OR workspace_id::text = $1)
		 GROUP BY status`, workspaceID)
	if err != nil {
		return AnalyticsSummary{}, err
	}
	if err := collectCounts(crmRows, &out.CRMStatus); err != nil {
		return AnalyticsSummary{}, err
	}

	engineRows, err := s.pool.Query(ctx,
		`SELECT status, count(*) FROM leads
		 WHERE source_kind = 'lead_engine'
		   AND ($1 = '' OR workspace_id::text = $1)
		 GROUP BY status`, workspaceID)
	if err != nil {
		return AnalyticsSummary{}, err
	}
	if err := collectCounts(engineRows, &out.EngineStatus); err != nil {
		return AnalyticsSummary{}, err
	}

	out.Totals.EngineFailed = out.EngineStatus["failed"]
	out.Totals.EngineBlacklist = out.EngineStatus["blacklisted"]

	return out, nil
}
