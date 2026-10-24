package integration

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// fakeGraph is one fake Graph API surface shared by the adapter and the
// publisher tests. The handler receives the final path after the version
// segment plus the decoded query or form values; unmatched routes answer 404
// so mismatched expectations surface as adapter errors instead of passing
// silently.
func newFakeGraph(t *testing.T, handler func(t *testing.T, path string, values url.Values, body string) (int, string)) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		values := r.URL.Query()
		body := ""
		if r.Method == http.MethodPost {
			raw, _ := io.ReadAll(r.Body)
			body = string(raw)
			form, err := url.ParseQuery(body)
			if err == nil {
				values = form
			}
		}
		status, payload := handler(t, strings.TrimPrefix(r.URL.Path, "/v21.0/"), values, body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, payload)
	}))
	t.Cleanup(server.Close)
	return server
}

// ---- adapter check ---------------------------------------------------

func TestMetaAdapterRejectsBadConfig(t *testing.T) {
	adapter := &MetaAdapter{}

	result := adapter.Check(context.Background(), map[string]any{"channel": "twitter", "targetId": "12345"}, nil)
	if result.LifecycleStatus != LifecycleError || !strings.Contains(result.Diagnostic, "channel") {
		t.Fatalf("expected a channel configuration error, got %+v", result)
	}

	result = adapter.Check(context.Background(), map[string]any{"channel": "facebook", "targetId": "  "}, nil)
	if result.LifecycleStatus != LifecycleError || !strings.Contains(result.Diagnostic, "targetId is required") {
		t.Fatalf("expected a targetId error, got %+v", result)
	}

	result = adapter.Check(context.Background(), map[string]any{"channel": "facebook", "targetId": "has space"}, nil)
	if result.LifecycleStatus != LifecycleError || !strings.Contains(result.Diagnostic, "targetId has an unsupported format") {
		t.Fatalf("expected a targetId format error, got %+v", result)
	}
}

func TestMetaAdapterCheckWithoutTokenIsLimited(t *testing.T) {
	adapter := &MetaAdapter{}
	result := adapter.Check(context.Background(),
		map[string]any{"channel": "facebook", "targetId": "123456789"}, nil)
	if result.LifecycleStatus != LifecycleLimited {
		t.Fatalf("expected limited without a token, got %q", result.LifecycleStatus)
	}
	for _, cap := range result.Capabilities {
		if cap.Capability == CapPublish && cap.Status != StatusUnverified {
			t.Fatalf("expected publish unverified without a token, got %+v", cap)
		}
	}
}

func TestMetaAdapterCheckVerifiesTargetReadonly(t *testing.T) {
	original := metaOpsConstructor
	defer func() { metaOpsConstructor = original }()

	fake := newFakeGraph(t, func(t *testing.T, path string, values url.Values, body string) (int, string) {
		if path != "123456789" {
			return 500, `{}` // the check must not touch any other route
		}
		if values.Get("access_token") == "" {
			t.Fatalf("the check must send the token")
		}
		return 200, `{"id":"123456789","name":"Demo Bakery Page"}`
	})
	metaOpsConstructor = func(config map[string]any, accessToken string) *MetaOps {
		return NewMetaOps(config, accessToken, fake.URL, nil)
	}

	adapter := &MetaAdapter{}
	result := adapter.Check(context.Background(),
		map[string]any{"channel": "facebook", "targetId": "123456789"},
		map[string]string{"accessToken": "tok"})
	if result.LifecycleStatus != LifecycleConnected {
		t.Fatalf("expected connected, got %q (%s)", result.LifecycleStatus, result.Diagnostic)
	}
	if !strings.Contains(result.Diagnostic, "Demo Bakery Page") || !strings.Contains(result.Diagnostic, "pages_manage_posts") {
		t.Fatalf("diagnostic should name the target and the permission, got %q", result.Diagnostic)
	}
	for _, cap := range result.Capabilities {
		if cap.Capability == CapPublish && cap.Status != StatusSupported {
			t.Fatalf("expected publish supported, got %+v", cap)
		}
	}
}

func TestMetaAdapterCheckReportsRejectedToken(t *testing.T) {
	original := metaOpsConstructor
	defer func() { metaOpsConstructor = original }()

	fake := newFakeGraph(t, func(t *testing.T, path string, values url.Values, body string) (int, string) {
		return 401, `{"error":{"message":"The access token could not be validated","type":"OAuthException","code":190}}`
	})
	metaOpsConstructor = func(config map[string]any, accessToken string) *MetaOps {
		return NewMetaOps(config, accessToken, fake.URL, nil)
	}

	adapter := &MetaAdapter{}
	result := adapter.Check(context.Background(),
		map[string]any{"channel": "facebook", "targetId": "123456789"},
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

// ---- publishing ------------------------------------------------------

func metaPayload() ContentPayload {
	return ContentPayload{
		ItemID: "item-1", Version: 3,
		Title: "New pastry drop", Slug: "new-pastry-drop",
		Excerpt: "Butter, sugar, love.",
		ChannelCopy: map[string]string{
			"facebook":  "Our March menu is live!",
			"instagram": "March menu live — link in bio.",
		},
	}
}

func newMetaPublisherForTest(cfg map[string]any, token, baseURL string) *MetaPublisher {
	return &MetaPublisher{
		ops:        NewMetaOps(cfg, token, baseURL, nil),
		linkPrefix: "https://demo.example/blog",
	}
}

func TestMetaPublisherNewPublisherValidation(t *testing.T) {
	adapter := MetaPublisherAdapter{}

	if _, err := adapter.NewPublisher(map[string]any{"channel": "twitter", "targetId": "1"}, map[string]string{"accessToken": "t"}, "", nil); !errors.Is(err, ErrPublisherNotConfigured) {
		t.Fatalf("expected not-configured for a bad channel, got %v", err)
	}
	if _, err := adapter.NewPublisher(map[string]any{"channel": "facebook", "targetId": "  "}, map[string]string{"accessToken": "t"}, "", nil); !errors.Is(err, ErrPublisherNotConfigured) {
		t.Fatalf("expected not-configured without a target, got %v", err)
	}
	if _, err := adapter.NewPublisher(map[string]any{"channel": "facebook", "targetId": "123"}, nil, "", nil); !errors.Is(err, ErrPublisherCredentialsMissing) {
		t.Fatalf("expected credentials missing, got %v", err)
	}
}

func TestMetaPublisherCreateDraftHasNoDestinationDraft(t *testing.T) {
	p := newMetaPublisherForTest(map[string]any{"channel": "facebook", "targetId": "123456789"}, "t", "https://unused.invalid")
	draftID, txID, err := p.CreateDraft(context.Background(), metaPayload())
	if draftID != "" || txID != "" || !errors.Is(err, ErrNoDestinationDraft) {
		t.Fatalf("CreateDraft must be the honest draftless sentinel, got (%q, %q, %v)", draftID, txID, err)
	}
	var safeErr SafePublisherError
	if !errors.As(err, &safeErr) {
		t.Fatalf("the sentinel must be a SafePublisherError")
	}
}

func TestMetaPublisherFacebookPostsMessageAndLink(t *testing.T) {
	var feedWrite url.Values
	fake := newFakeGraph(t, func(t *testing.T, path string, values url.Values, body string) (int, string) {
		switch {
		case strings.HasPrefix(path, "123456789/feed") && values.Get("fields") == "id,message,link":
			return 200, `{"data":[]}` // duplicate scan finds nothing
		case strings.HasPrefix(path, "123456789/feed") && values.Get("message") != "":
			feedWrite = values
			return 200, `{"id":"123456789_999888777"}`
		case path == "123456789_999888777":
			return 200, `{"permalink":"https://www.facebook.com/demo/posts/999888777"}`
		default:
			return 404, `{"error":{"message":"unexpected route","type":"GraphMethodException","code":100}}`
		}
	})

	p := newMetaPublisherForTest(map[string]any{"channel": "facebook", "targetId": "123456789"}, "tok", fake.URL)

	outcome, err := p.Publish(context.Background(), metaPayload())
	if err != nil {
		t.Fatalf("facebook publish failed: %v", err)
	}
	if outcome.ExternalID != "123456789_999888777" {
		t.Fatalf("expected the post id, got %q", outcome.ExternalID)
	}
	if feedWrite.Get("message") != "Our March menu is live!" {
		t.Fatalf("the feed post must carry the facebook channel copy, got %q", feedWrite.Get("message"))
	}
	if feedWrite.Get("link") != "https://demo.example/blog/new-pastry-drop" {
		t.Fatalf("the feed post must link the article via the prefix, got %q", feedWrite.Get("link"))
	}
	verification := p.VerifyPublicURL(context.Background(), "irrelevant", "irrelevant")
	if !verification.Verified || verification.URL != "https://www.facebook.com/demo/posts/999888777" {
		t.Fatalf("expected the permalink as the artifact URL, got %+v", verification)
	}
}

func TestMetaPublisherFacebookReusesExistingPost(t *testing.T) {
	feedWrites := 0
	fake := newFakeGraph(t, func(t *testing.T, path string, values url.Values, body string) (int, string) {
		switch {
		case strings.HasPrefix(path, "123456789/feed") && values.Get("fields") == "id,message,link":
			return 200, `{"data":[{"id":"123456789_111","message":"Our March menu is live!","link":"https://demo.example/blog/new-pastry-drop"}]}`
		case strings.HasPrefix(path, "123456789/feed"):
			feedWrites++
			return 200, `{"id":"123456789_222"}`
		case path == "123456789_111":
			return 200, `{"permalink":"https://www.facebook.com/demo/posts/111"}`
		default:
			return 404, `{}`
		}
	})

	p := newMetaPublisherForTest(map[string]any{"channel": "facebook", "targetId": "123456789"}, "tok", fake.URL)

	outcome, err := p.Publish(context.Background(), metaPayload())
	if err != nil {
		t.Fatalf("facebook publish failed: %v", err)
	}
	if outcome.ExternalID != "123456789_111" || feedWrites != 0 {
		t.Fatalf("the existing matching post must be reused without a new write (id %q, writes %d)", outcome.ExternalID, feedWrites)
	}
}

func TestMetaPublisherFacebookMapsRateLimit(t *testing.T) {
	fake := newFakeGraph(t, func(t *testing.T, path string, values url.Values, body string) (int, string) {
		if strings.HasPrefix(path, "123456789/feed") && values.Get("fields") == "id,message,link" {
			return 200, `{"data":[]}`
		}
		return 400, `{"error":{"message":"(#4) Application request limit reached","type":"OAuthException","code":4}}`
	})
	p := newMetaPublisherForTest(map[string]any{"channel": "facebook", "targetId": "123456789"}, "tok", fake.URL)

	_, err := p.Publish(context.Background(), metaPayload())
	var merr *MetaError
	if !errors.As(err, &merr) || merr.Kind != "rate_limited" {
		t.Fatalf("expected a rate-limited MetaError, got %v", err)
	}
	if !strings.Contains(merr.SafeMessage(), "rate limit") {
		t.Fatalf("the safe message should mention rate limiting, got %q", merr.SafeMessage())
	}
}

func TestMetaPublisherInstagramRequiresPublicImage(t *testing.T) {
	fake := newFakeGraph(t, func(t *testing.T, path string, values url.Values, body string) (int, string) {
		t.Fatalf("no request should be made without a public image")
		return 500, `{}`
	})
	p := newMetaPublisherForTest(map[string]any{"channel": "instagram", "targetId": "17841400000000000"}, "tok", fake.URL)

	_, err := p.Publish(context.Background(), metaPayload())
	var merr *MetaError
	if !errors.As(err, &merr) || !strings.Contains(merr.SafeMessage(), "image") {
		t.Fatalf("expected a safe image error, got %v", err)
	}
}

func TestMetaPublisherInstagramTwoStepPublish(t *testing.T) {
	var containerValues, publishValues url.Values
	fake := newFakeGraph(t, func(t *testing.T, path string, values url.Values, body string) (int, string) {
		switch {
		case path == "17841400000000000/media":
			containerValues = values
			return 200, `{"id":"container-1"}`
		case path == "17841400000000000/media_publish":
			publishValues = values
			return 200, `{"id":"ig-media-9"}`
		case path == "ig-media-9":
			return 200, `{"permalink":"https://www.instagram.com/p/Cabc123/"}`
		default:
			return 404, `{}`
		}
	})
	p := newMetaPublisherForTest(map[string]any{"channel": "instagram", "targetId": "17841400000000000"}, "tok", fake.URL)

	payload := metaPayload()
	payload.MainImage = &PayloadMainImage{Ref: "image-abc", URL: "https://cdn.example/march.jpg", Alt: "pastries"}
	outcome, err := p.Publish(context.Background(), payload)
	if err != nil {
		t.Fatalf("instagram publish failed: %v", err)
	}
	if outcome.ExternalID != "ig-media-9" {
		t.Fatalf("expected the published media id, got %q", outcome.ExternalID)
	}
	if containerValues.Get("image_url") != "https://cdn.example/march.jpg" {
		t.Fatalf("the container must fetch the public image URL, got %q", containerValues.Get("image_url"))
	}
	if containerValues.Get("caption") != "March menu live — link in bio." {
		t.Fatalf("the container must carry the instagram channel copy, got %q", containerValues.Get("caption"))
	}
	if publishValues.Get("creation_id") != "container-1" {
		t.Fatalf("media_publish must reference the container, got %q", publishValues.Get("creation_id"))
	}
	if p.VerifyPublicURL(context.Background(), "", "").URL != "https://www.instagram.com/p/Cabc123/" {
		t.Fatalf("the permalink must be recorded as the artifact URL")
	}
}

func TestMetaPublisherInstagramCaptionTruncation(t *testing.T) {
	long := strings.Repeat("x", 3000)
	var caption string
	fake := newFakeGraph(t, func(t *testing.T, path string, values url.Values, body string) (int, string) {
		switch {
		case path == "17841400000000000/media":
			caption = values.Get("caption")
			return 200, `{"id":"c"}`
		case path == "17841400000000000/media_publish":
			return 200, `{"id":"m"}`
		default:
			return 404, `{}`
		}
	})
	p := newMetaPublisherForTest(map[string]any{"channel": "instagram", "targetId": "17841400000000000"}, "tok", fake.URL)

	payload := metaPayload()
	payload.ChannelCopy = map[string]string{"instagram": long}
	payload.MainImage = &PayloadMainImage{URL: "https://cdn.example/x.jpg"}
	if _, err := p.Publish(context.Background(), payload); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if got := len([]rune(caption)); got != metaIGCaptionLimit {
		t.Fatalf("the caption must be truncated to %d runes, got %d", metaIGCaptionLimit, got)
	}
}

func TestMetaPublisherFacebookFallsBackToTitleExcerpt(t *testing.T) {
	var message string
	fake := newFakeGraph(t, func(t *testing.T, path string, values url.Values, body string) (int, string) {
		switch {
		case strings.HasPrefix(path, "123456789/feed") && values.Get("fields") == "id,message,link":
			return 200, `{"data":[]}`
		case strings.HasPrefix(path, "123456789/feed"):
			message = values.Get("message")
			return 200, `{"id":"1"}`
		case path == "1":
			return 200, `{"permalink":"https://www.facebook.com/x"}`
		default:
			return 404, `{}`
		}
	})
	p := newMetaPublisherForTest(map[string]any{"channel": "facebook", "targetId": "123456789"}, "tok", fake.URL)

	payload := metaPayload()
	payload.ChannelCopy = nil
	if _, err := p.Publish(context.Background(), payload); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if !strings.Contains(message, "New pastry drop") || !strings.Contains(message, "Butter, sugar, love.") {
		t.Fatalf("expected fallback to title + excerpt, got %q", message)
	}
}
