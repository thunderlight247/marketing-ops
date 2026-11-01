package httpapi

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"marketingops/api/internal/store"
)

func TestWorkspaceListFlagsDeliveryProblems(t *testing.T) {
	f := connectedEmailFake(t)
	const quiet = "33333333-3333-3333-3333-333333333333"
	f.listResult = []store.Workspace{
		{ID: emailTestWsID, Slug: "demo", Name: "Demo Studio", Status: "active"},
		{ID: quiet, Slug: "quiet-co", Name: "Quiet Co", Status: "active"},
	}
	leadID := "66666666-6666-6666-6666-666666666901"
	now := time.Now()
	f.deliveries = []store.EmailDelivery{
		{
			ID: "dddddddd-dddd-dddd-dddd-dddddddddd11", WorkspaceID: emailTestWsID,
			Kind: "lead_reply", LeadID: &leadID, ToEmail: "jane@example.com",
			Subject: "Re: enquiry", Status: "failed", LastError: "the provider rejected the key",
			CreatedAt: now,
		},
		{
			ID: "dddddddd-dddd-dddd-dddd-dddddddddd12", WorkspaceID: emailTestWsID,
			Kind: "lead_reply", LeadID: &leadID, ToEmail: "jane@example.com",
			Subject: "Re: enquiry", Status: "sent", DeliveryState: "bounced",
			CreatedAt: now,
		},
		{
			// Accepted and delivered: nothing to look at.
			ID: "dddddddd-dddd-dddd-dddd-dddddddddd13", WorkspaceID: emailTestWsID,
			Kind: "lead_notification", LeadID: &leadID, ToEmail: "team@rosewood.example",
			Subject: "New enquiry", Status: "sent", DeliveryState: "delivered",
			CreatedAt: now,
		},
	}
	h := emailRouter(f, "http://127.0.0.1:1")

	rec := doJSON(t, h, http.MethodGet, "/api/v1/workspaces", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"deliveryAttention":{"failed":1,"bounced":1}`) {
		t.Errorf("the failed and bounced messages must be surfaced: %s", body)
	}
	// Only the workspace with problems carries the field.
	if strings.Count(body, "deliveryAttention") != 1 {
		t.Errorf("a clean workspace must not be flagged: %s", body)
	}
	// The list itself is still complete.
	if !strings.Contains(body, `"name":"Quiet Co"`) || !strings.Contains(body, `"name":"Demo Studio"`) {
		t.Errorf("the list must still contain every workspace: %s", body)
	}
}

func TestAcknowledgingClearsDeliveryProblemsUntilTheNextOne(t *testing.T) {
	f := connectedEmailFake(t)
	f.listResult = []store.Workspace{
		{ID: emailTestWsID, Slug: "demo", Name: "Demo Studio", Status: "active"},
	}
	leadID := "66666666-6666-6666-6666-666666666902"
	now := time.Now()
	f.deliveries = []store.EmailDelivery{
		{
			ID: "dddddddd-dddd-dddd-dddd-dddddddddd21", WorkspaceID: emailTestWsID,
			Kind: "lead_reply", LeadID: &leadID, ToEmail: "jane@example.com",
			Subject: "Re: enquiry", Status: "failed", LastError: "provider rejected the key",
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "dddddddd-dddd-dddd-dddd-dddddddddd22", WorkspaceID: emailTestWsID,
			Kind: "lead_reply", LeadID: &leadID, ToEmail: "jane@example.com",
			Subject: "Re: enquiry", Status: "sent", DeliveryState: "bounced",
			CreatedAt: now, UpdatedAt: now,
		},
	}
	h := emailRouter(f, "http://127.0.0.1:1")

	list := func() string {
		t.Helper()
		rec := doJSON(t, h, http.MethodGet, "/api/v1/workspaces", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}
	if body := list(); !strings.Contains(body, `"deliveryAttention":{"failed":1,"bounced":1}`) {
		t.Fatalf("the problems must be listed before acknowledging: %s", body)
	} else if strings.Contains(body, "deliveryAcknowledgedAt") {
		t.Errorf("a never-acknowledged workspace must not claim an acknowledgement: %s", body)
	}

	rec := doJSON(t, h, http.MethodPost,
		"/api/v1/workspaces/"+emailTestWsID+"/delivery-attention/acknowledge", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"acknowledgedAt"`) {
		t.Fatalf("acknowledge: %d %s", rec.Code, rec.Body.String())
	}
	if body := list(); strings.Contains(body, "deliveryAttention") {
		t.Errorf("acknowledged problems must clear the badge: %s", body)
	} else if !strings.Contains(body, `"deliveryAcknowledgedAt":"`) {
		t.Errorf("the list must say when the problems were acknowledged: %s", body)
	}
	// The log itself is untouched: acknowledging is not a deletion.
	if len(f.deliveries) != 2 {
		t.Errorf("acknowledging must not remove deliveries: %d", len(f.deliveries))
	}

	// A failure that arrives afterwards is new work and raises the badge again.
	f.deliveries = append(f.deliveries, store.EmailDelivery{
		ID: "dddddddd-dddd-dddd-dddd-dddddddddd23", WorkspaceID: emailTestWsID,
		Kind: "lead_reply", LeadID: &leadID, ToEmail: "sam@example.com",
		Subject: "Re: enquiry", Status: "failed", LastError: "provider timeout",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if body := list(); !strings.Contains(body, `"deliveryAttention":{"failed":1,"bounced":0}`) {
		t.Errorf("a new failure must raise the badge again: %s", body)
	}

	// An unknown workspace is a 404, not an acknowledgement of nothing.
	missing := doJSON(t, h, http.MethodPost,
		"/api/v1/workspaces/99999999-9999-9999-9999-999999999999/delivery-attention/acknowledge", "")
	if missing.Code != http.StatusNotFound {
		t.Errorf("unknown workspace: %d %s", missing.Code, missing.Body.String())
	}
}

func TestAcknowledgedProblemsListWhatWasDismissed(t *testing.T) {
	f := connectedEmailFake(t)
	leadID := "66666666-6666-6666-6666-666666666903"
	now := time.Now()
	f.deliveries = []store.EmailDelivery{
		{
			ID: "dddddddd-dddd-dddd-dddd-dddddddddd31", WorkspaceID: emailTestWsID,
			Kind: "lead_reply", LeadID: &leadID, ToEmail: "jane@example.com",
			Subject: "Re: enquiry", Status: "failed", LastError: "provider rejected the key",
			CreatedAt: now.Add(-3 * time.Minute), UpdatedAt: now.Add(-2 * time.Minute),
		},
		{
			ID: "dddddddd-dddd-dddd-dddd-dddddddddd32", WorkspaceID: emailTestWsID,
			Kind: "lead_reply", LeadID: &leadID, ToEmail: "sam@example.com",
			Subject: "Re: quote", Status: "sent", DeliveryState: "bounced",
			DeliveryDetail: "mailbox full",
			CreatedAt:      now.Add(-4 * time.Minute), UpdatedAt: now.Add(-1 * time.Minute),
		},
		{
			// Accepted and delivered: never a problem, so never acknowledged.
			ID: "dddddddd-dddd-dddd-dddd-dddddddddd33", WorkspaceID: emailTestWsID,
			Kind: "lead_reply", LeadID: &leadID, ToEmail: "team@example.com",
			Subject: "Re: enquiry", Status: "sent", DeliveryState: "delivered",
			CreatedAt: now.Add(-5 * time.Minute), UpdatedAt: now.Add(-5 * time.Minute),
		},
	}
	h := emailRouter(f, "http://127.0.0.1:1")

	get := func(query string) string {
		t.Helper()
		rec := doJSON(t, h, http.MethodGet,
			"/api/v1/workspaces/"+emailTestWsID+"/email-deliveries/acknowledged"+query, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("acknowledged problems: %d %s", rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}

	// Nothing dismissed yet: no timestamp, no list.
	if body := get(""); !strings.Contains(body, `"acknowledgedAt":null`) ||
		strings.Contains(body, `"deliveries":[{"`) {
		t.Fatalf("an unacknowledged workspace has nothing to list: %s", body)
	}

	if rec := doJSON(t, h, http.MethodPost,
		"/api/v1/workspaces/"+emailTestWsID+"/delivery-attention/acknowledge", ""); rec.Code != http.StatusOK {
		t.Fatalf("acknowledge: %d %s", rec.Code, rec.Body.String())
	}

	body := get("")
	if !strings.Contains(body, `"acknowledgedAt":"`) {
		t.Errorf("the list must carry when the dismissal happened: %s", body)
	}
	for _, id := range []string{"dddddddd-dddd-dddd-dddd-dddddddddd31", "dddddddd-dddd-dddd-dddd-dddddddddd32"} {
		if !strings.Contains(body, id) {
			t.Errorf("dismissed problem %s must be listed: %s", id, body)
		}
	}
	if strings.Contains(body, "dddddddd-dddd-dddd-dddd-dddddddddd33") {
		t.Errorf("a delivered message was never a problem: %s", body)
	}

	// A problem recorded after the acknowledgement belongs to the badge, not
	// to this list.
	f.deliveries = append(f.deliveries, store.EmailDelivery{
		ID: "dddddddd-dddd-dddd-dddd-dddddddddd34", WorkspaceID: emailTestWsID,
		Kind: "lead_reply", LeadID: &leadID, ToEmail: "new@example.com",
		Subject: "Re: enquiry", Status: "failed", LastError: "provider timeout",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if body := get(""); strings.Contains(body, "dddddddd-dddd-dddd-dddd-dddddddddd34") {
		t.Errorf("a problem after the acknowledgement must not be called dismissed: %s", body)
	}

	// The limit applies like the delivery log's.
	if body := get("?limit=1"); strings.Count(body, `"id":"`) != 1 {
		t.Errorf("limit=1 must return one dismissed problem: %s", body)
	}

	missing := doJSON(t, h, http.MethodGet,
		"/api/v1/workspaces/99999999-9999-9999-9999-999999999999/email-deliveries/acknowledged", "")
	if missing.Code != http.StatusNotFound {
		t.Errorf("unknown workspace: %d %s", missing.Code, missing.Body.String())
	}
}
