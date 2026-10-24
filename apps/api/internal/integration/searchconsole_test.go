package integration

// Contract tests for the Google Search Console adapter, all against a fake
// server. The real Google endpoints are never contacted in tests: GSCOps
// accepts a Base URL injection point precisely for this.

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// gscTestKey builds a throwaway RSA service-account JSON for signing tests.
func gscTestKey(t *testing.T, clientEmail string) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	raw, err := json.Marshal(map[string]string{
		"client_email": clientEmail,
		"private_key":  pemKey,
	})
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return string(raw)
}

// gscFakeServer is a stand-in for the Google OAuth + Search Console APIs.
type gscFakeServer struct {
	mu      sync.Mutex
	issued  int
	sites   map[string]string // siteURL -> permissionLevel
	queries []string          // recorded query body prefixes
}

func (f *gscFakeServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			f.mu.Lock()
			f.issued++
			f.mu.Unlock()
			if r.ParseForm() == nil && r.PostForm.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "unsupported_grant_type"})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "fake-token", "expires_in": 3600, "token_type": "Bearer",
			})
		case strings.HasPrefix(r.URL.Path, "/webmasters/v3/sites/"):
			rest := strings.TrimPrefix(r.URL.Path, "/webmasters/v3/sites/")
			if strings.HasSuffix(rest, "/searchAnalytics/query") {
				f.mu.Lock()
				defer f.mu.Unlock()
				f.queries = append(f.queries, rest[:len(rest)-len("/searchAnalytics/query")])
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"rows": []GSCQueryRow{
						{Keys: []string{"https://example.com/blog"}, Clicks: 12, Impressions: 300, Ctr: 0.04, Position: 8.5},
					},
				})
				return
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			level, ok := f.sites[rest]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"permissionLevel": level, "siteUrl": rest})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func gscNewFake(t *testing.T) (*gscFakeServer, *httptest.Server) {
	t.Helper()
	fake := &gscFakeServer{sites: map[string]string{
		"https://example.com":  "siteOwner",
		"sc-domain:other.test": "siteFullUser",
	}}
	server := httptest.NewServer(fake.handler())
	t.Cleanup(server.Close)
	return fake, server
}

// gscUseFake redirects the adapter's Check-time ops constructor to the fake
// server by setting GSCOps.Base and TokenURL; it restores the constructor on cleanup.
func gscUseFake(t *testing.T, server *httptest.Server) {
	t.Helper()
	original := gscOpsConstructor
	gscOpsConstructor = func(serviceJSON string) *GSCOps {
		return &GSCOps{ServiceAccountJSON: serviceJSON, Base: server.URL, TokenURL: server.URL + "/token"}
	}
	t.Cleanup(func() { gscOpsConstructor = original })
}

func gscConfig(property string) map[string]any {
	return map[string]any{"property": property}
}

func TestSearchConsoleCheckNotConfigured(t *testing.T) {
	adapter := &SearchConsoleAdapter{}
	result := adapter.Check(context.Background(), map[string]any{}, map[string]string{})
	if result.LifecycleStatus != LifecycleError {
		t.Fatalf("lifecycle = %q, want error", result.LifecycleStatus)
	}
	assertGSCReadOnly(t, result)
	if capStatus(result, CapAnalytics) != StatusUnsupported {
		t.Fatalf("analytics status = %q, want unsupported when property missing", capStatus(result, CapAnalytics))
	}
}

func TestSearchConsoleCheckBadProperty(t *testing.T) {
	adapter := &SearchConsoleAdapter{}
	result := adapter.Check(context.Background(), gscConfig("not-a-url"), map[string]string{})
	if result.LifecycleStatus != LifecycleError {
		t.Fatalf("lifecycle = %q, want error", result.LifecycleStatus)
	}
}

func TestSearchConsoleCheckWithoutKey(t *testing.T) {
	adapter := &SearchConsoleAdapter{}
	result := adapter.Check(context.Background(), gscConfig("https://example.com/"), map[string]string{})
	if result.LifecycleStatus != LifecycleLimited {
		t.Fatalf("lifecycle = %q, want limited", result.LifecycleStatus)
	}
	assertGSCReadOnly(t, result)
}

func TestSearchConsoleCheckBadKeyJSON(t *testing.T) {
	adapter := &SearchConsoleAdapter{}
	result := adapter.Check(context.Background(), gscConfig("https://example.com/"), map[string]string{"serviceAccount": `{not json`})
	if result.LifecycleStatus != LifecycleError {
		t.Fatalf("lifecycle = %q, want error", result.LifecycleStatus)
	}
	diag := result.Diagnostic
	if strings.Contains(diag, "BEGIN") || strings.Contains(diag, "PRIVATE") || strings.Contains(diag, "not json") {
		t.Fatalf("diagnostic leaks key material: %q", diag)
	}
}

func TestSearchConsoleCheckVerified(t *testing.T) {
	_, server := gscNewFake(t)
	gscUseFake(t, server)
	adapter := &SearchConsoleAdapter{}
	creds := map[string]string{"serviceAccount": gscTestKey(t, "svc@example.iam.gserviceaccount.com")}

	result := adapter.Check(context.Background(), gscConfig("https://example.com/"), creds)
	if result.LifecycleStatus != LifecycleConnected {
		t.Fatalf("lifecycle = %q (diag: %s), want connected", result.LifecycleStatus, result.Diagnostic)
	}
	assertGSCReadOnly(t, result)
	if capStatus(result, CapAnalytics) != StatusSupported {
		t.Fatalf("analytics status = %q (diag: %s), want supported", capStatus(result, CapAnalytics), result.Diagnostic)
	}
	if detailsFor(result, CapAnalytics) == "" {
		t.Fatalf("analytics details empty, want the permission level")
	}
}

func TestSearchConsoleCheckDomainProperty(t *testing.T) {
	_, server := gscNewFake(t)
	gscUseFake(t, server)
	adapter := &SearchConsoleAdapter{}
	creds := map[string]string{"serviceAccount": gscTestKey(t, "svc@example.iam.gserviceaccount.com")}

	result := adapter.Check(context.Background(), gscConfig("sc-domain:other.test"), creds)
	if result.LifecycleStatus != LifecycleConnected {
		t.Fatalf("lifecycle = %q (diag: %s), want connected", result.LifecycleStatus, result.Diagnostic)
	}
}

func TestSearchConsoleCheckPropertyMissing(t *testing.T) {
	_, server := gscNewFake(t)
	gscUseFake(t, server)
	adapter := &SearchConsoleAdapter{}
	creds := map[string]string{"serviceAccount": gscTestKey(t, "svc@example.iam.gserviceaccount.com")}

	result := adapter.Check(context.Background(), gscConfig("https://missing.test/"), creds)
	if result.LifecycleStatus != LifecycleLimited {
		t.Fatalf("lifecycle = %q (diag: %s), want limited", result.LifecycleStatus, result.Diagnostic)
	}
	if capStatus(result, CapAnalytics) != StatusUnverified {
		t.Fatalf("analytics status = %q, want unverified", capStatus(result, CapAnalytics))
	}
}

func TestSearchConsoleAnalyticsQuery(t *testing.T) {
	fake, server := gscNewFake(t)
	gscUseFake(t, server)
	ops := &GSCOps{ServiceAccountJSON: gscTestKey(t, "svc@example.iam.gserviceaccount.com"), Base: server.URL, TokenURL: server.URL + "/token"}

	rows, err := ops.AnalyticsQuery(context.Background(), time.Now(), "https://example.com/", "2026-10-01", "2026-10-20", []string{"page"}, 100)
	if err != nil {
		t.Fatalf("AnalyticsQuery: %v", err)
	}
	if len(rows) != 1 || rows[0].Clicks != 12 {
		t.Fatalf("rows = %+v, want one row with 12 clicks", rows)
	}
	fake.mu.Lock()
	queryCount := len(fake.queries)
	fake.mu.Unlock()
	if queryCount != 1 {
		t.Fatalf("queries recorded = %d, want 1", queryCount)
	}
}

func TestSearchConsoleTokenReuse(t *testing.T) {
	fake, server := gscNewFake(t)
	gscUseFake(t, server)
	ops := &GSCOps{ServiceAccountJSON: gscTestKey(t, "svc@example.iam.gserviceaccount.com"), Base: server.URL, TokenURL: server.URL + "/token"}
	ctx := context.Background()

	if _, err := ops.AccessToken(ctx, time.Now()); err != nil {
		t.Fatalf("first AccessToken: %v", err)
	}
	if _, err := ops.AccessToken(ctx, time.Now()); err != nil {
		t.Fatalf("second AccessToken: %v", err)
	}

	fake.mu.Lock()
	issued := fake.issued
	fake.mu.Unlock()
	if issued != 1 {
		t.Fatalf("token issued %d times, want 1 (cached)", issued)
	}
}

func TestSearchConsoleRegistryKind(t *testing.T) {
	reg := NewDefaultRegistry()
	got, ok := reg.Get("search_console")
	if !ok || got.Kind() != "search_console" {
		t.Fatal("search_console not registered in the default registry")
	}
	if _, pub := reg.Publisher("search_console"); pub {
		t.Fatal("search_console must have no publishing implementation")
	}
}

// assertGSCReadOnly enforces the read-only contract on every capability.
func assertGSCReadOnly(t *testing.T, result CheckResult) {
	t.Helper()
	for _, c := range result.Capabilities {
		switch c.Capability {
		case CapCreateDraft, CapPublish, CapUpdateContent, CapSchedule, CapLeadIntake:
			if c.Status != StatusUnsupported {
				t.Fatalf("%s status = %q, want unsupported: Search Console is read-only", c.Capability, c.Status)
			}
		}
	}
}

func capStatus(result CheckResult, capability string) string {
	for _, c := range result.Capabilities {
		if c.Capability == capability {
			return c.Status
		}
	}
	return "<missing>"
}

func detailsFor(result CheckResult, capability string) string {
	for _, c := range result.Capabilities {
		if c.Capability == capability {
			return c.Details
		}
	}
	return ""
}
