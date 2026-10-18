package integration

import (
	"context"
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

	t.Run("connected mode with apiKey", func(t *testing.T) {
		res := adapter.Check(ctx, map[string]any{"baseUrl": "https://api.example.com"}, map[string]string{"apiKey": "secret-key"})
		if res.LifecycleStatus != LifecycleConnected {
			t.Errorf("status = %s, want %s", res.LifecycleStatus, LifecycleConnected)
		}
		if strings.Contains(res.Diagnostic, "secret-key") {
			t.Errorf("secret apiKey leaked into diagnostic: %s", res.Diagnostic)
		}
	})
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
