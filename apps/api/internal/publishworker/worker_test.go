package publishworker

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"marketingops/api/internal/integration"
	"marketingops/api/internal/secretcipher"
	"marketingops/api/internal/store"
)

var fakeClock = time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)

func clockForCap() func() time.Time { return func() time.Time { return fakeClock } }

func quietJobLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

// fakeJobStore records the scheduling outcomes so tests can assert the retry
// policy and lifecycle transitions without a database.
type fakeJobStore struct {
	sync.Mutex
	claimed           []store.PublishingJobRun
	completeOK        []string
	failRetryAt       map[string]time.Time
	failRetried       []string
	failTerminal      []string
	failReasons       map[string]string
	audits            []string
	auditsDetail      []store.AuditEvent
	creds             map[string]store.CredentialSet
	sweeps            []sweepCall
	sweptCount        int
	statusFlips       []statusFlip
	capabilityUpserts []store.IntegrationCapability
}

// statusFlip records one SetIntegrationStatus call.
type statusFlip struct {
	workspaceID   string
	integrationID string
	status        string
}

func (f *fakeJobStore) completeCount() int {
	f.Lock()
	defer f.Unlock()
	return len(f.completeOK)
}

func (f *fakeJobStore) sweepsCount() int {
	f.Lock()
	defer f.Unlock()
	return len(f.sweeps)
}

func (f *fakeJobStore) getSweeps() []sweepCall {
	f.Lock()
	defer f.Unlock()
	cp := make([]sweepCall, len(f.sweeps))
	copy(cp, f.sweeps)
	return cp
}

func (f *fakeJobStore) ClaimDuePublishingJob(ctx context.Context, maxAttempts int, now time.Time) (store.PublishingJobRun, error) {
	f.Lock()
	defer f.Unlock()
	if len(f.claimed) == 0 {
		return store.PublishingJobRun{}, store.ErrNotFound
	}
	run := f.claimed[0]
	f.claimed = f.claimed[1:]
	return run, nil
}

func (f *fakeJobStore) CompletePublishingJobSucceeded(ctx context.Context, jobID, integrationID string, externalID, externalURL *string) (store.ContentItem, error) {
	f.Lock()
	defer f.Unlock()
	f.completeOK = append(f.completeOK, jobID)
	return store.ContentItem{ID: "it-1", Status: "published"}, nil
}

func (f *fakeJobStore) CompletePublishingJobFailed(ctx context.Context, jobID, lastErr string, retryAfter *time.Time) error {
	f.Lock()
	defer f.Unlock()
	if f.failRetryAt == nil {
		f.failRetryAt = map[string]time.Time{}
	}
	if retryAfter != nil {
		f.failRetryAt[jobID] = *retryAfter
		f.failRetried = append(f.failRetried, jobID)
	} else {
		f.failTerminal = append(f.failTerminal, jobID)
	}
	if f.failReasons == nil {
		f.failReasons = map[string]string{}
	}
	f.failReasons[jobID] = lastErr
	return nil
}

func (f *fakeJobStore) GetCredentialSet(ctx context.Context, integrationID string) (store.CredentialSet, error) {
	f.Lock()
	defer f.Unlock()
	set, ok := f.creds[integrationID]
	if !ok {
		return store.CredentialSet{}, store.ErrNotFound
	}
	return set, nil
}

func (f *fakeJobStore) AddAuditEvent(ctx context.Context, e store.AuditEvent) error {
	f.Lock()
	defer f.Unlock()
	f.audits = append(f.audits, e.Action)
	f.auditsDetail = append(f.auditsDetail, e)
	return nil
}

func (f *fakeJobStore) RecoverStuckPublishingJobs(ctx context.Context, olderThan time.Duration, now time.Time) (int, error) {
	f.Lock()
	defer f.Unlock()
	f.sweeps = append(f.sweeps, sweepCall{olderThan: olderThan, now: now})
	return f.sweptCount, nil
}

func (f *fakeJobStore) SetIntegrationStatus(ctx context.Context, workspaceID, integrationID, status string) error {
	f.Lock()
	defer f.Unlock()
	f.statusFlips = append(f.statusFlips, statusFlip{workspaceID, integrationID, status})
	return nil
}

func (f *fakeJobStore) UpsertCapability(ctx context.Context, integrationID string, c store.IntegrationCapability) error {
	f.Lock()
	defer f.Unlock()
	f.capabilityUpserts = append(f.capabilityUpserts, c)
	return nil
}

// stubPassthroughAdapter satisfies integration.Adapter for custom kinds.
type stubPassthroughAdapter struct{ kind string }

func (a stubPassthroughAdapter) Kind() string        { return a.kind }
func (a stubPassthroughAdapter) DisplayName() string { return "Stub" }
func (a stubPassthroughAdapter) Description() string { return "test stub" }

func (a stubPassthroughAdapter) DefaultCapabilities() []integration.CapabilityReport {
	return nil
}

func (a stubPassthroughAdapter) Check(ctx context.Context, config map[string]any, creds map[string]string) integration.CheckResult {
	return integration.CheckResult{}
}

// scriptedPublisher is a Registered publisher returning either a success
// outcome or a provider failure.
type scriptedPublisher struct {
	fail error
}

func (p scriptedPublisher) CreateDraft(ctx context.Context, payload integration.ContentPayload) (string, string, error) {
	if p.fail != nil {
		return "", "", p.fail
	}
	return "drafts.mktops-" + payload.ItemID, "tx-1", nil
}

func (p scriptedPublisher) Publish(ctx context.Context, payload integration.ContentPayload) (integration.PublishOutcome, error) {
	if p.fail != nil {
		return integration.PublishOutcome{}, p.fail
	}
	return integration.PublishOutcome{ExternalID: "ext-" + payload.ItemID, TransactionID: "tx-2"}, nil
}

func (p scriptedPublisher) VerifyPublicURL(ctx context.Context, websiteURL, slug string) integration.URLVerification {
	return integration.URLVerification{
		URL: "https://public.example/posts/" + slug, Verified: true, HTTPStatus: 200,
	}
}

type scriptedAdapter struct{ fail error }

func (a scriptedAdapter) NewPublisher(map[string]any, map[string]string, string, *http.Client) (integration.Publisher, error) {
	return scriptedPublisher{fail: a.fail}, nil
}

var testJobKey = []byte(strings.Repeat("k", 32))

func workerWith(t *testing.T, jobs *fakeJobStore, fail error, maxAttempts int) *Worker {
	t.Helper()
	jobs.failRetryAt = map[string]time.Time{}
	enc, err := secretcipher.Encrypt(testJobKey, []byte(`{"token":"sk-test-write"}`))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	jobs.creds = map[string]store.CredentialSet{
		"in-1": {IntegrationID: "in-1", KeyVersion: 1, Ciphertext: enc},
	}
	registry := integration.NewRegistry()
	registry.Register(stubPassthroughAdapter{kind: "custom_kind"})
	registry.RegisterPublisher("custom_kind", scriptedAdapter{fail: fail})
	return New(Options{
		Store:         jobs,
		Registry:      registry,
		CredentialKey: testJobKey,
		Logger:        quietJobLogger(),
		BaseDelay:     time.Minute,
		MaxDelay:      4 * time.Minute,
		MaxAttempts:   maxAttempts,
		Clock:         clockForCap(),
	})
}

func sampleRun(jobID string) store.PublishingJobRun {
	website := "https://public.example"
	slug := "probe-slug"
	return store.PublishingJobRun{
		Job: store.PublishingJob{
			ID: jobID, ContentItemID: "it-1", ContentVersionID: "v-1", IntegrationID: "in-1",
			IdempotencyKey: "ws:it-1:v-1:in-1", Status: "pending", Attempts: 1,
		},
		Workspace:   store.Workspace{ID: "ws-1", Slug: "probe", Name: "Probe", WebsiteURL: &website},
		Integration: store.Integration{ID: "in-1", WorkspaceID: "ws-1", AdapterKind: "custom_kind", Name: "probe", Config: map[string]any{}},
		Item:        store.ContentItem{ID: "it-1", WorkspaceID: "ws-1", Kind: "blog_article", Status: "approved", Title: "Probe", Slug: &slug},
		Version:     store.ContentVersion{ID: "v-1", ContentItemID: "it-1", VersionNumber: 2, Title: "Probe", Body: "# Probe\n\nBody."},
	}
}

func TestWorkerPublishesDueJob(t *testing.T) {
	jobs := &fakeJobStore{creds: map[string]store.CredentialSet{}}
	worker := workerWith(t, jobs, nil, 3)
	jobs.claimed = []store.PublishingJobRun{sampleRun("job-1")}
	worker.once(context.Background())

	if len(jobs.completeOK) != 1 || jobs.completeOK[0] != "job-1" {
		t.Fatalf("succeeded completions = %v", jobs.completeOK)
	}
	if len(jobs.failTerminal) != 0 || len(jobs.failRetried) != 0 {
		t.Fatalf("unexpected failure records: %+v %+v", jobs.failTerminal, jobs.failRetried)
	}
	if len(jobs.audits) != 1 || jobs.audits[0] != "publishing_job_succeeded" {
		t.Fatalf("audits = %v", jobs.audits)
	}
	if d := jobs.auditsDetail[0].Details; d["publishedId"] != "ext-it-1" || d["verified"] != true {
		t.Fatalf("success audit details = %+v", d)
	}
}

func TestWorkerTokenExpiryFlagsIntegrationForReconnection(t *testing.T) {
	jobs := &fakeJobStore{creds: map[string]store.CredentialSet{}}
	// The token expiry comes from the publish step; CreateDraft is skipped
	// (draftless) or passes, mirroring the Meta draftless path.
	worker := workerWith(t, jobs, &integration.MetaError{
		Kind: "token_expired",
		Safe: "the Meta access token was rejected or has expired; reconnect the integration to refresh authorization",
	}, 3)
	jobs.claimed = []store.PublishingJobRun{sampleRun("job-2")}
	worker.once(context.Background())

	if len(jobs.statusFlips) != 1 {
		t.Fatalf("expected exactly one integration status flip, got %+v", jobs.statusFlips)
	}
	flip := jobs.statusFlips[0]
	if flip.workspaceID != "ws-1" || flip.integrationID != "in-1" {
		t.Fatalf("the flip must be workspace/integration scoped, got %+v", flip)
	}
	if flip.status != integration.LifecycleError {
		t.Fatalf("expected the error lifecycle, got %q", flip.status)
	}
	flips := jobs.capabilityUpserts
	if len(flips) != 1 || flips[0].Capability != integration.CapPublish || flips[0].Status != integration.StatusFailed {
		t.Fatalf("expected the publish capability marked failed, got %+v", flips)
	}
	if flips[0].Details == nil || !strings.Contains(*flips[0].Details, "token") {
		t.Fatalf("the capability detail should carry the safe token message, got %+v", flips[0])
	}
}

func TestWorkerRetriesWithBackoffBeforeTerminal(t *testing.T) {
	jobs := &fakeJobStore{creds: map[string]store.CredentialSet{}}
	worker := workerWith(t, jobs, &integration.SanityWriteError{Kind: integration.SanityFailProvider, HTTP: 503}, 4)
	jobs.claimed = []store.PublishingJobRun{sampleRun("job-1")}
	worker.once(context.Background())
	// attempts=1 < max → one retry slot at base delay, not terminal.
	if len(jobs.failRetried) != 1 || jobs.failRetried[0] != "job-1" {
		t.Fatalf("retryable failure not recorded: %+v", jobs.failRetried)
	}
	if got := jobs.failRetryAt["job-1"]; got != fakeClock.Add(time.Minute) {
		t.Fatalf("first backoff = %v, want base delay after %v", got, fakeClock)
	}
	if len(jobs.failTerminal) != 0 {
		t.Fatalf("job marked terminal too early: %v", jobs.failTerminal)
	}
	if reason := jobs.failReasons["job-1"]; !strings.Contains(reason, "HTTP 503") {
		t.Fatalf("safe provider reason not used: %q", reason)
	}

	// attempts == max → terminal, no further retry slot.
	jobs.failRetryAt = map[string]time.Time{}
	jobs.failRetried = nil
	terminal := sampleRun("job-1")
	terminal.Job.Attempts = 4
	jobs.claimed = []store.PublishingJobRun{terminal}
	worker.once(context.Background())
	if len(jobs.failTerminal) != 1 || jobs.failTerminal[0] != "job-1" {
		t.Fatalf("terminal failure not recorded: %v", jobs.failTerminal)
	}
	if len(jobs.failRetried) != 0 || len(jobs.failRetryAt) != 0 {
		t.Errorf("terminal job must not schedule a retry: %+v", jobs.failRetryAt)
	}
}

func TestWorkerBackoffGrowsAndCaps(t *testing.T) {
	worker := New(Options{
		Store:       &fakeJobStore{},
		Registry:    integration.NewRegistry(),
		Logger:      quietJobLogger(),
		BaseDelay:   time.Minute,
		MaxDelay:    4 * time.Minute,
		MaxAttempts: 10,
	})
	cases := map[int]time.Duration{
		1: time.Minute, 2: 2 * time.Minute, 3: 4 * time.Minute,
		4: 4 * time.Minute, 9: 4 * time.Minute,
	}
	for attempts, want := range cases {
		if got := worker.backoff(attempts); got != want {
			t.Errorf("backoff(%d) = %v, want %v", attempts, got, want)
		}
	}
}

func TestWorkerUnregisteredKindFailsTerminally(t *testing.T) {
	jobs := &fakeJobStore{}
	enc, err := secretcipher.Encrypt(testJobKey, []byte(`{"token":"sk-test-write"}`))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	jobs.creds = map[string]store.CredentialSet{
		"in-1": {IntegrationID: "in-1", KeyVersion: 1, Ciphertext: enc},
	}
	registry := integration.NewRegistry() // no adapter at all for the job's kind
	worker := New(Options{
		Store:         jobs,
		Registry:      registry,
		CredentialKey: testJobKey,
		Logger:        quietJobLogger(),
		MaxAttempts:   3,
		Clock:         clockForCap(),
	})
	jobs.claimed = []store.PublishingJobRun{sampleRun("job-9")}
	worker.once(context.Background())
	if len(jobs.failTerminal) != 1 || jobs.failTerminal[0] != "job-9" {
		t.Fatalf("terminal failure not recorded: %v", jobs.failTerminal)
	}
	if reason := jobs.failReasons["job-9"]; !strings.Contains(reason, "publishing") {
		t.Fatalf("terminal reason should be safe and descriptive: %q", reason)
	}
	if len(jobs.completeOK) != 0 {
		t.Errorf("no external write may happen without an adapter")
	}
}

func TestWorkerPayloadMapping(t *testing.T) {
	run := sampleRun("job-2")
	run.Version.CallToAction = strPtr("Book now")
	run.Version.ChannelCopy = map[string]any{"FACEBOOK": "social copy"}
	run.Version.Metadata = map[string]any{
		"mainImage": map[string]any{"ref": "image-x", "src": "https://cdn", "alt": "Hero"},
	}
	payload := workerPayload(run)
	if payload.Title != "Probe" || payload.Slug != "probe-slug" || payload.Body == "" {
		t.Fatalf("basic mapping wrong: %+v", payload)
	}
	if payload.CallToAction != "Book now" {
		t.Fatalf("cta = %q", payload.CallToAction)
	}
	if payload.ChannelCopy["facebook"] != "social copy" {
		t.Fatalf("channel copy = %v", payload.ChannelCopy)
	}
	if payload.MainImage == nil || payload.MainImage.Ref != "image-x" || payload.MainImage.Alt != "Hero" {
		t.Fatalf("main image = %+v", payload.MainImage)
	}
}

func TestWorkerNothingDueIsNoop(t *testing.T) {
	jobs := &fakeJobStore{}
	worker := New(Options{Store: jobs, Registry: integration.NewRegistry(), Logger: quietJobLogger(), CredentialKey: testJobKey})
	worker.once(context.Background())
	if len(jobs.completeOK) != 0 || len(jobs.failTerminal) != 0 {
		t.Errorf("expected no work, got completed=%v failed=%v", jobs.completeOK, jobs.failTerminal)
	}
}

func TestWorkerDrainsBeforeFirstTick(t *testing.T) {
	jobs := &fakeJobStore{creds: map[string]store.CredentialSet{}}
	worker := workerWith(t, jobs, nil, 3)
	jobs.claimed = []store.PublishingJobRun{sampleRun("job-immediate")}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()
	deadline := time.After(2 * time.Second)
	for jobs.completeCount() == 0 {
		select {
		case <-deadline:
			t.Fatalf("worker did not drain before its first tick")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("worker did not stop after cancel")
	}
}

func strPtr(s string) *string { return &s }

type sweepCall struct {
	olderThan time.Duration
	now       time.Time
}

func TestWorkerRecoversStuckJobsAtStartup(t *testing.T) {
	jobs := &fakeJobStore{creds: map[string]store.CredentialSet{}}
	jobs.sweptCount = 2
	worker := workerWith(t, jobs, nil, 3)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	workflowDone := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(workflowDone)
	}()
	deadline := time.After(2 * time.Second)
	for jobs.sweepsCount() == 0 {
		select {
		case <-deadline:
			t.Fatalf("worker never ran the stuck-job sweep")
		case <-time.After(5 * time.Millisecond):
		}
	}
	sweeps := jobs.getSweeps()
	if sweeps[0].olderThan != DefaultStuckAfter {
		t.Errorf("sweep threshold = %v, want default %v", sweeps[0].olderThan, DefaultStuckAfter)
	}
	cancel()
	select {
	case <-workflowDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("worker did not stop after cancel")
	}
}

// Custom-threshold sweep test via worker.Run using an explicit StuckAfter.
func TestWorkerSweepUsesConfiguredThreshold(t *testing.T) {
	jobs := &fakeJobStore{creds: map[string]store.CredentialSet{}}
	worker := workerWith(t, jobs, nil, 3)
	worker.StuckAfter = 3 * time.Minute
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	workflowDone := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(workflowDone)
	}()
	deadline := time.After(2 * time.Second)
	for jobs.sweepsCount() == 0 {
		select {
		case <-deadline:
			t.Fatalf("worker never ran the sweep")
		case <-time.After(5 * time.Millisecond):
		}
	}
	sweeps := jobs.getSweeps()
	if sweeps[0].olderThan != 3*time.Minute {
		t.Errorf("sweep threshold = %v, want configured 3m", sweeps[0].olderThan)
	}
	cancel()
	select {
	case <-workflowDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("worker did not stop")
	}
}
