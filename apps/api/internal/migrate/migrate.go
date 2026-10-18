// Package migrate is a small forward-only SQL migration runner.
//
// Files are named NNNN_description.sql (four digits, starting at 0001, no
// gaps). Applied versions are recorded in the schema_migrations table, and each
// migration runs in its own transaction while holding a PostgreSQL advisory
// lock so two processes cannot migrate at once.
package migrate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// advisoryLockKey is an arbitrary constant shared by all migrate processes.
const advisoryLockKey int64 = 7260510001

var filenamePattern = regexp.MustCompile(`^(\d{4})_([a-z0-9_]+)\.sql$`)

// Migration is one SQL file.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// StatusRow reports whether a migration has been applied.
type StatusRow struct {
	Version   int
	Name      string
	Applied   bool
	AppliedAt *time.Time
}

// Load reads and validates migrations from dir inside fsys. Non-.sql files are
// ignored. It performs no database access.
func Load(fsys fs.FS, dir string) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}

	var migrations []Migration
	seen := make(map[int]string)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}
		match := filenamePattern.FindStringSubmatch(name)
		if match == nil {
			return nil, fmt.Errorf("migration file %q must be named NNNN_description.sql", name)
		}
		version, _ := strconv.Atoi(match[1])
		if prev, dup := seen[version]; dup {
			return nil, fmt.Errorf("duplicate migration version %04d: %q and %q", version, prev, name)
		}
		seen[version] = name

		body, err := fs.ReadFile(fsys, path.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", name, err)
		}
		if strings.TrimSpace(string(body)) == "" {
			return nil, fmt.Errorf("migration file %q is empty", name)
		}
		migrations = append(migrations, Migration{Version: version, Name: match[2], SQL: string(body)})
	}

	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	for i, m := range migrations {
		if want := i + 1; m.Version != want {
			return nil, fmt.Errorf("migration versions must be contiguous from 0001: expected %04d, found %04d", want, m.Version)
		}
	}
	return migrations, nil
}

// Up applies every pending migration in order and returns how many ran.
func Up(ctx context.Context, pool *pgxpool.Pool, migrations []Migration, logger *slog.Logger) (int, error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return 0, errors.New("could not connect to the database (check DATABASE_URL and that PostgreSQL is running)")
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", advisoryLockKey); err != nil {
		return 0, fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		// Use a fresh context so the lock is released even if ctx was cancelled.
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(unlockCtx, "SELECT pg_advisory_unlock($1)", advisoryLockKey)
	}()

	const createTable = `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    integer PRIMARY KEY,
		name       text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`
	if _, err := conn.Exec(ctx, createTable); err != nil {
		return 0, fmt.Errorf("create schema_migrations: %w", err)
	}

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, m := range migrations {
		if _, done := applied[m.Version]; done {
			continue
		}
		if err := applyOne(ctx, conn, m); err != nil {
			return count, err
		}
		count++
		logger.Info("migration applied", "version", m.Version, "name", m.Name)
	}
	return count, nil
}

func applyOne(ctx context.Context, conn *pgxpool.Conn, m Migration) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration %04d_%s: %w", m.Version, m.Name, err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after a successful commit

	// With no arguments pgx uses the simple protocol, so a file may contain
	// several statements.
	if _, err := tx.Exec(ctx, m.SQL); err != nil {
		return fmt.Errorf("migration %04d_%s failed: %w", m.Version, m.Name, err)
	}
	if _, err := tx.Exec(ctx,
		"INSERT INTO schema_migrations (version, name) VALUES ($1, $2)", m.Version, m.Name); err != nil {
		return fmt.Errorf("record migration %04d_%s: %w", m.Version, m.Name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %04d_%s: %w", m.Version, m.Name, err)
	}
	return nil
}

func appliedVersions(ctx context.Context, conn *pgxpool.Conn) (map[int]time.Time, error) {
	rows, err := conn.Query(ctx, "SELECT version, applied_at FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]time.Time)
	for rows.Next() {
		var version int
		var at time.Time
		if err := rows.Scan(&version, &at); err != nil {
			return nil, fmt.Errorf("scan schema_migrations: %w", err)
		}
		applied[version] = at
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	return applied, nil
}

// Status lists every known migration and whether it has been applied. It is
// read-only and works before schema_migrations exists.
func Status(ctx context.Context, pool *pgxpool.Pool, migrations []Migration) ([]StatusRow, error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, errors.New("could not connect to the database (check DATABASE_URL and that PostgreSQL is running)")
	}
	defer conn.Release()

	var exists bool
	if err := conn.QueryRow(ctx, "SELECT to_regclass('schema_migrations') IS NOT NULL").Scan(&exists); err != nil {
		return nil, fmt.Errorf("check schema_migrations: %w", err)
	}

	applied := map[int]time.Time{}
	if exists {
		if applied, err = appliedVersions(ctx, conn); err != nil {
			return nil, err
		}
	}

	rows := make([]StatusRow, 0, len(migrations))
	for _, m := range migrations {
		row := StatusRow{Version: m.Version, Name: m.Name}
		if at, ok := applied[m.Version]; ok {
			at := at
			row.Applied, row.AppliedAt = true, &at
		}
		rows = append(rows, row)
	}
	return rows, nil
}
