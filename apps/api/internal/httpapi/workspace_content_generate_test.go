package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"marketingops/api/internal/aiprovider"
	"marketingops/api/internal/store"
)

// strPtr is a small fixture helper (store-side tests have their own).
func strPtr(s string) *string { return &s }

// fakeGenerator answers deterministically or fails as configured.
type fakeGenerator struct {
	// result, when set, overrides the deterministic success.
	result aiprovider.GenerateResponse
	// fail, when set, is returned from Generate.
	fail error
	// calls records the requests seen.
	calls []aiprovider.GenerateRequest
}

func (f *fakeGenerator) Name() string { return "fake" }

func (f *fakeGenerator) Generate(ctx context.Context, req aiprovider.GenerateRequest) (aiprovider.GenerateResponse, error) {
	f.calls = append(f.calls, req)
	if f.fail != nil {
		return aiprovider.GenerateResponse{}, f.fail
	}
	if f.result.Provider != "" {
		return f.result, nil
	}
	return aiprovider.GenerateResponse{
		Provider: "fake", Model: "fake-1",
		Title:        "Generated title",
		Slug:         "generated-title",
		Excerpt:      "Generated excerpt",
		Body:         "Generated body",
		CallToAction: "Book now",
		ChannelCopy:  map[string]string{"instagram": "chan copy"},
		Metadata: map[string]any{
			"provider":   "fake",
			"model":      "fake-1",
			"reply_hash": "abc123",
		},
	}, nil
}

func workspaceDeps(gen ContentGenerator, brand *store.BrandProfile) *fakeWorkspaceStore {
	wsID := "11111111-1111-1111-1111-111111111111"
	fake := &fakeWorkspaceStore{
		byID: map[string]store.Workspace{
			wsID: {ID: wsID, Slug: "demo", Name: "Demo Cafe", WebsiteURL: strPtr("https://demo.example")},
		},
		contentItemsByID:      map[string]store.ContentItem{},
		contentVersionsByItem: map[string][]store.ContentVersion{},
	}
	// The fake uses the brand field from the existing struct.
	if brand != nil {
		fake.brand = brand
	}
	return fake
}

func TestGenerateWorkspaceContent(t *testing.T) {
	wsID := "11111111-1111-1111-1111-111111111111"
	brand := &store.BrandProfile{
		WorkspaceID:         wsID,
		BusinessDescription: strPtr("A small cafe"),
		Tone:                strPtr("warm"),
		ProhibitedClaims:    []string{"cures diseases"},
		CallsToAction:       []string{"Book a table"},
	}

	t.Run("returns 501 when store is nil", func(t *testing.T) {
		h := NewRouter(Deps{Logger: testLogger, AIGenerator: &fakeGenerator{}})
		rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+wsID+"/content/generate", `{"kind":"snippet","topic":"t"}`)
		if rec.Code != http.StatusNotImplemented {
			t.Fatalf("status = %d, want 501", rec.Code)
		}
	})

	t.Run("returns 501 when no generator is configured", func(t *testing.T) {
		h := NewRouter(Deps{Logger: testLogger, Workspaces: workspaceDeps(nil, nil)})
		rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+wsID+"/content/generate", `{"kind":"snippet","topic":"t"}`)
		if rec.Code != http.StatusNotImplemented {
			t.Fatalf("status = %d, want 501", rec.Code)
		}
	})

	t.Run("returns 404 for unknown workspace", func(t *testing.T) {
		h := NewRouter(Deps{Logger: testLogger, Workspaces: &fakeWorkspaceStore{
			byID: map[string]store.Workspace{},
		}, AIGenerator: &fakeGenerator{}})
		rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+wsID+"/content/generate", `{"kind":"snippet","topic":"t"}`)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})

	t.Run("rejects a bad kind and missing topic", func(t *testing.T) {
		h := NewRouter(Deps{Logger: testLogger, Workspaces: workspaceDeps(nil, nil), AIGenerator: &fakeGenerator{}})
		rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+wsID+"/content/generate", `{"kind":"podcast"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "kind must be") || !strings.Contains(rec.Body.String(), "topic is required") {
			t.Fatalf("expected validation errors, got: %s", rec.Body.String())
		}
	})

	t.Run("generates, stores a draft version 1 with ai metadata, and audits", func(t *testing.T) {
		fake := workspaceDeps(nil, brand)
		gen := &fakeGenerator{}
		h := NewRouter(Deps{Logger: testLogger, Workspaces: fake, AIGenerator: gen, GenerateTimeout: 5 * time.Second})
		rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+wsID+"/content/generate",
			`{"kind":"blog_article","topic":"Healthy breakfast","channel":"instagram","instructions":"Keep it short"}`)

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body: %s", rec.Code, rec.Body.String())
		}
		var resp struct {
			Status   string `json:"status"`
			Provider string `json:"provider"`
			Version  struct {
				VersionNumber int               `json:"versionNumber"`
				Body          string            `json:"body"`
				ChannelCopy   map[string]string `json:"channelCopy"`
				Metadata      map[string]any    `json:"metadata"`
			} `json:"version"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if resp.Status != "draft" {
			t.Fatalf("generated content must always be a draft, got %q", resp.Status)
		}
		if resp.Version.VersionNumber != 1 || resp.Version.Body != "Generated body" {
			t.Fatalf("unexpected version: %+v", resp.Version)
		}
		if resp.Version.Metadata["source"] != "ai" {
			t.Fatalf("metadata must record the ai source: %+v", resp.Version.Metadata)
		}
		if _, ok := resp.Version.Metadata["reply_hash"]; !ok {
			t.Fatalf("debug metadata retained: %+v", resp.Version.Metadata)
		}
		if resp.Version.ChannelCopy["instagram"] != "chan copy" {
			t.Fatalf("channel copy not stored: %+v", resp.Version.ChannelCopy)
		}
		if resp.Provider != "fake" {
			t.Fatalf("provider not echoed: %q", resp.Provider)
		}

		// Prompt context: the brand fields must reach the provider.
		if len(gen.calls) != 1 {
			t.Fatalf("expected 1 generate call, got %d", len(gen.calls))
		}
		call := gen.calls[0]
		if call.Workspace.Name != "Demo Cafe" || call.Workspace.Tone != "warm" {
			t.Fatalf("workspace context incomplete: %+v", call.Workspace)
		}
		if len(call.Workspace.ProhibitedClaims) != 1 || call.Workspace.ProhibitedClaims[0] != "cures diseases" {
			t.Fatalf("prohibited claims missing: %+v", call.Workspace)
		}
		if call.Topic != "Healthy breakfast" || call.Instructions != "Keep it short" {
			t.Fatalf("brief fields missing: %+v", call)
		}

		// One stored item + one stored draft + one audit event.
		if len(fake.insertedContent) != 1 || len(fake.savedDrafts) != 1 {
			t.Fatalf("expected 1 item insert and 1 draft save, got %d/%d", len(fake.insertedContent), len(fake.savedDrafts))
		}
		if len(fake.auditEvents) != 1 || fake.auditEvents[0].Action != "content.generated" {
			t.Fatalf("expected content.generated audit event, got %+v", fake.auditEvents)
		}
	})

	t.Run("fails 502 with the provider's safe message", func(t *testing.T) {
		gen := &fakeGenerator{fail: &aiprovider.ProviderError{Kind: aiprovider.FailTimeout, Cause: "deadline"}}
		h := NewRouter(Deps{Logger: testLogger, Workspaces: workspaceDeps(nil, nil), AIGenerator: gen})
		rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+wsID+"/content/generate", `{"kind":"snippet","topic":"t"}`)
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want 502", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "did not answer in time") {
			t.Fatalf("expected a safe timeout message, got: %s", rec.Body.String())
		}
	})

	t.Run("non-provider errors stay generic in the 502", func(t *testing.T) {
		gen := &fakeGenerator{fail: errors.New("boom: internal detail 10.0.0.1:53")}
		h := NewRouter(Deps{Logger: testLogger, Workspaces: workspaceDeps(nil, nil), AIGenerator: gen})
		rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+wsID+"/content/generate", `{"kind":"snippet","topic":"t"}`)
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want 502", rec.Code)
		}
		body := rec.Body.String()
		if strings.Contains(body, "boom") || strings.Contains(body, "10.0.0.1") {
			t.Fatalf("non-provider error details leaked: %s", body)
		}
		if !strings.Contains(body, "the AI provider could not be reached") {
			t.Fatalf("expected the generic provider message, got: %s", body)
		}
	})

	t.Run("conflicts 409 when the generated slug is taken", func(t *testing.T) {
		fake := workspaceDeps(nil, nil)
		fake.insertContentErr = store.ErrSlugTaken
		h := NewRouter(Deps{Logger: testLogger, Workspaces: fake, AIGenerator: &fakeGenerator{}})
		rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+wsID+"/content/generate", `{"kind":"snippet","topic":"t"}`)
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409", rec.Code)
		}
	})
}
