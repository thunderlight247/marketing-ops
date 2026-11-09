// Date-ranged analytics: a daily lead trend and UTM source/medium/campaign
// breakdowns. Both are computed in SQL over the leads table, scoped to one
// workspace or to all active workspaces, and limited to [from, to).
package store

import (
	"context"
	"time"
)

// DailyLeadPoint is one day of new leads in the requested range.
type DailyLeadPoint struct {
	Day    string `json:"day"`
	CRM    int    `json:"crm"`
	Engine int    `json:"engine"`
}

// UTMBreakdown lists lead counts per UTM value; empty values are "(none)".
type UTMBreakdown struct {
	Source   ReportCounts `json:"source"`
	Medium   ReportCounts `json:"medium"`
	Campaign ReportCounts `json:"campaign"`
}

// AnalyticsBreakdown is the date-ranged payload.
type AnalyticsBreakdown struct {
	From  time.Time        `json:"from"`
	To    time.Time        `json:"to"`
	Trend []DailyLeadPoint `json:"trend"`
	UTM   UTMBreakdown     `json:"utm"`
}

// AnalyticsBreakdown returns the trend and UTM counts for leads created in
// [from, to). An empty workspaceID covers all workspaces.
func (s *Store) AnalyticsBreakdown(ctx context.Context, workspaceID string, from, to time.Time) (AnalyticsBreakdown, error) {
	out := AnalyticsBreakdown{From: from, To: to, Trend: []DailyLeadPoint{}}
	out.UTM = UTMBreakdown{Source: ReportCounts{}, Medium: ReportCounts{}, Campaign: ReportCounts{}}

	rows, err := s.pool.Query(ctx,
		`SELECT to_char(date_trunc('day', created_at), 'YYYY-MM-DD'),
		        count(*) FILTER (WHERE source_kind <> 'lead_engine'),
		        count(*) FILTER (WHERE source_kind = 'lead_engine')
		 FROM leads
		 WHERE created_at >= $2 AND created_at < $3
		   AND ($1 = '' OR workspace_id::text = $1)
		 GROUP BY 1 ORDER BY 1`, workspaceID, from, to)
	if err != nil {
		return AnalyticsBreakdown{}, err
	}
	for rows.Next() {
		var p DailyLeadPoint
		if err := rows.Scan(&p.Day, &p.CRM, &p.Engine); err != nil {
			rows.Close()
			return AnalyticsBreakdown{}, err
		}
		out.Trend = append(out.Trend, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return AnalyticsBreakdown{}, err
	}

	// Column names come from this fixed list, never from user input.
	for _, col := range []struct {
		name string
		dst  *ReportCounts
	}{
		{"utm_source", &out.UTM.Source},
		{"utm_medium", &out.UTM.Medium},
		{"utm_campaign", &out.UTM.Campaign},
	} {
		utmRows, err := s.pool.Query(ctx,
			`SELECT coalesce(nullif(`+col.name+`, ''), '(none)'), count(*)
			 FROM leads
			 WHERE created_at >= $2 AND created_at < $3
			   AND ($1 = '' OR workspace_id::text = $1)
			 GROUP BY 1`, workspaceID, from, to)
		if err != nil {
			return AnalyticsBreakdown{}, err
		}
		if err := collectCounts(utmRows, col.dst); err != nil {
			return AnalyticsBreakdown{}, err
		}
	}
	return out, nil
}
