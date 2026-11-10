package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"marketingops/api/internal/firecrawl"
	"marketingops/api/internal/store"
)

type fakeBrandScanner struct {
	page    firecrawl.Page
	err     error
	targets []string
}

func (f *fakeBrandScanner) Scrape(ctx context.Context, target string) (firecrawl.Page, error) {
	f.targets = append(f.targets, target)
	return f.page, f.err
}

func brandScanWorkspace(site *string) *fakeWorkspaceStore {
	return &fakeWorkspaceStore{
		byID: map[string]store.Workspace{
			"ws-1": {ID: "ws-1", Name: "Acme", WebsiteURL: site},
		},
	}
}

func runBrandScan(t *testing.T, ws WorkspaceStore, scanner BrandScanner) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/workspaces/{id}/brand-scan", postBrandScanHandler(ws, scanner))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/ws-1/brand-scan", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestBrandScanUnconfiguredAnswers501(t *testing.T) {
	site := "https://acme.example"
	rec := runBrandScan(t, brandScanWorkspace(&site), nil)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
}

func TestBrandScanReadsStoredWebsiteOnly(t *testing.T) {
	site := "https://acme.example"
	scanner := &fakeBrandScanner{page: firecrawl.Page{
		Title: "Acme Bakery", Description: "Bread in Accra", Markdown: "We bake daily.",
	}}
	rec := runBrandScan(t, brandScanWorkspace(&site), scanner)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if len(scanner.targets) != 1 || scanner.targets[0] != "https://acme.example" {
		t.Fatalf("scanned %v, want the stored website only", scanner.targets)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["source"] != "scraped" || body["title"] != "Acme Bakery" || body["truncated"] != false {
		t.Errorf("body = %v", body)
	}
}

func TestBrandScanRejectsMissingOrInvalidWebsite(t *testing.T) {
	empty := ""
	bad := "file:///etc/passwd"
	for name, site := range map[string]*string{"unset": nil, "blank": &empty, "non-http": &bad} {
		t.Run(name, func(t *testing.T) {
			scanner := &fakeBrandScanner{}
			rec := runBrandScan(t, brandScanWorkspace(site), scanner)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			if len(scanner.targets) != 0 {
				t.Fatalf("scanner was called for %v", scanner.targets)
			}
		})
	}
}

func TestBrandScanFailureDoesNotLeakDetail(t *testing.T) {
	site := "https://acme.example"
	scanner := &fakeBrandScanner{err: &firecrawl.ScrapeError{Kind: firecrawl.KindProvider, HTTP: 402}}
	rec := runBrandScan(t, brandScanWorkspace(&site), scanner)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "fc-") || strings.Contains(rec.Body.String(), "Payment") {
		t.Errorf("body leaks detail: %s", rec.Body.String())
	}
	var se *firecrawl.ScrapeError
	if !errors.As(scanner.err, &se) {
		t.Fatal("test setup")
	}
}

func TestBrandScanCapsExcerptAndNeverWritesBrand(t *testing.T) {
	site := "https://acme.example"
	ws := brandScanWorkspace(&site)
	scanner := &fakeBrandScanner{page: firecrawl.Page{Markdown: strings.Repeat("a", brandScanExcerptLimit+500)}}
	rec := runBrandScan(t, ws, scanner)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Excerpt   string `json:"excerpt"`
		Truncated bool   `json:"truncated"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len([]rune(body.Excerpt)) != brandScanExcerptLimit || !body.Truncated {
		t.Errorf("excerpt len=%d truncated=%v", len([]rune(body.Excerpt)), body.Truncated)
	}
	if ws.upsertedBrand != nil || len(ws.updated) != 0 || len(ws.auditEvents) != 0 {
		t.Error("scan wrote to the store; it must be read-only")
	}
}
