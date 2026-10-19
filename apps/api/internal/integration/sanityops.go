// Sanity write operations (Milestone 6): create/update drafts, publish, and
// verify the public URL — all server-side; the token never leaves the server
// anderror text is classified, never echoing credentials.
//
// Design notes:
//   - Draft document ids are deterministic ("drafts.mktops-<itemId>-v<num>"),
//     so re-submitting the same version replaces one draft instead of
//     accumulating duplicates (duplicate prevention).
//   - Publishing is a Sanity transaction: createOrReplace the published id
//     with the draft content, then delete the draft, then the workflow records
//     the published artifact. The public URL is only VERIFIED (read-only GET);
//     never assumed live after the mutation.
//   - Bodies travel as Portable Text blocks converted from the draft's saved
//     markdown-ish body. Field names default to the pilot schema (`post` body
//     as one document of blocks) and follow the integration's `docType`.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// WriteFailure kinds for SanityWriteError.Kind.
const (
	SanityFailNetwork   = "network"
	SanityFailProvider  = "provider" // non-2xx from the API (402 credits, 403 token, ...)
	SanityFailMalformed = "malformed"
	SanityFailDraftGone = "draft_gone" // the draft document disappeared before publish
	SanityFailTimeout   = "timeout"
)

// SanityWriteError is a classified, credential-free failure.
type SanityWriteError struct {
	Kind string
	HTTP int
	Text string
}

func (e *SanityWriteError) Error() string {
	switch e.Kind {
	case SanityFailNetwork:
		return "the Sanity API could not be reached"
	case SanityFailProvider:
		return fmt.Sprintf("the Sanity API refused the request (HTTP %d)", e.HTTP)
	case SanityFailMalformed:
		return "the Sanity API reply could not be read"
	case SanityFailDraftGone:
		return "the Sanity draft no longer exists; create it again before publishing"
	case SanityFailTimeout:
		return "the Sanity API did not answer in time"
	default:
		return "the Sanity operation failed"
	}
}

// SanityCredentials carries connection facts + the write token.
type SanityCredentials struct {
	ProjectID  string
	Dataset    string
	Token      string
	APIVersion string // optional; default "v2024-10-14"
}

// SanityOps performs write operations; Base/Client are for tests.
type SanityOps struct {
	Creds  SanityCredentials
	Base   string // default https://<project>.api.sanity.io
	Client *http.Client
}

func (s SanityOps) apiBase() string {
	if s.Base != "" {
		return s.Base
	}
	return "https://" + s.Creds.ProjectID + ".api.sanity.io"
}

func (s SanityOps) version() string {
	if v := strings.TrimSpace(s.Creds.APIVersion); strings.HasPrefix(v, "v") {
		return v
	}
	return "v2024-10-14"
}

// ---- Portable Text -----------------------------------------------------------

// PTChildSpan is one text child of a Portable Text block.
type PTChildSpan struct {
	Type  string   `json:"_type"`
	Key   string   `json:"_key"`
	Text  string   `json:"text"`
	Marks []string `json:"marks,omitempty"`
}

// PTBlock is one Portable Text block.
type PTBlock struct {
	Type     string        `json:"_type"`
	Key      string        `json:"_key"`
	Style    string        `json:"style,omitempty"`
	Level    int           `json:"level,omitempty"`
	ListItem string        `json:"listItem,omitempty"`
	MarkDefs []MarkDef     `json:"markDefs,omitempty"`
	Children []PTChildSpan `json:"children"`
}

// MarkDef is, e.g., a "strong" mark.
type MarkDef struct {
	Key  string `json:"_key"`
	Type string `json:"_type"`
}

// SanityPost is the assembled document for one draft.
type SanityPost struct {
	DocType    string // default "post"
	Title      string
	Slug       string
	Excerpt    string
	BodyBlocks []PTBlock
	// Extra top-level fields the pilot site may require; raw passthrough.
	Extra map[string]any
}

// ConvertBodyToBlocks is a Go re-implementation of the web preview parser,
// producing Portable Text blocks. Soft-wrapped lines join into one paragraph;
// bold runs become _strong marks with markDefs.
func ConvertBodyToBlocks(body string) []PTBlock {
	var blocks []PTBlock
	keyN := 0
	nextKey := func() string {
		keyN++
		return fmt.Sprintf("mktk%d", keyN)
	}

	newBlock := func(style, listItem string, level int) (*PTBlock, func(string, bool)) {
		key := nextKey()
		block := &PTBlock{Type: "block", Key: key, Style: style, ListItem: listItem, Level: level}
		spanKeyIndex := 0
		addChild := func(text string, bold bool) {
			spanKeyIndex++
			child := PTChildSpan{Type: "span", Key: key + "s" + fmt.Sprint(spanKeyIndex), Text: text}
			mark := ""
			if bold {
				mark = key + "smark"
				child.Marks = []string{mark}
			}
			block.Children = append(block.Children, child)
		}
		// markDefs are filled when children are appended with marks; the def
		// itself is registered by the caller function below through markIn.
		return block, addChild
	}

	// lineToBlock builds one block from one labeled line, handling bold runs.
	build := func(style, listItem string, level int, text string) *PTBlock {
		block, addChild := newBlock(style, listItem, level)
		if text == "" {
			text = " "
		}
		parts := strings.Split(text, "**")
		markers := len(parts) - 1
		if len(parts) == 1 || markers%2 != 0 {
			// No closed pairs: single span, literal text.
			addChild(text, false)
		} else {
			for i, part := range parts {
				if part == "" {
					continue
				}
				addChild(part, i%2 == 1)
			}
		}
		// Register markDefs for any bold spans this block produced.
		for _, child := range block.Children {
			if len(child.Marks) > 0 {
				block.MarkDefs = append(block.MarkDefs, MarkDef{Key: child.Marks[0], Type: "strong"})
			}
		}
		return block
	}

	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			// blank line: paragraph separator, nothing to emit
		case strings.HasPrefix(trimmed, "####"), strings.HasPrefix(trimmed, "###"):
			blocks = append(blocks, *build("h4", "", 0, strings.TrimLeft(trimmed, "# ")))
		case strings.HasPrefix(trimmed, "##"):
			blocks = append(blocks, *build("h3", "", 0, strings.TrimPrefix(trimmed, "## ")))
		case strings.HasPrefix(trimmed, "#"):
			blocks = append(blocks, *build("h2", "", 0, strings.TrimPrefix(trimmed, "# ")))
		case strings.HasPrefix(trimmed, "- "):
			blocks = append(blocks, *build("normal", "bullet", 1, strings.TrimPrefix(trimmed, "- ")))
		case strings.HasPrefix(trimmed, "* "):
			blocks = append(blocks, *build("normal", "bullet", 1, strings.TrimPrefix(trimmed, "* ")))
		default:
			numbered := len(trimmed) > 2 && trimmed[0] >= '0' && trimmed[0] <= '9' &&
				(trimmed[1] == '.' || trimmed[1] == ')') && len(trimmed) > 2 &&
				(trimmed[2] == ' ' || (len(trimmed) > 2 && trimmed[2] == ' '))
			if numbered {
				blocks = append(blocks, *build("normal", "number", 1, strings.TrimPrefix(trimmed[2:], " ")))
				continue
			}
			blocks = append(blocks, *build("normal", "", 0, trimmed))
		}
	}
	return blocks
}

// ---- Mutate plumbing --------------------------------------------------------

type mutationEnvelope struct {
	Mutations []map[string]any `json:"mutations"`
}

type mutateResult struct {
	TransactionID string `json:"transactionId"`
}

func (s SanityOps) mutate(ctx context.Context, env mutationEnvelope) (string, error) {
	payload, err := json.Marshal(env)
	if err != nil {
		return "", &SanityWriteError{Kind: SanityFailMalformed}
	}
	url := fmt.Sprintf("%s/%s/data/mutate/%s?returnDocuments=false", s.apiBase(), s.version(), s.Creds.Dataset)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", &SanityWriteError{Kind: SanityFailNetwork}
	}
	req.Header.Set("Authorization", "Bearer "+s.Creds.Token)
	req.Header.Set("Content-Type", "application/json")

	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return "", &SanityWriteError{Kind: SanityFailTimeout}
		}
		return "", &SanityWriteError{Kind: SanityFailNetwork}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return "", &SanityWriteError{Kind: SanityFailMalformed}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &SanityWriteError{Kind: SanityFailProvider, HTTP: resp.StatusCode,
			Text: firstSafe(body)}
	}
	var result mutateResult
	if err := json.Unmarshal(body, &result); err != nil {
		return "", &SanityWriteError{Kind: SanityFailMalformed}
	}
	return result.TransactionID, nil
}

// draftIDFor returns the deterministic draft document id for one version.
func draftIDFor(itemID string, versionNumber int) string {
	return fmt.Sprintf("drafts.mktops-%s-v%d", itemID, versionNumber)
}

// publishedIDFor returns the published id for one version.
func publishedIDFor(itemID string, versionNumber int) string {
	return strings.TrimPrefix(draftIDFor(itemID, versionNumber), "drafts.")
}

// documentFromPayload turns a version's fields + body into the payload map
// handed to mutations.
func (p SanityPost) documentPayload() map[string]any {
	docType := p.DocType
	if docType == "" {
		docType = "post"
	}
	doc := map[string]any{
		"_type": docType,
		"title": p.Title,
		"slug":  map[string]any{"_type": "slug", "current": p.Slug},
	}
	if p.Excerpt != "" {
		doc["excerpt"] = p.Excerpt
	}
	doc["body"] = p.BodyBlocks
	for k, v := range p.Extra {
		doc[k] = v
	}
	return doc
}

// CreateDraft writes (createOrReplace) the deterministic draft document for
// one version. Re-running it REPLACES the same draft — no duplicates.
func (s SanityOps) CreateDraft(ctx context.Context, itemID string, versionNumber int, post SanityPost) (draftID string, transactionID string, err error) {
	draftID = draftIDFor(itemID, versionNumber)
	payload := post.documentPayload()
	payload["_id"] = draftID
	txID, err := s.mutate(ctx, mutationEnvelope{
		Mutations: []map[string]any{{"createOrReplace": payload}},
	})
	if err != nil {
		return "", "", err
	}
	return draftID, txID, nil
}

// fetchDraftDoc reads the current draft document (needed to publish its exact
// content).
func (s SanityOps) fetchDraftDoc(ctx context.Context, draftID string) (map[string]any, error) {
	url := fmt.Sprintf("%s/%s/data/doc/%s/%s", s.apiBase(), s.version(), s.Creds.Dataset, draftID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, &SanityWriteError{Kind: SanityFailNetwork}
	}
	req.Header.Set("Authorization", "Bearer "+s.Creds.Token)
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return nil, &SanityWriteError{Kind: SanityFailTimeout}
		}
		return nil, &SanityWriteError{Kind: SanityFailNetwork}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, &SanityWriteError{Kind: SanityFailMalformed}
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, &SanityWriteError{Kind: SanityFailDraftGone}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &SanityWriteError{Kind: SanityFailProvider, HTTP: resp.StatusCode, Text: firstSafe(body)}
	}
	var wrapper struct {
		Documents []map[string]any `json:"documents"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil || len(wrapper.Documents) == 0 {
		return nil, &SanityWriteError{Kind: SanityFailDraftGone}
	}
	return wrapper.Documents[0], nil
}

// Publish runs the transaction: createOrReplace the published id with the
// draft's exact content, then delete the draft. The published document id is
// deterministic per version, so re-publishing the same version is idempotent;
// publishing a DIFFERENT version writes a different id and does not clobber
// the previously published article without the workflow saying so.
// VerifyPublicURL performs the read-only verification of the pilot site's
// published article URL. Nothing is written; a 2xx GET means the page exists.
// urlPrefix defaults to "/posts/" when empty.
func (s SanityOps) VerifyPublicURL(ctx context.Context, siteURL string, urlPrefix, slug string) (string, bool, int) {
	site := strings.TrimRight(strings.TrimSpace(siteURL), "/")
	prefix := urlPrefix
	if prefix == "" {
		prefix = "/posts/"
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	target := site + prefix + slug

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return target, false, 0
	}
	req.Header.Set("User-Agent", "marketing-ops-platform (+verification; read-only)")
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return target, false, 0
	}
	defer func() { _ = resp.Body.Close() }()
	// Drain a bit so the connection closes cleanly.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
	return target, resp.StatusCode >= 200 && resp.StatusCode < 300, resp.StatusCode
}

// firstSafe summarizes a non-2xx reply in one short, key-free line.
func firstSafe(body []byte) string {
	line := string(body)
	if idx := bytes.IndexByte(body, '\n'); idx >= 0 {
		line = line[:idx]
	}
	if len(line) > 160 {
		line = line[:160]
	}
	return strings.TrimSpace(line)
}

func (s SanityOps) Publish(ctx context.Context, itemID string, versionNumber int) (publishedID, transactionID string, err error) {
	draftID := draftIDFor(itemID, versionNumber)
	doc, err := s.fetchDraftDoc(ctx, draftID)
	if err != nil {
		return "", "", err
	}
	// Publish the exact draft content under the published id.
	doc["_id"] = publishedIDFor(itemID, versionNumber)
	txID, err := s.mutate(ctx, mutationEnvelope{
		Mutations: []map[string]any{
			{"createOrReplace": doc},
			{"delete": map[string]any{"id": draftID}},
		},
	})
	if err != nil {
		return "", "", err
	}
	return doc["_id"].(string), txID, nil
}
