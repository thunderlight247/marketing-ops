package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

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
