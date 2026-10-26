// Package aiprovider abstracts model providers used for AI-assisted drafting.
//
// The interface is deliberately small so a provider can be swapped without
// touching content workflows. All invocation happens server-side; keys never
// reach the browser, and metadata stored for debugging must stay free of
// secrets.
package aiprovider

import (
	"context"
	"encoding/json"
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

// PromptReply is one raw text completion, used by agent specialists that
// produce advisory reports instead of content drafts. Metadata is scalar
// only; keys and raw payloads never reach the logs.
type PromptReply struct {
	Provider string
	Model    string
	Text     string
	Metadata map[string]any // scalar values only
}

// RunPromptRequest is one free-form system+user completion ask. It is the
// provider half of the internal agent interface: agent specialists assemble
// the bounded prompts, the provider only transports them.
type RunPromptRequest struct {
	System    string
	User      string
	MaxTokens int // 0 selects the provider default
}

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

// stubAdvisoryMarker is the exact prompt-line prefix the agents package ends
// the user prompt with so the offline stub can state the data basis honestly
// instead of inventing metrics. Contract between this file and the agents
// package.
const stubAdvisoryMarker = "GSC_DATA_AVAILABILITY: "

// stubAdvisoryValueNone / stubAdvisoryValueGSC are the two marker values.
const (
	stubAdvisoryValueNone = "none"
	stubAdvisoryValueGSC  = "search_console"
)

// RunPrompt gives the offline stub a deterministic advisory reply. It states
// (from the marker) only whether Search Console data was available; it never
// fabricates metrics, positions, or cannibalization findings.
func (s StubProvider) RunPrompt(ctx context.Context, req RunPromptRequest) (PromptReply, error) {
	if err := ctx.Err(); err != nil {
		return PromptReply{}, &ProviderError{Kind: FailTimeout, Cause: err.Error()}
	}
	if strings.TrimSpace(req.System) == "" || strings.TrimSpace(req.User) == "" {
		return PromptReply{}, &ProviderError{Kind: FailEmpty, Cause: "empty prompt"}
	}

	if strings.HasPrefix(req.System, "You are the internal content creator ") ||
		strings.HasPrefix(req.System, "You are the internal search-gap writer ") {
		// The stub never names a target query: it cannot honestly confirm a
		// query from rows it does not read, and planFromReply enforces exactly
		// that (leaving it empty is valid in no-data runs).
		encoded, err := json.Marshal(map[string]any{
			"title":        "[Stub] Content draft for review",
			"slug":         "stub-content-draft",
			"excerpt":      "Offline placeholder for human review.",
			"body":         "[Stub] This is an offline plumbing check, not model-generated content. Configure a real AI provider and review all claims before approval.",
			"callToAction": "Review this draft.",
			"keyPoints":    []string{"Offline placeholder only", "Human review required"},
		})
		if err != nil {
			return PromptReply{}, &ProviderError{Kind: FailMalformed, Cause: "internal stub payload"}
		}
		return PromptReply{Provider: "stub", Model: "stub-v1", Text: string(encoded)}, nil
	}

	dataSource := stubAdvisoryValueNone
	cannibalizationChecked := false
	if strings.HasSuffix(strings.TrimSpace(req.User),
		"\n"+stubAdvisoryMarker+stubAdvisoryValueGSC) {
		dataSource = stubAdvisoryValueGSC
		cannibalizationChecked = true
	}

	var gscLine string
	if dataSource == stubAdvisoryValueGSC {
		gscLine = "The Search Console rows above were handed to this stub; it echoes availability only and does not read them."
	} else {
		gscLine = "No Search Console data is connected for this workspace yet."
	}

	report := map[string]any{
		"dataSource":             dataSource,
		"cannibalizationChecked": cannibalizationChecked,
		"summary": fmt.Sprintf(
			"[Stub advisory] Offline placeholder report for %s. Configure a real AI provider key to replace this text.",
			stubBusinessName(req.User)),
		"gaps": []string{
			"[Stub] This report comes from the offline stub provider, not a model; treat it as a plumbing check only.",
			"[Stub] " + gscLine,
		},
		"recommendations": []map[string]string{
			{
				"priority": "high",
				"area":     "connect_data",
				"issue":    "[Stub] No real specialist analysis is available while the offline stub provider is active.",
				"evidence": "[Stub] The stub never invents performance data and cannot read the workspace rows.",
				"action":   "Connect Search Console in this workspace and configure a real AI provider, then request the report again.",
			},
		},
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		return PromptReply{}, &ProviderError{Kind: FailMalformed, Cause: "internal stub payload"}
	}
	return PromptReply{
		Provider: "stub",
		Model:    "stub-v1",
		Text:     string(encoded),
		Metadata: map[string]any{
			"provider": "stub",
			"model":    "stub-v1",
		},
	}, nil
}

// stubBusinessName finds the Business line the prompt builder writes first;
// it only extracts a display name and never returns prompt bodies.
func stubBusinessName(user string) string {
	for _, line := range strings.Split(user, "\n") {
		if strings.HasPrefix(line, "Business: ") {
			return strings.TrimPrefix(line, "Business: ")
		}
	}
	return "the workspace"
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
