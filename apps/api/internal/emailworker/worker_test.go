package emailworker

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"marketingops/api/internal/secretcipher"
	"marketingops/api/internal/store"
)

type sentCall struct {
	ID        string
	MessageID string
}

type failedCall struct {
	ID      string
	Message string
	RetryAt *time.Time
}

type fakeStore struct {
	mu         sync.Mutex
	deliveries []store.EmailDelivery
	settings   map[string]store.EmailSettings
	statuses   map[string]string
	sent       []sentCall
	failed     []failedCall
	audits     []store.AuditEvent
}

func newFakeStore(key []byte, workspaceID string) *fakeStore {
	ciphertext, err := secretcipher.Encrypt(key, []byte("re_workspace_key"))
	if err != nil {
		panic(err)
	}
	return &fakeStore{
		settings: map[string]store.EmailSettings{
			workspaceID: {
				WorkspaceID:      workspaceID,
				Provider:         "resend",
				FromName:         "Rosewood Heating",
				FromEmail:        "hello@rosewood.example",
				APIKeyCiphertext: ciphertext,
				Status:           "connected",
			},
		},
		statuses: map[string]string{},
		deliveries: []store.EmailDelivery{
			{
				ID: "delivery-1", WorkspaceID: workspaceID, Kind: "lead_reply",
				ToEmail: "jane@example.com", Subject: "Re: furnace check",
				Body: "Hi Jane, we can visit on Thursday.", Status: "queued",
				NextAttemptAt: ptrTime(time.Now().Add(-time.Second)),
			},
		},
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func (f *fakeStore) ClaimDueEmailDelivery(_ context.Context, maxAttempts int, now time.Time) (store.EmailDelivery, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.deliveries {
		d := &f.deliveries[i]
		if d.Status != "queued" && d.Status != "failed" {
			continue
		}
		if d.NextAttemptAt == nil || d.NextAttemptAt.After(now) || d.Attempts >= maxAttempts {
			continue
		}
		d.Status = "sending"
		d.Attempts++
		return *d, nil
	}
	return store.EmailDelivery{}, store.ErrNotFound
}

func (f *fakeStore) CompleteEmailDeliverySent(_ context.Context, id, providerMessageID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, sentCall{ID: id, MessageID: providerMessageID})
	for i := range f.deliveries {
		if f.deliveries[i].ID == id {
			f.deliveries[i].Status = "sent"
			f.deliveries[i].ProviderMessageID = providerMessageID
		}
	}
	return nil
}

func (f *fakeStore) CompleteEmailDeliveryFailed(_ context.Context, id, lastErr string, retryAfter *time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failed = append(f.failed, failedCall{ID: id, Message: lastErr, RetryAt: retryAfter})
	for i := range f.deliveries {
		if f.deliveries[i].ID == id {
			f.deliveries[i].Status = "failed"
			f.deliveries[i].LastError = lastErr
			f.deliveries[i].NextAttemptAt = retryAfter
		}
	}
	return nil
}

func (f *fakeStore) RecoverStuckEmailDeliveries(context.Context, time.Duration, time.Time) (int, error) {
	return 0, nil
}

func (f *fakeStore) GetEmailSettings(_ context.Context, workspaceID string) (store.EmailSettings, error) {
	s, ok := f.settings[workspaceID]
	if !ok {
		return store.EmailSettings{}, store.ErrNotFound
	}
	return s, nil
}

func (f *fakeStore) MarkEmailSettingsStatus(_ context.Context, workspaceID, status string, _ *time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses[workspaceID] = status
	return nil
}

func (f *fakeStore) AddAuditEvent(_ context.Context, e store.AuditEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.audits = append(f.audits, e)
	return nil
}

func testKey() []byte { return []byte(strings.Repeat("k", secretcipher.KeyLength)) }

const wsID = "44444444-4444-4444-4444-444444444444"

func TestWorkerSendsAndRecordsProviderMessageID(t *testing.T) {
	var gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"id":"msg_42"}`)
	}))
	defer srv.Close()

	fs := newFakeStore(testKey(), wsID)
	w := New(Options{Store: fs, CredentialKey: testKey(), APIBase: srv.URL, HTTPClient: srv.Client(), Interval: time.Second})
	w.once(context.Background())

	if len(fs.sent) != 1 || fs.sent[0].ID != "delivery-1" || fs.sent[0].MessageID != "msg_42" {
		t.Fatalf("expected one sent delivery with the provider id, got %+v", fs.sent)
	}
	if len(fs.failed) != 0 {
		t.Fatalf("unexpected failures: %+v", fs.failed)
	}
	if gotAuth != "Bearer re_workspace_key" {
		t.Errorf("authorization = %q", gotAuth)
	}
	if !strings.Contains(gotBody, "hello@rosewood.example") || !strings.Contains(gotBody, "jane@example.com") {
		t.Errorf("payload lost the workspace's own addresses: %s", gotBody)
	}
	var sentAudit bool
	for _, a := range fs.audits {
		if a.Action == "email_delivery_sent" {
			sentAudit = true
		}
	}
	if !sentAudit {
		t.Errorf("sent audit event missing: %+v", fs.audits)
	}
}

func TestWorkerRetriesOnlyRetryableFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	fs := newFakeStore(testKey(), wsID)
	w := New(Options{Store: fs, CredentialKey: testKey(), APIBase: srv.URL, HTTPClient: srv.Client(), Interval: time.Second})
	w.once(context.Background())

	if len(fs.failed) != 1 {
		t.Fatalf("expected one failure, got %+v", fs.failed)
	}
	if fs.failed[0].RetryAt == nil {
		t.Fatalf("a 503 must stay retryable, got terminal failure: %+v", fs.failed[0])
	}
	if fs.failed[0].Message != "the email provider is temporarily unavailable" {
		t.Errorf("unexpected safe message %q", fs.failed[0].Message)
	}
	if _, ok := fs.statuses[wsID]; ok {
		t.Error("a provider outage must not flag the stored key as broken")
	}

	// Past the backoff the next attempt runs, and the attempt count advances.
	w.Clock = func() time.Time { return time.Now().Add(2 * time.Minute) }
	w.once(context.Background())
	if fs.deliveries[0].Attempts != 2 {
		t.Errorf("attempts = %d, want 2", fs.deliveries[0].Attempts)
	}
}

func TestWorkerFlagsRejectedKeyAndStops(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	fs := newFakeStore(testKey(), wsID)
	w := New(Options{Store: fs, CredentialKey: testKey(), APIBase: srv.URL, HTTPClient: srv.Client(), Interval: time.Second})
	w.once(context.Background())

	if len(fs.failed) != 1 || fs.failed[0].RetryAt != nil {
		t.Fatalf("a rejected key must not be retried: %+v", fs.failed)
	}
	if fs.statuses[wsID] != "error" {
		t.Errorf("connection status = %q, want error", fs.statuses[wsID])
	}
	if fs.deliveries[0].Status != "failed" || fs.deliveries[0].LastError == "" {
		t.Errorf("delivery not marked failed with a reason: %+v", fs.deliveries[0])
	}
	// A second drain finds nothing to do.
	w.once(context.Background())
	if len(fs.failed) != 1 {
		t.Errorf("terminal failure was attempted again: %+v", fs.failed)
	}
}

func TestWorkerFailsWhenWorkspaceDisconnected(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer srv.Close()

	fs := newFakeStore(testKey(), wsID)
	delete(fs.settings, wsID) // the operator disconnected mid-queue
	w := New(Options{Store: fs, CredentialKey: testKey(), APIBase: srv.URL, HTTPClient: srv.Client(), Interval: time.Second})
	w.once(context.Background())

	if called {
		t.Error("no provider call may happen without a stored connection")
	}
	if len(fs.failed) != 1 || fs.failed[0].Message != "email is not connected for this workspace" {
		t.Fatalf("expected a terminal not-configured failure, got %+v", fs.failed)
	}
	if fs.failed[0].RetryAt != nil {
		t.Error("a missing connection must not be retried")
	}
}

func TestWorkerReportsUnreadableKeyAsTerminal(t *testing.T) {
	fs := newFakeStore(testKey(), wsID)
	settings := fs.settings[wsID]
	settings.APIKeyCiphertext = []byte("not ciphertext")
	fs.settings[wsID] = settings

	w := New(Options{Store: fs, CredentialKey: testKey(), Interval: time.Second})
	w.once(context.Background())
	if len(fs.failed) != 1 || fs.failed[0].RetryAt != nil {
		t.Fatalf("expected one terminal failure, got %+v", fs.failed)
	}
}
