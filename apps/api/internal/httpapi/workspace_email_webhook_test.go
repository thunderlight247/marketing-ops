package httpapi

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"marketingops/api/internal/secretcipher"
	"marketingops/api/internal/store"
)

// The example signing secret Resend documents; its base64 payload is the HMAC
// key for callbacks.
const webhookTestSecret = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"

func webhookKey(t *testing.T, secret string) []byte {
	t.Helper()
	raw := strings.TrimPrefix(strings.TrimSpace(secret), "whsec_")
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		t.Fatalf("decode secret: %v", err)
	}
	return key
}

// sendWebhook posts a body signed exactly the way the provider signs it.
func sendWebhook(t *testing.T, h http.Handler, workspaceID, secret, eventID string, at time.Time, body string) *httptest.ResponseRecorder {
	t.Helper()
	stamp := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, webhookKey(t, secret))
	mac.Write([]byte(eventID + "." + stamp + "."))
	mac.Write([]byte(body))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/email/webhooks/resend/"+workspaceID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("svix-id", eventID)
	req.Header.Set("svix-timestamp", stamp)
	req.Header.Set("svix-signature", "v1,"+base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func connectedEmailFake(t *testing.T) *emailFake {
	t.Helper()
	f := newEmailFake()
	ciphertext, err := secretcipher.Encrypt(emailTestKey(), []byte("re_workspace_key"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	f.settings = &store.EmailSettings{
		WorkspaceID: emailTestWsID, Provider: "resend", FromName: "Rosewood Heating",
		FromEmail: "hello@rosewood.example", NotifyEmail: "team@rosewood.example",
		APIKeyCiphertext: ciphertext, APIKeyVersion: 1, Status: "connected",
	}
	return f
}

func TestEmailWebhookSecretLifecycle(t *testing.T) {
	f := connectedEmailFake(t)
	h := emailRouter(f, "http://127.0.0.1:1")

	rec := doJSON(t, h, http.MethodPut, "/api/v1/workspaces/"+emailTestWsID+"/email-settings/webhook",
		`{"webhookSecret":"not-a-secret"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a wrong-shaped secret must be refused: %d %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, h, http.MethodPut, "/api/v1/workspaces/"+emailTestWsID+"/email-settings/webhook",
		`{"webhookSecret":"`+webhookTestSecret+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("save secret: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"webhookConfigured":true`) {
		t.Errorf("settings must report the webhook: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), emailTestWsID) {
		t.Errorf("the settings must carry this workspace's callback path: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), webhookTestSecret) ||
		strings.Contains(rec.Body.String(), strings.TrimPrefix(webhookTestSecret, "whsec_")) {
		t.Fatalf("the signing secret must never be returned: %s", rec.Body.String())
	}
	if f.settings == nil || len(f.settings.WebhookSecretCiphertext) == 0 {
		t.Fatal("the secret was not stored")
	}
	if string(f.settings.WebhookSecretCiphertext) == webhookTestSecret {
		t.Error("the signing secret must be stored encrypted")
	}
	var audited bool
	for _, e := range f.auditEvents {
		if e.Action == "email_webhook_secret_saved" {
			audited = true
		}
	}
	if !audited {
		t.Errorf("saving the secret must be audited: %+v", f.auditEvents)
	}

	rec = doJSON(t, h, http.MethodDelete, "/api/v1/workspaces/"+emailTestWsID+"/email-settings/webhook", "")
	if rec.Code != http.StatusOK || len(f.settings.WebhookSecretCiphertext) != 0 {
		t.Fatalf("remove secret: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"webhookConfigured":false`) {
		t.Errorf("settings must report the removed webhook: %s", rec.Body.String())
	}

	// Without a connected account there is nothing to attach a secret to.
	f.settings = nil
	rec = doJSON(t, h, http.MethodPut, "/api/v1/workspaces/"+emailTestWsID+"/email-settings/webhook",
		`{"webhookSecret":"`+webhookTestSecret+`"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("secret without a connection must conflict: %d %s", rec.Code, rec.Body.String())
	}
}

func TestEmailWebhookMovesDeliveryState(t *testing.T) {
	f := connectedEmailFake(t)
	f.settings.WebhookSecretCiphertext = nil
	h := emailRouter(f, "http://127.0.0.1:1")
	rec := doJSON(t, h, http.MethodPut, "/api/v1/workspaces/"+emailTestWsID+"/email-settings/webhook",
		`{"webhookSecret":"`+webhookTestSecret+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("save secret: %d %s", rec.Code, rec.Body.String())
	}

	f.deliveries = []store.EmailDelivery{{
		ID: "dddddddd-dddd-dddd-dddd-ddddddddddd5", WorkspaceID: emailTestWsID,
		Kind: "lead_reply", ToEmail: "jane@example.com", Subject: "Re: enquiry",
		Status: "sent", ProviderMessageID: "msg_1", CreatedAt: time.Now(),
	}}

	now := time.Now().UTC()
	body := `{"type":"email.delivered","created_at":"` + now.Format(time.RFC3339) + `","data":{"email_id":"msg_1"}}`
	rec = sendWebhook(t, h, emailTestWsID, webhookTestSecret, "evt_1", now, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("delivered callback: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"deliveryState":"delivered"`) ||
		!strings.Contains(rec.Body.String(), `"updated":true`) {
		t.Errorf("unexpected answer: %s", rec.Body.String())
	}
	if f.deliveries[0].DeliveryState != "delivered" || f.deliveries[0].DeliveryStateAt == nil {
		t.Errorf("delivery was not moved: %+v", f.deliveries[0])
	}

	// The provider retries the same event: acknowledged, not applied twice.
	rec = sendWebhook(t, h, emailTestWsID, webhookTestSecret, "evt_1", now, body)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"duplicate":true`) {
		t.Errorf("retried callback: %d %s", rec.Code, rec.Body.String())
	}

	// A bounce is a newer fact.
	bounce := `{"type":"email.bounced","created_at":"` + now.Add(time.Minute).Format(time.RFC3339) +
		`","data":{"email_id":"msg_1","bounce":{"message":"mailbox full"}}}`
	rec = sendWebhook(t, h, emailTestWsID, webhookTestSecret, "evt_2", now, bounce)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"deliveryState":"bounced"`) {
		t.Errorf("bounced callback: %d %s", rec.Code, rec.Body.String())
	}
	if f.deliveries[0].DeliveryState != "bounced" || f.deliveries[0].DeliveryDetail != "mailbox full" {
		t.Errorf("bounce not recorded: %+v", f.deliveries[0])
	}

	// Unknown messages and uninteresting event types are acknowledged, because
	// a non-2xx would only make the provider retry something unusable.
	unknown := `{"type":"email.delivered","data":{"email_id":"msg_never"}}`
	rec = sendWebhook(t, h, emailTestWsID, webhookTestSecret, "evt_3", now, unknown)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ignored":true`) {
		t.Errorf("unknown message: %d %s", rec.Code, rec.Body.String())
	}
	uninteresting := `{"type":"email.sent","data":{"email_id":"msg_1"}}`
	rec = sendWebhook(t, h, emailTestWsID, webhookTestSecret, "evt_4", now, uninteresting)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ignored":true`) {
		t.Errorf("unsupported event: %d %s", rec.Code, rec.Body.String())
	}
	// Delivered, its retry, the bounce, and the unknown message reached the
	// store; the unsupported event type did not.
	if len(f.providerEvents) != 4 {
		t.Fatalf("unexpected store calls: %+v", f.providerEvents)
	}
	if f.providerEvents[3].EventType == "email.sent" {
		t.Error("unsupported events must not reach the store")
	}
}

func TestEmailWebhookRefusesUnverifiedRequests(t *testing.T) {
	f := connectedEmailFake(t)
	h := emailRouter(f, "http://127.0.0.1:1")
	now := time.Now().UTC()
	body := `{"type":"email.delivered","data":{"email_id":"msg_1"}}`

	// No stored secret: the endpoint does not exist for this workspace.
	rec := sendWebhook(t, h, emailTestWsID, webhookTestSecret, "evt_1", now, body)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("no secret configured: %d %s", rec.Code, rec.Body.String())
	}

	f.settings.WebhookSecretCiphertext = mustSeal(t, webhookTestSecret)

	// A body mutated after signing.
	stamp := strconv.FormatInt(now.Unix(), 10)
	mac := hmac.New(sha256.New, webhookKey(t, webhookTestSecret))
	mac.Write([]byte("evt_2." + stamp + "."))
	mac.Write([]byte(body))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/email/webhooks/resend/"+emailTestWsID,
		bytes.NewReader([]byte(`{"type":"email.bounced","data":{"email_id":"msg_1"}}`)))
	req.Header.Set("svix-id", "evt_2")
	req.Header.Set("svix-timestamp", stamp)
	req.Header.Set("svix-signature", "v1,"+base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("tampered body must be refused: %d %s", rec.Code, rec.Body.String())
	}

	// A signature from an hour ago is a replay.
	rec = sendWebhook(t, h, emailTestWsID, webhookTestSecret, "evt_3", now.Add(-time.Hour), body)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("stale callback must be refused: %d %s", rec.Code, rec.Body.String())
	}

	// Another workspace's id has no secret of its own.
	rec = sendWebhook(t, h, "22222222-2222-2222-2222-222222222222", webhookTestSecret, "evt_4", now, body)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown workspace: %d %s", rec.Code, rec.Body.String())
	}

	// Only the provider this platform supports answers.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/email/webhooks/sendgrid/"+emailTestWsID, strings.NewReader(body))
	req.Header.Set("svix-id", "evt_5")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown provider: %d %s", rec.Code, rec.Body.String())
	}

	if len(f.providerEvents) != 0 {
		t.Errorf("refused callbacks must never reach the store: %+v", f.providerEvents)
	}
}

func TestEmailWebhookWorksWithoutOperatorKey(t *testing.T) {
	f := connectedEmailFake(t)
	f.settings.WebhookSecretCiphertext = mustSeal(t, webhookTestSecret)
	f.deliveries = []store.EmailDelivery{{
		ID: "dddddddd-dddd-dddd-dddd-ddddddddddd6", WorkspaceID: emailTestWsID,
		Kind: "lead_notification", ToEmail: "team@rosewood.example", Subject: "New enquiry",
		Status: "sent", ProviderMessageID: "msg_9", CreatedAt: time.Now(),
	}}
	h := NewRouter(Deps{
		Logger:               testLogger,
		Workspaces:           f,
		CredentialKey:        emailTestKey(),
		CredentialKeyVersion: 1,
		APIKey:               "operator-key",
	})

	// Every operator route still needs the key…
	rec := doJSON(t, h, http.MethodGet, "/api/v1/workspaces/"+emailTestWsID+"/email-settings", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("operator routes must stay protected: %d %s", rec.Code, rec.Body.String())
	}
	// …but the provider has no key and must still be able to report delivery.
	now := time.Now().UTC()
	body := `{"type":"email.delivered","data":{"email_id":"msg_9"}}`
	rec = sendWebhook(t, h, emailTestWsID, webhookTestSecret, "evt_auth", now, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("callback must not need the operator key: %d %s", rec.Code, rec.Body.String())
	}
}

func mustSeal(t *testing.T, plaintext string) []byte {
	t.Helper()
	sealed, err := secretcipher.Encrypt(emailTestKey(), []byte(plaintext))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	return sealed
}
