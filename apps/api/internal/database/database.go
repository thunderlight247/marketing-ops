// Package database creates the PostgreSQL connection pool.
package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool builds a pool for databaseURL. It does not open a connection, so the
// API can start while PostgreSQL is down and report that state via /health.
//
// Errors are deliberately generic: driver errors can echo connection details.
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, errors.New("DATABASE_URL could not be parsed by the PostgreSQL driver")
	}
	cfg.MaxConns = 5
	cfg.ConnConfig.ConnectTimeout = 3 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("could not create the database connection pool")
	}
	return pool, nil
}
