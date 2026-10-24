// Lead storage (Milestone 11): enquiries captured manually, through forms,
// or via authorized imports. Contact fields are all optional (data
// minimization). source_kind is required so a lead either has a traceable
// origin or is explicitly marked 'unknown'. UTM columns are nullable:
// attribution is stored only where real tracking data exists, never inferred.
package store

import (
	"context"
	"strings"
	"time"
)

// Lead is one enquiry inside one workspace.
type Lead struct {
	ID           string
	WorkspaceID  string
	CampaignID   *string
	SourceKind   string
	SourceDetail string
	Status       string
	ContactName  *string
	ContactEmail *string
	ContactPhone *string
	Message      *string
	Notes        string
	UtmSource    *string
	UtmMedium    *string
	UtmCampaign  *string
	FollowUpAt   *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

const leadColumns = `id, workspace_id, campaign_id, source_kind, source_detail, status,
	contact_name, contact_email, contact_phone, message, notes,
	utm_source, utm_medium, utm_campaign, follow_up_at, created_at, updated_at`

func leadValidSource(kind string) bool {
	switch kind {
	case "manual", "form_api", "import", "unknown":
		return true
	}
	return false
}

func leadValidStatus(status string) bool {
	switch status {
	case "new", "contacted", "qualified", "won", "lost":
		return true
	}
	return false
}

// InsertLead creates one lead in a workspace. A non-empty campaign must exist
// inside the same workspace.
func (s *Store) InsertLead(ctx context.Context, l Lead) (Lead, error) {
	l.SourceKind = strings.TrimSpace(l.SourceKind)
	if l.SourceKind == "" {
		l.SourceKind = "unknown"
	}
	if !leadValidSource(l.SourceKind) {
		return Lead{}, ErrInvalidStatus
	}
	if l.Status == "" {
		l.Status = "new"
	}
	if !leadValidStatus(l.Status) {
		return Lead{}, ErrInvalidStatus
	}
	if err := s.leadCampaignScopeOK(ctx, l.WorkspaceID, l.CampaignID); err != nil {
		return Lead{}, err
	}
	out, err := scanLead(s.pool.QueryRow(ctx,
		`INSERT INTO leads (workspace_id, campaign_id, source_kind, source_detail, status,
			contact_name, contact_email, contact_phone, message, notes,
			utm_source, utm_medium, utm_campaign, follow_up_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		 RETURNING `+leadColumns,
		l.WorkspaceID, l.CampaignID, l.SourceKind, l.SourceDetail, l.Status,
		l.ContactName, l.ContactEmail, l.ContactPhone, l.Message, l.Notes,
		l.UtmSource, l.UtmMedium, l.UtmCampaign, l.FollowUpAt))
	return out, mapNotFound(err)
}

// ListLeads returns the leads of exactly one workspace, most urgent first:
// following-up leads before others, then newest activity first.
func (s *Store) ListLeads(ctx context.Context, workspaceID string) ([]Lead, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+leadColumns+` FROM leads
		 WHERE workspace_id = $1
		 ORDER BY (follow_up_at IS NOT NULL AND status NOT IN ('won','lost')) DESC,
			follow_up_at ASC NULLS LAST, updated_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Lead{}
	for rows.Next() {
		l, err := scanLead(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// GetLeadInWorkspace reads one lead only if both match.
func (s *Store) GetLeadInWorkspace(ctx context.Context, workspaceID, id string) (Lead, error) {
	l, err := scanLead(s.pool.QueryRow(ctx,
		`SELECT `+leadColumns+` FROM leads
		 WHERE id = $1 AND workspace_id = $2`, id, workspaceID))
	return l, mapNotFound(err)
}

// LeadUpdate narrows what PATCH edits on a lead; nil pointers mean "leave
// unchanged". The source and created_at history are never rewritable.
type LeadUpdate struct {
	Status        string // '' keeps current
	ContactName   *string
	ContactEmail  *string
	ContactPhone  *string
	Message       *string
	Notes         string
	KeepNotes     bool
	UtmSource     *string
	UtmMedium     *string
	UtmCampaign   *string
	FollowUpAt    *time.Time
	ClearFollow   bool
	CampaignID    *string
	ClearCampaign bool
}

// UpdateLead applies a LeadUpdate to one lead bounded to its workspace.
// source_kind/source_detail are immutable after creation (traceability).
func (s *Store) UpdateLead(ctx context.Context, workspaceID, leadID string, in LeadUpdate) (Lead, error) {
	if in.Status != "" && !leadValidStatus(in.Status) {
		return Lead{}, ErrInvalidStatus
	}
	if err := s.leadCampaignScopeOK(ctx, workspaceID, in.CampaignID); err != nil {
		return Lead{}, err
	}
	notes := in.Notes
	if in.KeepNotes {
		notes = ""
	}
	out, err := scanLead(s.pool.QueryRow(ctx,
		`UPDATE leads SET
			campaign_id = CASE
				WHEN $3::bool THEN NULL
				WHEN $4::text IS NOT NULL AND $4::text <> '' THEN $4::uuid
				ELSE campaign_id END,
			status = CASE WHEN $5 = '' THEN status ELSE $5 END,
			contact_name  = COALESCE($6, contact_name),
			contact_email = COALESCE($7, contact_email),
			contact_phone = COALESCE($8, contact_phone),
			message       = COALESCE($9, message),
			notes = CASE WHEN $10::bool THEN notes ELSE $11 END,
			utm_source    = COALESCE($12, utm_source),
			utm_medium    = COALESCE($13, utm_medium),
			utm_campaign  = COALESCE($14, utm_campaign),
			follow_up_at  = CASE WHEN $15::bool THEN NULL
				WHEN $16::timestamptz IS NOT NULL THEN $16::timestamptz
				ELSE follow_up_at END,
			updated_at = now()
		 WHERE id = $2 AND workspace_id = $1
		 RETURNING `+leadColumns,
		workspaceID, leadID,
		in.ClearCampaign, in.CampaignID,
		in.Status,
		in.ContactName, in.ContactEmail, in.ContactPhone, in.Message,
		in.KeepNotes, notes,
		in.UtmSource, in.UtmMedium, in.UtmCampaign,
		in.ClearFollow, in.FollowUpAt))
	return out, mapNotFound(err)
}

// leadCampaignScopeOK rejects campaign references that point outside the
// workspace (workspace isolation) or do not exist.
func (s *Store) leadCampaignScopeOK(ctx context.Context, workspaceID string, campaignID *string) error {
	if campaignID == nil || *campaignID == "" {
		return nil
	}
	if _, err := s.GetCampaignInWorkspace(ctx, workspaceID, *campaignID); err != nil {
		return ErrNotFound
	}
	return nil
}

func scanLead(row interface{ Scan(dest ...any) error }) (Lead, error) {
	var l Lead
	if err := row.Scan(&l.ID, &l.WorkspaceID, &l.CampaignID, &l.SourceKind, &l.SourceDetail,
		&l.Status, &l.ContactName, &l.ContactEmail, &l.ContactPhone, &l.Message, &l.Notes,
		&l.UtmSource, &l.UtmMedium, &l.UtmCampaign, &l.FollowUpAt, &l.CreatedAt, &l.UpdatedAt); err != nil {
		return Lead{}, err
	}
	return l, nil
}
