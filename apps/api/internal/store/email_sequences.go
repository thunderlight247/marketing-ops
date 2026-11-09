// Email drip sequence store. Sequences are owned by a workspace and contain
// an ordered list of steps (delay_days, subject, body) stored as JSONB.
// This keeps the schema simple while giving the UI full editing flexibility.
package store

import (
	"context"
	"encoding/json"
	"time"
)

// EmailSequenceStep is one step in a drip sequence.
type EmailSequenceStep struct {
	DelayDays int    `json:"delayDays"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
}

// EmailSequence is a named multi-step drip sequence belonging to a workspace.
type EmailSequence struct {
	ID          string              `json:"id"`
	WorkspaceID string              `json:"workspaceId"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Steps       []EmailSequenceStep `json:"steps"`
	CreatedAt   time.Time           `json:"createdAt"`
	UpdatedAt   time.Time           `json:"updatedAt"`
}

// ListEmailSequences returns all sequences for a workspace, newest first.
func (s *Store) ListEmailSequences(ctx context.Context, workspaceID string) ([]EmailSequence, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, workspace_id, name, description, steps, created_at, updated_at
		   FROM email_sequences
		  WHERE workspace_id = $1
		  ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EmailSequence
	for rows.Next() {
		var seq EmailSequence
		var stepsJSON []byte
		if err := rows.Scan(&seq.ID, &seq.WorkspaceID, &seq.Name, &seq.Description,
			&stepsJSON, &seq.CreatedAt, &seq.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(stepsJSON, &seq.Steps); err != nil {
			seq.Steps = []EmailSequenceStep{}
		}
		out = append(out, seq)
	}
	if out == nil {
		out = []EmailSequence{}
	}
	return out, rows.Err()
}

// CreateEmailSequence inserts a new sequence and returns it with its generated ID.
func (s *Store) CreateEmailSequence(ctx context.Context, in EmailSequence) (EmailSequence, error) {
	stepsJSON, err := json.Marshal(in.Steps)
	if err != nil {
		return EmailSequence{}, err
	}
	var out EmailSequence
	var stepsOut []byte
	err = s.pool.QueryRow(ctx,
		`INSERT INTO email_sequences (workspace_id, name, description, steps)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, workspace_id, name, description, steps, created_at, updated_at`,
		in.WorkspaceID, in.Name, in.Description, stepsJSON,
	).Scan(&out.ID, &out.WorkspaceID, &out.Name, &out.Description,
		&stepsOut, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		return EmailSequence{}, err
	}
	if err := json.Unmarshal(stepsOut, &out.Steps); err != nil {
		out.Steps = []EmailSequenceStep{}
	}
	return out, nil
}

// UpdateEmailSequence updates an existing sequence's name, description, and steps.
// It scopes the update to the workspace to prevent cross-workspace edits.
func (s *Store) UpdateEmailSequence(ctx context.Context, in EmailSequence) (EmailSequence, error) {
	stepsJSON, err := json.Marshal(in.Steps)
	if err != nil {
		return EmailSequence{}, err
	}
	var out EmailSequence
	var stepsOut []byte
	err = s.pool.QueryRow(ctx,
		`UPDATE email_sequences
		    SET name = $3, description = $4, steps = $5, updated_at = now()
		  WHERE id = $1 AND workspace_id = $2
		  RETURNING id, workspace_id, name, description, steps, created_at, updated_at`,
		in.ID, in.WorkspaceID, in.Name, in.Description, stepsJSON,
	).Scan(&out.ID, &out.WorkspaceID, &out.Name, &out.Description,
		&stepsOut, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		return EmailSequence{}, err
	}
	if err := json.Unmarshal(stepsOut, &out.Steps); err != nil {
		out.Steps = []EmailSequenceStep{}
	}
	return out, nil
}

// DeleteEmailSequence removes a sequence, scoped to its workspace.
func (s *Store) DeleteEmailSequence(ctx context.Context, workspaceID, id string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM email_sequences WHERE id = $1 AND workspace_id = $2`, id, workspaceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
