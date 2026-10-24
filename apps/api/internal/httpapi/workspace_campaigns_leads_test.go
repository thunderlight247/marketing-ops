package httpapi

// Router-level tests for the Milestone 11 campaign and lead endpoints.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"marketingops/api/internal/store"
)

const campaignsTestWsID = "11111111-1111-1111-1111-111111111111"

func TestWorkspaceCampaignEndpoints(t *testing.T) {
	wsID := campaignsTestWsID
	workspace := store.Workspace{ID: wsID, Slug: "demo", Name: "Demo"}

	t.Run("create validates input and audits", func(t *testing.T) {
		fake := &fakeWorkspaceStore{byID: map[string]store.Workspace{wsID: workspace}}
		h := NewRouter(Deps{Logger: testLogger, Workspaces: fake})

		rec := doJSON(t, h, "POST", "/api/v1/workspaces/"+wsID+"/campaigns",
			`{"name":"Spring newsletter","status":"active","utmCampaign":" spring-newsletter ","notes":" push "}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
		}
		var created campaignJSON
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if created.Name != "Spring newsletter" || created.Status != "active" || created.UtmCampaign != "spring-newsletter" {
			t.Errorf("created campaign mismatch: %+v", created)
		}
		if len(fake.insertedCampaigns) != 1 {
			t.Fatalf("InsertCampaign not called once: %d", len(fake.insertedCampaigns))
		}
		if got := fake.insertedCampaigns[0].UtmCampaign; got != "spring-newsletter" {
			t.Errorf("utmCampaign must be trimmed: %q", got)
		}
		if got := fake.insertedCampaigns[0].Notes; got != "push" {
			t.Errorf("notes must be trimmed: %q", got)
		}
		if len(fake.auditEvents) != 1 || fake.auditEvents[0].Action != "campaign.created" {
			t.Errorf("audit row missing: %+v", fake.auditEvents)
		}
	})

	t.Run("create rejects bad values", func(t *testing.T) {
		fake := &fakeWorkspaceStore{byID: map[string]store.Workspace{wsID: workspace}}
		h := NewRouter(Deps{Logger: testLogger, Workspaces: fake})

		rec := doJSON(t, h, "POST", "/api/v1/workspaces/"+wsID+"/campaigns", `{"name":"x","status":"viral"}`)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "status must be") {
			t.Errorf("bad status must be 400 with reason, got %d body=%s", rec.Code, rec.Body.String())
		}
		rec = doJSON(t, h, "POST", "/api/v1/workspaces/"+wsID+"/campaigns", `{}`)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "name is required") {
			t.Errorf("missing name must be 400, got %d body=%s", rec.Code, rec.Body.String())
		}
		if len(fake.insertedCampaigns) != 0 {
			t.Errorf("no campaign may be stored on invalid input")
		}
	})

	t.Run("conflict maps to 409 without leaking internals", func(t *testing.T) {
		fake := &fakeWorkspaceStore{byID: map[string]store.Workspace{wsID: workspace}, insertCampaignErr: store.ErrSlugTaken}
		h := NewRouter(Deps{Logger: testLogger, Workspaces: fake})
		rec := doJSON(t, h, "POST", "/api/v1/workspaces/"+wsID+"/campaigns", `{"name":"dup"}`)
		if rec.Code != http.StatusConflict {
			t.Fatalf("ErrSlugTaken must be 409, got %d", rec.Code)
		}
		if strings.Contains(rec.Body.String(), "ErrSlugTaken") {
			t.Errorf("internal sentinel must not leak: %s", rec.Body.String())
		}
	})

	t.Run("patch updates editable fields and audits", func(t *testing.T) {
		existing := store.Campaign{ID: "c-1", WorkspaceID: wsID, Name: "Old", Status: "planned", UtmCampaign: "old-utm"}
		fake := &fakeWorkspaceStore{
			byID:          map[string]store.Workspace{wsID: workspace},
			campaignsByID: map[string]store.Campaign{"c-1": existing},
		}
		h := NewRouter(Deps{Logger: testLogger, Workspaces: fake})

		rec := doJSON(t, h, "PATCH", "/api/v1/workspaces/"+wsID+"/campaigns/c-1",
			`{"status":"completed","notes":"wrapped"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
		}
		saved := fake.updatedCampaigns[0]
		if saved.Status != "completed" || saved.Notes != "wrapped" {
			t.Errorf("patch not applied: %+v", saved)
		}
		if saved.Name != "Old" {
			t.Errorf("omitted name must stay: %+v", saved)
		}
		if len(fake.auditEvents) == 0 || fake.auditEvents[len(fake.auditEvents)-1].Action != "campaign.updated" {
			t.Errorf("audit row missing: %+v", fake.auditEvents)
		}

		// Other-workspace or missing campaign is 404.
		rec = doJSON(t, h, "PATCH", "/api/v1/workspaces/"+wsID+"/campaigns/nope", `{"notes":"x"}`)
		if rec.Code != http.StatusNotFound {
			t.Errorf("unknown campaign must be 404, got %d", rec.Code)
		}
	})

	t.Run("list is workspace-scoped", func(t *testing.T) {
		listed := []store.Campaign{{ID: "c-2", WorkspaceID: wsID, Name: "Mine", Status: "active"}}
		fake := &fakeWorkspaceStore{byID: map[string]store.Workspace{wsID: workspace}, campaignsList: listed}
		h := NewRouter(Deps{Logger: testLogger, Workspaces: fake})

		rec := doJSON(t, h, "GET", "/api/v1/workspaces/"+wsID+"/campaigns", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		var body struct {
			Campaigns []campaignJSON `json:"campaigns"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(body.Campaigns) != 1 || body.Campaigns[0].Name != "Mine" {
			t.Errorf("unexpected list: %+v", body.Campaigns)
		}

		// Unknown workspace is 404.
		rec = doJSON(t, h, "GET", "/api/v1/workspaces/00000000-0000-0000-0000-000000000000/campaigns", "")
		if rec.Code != http.StatusNotFound {
			t.Errorf("unknown workspace must be 404, got %d", rec.Code)
		}
	})
}

func TestWorkspaceLeadEndpoints(t *testing.T) {
	wsID := campaignsTestWsID
	workspace := store.Workspace{ID: wsID, Slug: "demo", Name: "Demo"}

	t.Run("create records the source honestly and audits", func(t *testing.T) {
		fake := &fakeWorkspaceStore{byID: map[string]store.Workspace{wsID: workspace}}
		h := NewRouter(Deps{Logger: testLogger, Workspaces: fake})

		rec := doJSON(t, h, "POST", "/api/v1/workspaces/"+wsID+"/leads",
			`{"sourceKind":"form_api","sourceDetail":"contact form on nutritionguide.xyz","contactName":" Jane ","contactEmail":"jane@example.test","notes":"pricing question","campaignId":"c-1"}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
		}
		var created leadJSON
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if created.SourceKind != "form_api" || created.ContactName == nil || *created.ContactName != " Jane " {
			t.Errorf("created lead mismatch: %+v", created)
		}
		if len(fake.insertedLeads) != 1 {
			t.Fatalf("InsertLead not called once: %d", len(fake.insertedLeads))
		}
		inserted := fake.insertedLeads[0]
		if inserted.SourceDetail != "contact form on nutritionguide.xyz" {
			t.Errorf("source detail wrong: %q", inserted.SourceDetail)
		}
		if len(fake.auditEvents) != 1 || fake.auditEvents[0].Action != "lead.created" {
			t.Errorf("audit row missing: %+v", fake.auditEvents)
		}
	})

	t.Run("create defaults an omitted source to unknown", func(t *testing.T) {
		fake := &fakeWorkspaceStore{byID: map[string]store.Workspace{wsID: workspace}}
		h := NewRouter(Deps{Logger: testLogger, Workspaces: fake})

		rec := doJSON(t, h, "POST", "/api/v1/workspaces/"+wsID+"/leads", `{}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
		}
		if fake.insertedLeads[0].SourceKind != "unknown" {
			t.Errorf("omitted source must be 'unknown', got %q", fake.insertedLeads[0].SourceKind)
		}
	})

	t.Run("create rejects an unknown sourceKind and an outside campaign", func(t *testing.T) {
		fake := &fakeWorkspaceStore{
			byID:          map[string]store.Workspace{wsID: workspace},
			campaignsByID: map[string]store.Campaign{"c-1": {ID: "c-1", WorkspaceID: "other-ws", Name: "Other"}},
		}
		h := NewRouter(Deps{Logger: testLogger, Workspaces: fake})

		rec := doJSON(t, h, "POST", "/api/v1/workspaces/"+wsID+"/leads", `{"sourceKind":"scraped"}`)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "sourceKind") {
			t.Errorf("bad sourceKind must be 400, got %d body=%s", rec.Code, rec.Body.String())
		}
		fake.insertLeadErr = store.ErrNotFound
		rec = doJSON(t, h, "POST", "/api/v1/workspaces/"+wsID+"/leads", `{"sourceKind":"manual"}`)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "campaignId") {
			t.Errorf("a scoped campaign reference must be rejected as 400, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("patch keeps the source immutable and applies updates", func(t *testing.T) {
		existing := store.Lead{ID: "l-1", WorkspaceID: wsID, SourceKind: "manual", SourceDetail: "phone", Status: "new", Notes: "warm"}
		fake := &fakeWorkspaceStore{
			byID:      map[string]store.Workspace{wsID: workspace},
			leadsByID: map[string]store.Lead{"l-1": existing},
		}
		h := NewRouter(Deps{Logger: testLogger, Workspaces: fake})

		rec := doJSON(t, h, "PATCH", "/api/v1/workspaces/"+wsID+"/leads/l-1",
			`{"status":"contacted","notes":"called","clearFollowUp":true}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
		}
		if len(fake.updatedLeads) != 1 {
			t.Fatalf("UpdateLead not called once: %d", len(fake.updatedLeads))
		}
		apply := fake.updatedLeads[0]
		if apply.leadID != "l-1" || apply.workspaceID != wsID {
			t.Errorf("update must be workspace-bounded: %+v", apply)
		}
		up := apply.update
		if up.Status != "contacted" || up.KeepNotes {
			t.Errorf("update not carried through: %+v", up)
		}
		if up.Notes != "called" {
			t.Errorf("notes not applied: %q", up.Notes)
		}

		// Omitted notes must keep the current value (KeepNotes).
		fake.updatedLeads = nil
		_ = doJSON(t, h, "PATCH", "/api/v1/workspaces/"+wsID+"/leads/l-1", `{"status":"qualified"}`)
		if !fake.updatedLeads[0].update.KeepNotes {
			t.Errorf("omitted notes must keep the current value")
		}
	})

	t.Run("patch rejects invalid status and unknown leads", func(t *testing.T) {
		fake := &fakeWorkspaceStore{
			byID:      map[string]store.Workspace{wsID: workspace},
			leadsByID: map[string]store.Lead{"l-1": {ID: "l-1", WorkspaceID: wsID, SourceKind: "manual", Status: "new"}},
		}
		h := NewRouter(Deps{Logger: testLogger, Workspaces: fake})

		rec := doJSON(t, h, "PATCH", "/api/v1/workspaces/"+wsID+"/leads/l-1", `{"status":"pending"}`)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "status") {
			t.Errorf("invalid status must be 400, got %d body=%s", rec.Code, rec.Body.String())
		}
		rec = doJSON(t, h, "PATCH", "/api/v1/workspaces/"+wsID+"/leads/other", `{"status":"won"}`)
		if rec.Code != http.StatusNotFound {
			t.Errorf("unknown lead must be 404, got %d", rec.Code)
		}
	})

	t.Run("list is workspace-scoped with follow-up ordering hint", func(t *testing.T) {
		follow := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
		listed := []store.Lead{
			{ID: "l-2", WorkspaceID: wsID, SourceKind: "unknown", Status: "contacted", FollowUpAt: &follow},
			{ID: "l-3", WorkspaceID: wsID, SourceKind: "manual", Status: "new"},
		}
		fake := &fakeWorkspaceStore{byID: map[string]store.Workspace{wsID: workspace}, leadsList: listed}
		h := NewRouter(Deps{Logger: testLogger, Workspaces: fake})

		rec := doJSON(t, h, "GET", "/api/v1/workspaces/"+wsID+"/leads", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
		}
		var body struct {
			Leads []leadJSON `json:"leads"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(body.Leads) != 2 || body.Leads[0].ID != "l-2" {
			t.Errorf("unexpected list: %+v", body.Leads)
		}

		// Unknown workspace is 404.
		rec = doJSON(t, h, "GET", "/api/v1/workspaces/00000000-0000-0000-0000-000000000000/leads", "")
		if rec.Code != http.StatusNotFound {
			t.Errorf("unknown workspace must be 404, got %d", rec.Code)
		}
	})
}
