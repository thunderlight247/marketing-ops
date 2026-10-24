package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"marketingops/api/internal/integration"
	"marketingops/api/internal/secretcipher"
	"marketingops/api/internal/store"
)

func TestListAdapters(t *testing.T) {
	reg := integration.NewDefaultRegistry()
	h := NewRouter(Deps{Logger: testLogger, Integrations: reg})

	rec := doJSON(t, h, http.MethodGet, "/api/v1/integrations/adapters", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Adapters []integration.AdapterDescriptor `json:"adapters"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(body.Adapters) < 3 {
		t.Errorf("expected at least 3 adapters, got %d", len(body.Adapters))
	}
}

func TestWorkspaceIntegrations(t *testing.T) {
	wsID := "11111111-1111-1111-1111-111111111111"
	reg := integration.NewDefaultRegistry()
	testKey := make([]byte, secretcipher.KeyLength)
	for i := range testKey {
		testKey[i] = 0x55
	}

	t.Run("list returns 404 for unknown workspace", func(t *testing.T) {
		h := NewRouter(Deps{
			Logger:       testLogger,
			Integrations: reg,
			Workspaces: &fakeWorkspaceStore{
				byID: map[string]store.Workspace{},
			},
		})
		rec := doJSON(t, h, http.MethodGet, "/api/v1/workspaces/"+wsID+"/integrations", "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})

	t.Run("create rejects invalid adapterKind", func(t *testing.T) {
		h := NewRouter(Deps{
			Logger:       testLogger,
			Integrations: reg,
			Workspaces: &fakeWorkspaceStore{
				byID: map[string]store.Workspace{
					wsID: {ID: wsID, Slug: "demo", Name: "Demo"},
				},
			},
		})
		rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+wsID+"/integrations", `{"name":"My Blog","adapterKind":"unknown_kind"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "unsupported adapterKind") {
			t.Errorf("expected adapterKind error, got: %s", rec.Body.String())
		}
	})

	t.Run("create succeeds, seeds default capabilities, and logs audit event", func(t *testing.T) {
		fake := &fakeWorkspaceStore{
			byID: map[string]store.Workspace{
				wsID: {ID: wsID, Slug: "demo", Name: "Demo"},
			},
		}
		h := NewRouter(Deps{
			Logger:       testLogger,
			Integrations: reg,
			Workspaces:   fake,
		})
		rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+wsID+"/integrations", `{"name":"Nutrition Guide Sanity","adapterKind":"sanity","config":{"projectId":"y97t5m4h","dataset":"production"}}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body: %s", rec.Code, rec.Body.String())
		}
		var created integrationJSON
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if created.Name != "Nutrition Guide Sanity" || created.AdapterKind != "sanity" {
			t.Errorf("unexpected created integration: %+v", created)
		}
		if len(created.Capabilities) != 7 {
			t.Errorf("expected 7 default capabilities, got %d", len(created.Capabilities))
		}
		if len(fake.auditEvents) != 1 || fake.auditEvents[0].Action != "integration.created" {
			t.Errorf("unexpected audit events: %+v", fake.auditEvents)
		}
	})

	t.Run("save credentials encrypts secret and triggers capability check", func(t *testing.T) {
		inID := "in-123"
		fake := &fakeWorkspaceStore{
			byID: map[string]store.Workspace{
				wsID: {ID: wsID, Slug: "demo", Name: "Demo"},
			},
			integrationsByID: map[string]store.Integration{
				inID: {
					ID:          inID,
					WorkspaceID: wsID,
					AdapterKind: "sanity",
					Name:        "Sanity Blog",
					Status:      integration.LifecycleUnconfigured,
					Config:      map[string]any{"projectId": "y97t5m4h", "dataset": "production"},
				},
			},
		}
		h := NewRouter(Deps{
			Logger:               testLogger,
			Integrations:         reg,
			Workspaces:           fake,
			CredentialKey:        testKey,
			CredentialKeyVersion: 1,
		})

		rec := doJSON(t, h, http.MethodPut, "/api/v1/workspaces/"+wsID+"/integrations/"+inID+"/credentials", `{"credentials":{"token":"sk-secret-token"}}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
		}

		var updated integrationJSON
		if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if !updated.HasCredentials {
			t.Error("expected hasCredentials to be true")
		}
		if updated.Status != integration.LifecycleConnected {
			t.Errorf("expected status = connected, got %s", updated.Status)
		}

		// Ensure raw token is NEVER present in response
		if strings.Contains(rec.Body.String(), "sk-secret-token") {
			t.Error("secret token leaked in response body!")
		}

		// Verify stored ciphertext is encrypted
		credSet, ok := fake.credentialSetsByInID[inID]
		if !ok {
			t.Fatal("expected credential set to be stored")
		}
		if strings.Contains(string(credSet.Ciphertext), "sk-secret-token") {
			t.Error("raw token found in stored ciphertext")
		}

		// Verify audit event details do NOT contain secret token
		if len(fake.auditEvents) != 1 || fake.auditEvents[0].Action != "integration.credentials_updated" {
			t.Errorf("unexpected audit events: %+v", fake.auditEvents)
		}
		auditBytes, _ := json.Marshal(fake.auditEvents[0].Details)
		if strings.Contains(string(auditBytes), "sk-secret-token") {
			t.Error("secret token found in audit event details")
		}
	})

	t.Run("check integration capabilities returns report", func(t *testing.T) {
		inID := "in-456"
		fake := &fakeWorkspaceStore{
			byID: map[string]store.Workspace{
				wsID: {ID: wsID, Slug: "demo", Name: "Demo"},
			},
			integrationsByID: map[string]store.Integration{
				inID: {
					ID:          inID,
					WorkspaceID: wsID,
					AdapterKind: "manual_export",
					Name:        "Manual Checklist",
					Status:      integration.LifecycleUnconfigured,
				},
			},
		}
		h := NewRouter(Deps{
			Logger:       testLogger,
			Integrations: reg,
			Workspaces:   fake,
		})

		rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+wsID+"/integrations/"+inID+"/check", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
		}
		var report integration.CheckResult
		if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if report.LifecycleStatus != integration.LifecycleConnected {
			t.Errorf("status = %s, want connected", report.LifecycleStatus)
		}
	})

	t.Run("check names the reason when stored credentials cannot be decrypted", func(t *testing.T) {
		inID := "in-cred-bad"
		fake := &fakeWorkspaceStore{
			byID: map[string]store.Workspace{
				wsID: {ID: wsID, Slug: "demo", Name: "Demo"},
			},
			integrationsByID: map[string]store.Integration{
				inID: {
					ID:          inID,
					WorkspaceID: wsID,
					AdapterKind: "sanity",
					Name:        "Sanity Broken Creds",
					Status:      integration.LifecycleConnected,
					Config:      map[string]any{"projectId": "proj123", "dataset": "production"},
				},
			},
			// A credential set stored under a DIFFERENT key (or corrupt bytes):
			// the check must not silently pretend no credentials exist.
			credentialSetsByInID: map[string]store.CredentialSet{
				inID: {IntegrationID: inID, KeyVersion: 9, Ciphertext: []byte("poison-poison-poison-poison")},
			},
		}
		h := NewRouter(Deps{
			Logger:               testLogger,
			Integrations:         reg,
			Workspaces:           fake,
			CredentialKey:        testKey,
			CredentialKeyVersion: 1,
		})

		rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+wsID+"/integrations/"+inID+"/check", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
		}
		var report integration.CheckResult
		if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if report.LifecycleStatus != integration.LifecycleError {
			t.Errorf("status = %s, want error (credentials cannot be applied)", report.LifecycleStatus)
		}
		if !strings.Contains(report.Diagnostic, "re-enter the credentials") {
			t.Errorf("diagnostic must tell the operator the fix, got: %s", report.Diagnostic)
		}
		if strings.Contains(report.Diagnostic, "poison") {
			t.Errorf("diagnostic must not echo ciphertext material, got: %s", report.Diagnostic)
		}
		if len(fake.auditEvents) == 0 || fake.auditEvents[len(fake.auditEvents)-1].Action != "integration.checked" {
			t.Fatalf("audit event missing for check: %+v", fake.auditEvents)
		}
		var details map[string]any
		if b, _ := json.Marshal(fake.auditEvents[len(fake.auditEvents)-1].Details); json.Unmarshal(b, &details) == nil {
			if details["credentialProblem"] == "" || details["credentialProblem"] == nil {
				t.Errorf("audit event must name the credential problem, got: %s", b)
			}
		}
	})

	t.Run("delete integration removes integration and logs audit event", func(t *testing.T) {
		inID := "in-789"
		fake := &fakeWorkspaceStore{
			byID: map[string]store.Workspace{
				wsID: {ID: wsID, Slug: "demo", Name: "Demo"},
			},
			integrationsByID: map[string]store.Integration{
				inID: {
					ID:          inID,
					WorkspaceID: wsID,
					AdapterKind: "manual_export",
					Name:        "Old Export",
					Status:      integration.LifecycleConnected,
				},
			},
		}
		h := NewRouter(Deps{
			Logger:       testLogger,
			Integrations: reg,
			Workspaces:   fake,
		})

		rec := doJSON(t, h, http.MethodDelete, "/api/v1/workspaces/"+wsID+"/integrations/"+inID, "")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", rec.Code)
		}
		if _, ok := fake.integrationsByID[inID]; ok {
			t.Error("integration was not deleted from store")
		}
		if len(fake.auditEvents) != 1 || fake.auditEvents[0].Action != "integration.deleted" {
			t.Errorf("unexpected audit event: %+v", fake.auditEvents)
		}
	})
}
