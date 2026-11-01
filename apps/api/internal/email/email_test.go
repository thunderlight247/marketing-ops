package email

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSenderPostsOneMessageAndReturnsProviderID(t *testing.T) {
	var (
		gotPath, gotAuth, gotIdempotency string
		gotBody                          map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		gotIdempotency = r.Header.Get("Idempotency-Key")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"id":"msg_123"}`)
	}))
	defer srv.Close()

	s := Sender{APIBase: srv.URL + "/", APIKey: "re_key", Client: srv.Client()}
	id, err := s.Send(context.Background(), Message{
		FromName:       "Rosewood Heating",
		From:           "hello@rosewood.example",
		To:             "jane@example.com",
		Subject:        "Re: furnace check",
		Text:           "Hi Jane,\n\nWe can visit on Thursday.",
		IdempotencyKey: "delivery-1",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if id != "msg_123" {
		t.Errorf("message id = %q, want msg_123", id)
	}
	if gotPath != "/emails" || gotAuth != "Bearer re_key" || gotIdempotency != "delivery-1" {
		t.Errorf("request shape wrong: path=%q auth=%q idempotency=%q", gotPath, gotAuth, gotIdempotency)
	}
	if gotBody["from"] != "Rosewood Heating <hello@rosewood.example>" {
		t.Errorf("from = %v", gotBody["from"])
	}
	if gotBody["subject"] != "Re: furnace check" || gotBody["text"] != "Hi Jane,\n\nWe can visit on Thursday." {
		t.Errorf("payload lost content: %+v", gotBody)
	}
	to, ok := gotBody["to"].([]any)
	if !ok || len(to) != 1 || to[0] != "jane@example.com" {
		t.Errorf("to = %v", gotBody["to"])
	}
}

func TestSenderClassifiesFailuresWithoutLeakingProviderText(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		retryable bool
		reason    string
	}{
		{"bad key", http.StatusUnauthorized, false, "credential_rejected"},
		{"forbidden", http.StatusForbidden, false, "credential_rejected"},
		{"rejected message", http.StatusUnprocessableEntity, false, "rejected"},
		{"rate limited", http.StatusTooManyRequests, true, "rate_limited"},
		{"provider down", http.StatusServiceUnavailable, true, "provider_unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				// A provider body can carry account details; none of it may
				// reach the safe message.
				_, _ = io.WriteString(w, `{"message":"internal detail abc@example.com"}`)
			}))
			defer srv.Close()

			s := Sender{APIBase: srv.URL, APIKey: "re_key", Client: srv.Client()}
			_, err := s.Send(context.Background(), Message{
				From: "hello@rosewood.example", To: "jane@example.com",
				Subject: "Re: hi", Text: "body",
			})
			if err == nil {
				t.Fatal("expected a failure")
			}
			if Retryable(err) != tc.retryable {
				t.Errorf("Retryable = %v, want %v", Retryable(err), tc.retryable)
			}
			if Reason(err) != tc.reason {
				t.Errorf("Reason = %q, want %q", Reason(err), tc.reason)
			}
			if safe := SafeMessage(err); strings.Contains(safe, "internal detail") {
				t.Errorf("safe message leaked provider text: %q", safe)
			}
		})
	}
}

func TestSenderTransportFailures(t *testing.T) {
	t.Run("timeout is never retried automatically", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Answer much later than the client will wait.
			time.Sleep(300 * time.Millisecond)
		}))
		defer srv.Close()
		client := srv.Client()
		client.Timeout = 100 * time.Millisecond
		s := Sender{APIBase: srv.URL, APIKey: "re_key", Client: client}
		_, err := s.Send(context.Background(), Message{
			From: "a@example.com", To: "b@example.com", Subject: "s", Text: "t",
		})
		if err == nil {
			t.Fatal("expected a timeout")
		}
		if Retryable(err) {
			t.Error("an unanswered send must not be retried: it may already have been delivered")
		}
		if Reason(err) != "outcome_unknown" {
			t.Errorf("Reason = %q", Reason(err))
		}
	})

	t.Run("unreachable host is retryable", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		url := srv.URL
		srv.Close() // now nothing listens
		s := Sender{APIBase: url, APIKey: "re_key", Client: &http.Client{}}
		_, err := s.Send(context.Background(), Message{
			From: "a@example.com", To: "b@example.com", Subject: "s", Text: "t",
		})
		if err == nil || !Retryable(err) {
			t.Fatalf("expected a retryable transport failure, got %v", err)
		}
	})
}

func TestSenderRefusesUnusableInputBeforeAnyCall(t *testing.T) {
	cases := []struct {
		name    string
		sender  Sender
		message Message
		want    error
	}{
		{
			name:    "no api key",
			sender:  Sender{APIBase: "http://127.0.0.1:1"},
			message: Message{From: "a@example.com", To: "b@example.com", Subject: "s"},
			want:    ErrNotConfigured,
		},
		{
			name:    "no recipient",
			sender:  Sender{APIKey: "re_key", APIBase: "http://127.0.0.1:1"},
			message: Message{From: "a@example.com", Subject: "s"},
			want:    ErrNoRecipient,
		},
		{
			name:    "display-name from address",
			sender:  Sender{APIKey: "re_key", APIBase: "http://127.0.0.1:1"},
			message: Message{From: "Jane <a@example.com>", To: "b@example.com", Subject: "s"},
			want:    ErrInvalidAddress,
		},
		{
			name:    "header injection in recipient",
			sender:  Sender{APIKey: "re_key", APIBase: "http://127.0.0.1:1"},
			message: Message{From: "a@example.com", To: "b@example.com\r\nBcc: c@example.com", Subject: "s"},
			want:    ErrInvalidAddress,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.sender.Send(context.Background(), tc.message); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestFormatFromQuotesUnsafeDisplayNames(t *testing.T) {
	cases := map[string]string{
		"":                  "a@example.com",
		"Rosewood Heating":  "Rosewood Heating <a@example.com>",
		"Smith, Jones & Co": `"Smith, Jones & Co" <a@example.com>`,
		// Line breaks are stripped and the colon forces quoting, so the value
		// cannot start a new header.
		"Bad\r\nBcc: x@evil": `"Bad  Bcc: x@evil" <a@example.com>`,
		`He said "hi"`:       "He said hi <a@example.com>",
	}
	for name, want := range cases {
		if got := formatFrom(name, "a@example.com"); got != want {
			t.Errorf("formatFrom(%q) = %q, want %q", name, got, want)
		}
	}
}
