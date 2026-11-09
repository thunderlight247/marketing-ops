package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"marketingops/api/internal/store"
)

// newsletterFake keeps subscribers per workspace in memory. Addresses are
// stored as given; the handler is responsible for lowercasing them.
type newsletterFake struct {
	*fakeWorkspaceStore
	subs       map[string]store.NewsletterSubscriber // key: ws|email
	suppressed map[string]bool                       // key: ws|email
}

func (f *newsletterFake) AddNewsletterSubscriber(_ context.Context, ws, email, name, source string) (bool, error) {
	key := ws + "|" + email
	if f.suppressed[key] {
		return false, nil
	}
	if _, ok := f.subs[key]; ok {
		return false, nil
	}
	f.subs[key] = store.NewsletterSubscriber{WorkspaceID: ws, Email: email, Name: name, Source: source, CreatedAt: time.Now()}
	return true, nil
}

func (f *newsletterFake) ListNewsletterSubscribers(_ context.Context, ws string) ([]store.NewsletterSubscriber, error) {
	out := []store.NewsletterSubscriber{}
	for _, s := range f.subs {
		if s.WorkspaceID == ws {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *newsletterFake) DeleteNewsletterSubscriber(_ context.Context, ws, email string) error {
	key := ws + "|" + email
	if _, ok := f.subs[key]; !ok {
		return store.ErrNotFound
	}
	delete(f.subs, key)
	return nil
}

func TestNewsletterSubscriberLifecycle(t *testing.T) {
	const ws = "11111111-1111-1111-1111-111111111111"
	f := &newsletterFake{
		fakeWorkspaceStore: &fakeWorkspaceStore{byID: map[string]store.Workspace{ws: {ID: ws}}},
		subs:               map[string]store.NewsletterSubscriber{},
		suppressed:         map[string]bool{ws + "|opted-out@example.test": true},
	}
	h := NewRouter(Deps{Logger: testLogger, Workspaces: f, APIKey: "operator-secret"})
	base := "/api/v1/workspaces/" + ws + "/newsletter/subscribers"
	send := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer operator-secret")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	if rec := send(http.MethodPost, base, `{"email":"Reader@Example.test","name":" Ada "}`); rec.Code != http.StatusCreated {
		t.Fatalf("add: got %d %s", rec.Code, rec.Body.String())
	}
	if rec := send(http.MethodPost, base, `{"email":"reader@example.test"}`); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate (case-insensitive): got %d, want 409", rec.Code)
	}
	if rec := send(http.MethodPost, base, `{"email":"opted-out@example.test"}`); rec.Code != http.StatusConflict {
		t.Fatalf("suppressed address: got %d, want 409", rec.Code)
	}
	if rec := send(http.MethodPost, base, `{"email":"not-an-address"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid email: got %d, want 400", rec.Code)
	}

	rec := send(http.MethodGet, base, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: got %d", rec.Code)
	}
	var listed struct {
		Subscribers []struct {
			Email string `json:"email"`
			Name  string `json:"name"`
		} `json:"subscribers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("list body: %v", err)
	}
	if len(listed.Subscribers) != 1 || listed.Subscribers[0].Email != "reader@example.test" || listed.Subscribers[0].Name != "Ada" {
		t.Fatalf("list: got %+v", listed.Subscribers)
	}

	if rec := send(http.MethodDelete, base+"?email=Reader@Example.test", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("remove: got %d", rec.Code)
	}
	if rec := send(http.MethodDelete, base+"?email=reader@example.test", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("remove again: got %d, want 404", rec.Code)
	}
}
