// Provider delivery webhooks.
//
// Until now a delivery could only be "the provider accepted the message".
// These callbacks add what happened afterwards: delivered, bounced, complained
// about, or delayed. They arrive from the provider's servers, not from the
// operator, so each one is authenticated by the provider's signature scheme
// (Svix) using the workspace's own signing secret.
//
// The scheme: the signed content is "<svix-id>.<svix-timestamp>.<raw body>",
// HMAC-SHA256 under the base64-decoded secret, sent as "v1,<base64 sig>" (a
// header may carry several space-separated versions). A timestamp outside the
// tolerance window is refused, so a captured request cannot be replayed later.
package email

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// WebhookTolerance bounds how old a signed callback may be.
const WebhookTolerance = 5 * time.Minute

var (
	// ErrWebhookUnauthorized means the signature is missing or wrong.
	ErrWebhookUnauthorized = errors.New("email webhook signature is not valid")
	// ErrWebhookMalformed means the request is signed but unreadable.
	ErrWebhookMalformed = errors.New("email webhook payload is not readable")
)

// WebhookEvent is one verified provider callback.
type WebhookEvent struct {
	// ID is the provider's own event id; retries repeat it, which is what makes
	// handling idempotent.
	ID         string
	Type       string
	MessageID  string
	OccurredAt time.Time
	// Detail is a short, sanitized explanation (a bounce reason, for example).
	Detail string
}

// ParseWebhook verifies the signature over the raw body and decodes the event.
// now and tolerance are parameters so replay-window behaviour is testable.
func ParseWebhook(secret string, header http.Header, body []byte, now time.Time, tolerance time.Duration) (WebhookEvent, error) {
	if strings.TrimSpace(secret) == "" {
		return WebhookEvent{}, ErrWebhookUnauthorized
	}
	if tolerance <= 0 {
		tolerance = WebhookTolerance
	}
	id := strings.TrimSpace(header.Get("svix-id"))
	stamp := strings.TrimSpace(header.Get("svix-timestamp"))
	signatures := strings.TrimSpace(header.Get("svix-signature"))
	if id == "" || stamp == "" || signatures == "" {
		return WebhookEvent{}, ErrWebhookUnauthorized
	}
	seconds, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil {
		return WebhookEvent{}, ErrWebhookUnauthorized
	}
	sentAt := time.Unix(seconds, 0)
	if now.Sub(sentAt) > tolerance || sentAt.Sub(now) > tolerance {
		return WebhookEvent{}, ErrWebhookUnauthorized
	}
	key, err := decodeWebhookSecret(secret)
	if err != nil {
		return WebhookEvent{}, ErrWebhookUnauthorized
	}
	expected := signWebhook(key, id, stamp, body)
	if !signatureMatches(signatures, expected) {
		return WebhookEvent{}, ErrWebhookUnauthorized
	}

	var payload struct {
		Type      string `json:"type"`
		CreatedAt string `json:"created_at"`
		Data      struct {
			// Providers differ slightly here; both are accepted.
			EmailID string   `json:"email_id"`
			ID      string   `json:"id"`
			Subject string   `json:"subject"`
			To      []string `json:"to"`
			Bounce  struct {
				Message string `json:"message"`
			} `json:"bounce"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return WebhookEvent{}, ErrWebhookMalformed
	}
	eventType := strings.TrimSpace(strings.ToLower(payload.Type))
	if eventType == "" {
		return WebhookEvent{}, ErrWebhookMalformed
	}
	messageID := payload.Data.EmailID
	if messageID == "" {
		messageID = payload.Data.ID
	}
	occurredAt := now
	if parsed, err := time.Parse(time.RFC3339, payload.CreatedAt); err == nil {
		occurredAt = parsed
	}
	return WebhookEvent{
		ID:         id,
		Type:       eventType,
		MessageID:  strings.TrimSpace(messageID),
		OccurredAt: occurredAt.UTC(),
		Detail:     sanitizeDetail(payload.Data.Bounce.Message),
	}, nil
}

// ValidateWebhookSecret reports whether a pasted secret can be used as a
// signing key, so a typo fails at save time instead of silently dropping every
// callback later.
func ValidateWebhookSecret(secret string) bool {
	_, err := decodeWebhookSecret(secret)
	return err == nil
}

// DeliveryStateFor maps a provider event type onto the delivery state it
// proves. Unknown events are acknowledged and ignored, never guessed at.
func DeliveryStateFor(eventType string) (string, bool) {
	switch eventType {
	case "email.delivered":
		return "delivered", true
	case "email.delivery_delayed":
		return "delayed", true
	case "email.bounced":
		return "bounced", true
	case "email.complained":
		return "complained", true
	default:
		return "", false
	}
}

// decodeWebhookSecret accepts the provider's "whsec_<base64>" form and a bare
// base64 secret, with or without padding.
func decodeWebhookSecret(secret string) ([]byte, error) {
	secret = strings.TrimSpace(secret)
	secret = strings.TrimPrefix(secret, "whsec_")
	if secret == "" {
		return nil, ErrWebhookUnauthorized
	}
	if decoded, err := base64.StdEncoding.DecodeString(secret); err == nil {
		return decoded, nil
	}
	decoded, err := base64.RawStdEncoding.DecodeString(secret)
	if err != nil {
		return nil, ErrWebhookUnauthorized
	}
	return decoded, nil
}

// signWebhook renders the header value the provider would send.
func signWebhook(key []byte, id, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + timestamp + "."))
	mac.Write(body)
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// signatureMatches compares every offered version in constant time. A header
// with several space-separated signatures is valid when any one matches.
func signatureMatches(offered, expected string) bool {
	expected = strings.TrimPrefix(expected, "v1,")
	for _, candidate := range strings.Fields(offered) {
		candidate = strings.TrimPrefix(candidate, "v1,")
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(expected)) == 1 {
			return true
		}
	}
	return false
}

// sanitizeDetail keeps a provider detail short and single-line: it is written
// to an operator-visible column.
func sanitizeDetail(raw string) string {
	clean := strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, raw)
	clean = strings.Join(strings.Fields(clean), " ")
	runes := []rune(clean)
	if len(runes) > 300 {
		return string(runes[:300])
	}
	return clean
}
