// Google Search Console read-only operations (Milestone 10): service-account
// OAuth token exchange, property verification, and search-analytics queries.
// Read-only by contract: nothing here writes, indexes, or requests
// verification of any property, and the token never reaches logs.
package integration

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// gscScope is the read-only Search Console API scope.
const gscScope = "https://www.googleapis.com/auth/webmasters.readonly"

const (
	// GSCTokenURL is Google's OAuth token endpoint for the JWT-bearer flow.
	GSCTokenURL = "https://oauth2.googleapis.com/token"
	// GSCAPIBase is the Search Console API root.
	GSCAPIBase = "https://www.googleapis.com"
)

// Failure kinds for GSCError.Kind, classified without echoing secrets.
const (
	GSCFailNetwork   = "network"
	GSCFailProvider  = "provider" // non-2xx from Google (quota, permission, ...)
	GSCFailMalformed = "malformed"
	GSCFailKey       = "bad_key" // service-account JSON is not parseable
	GSCFailTimeout   = "timeout"
)

// GSCError is a classified, credential-free failure.
type GSCError struct {
	Kind string
	HTTP int
	Text string
}

func (e *GSCError) Error() string { return e.SafeMessage() }

// SafeMessage returns a classified message that never includes response
// bodies, service-account JSON, or tokens. Text is returned only for
// provider failures and only as a short summarized line.
func (e *GSCError) SafeMessage() string {
	switch e.Kind {
	case GSCFailProvider:
		return fmt.Sprintf("the Google API refused the request (HTTP %d): %s", e.HTTP, shortenText(e.Text, 120))
	case GSCFailNetwork:
		return "the Google API could not be reached"
	case GSCFailMalformed:
		return "the Google API reply could not be read"
	case GSCFailKey:
		return "the service-account key could not be read"
	case GSCFailTimeout:
		return "the Google API did not answer in time"
	default:
		return "the Google Search Console operation failed"
	}
}

func shortenText(raw string, max int) string {
	line := raw
	if idx := bytes.IndexByte([]byte(line), '\n'); idx >= 0 {
		line = line[:idx]
	}
	line = strings.TrimSpace(line)
	if len(line) > max {
		line = line[:max]
	}
	return line
}

// ServiceAccountKey is the parsed service-account credential.
type ServiceAccountKey struct {
	ClientEmail string
	PrivateKey  *rsa.PrivateKey
}

// ParseServiceAccount reads a Google service-account JSON key file and
// extracts the client email and the RSA signing key. The JSON never reaches
// logs or error messages.
func ParseServiceAccount(raw string) (ServiceAccountKey, error) {
	var doc struct {
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil ||
		strings.TrimSpace(doc.ClientEmail) == "" || doc.PrivateKey == "" {
		return ServiceAccountKey{}, &GSCError{Kind: GSCFailKey}
	}
	key, err := gscParsePrivateKey(doc.PrivateKey)
	if err != nil {
		return ServiceAccountKey{}, &GSCError{Kind: GSCFailKey}
	}
	return ServiceAccountKey{ClientEmail: strings.TrimSpace(doc.ClientEmail), PrivateKey: key}, nil
}

// gscParsePrivateKey parses the PKCS#8 PEM body that Google issues. JSON
// transports escape the newlines as \n, so normalize first.
func gscParsePrivateKey(pemish string) (*rsa.PrivateKey, error) {
	normalized := strings.ReplaceAll(pemish, "\\n", "\n")
	block, rest := pem.Decode([]byte(normalized))
	if block == nil {
		return nil, errors.New("no PEM block")
	}
	_ = rest
	keyAny, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaKey, ok := keyAny.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("key is not RSA")
	}
	return rsaKey, nil
}

// GSCOps performs read-only Search Console operations; Base and Client are
// for tests. Token caching is per-instance and safe for concurrent use.
type GSCOps struct {
	ServiceAccountJSON string
	Scope              string // default webmasters.readonly
	Base               string // default GSCAPIBase
	TokenURL           string // default GSCTokenURL; overridable for tests
	Client             *http.Client

	mu    sync.Mutex
	token gscToken
}

// gscToken is a cached OAuth access token.
type gscToken struct {
	AccessToken string
	ExpiresAt   time.Time
}

func (s *GSCOps) apiBase() string {
	if s.Base != "" {
		return s.Base
	}
	return GSCAPIBase
}

func (s *GSCOps) scope() string {
	if s.Scope != "" {
		return s.Scope
	}
	return gscScope
}

func (s *GSCOps) tokenURL() string {
	if s.TokenURL != "" {
		return s.TokenURL
	}
	return GSCTokenURL
}

func (s *GSCOps) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return http.DefaultClient
}

func (s *GSCOps) cachedToken(now time.Time) (gscToken, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token.AccessToken != "" && now.Before(s.token.ExpiresAt) {
		return s.token, true
	}
	return gscToken{}, false
}

// AccessToken exchanges the service-account JSON for an access token
// (JWT-bearer grant), caching it until 60s before expiry.
func (s *GSCOps) AccessToken(ctx context.Context, now time.Time) (string, error) {
	if cached, ok := s.cachedToken(now); ok {
		return cached.AccessToken, nil
	}

	key, err := ParseServiceAccount(s.ServiceAccountJSON)
	if err != nil {
		return "", err
	}
	assertion, err := s.signJWT(key, now)
	if err != nil {
		return "", err
	}

	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {assertion},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", &GSCError{Kind: GSCFailNetwork}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client().Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", &GSCError{Kind: GSCFailTimeout}
		}
		return "", &GSCError{Kind: GSCFailNetwork}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", &GSCError{Kind: GSCFailMalformed}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &GSCError{Kind: GSCFailProvider, HTTP: resp.StatusCode, Text: shortenText(string(body), 120)}
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.AccessToken == "" {
		return "", &GSCError{Kind: GSCFailMalformed}
	}

	s.mu.Lock()
	s.token = gscToken{
		AccessToken: parsed.AccessToken,
		ExpiresAt:   now.Add(time.Duration(parsed.ExpiresIn-60) * time.Second),
	}
	s.mu.Unlock()
	return parsed.AccessToken, nil
}

// signJWT builds and RS256-signs the one-assertion JWT for the token exchange.
func (s *GSCOps) signJWT(key ServiceAccountKey, now time.Time) (string, error) {
	enc := base64.RawURLEncoding.EncodeToString
	header := enc([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims := fmt.Sprintf(
		`{"iss":%q,"scope":%q,"aud":%q,"exp":%d,"iat":%d}`,
		key.ClientEmail, s.scope(), s.tokenURL(), now.Add(3600*time.Second).Unix(), now.Unix())
	payload := enc([]byte(claims))
	signing := header + "." + payload

	digest := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key.PrivateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", &GSCError{Kind: GSCFailKey}
	}
	return signing + "." + enc(sig), nil
}

// GSCSiteStatus is the verification state of one property for the connected
// service account.
type GSCSiteStatus struct {
	Found   bool
	Level   string
	SiteURL string
}

// VerifySite performs a read-only check that the property exists and the
// service account holds a permission on it. The adapter by contract never
// requests or performs verification; it only reads existing state.
func (s *GSCOps) VerifySite(ctx context.Context, now time.Time, siteURL string) (GSCSiteStatus, error) {
	token, err := s.AccessToken(ctx, now)
	if err != nil {
		return GSCSiteStatus{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		s.apiBase()+"/webmasters/v3/sites/"+url.PathEscape(strings.TrimSuffix(siteURL, "/")), nil)
	if err != nil {
		return GSCSiteStatus{}, &GSCError{Kind: GSCFailNetwork}
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.client().Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return GSCSiteStatus{}, &GSCError{Kind: GSCFailTimeout}
		}
		return GSCSiteStatus{}, &GSCError{Kind: GSCFailNetwork}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return GSCSiteStatus{}, &GSCError{Kind: GSCFailMalformed}
	}
	if resp.StatusCode == http.StatusNotFound {
		return GSCSiteStatus{Found: false, SiteURL: siteURL}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return GSCSiteStatus{}, &GSCError{Kind: GSCFailProvider, HTTP: resp.StatusCode, Text: shortenText(string(body), 120)}
	}
	var parsed struct {
		PermissionLevel string `json:"permissionLevel"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return GSCSiteStatus{}, &GSCError{Kind: GSCFailMalformed}
	}
	return GSCSiteStatus{Found: true, Level: parsed.PermissionLevel, SiteURL: siteURL}, nil
}

// GSCQueryRow is one analytics aggregation row.
type GSCQueryRow struct {
	Keys        []string `json:"keys"`
	Clicks      float64  `json:"clicks"`
	Impressions float64  `json:"impressions"`
	Ctr         float64  `json:"ctr"`
	Position    float64  `json:"position"`
}

// AnalyticsQuery reads one date-range slice of search analytics for one
// property. Read-only.
func (s *GSCOps) AnalyticsQuery(ctx context.Context, now time.Time, siteURL, startDate, endDate string, dimensions []string, limit int) ([]GSCQueryRow, error) {
	if limit <= 0 || limit > 25000 {
		limit = 1000
	}
	token, err := s.AccessToken(ctx, now)
	if err != nil {
		return nil, err
	}
	request := map[string]any{
		"startDate":  startDate,
		"endDate":    endDate,
		"dimensions": dimensions,
		"rowLimit":   limit,
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return nil, &GSCError{Kind: GSCFailMalformed}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.apiBase()+"/webmasters/v3/sites/"+url.PathEscape(strings.TrimSuffix(siteURL, "/"))+"/searchAnalytics/query",
		bytes.NewReader(raw))
	if err != nil {
		return nil, &GSCError{Kind: GSCFailNetwork}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client().Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, &GSCError{Kind: GSCFailTimeout}
		}
		return nil, &GSCError{Kind: GSCFailNetwork}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, &GSCError{Kind: GSCFailMalformed}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &GSCError{Kind: GSCFailProvider, HTTP: resp.StatusCode, Text: shortenText(string(body), 120)}
	}
	var parsed struct {
		Rows []GSCQueryRow `json:"rows"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, &GSCError{Kind: GSCFailMalformed}
	}
	return parsed.Rows, nil
}
