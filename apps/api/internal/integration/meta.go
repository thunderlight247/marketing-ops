package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Meta social adapter (Milestone 8). It targets ONE official Graph API surface
// per integration: a Facebook Page feed or an Instagram Business account
// connected to that Page. Access always uses an app-authorised access token
// (pages_manage_posts for Facebook, instagram_content_publish for Instagram);
// the adapter never scrapes, never logs in with passwords, and never touches
// unofficial endpoints.
//
// Honesty rules encoded below:
//   - automation exists only where Meta documents it, so an integration whose
//     token is expired or revoked is reported failed rather than assumed;
//   - the "publish" capability means approval-gated post creation through
//     /feed or the two-step Instagram container call. There is no
//     destination-side draft concept: preview happens in this dashboard, and
//     publishing jobs scheduled in this platform run by its own worker.
const (
	DefaultGraphAPIVersion = "v21.0"
	defaultGraphBase       = "https://graph.facebook.com"

	MetaChannelFacebook  = "facebook"
	MetaChannelInstagram = "instagram"

	// metaIGCaptionLimit is the documented caption cap for API-published
	// Instagram posts.
	metaIGCaptionLimit = 2200
	// metaFeedScanLimit bounds the read used for duplicate prevention: the
	// publisher scans this many of the Page's most recent posts before
	// creating a new one.
	metaFeedScanLimit = 25
)

var metaTargetPattern = regexp.MustCompile(`^[0-9a-zA-Z._-]{3,100}$`)

// metaOpsConstructor is swappable so tests can point Check at a fake server.
var metaOpsConstructor = func(config map[string]any, accessToken string) *MetaOps {
	return NewMetaOps(config, accessToken, defaultGraphBase, nil)
}

// MetaAdapter implements Adapter for Meta Graph API surfaces.
type MetaAdapter struct{}

func (a *MetaAdapter) Kind() string        { return "meta" }
func (a *MetaAdapter) DisplayName() string { return "Meta (Facebook Page / Instagram)" }

func (a *MetaAdapter) Description() string {
	return "Official Graph API publishing to a Facebook Page feed or an Instagram Business account"
}

func (a *MetaAdapter) DefaultCapabilities() []CapabilityReport {
	return []CapabilityReport{
		{Capability: CapReadContent, Status: StatusUnsupported, Details: "Social platforms are not content storage for articles; the website/CMS integration keeps the article"},
		{Capability: CapCreateDraft, Status: StatusUnsupported, Details: "Meta has no external draft step; previews happen in this dashboard before approval"},
		{Capability: CapPublish, Status: StatusUnverified, Details: "Requires an access token with the channel's manage-posts permission"},
		{Capability: CapUpdateContent, Status: StatusUnsupported, Details: "Published posts are not edited back from here"},
		{Capability: CapSchedule, Status: StatusUnsupported, Details: "Scheduled publishes queue in this platform's scheduler and run at their due time"},
		{Capability: CapAnalytics, Status: StatusUnsupported, Details: "Post insights are out of scope for this adapter"},
		{Capability: CapLeadIntake, Status: StatusUnsupported, Details: "Not an enquiry endpoint"},
	}
}

func (a *MetaAdapter) Check(ctx context.Context, config map[string]any, creds map[string]string) CheckResult {
	now := time.Now().UTC()

	channel, _ := config["channel"].(string)
	channel = strings.TrimSpace(strings.ToLower(channel))
	if channel != MetaChannelFacebook && channel != MetaChannelInstagram {
		return CheckResult{
			LifecycleStatus: LifecycleError,
			Diagnostic:      "Configuration error: channel must be \"facebook\" or \"instagram\"",
			CheckedAt:       now,
			Capabilities:    a.DefaultCapabilities(),
		}
	}

	targetID, _ := config["targetId"].(string)
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		what := "Facebook Page ID"
		if channel == MetaChannelInstagram {
			what = "Instagram business account ID"
		}
		return CheckResult{
			LifecycleStatus: LifecycleError,
			Diagnostic:      "Configuration error: targetId is required (the " + what + ")",
			CheckedAt:       now,
			Capabilities:    a.DefaultCapabilities(),
		}
	}
	if !metaTargetPattern.MatchString(targetID) {
		return CheckResult{
			LifecycleStatus: LifecycleError,
			Diagnostic:      "Configuration error: targetId has an unsupported format",
			CheckedAt:       now,
			Capabilities:    a.DefaultCapabilities(),
		}
	}

	token := strings.TrimSpace(creds["accessToken"])
	publishCap := func(status, details string) CapabilityReport {
		return CapabilityReport{Capability: CapPublish, Status: status, Details: details}
	}
	limited := func(diagnostic, capStatus, capDetails string) CheckResult {
		base := a.DefaultCapabilities()
		base[2] = publishCap(capStatus, capDetails)
		return CheckResult{
			LifecycleStatus: LifecycleLimited,
			Diagnostic:      diagnostic,
			CheckedAt:       now,
			Capabilities:    base,
		}
	}
	if token == "" {
		return limited(fmt.Sprintf("Channel %s target %q is configured, but no access token has been saved, so publishing access could not be verified.", channel, targetID),
			StatusUnverified, "Waiting for the access token")
	}

	ops := metaOpsConstructor(config, token)
	name, checkErr := ops.VerifyTarget(ctx)
	if checkErr != nil {
		diag := "the Meta API could not be reached"
		var merr *MetaError
		if errors.As(checkErr, &merr) {
			diag = merr.SafeMessage()
		}
		base := a.DefaultCapabilities()
		base[2] = publishCap(StatusFailed, "Verification against the Meta API failed")
		return CheckResult{
			LifecycleStatus: LifecycleError,
			Diagnostic:      fmt.Sprintf("Check failed against the Meta API: %s.", diag),
			CheckedAt:       now,
			Capabilities:    base,
		}
	}

	perm := "pages_manage_posts"
	if channel == MetaChannelInstagram {
		perm = "instagram_content_publish"
	}
	return CheckResult{
		LifecycleStatus: LifecycleConnected,
		Diagnostic:      fmt.Sprintf("Connected to %q with a token accepted for the %s channel; publishing requires the %s permission.", name, channel, perm),
		CheckedAt:       now,
		Capabilities: func() []CapabilityReport {
			base := a.DefaultCapabilities()
			base[2] = publishCap(StatusSupported, "Automated posting verified for this token; every publish is still approval-gated here")
			return base
		}(),
	}
}

// ---- Graph operations --------------------------------------------------------

// MetaOps is the thin Graph API client used by both Check and the publisher.
// Only the access token it is constructed from is secret; nothing it returns
// or stores includes credentials.
type MetaOps struct {
	Base       string
	APIVersion string
	Channel    string
	TargetID   string
	Token      string
	Client     *http.Client
}

// NewMetaOps builds the ops client from the integration's non-secret config
// plus the decrypted access token. Base and client are injectable for tests.
func NewMetaOps(config map[string]any, accessToken, base string, client *http.Client) *MetaOps {
	channel, _ := config["channel"].(string)
	targetID, _ := config["targetId"].(string)
	version := strings.TrimSpace(stringConfig(config, "apiVersion"))
	if version == "" {
		version = DefaultGraphAPIVersion
	}
	if base == "" {
		base = defaultGraphBase
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &MetaOps{
		Base:       strings.TrimRight(base, "/"),
		APIVersion: version,
		Channel:    strings.ToLower(strings.TrimSpace(channel)),
		TargetID:   strings.TrimSpace(targetID),
		Token:      strings.TrimSpace(accessToken),
		Client:     client,
	}
}

// metaError is the adapter's only error type: it carries an operator-safe
// message and never embeds tokens or network internals.
type MetaError struct {
	Code int    // Graph error code when the API answered
	Kind string // token_expired | token_rejected | rate_limited | forbidden | validation | network | unavailable
	Safe string
}

func (e *MetaError) Error() string       { return e.Safe }
func (e *MetaError) SafeMessage() string { return e.Safe }

var _ SafePublisherError = (*MetaError)(nil)

func metaNetworkErr() *MetaError {
	return &MetaError{Kind: "network", Safe: "the Meta API could not be reached"}
}

func metaUnexpectedErr() *MetaError {
	return &MetaError{Kind: "unavailable", Safe: "the Meta API returned an unexpected response"}
}

// classifyMetaResponse maps a Graph error body to one safe message. Specific
// documented error codes win over the generic "OAuthException" type, because
// rate-limit errors also carry that type; code 190 without a known code is
// the token problem: expired, revoked, or missing permissions, and the only
// remedy is reconnecting the integration.
func classifyMetaResponse(status int, body []byte) *MetaError {
	var parsed struct {
		Error struct {
			Code    int    `json:"code"`
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &parsed)
	e := parsed.Error

	switch {
	case e.Code == 4 || e.Code == 17 || e.Code == 32 || e.Code == 613:
		return &MetaError{Code: e.Code, Kind: "rate_limited", Safe: "the Meta API is rate limiting this app; wait before retrying"}
	case e.Code == 190 || e.Type == "OAuthException":
		return &MetaError{Code: e.Code, Kind: "token_expired", Safe: "the Meta access token was rejected or has expired; reconnect the integration to refresh authorization"}
	case status == http.StatusForbidden || e.Code == 10 || e.Code == 200 || e.Code == 210:
		return &MetaError{Code: e.Code, Kind: "forbidden", Safe: "the Meta access token lacks the permission this action requires; reconnect with the required publishing permission"}
	case status >= 500:
		return &MetaError{Code: e.Code, Kind: "unavailable", Safe: "the Meta API is temporarily unavailable"}
	default:
		brief := strings.TrimSpace(e.Message)
		if len(brief) > 200 {
			brief = brief[:200]
		}
		if brief == "" {
			return &MetaError{Code: e.Code, Kind: "validation", Safe: fmt.Sprintf("the Meta API rejected the request (HTTP %d)", status)}
		}
		return &MetaError{Code: e.Code, Kind: "validation", Safe: fmt.Sprintf("the Meta API rejected the request: %s", brief)}
	}
}

// graphURL builds one graph endpoint URL with the access token attached for
// read calls; write calls keep the token out of the URL where possible.
func (o *MetaOps) graphURL(path string, values url.Values) string {
	return fmt.Sprintf("%s/%s/%s?%s", o.Base, o.APIVersion, strings.TrimPrefix(path, "/"), values.Encode())
}

// VerifyTarget performs the read-only connection check: the target object is
// read with the token itself.
func (o *MetaOps) VerifyTarget(ctx context.Context) (string, error) {
	values := url.Values{}
	values.Set("access_token", o.Token)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.graphURL(o.TargetID, values), nil)
	if err != nil {
		return "", metaNetworkErr()
	}
	body, status, err := o.do(req)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", classifyMetaResponse(status, body)
	}
	var parsed struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", metaUnexpectedErr()
	}
	return fallbackValue(parsed.Name, fallbackValue(parsed.ID, "the target")), nil
}

// metaPost is one scanned recent Page post (duplicate prevention only).
type metaPost struct {
	ID      string `json:"id"`
	Message string `json:"message"`
	Link    string `json:"link"`
}

// RecentPagePosts reads the Page's most recent posts — read-only, used to
// prevent duplicate posts after a retry.
func (o *MetaOps) RecentPagePosts(ctx context.Context) ([]metaPost, error) {
	values := url.Values{}
	values.Set("fields", "id,message,link")
	values.Set("limit", strconv.Itoa(metaFeedScanLimit))
	values.Set("access_token", o.Token)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.graphURL(o.TargetID+"/feed", values), nil)
	if err != nil {
		return nil, metaNetworkErr()
	}
	body, status, err := o.do(req)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, classifyMetaResponse(status, body)
	}
	var parsed struct {
		Data []metaPost `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, metaUnexpectedErr()
	}
	return parsed.Data, nil
}

// CreatePagePost writes one message (+ optional link) to the Page feed and
// returns the new post id.
func (o *MetaOps) CreatePagePost(ctx context.Context, message, link string) (string, error) {
	values := url.Values{}
	values.Set("message", message)
	values.Set("access_token", o.Token)
	if link != "" {
		values.Set("link", link)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/%s/%s/feed", o.Base, o.APIVersion, url.PathEscape(o.TargetID)), strings.NewReader(values.Encode()))
	if err != nil {
		return "", metaNetworkErr()
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	body, status, err := o.do(req)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return "", classifyMetaResponse(status, body)
	}
	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.ID == "" {
		return "", metaUnexpectedErr()
	}
	return parsed.ID, nil
}

// PermissionsFor checks a Page post's permalink read-only. The proxy target of
// a post id (pageid_postid) resolves via the same GET pattern.
func (o *MetaOps) Permalink(ctx context.Context, objectID string) (string, error) {
	values := url.Values{}
	values.Set("fields", "permalink")
	values.Set("access_token", o.Token)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.graphURL(objectID, values), nil)
	if err != nil {
		return "", metaNetworkErr()
	}
	body, status, err := o.do(req)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", classifyMetaResponse(status, body)
	}
	var parsed struct {
		Permalink string `json:"permalink"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", metaUnexpectedErr()
	}
	return strings.TrimSpace(parsed.Permalink), nil
}

// CreateIGContainer starts the documented two-step Instagram publish: one
// container for the (public) image and caption.
func (o *MetaOps) CreateIGContainer(ctx context.Context, imageURL, caption string) (string, error) {
	values := url.Values{}
	values.Set("image_url", imageURL)
	values.Set("caption", caption)
	values.Set("access_token", o.Token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/%s/%s/media", o.Base, o.APIVersion, url.PathEscape(o.TargetID)), strings.NewReader(values.Encode()))
	if err != nil {
		return "", metaNetworkErr()
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	body, status, err := o.do(req)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return "", classifyMetaResponse(status, body)
	}
	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.ID == "" {
		return "", metaUnexpectedErr()
	}
	return parsed.ID, nil
}

// PublishIGMedia completes the second step of the Instagram publish.
func (o *MetaOps) PublishIGMedia(ctx context.Context, containerID string) (string, error) {
	values := url.Values{}
	values.Set("creation_id", containerID)
	values.Set("access_token", o.Token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/%s/%s/media_publish", o.Base, o.APIVersion, url.PathEscape(o.TargetID)), strings.NewReader(values.Encode()))
	if err != nil {
		return "", metaNetworkErr()
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	body, status, err := o.do(req)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return "", classifyMetaResponse(status, body)
	}
	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.ID == "" {
		return "", metaUnexpectedErr()
	}
	return parsed.ID, nil
}

// do executes one request with the injectable client and a bounded timeout
// caller-side context. It never returns response bodies on errors other than
// through classifyMetaResponse, and strips everything from the error path so
// no token or URL with a token can leak.
func (o *MetaOps) do(req *http.Request) ([]byte, int, error) {
	resp, err := o.Client.Do(req)
	if err != nil {
		return nil, 0, metaNetworkErr()
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return nil, resp.StatusCode, metaNetworkErr()
	}
	return body, resp.StatusCode, nil
}

func fallbackValue(value, def string) string {
	if strings.TrimSpace(value) == "" {
		return def
	}
	return value
}
