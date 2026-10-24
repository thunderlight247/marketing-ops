package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// The YouTube publishing adapter mirrors the Meta one: NewPublisher takes
// non-secret config plus already-decrypted credentials and returns a
// Publisher bound to one authorized YouTube channel.
type YouTubePublisherAdapter struct{}

func (YouTubePublisherAdapter) NewPublisher(config map[string]any, credentials map[string]string, apiBase string, client *http.Client) (Publisher, error) {
	if strings.TrimSpace(stringConfig(config, "channelId")) == "" {
		return nil, ErrPublisherNotConfigured
	}
	if strings.TrimSpace(credentials["refreshToken"]) == "" ||
		strings.TrimSpace(credentials["clientId"]) == "" ||
		strings.TrimSpace(credentials["clientSecret"]) == "" {
		return nil, ErrPublisherCredentialsMissing
	}
	// The shared apiBase argument points at the workspace website API (used
	// by Sanity), not Google, so the Google endpoints stay at the ops
	// defaults and only the HTTP client override is honored (tests).
	ops := youtubeOpsConstructor(config, credentials)
	if client != nil {
		ops.Client = client
	}
	return &YouTubePublisher{ops: ops}, nil
}

// YouTubePublisher implements the platform-neutral Publisher contract for one
// authorized YouTube channel.
//
// Milestone 9 honesty notes:
//   - CreateDraft is the draftless sentinel: an uploaded video is
//     public-facing as soon as the upload completes, so there is no external
//     draft step. The dashboard preview plus the approval gate is the review
//     path.
//   - The video file itself is fetched server-side from the approved
//     version's videoUrl metadata and streamed through a resumable upload
//     session; no bytes are stored by this platform.
//   - videos.insert has no idempotent upsert: a retry of an attempt that
//     reached the upload can create a second video. The job history shows
//     both occurrences (known limitation, like Instagram's two-step flow).
//   - The resulting watch URL is composed from the video id the API returns.
type YouTubePublisher struct {
	ops *YouTubeOps
	// lastVideoID remembers the most recent successful upload so
	// VerifyPublicURL can hand the real watch URL to the artifact record.
	lastVideoID string
}

var (
	_ Publisher         = (*YouTubePublisher)(nil)
	_ PublishingAdapter = YouTubePublisherAdapter{}
)

func (p *YouTubePublisher) CreateDraft(ctx context.Context, payload ContentPayload) (string, string, error) {
	return "", "", ErrNoDestinationDraft
}

func (p *YouTubePublisher) Publish(ctx context.Context, payload ContentPayload) (PublishOutcome, error) {
	source := strings.TrimSpace(payload.VideoURL)
	if source == "" {
		return PublishOutcome{}, &YouTubeError{
			Kind: "validation",
			Safe: "YouTube requires a video file: add a videoUrl to this version (a public URL to the .mp4/.mov file), approve it again, then publish",
		}
	}

	title := truncateMetaRunes(strings.TrimSpace(payload.Title), youtubeTitleLimit)
	description := p.videoDescription(payload)
	if title == "" && description == "" {
		return PublishOutcome{}, &YouTubeError{Kind: "validation", Safe: "there is nothing to upload: the approved version has an empty title and description"}
	}

	token, err := p.ops.AccessToken(ctx)
	if err != nil {
		return PublishOutcome{}, err
	}

	// Fetch the source video with the same context as the upload so a
	// cancelled job stops the download too. The file is streamed — never
	// buffered whole — and the source must report its length.
	srcReq, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return PublishOutcome{}, &YouTubeError{Kind: "validation", Safe: "the videoUrl in this version is not a usable URL"}
	}
	srcResp, err := p.ops.Client.Do(srcReq)
	if err != nil {
		return PublishOutcome{}, &YouTubeError{Kind: "network", Safe: "the video file could not be downloaded from the videoUrl in this version"}
	}
	defer func() { _ = srcResp.Body.Close() }()
	contentType := strings.TrimSpace(srcResp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if srcResp.StatusCode != http.StatusOK {
		return PublishOutcome{}, &YouTubeError{
			Kind: "validation",
			Safe: fmt.Sprintf("the video file could not be downloaded from the videoUrl in this version (HTTP %d)", srcResp.StatusCode),
		}
	}
	length := strings.TrimSpace(srcResp.Header.Get("Content-Length"))
	if length == "" {
		return PublishOutcome{}, &YouTubeError{
			Kind: "validation",
			Safe: "the video source does not report its file size; host the video where a Content-Length header is sent",
		}
	}
	lengthBytes, lengthErr := strconv.ParseInt(length, 10, 64)
	if lengthErr != nil || lengthBytes < 0 {
		return PublishOutcome{}, &YouTubeError{Kind: "validation", Safe: "the video source reported an unreadable file size"}
	}

	videoID, err := p.ops.upload(ctx, token, title, description, contentType, lengthBytes, srcResp.Body)
	if err != nil {
		return PublishOutcome{}, err
	}
	p.lastVideoID = videoID
	return PublishOutcome{ExternalID: videoID}, nil
}

// videoDescription picks the operator's channel copy when it exists, falling
// back to the article title and excerpt, capped at the documented limit.
func (p *YouTubePublisher) videoDescription(payload ContentPayload) string {
	description := strings.TrimSpace(payload.ChannelCopy["youtube"])
	if description == "" {
		description = strings.TrimSpace(payload.Title)
		if excerpt := strings.TrimSpace(payload.Excerpt); excerpt != "" {
			if description == "" {
				description = excerpt
			} else {
				description += "\n\n" + excerpt
			}
		}
	}
	return truncateMetaRunes(description, youtubeDescriptionLimit)
}

// resumableUpload starts one resumable upload session and streams the video
// body to it in a single PUT, returning the new video id.
func (o *YouTubeOps) upload(ctx context.Context, token, title, description, contentType string, contentLength int64, body io.Reader) (string, error) {
	meta := map[string]any{
		"snippet": map[string]any{
			"title":       title,
			"description": description,
			"categoryId":  o.Category,
		},
		"status": map[string]any{
			"privacyStatus": o.Privacy,
		},
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return "", youtubeUnexpectedErr()
	}

	initURL := fmt.Sprintf("%s/upload/youtube/v3/videos?uploadType=resumable&part=snippet,status&notifySubscribers=%t",
		o.APIBase, o.Notify)
	initReq, err := http.NewRequestWithContext(ctx, http.MethodPost, initURL, strings.NewReader(string(metaJSON)))
	if err != nil {
		return "", youtubeNetworkErr()
	}
	initReq.Header.Set("Authorization", "Bearer "+token)
	initReq.Header.Set("Content-Type", "application/json; charset=UTF-8")
	initReq.Header.Set("X-Upload-Content-Type", contentType)
	initReq.Header.Set("X-Upload-Content-Length", strconv.FormatInt(contentLength, 10))
	initBody, initStatus, initHeader, err := o.do(initReq)
	if err != nil {
		return "", err
	}
	if initStatus != http.StatusOK && initStatus != http.StatusCreated {
		return "", classifyYouTubeResponse(initStatus, initBody)
	}
	sessionURL := strings.TrimSpace(initHeader.Get("Location"))
	if sessionURL == "" {
		return "", youtubeUnexpectedErr()
	}

	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, sessionURL, body)
	if err != nil {
		return "", youtubeNetworkErr()
	}
	putReq.Header.Set("Authorization", "Bearer "+token)
	putReq.Header.Set("Content-Type", contentType)
	putReq.ContentLength = contentLength
	putBody, putStatus, _, err := o.do(putReq)
	if err != nil {
		return "", err
	}
	if putStatus != http.StatusOK && putStatus != http.StatusCreated {
		return "", classifyYouTubeResponse(putStatus, putBody)
	}
	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(putBody, &parsed); err != nil || strings.TrimSpace(parsed.ID) == "" {
		return "", youtubeUnexpectedErr()
	}
	return strings.TrimSpace(parsed.ID), nil
}

// VerifyPublicURL records the composed watch URL as the artifact URL. The
// website argument is ignored: for a social network the destination URL is
// the video, not a page of the client website.
func (p *YouTubePublisher) VerifyPublicURL(ctx context.Context, websiteURL, slug string) URLVerification {
	id := strings.TrimSpace(p.lastVideoID)
	if id == "" {
		return URLVerification{}
	}
	watchURL := "https://www.youtube.com/watch?v=" + url.QueryEscape(id)
	return URLVerification{URL: watchURL, Verified: true}
}
