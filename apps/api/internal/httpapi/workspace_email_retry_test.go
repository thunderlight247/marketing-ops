package httpapi

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"marketingops/api/internal/store"
)

// failedReplyFake is a connected workspace with one failed lead reply in its
// delivery log — the state an operator finds when a provider rejected a send.
func failedReplyFake(t *testing.T) (*emailFake, *store.EmailDelivery) {
	t.Helper()
	f := connectedEmailFake(t)
	leadID := "66666666-6666-6666-6666-666666666701"
	failed := store.EmailDelivery{
		ID: "dddddddd-dddd-dddd-dddd-ddddddddddd7", WorkspaceID: emailTestWsID,
		Kind: "lead_reply", LeadID: &leadID, ToEmail: "jane@example.com",
		Subject: "Re: furnace check", Body: "Hi Jane, Thursday at 9am works.",
		Status: "failed", Attempts: 1,
		LastError: "the email provider rejected this workspace's API key",
		CreatedAt: time.Now(),
	}
	f.deliveries = []store.EmailDelivery{failed}
	return f, &failed
}

func TestRetryFailedLeadReplyQueuesAFreshAttempt(t *testing.T) {
	f, failed := failedReplyFake(t)
	h := emailRouter(f, "http://127.0.0.1:1")

	rec := doJSON(t, h, http.MethodPost,
		"/api/v1/workspaces/"+emailTestWsID+"/email-deliveries/"+failed.ID+"/retry", "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("retry: %d %s", rec.Code, rec.Body.String())
	}
	if len(f.deliveries) != 2 {
		t.Fatalf("expected a second delivery row, got %d", len(f.deliveries))
	}
	retried := f.deliveries[1]
	if retried.Kind != "lead_reply" || retried.Status != "queued" {
		t.Errorf("retried row must be a queued reply: %+v", retried)
	}
	// The exact logged text is resent, so the lead's record and the log agree.
	if retried.ToEmail != failed.ToEmail || retried.Subject != failed.Subject || retried.Body != failed.Body {
		t.Errorf("retry must copy the logged message: %+v", retried)
	}
	if retried.LeadID == nil || *retried.LeadID != *failed.LeadID {
		t.Errorf("retry must stay on the same lead: %+v", retried)
	}
	if !strings.Contains(rec.Body.String(), `"status":"queued"`) {
		t.Errorf("response must carry the queued delivery: %s", rec.Body.String())
	}

	// The old failed row is history; it is not rewritten or deleted.
	storedOld := f.deliveries[0]
	if storedOld.ID != failed.ID || storedOld.Status != "failed" {
		t.Errorf("the failed attempt must stay readable: %+v", storedOld)
	}

	var audited bool
	for _, e := range f.auditEvents {
		if e.Action == "email_delivery_retried" {
			audited = true
			if e.Details["retriedDeliveryId"] != failed.ID {
				t.Errorf("audit must name the failed delivery: %+v", e.Details)
			}
		}
	}
	if !audited {
		t.Errorf("a retry must be audited: %+v", f.auditEvents)
	}
}

func TestRetryRefusesDeliveriesThatAreNotAFailedReply(t *testing.T) {
	f, failed := failedReplyFake(t)
	h := emailRouter(f, "http://127.0.0.1:1")

	// A message that is still moving, or already accepted, is not a failure.
	f.deliveries[0].Status = "sent"
	rec := doJSON(t, h, http.MethodPost,
		"/api/v1/workspaces/"+emailTestWsID+"/email-deliveries/"+failed.ID+"/retry", "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "failed or bounced") {
		t.Fatalf("sent delivery must not be retried: %d %s", rec.Code, rec.Body.String())
	}

	// A delivery report that is not a bounce still means the person got it.
	f.deliveries[0].Status = "sent"
	f.deliveries[0].DeliveryState = "delivered"
	rec = doJSON(t, h, http.MethodPost,
		"/api/v1/workspaces/"+emailTestWsID+"/email-deliveries/"+failed.ID+"/retry", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("a delivered reply must not be resent: %d %s", rec.Code, rec.Body.String())
	}
	f.deliveries[0].DeliveryState = ""

	// New-lead alerts are one per enquiry by design; there is nothing to retry
	// into without alerting the team twice.
	f.deliveries[0].Status = "failed"
	f.deliveries[0].Kind = "lead_notification"
	rec = doJSON(t, h, http.MethodPost,
		"/api/v1/workspaces/"+emailTestWsID+"/email-deliveries/"+failed.ID+"/retry", "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "sent once per enquiry") {
		t.Fatalf("notification must not be retried: %d %s", rec.Code, rec.Body.String())
	}

	// A reply whose lead was deleted has no one to send to.
	f.deliveries[0].Kind = "lead_reply"
	f.deliveries[0].LeadID = nil
	rec = doJSON(t, h, http.MethodPost,
		"/api/v1/workspaces/"+emailTestWsID+"/email-deliveries/"+failed.ID+"/retry", "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "no longer exists") {
		t.Fatalf("lead-less reply must be refused: %d %s", rec.Code, rec.Body.String())
	}

	// Unknown and foreign ids are indistinguishable from the outside.
	rec = doJSON(t, h, http.MethodPost,
		"/api/v1/workspaces/"+emailTestWsID+"/email-deliveries/dddddddd-dddd-dddd-dddd-ddddddddddd9/retry", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown delivery: %d %s", rec.Code, rec.Body.String())
	}
	foreign := "22222222-2222-2222-2222-222222222222"
	rec = doJSON(t, h, http.MethodPost,
		"/api/v1/workspaces/"+foreign+"/email-deliveries/"+failed.ID+"/retry", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign workspace: %d %s", rec.Code, rec.Body.String())
	}

	if len(f.deliveries) != 1 {
		t.Fatalf("refusals must not queue anything: %+v", f.deliveries)
	}
}

func TestRetryBouncedReplyQueuesAResend(t *testing.T) {
	f, failed := failedReplyFake(t)
	h := emailRouter(f, "http://127.0.0.1:1")

	// The provider accepted the message and then the recipient's server refused
	// it: the row is `sent` + `bounced`, and the person never received it.
	f.deliveries[0].Status = "sent"
	f.deliveries[0].DeliveryState = "bounced"
	f.deliveries[0].DeliveryDetail = "mailbox full"
	f.deliveries[0].LastError = ""

	rec := doJSON(t, h, http.MethodPost,
		"/api/v1/workspaces/"+emailTestWsID+"/email-deliveries/"+failed.ID+"/retry", "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("resend: %d %s", rec.Code, rec.Body.String())
	}
	if len(f.deliveries) != 2 {
		t.Fatalf("expected a resend row, got %d", len(f.deliveries))
	}
	resend := f.deliveries[1]
	if resend.Status != "queued" || resend.Kind != "lead_reply" {
		t.Errorf("resend must be a queued reply: %+v", resend)
	}
	if resend.Subject != failed.Subject || resend.Body != failed.Body || resend.ToEmail != failed.ToEmail {
		t.Errorf("resend must copy the logged message: %+v", resend)
	}
	// The bounce stays visible as what happened; it is never rewritten.
	if f.deliveries[0].Status != "sent" || f.deliveries[0].DeliveryState != "bounced" {
		t.Errorf("the bounced row must stay readable: %+v", f.deliveries[0])
	}

	var audited bool
	for _, e := range f.auditEvents {
		if e.Action == "email_delivery_retried" {
			audited = true
			if e.Details["wasBounced"] != true {
				t.Errorf("audit must record that the retry answered a bounce: %+v", e.Details)
			}
		}
	}
	if !audited {
		t.Errorf("a resend must be audited: %+v", f.auditEvents)
	}

	// The queued resend now blocks a second copy, bounced or not.
	rec = doJSON(t, h, http.MethodPost,
		"/api/v1/workspaces/"+emailTestWsID+"/email-deliveries/"+failed.ID+"/retry", "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "already queued") {
		t.Fatalf("a queued resend must block another: %d %s", rec.Code, rec.Body.String())
	}
}

func TestRetryRequiresAConnectedAccountAndALiveReply(t *testing.T) {
	f, failed := failedReplyFake(t)
	h := emailRouter(f, "http://127.0.0.1:1")

	// Without a connected account there is nothing to send through.
	f.settings = nil
	rec := doJSON(t, h, http.MethodPost,
		"/api/v1/workspaces/"+emailTestWsID+"/email-deliveries/"+failed.ID+"/retry", "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "connect an email account") {
		t.Fatalf("disconnected workspace: %d %s", rec.Code, rec.Body.String())
	}

	// With another reply already live for this lead, the uniqueness rule stops
	// a second copy from reaching the person.
	f.settings = connectedEmailFake(t).settings
	live := store.EmailDelivery{
		ID: "dddddddd-dddd-dddd-dddd-ddddddddddd8", WorkspaceID: emailTestWsID,
		Kind: "lead_reply", LeadID: failed.LeadID, ToEmail: "jane@example.com",
		Subject: "Re: furnace check", Body: "again", Status: "sent", CreatedAt: time.Now(),
	}
	f.deliveries = append(f.deliveries, live)
	rec = doJSON(t, h, http.MethodPost,
		"/api/v1/workspaces/"+emailTestWsID+"/email-deliveries/"+failed.ID+"/retry", "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "already queued or was already sent") {
		t.Fatalf("live reply must block a retry: %d %s", rec.Code, rec.Body.String())
	}
}
