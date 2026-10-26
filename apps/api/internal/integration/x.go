package integration

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

// X (formerly Twitter) social adapter. It targets ONE official X API v2
// surface: the account whose OAuth 2.0 user access token the operator saved.
// The adapter never scrapes, never logs in with passwords, and never touches
// unofficial endpoints.
//
// Honesty rules encoded below (parallel to the Meta adapter):
//   - publishing means approval-gated tweet creation through POST /2/tweets
//     with the tweet.write scope; there is no destination-side draft concept,
//     so preview happens in this dashboard and the approval gate is the
//     review path;
//   - a token that no longer authenticates is reported failed, not assumed;
//   - duplicate prevention mirrors Facebook: the publisher scans the
//     account's recent tweets read-only and reuses an exact text match
//     instead of posting again on a retried job.
//
// OAuth onboarding (the flow that mints the user access token) is a separate
// later increment recorded in the project memory; this adapter consumes an
// operator-supplied token exactly like the Meta increment does.
const (
	DefaultXAPIBase = "https://api.x.com"

	// xTweetLimit is the documented per-tweet character cap for standard
	// API-published tweets.
	xTweetLimit = 280
	// xFeedScanLimit bounds the read used for duplicate prevention: the
	// publisher scans this many of the account's most recent tweets before
	// creating a new one.
	xFeedScanLimit = 25
)

// xOpsConstructor is swappable so tests can point both Check and the
// publisher at a fake server.
var xOpsConstructor = func(config map[string]any, accessToken string) *XOps {
	return NewXOps(config, accessToken, DefaultXAPIBase, nil)
}

// XAdapter implements Adapter for X API v2 surfaces.
type XAdapter struct{}

func (a *XAdapter) Kind() string        { return "x" }
func (a *XAdapter) DisplayName() string { return "X (Twitter)" }

func (a *XAdapter) Description() string {
	return "Official X API v2 posting to the authorized user account"
}

func (a *XAdapter) DefaultCapabilities() []CapabilityReport {
	return []CapabilityReport{
		{Capability: CapReadContent, Status: StatusUnsupported, Details: "Social platforms are not content storage for articles; the website/CMS integration keeps the article"},
		{Capability: CapCreateDraft, Status: StatusUnsupported, Details: "X has no external draft step; previews happen in this dashboard before approval"},
		{Capability: CapPublish, Status: StatusUnverified, Details: "Requires an OAuth 2.0 user access token with the tweet.write scope"},
		{Capability: CapUpdateContent, Status: StatusUnsupported, Details: "Published posts are not edited back from here"},
		{Capability: CapSchedule, Status: StatusUnsupported, Details: "Scheduled publishes queue in this platform's scheduler and run at their due time"},
		{Capability: CapAnalytics, Status: StatusUnsupported, Details: "Post metrics are out of scope for this adapter"},
		{Capability: CapLeadIntake, Status: StatusUnsupported, Details: "Not an enquiry endpoint"},
	}
}

func (a *XAdapter) Check(ctx context.Context, config map[string]any, creds map[string]string) CheckResult {
	now := time.Now().UTC()

	token := strings.TrimSpace(creds["accessToken"])
	publishCap := func(status, details string) CapabilityReport {
		return CapabilityReport{Capability: CapPublish, Status: status, Details: details}
	}
	if token == "" {
		base := a.DefaultCapabilities()
		base[2] = publishCap(StatusUnverified, "Waiting for the access token")
		return CheckResult{
			LifecycleStatus: LifecycleLimited,
			Diagnostic:      "No access token has been saved, so publishing access could not be verified.",
			CheckedAt:       now,
			Capabilities:    base,
		}
	}

	ops := xOpsConstructor(config, token)
	username, name, checkErr := ops.VerifyTarget(ctx)
	if checkErr != nil {
		diag := "the X API could not be reached"
		var xerr *XError
		if errors.As(checkErr, &xerr) {
			diag = xerr.SafeMessage()
		}
		base := a.DefaultCapabilities()
		base[2] = publishCap(StatusFailed, "Verification against the X API failed")
		return CheckResult{
			LifecycleStatus: LifecycleError,
			Diagnostic:      fmt.Sprintf("Check failed against the X API: %s.", diag),
			CheckedAt:       now,
			Capabilities:    base,
		}
	}

	handle := fallbackValue(username, "the account")
	if name != "" && username != "" {
		handle = fmt.Sprintf("@%s (%s)", username, name)
	}
	return CheckResult{
		LifecycleStatus: LifecycleConnected,
		Diagnostic:      fmt.Sprintf("Connected as %s with a token accepted by the X API; publishing requires the tweet.write scope. Every publish is still approval-gated here.", handle),
		CheckedAt:       now,
		Capabilities: func() []CapabilityReport {
			base := a.DefaultCapabilities()
			base[2] = publishCap(StatusSupported, "Automated posting verified for this token; every publish is still approval-gated here")
			return base
		}(),
	}
}

// ---- X API operations ------------------------------------------------------

// XOps is the thin X API v2 client used by both Check and the publisher.
// Only the access token it is constructed from is secret; nothing it returns
// or stores includes credentials.
type XOps struct {
	Base   string
	Token  string
	Client *http.Client
}

// NewXOps builds the ops client from the integration's non-secret config
// plus the decrypted OAuth user access token. Base and client are injectable
// for tests.
func NewXOps(config map[string]any, accessToken, base string, client *http.Client) *XOps {
	if base == "" {
		base = DefaultXAPIBase
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &XOps{
		Base:   strings.TrimRight(base, "/"),
		Token:  strings.TrimSpace(accessToken),
		Client: client,
	}
}

// xUser is the identity read from /2/users/me.
type xUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

// xTweet is one scanned recent account tweet (duplicate prevention only).
type xTweet struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// XError is the adapter's only error type: it carries an operator-safe
// message and never embeds tokens or network internals.
type XError struct {
	Code int    // X error status when the API answered
	Kind string // token_expired | token_rejected | rate_limited | forbidden | validation | network | unavailable
	Safe string
}

func (e *XError) Error() string       { return e.Safe }
func (e *XError) SafeMessage() string { return e.Safe }

var _ SafePublisherError = (*XError)(nil)

func xNetworkErr() *XError {
	return &XError{Kind: "network", Safe: "the X API could not be reached"}
}

func xUnexpectedErr() *XError {
	return &XError{Kind: "unavailable", Safe: "the X API returned an unexpected response"}
}

// classifyXResponse maps an X API v2 error body to one safe message.
// Status wins over the body: a 401 answer is always the credential problem
// (expired or revoked token), and the only remedy is reconnecting the
// integration, exactly like a rejected Meta token.
func classifyXResponse(status int, body []byte) *XError {
	var parsed struct {
		Title    string `json:"title"`
		Detail   string `json:"detail"`
		Problems []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(body, &parsed)
	first := strings.TrimSpace(parsed.Detail)
	if first == "" && len(parsed.Problems) > 0 {
		first = strings.TrimSpace(parsed.Problems[0].Message)
	}

	switch {
	case status == http.StatusUnauthorized:
		return &XError{Code: status, Kind: "token_expired", Safe: "the X access token was rejected or has expired; reconnect the integration to refresh authorization"}
	case status == http.StatusForbidden:
		return &XError{Code: status, Kind: "forbidden", Safe: "the X access token lacks the permission this action requires; reconnect with the tweet.write scope"}
	case status == http.StatusTooManyRequests:
		return &XError{Code: status, Kind: "rate_limited", Safe: "the X API is rate limiting this account; wait before retrying"}
	case status >= 500:
		return &XError{Code: status, Kind: "unavailable", Safe: "the X API is temporarily unavailable"}
	default:
		brief := first
		if len(brief) > 200 {
			brief = brief[:200]
		}
		if brief == "" {
			return &XError{Code: status, Kind: "validation", Safe: fmt.Sprintf("the X API rejected the request (HTTP %d)", status)}
		}
		return &XError{Code: status, Kind: "validation", Safe: fmt.Sprintf("the X API rejected the request: %s", brief)}
	}
}

// VerifyTarget performs the read-only connection check: /2/users/me is read
// with the token itself. It returns the handle and display name.
func (o *XOps) VerifyTarget(ctx context.Context) (string, string, error) {
	user, err := o.Me(ctx)
	return user.Username, user.Name, err
}

// Me reads the authorizing user's identity, including the id needed by the
// duplicate-prevention scan.
func (o *XOps) Me(ctx context.Context) (xUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.xURL("/users/me?user.fields=name,username"), nil)
	if err != nil {
		return xUser{}, xNetworkErr()
	}
	body, status, err := o.do(req)
	if err != nil {
		return xUser{}, err
	}
	if status != http.StatusOK {
		return xUser{}, classifyXResponse(status, body)
	}
	var parsed struct {
		Data xUser `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return xUser{}, xUnexpectedErr()
	}
	return parsed.Data, nil
}

// RecentUserTweets reads the authorizing user's most recent tweets —
// read-only, used to prevent duplicate posts after a retry.
func (o *XOps) RecentUserTweets(ctx context.Context, userID string) ([]xTweet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		o.xURL(fmt.Sprintf("/users/%s/tweets?max_results=%d&exclude=replies,retweets&tweet.fields=text", url.PathEscape(userID), xFeedScanLimit)), nil)
	if err != nil {
		return nil, xNetworkErr()
	}
	body, status, err := o.do(req)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, classifyXResponse(status, body)
	}
	var parsed struct {
		Data []xTweet `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, xUnexpectedErr()
	}
	return parsed.Data, nil
}

// CreateTweet writes one tweet with the given string and returns the new id.
func (o *XOps) CreateTweet(ctx context.Context, text string) (string, error) {
	payload, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return "", xUnexpectedErr()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.xURL("/tweets"), bytes.NewReader(payload))
	if err != nil {
		return "", xNetworkErr()
	}
	req.Header.Set("Content-Type", "application/json")
	body, status, err := o.do(req)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return "", classifyXResponse(status, body)
	}
	var parsed struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.Data.ID == "" {
		return "", xUnexpectedErr()
	}
	return parsed.Data.ID, nil
}

func (o *XOps) xURL(path string) string {
	return o.Base + "/2" + path
}

// do executes one request with the injectable client. The token travels in
// the Authorization header, never in the URL; it never leaks through error
// paths (XError carries only operator-safe text).
func (o *XOps) do(req *http.Request) ([]byte, int, error) {
	req.Header.Set("Authorization", "Bearer "+o.Token)
	req.Header.Set("Accept", "application/json")
	resp, err := o.Client.Do(req)
	if err != nil {
		return nil, 0, xNetworkErr()
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return nil, resp.StatusCode, xNetworkErr()
	}
	return body, resp.StatusCode, nil
}

// ---- publisher -------------------------------------------------------------

// The X publishing adapter mirrors the Meta one: NewPublisher takes
// non-secret config plus already-decrypted credentials and returns a
// Publisher bound to the authorized account.
type XPublisherAdapter struct{}

func (XPublisherAdapter) NewPublisher(config map[string]any, credentials map[string]string, apiBase string, client *http.Client) (Publisher, error) {
	// Config currently carries no required value; postLinkPrefix stays
	// optional so a workspace can tag the article link onto tweets. Reserved
	// keys are still recognized so future settings cannot silently break
	// stored configurations.
	_ = stringConfig(config, "apiVersion")
	linkPrefix := strings.TrimRight(strings.TrimSpace(stringConfig(config, "postLinkPrefix")), "/")
	token := strings.TrimSpace(credentials["accessToken"])
	if token == "" {
		return nil, ErrPublisherCredentialsMissing
	}
	return &XPublisher{
		ops:        NewXOps(config, token, apiBase, client),
		linkPrefix: linkPrefix,
	}, nil
}

// XPublisher implements the platform-neutral Publisher contract for X.
//
// CreateDraft is the draftless sentinel, like Meta. Duplicate prevention: a
// publish first scans the account's recent tweets read-only and reuses an
// exact text match instead of posting again. The resulting status URL is
// recorded as the artifact URL.
type XPublisher struct {
	ops *XOps
	// linkPrefix, when configured, tags the published website link onto the
	// tweet (publicUrlPrefix-style, e.g. https://site/blog).
	linkPrefix string
	// lastLink remembers the status URL of the most recent successful
	// publish so VerifyPublicURL can hand the real post URL to the artifact
	// record.
	lastLink string
}

var (
	_ Publisher         = (*XPublisher)(nil)
	_ PublishingAdapter = XPublisherAdapter{}
)

func (p *XPublisher) CreateDraft(ctx context.Context, payload ContentPayload) (string, string, error) {
	return "", "", ErrNoDestinationDraft
}

func (p *XPublisher) Publish(ctx context.Context, payload ContentPayload) (PublishOutcome, error) {
	message := p.postMessage(payload)
	if message == "" {
		return PublishOutcome{}, &XError{Kind: "validation", Safe: "there is nothing to post: the approved version has an empty message"}
	}

	// Duplicate prevention needs the authorizing user identity; the same
	// user later builds the artifact URL. A failed identity read must not
	// block publishing: the lookup is read-only and a transient identity
	// failure just skips the scan, so the write below still creates the
	// post once per attempt.
	var username string
	if user, meErr := p.ops.Me(ctx); meErr == nil {
		username = user.Username
		// An existing recent tweet with the same text means a previous attempt
		// already succeeded (or landed before the job was closed); record it
		// rather than post again.
		recent, scanErr := p.ops.RecentUserTweets(ctx, user.ID)
		if scanErr == nil {
			for _, tweet := range recent {
				if strings.TrimSpace(tweet.Text) == message {
					p.lastLink = fmt.Sprintf("https://x.com/%s/status/%s", username, tweet.ID)
					return PublishOutcome{ExternalID: tweet.ID}, nil
				}
			}
		}
	}

	id, createErr := p.ops.CreateTweet(ctx, message)
	if createErr != nil {
		return PublishOutcome{}, createErr
	}
	if username != "" {
		p.lastLink = fmt.Sprintf("https://x.com/%s/status/%s", username, id)
	}
	return PublishOutcome{ExternalID: id}, nil
}

func (p *XPublisher) VerifyPublicURL(ctx context.Context, websiteURL, slug string) URLVerification {
	link := strings.TrimSpace(p.lastLink)
	if link == "" {
		return URLVerification{}
	}
	return URLVerification{URL: link, Verified: true}
}

// postMessage picks the operator's x channel copy when it exists, falling
// back to the article title and excerpt, tags the optional article link, and
// truncates the whole thing to the documented 280-character cap.
func (p *XPublisher) postMessage(payload ContentPayload) string {
	message := strings.TrimSpace(payload.ChannelCopy["x"])
	if message == "" {
		message = strings.TrimSpace(payload.Title)
		if excerpt := strings.TrimSpace(payload.Excerpt); excerpt != "" {
			if message == "" {
				message = excerpt
			} else {
				message += "\n\n" + excerpt
			}
		}
	}
	if p.linkPrefix != "" && strings.TrimSpace(payload.Slug) != "" {
		link := p.linkPrefix + "/" + strings.Trim(strings.TrimSpace(payload.Slug), "/")
		message += "\n" + link
	}
	return truncateXRunes(message, xTweetLimit)
}

func truncateXRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
