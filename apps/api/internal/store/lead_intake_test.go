package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"testing"
)

func TestLeadIntakeIsolationAndConcurrentRetries(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()
	a := uniqueWorkspace(t, s, "intake-a")
	b := uniqueWorkspace(t, s, "intake-b")
	secretHash := sha256.Sum256([]byte("secret"))
	created, err := s.CreateLeadIntakeConnection(ctx, a.ID, "contact form", secretHash[:])
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.GetLeadIntakeConnection(ctx, b.ID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-workspace lookup: %v", err)
	}
	if err = s.DeleteLeadIntakeConnection(ctx, b.ID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-workspace delete: %v", err)
	}
	c, err := s.GetLeadIntakeConnection(ctx, a.ID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	payload := sha256.Sum256([]byte("payload"))
	email := "person@example.com"
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, _, err := s.RecordLeadIntake(ctx, c, "same-id", payload[:], Lead{ContactEmail: &email})
			ids <- id
			errs <- err
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	first := ""
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatalf("duplicate lead: %s != %s", id, first)
		}
	}
	leads, err := s.ListLeads(ctx, a.ID)
	if err != nil || len(leads) != 1 || leads[0].SourceKind != "form_api" || leads[0].SourceDetail != "contact form" {
		t.Fatalf("leads=%+v err=%v", leads, err)
	}
	changed := sha256.Sum256([]byte("other"))
	if _, _, err = s.RecordLeadIntake(ctx, c, "same-id", changed[:], Lead{ContactEmail: &email}); !errors.Is(err, ErrSubmissionConflict) {
		t.Fatalf("conflicting payload: %v", err)
	}
	if err = s.DeleteLeadIntakeConnection(ctx, a.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.RecordLeadIntake(ctx, c, "new-id", payload[:], Lead{ContactEmail: &email}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked connection: %v", err)
	}
}
