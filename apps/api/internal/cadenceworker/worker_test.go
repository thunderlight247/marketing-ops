package cadenceworker_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"marketingops/api/internal/cadenceworker"
	"marketingops/api/internal/events"
	"marketingops/api/internal/store"
)

type fakeOverdueStore struct {
	leads []store.OverdueLead
	err   error
	calls int
}

func (f *fakeOverdueStore) ListOverdueFollowUpLeads(_ context.Context, _ time.Time, _ int) ([]store.OverdueLead, error) {
	f.calls++
	return f.leads, f.err
}

func TestCadenceWorkerPublishesEvents(t *testing.T) {
	now := time.Now().UTC()
	wsID := "ws-aaa"
	name := "Alice"
	leads := []store.OverdueLead{
		{ID: "l-1", WorkspaceID: wsID, ContactName: &name, Status: "contacted", FollowUpAt: now.Add(-2 * time.Hour)},
		{ID: "l-2", WorkspaceID: wsID, ContactName: nil, Status: "new", FollowUpAt: now.Add(-1 * time.Hour)},
	}
	fake := &fakeOverdueStore{leads: leads}
	hub := events.NewHub()

	ch, unsub := hub.Subscribe(wsID)
	defer unsub()

	w := cadenceworker.New(cadenceworker.Options{
		Store:    fake,
		Hub:      hub,
		Interval: time.Hour, // prevent ticker from firing
		Clock:    func() time.Time { return now },
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Run once manually
	go w.Run(ctx)

	// Should receive one event for workspace wsID
	select {
	case ev := <-ch:
		if ev.Type != "lead_followup_overdue" {
			t.Fatalf("expected lead_followup_overdue event, got %s", ev.Type)
		}
		var payload struct {
			Count int `json:"count"`
		}
		if err := json.Unmarshal([]byte(ev.Message), &payload); err != nil {
			t.Fatalf("bad payload: %v", err)
		}
		if payload.Count != 2 {
			t.Errorf("expected count=2, got %d", payload.Count)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for cadence event")
	}
}

func TestCadenceWorkerNoEventsWhenNoOverdueLeads(t *testing.T) {
	fake := &fakeOverdueStore{leads: nil}
	hub := events.NewHub()
	ch, unsub := hub.Subscribe("ws-empty")
	defer unsub()

	w := cadenceworker.New(cadenceworker.Options{
		Store:    fake,
		Hub:      hub,
		Interval: time.Hour,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	go w.Run(ctx)
	<-ctx.Done()

	select {
	case ev := <-ch:
		t.Errorf("unexpected event: %+v", ev)
	default:
		// correct — nothing published
	}
}
