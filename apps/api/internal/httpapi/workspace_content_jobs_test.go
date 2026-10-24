package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"marketingops/api/internal/integration"
	"marketingops/api/internal/secretcipher"
	"marketingops/api/internal/store"
)

// jobsTestHarness wires the fake store into the full router so jobs endpoints
// are exercised through real path values and JSON.
type jobsTestHarness struct {
	fake *fakeWorkspaceStore
	h    http.Handler
	key  []byte
}

func jobsHarness(t *testing.T) *jobsTestHarness {
	t.Helper()
	key := []byte(strings.Repeat("k", 32))
	fake := &fakeWorkspaceStore{
		byID: map[string]store.Workspace{
			"ws-1": {ID: "ws-1", Slug: "probe", Name: "Probe", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
		contentItemsByID: map[string]store.ContentItem{
			"it-1": {
				ID: "it-1", WorkspaceID: "ws-1", Kind: "blog_article", Status: "approved",
				Title: "Probe", Slug: strPtr("probe-slug"),
				CreatedAt: time.Now(), UpdatedAt: time.Now(),
			},
		},
		contentVersionsByItem: map[string][]store.ContentVersion{
			"it-1": {
				{ID: "v-1", ContentItemID: "it-1", VersionNumber: 1, Title: "Probe", Body: "body", CreatedAt: time.Now()},
			},
		},
		decisionsByItem: map[string][]store.Approval{
			"it-1": {{ID: "a-1", ContentItemID: "it-1", ContentVersionID: "v-1", Decision: "approved"}},
		},
		integrationsByID: map[string]store.Integration{
			"in-1": {
				ID: "in-1", WorkspaceID: "ws-1", AdapterKind: "sanity", Name: "probe sanity",
				Status: "connected",
				Config: map[string]any{"projectId": "p123", "dataset": "production"},
			},
		},
	}
	enc, err := secretcipher.Encrypt(key, []byte(`{"token":"sk-test-write"}`))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	fake.credentialSetsByInID = map[string]store.CredentialSet{
		"in-1": {IntegrationID: "in-1", KeyVersion: 1, Ciphertext: enc},
	}
	h := NewRouter(Deps{
		Logger:        testLogger,
		Workspaces:    fake,
		Integrations:  integration.NewDefaultRegistry(),
		CredentialKey: key,
	})
	return &jobsTestHarness{fake: fake, h: h, key: key}
}

func scheduledFor(offsetHours int) string {
	return time.Now().UTC().Add(time.Duration(offsetHours) * time.Hour).Format(time.RFC3339)
}

func TestScheduledPublishQueuesForWorker(t *testing.T) {
	h := jobsHarness(t)
	body := `{"integrationId":"in-1","mode":"publish","scheduledAt":"` + scheduledFor(6) + `"}`
	rec := doJSON(t, h.h, http.MethodPost, "/api/v1/workspaces/ws-1/content/it-1/publish", body)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Mode    string `json:"mode"`
		JobID   string `json:"jobId"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Mode != "publish" || resp.JobID == "" {
		t.Fatalf("malformed scheduled response: %s", rec.Body.String())
	}
	if len(h.fake.insertedJobs) != 1 {
		t.Fatalf("no job inserted: %+v", h.fake.insertedJobs)
	}
	job := h.fake.insertedJobs[0]
	if job.Status != "pending" || job.ScheduledAt == nil || job.NextAttemptAt == nil {
		t.Fatalf("job not scheduled properly: %+v", job)
	}
	if item := h.fake.contentItemsByID["it-1"]; item.Status != "approved" {
		t.Fatalf("scheduled publish must not publish inline, status = %s", item.Status)
	}
	if !containsJobAction(h.fake.auditEvents, "content.publish_scheduled") {
		t.Errorf("scheduled audit event missing: %+v", h.fake.auditEvents)
	}
}

func TestScheduledPublishRejectsBadTimestamp(t *testing.T) {
	h := jobsHarness(t)
	rec := doJSON(t, h.h, http.MethodPost, "/api/v1/workspaces/ws-1/content/it-1/publish",
		`{"integrationId":"in-1","mode":"publish","scheduledAt":"not-a-time"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body.String())
	}
	if len(h.fake.insertedJobs) != 0 {
		t.Fatalf("no job may be created for an invalid schedule: %+v", h.fake.insertedJobs)
	}
}

func TestListContentJobsReturnsHistory(t *testing.T) {
	h := jobsHarness(t)
	h.fake.jobsByKey = map[string]store.PublishingJob{
		"ws:it-1:v-1:in-1": {
			ID: "job-1", ContentItemID: "it-1", ContentVersionID: "v-1", IntegrationID: "in-1",
			IdempotencyKey: "ws:it-1:v-1:in-1", Status: "failed", Attempts: 3,
		},
	}
	rec := doJSON(t, h.h, http.MethodGet, "/api/v1/workspaces/ws-1/content/it-1/jobs", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Jobs []publishJobJSON `json:"jobs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Jobs) != 1 || resp.Jobs[0].ID != "job-1" || resp.Jobs[0].Status != "failed" {
		t.Fatalf("unexpected jobs payload: %+v", resp.Jobs)
	}
}

func TestCancelContentJob(t *testing.T) {
	h := jobsHarness(t)
	h.fake.jobsByKey = map[string]store.PublishingJob{
		"key": {
			ID: "job-1", ContentItemID: "it-1", ContentVersionID: "v-1", IntegrationID: "in-1",
			IdempotencyKey: "key", Status: "pending",
		},
	}
	rec := doJSON(t, h.h, http.MethodPost, "/api/v1/workspaces/ws-1/content/it-1/jobs/job-1/cancel", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Job publishJobJSON `json:"job"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Job.Status != "cancelled" {
		t.Fatalf("job status = %s", resp.Job.Status)
	}
	if !containsJobAction(h.fake.auditEvents, "publishing_job_cancelled") {
		t.Errorf("cancel audit event missing: %+v", h.fake.auditEvents)
	}

	// Cancelling a succeeded job is refused; the fake store mirrors the real
	// one and rejects anything but pending/failed.
	h.fake.jobsByKey["key2"] = store.PublishingJob{
		ID: "job-2", ContentItemID: "it-1", ContentVersionID: "v-1", IntegrationID: "in-1",
		IdempotencyKey: "key2", Status: "succeeded",
	}
	rec = doJSON(t, h.h, http.MethodPost, "/api/v1/workspaces/ws-1/content/it-1/jobs/job-2/cancel", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("succeeded cancel status = %d body %s", rec.Code, rec.Body.String())
	}
}

func containsJobAction(list []store.AuditEvent, want string) bool {
	for _, e := range list {
		if e.Action == want {
			return true
		}
	}
	return false
}

func TestRetryContentJob(t *testing.T) {
	h := jobsHarness(t)
	h.fake.jobsByKey = map[string]store.PublishingJob{
		"key": {
			ID: "job-1", ContentItemID: "it-1", ContentVersionID: "v-1", IntegrationID: "in-1",
			IdempotencyKey: "key", Status: "failed", Attempts: 2,
		},
	}
	rec := doJSON(t, h.h, http.MethodPost, "/api/v1/workspaces/ws-1/content/it-1/jobs/job-1/retry", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Job publishJobJSON `json:"job"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Job.Status != "pending" || resp.Job.NextAttemptAt == nil {
		t.Fatalf("retried job not queued: %+v", resp.Job)
	}
	if !containsJobAction(h.fake.auditEvents, "publishing_job_retried") {
		t.Errorf("retry audit event missing: %+v", h.fake.auditEvents)
	}

	// Retrying when the approval was invalidated is refused.
	versions := h.fake.contentVersionsByItem["it-1"]
	versions = append(versions, store.ContentVersion{ID: "v-2", ContentItemID: "it-1", VersionNumber: 2, Title: "Probe", Body: "new", CreatedAt: time.Now()})
	h.fake.contentVersionsByItem["it-1"] = versions
	rec = doJSON(t, h.h, http.MethodPost, "/api/v1/workspaces/ws-1/content/it-1/jobs/job-1/retry", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("stale-approval retry status = %d body %s", rec.Code, rec.Body.String())
	}
}
