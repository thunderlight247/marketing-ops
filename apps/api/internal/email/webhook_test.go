package email

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// testWebhookSecret is Resend's documented example signing secret; its base64
// payload is what the HMAC key becomes.
const testWebhookSecret = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"

func signedHeaders(t *testing.T, secret string, body []byte, at time.Time) http.Header {
	t.Helper()
	key, err := decodeWebhookSecret(secret)
	if err != nil {
		t.Fatalf("decode secret: %v", err)
	}
	stamp := strconv.FormatInt(at.Unix(), 10)
	headers := http.Header{}
	headers.Set("svix-id", "evt_test_1")
	headers.Set("svix-timestamp", stamp)
	headers.Set("svix-signature", signWebhook(key, "evt_test_1", stamp, body))
	return headers
}

func TestParseWebhookAcceptsAVerifiedEvent(t *testing.T) {
	body := []byte(`{"type":"email.delivered","created_at":"2026-10-30T09:00:00Z",
		"data":{"email_id":"msg_42","to":["jane@example.com"]}}`)
	now := time.Now().UTC()
	event, err := ParseWebhook(testWebhookSecret, signedHeaders(t, testWebhookSecret, body, now), body, now, WebhookTolerance)
	if err != nil {
		t.Fatalf("ParseWebhook: %v", err)
	}
	if event.Type != "email.delivered" || event.MessageID != "msg_42" || event.ID != "evt_test_1" {
		t.Errorf("event = %+v", event)
	}
	if !event.OccurredAt.Equal(time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("occurred at = %s", event.OccurredAt)
	}
}

func TestParseWebhookRefusesAnythingUnverified(t *testing.T) {
	now := time.Now().UTC()
	body := []byte(`{"type":"email.bounced","data":{"email_id":"msg_42"}}`)
	good := signedHeaders(t, testWebhookSecret, body, now)

	tampered := []byte(`{"type":"email.delivered","data":{"email_id":"msg_42"}}`)
	stale := signedHeaders(t, testWebhookSecret, body, now.Add(-time.Hour))
	wrongKeyBody := []byte(`{"type":"email.bounced","data":{"email_id":"msg_99"}}`)

	cases := []struct {
		name    string
		secret  string
		headers http.Header
		body    []byte
	}{
		{name: "no secret", secret: "", headers: good, body: body},
		{name: "no signature", secret: testWebhookSecret, headers: http.Header{}, body: body},
		{name: "wrong signature", secret: testWebhookSecret, headers: good, body: tampered},
		{name: "signature for another body", secret: testWebhookSecret, headers: good, body: wrongKeyBody},
		{name: "stale timestamp", secret: testWebhookSecret, headers: stale, body: body},
		{name: "unreadable secret", secret: "whsec_!!!not-base64!!!", headers: good, body: body},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseWebhook(tc.secret, tc.headers, tc.body, now, WebhookTolerance)
			if !errors.Is(err, ErrWebhookUnauthorized) {
				t.Fatalf("err = %v, want ErrWebhookUnauthorized", err)
			}
		})
	}
}

func TestParseWebhookRejectsUnreadablePayloads(t *testing.T) {
	now := time.Now().UTC()
	for _, body := range [][]byte{
		[]byte(`not json`),
		[]byte(`{"data":{"email_id":"msg_42"}}`),
		[]byte(`{"type":"   ","data":{"email_id":"msg_42"}}`),
	} {
		headers := signedHeaders(t, testWebhookSecret, body, now)
		_, err := ParseWebhook(testWebhookSecret, headers, body, now, WebhookTolerance)
		if !errors.Is(err, ErrWebhookMalformed) {
			t.Errorf("body %s: err = %v, want ErrWebhookMalformed", body, err)
		}
	}
}

func TestParseWebhookSanitizesBounceDetail(t *testing.T) {
	now := time.Now().UTC()
	body := []byte(`{"type":"email.bounced","data":{"email_id":"msg_42",
		"bounce":{"message":"mailbox full\r\n\tplease retry\u0000 later"}}}`)
	event, err := ParseWebhook(testWebhookSecret, signedHeaders(t, testWebhookSecret, body, now), body, now, WebhookTolerance)
	if err != nil {
		t.Fatalf("ParseWebhook: %v", err)
	}
	if strings.ContainsAny(event.Detail, "\r\n\t\x00") {
		t.Errorf("detail must be single-line and control-free: %q", event.Detail)
	}
	if !strings.Contains(event.Detail, "mailbox full") {
		t.Errorf("detail lost the provider explanation: %q", event.Detail)
	}
}

func TestDeliveryStateForOnlyKnownEvents(t *testing.T) {
	cases := map[string]string{
		"email.delivered":        "delivered",
		"email.delivery_delayed": "delayed",
		"email.bounced":          "bounced",
		"email.complained":       "complained",
	}
	for eventType, want := range cases {
		got, ok := DeliveryStateFor(eventType)
		if !ok || got != want {
			t.Errorf("%s → %q, %v; want %q", eventType, got, ok, want)
		}
	}
	if _, ok := DeliveryStateFor("email.sent"); ok {
		t.Error("email.sent proves nothing new; it must be ignored")
	}
}

func TestValidateWebhookSecret(t *testing.T) {
	for _, secret := range []string{
		testWebhookSecret,
		strings.TrimPrefix(testWebhookSecret, "whsec_"),
	} {
		if !ValidateWebhookSecret(secret) {
			t.Errorf("%q must be accepted", secret)
		}
	}
	for _, secret := range []string{"", "re_apikey_not_a_secret", "whsec_", "whsec_a b c"} {
		if ValidateWebhookSecret(secret) {
			t.Errorf("%q must be refused", secret)
		}
	}
}

func TestSignatureMatchesAnyOfferedVersion(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("signature-bytes"))
	if !signatureMatches("v2,other v1,"+encoded, "v1,"+encoded) {
		t.Error("a matching version among several must verify")
	}
	if signatureMatches("v1,other", "v1,"+encoded) {
		t.Error("a different signature must not verify")
	}
}
