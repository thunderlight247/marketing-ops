package integration

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func opsFor(base string) SanityOps {
	return SanityOps{
		Creds:  SanityCredentials{ProjectID: "proj1234", Dataset: "production", Token: "sk-test"},
		Base:   base,
		Client: http.DefaultClient,
	}
}

func TestCreateDraftMutationAndDeterminism(t *testing.T) {
	var capturedPath string
	var capturedAuth string
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedBody)
		_, _ = w.Write([]byte(`{"transactionId":"tx-1"}`))
	}))
	defer srv.Close()

	ctx := context.Background()
	draftID, txID, err := opsFor(srv.URL).CreateDraft(ctx, "item-abc", 3, SanityPost{
		Title: "Hello", Slug: "hello-world", Excerpt: "short",
		BodyBlocks: ConvertBodyToBlocks("## Heading\n\nBody text."),
	})
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if capturedPath != "/v2024-10-14/data/mutate/production" {
		t.Fatalf("unexpected path %q", capturedPath)
	}
	if capturedAuth == "" || !contains(capturedAuth, "Bearer sk-test") {
		t.Fatalf("token missing from request: %q", capturedAuth)
	}
	if got := draftID; got != "drafts.mktops-item-abc-v3" {
		t.Fatalf("draft id not deterministic: %q", got)
	}
	if txID != "tx-1" {
		t.Fatalf("transactionId = %q", txID)
	}
	mutations, _ := capturedBody["mutations"].([]any)
	if len(mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %d", len(mutations))
	}
	createOrReplace := mutations[0].(map[string]any)["createOrReplace"].(map[string]any)
	if createOrReplace["_id"] != draftID || createOrReplace["_type"] != "post" {
		t.Fatalf("document id/type wrong: %+v", createOrReplace)
	}
	if createOrReplace["title"] != "Hello" {
		t.Fatalf("title missing: %+v", createOrReplace)
	}
	slug := createOrReplace["slug"].(map[string]any)
	if slug["current"] != "hello-world" {
		t.Fatalf("slug wrong: %+v", slug)
	}
	if slug["_type"] != "slug" {
		t.Fatalf("slug _type wrong: %+v", slug)
	}
	blocks := createOrReplace["body"].([]any)
	if len(blocks) == 0 {
		t.Fatalf("body blocks missing")
	}

	// Re-submitting the same version produces the same draft id (replacement).
	if again, _, err := opsFor(srv.URL).CreateDraft(ctx, "item-abc", 3, SanityPost{
		Title: "Hello", Slug: "hello-world", BodyBlocks: ConvertBodyToBlocks("x"),
	}); err != nil || again != draftID {
		t.Fatalf("draft id must be deterministic, got %q err=%v", again, err)
	}
}

func TestPublishTransaction(t *testing.T) {
	var mutateBody map[string]any
	var fetchedDocID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && contains(r.URL.Path, "/data/doc/production/"):
			fetchedDocID = r.URL.Path
			_, _ = w.Write([]byte(`{"documents":[{"_id":"drafts.mktops-item-x-v1","_type":"post","title":"T","body":[]}]}`))
		case r.Method == http.MethodPost && contains(r.URL.Path, "/data/mutate/production"):
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &mutateBody)
			_, _ = w.Write([]byte(`{"transactionId":"tx-pub"}`))
		default:
			t.Errorf("unexpected call %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	publishedID, txID, err := opsFor(srv.URL).Publish(context.Background(), "item-x", 1)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if publishedID != "mktops-item-x-v1" || txID != "tx-pub" {
		t.Fatalf("published id/tx wrong: %q %q", publishedID, txID)
	}
	if !contains(fetchedDocID, "drafts.mktops-item-x-v1") {
		t.Fatalf("draft doc fetch missing: %q", fetchedDocID)
	}
	mutations := mutateBody["mutations"].([]any)
	if len(mutations) != 2 {
		t.Fatalf("expected createOrReplace + delete, got %d", len(mutations))
	}
	replace := mutations[0].(map[string]any)["createOrReplace"].(map[string]any)
	if replace["_id"] != "mktops-item-x-v1" {
		t.Fatalf("published id wrong in mutation: %+v", replace)
	}
	if replace["title"] != "T" {
		t.Fatalf("published doc must carry the draft's content: %+v", replace)
	}
	del := mutations[1].(map[string]any)["delete"].(map[string]any)
	if del["id"] != "drafts.mktops-item-x-v1" {
		t.Fatalf("draft not deleted in publish transaction: %+v", del)
	}
}

func TestSanityFailureClassification(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			http.Error(w, "nope", http.StatusUnauthorized)
			return
		}
		http.Error(w, "insufficient credits", http.StatusPaymentRequired)
	}))
	defer srv.Close()
	ctx := context.Background()
	ops := opsFor(srv.URL)

	if _, _, err := ops.CreateDraft(ctx, "i", 1, SanityPost{Title: "t", Slug: "s"}); !extractKind(err, SanityFailProvider) {
		t.Fatalf("expected provider failure, got %v", err)
	}
	_, err := ops.fetchDraftDoc(ctx, "drafts.mktops-i-v1")
	if !extractKind(err, SanityFailProvider) {
		t.Fatalf("expected provider failure on 401 doc fetch, got %v", err)
	}

	_, _, err = ops.Publish(ctx, "gone", 1)
	// The fixture answers 401 for ALL fetch calls (not just the draft id), so
	// the publish fails with the provider classification. Draft-gone logic is
	// separately verified below.
	if !extractKind(err, SanityFailProvider) {
		t.Fatalf("expected provider failure from the shared 401 fixture, got %v", err)
	}

	// The failure text must not contain the token.
	if _, _, err := ops.CreateDraft(ctx, "i", 1, SanityPost{Title: "t", Slug: "s"}); err != nil && contains(err.Error(), "sk-test") {
		t.Fatalf("error text leaked the token")
	}

	// A genuinely missing draft (server not found on that id) is draft_gone.
	goneSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer goneSrv.Close()
	_, _, err = opsFor(goneSrv.URL).Publish(ctx, "gone", 1)
	if !extractKind(err, SanityFailDraftGone) {
		t.Fatalf("expected draft-gone on a 404 doc fetch, got %v", err)
	}
}

func extractKind(err error, kind string) bool {
	var we *SanityWriteError
	if !errors.As(err, &we) {
		return false
	}
	return we.Kind == kind
}

func TestConvertBodyToBlocks(t *testing.T) {
	blocks := ConvertBodyToBlocks("# Title\n\nFirst para with **bold** bits.\n\n- one\n- two\n\n1. x\n\nSigned off.")
	if len(blocks) != 6 {
		t.Fatalf("expected 6 blocks (each list item is its own block), got %d: %+v", len(blocks), blocks)
	}
	if blocks[0].Style != "h2" || blocks[0].Children[0].Text != "Title" {
		t.Fatalf("h2 wrong: %+v", blocks[0])
	}
	first := blocks[1].Children
	if len(first) != 3 || first[1].Text != "bold" {
		t.Fatalf("bold spans wrong: %+v", first)
	}
	if first[1].Marks == nil || len(blocks[1].MarkDefs) != 1 || blocks[1].MarkDefs[0].Type != "strong" {
		t.Fatalf("strong markDefs wrong: %+v", blocks[1])
	}
	if blocks[2].ListItem != "bullet" || len(blocks[2].Children) != 1 ||
		blocks[2].Children[0].Text != "one" {
		t.Fatalf("bullet block wrong: %+v", blocks[2])
	}
	if blocks[3].Children[0].Text != "two" || blocks[4].ListItem != "number" {
		t.Fatalf("second bullet / numbered item wrong: %+v %+v", blocks[3], blocks[4])
	}
	if blocks[5].Children[0].Text != "Signed off." {
		t.Fatalf("paragraph wrong: %+v", blocks[5])
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle ||
		(len(needle) > 0 && indexOf(haystack, needle) >= 0))
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
