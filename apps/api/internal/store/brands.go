package store

import (
	"context"
	"time"
)

// BrandProfile is the per-workspace brand and business context (M2 UI data).
type BrandProfile struct {
	WorkspaceID         string
	BusinessDescription *string
	Audience            *string
	Services            *string
	Tone                *string
	Rules               map[string]any
	ProhibitedClaims    []string
	CallsToAction       []string
	UpdatedAt           time.Time
}

const brandColumns = `workspace_id, business_description, audience, services, tone,
	rules, prohibited_claims, calls_to_action, updated_at`

// GetBrandProfile returns the brand profile of exactly one workspace, or
// ErrNotFound when the workspace has none.
func (s *Store) GetBrandProfile(ctx context.Context, workspaceID string) (BrandProfile, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT workspace_id, business_description, audience, services, tone,
			rules, prohibited_claims, calls_to_action, updated_at
		 FROM brand_profiles WHERE workspace_id = $1`, workspaceID)
	var b BrandProfile
	if err := row.Scan(&b.WorkspaceID, &b.BusinessDescription, &b.Audience, &b.Services,
		&b.Tone, &b.Rules, &b.ProhibitedClaims, &b.CallsToAction, &b.UpdatedAt); err != nil {
		return BrandProfile{}, mapNotFound(err)
	}
	return b, nil
}

// UpsertBrandProfile creates or replaces the brand profile of one workspace.
func (s *Store) UpsertBrandProfile(ctx context.Context, b BrandProfile) (BrandProfile, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO brand_profiles (workspace_id, business_description, audience, services, tone, rules,
			prohibited_claims, calls_to_action)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (workspace_id) DO UPDATE SET
			business_description = EXCLUDED.business_description,
			audience = EXCLUDED.audience,
			services = EXCLUDED.services,
			tone = EXCLUDED.tone,
			rules = EXCLUDED.rules,
			prohibited_claims = EXCLUDED.prohibited_claims,
			calls_to_action = EXCLUDED.calls_to_action,
			updated_at = now()
		 RETURNING `+brandColumns,
		b.WorkspaceID, b.BusinessDescription, b.Audience, b.Services, b.Tone,
		b.Rules, b.ProhibitedClaims, b.CallsToAction)
	var out BrandProfile
	if err := row.Scan(&out.WorkspaceID, &out.BusinessDescription, &out.Audience, &out.Services,
		&out.Tone, &out.Rules, &out.ProhibitedClaims, &out.CallsToAction, &out.UpdatedAt); err != nil {
		return BrandProfile{}, err
	}
	return out, nil
}
