package aiprovider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStubGenerate(t *testing.T) {
	stub := StubProvider{}
	resp, err := stub.Generate(context.Background(), GenerateRequest{
		Kind: "blog_article", Topic: "Healthy Ghanaian Foods",
		Workspace: WorkspaceContext{Name: "Demo Cafe", CallsToAction: []string{"Book a table"}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Provider != "stub" || !strings.Contains(resp.Title, "Healthy Ghanaian Foods") {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.CallToAction != "Book a table" {
		t.Fatalf("stub must reuse the brand CTA, got %q", resp.CallToAction)
	}
	if resp.Slug != "healthy-ghanaian-foods" {
		t.Fatalf("slugify: %q", resp.Slug)
	}

	t.Run("rejects unsupported kind", func(t *testing.T) {
		_, err := stub.Generate(context.Background(), GenerateRequest{Kind: "podcast", Topic: "x"})
		var pe *ProviderError
		if !errors.As(err, &pe) || pe.Kind != FailUnsupported {
			t.Fatalf("expected FailUnsupported, got %v", err)
		}
	})

	t.Run("rejects empty topic", func(t *testing.T) {
		_, err := stub.Generate(context.Background(), GenerateRequest{Kind: "snippet", Topic: "  "})
		if !errors.As(err, new(*ProviderError)) || !strings.Contains(err.Error(), "no usable content") {
			t.Fatalf("expected FailEmpty, got %v", err)
		}
	})
}

func TestOpenRouterClassification(t *testing.T) {
	makeServer := func(handler http.HandlerFunc) *httptest.Server {
		srv := httptest.NewServer(handler)
		t.Cleanup(srv.Close)
		return srv
	}

	provider := func(base string) OpenRouter {
		return OpenRouter{APIKey: "test-key", Model: "model-x", Base: base}
	}

	t.Run("parses a fenced JSON draft", func(t *testing.T) {
		content := "Sure! Here it is:\n```json\n{\"title\":\"Big Title\",\"slug\":\"Big Title\",\"excerpt\":\"e\",\"body\":\"body text\",\"callToAction\":\"CTA\",\"channelCopy\":{\"Instagram\":\"story copy\"}}\n```"
		srv := makeServer(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/chat/completions" {
				t.Errorf("unexpected path %s", r.URL.Path)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
				t.Errorf("missing bearer header")
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":` + mustJSON(t, content) + `}}]}`))
		})
		resp, err := provider(srv.URL).Generate(context.Background(), GenerateRequest{
			Kind: "blog_article", Topic: "t", Workspace: WorkspaceContext{Name: "ACME"},
		})
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if resp.Title != "Big Title" || resp.Body != "body text" {
			t.Fatalf("unexpected draft: %+v", resp)
		}
		if resp.Slug != "big-title" {
			t.Fatalf("slug should be normalized: %q", resp.Slug)
		}
		if resp.ChannelCopy["instagram"] != "story copy" {
			t.Fatalf("channel copy keys must be normalized: %+v", resp.ChannelCopy)
		}
		if resp.Metadata["model"] != "model-x" {
			t.Fatalf("metadata must record the model: %+v", resp.Metadata)
		}
	})

	t.Run("classifies 401 without echoing the key", func(t *testing.T) {
		srv := makeServer(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "unauthorized, key sk-test-key-123 was rejected", http.StatusUnauthorized)
		})
		_, err := provider(srv.URL).Generate(context.Background(), GenerateRequest{Kind: "snippet", Topic: "t"})
		var pe *ProviderError
		if !errors.As(err, &pe) || pe.Kind != FailProvider {
			t.Fatalf("expected FailProvider, got %v", err)
		}
		if strings.Contains(err.Error(), "sk-test-key-123") {
			t.Fatalf("error text leaked a key fragment: %q", err.Error())
		}
	})

	t.Run("classifies a malformed reply", func(t *testing.T) {
		srv := makeServer(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":` + mustJSON(t, "no json here") + `}}]}`))
		})
		_, err := provider(srv.URL).Generate(context.Background(), GenerateRequest{Kind: "snippet", Topic: "t"})
		if !errors.As(err, new(*ProviderError)) || !strings.Contains(err.Error(), "could not be structured") {
			t.Fatalf("expected FailMalformed, got %v", err)
		}
	})

	t.Run("classifies an empty body as empty", func(t *testing.T) {
		srv := makeServer(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":` + mustJSON(t, `{"title":"t","body":"  "}`) + `}}]}`))
		})
		_, err := provider(srv.URL).Generate(context.Background(), GenerateRequest{Kind: "snippet", Topic: "t"})
		if !errors.As(err, new(*ProviderError)) || !strings.Contains(err.Error(), "no usable content") {
			t.Fatalf("expected FailEmpty, got %v", err)
		}
	})

	t.Run("classifies a timeout", func(t *testing.T) {
		srv := makeServer(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(3 * time.Second):
			}
		})
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, err := provider(srv.URL).Generate(ctx, GenerateRequest{Kind: "snippet", Topic: "t"})
		if !errors.As(err, new(*ProviderError)) || !strings.Contains(err.Error(), "in time") {
			t.Fatalf("expected FailTimeout, got %v", err)
		}
	})

	t.Run("classifies a network failure", func(t *testing.T) {
		srv := makeServer(func(w http.ResponseWriter, r *http.Request) {})
		srv.Close() // closed immediately: connection refused
		_, err := provider(srv.URL).Generate(context.Background(), GenerateRequest{Kind: "snippet", Topic: "t"})
		if !errors.As(err, new(*ProviderError)) || !strings.Contains(err.Error(), "could not be reached") {
			t.Fatalf("expected FailNetwork, got %v", err)
		}
	})
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return string(raw)
}
