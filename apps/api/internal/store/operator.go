package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Operator represents a team member with role-based access.
type Operator struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`   // 'admin', 'editor', 'viewer'
	Status    string    `json:"status"` // 'active', 'inactive'
	CreatedAt time.Time `json:"createdAt"`
}

// ListOperators returns all team members ordered by creation date.
func (s *Store) ListOperators(ctx context.Context) ([]Operator, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, email, role, status, created_at
		 FROM operators
		 ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Operator
	for rows.Next() {
		var o Operator
		if err := rows.Scan(&o.ID, &o.Name, &o.Email, &o.Role, &o.Status, &o.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, o)
	}
	return list, rows.Err()
}

// CreateOperator inserts a new team member with an assigned role.
func (s *Store) CreateOperator(ctx context.Context, name, email, role string) (Operator, error) {
	if role == "" {
		role = "editor"
	}
	var o Operator
	err := s.pool.QueryRow(ctx,
		`INSERT INTO operators (name, email, role, status)
		 VALUES ($1, $2, $3, 'active')
		 RETURNING id, name, email, role, status, created_at`,
		name, email, role,
	).Scan(&o.ID, &o.Name, &o.Email, &o.Role, &o.Status, &o.CreatedAt)
	return o, err
}

// UpdateOperator updates an operator's role or active status.
func (s *Store) UpdateOperator(ctx context.Context, id, role, status string) (Operator, error) {
	var o Operator
	err := s.pool.QueryRow(ctx,
		`UPDATE operators
		 SET role = CASE WHEN $2 != '' THEN $2 ELSE role END,
		     status = CASE WHEN $3 != '' THEN $3 ELSE status END
		 WHERE id = $1
		 RETURNING id, name, email, role, status, created_at`,
		id, role, status,
	).Scan(&o.ID, &o.Name, &o.Email, &o.Role, &o.Status, &o.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Operator{}, ErrNotFound
	}
	return o, err
}

// EnsureOperator resolves the acting operator in single-operator mode
// (decision D16: authentication deferred). It returns the first operator row,
// creating the standard demo operator when none exists yet, so approval
// decisions always carry a real operator_id for FK integrity. When
// authentication arrives (Milestone 13), callers pass the session identity
// through instead of this lookup.
func (s *Store) EnsureOperator(ctx context.Context) (string, error) {
	var id string
	if err := s.pool.QueryRow(ctx,
		`SELECT id FROM operators ORDER BY created_at LIMIT 1`,
	).Scan(&id); err == nil {
		return id, nil
	}
	// No operator row: insert the standard one. An insert race with another
	// request resolves into the follow-up select below.
	if _, insertErr := s.pool.Exec(ctx,
		`INSERT INTO operators (name, email) VALUES ('Operator', 'operator@local.example')`,
	); insertErr != nil && !isUniqueViolation(insertErr) {
		return "", insertErr
	}
	if err := s.pool.QueryRow(ctx,
		`SELECT id FROM operators ORDER BY created_at LIMIT 1`,
	).Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

// isUniqueViolation reports a PostgreSQL uniqueness conflict (23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505"
}
