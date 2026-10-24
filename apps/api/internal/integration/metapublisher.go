package integration

import (
	"context"
	"net/http"
	"strings"
)

// The Meta publishing adapter mirrors the Sanity one: NewPublisher takes
// non-secret config plus already-decrypted credentials and returns a
// Publisher bound to one channel (facebook or instagram).
type MetaPublisherAdapter struct{}

func (MetaPublisherAdapter) NewPublisher(config map[string]any, credentials map[string]string, apiBase string, client *http.Client) (Publisher, error) {
	channel := lowerTrim(stringConfig(config, "channel"))
	if channel != MetaChannelFacebook && channel != MetaChannelInstagram {
		return nil, ErrPublisherNotConfigured
	}
	if strings.TrimSpace(stringConfig(config, "targetId")) == "" {
		return nil, ErrPublisherNotConfigured
	}
	token := strings.TrimSpace(credentials["accessToken"])
	if token == "" {
		return nil, ErrPublisherCredentialsMissing
	}
	return &MetaPublisher{
		ops: NewMetaOps(config, token, apiBase, client),
		// linkPrefix, when configured, tags the published website link onto a
		// Facebook post (publicUrlPrefix-style, e.g. https://site/blog).
		// Instagram captions cannot carry link previews, so the prefix applies
		// only to the Facebook channel.
		linkPrefix: strings.TrimRight(strings.TrimSpace(stringConfig(config, "postLinkPrefix")), "/"),
	}, nil
}

// MetaPublisher implements the platform-neutral Publisher contract for one
// Meta channel.
//
// Milestone 8 honesty notes:
//   - CreateDraft is a no-op sentinel: Meta documents no pre-approval draft
//     surface we can use, so external preview does not exist. The dashboard
//     preview plus the approval gate is the review path.
//   - Duplicate prevention: a Facebook publish first scans the Page's recent
//     posts read-only and reuses an exact message+link match instead of
//     posting again. Instagram has no such lookup, so an attempt interrupted
//     after the container/publish write can re-create the post on retry —
//     recorded as a known limitation.
//   - The resulting post permalink is recorded as the artifact URL.
//
// Methods use pointer receivers because the last-published permalink is
// instance state carried from Publish to VerifyPublicURL.
type MetaPublisher struct {
	ops *MetaOps
	// linkPrefix, when configured, tags the published website link onto a
	// Facebook post (publicUrlPrefix-style, e.g. https://site/blog). Instagram
	// captions cannot carry link previews, so the prefix applies only to the
	// Facebook channel.
	linkPrefix string
	// lastLink remembers the permalink of the most recent successful publish
	// so VerifyPublicURL can hand the real post URL to the artifact record.
	lastLink string
}

var (
	_ Publisher         = (*MetaPublisher)(nil)
	_ PublishingAdapter = MetaPublisherAdapter{}
)

func (p *MetaPublisher) CreateDraft(ctx context.Context, payload ContentPayload) (string, string, error) {
	return "", "", ErrNoDestinationDraft
}

func (p *MetaPublisher) Publish(ctx context.Context, payload ContentPayload) (PublishOutcome, error) {
	switch p.ops.Channel {
	case MetaChannelInstagram:
		return p.publishInstagram(ctx, payload)
	default:
		return p.publishFacebook(ctx, payload)
	}
}

// postMessage picks the operator's channel copy when it exists, falling back
// to the article title and excerpt. Instagram captions are truncated to the
// documented 2200-character cap.
func (p *MetaPublisher) postMessage(payload ContentPayload, cap int) string {
	channelKey := p.ops.Channel
	message := strings.TrimSpace(payload.ChannelCopy[channelKey])
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
	if cap > 0 && len([]rune(message)) > cap {
		message = truncateMetaRunes(message, cap)
	}
	return message
}

func (p *MetaPublisher) publishFacebook(ctx context.Context, payload ContentPayload) (PublishOutcome, error) {
	message := p.postMessage(payload, 0)
	if message == "" {
		return PublishOutcome{}, &MetaError{Kind: "validation", Safe: "there is nothing to post: the approved version has an empty message"}
	}
	link := ""
	if p.linkPrefix != "" && strings.TrimSpace(payload.Slug) != "" {
		link = p.linkPrefix + "/" + strings.Trim(strings.TrimSpace(payload.Slug), "/")
	}

	// Duplicate prevention: an existing recent post with the same message and
	// link means a previous attempt already succeeded (or landed before the
	// job was closed); record it rather than post again.
	recent, scanErr := p.ops.RecentPagePosts(ctx)
	if scanErr == nil {
		for _, post := range recent {
			if strings.TrimSpace(post.Message) == message && strings.TrimSpace(post.Link) == link {
				permalink, linkErr := p.ops.Permalink(ctx, post.ID)
				if linkErr == nil && permalink != "" {
					p.lastLink = permalink
				} else {
					p.lastLink = link
				}
				return PublishOutcome{ExternalID: post.ID}, nil
			}
		}
	}
	// A failed duplicate scan must not block publishing; the next combination
	// below still creates the post once per attempt.

	id, err := p.ops.CreatePagePost(ctx, message, link)
	if err != nil {
		return PublishOutcome{}, err
	}
	permalink, linkErr := p.ops.Permalink(ctx, id)
	if linkErr == nil && permalink != "" {
		p.lastLink = permalink
	} else if link != "" {
		p.lastLink = link
	}
	return PublishOutcome{ExternalID: id}, nil
}

func (p *MetaPublisher) publishInstagram(ctx context.Context, payload ContentPayload) (PublishOutcome, error) {
	if payload.MainImage == nil || strings.TrimSpace(payload.MainImage.URL) == "" {
		return PublishOutcome{}, &MetaError{
			Kind: "validation",
			Safe: "Instagram requires an image: add an image to this version, upload it through a connected integration, and approve it again before publishing",
		}
	}
	caption := p.postMessage(payload, metaIGCaptionLimit)
	container, err := p.ops.CreateIGContainer(ctx, strings.TrimSpace(payload.MainImage.URL), caption)
	if err != nil {
		return PublishOutcome{}, err
	}
	mediaID, err := p.ops.PublishIGMedia(ctx, container)
	if err != nil {
		// A container that was created but never published is invisible to
		// the public feed; the retry re-creates it, which Meta documents as
		// the only safe automated path.
		return PublishOutcome{}, err
	}
	permalink, linkErr := p.ops.Permalink(ctx, mediaID)
	if linkErr == nil && permalink != "" {
		p.lastLink = permalink
	}
	return PublishOutcome{ExternalID: mediaID}, nil
}

// VerifyPublicURL records the real post permalink as the artifact URL. The
// website argument is ignored: for a social network the destination URL is
// the post, not a page of the client website.
func (p *MetaPublisher) VerifyPublicURL(ctx context.Context, websiteURL, slug string) URLVerification {
	link := strings.TrimSpace(p.lastLink)
	if link == "" {
		return URLVerification{}
	}
	return URLVerification{URL: link, Verified: true}
}

// ErrNoDestinationDraft marks publishers whose destination has no external
// draft step (Meta): the approval gate in this platform is the review path,
// and publishing writes the public post directly. The worker skips the draft
// step for such publishers instead of failing the job.
var ErrNoDestinationDraft = draftlessDestination{}

type draftlessDestination struct{}

func (draftlessDestination) Error() string {
	return "this destination has no external draft step; the in-app approval is the review path"
}

func (draftlessDestination) SafeMessage() string {
	return "this destination has no external draft step; the in-app approval is the review path"
}

var _ SafePublisherError = ErrNoDestinationDraft

func lowerTrim(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func truncateMetaRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
