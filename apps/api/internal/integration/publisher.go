package integration

import (
	"context"
	"errors"
	"net/http"
)

// SafePublisherError is returned only when the message is explicitly scrubbed of credentials.
type SafePublisherError interface {
	error
	SafeMessage() string
}

// ErrPublisherNotConfigured means the connection is missing non-secret setup values.
var ErrPublisherNotConfigured = errors.New("the publishing destination is not fully configured")

// ErrPublisherCredentialsMissing means no usable encrypted credentials were supplied.
var ErrPublisherCredentialsMissing = errors.New("the publishing destination has no usable credentials")

// ErrPublisherCredentialsUnreadable means a credential set is stored but it
// could not be decrypted or parsed with the current server key (the server
// key may have changed since the credentials were saved). Distinct from the
// "missing" sentinel so the operator is told to re-enter the credentials
// instead of just adding them.
var ErrPublisherCredentialsUnreadable = errors.New("stored credentials could not be decrypted with the current server key; re-enter them")

// ErrPublisherUnsupported means the destination kind has no implemented
// publishing adapter. The message is intentionally operator-safe.
var ErrPublisherUnsupported = publisherUnsupported{}

type publisherUnsupported struct{}

func (publisherUnsupported) Error() string {
	return "this destination does not support automated publishing yet"
}

func (publisherUnsupported) SafeMessage() string {
	return "this destination does not support automated publishing yet"
}

// PublisherNeedsReconnection reports whether a publisher failure is a
// credential problem the operator fixes by reconnecting the integration:
// a rejected or expired Meta access token, a rejected YouTube authorization,
// a rejected or expired X (Twitter) OAuth user token, or a Sanity token the
// API refuses with 401/403. Stored-but-unreadable credentials also count:
// re-entering them is the fix. Other failure kinds (rate limits, network
// trouble, validation) are not credential problems.
func PublisherNeedsReconnection(err error) bool {
	if errors.Is(err, ErrPublisherCredentialsUnreadable) {
		return true
	}
	var merr *MetaError
	if errors.As(err, &merr) && merr.Kind == "token_expired" {
		return true
	}
	var yerr *YouTubeError
	if errors.As(err, &yerr) && yerr.Kind == "token_expired" {
		return true
	}
	var xerr *XError
	if errors.As(err, &xerr) && xerr.Kind == "token_expired" {
		return true
	}
	var serr *SanityWriteError
	if errors.As(err, &serr) && serr.Kind == SanityFailProvider &&
		(serr.HTTP == http.StatusUnauthorized || serr.HTTP == http.StatusForbidden) {
		return true
	}
	return false
}

// PayloadMainImage is the optional hero image of a content version. Ref is
// the Sanity asset document id produced by an earlier upload; URL, when
// present, is the asset's public CDN location (required by destinations
// that fetch the image themselves, such as Instagram's container API).
type PayloadMainImage struct {
	Ref string
	URL string
	Alt string
}

// ContentPayload is an immutable Marketing Ops content snapshot passed to a publisher.
type ContentPayload struct {
	ItemID       string
	Version      int
	Title        string
	Slug         string
	Excerpt      string
	Body         string
	CallToAction string
	ChannelCopy  map[string]string
	MainImage    *PayloadMainImage
	// VideoURL, when set, is a public URL to the video file of a video
	// destination version (YouTube). The publisher downloads it server-side
	// and streams it straight into the upload — it is never stored here.
	VideoURL string
}

// PublishOutcome identifies the result written by an external publishing system.
type PublishOutcome struct {
	ExternalID    string
	TransactionID string
}

// URLVerification describes a read-only check of the resulting public page.
type URLVerification struct {
	URL        string
	Verified   bool
	HTTPStatus int
}

// Publisher implements draft creation, approval-gated publication, and optional URL checking.
type Publisher interface {
	CreateDraft(context.Context, ContentPayload) (draftID string, transactionID string, err error)
	Publish(context.Context, ContentPayload) (PublishOutcome, error)
	VerifyPublicURL(context.Context, string, string) URLVerification
}

// PublishingAdapter creates a publisher from non-secret configuration and decrypted credentials.
// The API host override and HTTP client are intended for safe local contract tests.
type PublishingAdapter interface {
	NewPublisher(config map[string]any, credentials map[string]string, apiBase string, client *http.Client) (Publisher, error)
}

// SanityPublisherAdapter builds the built-in Sanity implementation from generic connection data.
type SanityPublisherAdapter struct{}

// NewPublisher creates a Sanity publisher without exposing credentials outside this adapter.
func (SanityPublisherAdapter) NewPublisher(config map[string]any, credentials map[string]string, apiBase string, client *http.Client) (Publisher, error) {
	projectID, _ := config["projectId"].(string)
	dataset, _ := config["dataset"].(string)
	token := credentials["token"]
	if projectID == "" || dataset == "" {
		return nil, ErrPublisherNotConfigured
	}
	if token == "" {
		return nil, ErrPublisherCredentialsMissing
	}
	return SanityPublisher{
		ops: SanityOps{
			Creds: SanityCredentials{
				ProjectID:  projectID,
				Dataset:    dataset,
				Token:      token,
				APIVersion: stringConfig(config, "apiVersion"),
			},
			Base:   apiBase,
			Client: client,
		},
		docType:   stringConfig(config, "docType"),
		urlPrefix: stringConfig(config, "publicUrlPrefix"),
	}, nil
}

// stringConfig reads a string setting without asserting provider-specific config in callers.
func stringConfig(config map[string]any, key string) string {
	value, _ := config[key].(string)
	return value
}

// SanityPublisher adapts Sanity operations to the platform-neutral Publisher contract.
type SanityPublisher struct {
	ops       SanityOps
	docType   string
	urlPrefix string
}

func (p SanityPublisher) CreateDraft(ctx context.Context, payload ContentPayload) (string, string, error) {
	return p.ops.CreateDraft(ctx, payload.ItemID, payload.Version, p.post(payload))
}

func (p SanityPublisher) Publish(ctx context.Context, payload ContentPayload) (PublishOutcome, error) {
	id, transaction, err := p.ops.Publish(ctx, payload.ItemID, payload.Version, p.post(payload))
	if err != nil {
		return PublishOutcome{}, err
	}
	return PublishOutcome{ExternalID: id, TransactionID: transaction}, nil
}

func (p SanityPublisher) VerifyPublicURL(ctx context.Context, websiteURL, slug string) URLVerification {
	url, verified, status := p.ops.VerifyPublicURL(ctx, websiteURL, p.urlPrefix, slug)
	return URLVerification{URL: url, Verified: verified, HTTPStatus: status}
}

func (p SanityPublisher) post(payload ContentPayload) SanityPost {
	extra := map[string]any{}
	if payload.CallToAction != "" {
		extra["callToAction"] = payload.CallToAction
	}
	if len(payload.ChannelCopy) > 0 {
		extra["marketingOpsChannels"] = payload.ChannelCopy
	}
	if payload.MainImage != nil && payload.MainImage.Ref != "" {
		image := map[string]any{
			"_type": "image",
			"asset": map[string]any{"_type": "reference", "_ref": payload.MainImage.Ref},
		}
		if payload.MainImage.Alt != "" {
			image["alt"] = payload.MainImage.Alt
		}
		extra["mainImage"] = image
	}
	return SanityPost{
		DocType:    p.docType,
		Title:      payload.Title,
		Slug:       payload.Slug,
		Excerpt:    payload.Excerpt,
		BodyBlocks: ConvertBodyToBlocks(payload.Body),
		Extra:      extra,
	}
}

var _ Publisher = SanityPublisher{}
var _ PublishingAdapter = SanityPublisherAdapter{}
