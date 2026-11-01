package httpapi

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"marketingops/api/internal/store"
)

// leadHistoryFake seeds one workspace with two enquiries and the messages the
// platform tried to send about them.
func leadHistoryFake(t *testing.T) (*emailFake, string, string) {
	t.Helper()
	f := connectedEmailFake(t)
	const leadA = "66666666-6666-6666-6666-666666666801"
	const leadB = "66666666-6666-6666-6666-666666666802"
	f.leadsByID = map[string]store.Lead{
		leadA: {ID: leadA, WorkspaceID: emailTestWsID, Status: "contacted", SourceKind: "form_api"},
		leadB: {ID: leadB, WorkspaceID: emailTestWsID, Status: "new", SourceKind: "form_api"},
	}
	idA, idB := leadA, leadB
	now := time.Now()
	f.deliveries = []store.EmailDelivery{
		{
			ID: "dddddddd-dddd-dddd-dddd-dddddddddd01", WorkspaceID: emailTestWsID,
			Kind: "lead_notification", LeadID: &idA, ToEmail: "team@rosewood.example",
			Subject: "New enquiry: Jane Doe", Status: "sent", ProviderMessageID: "msg_alert",
			CreatedAt: now.Add(-2 * time.Hour), SentAt: &now,
		},
		{
			ID: "dddddddd-dddd-dddd-dddd-dddddddddd02", WorkspaceID: emailTestWsID,
			Kind: "lead_reply", LeadID: &idA, ToEmail: "jane@example.com",
			Subject: "Re: furnace check", Status: "sent", ProviderMessageID: "msg_reply",
			DeliveryState: "bounced", DeliveryDetail: "mailbox full",
			CreatedAt: now.Add(-time.Hour), SentAt: &now,
		},
		{
			ID: "dddddddd-dddd-dddd-dddd-dddddddddd03", WorkspaceID: emailTestWsID,
			Kind: "lead_reply", LeadID: &idB, ToEmail: "sam@example.com",
			Subject: "Re: quote", Status: "failed", LastError: "the provider rejected the key",
			CreatedAt: now,
		},
	}
	return f, leadA, leadB
}

func TestLeadEmailHistoryGroupsEveryAttemptForOneEnquiry(t *testing.T) {
	f, leadA, leadB := leadHistoryFake(t)
	h := emailRouter(f, "http://127.0.0.1:1")

	rec := doJSON(t, h, http.MethodGet,
		"/api/v1/workspaces/"+emailTestWsID+"/leads/"+leadA+"/email-deliveries", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("history: %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// Both attempts for this enquiry are here: the alert it produced and the
	// reply, including what the provider later said about it.
	for _, want := range []string{
		`"kind":"lead_notification"`, `"kind":"lead_reply"`,
		`"deliveryState":"bounced"`, `"deliveryDetail":"mailbox full"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("history lost %s: %s", want, body)
		}
	}
	// The other enquiry's failure is not mixed in.
	if strings.Contains(body, "sam@example.com") {
		t.Errorf("history leaked another lead's message: %s", body)
	}
	if strings.Count(body, `"id":"dddddddd`) != 2 {
		t.Errorf("expected exactly this lead's two messages: %s", body)
	}

	// The other enquiry sees only its own attempt.
	rec = doJSON(t, h, http.MethodGet,
		"/api/v1/workspaces/"+emailTestWsID+"/leads/"+leadB+"/email-deliveries?limit=5", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "sam@example.com") {
		t.Fatalf("second lead history: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "jane@example.com") {
		t.Errorf("the second lead's history must not include the first: %s", rec.Body.String())
	}

	// A lead nothing was sent about yet is an empty history, not an error.
	quiet := "66666666-6666-6666-6666-666666666803"
	f.leadsByID[quiet] = store.Lead{ID: quiet, WorkspaceID: emailTestWsID, Status: "new", SourceKind: "manual"}
	rec = doJSON(t, h, http.MethodGet,
		"/api/v1/workspaces/"+emailTestWsID+"/leads/"+quiet+"/email-deliveries", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"deliveries":[]`) {
		t.Fatalf("quiet lead history: %d %s", rec.Code, rec.Body.String())
	}
}

func TestLeadEmailHistoryRequiresAnOwnedLead(t *testing.T) {
	f, leadA, _ := leadHistoryFake(t)
	h := emailRouter(f, "http://127.0.0.1:1")

	// An unknown lead is a 404, not an empty list that would look like "nothing
	// was ever sent to this person".
	rec := doJSON(t, h, http.MethodGet,
		"/api/v1/workspaces/"+emailTestWsID+"/leads/66666666-6666-6666-6666-666666666899/email-deliveries", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown lead: %d %s", rec.Code, rec.Body.String())
	}

	// Another workspace's path cannot read this lead's history.
	rec = doJSON(t, h, http.MethodGet,
		"/api/v1/workspaces/22222222-2222-2222-2222-222222222222/leads/"+leadA+"/email-deliveries", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign workspace: %d %s", rec.Code, rec.Body.String())
	}
}
