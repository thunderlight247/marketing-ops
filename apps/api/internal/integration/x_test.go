package integration

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// xFakeAPI is one fake X API v2 surface shared by the adapter and the
// publisher tests. The handler receives the path after the /2 segment, the
// raw body for POSTs, and the Authorization header value; unmatched routes
// answer 404 so mismatched expectations surface as adapter errors instead of
// passing silently.
func newFakeXAPI(t *testing.T, handler func(method, path, body, auth string) (int, string)) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := ""
		if r.Method == http.MethodPost {
			raw, _ := io.ReadAll(r.Body)
			body = string(raw)
		}
		status, payload := handler(r.Method, strings.TrimPrefix(r.URL.Path, "/2"), body, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, payload)
	}))
	t.Cleanup(server.Close)
	return server
}

func withFakeXOps(t *testing.T, server *httptest.Server) {
	t.Helper()
	original := xOpsConstructor
	t.Cleanup(func() { xOpsConstructor = original })
	xOpsConstructor = func(config map[string]any, accessToken string) *XOps {
		return NewXOps(config, accessToken, server.URL, nil)
	}
}

// ---- adapter check ---------------------------------------------------------

func TestXAdapterCheckWithoutTokenIsLimited(t *testing.T) {
	adapter := &XAdapter{}
	result := adapter.Check(context.Background(), map[string]any{}, nil)
	if result.LifecycleStatus != LifecycleLimited {
		t.Fatalf("expected limited without a token, got %q", result.LifecycleStatus)
	}
	for _, cap := range result.Capabilities {
		if cap.Capability == CapPublish && cap.Status != StatusUnverified {
			t.Fatalf("expected publish unverified without a token, got %+v", cap)
		}
	}
}

func TestXAdapterCheckVerifiesTokenReadonly(t *testing.T) {
	server := newFakeXAPI(t, func(method, path, body, auth string) (int, string) {
		if method != http.MethodGet || path != "/users/me" {
			t.Fatalf("the check must only read /users/me, got %s %s", method, path)
		}
		if auth != "Bearer save-me" {
			t.Fatalf("the check must send the token, got %q", auth)
		}
		return 200, `{"data":{"id":"9988776655","username":"demobakery","name":"Demo Bakery"}}`
	})
	withFakeXOps(t, server)

	adapter := &XAdapter{}
	result := adapter.Check(context.Background(), map[string]any{},
		map[string]string{"accessToken": "save-me"})
	if result.LifecycleStatus != LifecycleConnected {
		t.Fatalf("expected connected, got %q (%s)", result.LifecycleStatus, result.Diagnostic)
	}
	if !strings.Contains(result.Diagnostic, "@demobakery") || !strings.Contains(result.Diagnostic, "tweet.write") {
		t.Fatalf("diagnostic should name the handle and the scope, got %q", result.Diagnostic)
	}
	for _, cap := range result.Capabilities {
		if cap.Capability == CapPublish && cap.Status != StatusSupported {
			t.Fatalf("expected publish supported, got %+v", cap)
		}
	}
}

func TestXAdapterCheckReportsRejectedToken(t *testing.T) {
	server := newFakeXAPI(t, func(method, path, body, auth string) (int, string) {
		return 401, `{"title":"Unauthorized","detail":"Invalid or expired token"}`
	})
	withFakeXOps(t, server)

	adapter := &XAdapter{}
	result := adapter.Check(context.Background(), map[string]any{},
		map[string]string{"accessToken": "expired-token-value"})
	if result.LifecycleStatus != LifecycleError {
		t.Fatalf("expected error lifecycle for a rejected token, got %q", result.LifecycleStatus)
	}
	if !strings.Contains(result.Diagnostic, "expired") {
		t.Fatalf("diagnostic should point at the expired token, got %q", result.Diagnostic)
	}
	if strings.Contains(result.Diagnostic, "expired-token-value") {
		t.Fatalf("diagnostic leaked the token: %q", result.Diagnostic)
	}
	for _, cap := range result.Capabilities {
		if cap.Capability == CapPublish && cap.Status != StatusFailed {
			t.Fatalf("expected publish failed, got %+v", cap)
		}
	}
}

func TestXAdapterCheckNeverEchoesConfig(t *testing.T) {
	// The check hostile to secret leaks: a 500 with a server echo in the body
	// must not surface the body verbatim in the diagnostic.
	server := newFakeXAPI(t, func(method, path, body, auth string) (int, string) {
		return 500, `{"title":"Internal","detail":"boom Bearer save-me"}`
	})
	withFakeXOps(t, server)

	adapter := &XAdapter{}
	result := adapter.Check(context.Background(), map[string]any{},
		map[string]string{"accessToken": "save-me"})
	if result.LifecycleStatus != LifecycleError {
		t.Fatalf("expected error for a server failure, got %q", result.LifecycleStatus)
	}
	if strings.Contains(result.Diagnostic, "save-me") {
		t.Fatalf("diagnostic echoed the token: %q", result.Diagnostic)
	}
}

// ---- publishing ------------------------------------------------------------

func xPayload() ContentPayload {
	return ContentPayload{
		ItemID: "item-1", Version: 2,
		Title: "New pastry drop", Slug: "new-pastry-drop",
		Excerpt: "Butter, sugar, love.",
		ChannelCopy: map[string]string{
			"x": "Our March menu is live!",
		},
	}
}

func newXPublisherForTest(cfg map[string]any, token, baseURL string) *XPublisher {
	return &XPublisher{
		ops:        NewXOps(cfg, token, baseURL, nil),
		linkPrefix: "https://demo.example/blog",
	}
}

func TestXPublisherAdapterValidation(t *testing.T) {
	adapter := XPublisherAdapter{}
	if _, err := adapter.NewPublisher(map[string]any{}, nil, "", nil); !errors.Is(err, ErrPublisherCredentialsMissing) {
		t.Fatalf("expected credentials missing, got %v", err)
	}
	if _, err := adapter.NewPublisher(map[string]any{}, map[string]string{"accessToken": "  "}, "", nil); !errors.Is(err, ErrPublisherCredentialsMissing) {
		t.Fatalf("expected credentials missing for an empty token, got %v", err)
	}
}

func TestXPublisherCreateDraftHasNoDestinationDraft(t *testing.T) {
	p := newXPublisherForTest(map[string]any{}, "t", "https://unused.invalid")
	draftID, txID, err := p.CreateDraft(context.Background(), xPayload())
	if draftID != "" || txID != "" || !errors.Is(err, ErrNoDestinationDraft) {
		t.Fatalf("CreateDraft must be the honest draftless sentinel, got (%q, %q, %v)", draftID, txID, err)
	}
	var safeErr SafePublisherError
	if !errors.As(err, &safeErr) {
		t.Fatalf("the sentinel must be a SafePublisherError")
	}
}

func TestXPublisherPostsCopyWithLink(t *testing.T) {
	var tweetBody string
	var tweetAuth string
	server := newFakeXAPI(t, func(method, path, body, auth string) (int, string) {
		switch path {
		case "/users/me":
			return 200, `{"data":{"id":"9988776655","username":"demobakery","name":"Demo Bakery"}}`
		case "/users/9988776655/tweets":
			return 200, `{"data":[]}` // duplicate scan finds nothing
		case "/tweets":
			tweetBody = body
			tweetAuth = auth
			return 201, `{"data":{"id":"1600000000000000001"}}`
		default:
			return 404, `{"title":"Not Found"}`
		}
	})

	p := newXPublisherForTest(map[string]any{}, "tok", server.URL)
	outcome, err := p.Publish(context.Background(), xPayload())
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if outcome.ExternalID != "1600000000000000001" {
		t.Fatalf("unexpected external id %q", outcome.ExternalID)
	}

	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(tweetBody), &payload); err != nil {
		t.Fatalf("tweet body is not the documented JSON shape: %v (%q)", err, tweetBody)
	}
	want := "Our March menu is live!\nhttps://demo.example/blog/new-pastry-drop"
	if payload.Text != want {
		t.Fatalf("tweet text mismatch, got %q want %q", payload.Text, want)
	}
	if tweetAuth != "Bearer tok" {
		t.Fatalf("the publish must authenticate with the user token, got %q", tweetAuth)
	}

	verification := p.VerifyPublicURL(context.Background(), "https://demo.example", "new-pastry-drop")
	if !verification.Verified || verification.URL != "https://x.com/demobakery/status/1600000000000000001" {
		t.Fatalf("artifact should be the status URL, got %+v", verification)
	}
}

func TestXPublisherReusesDuplicateRecentTweet(t *testing.T) {
	tweets := 0
	server := newFakeXAPI(t, func(method, path, body, auth string) (int, string) {
		switch {
		case path == "/users/me":
			return 200, `{"data":{"id":"9988776655","username":"demobakery"}}`
		case path == "/users/9988776655/tweets":
			return 200, `{"data":[{"id":"1500000000000000000","text":"Our March menu is live!\nhttps://demo.example/blog/new-pastry-drop"}]}`
		case path == "/tweets":
			tweets++
			return 201, `{"data":{"id":"1600000000000000002"}}`
		default:
			return 404, `{"title":"Not Found"}`
		}
	})

	p := newXPublisherForTest(map[string]any{}, "tok", server.URL)
	outcome, err := p.Publish(context.Background(), xPayload())
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if outcome.ExternalID != "1500000000000000000" {
		t.Fatalf("expected the existing tweet to be reused, got %q", outcome.ExternalID)
	}
	if tweets != 0 {
		t.Fatalf("a duplicate match must not create a second tweet")
	}
	if p.VerifyPublicURL(context.Background(), "", "").URL != "https://x.com/demobakery/status/1500000000000000000" {
		t.Fatalf("the reused tweet should carry its status URL")
	}
}

func TestXPublisherFallsBackToTitleAndExcerpt(t *testing.T) {
	server := newFakeXAPI(t, func(method, path, body, auth string) (int, string) {
		switch path {
		case "/users/me":
			return 200, `{"data":{"id":"1","username":"demobakery"}}`
		case "/users/1/tweets":
			return 200, `{"data":[]}`
		case "/tweets":
			return 201, `{"data":{"id":"2"}}`
		default:
			return 404, `{"title":"Not Found"}`
		}
	})

	p := newXPublisherForTest(map[string]any{}, "tok", server.URL)
	p.linkPrefix = "" // keep the fallback text link-free
	payload := xPayload()
	payload.ChannelCopy = nil

	_, err := p.Publish(context.Background(), payload)
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}
}

func TestXPublisherRejectsEmptyMessage(t *testing.T) {
	p := newXPublisherForTest(map[string]any{}, "tok", "https://unused.invalid")
	payload := ContentPayload{}
	_, err := p.Publish(context.Background(), payload)
	var xerr *XError
	if !errors.As(err, &xerr) || xerr.Kind != "validation" {
		t.Fatalf("expected a validation error for an empty message, got %v", err)
	}
}

func TestXPublisherTruncatesToDocumentedLimit(t *testing.T) {
	copy := strings.Repeat("much", 200) // 800 runes, above the 280 cap
	payload := ContentPayload{Title: copy, ChannelCopy: map[string]string{"x": copy}}

	var posted string
	server := newFakeXAPI(t, func(method, path, body, auth string) (int, string) {
		switch path {
		case "/users/me":
			return 200, `{"data":{"id":"1","username":"u"}}`
		case "/users/1/tweets":
			return 200, `{"data":[]}`
		case "/tweets":
			var parsed struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal([]byte(body), &parsed); err != nil {
				return 400, `{"title":"Bad Request"}`
			}
			posted = parsed.Text
			return 201, `{"data":{"id":"3"}}`
		default:
			return 404, `{"title":"Not Found"}`
		}
	})

	p := newXPublisherForTest(map[string]any{}, "tok", server.URL)
	if _, err := p.Publish(context.Background(), payload); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if runes := len([]rune(posted)); runes > xTweetLimit {
		t.Fatalf("tweet exceeds the documented cap: %d runes", runes)
	}
	if posted != truncateXRunes(copy, xTweetLimit) {
		t.Fatalf("truncation should keep the first %d runes", xTweetLimit)
	}
}

func TestXPublisherMapsTokenRejection(t *testing.T) {
	server := newFakeXAPI(t, func(method, path, body, auth string) (int, string) {
		return 401, `{"title":"Unauthorized","detail":"Invalid or expired token"}`
	})
	p := newXPublisherForTest(map[string]any{}, "tok-98x2-secret", server.URL)
	_, err := p.Publish(context.Background(), xPayload())
	var xerr *XError
	if !errors.As(err, &xerr) || xerr.Kind != "token_expired" {
		t.Fatalf("expected token_expired for a 401 publish, got %v", err)
	}
	if !PublisherNeedsReconnection(err) {
		t.Fatalf("a rejected X token should mark the integration for reconnection")
	}
	if !strings.Contains(err.Error(), "reconnect") || strings.Contains(err.Error(), "tok-98x2-secret") {
		t.Fatalf("error text must be operator-safe without the credential value, got %q", err.Error())
	}
}

func TestXPublisherNetworkFailureIsNotReconnection(t *testing.T) {
	// A closed server URL still exercises the network path deterministically.
	p := newXPublisherForTest(map[string]any{}, "tok", "http://127.0.0.1:1")
	_, err := p.Publish(context.Background(), xPayload())
	var xerr *XError
	if !errors.As(err, &xerr) || xerr.Kind != "network" {
		t.Fatalf("expected a network error, got %v", err)
	}
	if PublisherNeedsReconnection(err) {
		t.Fatalf("network trouble must not require reconnection")
	}
}

func TestXErrorImplementsSafePublisherError(t *testing.T) {
	var safe SafePublisherError = &XError{Kind: "validation", Safe: "safe text"}
	if safe.Error() != "safe text" {
		t.Fatalf("SafeMessage should be the error text, got %q", safe.Error())
	}
}

func TestXDefaults(t *testing.T) {
	// Guard rails on the constants other code relies on.
	if DefaultXAPIBase == "" || xTweetLimit != 280 || xFeedScanLimit != 25 {
		t.Fatalf("X adapter constants drifted: %q %d %d", DefaultXAPIBase, xTweetLimit, xFeedScanLimit)
	}
	if _, err := url.ParseQuery("exclude=replies,retweets"); err != nil {
		t.Fatalf("sanity: url values parsing should work")
	}
}
