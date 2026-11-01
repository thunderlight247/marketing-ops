// Package email sends one transactional message through one workspace's own
// email provider account.
//
// The provider is Resend's REST API (a single POST /emails call). The API
// already returns a message id, so a delivery can be traced back to the
// provider without a second integration. Local tests and end-to-end runs point
// APIBase at a mock host; production always talks to the real provider because
// the override is rejected by config.
//
// Failure classification matters more than the happy path: a retry that cannot
// succeed (a bad key, an unverified from-address) must not be retried, and an
// attempt whose outcome is unknown (a timeout after the request left) must
// never be retried automatically, or a recipient could get the email twice.
package email

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

// DefaultAPIBase is the real provider host.
const DefaultAPIBase = "https://api.resend.com"

// Message is one plain-text transactional email.
type Message struct {
	FromName       string
	From           string
	To             string
	Subject        string
	Text           string
	IdempotencyKey string
}

// Sender posts messages with one workspace's API key.
type Sender struct {
	APIBase string
	APIKey  string
	Client  *http.Client
}

// DeliveryError carries a safe, dashboard-ready message and whether another
// attempt could plausibly succeed.
type DeliveryError struct {
	Reason    string // machine-readable class, for logs and audits
	Safe      string // shown to the operator; never contains provider internals
	Retryable bool
}

func (e *DeliveryError) Error() string { return "email delivery failed: " + e.Reason }

// SafeMessage is the only text ever shown to an operator or stored on a row.
func (e *DeliveryError) SafeMessage() string { return e.Safe }

// SafeMessage returns the declared safe text for a send failure.
func SafeMessage(err error) string {
	var de *DeliveryError
	if errors.As(err, &de) {
		return de.Safe
	}
	return "the email provider failed"
}

// Retryable reports whether another attempt could plausibly succeed.
func Retryable(err error) bool {
	var de *DeliveryError
	if errors.As(err, &de) {
		return de.Retryable
	}
	return false
}

// Reason classifies a failure for logs and audit details.
func Reason(err error) string {
	var de *DeliveryError
	if errors.As(err, &de) {
		return de.Reason
	}
	return "send_failed"
}

var (
	// ErrNotConfigured means the workspace has no stored provider key.
	ErrNotConfigured = &DeliveryError{
		Reason: "not_configured",
		Safe:   "email is not connected for this workspace",
	}
	// ErrNoRecipient means there is nobody to send to.
	ErrNoRecipient = &DeliveryError{
		Reason: "no_recipient",
		Safe:   "there is no email address to send to",
	}
	// ErrInvalidAddress means a configured address is not a usable mailbox.
	ErrInvalidAddress = &DeliveryError{
		Reason: "invalid_address",
		Safe:   "a from or to address is not a valid email address",
	}
)

// Send posts one message and returns the provider's message id (empty when the
// provider accepted it without returning one).
func (s Sender) Send(ctx context.Context, m Message) (string, error) {
	if strings.TrimSpace(s.APIKey) == "" {
		return "", ErrNotConfigured
	}
	to := strings.TrimSpace(m.To)
	from := strings.TrimSpace(m.From)
	if to == "" || from == "" {
		return "", ErrNoRecipient
	}
	if !looksLikeAddress(to) || !looksLikeAddress(from) {
		return "", ErrInvalidAddress
	}
	payload := map[string]any{
		"from":    formatFrom(m.FromName, from),
		"to":      []string{to},
		"subject": strings.TrimSpace(m.Subject),
		"text":    m.Text,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", &DeliveryError{Reason: "encode_failed", Safe: "the message could not be encoded"}
	}
	base := strings.TrimRight(strings.TrimSpace(s.APIBase), "/")
	if base == "" {
		base = DefaultAPIBase
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/emails", bytes.NewReader(body))
	if err != nil {
		return "", &DeliveryError{Reason: "request_failed", Safe: "the request could not be built"}
	}
	req.Header.Set("Authorization", "Bearer "+s.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if key := strings.TrimSpace(m.IdempotencyKey); key != "" {
		// The provider deduplicates a repeated id with the same key, so an
		// operator retry of an ambiguous attempt cannot double-send.
		req.Header.Set("Idempotency-Key", key)
	}

	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		// A timeout may mean the provider accepted the message: retrying would
		// risk a duplicate, so the operator is told to check first.
		var timeout interface{ Timeout() bool }
		if errors.As(err, &timeout) && timeout.Timeout() {
			return "", &DeliveryError{
				Reason: "outcome_unknown",
				Safe:   "the email provider did not answer in time; check the provider's log before sending again",
			}
		}
		return "", &DeliveryError{
			Reason:    "unreachable",
			Safe:      "the email provider could not be reached",
			Retryable: true,
		}
	}
	defer func() { _ = res.Body.Close() }()
	// Read a bounded body: never buffer more than a diagnostic needs.
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 8<<10))

	if res.StatusCode >= 200 && res.StatusCode < 300 {
		var parsed struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(raw, &parsed)
		return strings.TrimSpace(parsed.ID), nil
	}

	switch {
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return "", &DeliveryError{
			Reason: "credential_rejected",
			Safe:   "the email provider rejected this workspace's API key",
		}
	case res.StatusCode == http.StatusTooManyRequests:
		return "", &DeliveryError{
			Reason:    "rate_limited",
			Safe:      "the email provider is rate limiting this account",
			Retryable: true,
		}
	case res.StatusCode == http.StatusBadRequest ||
		res.StatusCode == http.StatusUnprocessableEntity ||
		res.StatusCode == http.StatusNotFound:
		return "", &DeliveryError{
			Reason: "rejected",
			Safe:   "the email provider rejected the message; check the from address is verified for this account",
		}
	case res.StatusCode >= 500:
		return "", &DeliveryError{
			Reason:    "provider_unavailable",
			Safe:      "the email provider is temporarily unavailable",
			Retryable: true,
		}
	default:
		return "", &DeliveryError{
			Reason: "unexpected_status",
			Safe:   fmt.Sprintf("the email provider answered with HTTP %d", res.StatusCode),
		}
	}
}

// formatFrom renders `Name <address>` with the display name quoted when it
// contains characters that would break the header.
func formatFrom(name, address string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return address
	}
	clean := strings.NewReplacer("\r", " ", "\n", " ", "\"", "").Replace(name)
	if strings.ContainsAny(clean, "<>,;:") {
		clean = "\"" + clean + "\""
	}
	return clean + " <" + address + ">"
}

// looksLikeAddress is a deliberately loose shape check: the provider is the
// authority on address validity, this only rejects obvious mistakes before a
// network call. A display-name form ("Jane <jane@x.com>") is refused because
// the from-name is a separate field, so the address cannot carry a header.
func looksLikeAddress(raw string) bool {
	if len(raw) < 3 || len(raw) > 320 {
		return false
	}
	addr, err := mail.ParseAddress(raw)
	if err != nil {
		return false
	}
	return strings.EqualFold(addr.Address, raw)
}
