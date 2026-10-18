package store

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"marketingops/api/internal/database"
	"marketingops/api/internal/migrate"
	"marketingops/api/migrations"
)

// Integration tests run against a real PostgreSQL only when a database URL is
// configured. With none set, everything here is skipped, so unit-only runs
// (CI without a database, `go test` in isolation) still pass.
func TestMain(m *testing.M) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "store integration tests skipped: no DATABASE_URL/TEST_DATABASE_URL")
		os.Exit(m.Run())
	}
	os.Exit(runWithDB(m, dsn))
}

func runWithDB(m *testing.M, dsn string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pool, err := database.NewPool(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "storeIntegrationSetup: could not open the database pool:", err)
		return 1
	}
	defer pool.Close()

	all, err := migrate.Load(migrations.FS, ".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "storeIntegrationSetup: load migrations:", err)
		return 1
	}
	if _, err := migrate.Up(ctx, pool, all, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		fmt.Fprintln(os.Stderr, "storeIntegrationSetup: apply migrations:", err)
		return 1
	}

	testStore = New(pool)
	return m.Run()
}

var testStore *Store

func requireStore(t *testing.T) *Store {
	t.Helper()
	if testStore == nil {
		t.Skip("integration database not configured")
	}
	return testStore
}

func uniqueWorkspace(t *testing.T, s *Store, name string) Workspace {
	t.Helper()
	ctx := context.Background()
	w, err := s.InsertWorkspace(ctx, Workspace{
		Slug: fmt.Sprintf("test-%s-%d", name, time.Now().UnixNano()%1_000_000_000),
		Name: "Test workspace " + name,
	})
	if err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	t.Cleanup(func() {
		_ = s.DeleteWorkspace(ctx, w.ID)
	})
	return w
}

func TestWorkspaceLifecycle(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()

	w, err := s.InsertWorkspace(ctx, Workspace{
		Slug: fmt.Sprintf("lifecycle-%d", time.Now().UnixNano()%1_000_000_000),
		Name: "Lifecycle test",
	})
	if err != nil {
		t.Fatalf("InsertWorkspace: %v", err)
	}
	t.Cleanup(func() { _ = s.DeleteWorkspace(ctx, w.ID) })

	if w.Status != "active" {
		t.Errorf("new workspaces must be active, got %q", w.Status)
	}
	again, err := s.GetWorkspaceByID(ctx, w.ID)
	if err != nil || again.ID != w.ID || again.Slug != w.Slug {
		t.Errorf("GetWorkspaceByID mismatch: %+v err %v", again, err)
	}
	if _, err := s.InsertWorkspace(ctx, Workspace{Slug: w.Slug, Name: "Other"}); err != ErrSlugTaken {
		t.Errorf("duplicate slug must return ErrSlugTaken, got %v", err)
	}
	if _, err := s.GetWorkspaceByID(ctx, "00000000-0000-0000-0000-000000000000"); err != ErrNotFound {
		t.Errorf("missing id must return ErrNotFound, got %v", err)
	}
}

func TestCrossWorkspaceIsolation(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()

	wsA := uniqueWorkspace(t, s, "isolation-a")
	wsB := uniqueWorkspace(t, s, "isolation-b")

	item, err := s.InsertContentItem(ctx, ContentItem{
		WorkspaceID: wsA.ID,
		Kind:        "blog_article",
		Title:       "Workspace A secret brief",
	})
	if err != nil {
		t.Fatalf("InsertContentItem: %v", err)
	}

	// Workspace B sees none of A's content.
	if items, err := s.ListContentItems(ctx, wsB.ID); err != nil || len(items) != 0 {
		t.Errorf("workspace B must see none of workspace A's items: items=%d err=%v", len(items), err)
	}
	// Even with a valid item id, B cannot read A's row.
	if _, err := s.GetContentItemInWorkspace(ctx, wsB.ID, item.ID); err != ErrNotFound {
		t.Errorf("cross-workspace read must be ErrNotFound, got %v", err)
	}
	// A reads its own row.
	if got, err := s.GetContentItemInWorkspace(ctx, wsA.ID, item.ID); err != nil || got.ID != item.ID {
		t.Errorf("owner workspace read failed: id=%s err=%v", got.ID, err)
	}
	if item.Status != "draft" {
		t.Errorf("new content must be draft, got %q", item.Status)
	}

	// Deleting workspace A takes its content with it (cascade).
	if err := s.DeleteWorkspace(ctx, wsA.ID); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}
	if _, err := s.ListContentItems(ctx, wsA.ID); err != nil {
		t.Errorf("listing a deleted workspace must be empty, not an error: %v", err)
	}
	if _, err := s.GetContentItemInWorkspace(ctx, wsA.ID, item.ID); err != ErrNotFound {
		t.Errorf("row must be gone after workspace deletion, got %v", err)
	}
}
