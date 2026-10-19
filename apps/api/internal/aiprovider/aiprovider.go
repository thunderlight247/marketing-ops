// Package aiprovider abstracts model providers used for AI-assisted drafting.
//
// The interface is deliberately small so a provider can be swapped without
// touching content workflows. All invocation happens server-side; keys never
// reach the browser, and metadata stored for debugging must stay free of
// secrets.
package aiprovider

import (
	"context"
	"fmt"
	"strings"
)

// WorkspaceContext is the non-secret workspace side of a generation prompt.
type WorkspaceContext struct {
	Name             string
	WebsiteURL       string
	Description      string
	Audience         string
	Services         []string
	Tone             string
	ProhibitedClaims []string
	CallsToAction    []string
}

// GenerateRequest is one drafting ask. Topic is the operator's brief.
type GenerateRequest struct {
	Kind         string // blog_article | social_post | snippet
	Topic        string
	Channel      string // optional per-channel variant target (e.g. instagram)
	Instructions string // optional extra operator guidance
	Workspace    WorkspaceContext
}

// GenerateResponse is one structured draft produced by a provider.
type GenerateResponse struct {
	Provider     string
	Model        string
	Title        string
	Slug         string
	Excerpt      string
	Body         string
	CallToAction string
	ChannelCopy  map[string]string
	Metadata     map[string]any
}

// Failure kinds for ProviderError.Kind.
const (
	FailTimeout     = "timeout"     // context deadline exceeded
	FailNetwork     = "network"     // transport error around the call
	FailProvider    = "provider"    // the API answered with a non-2xx status
	FailMalformed   = "malformed"   // reply arrived but could not be structured
	FailEmpty       = "empty"       // reply was structured but had no usable content
	FailNotReady    = "not_ready"   // no provider is configured for the request
	FailUnsupported = "unsupported" // kind the provider does not draft
)

// ProviderError is a safe, classified failure. Its text never includes keys
// or raw provider payloads.
type ProviderError struct {
	Kind  string
	Cause string
}

func (e *ProviderError) Error() string {
	switch e.Kind {
	case FailTimeout:
		return "the model did not answer in time"
	case FailNetwork:
		return "the model provider could not be reached"
	case FailProvider:
		return "the model provider refused the request"
	case FailMalformed:
		return "the model reply could not be structured into a draft"
	case FailEmpty:
		return "the model returned no usable content"
	case FailNotReady:
		return "no AI provider is configured"
	case FailUnsupported:
		return "the requested content kind is not supported by the provider"
	default:
		return "generation failed"
	}
}

// Provider generates one structured draft per call.
type Provider interface {
	Name() string
	Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error)
}

// StubProvider is a deterministic, offline provider for development and
// tests. It drafts instantly, never touches the network, and labels output
// so no generated text is mistaken for a real model's words.
type StubProvider struct{}

func (StubProvider) Name() string { return "stub" }

func (s StubProvider) Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error) {
	if err := ctx.Err(); err != nil {
		return GenerateResponse{}, &ProviderError{Kind: FailTimeout, Cause: err.Error()}
	}
	if strings.TrimSpace(req.Topic) == "" {
		return GenerateResponse{}, &ProviderError{Kind: FailEmpty, Cause: "empty topic"}
	}
	switch req.Kind {
	case "blog_article", "social_post", "snippet":
	default:
		return GenerateResponse{}, &ProviderError{Kind: FailUnsupported, Cause: req.Kind}
	}

	var body strings.Builder
	if req.Kind == "blog_article" {
		body.WriteString(fmt.Sprintf("# %s\n\n", req.Topic))
	}
	body.WriteString(fmt.Sprintf(
		"[Stub draft] A placeholder article about %q for %s. Configure a real AI provider key to replace this text.",
		req.Topic, fallback(req.Workspace.Name, "the workspace")))
	if req.Channel != "" {
		body.WriteString(fmt.Sprintf(" Formatted for %s.", req.Channel))
	}

	channelCopy := map[string]string{}
	if req.Channel != "" {
		channelCopy[req.Channel] = fmt.Sprintf("[Stub] copy for %s on %s", req.Topic, req.Channel)
	}

	return GenerateResponse{
		Provider:     "stub",
		Model:        "stub-v1",
		Title:        fmt.Sprintf("[Stub] %s", req.Topic),
		Slug:         slugify(req.Topic),
		Excerpt:      fmt.Sprintf("[Stub] short excerpt about %s.", req.Topic),
		Body:         body.String(),
		CallToAction: firstCTA(req.Workspace.CallsToAction),
		ChannelCopy:  channelCopy,
		Metadata: map[string]any{
			"provider": "stub",
			"model":    "stub-v1",
		},
	}, nil
}

func fallback(value, def string) string {
	if strings.TrimSpace(value) == "" {
		return def
	}
	return value
}

func firstCTA(list []string) string {
	for _, cta := range list {
		if strings.TrimSpace(cta) != "" {
			return cta
		}
	}
	return ""
}

func slugify(value string) string {
	var out strings.Builder
	previousHyphen := true // suppresses a leading hyphen
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out.WriteRune(r)
			previousHyphen = false
		default:
			if !previousHyphen {
				out.WriteRune('-')
				previousHyphen = true
			}
		}
	}
	return strings.Trim(out.String(), "-")
}
