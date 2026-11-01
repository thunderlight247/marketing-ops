package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"marketingops/api/internal/secretcipher"
	"marketingops/api/internal/store"
)

const emailTestWsID = "11111111-1111-1111-1111-111111111111"

func emailTestKey() []byte { return bytes.Repeat([]byte{0x37}, secretcipher.KeyLength) }

// emailFake adds the outbound-email slice of the store to the shared fake, so
// the real router and handlers run unchanged.
type emailFake struct {
	*fakeWorkspaceStore
	settings      *store.EmailSettings
	deliveries    []store.EmailDelivery
	statusUpdates []string
	enqueueCalls  int
	// providerEvents records every verified callback the handler applied, and
	// seenEvents makes the provider's retries idempotent like the real store.
	providerEvents []store.EmailProviderEvent
	seenEvents     map[string]bool
	// acknowledgedAt holds the delivery-problem acknowledgements per workspace,
	// exactly like the real table.
	acknowledgedAt map[string]time.Time
}

func newEmailFake() *emailFake {
	return &emailFake{fakeWorkspaceStore: &fakeWorkspaceStore{
		byID: map[string]store.Workspace{emailTestWsID: {ID: emailTestWsID, Slug: "demo", Name: "Demo Studio"}},
	}}
}

func (f *emailFake) GetEmailSettings(_ context.Context, workspaceID string) (store.EmailSettings, error) {
	if f.settings == nil || f.settings.WorkspaceID != workspaceID {
		return store.EmailSettings{}, store.ErrNotFound
	}
	return *f.settings, nil
}

func (f *emailFake) SaveEmailSettings(_ context.Context, in store.EmailSettings) (store.EmailSettings, error) {
	in.UpdatedAt = time.Now()
	saved := in
	f.settings = &saved
	return saved, nil
}

func (f *emailFake) DeleteEmailSettings(_ context.Context, workspaceID string) error {
	if f.settings == nil || f.settings.WorkspaceID != workspaceID {
		return store.ErrNotFound
	}
	f.settings = nil
	return nil
}

func (f *emailFake) MarkEmailSettingsStatus(_ context.Context, workspaceID, status string, verifiedAt *time.Time) error {
	if f.settings == nil || f.settings.WorkspaceID != workspaceID {
		return store.ErrNotFound
	}
	f.statusUpdates = append(f.statusUpdates, status)
	f.settings.Status = status
	f.settings.LastVerifiedAt = verifiedAt
	return nil
}

func (f *emailFake) EnqueueEmailDelivery(_ context.Context, in store.EmailDelivery) (store.EmailDelivery, bool, error) {
	f.enqueueCalls++
	for _, existing := range f.deliveries {
		if in.Kind == "lead_reply" && existing.Kind == "lead_reply" &&
			existing.LeadID != nil && in.LeadID != nil && *existing.LeadID == *in.LeadID &&
			// A bounced acceptance is finished, not live (migration 0012), so it
			// must not block the resend.
			(existing.Status == "queued" || existing.Status == "sending" ||
				(existing.Status == "sent" && existing.DeliveryState != "bounced")) {
			return store.EmailDelivery{}, false, nil
		}
	}
	in.ID = "dddddddd-dddd-dddd-dddd-dddddddddddd"
	in.Status = "queued"
	in.CreatedAt = time.Now()
	f.deliveries = append(f.deliveries, in)
	return in, true, nil
}

func (f *emailFake) SetEmailWebhookSecret(_ context.Context, workspaceID string, ciphertext []byte) (store.EmailSettings, error) {
	if f.settings == nil || f.settings.WorkspaceID != workspaceID {
		return store.EmailSettings{}, store.ErrNotFound
	}
	f.settings.WebhookSecretCiphertext = append([]byte(nil), ciphertext...)
	f.settings.UpdatedAt = time.Now()
	return *f.settings, nil
}

func (f *emailFake) ApplyEmailProviderEvent(_ context.Context, in store.EmailProviderEvent) (store.EmailProviderEventResult, error) {
	f.providerEvents = append(f.providerEvents, in)
	index := -1
	for i := range f.deliveries {
		d := f.deliveries[i]
		if d.WorkspaceID == in.WorkspaceID && d.ProviderMessageID != "" && d.ProviderMessageID == in.MessageID {
			index = i
			break
		}
	}
	if index < 0 {
		return store.EmailProviderEventResult{Found: false}, nil
	}
	if f.seenEvents == nil {
		f.seenEvents = map[string]bool{}
	}
	if f.seenEvents[in.EventID] {
		return store.EmailProviderEventResult{DeliveryID: f.deliveries[index].ID, Found: true, Duplicate: true}, nil
	}
	f.seenEvents[in.EventID] = true
	at := in.At
	f.deliveries[index].DeliveryState = in.State
	f.deliveries[index].DeliveryStateAt = &at
	f.deliveries[index].DeliveryDetail = in.Detail
	return store.EmailProviderEventResult{DeliveryID: f.deliveries[index].ID, Found: true, Updated: true}, nil
}

func (f *emailFake) EmailDeliveryAttention(_ context.Context) (map[string]store.EmailDeliveryAttention, error) {
	out := map[string]store.EmailDeliveryAttention{}
	for _, d := range f.deliveries {
		if d.Status != "failed" && d.DeliveryState != "bounced" {
			continue
		}
		// The problem's timestamp, as the real query computes it: when the send
		// failed or when the provider reported the bounce.
		at := d.UpdatedAt
		if d.DeliveryStateAt != nil && d.DeliveryStateAt.After(at) {
			at = *d.DeliveryStateAt
		}
		if ack, ok := f.acknowledgedAt[d.WorkspaceID]; ok && !at.After(ack) {
			continue
		}
		a := out[d.WorkspaceID]
		if d.Status == "failed" {
			a.Failed++
		}
		if d.DeliveryState == "bounced" {
			a.Bounced++
		}
		out[d.WorkspaceID] = a
	}
	return out, nil
}

func (f *emailFake) EmailDeliveryAcknowledgements(_ context.Context) (map[string]time.Time, error) {
	out := map[string]time.Time{}
	for workspaceID, at := range f.acknowledgedAt {
		out[workspaceID] = at
	}
	return out, nil
}

func (f *emailFake) EmailDeliveryAcknowledgement(_ context.Context, workspaceID string) (*time.Time, error) {
	if at, ok := f.acknowledgedAt[workspaceID]; ok {
		return &at, nil
	}
	return nil, nil
}

func (f *emailFake) AcknowledgedEmailDeliveryProblems(_ context.Context, workspaceID string, limit int) ([]store.EmailDelivery, error) {
	ack, ok := f.acknowledgedAt[workspaceID]
	if !ok {
		return nil, nil
	}
	var out []store.EmailDelivery
	for _, d := range f.deliveries {
		if d.WorkspaceID != workspaceID || (d.Status != "failed" && d.DeliveryState != "bounced") {
			continue
		}
		at := d.UpdatedAt
		if d.DeliveryStateAt != nil && d.DeliveryStateAt.After(at) {
			at = *d.DeliveryStateAt
		}
		if at.After(ack) {
			continue
		}
		out = append(out, d)
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *emailFake) AcknowledgeEmailDeliveryAttention(_ context.Context, workspaceID string) (time.Time, error) {
	if _, ok := f.byID[workspaceID]; !ok {
		return time.Time{}, store.ErrNotFound
	}
	if f.acknowledgedAt == nil {
		f.acknowledgedAt = map[string]time.Time{}
	}
	at := time.Now()
	f.acknowledgedAt[workspaceID] = at
	return at, nil
}

func (f *emailFake) ListLeadEmailDeliveries(_ context.Context, workspaceID, leadID string, _ int) ([]store.EmailDelivery, error) {
	var out []store.EmailDelivery
	for _, d := range f.deliveries {
		if d.WorkspaceID == workspaceID && d.LeadID != nil && *d.LeadID == leadID {
			out = append(out, d)
		}
	}
	return out, nil
}

func (f *emailFake) GetEmailDeliveryInWorkspace(_ context.Context, workspaceID, id string) (store.EmailDelivery, error) {
	for _, d := range f.deliveries {
		if d.ID == id && d.WorkspaceID == workspaceID {
			return d, nil
		}
	}
	return store.EmailDelivery{}, store.ErrNotFound
}

func (f *emailFake) ListEmailDeliveries(_ context.Context, workspaceID string, _ int) ([]store.EmailDelivery, error) {
	var out []store.EmailDelivery
	for _, d := range f.deliveries {
		if d.WorkspaceID == workspaceID {
			out = append(out, d)
		}
	}
	return out, nil
}

func emailRouter(f *emailFake, apiBase string) http.Handler {
	return NewRouter(Deps{
		Logger:               testLogger,
		Workspaces:           f,
		CredentialKey:        emailTestKey(),
		CredentialKeyVersion: 1,
		EmailAPIBase:         apiBase,
	})
}

func emailLead(f *fakeWorkspaceStore, id, status, email string, withDraft bool) {
	f.leadsByID = map[string]store.Lead{}
	l := store.Lead{ID: id, WorkspaceID: emailTestWsID, Status: status, SourceKind: "form_api"}
	if email != "" {
		l.ContactEmail = &email
	}
	if withDraft {
		subject, body := "Re: your enquiry", "Thanks for getting in touch — we can visit on Thursday."
		l.ReplyDraftSubject, l.ReplyDraftBody = &subject, &body
	}
	f.leadsByID[id] = l
}

func TestEmailSettingsLifecycle(t *testing.T) {
	f := newEmailFake()
	h := emailRouter(f, "http://127.0.0.1:1")

	rec := doJSON(t, h, http.MethodGet, "/api/v1/workspaces/"+emailTestWsID+"/email-settings", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"configured":false`) {
		t.Fatalf("unconfigured settings: %d %s", rec.Code, rec.Body.String())
	}

	// A first connection needs a key.
	rec = doJSON(t, h, http.MethodPut, "/api/v1/workspaces/"+emailTestWsID+"/email-settings",
		`{"fromEmail":"hello@rosewood.example"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "API key is required") {
		t.Fatalf("missing key must be rejected: %d %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, http.MethodPut, "/api/v1/workspaces/"+emailTestWsID+"/email-settings",
		`{"fromEmail":"not-an-address","apiKey":"re_abcdefgh"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "from email") {
		t.Fatalf("bad address must be rejected: %d %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, h, http.MethodPut, "/api/v1/workspaces/"+emailTestWsID+"/email-settings",
		`{"fromName":"Rosewood Heating","fromEmail":"hello@rosewood.example","notifyEmail":"team@rosewood.example","apiKey":"re_workspace_key"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("connect: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "re_workspace_key") {
		t.Fatalf("the API key must never be returned: %s", rec.Body.String())
	}
	if f.settings == nil || len(f.settings.APIKeyCiphertext) == 0 {
		t.Fatal("settings were not stored")
	}
	if string(f.settings.APIKeyCiphertext) == "re_workspace_key" {
		t.Fatal("the API key must be stored encrypted")
	}
	if f.settings.Status != "unverified" {
		t.Errorf("a new key must start unverified, got %q", f.settings.Status)
	}
	cipherBefore := append([]byte(nil), f.settings.APIKeyCiphertext...)

	// Editing the from-address without a key keeps the stored credential.
	rec = doJSON(t, h, http.MethodPut, "/api/v1/workspaces/"+emailTestWsID+"/email-settings",
		`{"fromEmail":"studio@rosewood.example"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update without key: %d %s", rec.Code, rec.Body.String())
	}
	if !bytes.Equal(f.settings.APIKeyCiphertext, cipherBefore) {
		t.Error("omitting the key must keep the stored ciphertext")
	}
	if f.settings.FromEmail != "studio@rosewood.example" {
		t.Errorf("from email not updated: %q", f.settings.FromEmail)
	}

	rec = doJSON(t, h, http.MethodDelete, "/api/v1/workspaces/"+emailTestWsID+"/email-settings", "")
	if rec.Code != http.StatusNoContent || f.settings != nil {
		t.Fatalf("disconnect: %d %s", rec.Code, rec.Body.String())
	}
}

func TestEmailTestSendMarksVerifiedOnlyOnAcceptance(t *testing.T) {
	var gotSubject, gotAuth string
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		gotAuth = r.Header.Get("Authorization")
		var payload struct{ Subject string }
		_ = json.Unmarshal(raw, &payload)
		gotSubject = payload.Subject
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"id":"msg_test"}`)
	}))
	defer srv.Close()

	f := newEmailFake()
	ciphertext, err := secretcipher.Encrypt(emailTestKey(), []byte("re_workspace_key"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	f.settings = &store.EmailSettings{
		WorkspaceID: emailTestWsID, Provider: "resend", FromName: "Rosewood Heating",
		FromEmail: "hello@rosewood.example", NotifyEmail: "team@rosewood.example",
		APIKeyCiphertext: ciphertext, APIKeyVersion: 1, Status: "unverified",
	}
	h := emailRouter(f, srv.URL)

	rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+emailTestWsID+"/email-settings/test", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("test send: %d %s", rec.Code, rec.Body.String())
	}
	if f.settings.Status != "connected" || f.settings.LastVerifiedAt == nil {
		t.Errorf("acceptance must mark the connection connected: %+v", f.settings)
	}
	if gotAuth != "Bearer re_workspace_key" {
		t.Errorf("the workspace's own key must be used, got %q", gotAuth)
	}
	if !strings.Contains(gotBody, "team@rosewood.example") {
		t.Errorf("the test message must go to the notification address: %s", gotBody)
	}
	if !strings.Contains(gotSubject, "connected") {
		t.Errorf("subject = %q", gotSubject)
	}
}

func TestEmailTestSendFailureIsSafeAndFlagsTheConnection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"message":"key re_secret_in_provider_text is revoked"}`)
	}))
	defer srv.Close()

	f := newEmailFake()
	ciphertext, _ := secretcipher.Encrypt(emailTestKey(), []byte("re_workspace_key"))
	f.settings = &store.EmailSettings{
		WorkspaceID: emailTestWsID, Provider: "resend", FromName: "Rosewood Heating",
		FromEmail: "hello@rosewood.example", APIKeyCiphertext: ciphertext,
		APIKeyVersion: 1, Status: "unverified",
	}
	h := emailRouter(f, srv.URL)

	rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+emailTestWsID+"/email-settings/test", "")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("a rejected key must be a 502, got %d %s", rec.Code, rec.Body.String())
	}
	if f.settings.Status != "error" {
		t.Errorf("status = %q, want error", f.settings.Status)
	}
	if strings.Contains(rec.Body.String(), "re_secret_in_provider_text") {
		t.Errorf("provider text leaked: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "rejected this workspace's API key") {
		t.Errorf("expected the safe explanation, got %s", rec.Body.String())
	}
}

func TestSendLeadReplyQueuesExactlyOnce(t *testing.T) {
	const leadID = "66666666-6666-6666-6666-666666666666"
	f := newEmailFake()
	emailLead(f.fakeWorkspaceStore, leadID, "new", "jane@example.com", true)
	h := emailRouter(f, "http://127.0.0.1:1")

	// Not connected yet: the operator is told to connect first.
	rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+emailTestWsID+"/leads/"+leadID+"/reply-send", "{}")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "connect an email account") {
		t.Fatalf("unconnected send must be refused: %d %s", rec.Code, rec.Body.String())
	}

	f.settings = &store.EmailSettings{
		WorkspaceID: emailTestWsID, Provider: "resend", FromName: "Rosewood Heating",
		FromEmail: "hello@rosewood.example", APIKeyCiphertext: []byte("x"), APIKeyVersion: 1, Status: "connected",
	}

	rec = doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+emailTestWsID+"/leads/"+leadID+"/reply-send", "{}")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("send: %d %s", rec.Code, rec.Body.String())
	}
	if len(f.deliveries) != 1 {
		t.Fatalf("expected one queued delivery, got %d", len(f.deliveries))
	}
	queued := f.deliveries[0]
	if queued.Kind != "lead_reply" || queued.ToEmail != "jane@example.com" ||
		queued.Subject != "Re: your enquiry" || !strings.Contains(queued.Body, "Thursday") {
		t.Errorf("queued delivery does not match the reviewed draft: %+v", queued)
	}
	var audited bool
	for _, e := range f.auditEvents {
		if e.Action == "email_reply_queued" {
			audited = true
		}
	}
	if !audited {
		t.Errorf("queued reply must be audited: %+v", f.auditEvents)
	}

	// A second click cannot email the person twice.
	rec = doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+emailTestWsID+"/leads/"+leadID+"/reply-send", "{}")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "already queued") {
		t.Fatalf("duplicate send must be refused: %d %s", rec.Code, rec.Body.String())
	}
}

func TestSendLeadReplyRefusesUnsendableLeads(t *testing.T) {
	f := newEmailFake()
	f.settings = &store.EmailSettings{
		WorkspaceID: emailTestWsID, Provider: "resend",
		FromEmail: "hello@rosewood.example", APIKeyCiphertext: []byte("x"), APIKeyVersion: 1, Status: "connected",
	}
	h := emailRouter(f, "http://127.0.0.1:1")

	// No draft and no override: there is nothing to send.
	emailLead(f.fakeWorkspaceStore, "66666666-6666-6666-6666-666666666601", "new", "jane@example.com", false)
	rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+emailTestWsID+"/leads/66666666-6666-6666-6666-666666666601/reply-send", "{}")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "no sendable reply") {
		t.Fatalf("missing draft: %d %s", rec.Code, rec.Body.String())
	}

	// A lead without an address cannot be emailed.
	emailLead(f.fakeWorkspaceStore, "66666666-6666-6666-6666-666666666602", "new", "", true)
	rec = doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+emailTestWsID+"/leads/66666666-6666-6666-6666-666666666602/reply-send", "{}")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "no email address") {
		t.Fatalf("missing address: %d %s", rec.Code, rec.Body.String())
	}

	// A closed lead gets no first-reply send.
	emailLead(f.fakeWorkspaceStore, "66666666-6666-6666-6666-666666666603", "lost", "jane@example.com", true)
	rec = doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+emailTestWsID+"/leads/66666666-6666-6666-6666-666666666603/reply-send", "{}")
	if rec.Code != http.StatusConflict {
		t.Fatalf("closed lead: %d %s", rec.Code, rec.Body.String())
	}

	// An unknown lead is a 404, and no delivery is queued by any refusal.
	rec = doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+emailTestWsID+"/leads/66666666-6666-6666-6666-666666666699/reply-send", "{}")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown lead: %d %s", rec.Code, rec.Body.String())
	}
	if len(f.deliveries) != 0 {
		t.Fatalf("refusals must not queue deliveries: %+v", f.deliveries)
	}
}

func TestSendLeadReplyUsesEditedTextAndRecordsIt(t *testing.T) {
	const leadID = "66666666-6666-6666-6666-666666666610"
	f := newEmailFake()
	emailLead(f.fakeWorkspaceStore, leadID, "new", "jane@example.com", true)
	f.settings = &store.EmailSettings{
		WorkspaceID: emailTestWsID, Provider: "resend",
		FromEmail: "hello@rosewood.example", APIKeyCiphertext: []byte("x"), APIKeyVersion: 1, Status: "connected",
	}
	h := emailRouter(f, "http://127.0.0.1:1")

	rec := doJSON(t, h, http.MethodPost, "/api/v1/workspaces/"+emailTestWsID+"/leads/"+leadID+"/reply-send",
		`{"subject":"Re: furnace check","body":"Hi Jane, Thursday at 9am works."}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("send: %d %s", rec.Code, rec.Body.String())
	}
	if len(f.savedLeadReplies) != 1 || f.savedLeadReplies[0].body != "Hi Jane, Thursday at 9am works." {
		t.Fatalf("the sent text must be recorded on the lead: %+v", f.savedLeadReplies)
	}
	if f.deliveries[0].Body != "Hi Jane, Thursday at 9am works." {
		t.Errorf("delivery body = %q", f.deliveries[0].Body)
	}
}

func TestEmailDeliveryListing(t *testing.T) {
	f := newEmailFake()
	h := emailRouter(f, "http://127.0.0.1:1")
	leadID := "66666666-6666-6666-6666-666666666620"
	f.deliveries = []store.EmailDelivery{{
		ID: "dddddddd-dddd-dddd-dddd-ddddddddddd1", WorkspaceID: emailTestWsID,
		Kind: "lead_reply", LeadID: &leadID, ToEmail: "jane@example.com",
		Subject: "Re: enquiry", Status: "sent", ProviderMessageID: "msg_1",
		CreatedAt: time.Now(),
	}}
	rec := doJSON(t, h, http.MethodGet, "/api/v1/workspaces/"+emailTestWsID+"/email-deliveries?limit=5", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"status":"sent"`, `"providerMessageId":"msg_1"`, `"kind":"lead_reply"`} {
		if !strings.Contains(body, want) {
			t.Errorf("delivery listing lost %s: %s", want, body)
		}
	}
	if strings.Contains(body, "APIKey") || strings.Contains(body, "re_") {
		t.Errorf("the listing must not expose credentials: %s", body)
	}
}
