package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegistry(t *testing.T) {
	r := NewDefaultRegistry()

	t.Run("lists default adapters", func(t *testing.T) {
		descriptors := r.ListDescriptors()
		if len(descriptors) < 3 {
			t.Fatalf("expected at least 3 adapters, got %d", len(descriptors))
		}
		kinds := make(map[string]bool)
		for _, d := range descriptors {
			kinds[d.Kind] = true
		}
		if !kinds["sanity"] || !kinds["generic_api"] || !kinds["manual_export"] {
			t.Errorf("missing expected kinds in descriptors: %+v", descriptors)
		}
		if _, ok := r.Publisher("sanity"); !ok {
			t.Fatal("Sanity publishing implementation should be registered")
		}
		for _, kind := range []string{"generic_api", "manual_export"} {
			if _, ok := r.Publisher(kind); ok {
				t.Errorf("%s must not claim publish support before an implementation is registered", kind)
			}
		}
	})

	t.Run("gets registered adapter", func(t *testing.T) {
		a, ok := r.Get("sanity")
		if !ok || a == nil {
			t.Fatal("expected sanity adapter to be present")
		}
		if a.DisplayName() != "Sanity CMS" {
			t.Errorf("unexpected display name: %s", a.DisplayName())
		}
	})

	t.Run("returns false for unknown adapter", func(t *testing.T) {
		_, ok := r.Get("nonexistent")
		if ok {
			t.Fatal("expected nonexistent adapter to return false")
		}
	})
}

func TestSanityAdapter(t *testing.T) {
	adapter := &SanityAdapter{}
	ctx := context.Background()

	t.Run("fails when projectId is missing", func(t *testing.T) {
		res := adapter.Check(ctx, map[string]any{"dataset": "production"}, nil)
		if res.LifecycleStatus != LifecycleError {
			t.Errorf("status = %s, want %s", res.LifecycleStatus, LifecycleError)
		}
		if !strings.Contains(res.Diagnostic, "projectId is required") {
			t.Errorf("unexpected diagnostic: %s", res.Diagnostic)
		}
	})

	t.Run("fails when dataset is missing", func(t *testing.T) {
		res := adapter.Check(ctx, map[string]any{"projectId": "y97t5m4h"}, nil)
		if res.LifecycleStatus != LifecycleError {
			t.Errorf("status = %s, want %s", res.LifecycleStatus, LifecycleError)
		}
		if !strings.Contains(res.Diagnostic, "dataset is required") {
			t.Errorf("unexpected diagnostic: %s", res.Diagnostic)
		}
	})

	t.Run("limited mode when token is absent", func(t *testing.T) {
		res := adapter.Check(ctx, map[string]any{
			"projectId": "y97t5m4h",
			"dataset":   "production",
		}, nil)
		if res.LifecycleStatus != LifecycleLimited {
			t.Errorf("status = %s, want %s", res.LifecycleStatus, LifecycleLimited)
		}
		caps := make(map[string]string)
		for _, c := range res.Capabilities {
			caps[c.Capability] = c.Status
		}
		if caps[CapReadContent] != StatusSupported {
			t.Errorf("read_content = %s, want %s", caps[CapReadContent], StatusSupported)
		}
		if caps[CapCreateDraft] != StatusUnsupported {
			t.Errorf("create_draft = %s, want %s", caps[CapCreateDraft], StatusUnsupported)
		}
		if caps[CapPublish] != StatusUnsupported {
			t.Errorf("publish = %s, want %s", caps[CapPublish], StatusUnsupported)
		}
	})

	t.Run("connected mode when token is present", func(t *testing.T) {
		res := adapter.Check(ctx, map[string]any{
			"projectId": "y97t5m4h",
			"dataset":   "production",
		}, map[string]string{
			"token": "sk-secret-token-value",
		})
		if res.LifecycleStatus != LifecycleConnected {
			t.Errorf("status = %s, want %s", res.LifecycleStatus, LifecycleConnected)
		}
		// Ensure token was never leaked into diagnostic
		if strings.Contains(res.Diagnostic, "sk-secret-token-value") {
			t.Errorf("diagnostic contains secret token: %s", res.Diagnostic)
		}
		caps := make(map[string]string)
		for _, c := range res.Capabilities {
			caps[c.Capability] = c.Status
		}
		if caps[CapReadContent] != StatusSupported {
			t.Errorf("read_content = %s, want %s", caps[CapReadContent], StatusSupported)
		}
		if caps[CapCreateDraft] != StatusSupported {
			t.Errorf("create_draft = %s, want %s", caps[CapCreateDraft], StatusSupported)
		}
		if caps[CapPublish] != StatusSupported {
			t.Errorf("publish = %s, want %s", caps[CapPublish], StatusSupported)
		}
	})
}

func TestGenericAPIAdapter(t *testing.T) {
	adapter := &GenericAPIAdapter{}
	ctx := context.Background()

	t.Run("fails when baseUrl is missing or invalid", func(t *testing.T) {
		res := adapter.Check(ctx, map[string]any{}, nil)
		if res.LifecycleStatus != LifecycleError {
			t.Errorf("status = %s, want %s", res.LifecycleStatus, LifecycleError)
		}

		resInvalid := adapter.Check(ctx, map[string]any{"baseUrl": "not-a-url"}, nil)
		if resInvalid.LifecycleStatus != LifecycleError {
			t.Errorf("status = %s, want %s", resInvalid.LifecycleStatus, LifecycleError)
		}
	})

	t.Run("limited mode without auth", func(t *testing.T) {
		res := adapter.Check(ctx, map[string]any{"baseUrl": "https://api.example.com"}, nil)
		if res.LifecycleStatus != LifecycleLimited {
			t.Errorf("status = %s, want %s", res.LifecycleStatus, LifecycleLimited)
		}
	})

	t.Run("credentials do not imply an implemented custom publishing contract", func(t *testing.T) {
		res := adapter.Check(ctx, map[string]any{"baseUrl": "https://api.example.com"}, map[string]string{"apiKey": "secret-key"})
		if res.LifecycleStatus != LifecycleLimited {
			t.Errorf("status = %s, want %s", res.LifecycleStatus, LifecycleLimited)
		}
		if strings.Contains(res.Diagnostic, "secret-key") {
			t.Errorf("secret apiKey leaked into diagnostic: %s", res.Diagnostic)
		}
		caps := make(map[string]string)
		for _, c := range res.Capabilities {
			caps[c.Capability] = c.Status
		}
		if caps[CapPublish] != StatusUnsupported || caps[CapCreateDraft] != StatusUnsupported {
			t.Errorf("unimplemented custom write operations were overstated: %+v", caps)
		}
	})
}

func TestSanityPublisherAdapter(t *testing.T) {
	registry := NewDefaultRegistry()
	if _, ok := registry.Publisher("sanity"); !ok {
		t.Fatal("Sanity publisher was not registered")
	}
	if _, ok := registry.Publisher("generic_api"); ok {
		t.Fatal("custom API must not advertise publishing before a concrete schema adapter is implemented")
	}
	adapter := SanityPublisherAdapter{}
	if _, err := adapter.NewPublisher(map[string]any{"projectId": "p1"}, map[string]string{"token": "secret"}, "", nil); !errors.Is(err, ErrPublisherNotConfigured) {
		t.Fatalf("missing dataset error = %v", err)
	}
	if _, err := adapter.NewPublisher(map[string]any{"projectId": "p1", "dataset": "production"}, nil, "", nil); !errors.Is(err, ErrPublisherCredentialsMissing) {
		t.Fatalf("missing token error = %v", err)
	}

	publisher, err := adapter.NewPublisher(map[string]any{
		"projectId": "p1", "dataset": "production", "docType": "article",
		"publicUrlPrefix": "/articles/", "apiVersion": "v2026-01-01",
	}, map[string]string{"token": "secret"}, "http://127.0.0.1:1234", nil)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	sanityPublisher, ok := publisher.(SanityPublisher)
	if !ok {
		t.Fatalf("publisher type = %T, want SanityPublisher", publisher)
	}
	if sanityPublisher.ops.Creds.ProjectID != "p1" || sanityPublisher.ops.Creds.Dataset != "production" || sanityPublisher.ops.Creds.APIVersion != "v2026-01-01" {
		t.Errorf("Sanity configuration not passed through: %+v", sanityPublisher.ops.Creds)
	}
	if sanityPublisher.docType != "article" || sanityPublisher.urlPrefix != "/articles/" {
		t.Errorf("document settings not passed through: %+v", sanityPublisher)
	}

	var created map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if strings.Contains(r.URL.Path, "/data/doc/") {
				http.NotFound(w, r)
				return
			}
			if r.URL.Path == "/articles/a-post" {
				w.WriteHeader(http.StatusOK)
				return
			}
			http.NotFound(w, r)
			return
		}
		var envelope map[string]any
		if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
			t.Errorf("decode mutation: %v", err)
		}
		mutations := envelope["mutations"].([]any)
		created = mutations[0].(map[string]any)["createOrReplace"].(map[string]any)
		_, _ = w.Write([]byte(`{"transactionId":"contract-tx"}`))
	}))
	defer server.Close()
	publisher, err = adapter.NewPublisher(map[string]any{
		"projectId": "p1", "dataset": "production", "docType": "article",
		"publicUrlPrefix": "/articles/",
	}, map[string]string{"token": "contract-token"}, server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	payload := ContentPayload{ItemID: "item-1", Version: 4, Title: "Contract title", Slug: "a-post", Body: "hello"}
	draftID, transactionID, err := publisher.CreateDraft(context.Background(), payload)
	if err != nil || draftID != "drafts.mktops-item-1" || transactionID != "contract-tx" {
		t.Fatalf("CreateDraft = %q %q %v", draftID, transactionID, err)
	}
	if created["_type"] != "article" || created["title"] != "Contract title" {
		t.Errorf("Sanity payload mapping incorrect: %+v", created)
	}
	outcome, err := publisher.Publish(context.Background(), payload)
	if err != nil || outcome.ExternalID != "mktops-item-1" || outcome.TransactionID != "contract-tx" {
		t.Fatalf("Publish = %+v, %v", outcome, err)
	}
	verification := publisher.VerifyPublicURL(context.Background(), server.URL, "a-post")
	if !verification.Verified || verification.URL != server.URL+"/articles/a-post" || verification.HTTPStatus != http.StatusOK {
		t.Errorf("URL verification = %+v", verification)
	}
}

func TestSanityErrorSafeMessage(t *testing.T) {
	err := &SanityWriteError{Kind: SanityFailProvider, HTTP: http.StatusForbidden, Text: "secret token echoed by remote"}
	var safeErr SafePublisherError
	if !errors.As(err, &safeErr) {
		t.Fatal("Sanity errors must implement SafePublisherError")
	}
	if strings.Contains(safeErr.SafeMessage(), "secret token") || !strings.Contains(safeErr.SafeMessage(), "403") {
		t.Errorf("unsafe or unclear message: %q", safeErr.SafeMessage())
	}
}

func TestManualExportAdapter(t *testing.T) {
	adapter := &ManualExportAdapter{}
	ctx := context.Background()

	res := adapter.Check(ctx, nil, nil)
	if res.LifecycleStatus != LifecycleConnected {
		t.Errorf("status = %s, want %s", res.LifecycleStatus, LifecycleConnected)
	}
	caps := make(map[string]string)
	for _, c := range res.Capabilities {
		caps[c.Capability] = c.Status
	}
	if caps[CapPublish] != StatusSupported {
		t.Errorf("publish = %s, want %s", caps[CapPublish], StatusSupported)
	}
	if caps[CapReadContent] != StatusUnsupported {
		t.Errorf("read_content = %s, want %s", caps[CapReadContent], StatusUnsupported)
	}
}
