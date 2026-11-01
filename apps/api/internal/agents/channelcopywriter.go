package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"marketingops/api/internal/aiprovider"
)

// ---- Channel-copy-writer specialist profile ---------------------------------

// ChannelCopyWriterID and ChannelCopyWriterVersion identify the curated
// profile.
const (
	ChannelCopyWriterID      = "channel_copy_writer"
	ChannelCopyWriterVersion = "1"
)

// The closed set of channels the platform's publishers actually read: a reply
// may draft copy for any subset of these, and any other channel key is a
// malformed reply, not a new channel.
var channelCopyChannels = map[string]bool{
	"facebook":  true,
	"instagram": true,
	"x":         true,
	"youtube":   true,
}

// maxChannelCopyMessageRunes is the per-message cap for every channel. It is
// the platform's tightest consumer cap (the 280-character tweet), applied
// uniformly so one validator covers all consumers; overflow is refused, not
// truncated, so the operator never sees silently-shortened copy.
const maxChannelCopyMessageRunes = 280

// ChannelCopyPlan is the validated output of one channel-copy run: per-channel
// message text filed into an existing draft item's newest version via the
// file_channel_copy executor. It never creates items and never changes an
// item's status or approval.
type ChannelCopyPlan struct {
	AgentID     string            `json:"agentId"`
	Version     string            `json:"agentVersion"`
	Provider    string            `json:"provider"`
	Model       string            `json:"model"`
	GeneratedAt time.Time         `json:"generatedAt"`
	Channels    map[string]string `json:"channels"`
}

// ChannelCopyWriter drafts short per-channel copy variants (Facebook,
// Instagram, X, YouTube) for one existing draft content item, adapting the
// item's approved-in-review brief instead of writing new prose. Like every
// specialist it is bounded: facts from the draft and workspace context only,
// output validated before it is filed anywhere.
type ChannelCopyWriter struct{}

// ID and Version report the profile identity recorded in task history.
func (ChannelCopyWriter) ID() string      { return ChannelCopyWriterID }
func (ChannelCopyWriter) Version() string { return ChannelCopyWriterVersion }

// DisplayName and Description are the dashboard-facing identity of this
// curated profile.
func (ChannelCopyWriter) DisplayName() string { return "Channel copy writer" }

func (ChannelCopyWriter) Description() string {
	return "Drafts per-channel copy variants for an existing draft content item, filed into that item's version for review."
}

// OutputContract, WantsData, and AllowedActions declare the governance facts
// of this specialist: channel-copy output filed via the file_channel_copy
// executor, and no external data needs.
func (ChannelCopyWriter) OutputContract() string   { return ContractChannelCopy }
func (ChannelCopyWriter) WantsData() []string      { return nil }
func (ChannelCopyWriter) AllowedActions() []string { return []string{ActionFileChannelCopy} }

// Run performs one bounded channel-copy pass. The file_channel_copy executor
// is applied after the run, by the route; this method only produces the
// validated plan. RunInput.Focus carries the target draft's bounded context
// (title, body excerpt) prepared by the route before any model call.
func (c ChannelCopyWriter) Run(ctx context.Context, run PromptRun, input RunInput) (RunOutput, error) {
	plan, err := c.Draft(ctx, run, AgentInput{
		Workspace: input.Workspace,
		Focus:     input.Focus,
	})
	if err != nil {
		return RunOutput{}, err
	}
	return RunOutput{Copy: &plan}, nil
}

// Draft runs one channel-copy pass: build the bounded prompt, transport it via
// the provider, then validate the reply before it is filed anywhere.
func (c ChannelCopyWriter) Draft(ctx context.Context, run PromptRun, input AgentInput) (ChannelCopyPlan, error) {
	reply, err := run.RunPrompt(ctx, aiprovider.RunPromptRequest{
		System:    c.systemPrompt(),
		User:      c.buildUserPrompt(input),
		MaxTokens: 2048,
	})
	if err != nil {
		return ChannelCopyPlan{}, err
	}
	plan, err := channelCopyFromReply(reply)
	if err != nil {
		return ChannelCopyPlan{}, err
	}
	plan.AgentID = c.ID()
	plan.Version = c.Version()
	plan.GeneratedAt = time.Now().UTC()
	if plan.Provider == "" {
		plan.Provider = reply.Provider
		plan.Model = reply.Model
	}
	return plan, nil
}

// systemPrompt is the curated, versioned specialist profile for per-channel
// copy adaptation, independent of (and complementary to) the drafting and
// proposal specialists.
func (c ChannelCopyWriter) systemPrompt() string {
	return strings.Join([]string{
		`You are the internal channel copy writer of a marketing operations platform. You draft short per-channel copy variants for one existing draft content item, filed into that item for a human operator to review and approve before anything is published.`,
		`Rules you must never break:`,
		`1. FILE NOTHING NEW. You adapt the provided draft's message for the named channels; you never invent a new topic, publish, schedule, send, or claim that anything was published. The item's status and approval are not yours to change.`,
		`2. FACTS FROM THE DRAFT ONLY. Base every message strictly on the provided draft text and workspace context. Invent nothing about the business's history, staff, location, pricing, achievements, or audience size; where a real detail would be needed, keep the message generic instead.`,
		`3. NO PERFORMANCE DATA. You are given no Search Console or analytics rows; never cite positions, clicks, rankings, traffic, or any measured number.`,
		`4. RESPECT THE BRAND. Match the workspace's tone and audience and use no claim from the "Do not make these claims" list.`,
		`5. OUTPUT. Answer with a single JSON object and nothing else, with exactly this key:`,
		`{"channelCopy": {"facebook": string, "instagram": string, "x": string, "youtube": string}}`,
		`"channelCopy" maps channel names to message text. Only these channel keys are allowed; omit a channel entirely when its adjective was not requested or would be empty — do not send an empty string. Every message is ready to post as-is: no channel name prefix, no "[copy]" labels, plain text without Markdown headings, at most 280 characters, and one short call to action when the draft has one.`,
	}, "\n")
}

// buildUserPrompt renders the bounded, non-secret context of this run.
// Credentials, tokens, and any secret-derived data never appear here. The
// route already bounded the target draft's title/body excerpt into Focus.
func (c ChannelCopyWriter) buildUserPrompt(input AgentInput) string {
	var out strings.Builder
	ws := input.Workspace
	fmt.Fprintf(&out, "Business: %s", fallback(ws.Name, "(unknown business)"))
	if ws.WebsiteURL != "" {
		fmt.Fprintf(&out, " (%s)", ws.WebsiteURL)
	}
	out.WriteString("\n")
	if ws.Description != "" {
		fmt.Fprintf(&out, "About: %s\n", ws.Description)
	}
	if ws.Tone != "" {
		fmt.Fprintf(&out, "Tone: %s\n", ws.Tone)
	}
	if len(ws.ProhibitedClaims) > 0 {
		fmt.Fprintf(&out, "Do not make these claims: %s\n", strings.Join(ws.ProhibitedClaims, "; "))
	}
	if strings.TrimSpace(input.Focus) != "" {
		fmt.Fprintf(&out, "%s\n", strings.TrimSpace(input.Focus))
	}
	return out.String()
}

// validChannelCopyChannels returns the allowed keys in sorted order, for error
// messages and tests.
func validChannelCopyChannels() []string {
	out := make([]string, 0, len(channelCopyChannels))
	for name := range channelCopyChannels {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// channelCopyFromReply turns raw provider text into a validated
// ChannelCopyPlan. The channel key set is closed: an unknown key is a
// malformed reply, not a channel to invent.
func channelCopyFromReply(reply aiprovider.PromptReply) (ChannelCopyPlan, error) {
	var parsed struct {
		ChannelCopy map[string]string `json:"channelCopy"`
	}
	if err := json.Unmarshal(extractJSONObject([]byte(reply.Text)), &parsed); err != nil {
		return ChannelCopyPlan{}, &aiprovider.ProviderError{
			Kind:  aiprovider.FailMalformed,
			Cause: "the channel-copy reply could not be structured into a plan",
		}
	}
	if len(parsed.ChannelCopy) == 0 {
		return ChannelCopyPlan{}, &aiprovider.ProviderError{
			Kind:  aiprovider.FailMalformed,
			Cause: "the channel-copy reply contained no channel copy",
		}
	}
	channels := make(map[string]string, len(parsed.ChannelCopy))
	for name, message := range parsed.ChannelCopy {
		key := strings.ToLower(strings.TrimSpace(name))
		if !channelCopyChannels[key] {
			return ChannelCopyPlan{}, &aiprovider.ProviderError{
				Kind: aiprovider.FailMalformed,
				Cause: fmt.Errorf("the channel-copy reply used a channel outside the supported set %v: %q",
					validChannelCopyChannels(), name).Error(),
			}
		}
		if strings.TrimSpace(message) == "" {
			continue
		}
		if runeCount(message) > maxChannelCopyMessageRunes {
			return ChannelCopyPlan{}, &aiprovider.ProviderError{
				Kind: aiprovider.FailMalformed,
				Cause: fmt.Errorf("the %s copy message exceeds the %d-character cap (%d)",
					key, maxChannelCopyMessageRunes, runeCount(message)).Error(),
			}
		}
		channels[key] = strings.TrimSpace(message)
	}
	if len(channels) == 0 {
		return ChannelCopyPlan{}, &aiprovider.ProviderError{
			Kind:  aiprovider.FailMalformed,
			Cause: "the channel-copy reply had only blank messages",
		}
	}

	metadata := map[string]any{}
	for key, value := range reply.Metadata {
		if isScalar(value) {
			metadata[key] = value
		}
	}
	return ChannelCopyPlan{
		Channels: channels,
		Provider: stringFromMetadata(metadata, "provider"),
		Model:    stringFromMetadata(metadata, "model"),
	}, nil
}
