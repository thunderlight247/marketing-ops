// Report aggregation (Milestone 12, first increment). One call returns the
// raw counts a workspace report is built from. Everything is scoped to the
// workspace in SQL, and every number has an identifiable source record:
//
//   - content/campaign/lead counts are the CURRENT stored state (point in
//     time; they describe rows that still exist, not history).
//   - publishing counts are filtered by the job's created_at falling inside
//     [since, until), so a range report reflects work attempted in that range.
//
// Nothing outside the database is mixed in: analytics/social metrics that no
// verified integration supplies simply do not appear, so a missing
// integration can never look like a genuine zero.
package store

import (
	"context"
	"time"
)

// ReportRange bounds the in-range publishing figures. Both ends are optional
// (nil = unbounded); until must be after since when both are set.
type ReportRange struct {
	Since *time.Time
	Until *time.Time
}

// ReportCounts is a generic label->count map by status/source.
// Keys are stored values; unknown future statuses still surface.
type ReportCounts map[string]int

// PublishingFailure is one failed publishing job in the range, shown with its
// stored safe diagnostic (never raw provider/credental material).
type PublishingFailure struct {
	JobID         string    `json:"jobId"`
	ItemID        string    `json:"itemId"`
	IntegrationID string    `json:"integrationId"`
	Attempts      int       `json:"attempts"`
	LastError     string    `json:"lastError"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// WorkspaceReport is the complete report payload of one workspace.
type WorkspaceReport struct {
	WorkspaceID string `json:"workspaceId"`

	// Current state (whole table, not range-filtered).
	ContentStatus ReportCounts `json:"contentByStatus"`
	CampaignsBy   ReportCounts `json:"campaignsByStatus"`
	LeadsByStatus ReportCounts `json:"leadsByStatus"`
	LeadsBySource ReportCounts `json:"leadsBySource"`

	// Publishing activity restricted to [since, until).
	PublishingJobs ReportCounts        `json:"publishingJobsByStatus"`
	PublishedCount int                 `json:"publishedJobs"`
	FailedCount    int                 `json:"failedJobs"`
	RecentFailures []PublishingFailure `json:"recentFailures"`

	GeneratedAt time.Time `json:"generatedAt"`
}

// WorkspaceReport aggregates the report payload for one workspace. The range
// applies only to publishing jobs (see type comment on ReportRange).
func (s *Store) WorkspaceReport(ctx context.Context, workspaceID string, rg ReportRange) (WorkspaceReport, error) {
	r := WorkspaceReport{WorkspaceID: workspaceID, GeneratedAt: time.Now().UTC()}

	// Content items: current status distribution.
	itemRows, err := s.pool.Query(ctx,
		`SELECT status, count(*) FROM content_items
		 WHERE workspace_id = $1 GROUP BY status`, workspaceID)
	if err != nil {
		return WorkspaceReport{}, err
	}
	r.ContentStatus = ReportCounts{}
	if err := collectCounts(itemRows, &r.ContentStatus); err != nil {
		return WorkspaceReport{}, err
	}

	// Campaigns: current status distribution.
	campRows, err := s.pool.Query(ctx,
		`SELECT status, count(*) FROM campaigns
		 WHERE workspace_id = $1 GROUP BY status`, workspaceID)
	if err != nil {
		return WorkspaceReport{}, err
	}
	r.CampaignsBy = ReportCounts{}
	if err := collectCounts(campRows, &r.CampaignsBy); err != nil {
		return WorkspaceReport{}, err
	}

	// Leads: by status and by source (data-minimizing fields only).
	leadRows, err := s.pool.Query(ctx,
		`SELECT status, count(*) FROM leads
		 WHERE workspace_id = $1 GROUP BY status`, workspaceID)
	if err != nil {
		return WorkspaceReport{}, err
	}
	r.LeadsByStatus = ReportCounts{}
	if err := collectCounts(leadRows, &r.LeadsByStatus); err != nil {
		return WorkspaceReport{}, err
	}
	srcRows, err := s.pool.Query(ctx,
		`SELECT source_kind, count(*) FROM leads
		 WHERE workspace_id = $1 GROUP BY source_kind`, workspaceID)
	if err != nil {
		return WorkspaceReport{}, err
	}
	r.LeadsBySource = ReportCounts{}
	if err := collectCounts(srcRows, &r.LeadsBySource); err != nil {
		return WorkspaceReport{}, err
	}

	// Publishing jobs inside [since, until): jobs whose work was attempted
	// (created) in the range. Both bounds may be nil.
	jobRows, err := s.pool.Query(ctx,
		`SELECT pj.status, count(*) FROM publishing_jobs pj
		 JOIN content_items ci ON ci.id = pj.content_item_id
		 WHERE ci.workspace_id = $1
			AND ($2::timestamptz IS NULL OR pj.created_at >= $2)
			AND ($3::timestamptz IS NULL OR pj.created_at < $3)
		 GROUP BY pj.status`, workspaceID, rg.Since, rg.Until)
	if err != nil {
		return WorkspaceReport{}, err
	}
	r.PublishingJobs = ReportCounts{}
	if err := collectCounts(jobRows, &r.PublishingJobs); err != nil {
		return WorkspaceReport{}, err
	}
	r.PublishedCount = r.PublishingJobs["succeeded"]
	r.FailedCount = r.PublishingJobs["failed"]

	// The five most recent failed jobs in the range, for diagnosability.
	failRows, err := s.pool.Query(ctx,
		`SELECT pj.id, pj.content_item_id, pj.integration_id, pj.attempts,
			coalesce(pj.last_error, ''), pj.updated_at
		 FROM publishing_jobs pj
		 JOIN content_items ci ON ci.id = pj.content_item_id
		 WHERE ci.workspace_id = $1 AND pj.status = 'failed'
			AND ($2::timestamptz IS NULL OR pj.created_at >= $2)
			AND ($3::timestamptz IS NULL OR pj.created_at < $3)
		 ORDER BY pj.updated_at DESC LIMIT 5`, workspaceID, rg.Since, rg.Until)
	if err != nil {
		return WorkspaceReport{}, err
	}
	r.RecentFailures = []PublishingFailure{}
	for failRows.Next() {
		var f PublishingFailure
		if err := failRows.Scan(&f.JobID, &f.ItemID, &f.IntegrationID, &f.Attempts, &f.LastError, &f.UpdatedAt); err != nil {
			failRows.Close()
			return WorkspaceReport{}, err
		}
		r.RecentFailures = append(r.RecentFailures, f)
	}
	if err := failRows.Err(); err != nil {
		return WorkspaceReport{}, err
	}

	return r, nil
}

// collectCounts drains a (label, count) result set into the map.
func collectCounts(rows interface {
	Scan(dest ...any) error
	Next() bool
	Err() error
	Close()
}, m *ReportCounts) error {
	defer rows.Close()
	for rows.Next() {
		var label string
		var n int
		if err := rows.Scan(&label, &n); err != nil {
			return err
		}
		(*m)[label] = n
	}
	return rows.Err()
}
