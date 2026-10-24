package integration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// newFakeYouTube mounts fake Google OAuth token + Data API + resumable upload
// endpoints on one server, wiring the ops constructor to it.
func newFakeYouTube(t *testing.T, handler func(t *testing.T, w http.ResponseWriter, r *http.Request)) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler(t, w, r)
	}))
	t.Cleanup(server.Close)

	original := youtubeOpsConstructor
	youtubeOpsConstructor = func(config map[string]any, creds map[string]string) *YouTubeOps {
		ops := NewYouTubeOps(config, creds, server.URL, server.URL, nil)
		return ops
	}
	t.Cleanup(func() { youtubeOpsConstructor = original })
	return server
}

var youtubeTestCreds = map[string]string{
	"refreshToken": "refresh-abc",
	"clientId":     "client-id",
	"clientSecret": "client-secret",
}

func requireTokenBody(t *testing.T, r *http.Request) {
	t.Helper()
	raw, _ := io.ReadAll(r.Body)
	form, err := url.ParseQuery(string(raw))
	if err != nil {
		t.Fatalf("token endpoint received a non-form body")
	}
	if form.Get("grant_type") != "refresh_token" || form.Get("refresh_token") != "refresh-abc" {
		t.Fatalf("unexpected token request: %v", form)
	}
	if form.Get("client_id") != "client-id" || form.Get("client_secret") != "client-secret" {
		t.Fatalf("unexpected client credentials in token request: %v", form)
	}
}

func youtubeAuthOK(w http.ResponseWriter, r *http.Request) bool {
	return r.Header.Get("Authorization") == "Bearer access-token-1"
}

// ---- adapter check ---------------------------------------------------

func TestYouTubeAdapterRejectsBadConfig(t *testing.T) {
	adapter := &YouTubeAdapter{}

	result := adapter.Check(context.Background(), map[string]any{}, nil)
	if result.LifecycleStatus != LifecycleError || !strings.Contains(result.Diagnostic, "channelId is required") {
		t.Fatalf("expected a channel id error, got %+v", result)
	}
	result = adapter.Check(context.Background(), map[string]any{"channelId": "has space"}, nil)
	if result.LifecycleStatus != LifecycleError || !strings.Contains(result.Diagnostic, "unsupported format") {
		t.Fatalf("expected a format error, got %+v", result)
	}
	result = adapter.Check(context.Background(), map[string]any{"channelId": "UCabc12345678", "privacyStatus": "secret"}, nil)
	if result.LifecycleStatus != LifecycleError || !strings.Contains(result.Diagnostic, "privacyStatus") {
		t.Fatalf("expected a privacyStatus error, got %+v", result)
	}
}

func TestYouTubeAdapterCheckWithoutCredentialsIsLimited(t *testing.T) {
	adapter := &YouTubeAdapter{}
	result := adapter.Check(context.Background(), map[string]any{"channelId": "UCabc12345678"}, nil)
	if result.LifecycleStatus != LifecycleLimited {
		t.Fatalf("expected limited without credentials, got %q", result.LifecycleStatus)
	}
	for _, cap := range result.Capabilities {
		if cap.Capability == CapPublish && cap.Status != StatusUnverified {
			t.Fatalf("expected publish unverified, got %+v", cap)
		}
	}
}

func TestYouTubeAdapterCheckVerifiesChannelReadonly(t *testing.T) {
	fake := newFakeYouTube(t, func(t *testing.T, w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			requireTokenBody(t, r)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"access-token-1","expires_in":3600,"token_type":"Bearer"}`)
			return
		}
		if r.URL.Path == "/youtube/v3/channels" {
			if got := r.URL.Query().Get("id"); got != "UCabcdef1234567890" {
				t.Fatalf("channels.list must query the configured channel id, got %q", got)
			}
			if r.URL.Query().Get("part") != "snippet" {
				t.Fatalf("channels.list must request only snippet metadata")
			}
			if !youtubeAuthOK(w, r) {
				t.Fatalf("channels.list must carry the minted bearer token")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"items":[{"id":"UCabcdef1234567890","snippet":{"title":"Demo Bakery"}}]}`)
			return
		}
		t.Fatalf("the check must not touch %s", r.URL.Path)
	})

	adapter := &YouTubeAdapter{}
	result := adapter.Check(context.Background(),
		map[string]any{"channelId": "UCabcdef1234567890"},
		youtubeTestCreds)
	if result.LifecycleStatus != LifecycleConnected {
		t.Fatalf("expected connected, got %q (%s)", result.LifecycleStatus, result.Diagnostic)
	}
	if !strings.Contains(result.Diagnostic, "Demo Bakery") || !strings.Contains(result.Diagnostic, "youtube.upload") {
		t.Fatalf("diagnostic should name the channel and the scope, got %q", result.Diagnostic)
	}
	for _, cap := range result.Capabilities {
		if cap.Capability == CapPublish && cap.Status != StatusSupported {
			t.Fatalf("expected publish supported, got %+v", cap)
		}
	}
	_ = fake
}

func TestYouTubeAdapterCheckReportsExpiredAuthorization(t *testing.T) {
	fake := newFakeYouTube(t, func(t *testing.T, w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`)
			return
		}
		t.Fatalf("no API read should happen after a rejected token exchange")
	})

	adapter := &YouTubeAdapter{}
	result := adapter.Check(context.Background(),
		map[string]any{"channelId": "UCabcdef1234567890"},
		youtubeTestCreds)
	if result.LifecycleStatus != LifecycleError {
		t.Fatalf("expected error lifecycle, got %q", result.LifecycleStatus)
	}
	if !strings.Contains(result.Diagnostic, "expired") || strings.Contains(result.Diagnostic, "refresh-abc") {
		t.Fatalf("diagnostic should point at the revoked authorization without leaking secrets, got %q", result.Diagnostic)
	}
	_ = fake
}

func TestYouTubeAdapterCheckReportsUnknownChannel(t *testing.T) {
	fake := newFakeYouTube(t, func(t *testing.T, w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = io.WriteString(w, `{"access_token":"access-token-1","token_type":"Bearer"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[]}`)
	})

	adapter := &YouTubeAdapter{}
	result := adapter.Check(context.Background(),
		map[string]any{"channelId": "UCabcdef1234567890"},
		youtubeTestCreds)
	if result.LifecycleStatus != LifecycleError || !strings.Contains(result.Diagnostic, "cannot see channel") {
		t.Fatalf("expected an honest not-visible diagnostic, got %+v", result)
	}
	_ = fake
}

// ---- publishing ------------------------------------------------------

func youtubePayload(videoURL string) ContentPayload {
	return ContentPayload{
		ItemID: "item-1", Version: 4,
		Title: "Season 2 trailer",
		Slug:  "season-2-trailer",
		ChannelCopy: map[string]string{
			"youtube": "Behind the scenes of season two.",
		},
		VideoURL: videoURL,
	}
}

func TestYouTubePublisherNewPublisherValidation(t *testing.T) {
	adapter := YouTubePublisherAdapter{}

	if _, err := adapter.NewPublisher(map[string]any{}, map[string]string{}, "", nil); !errors.Is(err, ErrPublisherNotConfigured) {
		t.Fatalf("expected not-configured without a channel id, got %v", err)
	}
	if _, err := adapter.NewPublisher(map[string]any{"channelId": "UCabcdef1234567890"}, map[string]string{"refreshToken": "r"}, "", nil); !errors.Is(err, ErrPublisherCredentialsMissing) {
		t.Fatalf("expected credentials missing, got %v", err)
	}
}

func TestYouTubePublisherCreateDraftHasNoDestinationDraft(t *testing.T) {
	p, err := YouTubePublisherAdapter{}.NewPublisher(
		map[string]any{"channelId": "UCabcdef1234567890"},
		youtubeTestCreds, "https://unused.invalid", nil)
	if err != nil {
		t.Fatalf("publisher setup failed: %v", err)
	}
	draftID, txID, err := p.CreateDraft(context.Background(), youtubePayload(""))
	if draftID != "" || txID != "" || !errors.Is(err, ErrNoDestinationDraft) {
		t.Fatalf("CreateDraft must be the honest draftless sentinel, got (%q, %q, %v)", draftID, txID, err)
	}
}

func TestYouTubePublisherRequiresVideoURL(t *testing.T) {
	fake := newFakeYouTube(t, func(t *testing.T, w http.ResponseWriter, r *http.Request) {
		t.Fatalf("no request should be made without a video file")
	})
	p, err := YouTubePublisherAdapter{}.NewPublisher(
		map[string]any{"channelId": "UCabcdef1234567890"}, youtubeTestCreds, fake.URL, nil)
	if err != nil {
		t.Fatalf("publisher setup failed: %v", err)
	}

	_, err = p.Publish(context.Background(), youtubePayload(""))
	var yerr *YouTubeError
	if !errors.As(err, &yerr) || !strings.Contains(yerr.SafeMessage(), "video file") {
		t.Fatalf("expected a safe video-required error, got %v", err)
	}
}

func TestYouTubePublisherResumableUpload(t *testing.T) {
	videoBytes := "VIDEOBYTES-" + strings.Repeat("x", 300)
	var (
		initMeta         string
		initNotify       string
		uploadBody       string
		uploadType       string
		uploadAuth       string
		uploadLen        string
		initParts        string
		sendHeaderValues = map[string]string{"X-Upload-Content-Length": "", "X-Upload-Content-Type": ""}
	)
	fake := newFakeYouTube(t, func(t *testing.T, w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			requireTokenBody(t, r)
			_, _ = io.WriteString(w, `{"access_token":"access-token-1","token_type":"Bearer"}`)
		case r.URL.Path == "/upload/youtube/v3/videos" && r.URL.Query().Get("uploadType") == "resumable":
			if r.Method != http.MethodPost {
				t.Fatalf("session init must be a POST, got %s", r.Method)
			}
			if !youtubeAuthOK(w, r) {
				t.Fatalf("session init must carry the minted bearer token")
			}
			raw, _ := io.ReadAll(r.Body)
			initMeta = string(raw)
			initNotify = r.URL.Query().Get("notifySubscribers")
			initParts = r.URL.Query().Get("part")
			sendHeaderValues["X-Upload-Content-Length"] = r.Header.Get("X-Upload-Content-Length")
			sendHeaderValues["X-Upload-Content-Type"] = r.Header.Get("X-Upload-Content-Type")
			w.Header().Set("Location", "http://"+r.Host+"/upload/session-1")
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/upload/session-1":
			if r.Method != http.MethodPut {
				t.Fatalf("the upload must be a PUT to the session URL, got %s", r.Method)
			}
			raw, _ := io.ReadAll(r.Body)
			uploadBody = string(raw)
			uploadType = r.Header.Get("Content-Type")
			uploadAuth = r.Header.Get("Authorization")
			uploadLen = strconv.FormatInt(r.ContentLength, 10)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"abc123video","kind":"youtube#video"}`)
			return
		default:
			t.Fatalf("unexpected route %s", r.URL.Path)
		}
	})
	srcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/source.mp4" {
			t.Fatalf("unexpected source route %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", strconv.Itoa(len(videoBytes)))
		_, _ = io.WriteString(w, videoBytes)
	}))
	t.Cleanup(srcServer.Close)

	p, err := YouTubePublisherAdapter{}.NewPublisher(
		map[string]any{"channelId": "UCabcdef1234567890", "notifySubscribers": true},
		youtubeTestCreds, fake.URL, nil)
	if err != nil {
		t.Fatalf("publisher setup failed: %v", err)
	}

	outcome, err := p.Publish(context.Background(), youtubePayload(srcServer.URL+"/source.mp4"))
	if err != nil {
		t.Fatalf("youtube publish failed: %v", err)
	}
	if outcome.ExternalID != "abc123video" {
		t.Fatalf("expected the uploaded video id, got %q", outcome.ExternalID)
	}
	if !strings.Contains(initMeta, `"title":"Season 2 trailer"`) || !strings.Contains(initMeta, `"description":"Behind the scenes of season two."`) {
		t.Fatalf("the session init must carry snippet metadata, got %s", initMeta)
	}
	if !strings.Contains(initMeta, `"privacyStatus":"public"`) {
		t.Fatalf("the session init must carry the status, got %s", initMeta)
	}
	if initParts != "snippet,status" {
		t.Fatalf("expected part=snippet,status, got %q", initParts)
	}
	if initNotify != "true" {
		t.Fatalf("expected notifySubscribers=true from config, got %q", initNotify)
	}
	if sendHeaderValues["X-Upload-Content-Length"] != strconv.FormatInt(int64(len(videoBytes)), 10) || sendHeaderValues["X-Upload-Content-Type"] != "video/mp4" {
		t.Fatalf("unexpected declared upload size/type: %+v", sendHeaderValues)
	}
	if uploadBody != videoBytes {
		t.Fatalf("the video bytes must stream through the upload unchanged")
	}
	if uploadType != "video/mp4" || uploadLen != strconv.Itoa(len(videoBytes)) {
		t.Fatalf("unexpected PUT content type/length: %s / %s", uploadType, uploadLen)
	}
	if !strings.Contains(uploadAuth, "Bearer access-token-1") {
		t.Fatalf("the PUT must carry the minted bearer token")
	}
	if got := p.VerifyPublicURL(context.Background(), "irrelevant", "irrelevant"); !got.Verified || got.URL != "https://www.youtube.com/watch?v=abc123video" {
		t.Fatalf("expected the composed watch URL as the artifact, got %+v", got)
	}
}

func TestYouTubePublisherSourceWithoutLengthIsRejected(t *testing.T) {
	fake := newFakeYouTube(t, func(t *testing.T, w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = io.WriteString(w, `{"access_token":"access-token-1","token_type":"Bearer"}`)
		}
	})
	srcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Write([]byte("no-le")) // force chunked transfer: flush so the server sends no Content-Length
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		_, _ = io.WriteString(w, "ngth-here")
	}))
	t.Cleanup(srcServer.Close)

	p, err := YouTubePublisherAdapter{}.NewPublisher(
		map[string]any{"channelId": "UCabcdef1234567890"}, youtubeTestCreds, fake.URL, nil)
	if err != nil {
		t.Fatalf("publisher setup failed: %v", err)
	}

	_, err = p.Publish(context.Background(), youtubePayload(srcServer.URL+"/clip.mp4"))
	var yerr *YouTubeError
	if !errors.As(err, &yerr) || !strings.Contains(yerr.SafeMessage(), "file size") {
		t.Fatalf("expected a safe size-missing error, got %v", err)
	}
}

func TestYouTubePublisherMapsQuotaExhaustion(t *testing.T) {
	fake := newFakeYouTube(t, func(t *testing.T, w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = io.WriteString(w, `{"access_token":"access-token-1","token_type":"Bearer"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"code":403,"message":"The request cannot be completed because you have exceeded your quota.","errors":[{"reason":"quotaExceeded"}]}}`)
	})
	srcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", strconv.Itoa(2048))
		_, _ = io.WriteString(w, strings.Repeat("b", 2048))
	}))
	t.Cleanup(srcServer.Close)

	p, err := YouTubePublisherAdapter{}.NewPublisher(
		map[string]any{"channelId": "UCabcdef1234567890"}, youtubeTestCreds, fake.URL, nil)
	if err != nil {
		t.Fatalf("publisher setup failed: %v", err)
	}

	_, err = p.Publish(context.Background(), youtubePayload(srcServer.URL+"/clip.mp4"))
	var yerr *YouTubeError
	if !errors.As(err, &yerr) || yerr.Kind != "rate_limited" || !strings.Contains(yerr.SafeMessage(), "quota") {
		t.Fatalf("expected a rate_limited quota error, got %v", err)
	}
}

func TestYouTubePublisherTitleTruncation(t *testing.T) {
	long := strings.Repeat("y", youtubeTitleLimit+30)
	var initMeta string
	fake := newFakeYouTube(t, func(t *testing.T, w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			_, _ = io.WriteString(w, `{"access_token":"access-token-1","token_type":"Bearer"}`)
		case r.URL.Path == "/upload/youtube/v3/videos" && r.URL.Query().Get("uploadType") == "resumable":
			raw, _ := io.ReadAll(r.Body)
			initMeta = string(raw)
			w.Header().Set("Location", "http://"+r.Host+"/upload/session-1")
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/upload/session-1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"v"}`)
		default:
			t.Fatalf("unexpected route %s", r.URL.Path)
		}
	})
	srcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", strconv.Itoa(16))
		_, _ = io.WriteString(w, strings.Repeat("v", 16))
	}))
	t.Cleanup(srcServer.Close)

	p, err := YouTubePublisherAdapter{}.NewPublisher(
		map[string]any{"channelId": "UCabcdef1234567890"}, youtubeTestCreds, fake.URL, nil)
	if err != nil {
		t.Fatalf("publisher setup failed: %v", err)
	}

	payload := youtubePayload(srcServer.URL + "/clip.mp4")
	payload.Title = long
	if _, err := p.Publish(context.Background(), payload); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if !strings.Contains(initMeta, fmt.Sprintf("%q", long[:youtubeTitleLimit])) {
		t.Fatalf("the title must be truncated to %d characters, got %s", youtubeTitleLimit, initMeta)
	}
}
