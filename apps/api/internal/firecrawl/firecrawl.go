// Package firecrawl is a read-only client for Firecrawl's scrape API. It is
// used to read a business's own public website so the operator can review
// facts before they are saved to the brand profile.
//
// Honesty rules:
//   - Scraped text is returned as evidence only. Nothing here writes to the
//     database or changes a brand field; the caller decides what to keep.
//   - Errors are classified and never include the API key or response bodies.
package firecrawl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBase is Firecrawl's hosted API root.
const DefaultBase = "https://api.firecrawl.dev"

// maxResponseBytes bounds how much of the reply is read. A scraped page can be
// large; the brand scan only needs the markdown excerpt.
const maxResponseBytes = 2 << 20

// Error kinds for ScrapeError.Kind.
const (
	KindNetwork   = "network"
	KindProvider  = "provider" // non-2xx from the API (402 credits, 401 key, ...)
	KindRateLimit = "rate_limit"
	KindMalformed = "malformed"
	KindBlocked   = "blocked" // the target site could not be read
)

// ScrapeError is a classified, credential-free failure.
type ScrapeError struct {
	Kind string
	HTTP int
}

func (e *ScrapeError) Error() string {
	switch e.Kind {
	case KindNetwork:
		return "the Firecrawl API could not be reached"
	case KindProvider:
		return fmt.Sprintf("the Firecrawl API refused the request (HTTP %d)", e.HTTP)
	case KindRateLimit:
		return "the Firecrawl API is rate limiting requests; try again shortly"
	case KindBlocked:
		return "the website could not be read"
	default:
		return "the Firecrawl reply could not be read"
	}
}

// Page is the readable content Firecrawl extracted from one URL.
type Page struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Markdown    string `json:"markdown"`
}

// Client calls the scrape endpoint with a bearer token.
type Client struct {
	APIKey     string
	Base       string // empty means DefaultBase; tests point this at a stub
	HTTPClient *http.Client
}

// Scrape reads one public page and returns its main content as markdown.
func (c Client) Scrape(ctx context.Context, target string) (Page, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return Page{}, &ScrapeError{Kind: KindProvider, HTTP: http.StatusUnauthorized}
	}
	base := c.Base
	if base == "" {
		base = DefaultBase
	}
	body, err := json.Marshal(map[string]any{
		"url":             target,
		"formats":         []string{"markdown"},
		"onlyMainContent": true,
	})
	if err != nil {
		return Page{}, &ScrapeError{Kind: KindMalformed}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(base, "/")+"/v2/scrape", bytes.NewReader(body))
	if err != nil {
		return Page{}, &ScrapeError{Kind: KindMalformed}
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return Page{}, &ScrapeError{Kind: KindNetwork}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return Page{}, &ScrapeError{Kind: KindNetwork}
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return Page{}, &ScrapeError{Kind: KindRateLimit, HTTP: resp.StatusCode}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Page{}, &ScrapeError{Kind: KindProvider, HTTP: resp.StatusCode}
	}

	var reply struct {
		Success bool `json:"success"`
		Data    struct {
			Markdown string `json:"markdown"`
			Metadata struct {
				Title       string `json:"title"`
				Description string `json:"description"`
				StatusCode  int    `json:"statusCode"`
			} `json:"metadata"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		return Page{}, &ScrapeError{Kind: KindMalformed}
	}
	if !reply.Success {
		return Page{}, &ScrapeError{Kind: KindBlocked}
	}
	if sc := reply.Data.Metadata.StatusCode; sc != 0 && (sc < 200 || sc > 299) {
		return Page{}, &ScrapeError{Kind: KindBlocked, HTTP: sc}
	}
	return Page{
		URL:         target,
		Title:       strings.TrimSpace(reply.Data.Metadata.Title),
		Description: strings.TrimSpace(reply.Data.Metadata.Description),
		Markdown:    strings.TrimSpace(reply.Data.Markdown),
	}, nil
}

// ValidateSiteURL accepts only an absolute http(s) URL with a host, so the
// scan never reaches for local or malformed targets supplied by a request.
func ValidateSiteURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("website URL must be an absolute http or https URL")
	}
	return u.String(), nil
}
