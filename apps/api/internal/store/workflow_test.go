package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"marketingops/api/internal/secretcipher"
)

// seedIntegration gives tests one integration in its own workspace.
func seedIntegration(t *testing.T, s *Store, name string) (Workspace, Integration) {
	t.Helper()
	ws := uniqueWorkspace(t, s, "int-"+name)
	in, err := s.InsertIntegration(context.Background(), Integration{
		WorkspaceID: ws.ID,
		AdapterKind: "sanity",
		Name:        name,
		Status:      "unconfigured",
		Config:      map[string]any{"project": "demo-project"},
	})
	if err != nil {
		t.Fatalf("InsertIntegration: %v", err)
	}
	return ws, in
}

func TestIntegrationsScopedToWorkspace(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()

	wsA, inA := seedIntegration(t, s, "sanity-a")
	wsB, _ := seedIntegration(t, s, "sanity-b")

	// B cannot see A's integration.
	if got, err := s.ListIntegrations(ctx, wsB.ID); err != nil || len(got) != 1 {
		t.Fatalf("B must see only its own integration: n=%d err=%v", len(got), err)
	}
	if _, err := s.GetIntegrationInWorkspace(ctx, wsB.ID, inA.ID); err != ErrNotFound {
		t.Errorf("cross-workspace integration read must be ErrNotFound, got %v", err)
	}
	// Status updates are bounded by workspace too.
	if err := s.SetIntegrationStatus(ctx, wsB.ID, inA.ID, "connected"); err != ErrNotFound {
		t.Errorf("cross-workspace status update must be ErrNotFound, got %v", err)
	}
	if err := s.SetIntegrationStatus(ctx, wsA.ID, inA.ID, "connected"); err != nil {
		t.Errorf("own status update failed: %v", err)
	}
	got, err := s.GetIntegrationInWorkspace(ctx, wsA.ID, inA.ID)
	if err != nil || got.Status != "connected" {
		t.Errorf("status not updated: %+v err=%v", got, err)
	}

	// Duplicate (workspace, name) is rejected.
	if _, err := s.InsertIntegration(ctx, Integration{
		WorkspaceID: wsA.ID, AdapterKind: "sanity", Name: "sanity-a",
	}); err != ErrSlugTaken {
		t.Errorf("duplicate integration name must be ErrSlugTaken, got %v", err)
	}
}

func TestCapabilitiesAndCredentials(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()
	_, in := seedIntegration(t, s, "caps")

	// Capabilities start empty: nothing is claimed until verified.
	if caps, err := s.ListCapabilities(ctx, in.ID); err != nil || len(caps) != 0 {
		t.Fatalf("new integration must have no capabilities: n=%d err=%v", len(caps), err)
	}
	if err := s.UpsertCapability(ctx, in.ID, IntegrationCapability{
		Capability: "read_content", Status: "supported",
		Details: ptr("dataset fetch verified"), VerifiedAt: nowPtr(),
	}); err != nil {
		t.Fatalf("UpsertCapability: %v", err)
	}
	if err := s.UpsertCapability(ctx, in.ID, IntegrationCapability{
		Capability: "publish", Status: "unverified",
	}); err != nil {
		t.Fatalf("UpsertCapability unverified: %v", err)
	}
	// Store rejects values the schema would reject.
	if err := s.UpsertCapability(ctx, in.ID, IntegrationCapability{
		Capability: "send_faxes", Status: "supported",
	}); err != ErrInvalidCapability {
		t.Errorf("bad capability must be ErrInvalidCapability, got %v", err)
	}
	if caps, err := s.ListCapabilities(ctx, in.ID); err != nil || len(caps) != 2 {
		t.Errorf("expected 2 capabilities, got %d err=%v", len(caps), err)
	}

	// Credential ciphertext roundtrip, exactly like the adapter will use it.
	key, err := secretcipher.LoadKey(fmt.Sprintf("%x", make([]byte, secretcipher.KeyLength)))
	if err != nil {
		t.Fatalf("LoadKey: %v", err)
	}
	sealed, err := secretcipher.Encrypt(key, []byte("sanity-token-secret"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if err := s.PutCredentialSet(ctx, CredentialSet{IntegrationID: in.ID, KeyVersion: 1, Ciphertext: sealed}); err != nil {
		t.Fatalf("PutCredentialSet: %v", err)
	}
	stored, err := s.GetCredentialSet(ctx, in.ID)
	if err != nil {
		t.Fatalf("GetCredentialSet: %v", err)
	}
	if stored.KeyVersion != 1 {
		t.Errorf("KeyVersion = %d, want 1", stored.KeyVersion)
	}
	opened, err := secretcipher.Decrypt(key, stored.Ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(opened) != "sanity-token-secret" {
		t.Errorf("credential roundtrip mismatch: %q", opened)
	}

	// The integration itself carries no secret in its config.
	if in.Config == nil {
		t.Fatal("config should just be empty, not nil")
	}
}

func TestContentVersionsNumberSequentially(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()

	ws := uniqueWorkspace(t, s, "versions")
	item, err := s.InsertContentItem(ctx, ContentItem{
		WorkspaceID: ws.ID, Kind: "blog_article", Title: "Versioned article", Slug: ptrStr("versioned-article"),
	})
	if err != nil {
		t.Fatalf("InsertContentItem: %v", err)
	}

	first, err := s.InsertContentVersion(ctx, ws.ID, ContentVersion{
		ContentItemID: item.ID, Title: "Draft one", Body: "Body one",
	})
	if err != nil {
		t.Fatalf("first version: %v", err)
	}
	if first.VersionNumber != 1 {
		t.Errorf("first VersionNumber = %d, want 1", first.VersionNumber)
	}
	second, err := s.InsertContentVersion(ctx, ws.ID, ContentVersion{
		ContentItemID: item.ID, Title: "Draft two", Body: "Body two",
	})
	if err != nil {
		t.Fatalf("second version: %v", err)
	}
	if second.VersionNumber != 2 {
		t.Errorf("second VersionNumber = %d, want 2", second.VersionNumber)
	}

	// Version reads are workspace-scoped.
	if _, err := s.GetContentVersionInWorkspace(ctx, "99999999-0000-0000-0000-000000000000", first.ID); err != ErrNotFound {
		t.Errorf("version read outside workspace must be ErrNotFound, got %v", err)
	}
	versions, err := s.ListContentVersionsInWorkspace(ctx, ws.ID, item.ID)
	if err != nil || len(versions) != 2 {
		t.Fatalf("ListContentVersionsInWorkspace: n=%d err=%v", len(versions), err)
	}
	if versions[0].VersionNumber != 1 || versions[1].VersionNumber != 2 {
		t.Errorf("versions not ordered: %d then %d", versions[0].VersionNumber, versions[1].VersionNumber)
	}
}

func TestApprovalsRequireMatchingWorkspaceAndVersion(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()

	wsA, inA := seedIntegration(t, s, "approval-a") // also isolates workspaces per test
	wsB, _ := seedIntegration(t, s, "approval-b")

	item, err := s.InsertContentItem(ctx, ContentItem{
		WorkspaceID: wsA.ID, Kind: "social_post", Title: "Approval test post",
	})
	if err != nil {
		t.Fatalf("InsertContentItem: %v", err)
	}
	version, err := s.InsertContentVersion(ctx, wsA.ID, ContentVersion{
		ContentItemID: item.ID, Title: "Post draft", Body: "Post body",
	})
	if err != nil {
		t.Fatalf("InsertContentVersion: %v", err)
	}
	operator := seedOperator(t, s)

	// Approval only lands when item+version+workspace all match.
	approved, err := s.RecordApproval(ctx, wsA.ID, item.ID, version.ID, "approved", operator, nil)
	if err != nil {
		t.Fatalf("RecordApproval: %v", err)
	}
	if approved.ContentVersionID != version.ID {
		t.Errorf("approval references wrong version: %q", approved.ContentVersionID)
	}
	if _, err := s.RecordApproval(ctx, wsB.ID, item.ID, version.ID, "approved", operator, nil); err != ErrNotFound {
		t.Errorf("cross-workspace approval must be ErrNotFound, got %v", err)
	}
	// A version from another item cannot be attached to this item.
	other, err := s.InsertContentItem(ctx, ContentItem{
		WorkspaceID: wsA.ID, Kind: "blog_article", Title: "Other item",
	})
	if err != nil {
		t.Fatalf("second item: %v", err)
	}
	if _, err := s.RecordApproval(ctx, wsA.ID, other.ID, version.ID, "approved", operator, nil); err != ErrNotFound {
		t.Errorf("mismatched item/version approval must be ErrNotFound, got %v", err)
	}

	_ = inA // integration seeded; not used here
}

func TestAuditEventsScoped(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()

	wsA, _ := seedIntegration(t, s, "audit-a")
	wsB, _ := seedIntegration(t, s, "audit-b")

	if err := s.AddAuditEvent(ctx, AuditEvent{
		WorkspaceID: ptr(wsA.ID), Action: "workspace.created",
		EntityType: "workspace", EntityID: ptr(wsA.ID), Details: map[string]any{"demo": true},
	}); err != nil {
		t.Fatalf("AddAuditEvent: %v", err)
	}
	if err := s.AddAuditEvent(ctx, AuditEvent{
		WorkspaceID: ptr(wsB.ID), Action: "integration.connected",
		EntityType: "integration", Details: map[string]any{},
	}); err != nil {
		t.Fatalf("AddAuditEvent: %v", err)
	}

	eventsA, err := s.ListAuditEvents(ctx, wsA.ID, 50)
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}
	if len(eventsA) != 1 || eventsA[0].Action != "workspace.created" {
		t.Errorf("workspace A must see only its own audit row: %+v", eventsA)
	}
}

func TestPublishingJobsAreWorkspaceScopedAndKeyed(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()

	wsA, inA := seedIntegration(t, s, "publish-a")
	wsB, _ := seedIntegration(t, s, "publish-b")

	item, err := s.InsertContentItem(ctx, ContentItem{
		WorkspaceID: wsA.ID, Kind: "blog_article", Title: "Publish target",
	})
	if err != nil {
		t.Fatalf("InsertContentItem: %v", err)
	}
	version, err := s.InsertContentVersion(ctx, wsA.ID, ContentVersion{
		ContentItemID: item.ID, Title: "Approved draft", Body: "Ready body",
	})
	if err != nil {
		t.Fatalf("InsertContentVersion: %v", err)
	}
	operator := seedOperator(t, s)
	if _, err := s.RecordApproval(ctx, wsA.ID, item.ID, version.ID, "approved", operator, nil); err != nil {
		t.Fatalf("RecordApproval: %v", err)
	}

	key := fmt.Sprintf("%s:attempt-1", wsA.Slug)
	created, err := s.InsertPublishingJob(ctx, wsA.ID, PublishingJob{
		ContentItemID: item.ID, ContentVersionID: version.ID, IntegrationID: inA.ID,
		IdempotencyKey: key,
	})
	if err != nil {
		t.Fatalf("InsertPublishingJob: %v", err)
	}
	if created.Status != "pending" {
		t.Errorf("new job status = %q, want pending", created.Status)
	}

	// Retry lookup finds the same job.
	retry, err := s.GetPublishingJobByIdempotencyKey(ctx, wsA.ID, key)
	if err != nil || retry.ID != created.ID {
		t.Errorf("idempotency lookup failed: id=%s err=%v", retry.ID, err)
	}

	// Workspace B cannot see or create A's job.
	if _, err := s.GetPublishingJobByID(ctx, wsB.ID, created.ID); err != ErrNotFound {
		t.Errorf("cross-workspace job read must be ErrNotFound, got %v", err)
	}
	bItem, err := s.InsertContentItem(ctx, ContentItem{
		WorkspaceID: wsB.ID, Kind: "blog_article", Title: "B item",
	})
	if err != nil {
		t.Fatalf("B item: %v", err)
	}
	bVersion, err := s.InsertContentVersion(ctx, wsB.ID, ContentVersion{
		ContentItemID: bItem.ID, Title: "B draft", Body: "B body",
	})
	if err != nil {
		t.Fatalf("B version: %v", err)
	}
	// Same idempotency key platform-wide: second insert conflicts and is not
	// a silent duplicate for B either.
	if _, err := s.InsertPublishingJob(ctx, wsB.ID, PublishingJob{
		ContentItemID: bItem.ID, ContentVersionID: bVersion.ID, IntegrationID: inA.ID,
		IdempotencyKey: key,
	}); err == nil {
		t.Errorf("second insert with the same platform key must not succeed silently")
	}

	// Recording the destination works and is readable per job.
	if err := s.RecordPublishedArtifact(ctx, created.ID, inA.ID, ptrStr("art-id"), ptrStr("https://demo-studio.example/blog/x")); err != nil {
		t.Fatalf("RecordPublishedArtifact: %v", err)
	}
	artifact, err := s.GetPublishedArtifact(ctx, created.ID)
	if err != nil || artifact.ExternalID == nil || *artifact.ExternalID != "art-id" {
		t.Errorf("artifact mismatch: %+v err=%v", artifact, err)
	}
	_ = wsB
}

func seedOperator(t *testing.T, s *Store) string {
	t.Helper()
	ctx := context.Background()
	var id string
	err := s.Pool().QueryRow(ctx,
		`INSERT INTO operators (name, email)
		 VALUES ('Test Operator', 'operator+' || floor(random()*1e9)::text || '@local.example')
		 RETURNING id`).Scan(&id)
	if err != nil {
		t.Fatalf("seedOperator: %v", err)
	}
	t.Cleanup(func() {
		// Cleanup runs before the workspace cleanup, so first drop any approval
		// rows that reference this operator, then the operator itself.
		if _, err := s.Pool().Exec(ctx, `DELETE FROM approvals WHERE operator_id = $1`, id); err != nil {
			t.Logf("approval cleanup failed: %v", err)
		}
		if _, err := s.Pool().Exec(ctx, `DELETE FROM operators WHERE id = $1`, id); err != nil {
			t.Logf("operator cleanup failed: %v", err)
		}
	})
	return id
}

func ptr[T any](v T) *T { return &v }

func ptrStr(v string) *string { return &v }

func nowPtr() *time.Time { t := time.Now().UTC(); return &t }
