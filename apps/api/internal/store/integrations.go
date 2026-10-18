package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Integration is one connected website/CMS/social source of a workspace.
// Config holds only non-secret display values.
type Integration struct {
	ID          string
	WorkspaceID string
	AdapterKind string
	Name        string
	Status      string
	Config      map[string]any
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

const integrationColumns = `id, workspace_id, adapter_kind, name, status, config, created_at, updated_at`

// ValidCapability and ValidCapabilityStatus mirror the CHECK constraints in
// migration 0002 so the store rejects bad values before the database does.
var (
	validCapabilities = map[string]bool{
		"read_content": true, "create_draft": true, "publish": true,
		"update_content": true, "schedule": true, "analytics": true,
		"lead_intake": true,
	}
	validCapabilityStatus = map[string]bool{
		"supported": true, "unsupported": true, "unverified": true, "failed": true,
	}
)

// InsertIntegration creates one integration inside a workspace.
func (s *Store) InsertIntegration(ctx context.Context, in Integration) (Integration, error) {
	if in.Config == nil {
		in.Config = map[string]any{}
	}
	if in.Status == "" {
		in.Status = "unconfigured" // the lifecycle default from migration 0002
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO integrations (workspace_id, adapter_kind, name, status, config)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (workspace_id, name) DO NOTHING
		 RETURNING `+integrationColumns,
		in.WorkspaceID, in.AdapterKind, in.Name, in.Status, in.Config)
	out, err := scanIntegration(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Integration{}, ErrSlugTaken
	}
	return out, err
}

// ListIntegrations returns the integrations of exactly one workspace.
func (s *Store) ListIntegrations(ctx context.Context, workspaceID string) ([]Integration, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+integrationColumns+` FROM integrations
		 WHERE workspace_id = $1 ORDER BY created_at`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Integration{}
	for rows.Next() {
		in, err := scanIntegration(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	return out, rows.Err()
}

// GetIntegrationInWorkspace reads one integration only if both match.
func (s *Store) GetIntegrationInWorkspace(ctx context.Context, workspaceID, id string) (Integration, error) {
	in, err := scanIntegration(s.pool.QueryRow(ctx,
		`SELECT `+integrationColumns+` FROM integrations
		 WHERE id = $1 AND workspace_id = $2`, id, workspaceID))
	return in, mapNotFound(err)
}

// SetIntegrationStatus updates the lifecycle status of one integration,
// bounded to its workspace.
func (s *Store) SetIntegrationStatus(ctx context.Context, workspaceID, id, status string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE integrations SET status = $3, updated_at = now()
		 WHERE id = $1 AND workspace_id = $2`, id, workspaceID, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteIntegration removes one integration, bounded to its workspace.
func (s *Store) DeleteIntegration(ctx context.Context, workspaceID, id string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM integrations WHERE id = $1 AND workspace_id = $2`, id, workspaceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanIntegration(row interface{ Scan(dest ...any) error }) (Integration, error) {
	var i Integration
	if err := row.Scan(&i.ID, &i.WorkspaceID, &i.AdapterKind, &i.Name, &i.Status,
		&i.Config, &i.CreatedAt, &i.UpdatedAt); err != nil {
		return Integration{}, err
	}
	return i, nil
}

// ---- Capabilities -----------------------------------------------------------

// IntegrationCapability records whether an integration can really do one thing.
type IntegrationCapability struct {
	IntegrationID string
	Capability    string
	Status        string
	Details       *string
	VerifiedAt    *time.Time
}

// UpsertCapability records or replaces a capability entry of an integration.
// Write capabilities must only be marked 'supported' after a real adapter check.
func (s *Store) UpsertCapability(ctx context.Context, integrationID string, c IntegrationCapability) error {
	if !validCapabilities[c.Capability] {
		return ErrInvalidCapability
	}
	if !validCapabilityStatus[c.Status] {
		return ErrInvalidCapability
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO integration_capabilities (integration_id, capability, status, details, verified_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (integration_id, capability) DO UPDATE SET
			status = EXCLUDED.status, details = EXCLUDED.details,
			verified_at = EXCLUDED.verified_at`,
		integrationID, c.Capability, c.Status, c.Details, c.VerifiedAt)
	return err
}

// ListCapabilities returns the capability rows of one integration.
func (s *Store) ListCapabilities(ctx context.Context, integrationID string) ([]IntegrationCapability, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT integration_id, capability, status, details, verified_at
		 FROM integration_capabilities WHERE integration_id = $1
		 ORDER BY capability`, integrationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []IntegrationCapability{}
	for rows.Next() {
		var c IntegrationCapability
		if err := rows.Scan(&c.IntegrationID, &c.Capability, &c.Status, &c.Details, &c.VerifiedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---- Credential sets ----------------------------------------------------------

// CredentialSet is the encrypted-at-rest secret data of one integration. It
// stores ciphertext (nonce included) plus the key version used, never plaintext.
type CredentialSet struct {
	IntegrationID string
	KeyVersion    int
	Ciphertext    []byte
	UpdatedAt     time.Time
}

// PutCredentialSet creates or replaces the credential set of an integration.
func (s *Store) PutCredentialSet(ctx context.Context, c CredentialSet) error {
	tag, err := s.pool.Exec(ctx,
		`INSERT INTO integration_credential_sets (integration_id, key_version, ciphertext)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (integration_id) DO UPDATE SET
			key_version = EXCLUDED.key_version, ciphertext = EXCLUDED.ciphertext,
			updated_at = now()`,
		c.IntegrationID, c.KeyVersion, c.Ciphertext)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound // integration does not exist (FK target missing)
	}
	return nil
}

// GetCredentialSet returns the credential set of one integration, if any.
func (s *Store) GetCredentialSet(ctx context.Context, integrationID string) (CredentialSet, error) {
	var c CredentialSet
	err := s.pool.QueryRow(ctx,
		`SELECT integration_id, key_version, ciphertext, updated_at
		 FROM integration_credential_sets WHERE integration_id = $1`, integrationID,
	).Scan(&c.IntegrationID, &c.KeyVersion, &c.Ciphertext, &c.UpdatedAt)
	if err != nil {
		return CredentialSet{}, mapNotFound(err)
	}
	return c, nil
}

// DeleteCredentialSet removes the credential set of an integration.
func (s *Store) DeleteCredentialSet(ctx context.Context, integrationID string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM integration_credential_sets WHERE integration_id = $1`, integrationID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
