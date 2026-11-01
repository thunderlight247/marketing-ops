package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func emailTestLead(t *testing.T, s *Store, workspaceID string) Lead {
	t.Helper()
	email := "jane@example.com"
	l, err := s.InsertLead(context.Background(), Lead{
		WorkspaceID:  workspaceID,
		SourceKind:   "form_api",
		SourceDetail: "contact form",
		ContactEmail: &email,
	})
	if err != nil {
		t.Fatalf("seed lead: %v", err)
	}
	return l
}

func TestEmailSettingsLifecycle(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()
	ws := uniqueWorkspace(t, s, "email-settings")
	other := uniqueWorkspace(t, s, "email-settings-other")

	if _, err := s.GetEmailSettings(ctx, ws.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unconfigured workspace must be ErrNotFound, got %v", err)
	}

	saved, err := s.SaveEmailSettings(ctx, EmailSettings{
		WorkspaceID: ws.ID, Provider: "resend", FromName: "Rosewood Heating",
		FromEmail: "hello@rosewood.example", NotifyEmail: "team@rosewood.example",
		APIKeyCiphertext: []byte("sealed-key"), APIKeyVersion: 1, Status: "unverified",
	})
	if err != nil {
		t.Fatalf("SaveEmailSettings: %v", err)
	}
	if saved.Status != "unverified" || saved.CreatedAt.IsZero() {
		t.Fatalf("saved row incomplete: %+v", saved)
	}

	// A second save updates the one row instead of adding another connection.
	updated, err := s.SaveEmailSettings(ctx, EmailSettings{
		WorkspaceID: ws.ID, Provider: "resend", FromName: "Rosewood Heating",
		FromEmail: "studio@rosewood.example", NotifyEmail: "",
		APIKeyCiphertext: []byte("sealed-key-2"), APIKeyVersion: 1, Status: "unverified",
	})
	if err != nil {
		t.Fatalf("SaveEmailSettings update: %v", err)
	}
	if updated.FromEmail != "studio@rosewood.example" || updated.NotifyEmail != "" {
		t.Errorf("update did not apply: %+v", updated)
	}
	var count int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM workspace_email_settings WHERE workspace_id = $1`, ws.ID).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected one settings row per workspace, got %d", count)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if err := s.MarkEmailSettingsStatus(ctx, ws.ID, "connected", &now); err != nil {
		t.Fatalf("MarkEmailSettingsStatus: %v", err)
	}
	got, err := s.GetEmailSettings(ctx, ws.ID)
	if err != nil {
		t.Fatalf("GetEmailSettings: %v", err)
	}
	if got.Status != "connected" || got.LastVerifiedAt == nil || !got.LastVerifiedAt.Equal(now) {
		t.Errorf("verification not recorded: %+v", got)
	}
	if err := s.MarkEmailSettingsStatus(ctx, ws.ID, "viral", nil); !errors.Is(err, ErrInvalidStatus) {
		t.Errorf("unknown status must be refused, got %v", err)
	}

	// Workspace isolation: another workspace sees nothing.
	if _, err := s.GetEmailSettings(ctx, other.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-workspace read must be ErrNotFound, got %v", err)
	}
	if err := s.MarkEmailSettingsStatus(ctx, other.ID, "connected", nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-workspace status update must be ErrNotFound, got %v", err)
	}

	if err := s.DeleteEmailSettings(ctx, ws.ID); err != nil {
		t.Fatalf("DeleteEmailSettings: %v", err)
	}
	if _, err := s.GetEmailSettings(ctx, ws.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted settings must be gone, got %v", err)
	}
	if err := s.DeleteEmailSettings(ctx, ws.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting twice must be ErrNotFound, got %v", err)
	}
}

func TestEmailDeliveriesAreClaimedAndCompletedOnce(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()
	ws := uniqueWorkspace(t, s, "email-send")
	lead := emailTestLead(t, s, ws.ID)

	reply, created, err := s.EnqueueEmailDelivery(ctx, EmailDelivery{
		WorkspaceID: ws.ID, Kind: "lead_reply", LeadID: &lead.ID,
		ToEmail: "jane@example.com", Subject: "Re: your enquiry",
		Body: "Hi Jane, we can visit on Thursday.",
	})
	if err != nil || !created {
		t.Fatalf("enqueue reply: created=%v err=%v", created, err)
	}
	if reply.Status != "queued" || reply.Attempts != 0 || reply.NextAttemptAt == nil {
		t.Fatalf("queued row incomplete: %+v", reply)
	}

	// A second live reply to the same lead is refused by the database.
	if _, created, err := s.EnqueueEmailDelivery(ctx, EmailDelivery{
		WorkspaceID: ws.ID, Kind: "lead_reply", LeadID: &lead.ID,
		ToEmail: "jane@example.com", Subject: "Re: your enquiry", Body: "again",
	}); err != nil || created {
		t.Fatalf("duplicate live reply must not be created: created=%v err=%v", created, err)
	}

	// One alert per lead, however often the capture path retries.
	for i := 0; i < 2; i++ {
		_, created, err := s.EnqueueEmailDelivery(ctx, EmailDelivery{
			WorkspaceID: ws.ID, Kind: "lead_notification", LeadID: &lead.ID,
			ToEmail: "team@rosewood.example", Subject: "New enquiry", Body: "Someone wrote in.",
		})
		if err != nil {
			t.Fatalf("enqueue notification: %v", err)
		}
		if created != (i == 0) {
			t.Errorf("notification attempt %d created=%v", i, created)
		}
	}

	now := time.Now().UTC()
	claimed, err := s.ClaimDueEmailDelivery(ctx, 5, now)
	if err != nil {
		t.Fatalf("ClaimDueEmailDelivery: %v", err)
	}
	if claimed.ID != reply.ID || claimed.Status != "sending" || claimed.Attempts != 1 {
		t.Fatalf("claim mismatch: %+v", claimed)
	}
	// The claimed row is no longer available to a second worker.
	if _, err := s.ClaimDueEmailDelivery(ctx, 5, now); err != nil {
		// The notification is still queued, so a second claim is legitimate.
		t.Fatalf("second claim should find the other due message: %v", err)
	}

	if err := s.CompleteEmailDeliverySent(ctx, reply.ID, "msg_42"); err != nil {
		t.Fatalf("CompleteEmailDeliverySent: %v", err)
	}
	// Completing twice must not double-book the send.
	if err := s.CompleteEmailDeliverySent(ctx, reply.ID, "msg_42"); !errors.Is(err, ErrConflict) {
		t.Errorf("second completion must be ErrConflict, got %v", err)
	}

	stored, err := s.GetLeadInWorkspace(ctx, ws.ID, lead.ID)
	if err != nil {
		t.Fatalf("lead lookup: %v", err)
	}
	if stored.ReplySentAt == nil {
		t.Error("a provider-accepted reply must be recorded on the lead")
	}
	if stored.Status != "contacted" {
		t.Errorf("a sent first reply must move a new lead to contacted, got %q", stored.Status)
	}

	list, err := s.ListEmailDeliveries(ctx, ws.ID, 10)
	if err != nil {
		t.Fatalf("ListEmailDeliveries: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected the reply and one notification, got %d", len(list))
	}
	var sentRow EmailDelivery
	for _, d := range list {
		if d.ID == reply.ID {
			sentRow = d
		}
	}
	if sentRow.Status != "sent" || sentRow.ProviderMessageID != "msg_42" || sentRow.SentAt == nil {
		t.Errorf("sent row incomplete: %+v", sentRow)
	}

	// Another workspace sees none of it.
	other := uniqueWorkspace(t, s, "email-send-other")
	if rows, err := s.ListEmailDeliveries(ctx, other.ID, 10); err != nil || len(rows) != 0 {
		t.Errorf("cross-workspace listing leaked rows: %d err=%v", len(rows), err)
	}
}

func TestEmailDeliveryRetryAndTerminalStates(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()
	ws := uniqueWorkspace(t, s, "email-retry")
	lead := emailTestLead(t, s, ws.ID)

	reply, _, err := s.EnqueueEmailDelivery(ctx, EmailDelivery{
		WorkspaceID: ws.ID, Kind: "lead_reply", LeadID: &lead.ID,
		ToEmail: "jane@example.com", Subject: "Re: enquiry", Body: "body",
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	now := time.Now().UTC()

	claimed, err := s.ClaimDueEmailDelivery(ctx, 2, now)
	if err != nil || claimed.ID != reply.ID {
		t.Fatalf("claim: %+v err=%v", claimed, err)
	}
	retryAt := now.Add(time.Minute)
	if err := s.CompleteEmailDeliveryFailed(ctx, reply.ID, "the email provider is temporarily unavailable", &retryAt); err != nil {
		t.Fatalf("CompleteEmailDeliveryFailed: %v", err)
	}
	// Not due yet.
	if _, err := s.ClaimDueEmailDelivery(ctx, 2, now); !errors.Is(err, ErrNotFound) {
		t.Errorf("a retry-scheduled message must not be claimed early, got %v", err)
	}
	// Due now: second attempt.
	claimed, err = s.ClaimDueEmailDelivery(ctx, 2, now.Add(2*time.Minute))
	if err != nil || claimed.Attempts != 2 {
		t.Fatalf("second claim: %+v err=%v", claimed, err)
	}
	// Bounded attempts: at the cap the claim query refuses the row.
	if err := s.CompleteEmailDeliveryFailed(ctx, reply.ID, "still unavailable", &retryAt); err != nil {
		t.Fatalf("second failure: %v", err)
	}
	if _, err := s.ClaimDueEmailDelivery(ctx, 2, now.Add(10*time.Minute)); !errors.Is(err, ErrNotFound) {
		t.Errorf("attempts at the cap must stop the retries, got %v", err)
	}

	// Terminal failure: nil next attempt.
	terminal, _, err := s.EnqueueEmailDelivery(ctx, EmailDelivery{
		WorkspaceID: ws.ID, Kind: "lead_reply", LeadID: &lead.ID,
		ToEmail: "jane@example.com", Subject: "Re: enquiry", Body: "body again",
	})
	if err != nil {
		t.Fatalf("enqueue after exhausted attempts: %v", err)
	}
	claimed, err = s.ClaimDueEmailDelivery(ctx, 2, now.Add(20*time.Minute))
	if err != nil || claimed.ID != terminal.ID {
		t.Fatalf("claim terminal: %+v err=%v", claimed, err)
	}
	if err := s.CompleteEmailDeliveryFailed(ctx, terminal.ID, "the email provider rejected this workspace's API key", nil); err != nil {
		t.Fatalf("terminal failure: %v", err)
	}
	if _, err := s.ClaimDueEmailDelivery(ctx, 2, now.Add(time.Hour)); !errors.Is(err, ErrNotFound) {
		t.Errorf("a terminal failure must wait for an operator, got %v", err)
	}

	// After a terminal failure a fresh reply may be queued.
	if _, created, err := s.EnqueueEmailDelivery(ctx, EmailDelivery{
		WorkspaceID: ws.ID, Kind: "lead_reply", LeadID: &lead.ID,
		ToEmail: "jane@example.com", Subject: "Re: enquiry", Body: "one more try",
	}); err != nil || !created {
		t.Errorf("a failed reply must be retryable as a new delivery: created=%v err=%v", created, err)
	}

	// A retry starts from the failed row, which is read back scoped to its own
	// workspace: another workspace's path cannot even see it.
	stored, err := s.GetEmailDeliveryInWorkspace(ctx, ws.ID, terminal.ID)
	if err != nil {
		t.Fatalf("GetEmailDeliveryInWorkspace: %v", err)
	}
	if stored.Status != "failed" || stored.Subject != "Re: enquiry" || stored.ToEmail != "jane@example.com" {
		t.Errorf("failed row not readable for a retry: %+v", stored)
	}
	other := uniqueWorkspace(t, s, "email-retry-other")
	if _, err := s.GetEmailDeliveryInWorkspace(ctx, other.ID, terminal.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-workspace delivery read must be ErrNotFound, got %v", err)
	}
	if _, err := s.GetEmailDeliveryInWorkspace(ctx, ws.ID, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown delivery must be ErrNotFound, got %v", err)
	}
}

func TestRecoverStuckEmailDeliveries(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()
	ws := uniqueWorkspace(t, s, "email-stuck")
	lead := emailTestLead(t, s, ws.ID)

	d, _, err := s.EnqueueEmailDelivery(ctx, EmailDelivery{
		WorkspaceID: ws.ID, Kind: "lead_notification", LeadID: &lead.ID,
		ToEmail: "team@rosewood.example", Subject: "New enquiry", Body: "body",
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if _, err := s.ClaimDueEmailDelivery(ctx, 5, time.Now().UTC()); err != nil {
		t.Fatalf("claim: %v", err)
	}
	// Pretend the worker died an hour ago mid-attempt.
	if _, err := s.pool.Exec(ctx,
		`UPDATE email_deliveries SET updated_at = now() - interval '1 hour' WHERE id = $1`, d.ID); err != nil {
		t.Fatalf("age the row: %v", err)
	}
	recovered, err := s.RecoverStuckEmailDeliveries(ctx, 10*time.Minute, time.Now().UTC())
	if err != nil {
		t.Fatalf("RecoverStuckEmailDeliveries: %v", err)
	}
	if recovered < 1 {
		t.Fatalf("expected the abandoned row to be recovered, got %d", recovered)
	}
	list, err := s.ListEmailDeliveries(ctx, ws.ID, 5)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %d err=%v", len(list), err)
	}
	if list[0].Status != "failed" || list[0].NextAttemptAt != nil {
		t.Errorf("an interrupted send must wait for an operator, got %+v", list[0])
	}
	if list[0].LastError == "" {
		t.Error("expected an explanation on the recovered row")
	}
}

func TestEmailProviderEventsMoveDeliveryStateForward(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()
	ws := uniqueWorkspace(t, s, "email-events")
	other := uniqueWorkspace(t, s, "email-events-other")

	var deliveryID string
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO email_deliveries
			(workspace_id, kind, to_email, subject, body, status, provider_message_id, sent_at)
		 VALUES ($1, 'lead_notification', 'team@rosewood.example', 'New enquiry', 'body', 'sent', 'msg_evt_1', now())
		 RETURNING id`, ws.ID).Scan(&deliveryID); err != nil {
		t.Fatalf("seed sent delivery: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	res, err := s.ApplyEmailProviderEvent(ctx, EmailProviderEvent{
		WorkspaceID: ws.ID, EventID: "evt_delivered", EventType: "email.delivered",
		MessageID: "msg_evt_1", State: "delivered", At: now,
	})
	if err != nil {
		t.Fatalf("ApplyEmailProviderEvent: %v", err)
	}
	if !res.Found || !res.Updated || res.DeliveryID != deliveryID {
		t.Errorf("result = %+v, want the delivery updated", res)
	}
	rows, err := s.ListEmailDeliveries(ctx, ws.ID, 5)
	if err != nil || len(rows) != 1 {
		t.Fatalf("list: %d err=%v", len(rows), err)
	}
	if rows[0].DeliveryState != "delivered" || rows[0].DeliveryStateAt == nil {
		t.Errorf("delivery state not recorded: %+v", rows[0])
	}

	// The provider retries the same event id: nothing moves twice.
	res, err = s.ApplyEmailProviderEvent(ctx, EmailProviderEvent{
		WorkspaceID: ws.ID, EventID: "evt_delivered", EventType: "email.delivered",
		MessageID: "msg_evt_1", State: "delivered", At: now,
	})
	if err != nil || !res.Duplicate || res.Updated {
		t.Errorf("duplicate event = %+v err=%v", res, err)
	}

	// A late bounce that happened before the delivery report is recorded but
	// does not rewrite the newer fact.
	res, err = s.ApplyEmailProviderEvent(ctx, EmailProviderEvent{
		WorkspaceID: ws.ID, EventID: "evt_bounce_old", EventType: "email.bounced",
		MessageID: "msg_evt_1", State: "bounced", Detail: "mailbox unavailable",
		At: now.Add(-time.Minute),
	})
	if err != nil || !res.Found || res.Updated {
		t.Errorf("out-of-order event = %+v err=%v", res, err)
	}
	rows, _ = s.ListEmailDeliveries(ctx, ws.ID, 5)
	if rows[0].DeliveryState != "delivered" {
		t.Errorf("an older report must not overwrite a newer one: %+v", rows[0])
	}

	// A later bounce is the newest fact and wins.
	res, err = s.ApplyEmailProviderEvent(ctx, EmailProviderEvent{
		WorkspaceID: ws.ID, EventID: "evt_bounce_new", EventType: "email.bounced",
		MessageID: "msg_evt_1", State: "bounced", Detail: "mailbox unavailable",
		At: now.Add(time.Minute),
	})
	if err != nil || !res.Updated {
		t.Errorf("newer event = %+v err=%v", res, err)
	}
	rows, _ = s.ListEmailDeliveries(ctx, ws.ID, 5)
	if rows[0].DeliveryState != "bounced" || rows[0].DeliveryDetail != "mailbox unavailable" {
		t.Errorf("newest state not applied: %+v", rows[0])
	}

	// Another workspace cannot be moved by a callback carrying this message id.
	res, err = s.ApplyEmailProviderEvent(ctx, EmailProviderEvent{
		WorkspaceID: other.ID, EventID: "evt_other", EventType: "email.delivered",
		MessageID: "msg_evt_1", State: "delivered", At: now,
	})
	if err != nil || res.Found {
		t.Errorf("cross-workspace callback = %+v err=%v", res, err)
	}

	// An unknown message is acknowledged, never guessed at.
	res, err = s.ApplyEmailProviderEvent(ctx, EmailProviderEvent{
		WorkspaceID: ws.ID, EventID: "evt_unknown", EventType: "email.delivered",
		MessageID: "msg_never_sent", State: "delivered", At: now,
	})
	if err != nil || res.Found {
		t.Errorf("unknown message = %+v err=%v", res, err)
	}
}

func TestBouncedReplyCanBeResent(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()
	ws := uniqueWorkspace(t, s, "email-resend")
	lead := emailTestLead(t, s, ws.ID)

	// The provider accepted this message and the recipient's server then
	// refused it: it is `sent` + `bounced`, and the person never got it.
	var bouncedID string
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO email_deliveries
			(workspace_id, kind, lead_id, to_email, subject, body, status,
			 provider_message_id, sent_at, delivery_state, delivery_state_at, delivery_detail)
		 VALUES ($1, 'lead_reply', $2, 'jane@example.com', 'Re: enquiry', 'body', 'sent',
			 'msg_bounce', now(), 'bounced', now(), 'mailbox full')
		 RETURNING id`, ws.ID, lead.ID).Scan(&bouncedID); err != nil {
		t.Fatalf("seed bounced delivery: %v", err)
	}

	// The bounced acceptance is finished, not live, so a resend is allowed.
	resend, created, err := s.EnqueueEmailDelivery(ctx, EmailDelivery{
		WorkspaceID: ws.ID, Kind: "lead_reply", LeadID: &lead.ID,
		ToEmail: "jane@example.com", Subject: "Re: enquiry", Body: "body",
	})
	if err != nil || !created {
		t.Fatalf("a bounced reply must be resendable: created=%v err=%v", created, err)
	}

	// While that resend is queued, no second copy may go out.
	if _, created, err := s.EnqueueEmailDelivery(ctx, EmailDelivery{
		WorkspaceID: ws.ID, Kind: "lead_reply", LeadID: &lead.ID,
		ToEmail: "jane@example.com", Subject: "Re: enquiry", Body: "again",
	}); err != nil || created {
		t.Errorf("a live resend must block another: created=%v err=%v", created, err)
	}

	// Once accepted and reported delivered, the resend is live too.
	if _, err := s.pool.Exec(ctx,
		`UPDATE email_deliveries SET status = 'sent', delivery_state = 'delivered' WHERE id = $1`,
		resend.ID); err != nil {
		t.Fatalf("mark resend delivered: %v", err)
	}
	if _, created, err := s.EnqueueEmailDelivery(ctx, EmailDelivery{
		WorkspaceID: ws.ID, Kind: "lead_reply", LeadID: &lead.ID,
		ToEmail: "jane@example.com", Subject: "Re: enquiry", Body: "third",
	}); err != nil || created {
		t.Errorf("a delivered reply must not be resent: created=%v err=%v", created, err)
	}

	// The bounce itself is untouched history.
	stored, err := s.GetEmailDeliveryInWorkspace(ctx, ws.ID, bouncedID)
	if err != nil {
		t.Fatalf("bounced row lookup: %v", err)
	}
	if stored.Status != "sent" || stored.DeliveryState != "bounced" || stored.DeliveryDetail != "mailbox full" {
		t.Errorf("the bounced row must stay as it was: %+v", stored)
	}
}

func TestLeadEmailHistoryIsOneEnquirysAttempts(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()
	ws := uniqueWorkspace(t, s, "email-history")
	other := uniqueWorkspace(t, s, "email-history-other")
	mine := emailTestLead(t, s, ws.ID)
	second := emailTestLead(t, s, ws.ID)
	foreign := emailTestLead(t, s, other.ID)

	if _, _, err := s.EnqueueEmailDelivery(ctx, EmailDelivery{
		WorkspaceID: ws.ID, Kind: "lead_notification", LeadID: &mine.ID,
		ToEmail: "team@rosewood.example", Subject: "New enquiry", Body: "body",
	}); err != nil {
		t.Fatalf("enqueue alert: %v", err)
	}
	if _, _, err := s.EnqueueEmailDelivery(ctx, EmailDelivery{
		WorkspaceID: ws.ID, Kind: "lead_reply", LeadID: &mine.ID,
		ToEmail: "jane@example.com", Subject: "Re: enquiry", Body: "reply",
	}); err != nil {
		t.Fatalf("enqueue reply: %v", err)
	}
	if _, _, err := s.EnqueueEmailDelivery(ctx, EmailDelivery{
		WorkspaceID: ws.ID, Kind: "lead_reply", LeadID: &second.ID,
		ToEmail: "sam@example.com", Subject: "Re: other", Body: "other reply",
	}); err != nil {
		t.Fatalf("enqueue second lead reply: %v", err)
	}
	if _, _, err := s.EnqueueEmailDelivery(ctx, EmailDelivery{
		WorkspaceID: other.ID, Kind: "lead_reply", LeadID: &foreign.ID,
		ToEmail: "lee@example.com", Subject: "Re: foreign", Body: "foreign reply",
	}); err != nil {
		t.Fatalf("enqueue foreign reply: %v", err)
	}

	history, err := s.ListLeadEmailDeliveries(ctx, ws.ID, mine.ID, 25)
	if err != nil {
		t.Fatalf("ListLeadEmailDeliveries: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected the alert and the reply, got %d: %+v", len(history), history)
	}
	for _, d := range history {
		if d.LeadID == nil || *d.LeadID != mine.ID {
			t.Errorf("history contains another lead's message: %+v", d)
		}
	}

	// Another workspace's path cannot read this lead's history at all.
	if rows, err := s.ListLeadEmailDeliveries(ctx, other.ID, mine.ID, 25); err != nil || len(rows) != 0 {
		t.Errorf("cross-workspace history leaked rows: %d err=%v", len(rows), err)
	}
	// A lead nothing was sent about is an empty history.
	quiet := emailTestLead(t, s, ws.ID)
	if rows, err := s.ListLeadEmailDeliveries(ctx, ws.ID, quiet.ID, 25); err != nil || len(rows) != 0 {
		t.Errorf("quiet lead history: %d rows err=%v", len(rows), err)
	}
}

func TestEmailDeliveryAttentionCountsOnlyProblems(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()
	ws := uniqueWorkspace(t, s, "email-attention")
	clean := uniqueWorkspace(t, s, "email-attention-clean")

	// One lead per row: the schema allows only one live reply per lead.
	seed := func(kind, status, state string) {
		t.Helper()
		lead := emailTestLead(t, s, ws.ID)
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO email_deliveries
				(workspace_id, kind, lead_id, to_email, subject, body, status, delivery_state)
			 VALUES ($1, $2, $3, 'jane@example.com', 'Re: enquiry', 'body', $4, $5)`,
			ws.ID, kind, lead.ID, status, state); err != nil {
			t.Fatalf("seed %s/%s: %v", kind, status, err)
		}
	}
	seed("lead_reply", "failed", "unknown")
	seed("lead_reply", "failed", "unknown")
	seed("lead_reply", "sent", "bounced")
	// Accepted and delivered, or still on its way: nothing a human must fix.
	seed("lead_reply", "sent", "delivered")
	seed("lead_reply", "sent", "delayed")
	seed("lead_notification", "queued", "unknown")

	attention, err := s.EmailDeliveryAttention(ctx)
	if err != nil {
		t.Fatalf("attention: %v", err)
	}
	if got := attention[ws.ID]; got.Failed != 2 || got.Bounced != 1 {
		t.Errorf("failed and bounced must be counted per workspace: %+v", got)
	}
	if _, ok := attention[clean.ID]; ok {
		t.Errorf("a workspace with a clean log must be omitted: %+v", attention[clean.ID])
	}
}

func TestAcknowledgingDeliveryProblemsHidesOnlyOlderOnes(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()
	ws := uniqueWorkspace(t, s, "email-ack")
	lead := emailTestLead(t, s, ws.ID)
	other := emailTestLead(t, s, ws.ID)

	// One send the provider refused two minutes ago, one bounce reported a
	// minute ago: both are older than the acknowledgement that follows.
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO email_deliveries
			(workspace_id, kind, lead_id, to_email, subject, body, status, updated_at)
		 VALUES ($1, 'lead_reply', $2, 'jane@example.com', 'Re: enquiry', 'body',
			'failed', now() - interval '2 minutes')`, ws.ID, lead.ID); err != nil {
		t.Fatalf("seed failed send: %v", err)
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO email_deliveries
			(workspace_id, kind, lead_id, to_email, subject, body, status,
			 provider_message_id, sent_at, delivery_state, delivery_state_at)
		 VALUES ($1, 'lead_reply', $2, 'jane@example.com', 'Re: enquiry', 'body', 'sent',
			'msg_ack', now() - interval '3 minutes', 'bounced', now() - interval '1 minute')`,
		ws.ID, other.ID); err != nil {
		t.Fatalf("seed bounce: %v", err)
	}

	if a, err := s.EmailDeliveryAttention(ctx); err != nil {
		t.Fatalf("attention before ack: %v", err)
	} else if a[ws.ID].Failed != 1 || a[ws.ID].Bounced != 1 {
		t.Fatalf("both problems must be counted before ack: %+v", a[ws.ID])
	}

	at, err := s.AcknowledgeEmailDeliveryAttention(ctx, ws.ID)
	if err != nil {
		t.Fatalf("acknowledge: %v", err)
	}
	// The dashboard lists acknowledgements so an operator can see what a
	// cleared badge covers.
	if acks, err := s.EmailDeliveryAcknowledgements(ctx); err != nil {
		t.Fatalf("acknowledgements: %v", err)
	} else if got, ok := acks[ws.ID]; !ok || !got.Equal(at) {
		t.Errorf("the acknowledgement must be listed: got %v want %v", got, at)
	}

	if a, err := s.EmailDeliveryAttention(ctx); err != nil {
		t.Fatalf("attention after ack: %v", err)
	} else if _, ok := a[ws.ID]; ok {
		t.Errorf("acknowledged problems must stop counting: %+v", a[ws.ID])
	}
	// The log itself is intact, so the operator can still retry or resend.
	rows, err := s.ListEmailDeliveries(ctx, ws.ID, 10)
	if err != nil {
		t.Fatalf("list after ack: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("acknowledging must not delete deliveries: %d", len(rows))
	}

	// A send that fails after the acknowledgement is new work.
	var later time.Time
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO email_deliveries (workspace_id, kind, lead_id, to_email, subject, body, status)
		 VALUES ($1, 'lead_reply', $2, 'sam@example.com', 'Re: enquiry', 'body', 'failed')
		 RETURNING updated_at`, ws.ID, lead.ID).Scan(&later); err != nil {
		t.Fatalf("seed later failure: %v", err)
	}
	if !later.After(at) {
		t.Fatalf("the later failure must be recorded after the ack: %v vs %v", later, at)
	}
	if a, err := s.EmailDeliveryAttention(ctx); err != nil {
		t.Fatalf("attention after new failure: %v", err)
	} else if a[ws.ID].Failed != 1 || a[ws.ID].Bounced != 0 {
		t.Errorf("only the new failure must count: %+v", a[ws.ID])
	}

	// Acknowledging again clears it, and is idempotent.
	if _, err := s.AcknowledgeEmailDeliveryAttention(ctx, ws.ID); err != nil {
		t.Fatalf("second acknowledge: %v", err)
	}
	if a, err := s.EmailDeliveryAttention(ctx); err != nil {
		t.Fatalf("attention after second ack: %v", err)
	} else if _, ok := a[ws.ID]; ok {
		t.Errorf("the new failure must clear on the second ack: %+v", a[ws.ID])
	}
}

func TestAcknowledgedProblemsAreTheOnesTheAckCovered(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()
	ws := uniqueWorkspace(t, s, "email-ack-list")
	lead := emailTestLead(t, s, ws.ID)
	other := emailTestLead(t, s, ws.ID)

	// Never acknowledged: nothing to list, and no timestamp to claim.
	if at, err := s.EmailDeliveryAcknowledgement(ctx, ws.ID); err != nil || at != nil {
		t.Fatalf("unacknowledged workspace: at=%v err=%v", at, err)
	}
	if rows, err := s.AcknowledgedEmailDeliveryProblems(ctx, ws.ID, 10); err != nil || len(rows) != 0 {
		t.Fatalf("unacknowledged workspace must list nothing: %d %v", len(rows), err)
	}

	var failedID, bouncedID string
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO email_deliveries
			(workspace_id, kind, lead_id, to_email, subject, body, status, last_error, updated_at)
		 VALUES ($1, 'lead_reply', $2, 'jane@example.com', 'Re: enquiry', 'body', 'failed',
			'provider rejected the key', now() - interval '2 minutes')
		 RETURNING id`, ws.ID, lead.ID).Scan(&failedID); err != nil {
		t.Fatalf("seed failed send: %v", err)
	}
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO email_deliveries
			(workspace_id, kind, lead_id, to_email, subject, body, status,
			 provider_message_id, sent_at, delivery_state, delivery_state_at, delivery_detail)
		 VALUES ($1, 'lead_reply', $2, 'sam@example.com', 'Re: quote', 'body', 'sent',
			'msg_ack_list', now() - interval '6 minutes', 'bounced', now() - interval '4 minutes',
			'mailbox full')
		 RETURNING id`, ws.ID, other.ID).Scan(&bouncedID); err != nil {
		t.Fatalf("seed bounce: %v", err)
	}
	ackedAt, err := s.AcknowledgeEmailDeliveryAttention(ctx, ws.ID)
	if err != nil {
		t.Fatalf("acknowledge: %v", err)
	}
	if at, err := s.EmailDeliveryAcknowledgement(ctx, ws.ID); err != nil || at == nil || !at.Equal(ackedAt) {
		t.Fatalf("acknowledgement lookup: at=%v want %v err=%v", at, ackedAt, err)
	}

	rows, err := s.AcknowledgedEmailDeliveryProblems(ctx, ws.ID, 10)
	if err != nil {
		t.Fatalf("acknowledged problems: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("both dismissed problems must be listed: %d", len(rows))
	}
	// Newest problem first: the bounce was reported after the failure.
	if rows[0].ID != bouncedID || rows[1].ID != failedID {
		t.Errorf("dismissed problems must read newest first: %s, %s", rows[0].ID, rows[1].ID)
	}
	if rows[1].LastError == "" || rows[0].DeliveryDetail != "mailbox full" {
		t.Errorf("the reason each problem needed attention must survive: %+v", rows)
	}

	// A failure after the acknowledgement is the badge's business, not this list.
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO email_deliveries (workspace_id, kind, lead_id, to_email, subject, body, status)
		 VALUES ($1, 'lead_reply', $2, 'new@example.com', 'Re: enquiry', 'body', 'failed')`,
		ws.ID, lead.ID); err != nil {
		t.Fatalf("seed later failure: %v", err)
	}
	if rows, err := s.AcknowledgedEmailDeliveryProblems(ctx, ws.ID, 10); err != nil {
		t.Fatalf("acknowledged problems after new failure: %v", err)
	} else if len(rows) != 2 {
		t.Errorf("a problem after the ack must not be called dismissed: %d", len(rows))
	}
	if rows, err := s.AcknowledgedEmailDeliveryProblems(ctx, ws.ID, 1); err != nil || len(rows) != 1 {
		t.Errorf("limit must apply: %d %v", len(rows), err)
	}

	// Acknowledging again moves the boundary: the newer failure becomes dismissed.
	if _, err := s.AcknowledgeEmailDeliveryAttention(ctx, ws.ID); err != nil {
		t.Fatalf("second acknowledge: %v", err)
	}
	if rows, err := s.AcknowledgedEmailDeliveryProblems(ctx, ws.ID, 10); err != nil || len(rows) != 3 {
		t.Errorf("the newer failure must join the dismissed list: %d %v", len(rows), err)
	}
}
