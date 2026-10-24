package aiprovider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenRouter implements Provider against OpenRouter's OpenAI-compatible chat
// completions API (https://openrouter.ai/api/v1). One API key routes to many
// models; the model name (e.g. "anthropic/claude-3.5-haiku") is configuration.
type OpenRouter struct {
	APIKey string
	Model  string
	Base   string // default https://openrouter.ai/api/v1; override in tests
	Client *http.Client
	// MaxTokens caps the completion size. OpenRouter charges against a
	// token budget even before generation; without a cap it assumes the
	// model maximum, which fails with HTTP 402 on low-credit keys.
	MaxTokens int
}

func (o OpenRouter) Name() string { return "openrouter" }

// effectiveMaxTokens: an explicit cap, or a default that covers a full
// article with room to spare while fitting typical pay-as-you-go budgets.
const defaultMaxTokens = 4000

func effectiveMaxTokens(configured int) int {
	if configured > 0 && configured < 64000 {
		return configured
	}
	return defaultMaxTokens
}

// draftPayload is the JSON structure the model is asked to answer with.
type draftPayload struct {
	Title        string            `json:"title"`
	Slug         string            `json:"slug"`
	Excerpt      string            `json:"excerpt"`
	Body         string            `json:"body"`
	CallToAction string            `json:"callToAction"`
	ChannelCopy  map[string]string `json:"channelCopy"`
}

// replyShape is the subset of the OpenAI-compatible reply we need.
type replyShape struct {
	Choices []struct {
		Message struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (o OpenRouter) Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error) {
	started := time.Now()
	content, err := o.chatComplete(ctx, buildSystemPrompt(req), buildUserPrompt(req), effectiveMaxTokens(o.MaxTokens))
	if err != nil {
		return GenerateResponse{}, err
	}
	// Keep only a hash of the raw reply for debugging; never store its text.
	choiceHash := sha256.Sum256(content)

	draft, err := parseDraftJSON(content)
	if err != nil {
		return GenerateResponse{}, &ProviderError{Kind: FailMalformed, Cause: err.Error()}
	}
	if strings.TrimSpace(draft.Body) == "" {
		return GenerateResponse{}, &ProviderError{Kind: FailEmpty, Cause: "the draft body was blank"}
	}

	channelCopy := make(map[string]string, len(draft.ChannelCopy))
	for name, copy := range draft.ChannelCopy {
		if name = strings.ToLower(strings.TrimSpace(name)); name != "" {
			channelCopy[name] = copy
		}
	}

	return GenerateResponse{
		Provider:     "openrouter",
		Model:        o.Model,
		Title:        strings.TrimSpace(draft.Title),
		Slug:         slugify(draft.Slug),
		Excerpt:      strings.TrimSpace(draft.Excerpt),
		Body:         draft.Body,
		CallToAction: strings.TrimSpace(draft.CallToAction),
		ChannelCopy:  channelCopy,
		Metadata: map[string]any{
			"provider":     "openrouter",
			"model":        o.Model,
			"duration_ms":  time.Since(started).Milliseconds(),
			"reply_sha256": hex.EncodeToString(choiceHash[:]),
		},
	}, nil
}

// RunPrompt performs one free-form system+user completion and returns the raw
// reply text without parsing it into a draft shape. The internal agent
// specialists use this to request structured advisory reports.
func (o OpenRouter) RunPrompt(ctx context.Context, req RunPromptRequest) (PromptReply, error) {
	started := time.Now()
	content, err := o.chatComplete(ctx, req.System, req.User, effectiveMaxTokens(req.MaxTokens))
	if err != nil {
		return PromptReply{}, err
	}
	replyHash := sha256.Sum256(content)
	return PromptReply{
		Provider: "openrouter",
		Model:    o.Model,
		Text:     string(content),
		Metadata: map[string]any{
			"provider":     "openrouter",
			"model":        o.Model,
			"duration_ms":  time.Since(started).Milliseconds(),
			"reply_sha256": hex.EncodeToString(replyHash[:]),
		},
	}, nil
}

// chatComplete sends one chat-completions call and returns the first
// choice's message content. All classified failures are the established
// ProviderError kinds; the error never carries keys or raw provider payloads.
func (o OpenRouter) chatComplete(ctx context.Context, system, user string, maxTokens int) ([]byte, error) {
	base := o.Base
	if base == "" {
		base = "https://openrouter.ai/api/v1"
	}
	client := o.Client
	if client == nil {
		client = http.DefaultClient
	}

	payload := map[string]any{
		"model":      o.Model,
		"max_tokens": maxTokens,
		"messages": []map[string]any{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"response_format": map[string]any{"type": "json_object"},
		"temperature":     0.7,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, &ProviderError{Kind: FailMalformed, Cause: "internal payload"}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, &ProviderError{Kind: FailNetwork, Cause: "could not build the request"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+o.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	// Static, non-secret attribution header per OpenRouter recommendations.
	httpReq.Header.Set("X-Title", "marketing-ops-platform")

	httpResp, err := client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, &ProviderError{Kind: FailTimeout, Cause: "no reply within the timeout"}
		}
		return nil, &ProviderError{Kind: FailNetwork, Cause: "network error before or during the call"}
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(httpResp.Body, 4*1024*1024))
	if err != nil {
		return nil, &ProviderError{Kind: FailNetwork, Cause: "could not read the reply body"}
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return nil, &ProviderError{Kind: FailProvider,
			Cause: fmt.Sprintf("HTTP %d: %s", httpResp.StatusCode, firstSafeLine(body))}
	}

	content, err := decodeReply(body)
	if err != nil {
		return nil, &ProviderError{Kind: FailMalformed, Cause: err.Error()}
	}
	return content, nil
}

// decodeReply returns the message content of the first choice as bytes.
func decodeReply(body []byte) ([]byte, error) {
	var reply replyShape
	if err := json.Unmarshal(body, &reply); err != nil {
		return nil, errors.New("the reply was not the expected JSON shape")
	}
	if len(reply.Choices) == 0 || len(reply.Choices[0].Message.Content) == 0 {
		return nil, errors.New("the reply contained no message content")
	}
	// Content is usually a JSON string; decode it to its inner text.
	var text string
	if err := json.Unmarshal(reply.Choices[0].Message.Content, &text); err == nil {
		return []byte(text), nil
	}
	// Some providers return structured content; pass the raw bytes on.
	return reply.Choices[0].Message.Content, nil
}

// parseDraftJSON turns the model's text into a draftPayload. Models wrap JSON
// in code fences or chatter despite instructions, so extract the outermost
// JSON object first.
func parseDraftJSON(text []byte) (draftPayload, error) {
	var draft draftPayload
	extracted := extractJSONObject(text)
	if len(extracted) == 0 {
		return draft, errors.New("no JSON object found in the reply")
	}
	if err := json.Unmarshal(extracted, &draft); err != nil {
		return draft, errors.New("the JSON object was not a valid draft")
	}
	return draft, nil
}

func extractJSONObject(text []byte) []byte {
	start := bytes.IndexByte(text, '{')
	if start < 0 {
		return nil
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(text); i++ {
		c := text[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return text[start : i+1]
			}
		}
	}
	return nil
}

// firstSafeLine summarizes a non-2xx body in one short, key-free line.
func firstSafeLine(body []byte) string {
	line := string(body)
	if idx := bytes.IndexByte(body, '\n'); idx >= 0 {
		line = line[:idx]
	}
	if len(line) > 200 {
		line = line[:200]
	}
	return strings.TrimSpace(line)
}

func buildSystemPrompt(req GenerateRequest) string {
	var out strings.Builder
	out.WriteString("You are a marketing copywriter producing drafts for a small business. ")
	out.WriteString("Never publish anything yourself; you only produce text for human review. ")
	out.WriteString("Answer with a single JSON object and nothing else, with exactly these keys: ")
	out.WriteString(`{"title", "slug", "excerpt", "body", "callToAction", "channelCopy"}. `)
	out.WriteString(`"slug" must be lowercase letters, digits, and hyphens only. `)
	out.WriteString(`"channelCopy" must be an object of channel name to short copy string, and may be {}.`)
	if req.Kind == "blog_article" {
		out.WriteString(` "body" should be a complete article in markdown, around 600-900 words.`)
	} else if req.Kind == "social_post" {
		out.WriteString(` "body" should be one short post, a few sentences at most.`)
	} else {
		out.WriteString(` "body" should be a compact snippet of text.`)
	}
	out.WriteString(" Respect the prohibited claims list; if the topic conflicts with it, pick a compliant angle.")
	return out.String()
}

func buildUserPrompt(req GenerateRequest) string {
	ws := req.Workspace
	var out strings.Builder
	fmt.Fprintf(&out, "Business: %s", fallback(ws.Name, "(unknown business)"))
	if ws.WebsiteURL != "" {
		fmt.Fprintf(&out, " (%s)", ws.WebsiteURL)
	}
	out.WriteString("\n")
	if ws.Description != "" {
		fmt.Fprintf(&out, "About: %s\n", ws.Description)
	}
	if ws.Audience != "" {
		fmt.Fprintf(&out, "Audience: %s\n", ws.Audience)
	}
	if len(ws.Services) > 0 {
		fmt.Fprintf(&out, "Services: %s\n", strings.Join(ws.Services, ", "))
	}
	if ws.Tone != "" {
		fmt.Fprintf(&out, "Tone: %s\n", ws.Tone)
	}
	if len(ws.ProhibitedClaims) > 0 {
		fmt.Fprintf(&out, "Do not make these claims: %s\n", strings.Join(ws.ProhibitedClaims, "; "))
	}
	if len(ws.CallsToAction) > 0 {
		fmt.Fprintf(&out, "Preferred calls to action: %s\n", strings.Join(ws.CallsToAction, "; "))
	}
	fmt.Fprintf(&out, "\nContent kind: %s\n", req.Kind)
	fmt.Fprintf(&out, "Topic / brief: %s\n", req.Topic)
	if req.Channel != "" {
		fmt.Fprintf(&out, "Target channel (also put its copy in channelCopy): %s\n", req.Channel)
	}
	if req.Instructions != "" {
		fmt.Fprintf(&out, "Extra instructions from the operator: %s\n", strings.TrimSpace(req.Instructions))
	}
	fmt.Fprintf(&out, "\nReturn only the JSON object.")
	return out.String()
}
