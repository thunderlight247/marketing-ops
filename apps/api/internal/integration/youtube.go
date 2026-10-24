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
	"strings"
	"time"
)

// YouTube adapter (Milestone 9). One integration targets exactly one YouTube
// channel (the channel the OAuth authorization belongs to), through the
// official YouTube Data API v3 only. Authorization is an operator-obtained
// OAuth refresh token minted offline with the youtube.upload scope; the
// adapter exchanges it for short-lived access tokens server-side and never
// scrapes, never logs in with passwords, and never touches unofficial
// endpoints.
//
// Honesty rules encoded below:
//   - videos.insert has no idempotent upsert, so a retry of an attempt that
//     reached the upload can create a second video; the job history shows both
//     (recorded as a known limitation, like Instagram's two-step flow).
//   - uploads cost a large slice of the project's daily quota, so the check
//     verifies the channel with a cheap channels.list read only.
const (
	DefaultYouTubeScope  = "https://www.googleapis.com/auth/youtube.upload"
	defaultTokenBase     = "https://oauth2.googleapis.com"
	defaultGoogleAPIBase = "https://www.googleapis.com"

	defaultYouTubeCategory = "22"
	defaultYouTubePrivacy  = "public"

	// youtubeTitleLimit and youtubeDescriptionLimit are the documented
	// snippet limits enforced at upload time.
	youtubeTitleLimit       = 100
	youtubeDescriptionLimit = 5000

	// maxYouTubeResponseBody caps error-body reads so a hostile response
	// cannot balloon memory.
	maxYouTubeResponseBody = 256 * 1024
)

var youtubeChannelPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{12,60}$`)

// youtubeOpsConstructor is swappable so tests can point Check at fake servers.
var youtubeOpsConstructor = func(config map[string]any, creds map[string]string) *YouTubeOps {
	return NewYouTubeOps(config, creds, defaultTokenBase, defaultGoogleAPIBase, nil)
}

// YouTubeAdapter implements Adapter for the YouTube Data API v3.
type YouTubeAdapter struct{}

func (a *YouTubeAdapter) Kind() string        { return "youtube" }
func (a *YouTubeAdapter) DisplayName() string { return "YouTube (channel upload)" }

func (a *YouTubeAdapter) Description() string {
	return "Official YouTube Data API uploads to one authorized channel, with metadata and privacy from the approved version"
}

func (a *YouTubeAdapter) DefaultCapabilities() []CapabilityReport {
	return []CapabilityReport{
		{Capability: CapReadContent, Status: StatusUnsupported, Details: "YouTube is a video site, not content storage for articles; the website/CMS integration keeps the article"},
		{Capability: CapCreateDraft, Status: StatusUnsupported, Details: "YouTube uploads are public-facing immediately on publish; previews happen in this dashboard before approval"},
		{Capability: CapPublish, Status: StatusUnverified, Details: "Requires an OAuth refresh token minted with the youtube.upload scope"},
		{Capability: CapUpdateContent, Status: StatusUnsupported, Details: "Uploaded videos are not edited back from here"},
		{Capability: CapSchedule, Status: StatusUnsupported, Details: "Scheduled publishes queue in this platform's scheduler and run at their due time"},
		{Capability: CapAnalytics, Status: StatusUnsupported, Details: "View and watch-time analytics are out of scope for this adapter"},
		{Capability: CapLeadIntake, Status: StatusUnsupported, Details: "Not an enquiry endpoint"},
	}
}

// publishCap mirrors Meta's helper: one capability report for CapPublish.
func youtubePublishCap(status, details string) CapabilityReport {
	return CapabilityReport{Capability: CapPublish, Status: status, Details: details}
}

func (a *YouTubeAdapter) Check(ctx context.Context, config map[string]any, creds map[string]string) CheckResult {
	now := time.Now().UTC()

	channelID := strings.TrimSpace(stringConfig(config, "channelId"))
	if channelID == "" {
		return CheckResult{
			LifecycleStatus: LifecycleError,
			Diagnostic:      "Configuration error: channelId is required (the UC… channel id of the authorized channel)",
			CheckedAt:       now,
			Capabilities:    a.DefaultCapabilities(),
		}
	}
	if !youtubeChannelPattern.MatchString(channelID) {
		return CheckResult{
			LifecycleStatus: LifecycleError,
			Diagnostic:      "Configuration error: channelId has an unsupported format (expected a UC… channel id)",
			CheckedAt:       now,
			Capabilities:    a.DefaultCapabilities(),
		}
	}
	if privacy := lowerTrim(stringConfig(config, "privacyStatus")); privacy != "" &&
		privacy != "public" && privacy != "unlisted" && privacy != "private" {
		return CheckResult{
			LifecycleStatus: LifecycleError,
			Diagnostic:      "Configuration error: privacyStatus must be public, unlisted, or private",
			CheckedAt:       now,
			Capabilities:    a.DefaultCapabilities(),
		}
	}

	refreshToken := strings.TrimSpace(creds["refreshToken"])
	clientID := strings.TrimSpace(creds["clientId"])
	clientSecret := strings.TrimSpace(creds["clientSecret"])
	if refreshToken == "" || clientID == "" || clientSecret == "" {
		return CheckResult{
			LifecycleStatus: LifecycleLimited,
			Diagnostic:      fmt.Sprintf("Channel %s is configured, but the OAuth refresh token and client credentials have not been saved, so channel access could not be verified.", channelID),
			CheckedAt:       now,
			Capabilities: func() []CapabilityReport {
				base := a.DefaultCapabilities()
				base[2] = youtubePublishCap(StatusUnverified, "Waiting for the OAuth refresh token and client credentials")
				return base
			}(),
		}
	}

	ops := youtubeOpsConstructor(config, creds)
	title, checkErr := ops.VerifyChannel(ctx, channelID)
	if checkErr != nil {
		diag := "the YouTube API could not be reached"
		var yerr *YouTubeError
		if errors.As(checkErr, &yerr) {
			diag = yerr.SafeMessage()
		}
		base := a.DefaultCapabilities()
		base[2] = youtubePublishCap(StatusFailed, "Verification against the YouTube API failed")
		return CheckResult{
			LifecycleStatus: LifecycleError,
			Diagnostic:      fmt.Sprintf("Check failed against the YouTube API: %s.", diag),
			CheckedAt:       now,
			Capabilities:    base,
		}
	}

	return CheckResult{
		LifecycleStatus: LifecycleConnected,
		Diagnostic:      fmt.Sprintf("Connected to YouTube channel %q (%s) with a youtube.upload-scoped authorization; every upload is still approval-gated here.", title, channelID),
		CheckedAt:       now,
		Capabilities: func() []CapabilityReport {
			base := a.DefaultCapabilities()
			base[2] = youtubePublishCap(StatusSupported, "Automated uploads verified for this authorization; every publish is still approval-gated here and costs a large slice of the project's upload quota")
			return base
		}(),
	}
}

// ---- Operations ---------------------------------------------------------------

// YouTubeOps is the thin YouTube Data API v3 client used by both Check and the
// publisher. Only the OAuth credentials it is constructed from are secret;
// nothing it returns or stores includes credentials.
type YouTubeOps struct {
	TokenBase    string
	APIBase      string
	RefreshToken string
	ClientID     string
	ClientSecret string
	Privacy      string
	Category     string
	Notify       bool
	Client       *http.Client
}

// NewYouTubeOps builds the ops client from the integration's non-secret
// config plus the decrypted OAuth credentials. Bases and client are
// injectable for tests.
func NewYouTubeOps(config map[string]any, creds map[string]string, tokenBase, apiBase string, client *http.Client) *YouTubeOps {
	privacy := lowerTrim(stringConfig(config, "privacyStatus"))
	if privacy != "public" && privacy != "unlisted" && privacy != "private" {
		privacy = defaultYouTubePrivacy
	}
	category := strings.TrimSpace(stringConfig(config, "categoryId"))
	if category == "" {
		category = defaultYouTubeCategory
	}
	if tokenBase == "" {
		tokenBase = defaultTokenBase
	}
	if apiBase == "" {
		apiBase = defaultGoogleAPIBase
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &YouTubeOps{
		TokenBase:    strings.TrimRight(tokenBase, "/"),
		APIBase:      strings.TrimRight(apiBase, "/"),
		RefreshToken: strings.TrimSpace(creds["refreshToken"]),
		ClientID:     strings.TrimSpace(creds["clientId"]),
		ClientSecret: strings.TrimSpace(creds["clientSecret"]),
		Privacy:      privacy,
		Category:     category,
		Notify:       boolConfig(config, "notifySubscribers"),
		Client:       client,
	}
}

// youtubeError is the adapter's only error type: it carries an operator-safe
// message and never embeds tokens or network internals.
type YouTubeError struct {
	Reason string // documented API error reason when available
	Kind   string // token_expired | token_rejected | rate_limited | forbidden | validation | network | unavailable
	Safe   string
}

func (e *YouTubeError) Error() string       { return e.Safe }
func (e *YouTubeError) SafeMessage() string { return e.Safe }

var _ SafePublisherError = (*YouTubeError)(nil)

func youtubeNetworkErr() *YouTubeError {
	return &YouTubeError{Kind: "network", Safe: "the YouTube API could not be reached"}
}

func youtubeUnexpectedErr() *YouTubeError {
	return &YouTubeError{Kind: "unavailable", Safe: "the YouTube API returned an unexpected response"}
}

// classifyYouTubeTokenResponse maps an OAuth token-endpoint error to one safe
// message. Token-endpoint bodies use {error, error_description}.
func classifyYouTubeTokenResponse(body []byte) *YouTubeError {
	var parsed struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	_ = json.Unmarshal(body, &parsed)
	switch parsed.Error {
	case "invalid_grant", "invalid_client", "unauthorized_client":
		return &YouTubeError{Kind: "token_expired", Safe: "the YouTube authorization was rejected or has expired; reconnect the integration to refresh authorization"}
	case "slow_down", "exceeded_rate_limit":
		return &YouTubeError{Kind: "rate_limited", Safe: "the YouTube authorization endpoint is rate limiting this app; wait before retrying"}
	default:
		brief := strings.TrimSpace(parsed.ErrorDescription)
		if len(brief) > 200 {
			brief = brief[:200]
		}
		if brief == "" {
			return &YouTubeError{Kind: "token_rejected", Safe: "the YouTube authorization endpoint rejected this request"}
		}
		return &YouTubeError{Kind: "token_rejected", Safe: fmt.Sprintf("the YouTube authorization endpoint rejected this request: %s", brief)}
	}
}

// classifyYouTubeResponse maps a Data API error body
// ({error: {message, errors: [{reason}]}}) to one safe message.
func classifyYouTubeResponse(status int, body []byte) *YouTubeError {
	var parsed struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Errors  []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &parsed)
	reason := ""
	for _, item := range parsed.Error.Errors {
		if r := strings.TrimSpace(item.Reason); r != "" {
			reason = r
			break
		}
	}

	switch {
	case strings.Contains(reason, "quotaExceeded") || strings.Contains(reason, "rateLimit") || status == http.StatusTooManyRequests:
		return &YouTubeError{Reason: reason, Kind: "rate_limited", Safe: "the YouTube API is rate limiting or has exhausted this project's daily quota; wait or request a quota increase"}
	case status == http.StatusUnauthorized:
		return &YouTubeError{Reason: reason, Kind: "token_expired", Safe: "the YouTube access token was rejected; reconnect the integration to refresh authorization"}
	case status == http.StatusForbidden:
		switch {
		case strings.Contains(reason, "insufficient") || reason == "forbidden":
			return &YouTubeError{Reason: reason, Kind: "forbidden", Safe: "the YouTube authorization lacks the youtube.upload scope or channel rights this action requires; reconnect with the required scope"}
		default:
			return &YouTubeError{Reason: reason, Kind: "forbidden", Safe: "the YouTube authorization is not allowed to upload for this channel"}
		}
	case status >= 500:
		return &YouTubeError{Reason: reason, Kind: "unavailable", Safe: "the YouTube API is temporarily unavailable"}
	default:
		brief := strings.TrimSpace(parsed.Error.Message)
		if len(brief) > 200 {
			brief = brief[:200]
		}
		if brief == "" {
			return &YouTubeError{Reason: reason, Kind: "validation", Safe: fmt.Sprintf("the YouTube API rejected the request (HTTP %d)", status)}
		}
		return &YouTubeError{Reason: reason, Kind: "validation", Safe: fmt.Sprintf("the YouTube API rejected the request: %s", brief)}
	}
}

// AccessToken exchanges the stored refresh token for one short-lived access
// token. It is a network call per run (YouTube access tokens last ~1h), and
// the response never reaches callers: only the token value inside the ops
// client does.
func (o *YouTubeOps) AccessToken(ctx context.Context) (string, error) {
	if o.RefreshToken == "" || o.ClientID == "" || o.ClientSecret == "" {
		return "", &YouTubeError{Kind: "token_rejected", Safe: "the OAuth refresh token or client credentials are missing"}
	}
	values := url.Values{}
	values.Set("grant_type", "refresh_token")
	values.Set("refresh_token", o.RefreshToken)
	values.Set("client_id", o.ClientID)
	values.Set("client_secret", o.ClientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.TokenBase+"/token", strings.NewReader(values.Encode()))
	if err != nil {
		return "", youtubeNetworkErr()
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	body, status, _, err := o.do(req)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", classifyYouTubeTokenResponse(body)
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || strings.TrimSpace(parsed.AccessToken) == "" {
		return "", youtubeUnexpectedErr()
	}
	return strings.TrimSpace(parsed.AccessToken), nil
}

// VerifyChannel performs the read-only connection check: one channels.list
// read confirms the authorization is live and owns the configured channel.
func (o *YouTubeOps) VerifyChannel(ctx context.Context, channelID string) (string, error) {
	token, err := o.AccessToken(ctx)
	if err != nil {
		return "", err
	}
	endpoint := fmt.Sprintf("%s/youtube/v3/channels?part=snippet&id=%s", o.APIBase, url.QueryEscape(channelID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", youtubeNetworkErr()
	}
	req.Header.Set("Authorization", "Bearer "+token)
	body, status, _, err := o.do(req)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", classifyYouTubeResponse(status, body)
	}
	var parsed struct {
		Items []struct {
			ID      string `json:"id"`
			Snippet struct {
				Title string `json:"title"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", youtubeUnexpectedErr()
	}
	if len(parsed.Items) == 0 {
		return "", &YouTubeError{
			Kind: "not_found",
			Safe: fmt.Sprintf("the authorized YouTube account cannot see channel %s; confirm the channel id belongs to the OAuth user", channelID),
		}
	}
	return fallbackValue(parsed.Items[0].Snippet.Title, fallbackValue(parsed.Items[0].ID, "the channel")), nil
}

// do executes one request with the injectable client and the caller's context.
// It returns the status-code-capped response body and the response headers
// (the resumable upload session comes back as a Location header). Error
// paths never echo bodies or URLs, so no token can leak.
func (o *YouTubeOps) do(req *http.Request) ([]byte, int, http.Header, error) {
	resp, err := o.Client.Do(req)
	if err != nil {
		return nil, 0, nil, youtubeNetworkErr()
	}
	defer func() { _ = resp.Body.Close() }()
	header := resp.Header
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxYouTubeResponseBody))
	if err != nil {
		return nil, resp.StatusCode, header, youtubeNetworkErr()
	}
	return body, resp.StatusCode, header, nil
}

// boolConfig reads an optional boolean setting.
func boolConfig(config map[string]any, key string) bool {
	value, _ := config[key].(bool)
	return value
}
