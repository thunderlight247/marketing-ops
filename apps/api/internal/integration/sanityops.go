// Sanity write operations (Milestone 6): create/update drafts, publish, and
// verify the public URL — all server-side; the token never leaves the server
// anderror text is classified, never echoing credentials.
//
// Design notes:
//   - Sanity's draft ID is "drafts." + its published ID. Marketing Ops uses a
//     stable published ID per item, so every version replaces that item's one
//     draft rather than violating Sanity's draft/published document pairing.
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
	return e.SafeMessage()
}

// SafeMessage returns a classified message that never includes response bodies or credentials.
func (e *SanityWriteError) SafeMessage() string {
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

	// Non-text blocks: exactly one of these is set, and MarshalJSON emits
	// the Portable Text object for that block instead of a text span block.
	Image *PTImageRef `json:"-"`
	Embed *PTEmbed    `json:"-"`
}

// MarshalJSON renders image/embed blocks as their Portable Text objects and
// text blocks with the: standard struct fields.
func (b PTBlock) MarshalJSON() ([]byte, error) {
	switch {
	case b.Image != nil:
		doc := map[string]any{
			"_type": "image",
			"asset": map[string]any{"_type": "reference", "_ref": b.Image.AssetRef},
		}
		if b.Image.Alt != "" {
			doc["alt"] = b.Image.Alt
		}
		return json.Marshal(doc)
	case b.Embed != nil:
		return json.Marshal(map[string]any{"_type": b.Embed.Type, "url": b.Embed.URL})
	}
	type plainBlock PTBlock // avoid MarshalJSON recursion
	return json.Marshal(plainBlock(b))
}

// MarkDef is, e.g., a "strong" mark.
type MarkDef struct {
	Key  string `json:"_key"`
	Type string `json:"_type"`
}

// PTImageRef is an image block referencing an already-uploaded Sanity asset.
type PTImageRef struct {
	AssetRef string
	Alt      string
}

// PTEmbed is a non-text embed block (e.g. a YouTube object in the schema).
type PTEmbed struct {
	Type string
	URL  string
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
				// Each bold span gets one stable mark key and exactly one
				// corresponding markDef; repeated bold runs may reuse the def.
				if len(block.MarkDefs) == 0 {
					block.MarkDefs = append(block.MarkDefs, MarkDef{Key: mark, Type: "strong"})
				}
			}
			block.Children = append(block.Children, child)
		}
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
		return block
	}

	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			// blank line: paragraph separator, nothing to emit
		case strings.HasPrefix(trimmed, mediaImageMarkerPrefix):
			if block, ok := parseMediaImageMarker(trimmed); ok {
				blocks = append(blocks, *block)
			}
		case strings.HasPrefix(trimmed, mediaYoutubeMarkerPrefix):
			if block, ok := parseMediaYoutubeMarker(trimmed); ok {
				blocks = append(blocks, *block)
			}
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

// ---- Media inline markers ----------

// Draft bodies can embed already-uploaded media with single-line markers the
// editor inserts after an upload succeeds:
//
//	[media-image ref=<assetDocID> src=<cdnURL> alt="<text>"]
//	[media-youtube src=<url>]
//
// The ref is the asset document id used for the Portable Text asset reference;
// src is only for browser previews. Anything unparseable stays literal text.
const (
	mediaImageMarkerPrefix   = "[media-image "
	mediaYoutubeMarkerPrefix = "[media-youtube "
)

// splitMarkerTokens splits "key=value" chips on unquoted spaces only, so
// quoted values may contain spaces. Quotes are preserved for unquoting.
func splitMarkerTokens(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"':
			inQuote = !inQuote
			cur.WriteByte(c)
		case c == ' ' && !inQuote:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func unquoteMarkerValue(v string) string {
	if len(v) >= 2 && strings.HasPrefix(v, "\"") && strings.HasSuffix(v, "\"") {
		return strings.ReplaceAll(v[1:len(v)-1], "\\\"", "\"")
	}
	return v
}

// markerFields reads the inner key=value chips of a marker.
func markerFields(inner string) map[string]string {
	fields := map[string]string{}
	for _, token := range splitMarkerTokens(inner) {
		if eq := strings.IndexByte(token, '='); eq > 0 {
			fields[token[:eq]] = unquoteMarkerValue(token[eq+1:])
		}
	}
	return fields
}

func parseMediaImageMarker(trimmed string) (*PTBlock, bool) {
	if !strings.HasSuffix(trimmed, "]") {
		return nil, false
	}
	fields := markerFields(trimmed[len(mediaImageMarkerPrefix) : len(trimmed)-1])
	ref := fields["ref"]
	if ref == "" {
		return nil, false
	}
	return &PTBlock{Image: &PTImageRef{AssetRef: ref, Alt: fields["alt"]}}, true
}

func parseMediaYoutubeMarker(trimmed string) (*PTBlock, bool) {
	if !strings.HasSuffix(trimmed, "]") {
		return nil, false
	}
	fields := markerFields(trimmed[len(mediaYoutubeMarkerPrefix) : len(trimmed)-1])
	url := fields["src"]
	if url == "" {
		return nil, false
	}
	return &PTBlock{Embed: &PTEmbed{Type: "youtube", URL: url}}, true
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

// draftIDFor returns Sanity's canonical draft ID for the item's published ID.
// Sanity pairs drafts.foo with the published document foo; the version number
// is intentionally excluded because Sanity permits only one draft per document.
func draftIDFor(itemID string, _ int) string {
	return "drafts." + publishedIDFor(itemID, 0)
}

// publishedIDFor returns the stable published document id for one content item.
// Every approved version updates this same Sanity article; its draft is always
// the paired `drafts.` + published ID, as required by Sanity.
func publishedIDFor(itemID string, _ int) string {
	return "mktops-" + itemID
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

// CreateDraft writes (createOrReplace) Sanity's canonical draft for one item.
// Re-running replaces the same draft. Preserve _createdAt across replacement.
func (s SanityOps) CreateDraft(ctx context.Context, itemID string, versionNumber int, post SanityPost) (draftID string, transactionID string, err error) {
	draftID = draftIDFor(itemID, versionNumber)
	payload := post.documentPayload()
	payload["_id"] = draftID
	if existing, fetchErr := s.fetchDraftDoc(ctx, draftID); fetchErr == nil {
		if createdAt, ok := existing["_createdAt"].(string); ok && createdAt != "" {
			payload["_createdAt"] = createdAt
		}
	} else {
		var writeErr *SanityWriteError
		if !errors.As(fetchErr, &writeErr) || writeErr.Kind != SanityFailDraftGone {
			return "", "", fetchErr
		}
		// After a prior publish, there is no draft, but Sanity requires the
		// published document and its next draft to share _createdAt. Preserve
		// that value when starting another approved version.
		if published, publishedErr := s.fetchDraftDoc(ctx, publishedIDFor(itemID, versionNumber)); publishedErr == nil {
			if createdAt, ok := published["_createdAt"].(string); ok && createdAt != "" {
				payload["_createdAt"] = createdAt
			}
		} else {
			var publishedWriteErr *SanityWriteError
			if !errors.As(publishedErr, &publishedWriteErr) || publishedWriteErr.Kind != SanityFailDraftGone {
				return "", "", publishedErr
			}
		}
	}
	txID, err := s.mutate(ctx, mutationEnvelope{
		Mutations: []map[string]any{{"createOrReplace": payload}},
	})
	if err != nil {
		return "", "", err
	}
	return draftID, txID, nil
}

// fetchDraftDoc reads a draft or published document by ID without relying on
// the query index's eventual consistency.
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

// Publish runs one transaction: createOrReplace the stable published ID with
// the exact approved backend version, then delete the canonical draft. It does
// not trust a mutable Studio draft as the publication source. Re-publishing is
// idempotent; later approved versions update the same published article.
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

func (s SanityOps) Publish(ctx context.Context, itemID string, versionNumber int, post SanityPost) (publishedID, transactionID string, err error) {
	draftID := draftIDFor(itemID, versionNumber)
	publishedID = publishedIDFor(itemID, versionNumber)
	doc := post.documentPayload()
	doc["_id"] = publishedID

	// Sanity pairs the draft and published documents with a shared creation
	// timestamp. Preserve it from the draft, or from the published document on
	// retries after a successful write whose response was lost.
	prior, priorErr := s.fetchDraftDoc(ctx, draftID)
	if priorErr != nil {
		var writeErr *SanityWriteError
		if !errors.As(priorErr, &writeErr) || writeErr.Kind != SanityFailDraftGone {
			return "", "", priorErr
		}
		prior, priorErr = s.fetchDraftDoc(ctx, publishedID)
		if priorErr != nil {
			var publishedErr *SanityWriteError
			if !errors.As(priorErr, &publishedErr) || publishedErr.Kind != SanityFailDraftGone {
				return "", "", priorErr
			}
		}
	}
	if prior != nil {
		if createdAt, ok := prior["_createdAt"].(string); ok && createdAt != "" {
			doc["_createdAt"] = createdAt
		}
	}

	txID, err := s.mutate(ctx, mutationEnvelope{
		Mutations: []map[string]any{
			{"createOrReplace": doc},
			{"delete": map[string]any{"id": draftID}},
		},
	})
	if err != nil {
		return "", "", err
	}
	publishedID, ok := doc["_id"].(string)
	if !ok || publishedID == "" {
		return "", "", &SanityWriteError{Kind: SanityFailMalformed}
	}
	return publishedID, txID, nil
}
