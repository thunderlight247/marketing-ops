package store

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrSubmissionConflict = errors.New("submission ID already used for a different payload")

type LeadIntakeConnection struct {
	ID, WorkspaceID, Name string
	SecretHash            []byte
	CreatedAt             time.Time
}

func (s *Store) CreateLeadIntakeConnection(ctx context.Context, workspaceID, name string, hash []byte) (LeadIntakeConnection, error) {
	var c LeadIntakeConnection
	err := s.pool.QueryRow(ctx, `INSERT INTO lead_intake_connections (workspace_id, name, secret_hash)
		VALUES ($1,$2,$3) ON CONFLICT (workspace_id, name) DO NOTHING
		RETURNING id, workspace_id, name, created_at`, workspaceID, name, hash).Scan(&c.ID, &c.WorkspaceID, &c.Name, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrSlugTaken
	}
	return c, err
}

func (s *Store) ListLeadIntakeConnections(ctx context.Context, workspaceID string) ([]LeadIntakeConnection, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, workspace_id, name, created_at FROM lead_intake_connections WHERE workspace_id = $1 ORDER BY created_at`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LeadIntakeConnection{}
	for rows.Next() {
		var c LeadIntakeConnection
		if err := rows.Scan(&c.ID, &c.WorkspaceID, &c.Name, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetLeadIntakeConnection(ctx context.Context, workspaceID, id string) (LeadIntakeConnection, error) {
	var c LeadIntakeConnection
	err := s.pool.QueryRow(ctx, `SELECT id, workspace_id, name, secret_hash, created_at FROM lead_intake_connections WHERE workspace_id=$1 AND id=$2`, workspaceID, id).Scan(&c.ID, &c.WorkspaceID, &c.Name, &c.SecretHash, &c.CreatedAt)
	return c, mapNotFound(err)
}

func (s *Store) DeleteLeadIntakeConnection(ctx context.Context, workspaceID, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM lead_intake_connections WHERE workspace_id=$1 AND id=$2`, workspaceID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RecordLeadIntake serializes each (connection, submission ID), so concurrent
// retries return the same receipt and never create extra leads.
func (s *Store) RecordLeadIntake(ctx context.Context, c LeadIntakeConnection, submissionID string, payloadHash []byte, l Lead) (string, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// A row lock also makes deletion/revocation wait for an in-flight submission.
	var workspaceID, name string
	var currentHash []byte
	err = tx.QueryRow(ctx, `SELECT workspace_id, name, secret_hash FROM lead_intake_connections WHERE id=$1 AND workspace_id=$2 FOR UPDATE`, c.ID, c.WorkspaceID).Scan(&workspaceID, &name, &currentHash)
	if err != nil {
		return "", false, mapNotFound(err)
	}
	if !bytes.Equal(currentHash, c.SecretHash) {
		return "", false, ErrNotFound
	}
	var existingID string
	var existingHash []byte
	err = tx.QueryRow(ctx, `SELECT lead_id, payload_hash FROM lead_intake_receipts WHERE connection_id=$1 AND submission_id=$2`, c.ID, submissionID).Scan(&existingID, &existingHash)
	if err == nil {
		if !bytes.Equal(existingHash, payloadHash) {
			return "", false, ErrSubmissionConflict
		}
		if err = tx.Commit(ctx); err != nil {
			return "", false, err
		}
		return existingID, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, err
	}
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO leads (workspace_id, source_kind, source_detail, contact_name, contact_email, contact_phone, message, utm_source, utm_medium, utm_campaign)
		VALUES ($1,'form_api',$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`, workspaceID, name, l.ContactName, l.ContactEmail, l.ContactPhone, l.Message, l.UtmSource, l.UtmMedium, l.UtmCampaign).Scan(&id)
	if err != nil {
		return "", false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO lead_intake_receipts (connection_id, submission_id, payload_hash, lead_id) VALUES ($1,$2,$3,$4)`, c.ID, submissionID, payloadHash, id); err != nil {
		return "", false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", false, err
	}
	return id, false, nil
}
