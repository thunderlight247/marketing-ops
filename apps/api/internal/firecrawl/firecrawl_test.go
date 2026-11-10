package firecrawl

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func stubServer(t *testing.T, status int, body string, gotAuth *string) Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/scrape" || r.Method != http.MethodPost {
			http.Error(w, "unexpected route", http.StatusNotFound)
			return
		}
		if gotAuth != nil {
			*gotAuth = r.Header.Get("Authorization")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return Client{APIKey: "fc-test-key", Base: srv.URL}
}

func TestScrapeReturnsMarkdownAndMetadata(t *testing.T) {
	var auth string
	c := stubServer(t, http.StatusOK,
		`{"success":true,"data":{"markdown":"  # Acme Bakery\nFresh bread daily.  ","metadata":{"title":"Acme","description":"Bakery in Accra","statusCode":200}}}`,
		&auth)
	page, err := c.Scrape(context.Background(), "https://acme.example")
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if auth != "Bearer fc-test-key" {
		t.Errorf("Authorization = %q", auth)
	}
	if page.Title != "Acme" || page.Description != "Bakery in Accra" {
		t.Errorf("metadata = %+v", page)
	}
	if !strings.HasPrefix(page.Markdown, "# Acme Bakery") {
		t.Errorf("markdown not trimmed: %q", page.Markdown)
	}
}

func TestScrapeClassifiesFailuresWithoutLeakingKey(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		kind   string
	}{
		{"payment required", http.StatusPaymentRequired, `{"success":false,"error":"Payment required"}`, KindProvider},
		{"rate limited", http.StatusTooManyRequests, `{"error":"slow down"}`, KindRateLimit},
		{"site unreadable", http.StatusOK, `{"success":false,"error":"blocked"}`, KindBlocked},
		{"site returned 404", http.StatusOK, `{"success":true,"data":{"markdown":"x","metadata":{"statusCode":404}}}`, KindBlocked},
		{"garbled reply", http.StatusOK, `not json`, KindMalformed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := stubServer(t, tc.status, tc.body, nil)
			_, err := c.Scrape(context.Background(), "https://acme.example")
			var se *ScrapeError
			if !errors.As(err, &se) || se.Kind != tc.kind {
				t.Fatalf("err = %v, want kind %s", err, tc.kind)
			}
			if strings.Contains(err.Error(), "fc-test-key") || strings.Contains(err.Error(), "Payment") {
				t.Errorf("error leaks detail: %q", err.Error())
			}
		})
	}
}

func TestScrapeRequiresKey(t *testing.T) {
	_, err := Client{Base: "http://127.0.0.1:1"}.Scrape(context.Background(), "https://acme.example")
	var se *ScrapeError
	if !errors.As(err, &se) || se.Kind != KindProvider {
		t.Fatalf("err = %v, want provider error before any request", err)
	}
}

func TestScrapeNetworkFailure(t *testing.T) {
	_, err := Client{APIKey: "k", Base: "http://127.0.0.1:1"}.Scrape(context.Background(), "https://acme.example")
	var se *ScrapeError
	if !errors.As(err, &se) || se.Kind != KindNetwork {
		t.Fatalf("err = %v, want network error", err)
	}
}

func TestValidateSiteURL(t *testing.T) {
	good := []string{"https://acme.example", "http://acme.example/about"}
	for _, raw := range good {
		if _, err := ValidateSiteURL(raw); err != nil {
			t.Errorf("ValidateSiteURL(%q) rejected: %v", raw, err)
		}
	}
	bad := []string{"", "acme.example", "ftp://acme.example", "file:///etc/passwd", "https://"}
	for _, raw := range bad {
		if _, err := ValidateSiteURL(raw); err == nil {
			t.Errorf("ValidateSiteURL(%q) accepted", raw)
		}
	}
}
