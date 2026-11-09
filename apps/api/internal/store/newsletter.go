package store

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrSubscriberEmailRequired = errors.New("subscriber email is required")

// NewsletterSubscriber is one address on a workspace's newsletter list.
type NewsletterSubscriber struct {
	WorkspaceID, Email, Name, Source string
	CreatedAt                        time.Time
}

// AddNewsletterSubscriber adds an address to the list. It returns false when
// the address is already on the list or is suppressed, so nothing is written.
func (s *Store) AddNewsletterSubscriber(ctx context.Context, workspaceID, email, name, source string) (bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false, ErrSubscriberEmailRequired
	}
	tag, err := s.pool.Exec(ctx,
		`INSERT INTO newsletter_subscribers (workspace_id, email, name, source)
		 SELECT $1, $2, $3, $4
		 WHERE NOT EXISTS (
		   SELECT 1 FROM email_suppressions WHERE workspace_id = $1 AND email = $2
		 )
		 ON CONFLICT (workspace_id, email) DO NOTHING`,
		workspaceID, email, strings.TrimSpace(name), source)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// ListNewsletterSubscribers returns the workspace's subscribers, newest first.
func (s *Store) ListNewsletterSubscribers(ctx context.Context, workspaceID string) ([]NewsletterSubscriber, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT workspace_id, email, name, source, created_at
		 FROM newsletter_subscribers WHERE workspace_id = $1
		 ORDER BY created_at DESC, email`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NewsletterSubscriber{}
	for rows.Next() {
		var n NewsletterSubscriber
		if err := rows.Scan(&n.WorkspaceID, &n.Email, &n.Name, &n.Source, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// DeleteNewsletterSubscriber removes an address from the workspace's list.
func (s *Store) DeleteNewsletterSubscriber(ctx context.Context, workspaceID, email string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM newsletter_subscribers WHERE workspace_id = $1 AND email = $2`,
		workspaceID, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
