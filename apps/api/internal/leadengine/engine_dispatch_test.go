package leadengine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"marketingops/api/internal/store"
)

// dispatchFakeStore is an in-memory LeadEngineStore for dispatch tests. It
// keeps the outreach log so duplicate suppression can be exercised across runs.
type dispatchFakeStore struct {
	mu              sync.Mutex
	leads           []store.Lead
	logs            []store.LeadOutreachLog
	statusUpdates   []string
	saveOutreachErr error
	dedupeErr       error
	nextLogID       int
}

func (f *dispatchFakeStore) WasSearchedToday(context.Context, string, string, string, string) (bool, error) {
	return false, nil
}

func (f *dispatchFakeStore) RecordDiscoverySearch(context.Context, store.LeadDiscoverySearch) error {
	return nil
}

func (f *dispatchFakeStore) IsBlacklisted(context.Context, string, string) (bool, error) {
	return false, nil
}

func (f *dispatchFakeStore) SaveDiscoveredLead(context.Context, store.DiscoveredLead) (store.Lead, error) {
	return store.Lead{}, nil
}

func (f *dispatchFakeStore) ListLeadEngineLeads(context.Context, string) ([]store.Lead, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]store.Lead(nil), f.leads...), nil
}

func (f *dispatchFakeStore) UpdateLeadPitch(_ context.Context, _, leadID, pitch string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.leads {
		if f.leads[i].ID == leadID {
			f.leads[i].Pitch = pitch
			f.leads[i].Status = "pending"
		}
	}
	return nil
}

func (f *dispatchFakeStore) UpdateLeadStatus(_ context.Context, _, leadID, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statusUpdates = append(f.statusUpdates, status)
	for i := range f.leads {
		if f.leads[i].ID == leadID {
			f.leads[i].Status = status
		}
	}
	return nil
}

func (f *dispatchFakeStore) SaveOutreachLog(_ context.Context, l store.LeadOutreachLog) (store.LeadOutreachLog, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.saveOutreachErr != nil {
		return store.LeadOutreachLog{}, f.saveOutreachErr
	}
	f.nextLogID++
	l.ID = fmt.Sprintf("log-%d", f.nextLogID)
	f.logs = append(f.logs, l)
	return l, nil
}

func (f *dispatchFakeStore) UpdateOutreachLog(_ context.Context, _, logID, status, errorText string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.logs {
		if f.logs[i].ID == logID {
			f.logs[i].Status = status
			f.logs[i].Error = errorText
			return nil
		}
	}
	return errors.New("log not found")
}

func (f *dispatchFakeStore) HasRecentOutboundAttempt(context.Context, string, string, time.Duration) (bool, error) {
	return false, nil
}

func (f *dispatchFakeStore) HasRecentOutboundAttemptByPhone(context.Context, string, string, time.Duration) (bool, error) {
	return false, nil
}

func (f *dispatchFakeStore) HasOutboundAttemptByPhone(_ context.Context, _, phone string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dedupeErr != nil {
		return false, f.dedupeErr
	}
	key := store.PhoneKey(phone)
	for _, l := range f.logs {
		if l.Direction != "outbound" || strings.HasPrefix(l.Error, "not_sent:") {
			continue
		}
		for _, lead := range f.leads {
			if lead.ID == l.LeadID && lead.ContactPhone != nil && store.PhoneKey(*lead.ContactPhone) == key {
				return true, nil
			}
		}
	}
	return false, nil
}

func (f *dispatchFakeStore) CountOutboundSentToday(context.Context, string) (int, error) {
	return 0, nil
}

func (f *dispatchFakeStore) AddToBlacklist(context.Context, string, string, string) error {
	return nil
}

func (f *dispatchFakeStore) ResetStaleProcessingLeads(context.Context, string) (int64, error) {
	return 0, nil
}

func (f *dispatchFakeStore) AdvanceLeadCadence(context.Context, string, string, int, *time.Time) error {
	return nil
}

func (f *dispatchFakeStore) logCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.logs)
}

func (f *dispatchFakeStore) firstLog() store.LeadOutreachLog {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.logs[0]
}

func (f *dispatchFakeStore) leadStatus() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.leads[0].Status
}

func newDispatchTestStore(phone string) *dispatchFakeStore {
	return &dispatchFakeStore{leads: []store.Lead{{
		ID:           "lead-1",
		ContactPhone: &phone,
		Pitch:        "pre-written pitch",
		Status:       "discovered",
	}}}
}

// dispatcherStub serves the status check and counts /send calls.
type dispatcherStub struct {
	mu           sync.Mutex
	sends        int
	logAtSend    int
	store        *dispatchFakeStore
	failWithCode int
}

func (d *dispatcherStub) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/sessions/w1/status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"connected"}`))
	})
	mux.HandleFunc("/sessions/w1/send", func(w http.ResponseWriter, _ *http.Request) {
		d.mu.Lock()
		d.sends++
		d.logAtSend = d.store.logCount()
		code := d.failWithCode
		d.mu.Unlock()
		if code != 0 {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"success":false,"error":"send outcome unknown"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	})
	return mux
}

func (d *dispatcherStub) setFailCode(code int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.failWithCode = code
}

func (d *dispatcherStub) sendCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.sends
}

func dispatchParams(baseURL string) DispatchParams {
	return DispatchParams{
		WorkspaceID: "w1",
		DispatchURL: baseURL,
		DailyLimit:  1,
		MinDelayMs:  0,
		MaxDelayMs:  0,
	}
}

func TestRunDispatchRecordsAttemptBeforeSending(t *testing.T) {
	store := newDispatchTestStore("+233201234567")
	stub := &dispatcherStub{store: store}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	if err := RunDispatch(context.Background(), dispatchParams(srv.URL), store); err != nil {
		t.Fatalf("RunDispatch: %v", err)
	}
	if stub.sendCount() != 1 {
		t.Fatalf("send calls = %d, want 1", stub.sendCount())
	}
	if stub.logAtSend != 1 {
		t.Fatalf("outreach log rows present when send fired = %d, want 1", stub.logAtSend)
	}
	log := store.firstLog()
	if log.Status != "sent" || log.Error != "" {
		t.Fatalf("log after success = %+v, want status sent and no error", log)
	}
	if got := store.leadStatus(); got != "sent" {
		t.Fatalf("lead status = %q, want sent", got)
	}
}

func TestRunDispatchRefusesToSendWithoutAttemptRecord(t *testing.T) {
	store := newDispatchTestStore("+233201234567")
	store.saveOutreachErr = errors.New("database unavailable")
	stub := &dispatcherStub{store: store}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	if err := RunDispatch(context.Background(), dispatchParams(srv.URL), store); err != nil {
		t.Fatalf("RunDispatch: %v", err)
	}
	if stub.sendCount() != 0 {
		t.Fatalf("send calls = %d, want 0 when the attempt cannot be recorded", stub.sendCount())
	}
	if got := store.leadStatus(); got != "failed" {
		t.Fatalf("lead status = %q, want failed", got)
	}
	report, ok := LastDispatchReport("w1")
	if !ok || report.Skips["lead-1"] != "could not record the send attempt — not sent" {
		t.Fatalf("skip report = %+v, want the attempt-record failure reason", report)
	}
}

func TestRunDispatchDoesNotSendWhenDedupeCheckFails(t *testing.T) {
	store := newDispatchTestStore("+233201234567")
	store.dedupeErr = errors.New("database unavailable")
	stub := &dispatcherStub{store: store}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	if err := RunDispatch(context.Background(), dispatchParams(srv.URL), store); err != nil {
		t.Fatalf("RunDispatch: %v", err)
	}
	if stub.sendCount() != 0 {
		t.Fatalf("send calls = %d, want 0 when the duplicate check cannot be answered", stub.sendCount())
	}
	report, ok := LastDispatchReport("w1")
	if !ok || !strings.Contains(report.Skips["lead-1"], "previous sends") {
		t.Fatalf("skip report = %+v, want the duplicate-check failure reason", report)
	}
}

func TestRunDispatchNeverRetriesUnknownOutcomeAfterRestart(t *testing.T) {
	store := newDispatchTestStore("+233201234567")
	stub := &dispatcherStub{store: store, failWithCode: http.StatusServiceUnavailable}
	stub.failWithCode = http.StatusServiceUnavailable
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	if err := RunDispatch(context.Background(), dispatchParams(srv.URL), store); err != nil {
		t.Fatalf("first RunDispatch: %v", err)
	}
	if stub.sendCount() != 1 {
		t.Fatalf("first run send calls = %d, want 1", stub.sendCount())
	}
	if log := store.firstLog(); log.Status != "failed" || strings.HasPrefix(log.Error, "not_sent:") {
		t.Fatalf("unknown outcome log = %+v, want failed and permanently suppressing", log)
	}

	// Simulate a restart: the lead is queued again by stale-processing recovery.
	store.mu.Lock()
	store.leads[0].Status = "failed"
	store.mu.Unlock()
	stub.setFailCode(0)

	if err := RunDispatch(context.Background(), dispatchParams(srv.URL), store); err != nil {
		t.Fatalf("second RunDispatch: %v", err)
	}
	if stub.sendCount() != 1 {
		t.Fatalf("send calls after restart = %d, want 1 (unknown outcome must never be retried)", stub.sendCount())
	}
	if got := store.leadStatus(); got != "sent" {
		t.Fatalf("lead status after suppression = %q, want sent", got)
	}
	report, ok := LastDispatchReport("w1")
	if !ok || report.Skips["lead-1"] != "already contacted" {
		t.Fatalf("skip report = %+v, want already contacted", report)
	}
}

func TestRunDispatchCountsUnknownOutcomeAsUnconfirmed(t *testing.T) {
	store := newDispatchTestStore("+233201234567")
	stub := &dispatcherStub{store: store, failWithCode: http.StatusServiceUnavailable}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	if err := RunDispatch(context.Background(), dispatchParams(srv.URL), store); err != nil {
		t.Fatalf("RunDispatch: %v", err)
	}
	report, ok := LastDispatchReport("w1")
	if !ok {
		t.Fatal("no dispatch report recorded")
	}
	if report.Failed != 1 || report.Unconfirmed != 1 {
		t.Fatalf("report = %+v, want 1 failed and 1 unconfirmed", report)
	}
}

func TestRunDispatchRefusalIsNotCountedAsUnconfirmed(t *testing.T) {
	store := newDispatchTestStore("+233201234567")
	stub := &dispatcherStub{store: store, failWithCode: http.StatusConflict}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	if err := RunDispatch(context.Background(), dispatchParams(srv.URL), store); err != nil {
		t.Fatalf("RunDispatch: %v", err)
	}
	report, ok := LastDispatchReport("w1")
	if !ok {
		t.Fatal("no dispatch report recorded")
	}
	if report.Failed != 1 || report.Unconfirmed != 0 {
		t.Fatalf("report = %+v, want a provable refusal kept out of the unconfirmed count", report)
	}
	if log := store.firstLog(); !strings.HasPrefix(log.Error, "not_sent:") {
		t.Fatalf("log error = %q, want a not_sent: prefix", log.Error)
	}
}
