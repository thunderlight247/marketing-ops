package store

// Integration test for the agent-drafted lead reply column set (migration
// 0009). It runs only when a database is configured (see TestMain).

import (
	"context"
	"errors"
	"testing"
)

func TestLeadReplyDraftLifecycle(t *testing.T) {
	s := requireStore(t)
	ctx := context.Background()
	ws := uniqueWorkspace(t, s, "lead-reply")
	other := uniqueWorkspace(t, s, "lead-reply-other")

	name, email, message := "Jane Doe", "jane@example.test", "Do you service the north side?"
	lead, err := s.InsertLead(ctx, Lead{
		WorkspaceID: ws.ID, SourceKind: "form_api", SourceDetail: "contact form",
		ContactName: &name, ContactEmail: &email, Message: &message,
		Notes: "operator notes stay put",
	})
	if err != nil {
		t.Fatalf("InsertLead: %v", err)
	}
	if lead.ReplyDraftSubject != nil || lead.ReplyDraftBody != nil || lead.ReplyDraftedAt != nil {
		t.Fatalf("a new lead must have no reply draft: %+v", lead)
	}

	// Cross-workspace writes are not possible.
	if _, err := s.SaveLeadReplyDraft(ctx, other.ID, lead.ID, "s", "b"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-workspace reply draft must be ErrNotFound, got %v", err)
	}

	saved, err := s.SaveLeadReplyDraft(ctx, ws.ID, lead.ID,
		"Re: your furnace check", "Hi Jane, thanks for the note about the north side.")
	if err != nil {
		t.Fatalf("SaveLeadReplyDraft: %v", err)
	}
	if saved.ReplyDraftSubject == nil || *saved.ReplyDraftSubject != "Re: your furnace check" ||
		saved.ReplyDraftBody == nil || *saved.ReplyDraftBody == "" ||
		saved.ReplyDraftedAt == nil || saved.ReplyDraftedAt.IsZero() {
		t.Fatalf("stored reply draft mismatch: %+v", saved)
	}
	// A draft is not a contact: status, source, attribution, notes are intact.
	if saved.Status != "new" || saved.SourceKind != "form_api" || saved.SourceDetail != "contact form" ||
		saved.Notes != "operator notes stay put" || saved.ContactEmail == nil || *saved.ContactEmail != email {
		t.Fatalf("saving a reply must change nothing else: %+v", saved)
	}

	// Blank input is refused before the database constraint.
	if _, err := s.SaveLeadReplyDraft(ctx, ws.ID, lead.ID, "  ", "b"); !errors.Is(err, ErrNameRequired) {
		t.Fatalf("blank subject must be ErrNameRequired, got %v", err)
	}
	if _, err := s.SaveLeadReplyDraft(ctx, ws.ID, lead.ID, "s", "  "); !errors.Is(err, ErrNameRequired) {
		t.Fatalf("blank body must be ErrNameRequired, got %v", err)
	}

	// A second run replaces the draft (newest only) and read paths agree.
	replaced, err := s.SaveLeadReplyDraft(ctx, ws.ID, lead.ID, "Re: second draft", "A newer draft.")
	if err != nil {
		t.Fatalf("SaveLeadReplyDraft replace: %v", err)
	}
	if *replaced.ReplyDraftSubject != "Re: second draft" {
		t.Fatalf("the newest draft must win: %+v", replaced)
	}
	listed, err := s.ListLeads(ctx, ws.ID)
	if err != nil {
		t.Fatalf("ListLeads: %v", err)
	}
	found := false
	for _, l := range listed {
		if l.ID != lead.ID {
			continue
		}
		found = true
		if l.ReplyDraftBody == nil || *l.ReplyDraftBody != "A newer draft." {
			t.Fatalf("ListLeads lost the reply draft: %+v", l)
		}
	}
	if !found {
		t.Fatal("ListLeads did not return the lead")
	}
	read, err := s.GetLeadInWorkspace(ctx, ws.ID, lead.ID)
	if err != nil || read.ReplyDraftSubject == nil || *read.ReplyDraftSubject != "Re: second draft" {
		t.Fatalf("GetLeadInWorkspace lost the reply draft: %+v err %v", read, err)
	}
}
