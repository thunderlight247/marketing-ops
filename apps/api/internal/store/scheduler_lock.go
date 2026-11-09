package store

import (
	"context"
	"time"
)

// schedulerLockKey is distinct from the migration lock key. It keeps the
// lead-engine scheduler on one instance when several API processes share a
// database.
const schedulerLockKey int64 = 7260510002

// TryAcquireSchedulerLock takes a session-level PostgreSQL advisory lock without
// waiting. ok is false when another process holds it, and the caller should
// skip its pass. The returned release func must be called once the pass ends;
// it unlocks and returns the connection. The lock is tied to that connection,
// so it is also freed if this process dies.
func (s *Store) TryAcquireSchedulerLock(ctx context.Context) (release func(), ok bool, err error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, false, err
	}
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", schedulerLockKey).Scan(&ok); err != nil {
		conn.Release()
		return nil, false, err
	}
	if !ok {
		conn.Release()
		return nil, false, nil
	}
	return func() {
		// Use a fresh context so the unlock still runs if ctx was cancelled.
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(unlockCtx, "SELECT pg_advisory_unlock($1)", schedulerLockKey)
		conn.Release()
	}, true, nil
}
