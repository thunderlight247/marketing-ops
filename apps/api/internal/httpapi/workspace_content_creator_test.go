package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"marketingops/api/internal/aiprovider"
	"marketingops/api/internal/store"
)

const creatorReply = `{"title":"Winter care","slug":"winter-care","excerpt":"Prepare for winter.","body":"## Winter care\n\nReview your maintenance plan.","callToAction":"Contact us.","keyPoints":["Maintenance"]}`

func TestContentCreatorDraftWorkflow(t *testing.T) {
	ws := workspaceDeps(nil, nil)
	runner := &fakeAgentRunner{text: creatorReply}
	h := NewRouter(Deps{Logger: testLogger, Workspaces: ws, AgentRunner: runner})
	rec := doJSON(t, h, http.MethodPost, agentRunPath(agentWsID, "content_creator"), `{"kind":"social_post","focus":"Winter maintenance"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var result struct {
		Item    contentItemJSON    `json:"item"`
		Version contentVersionJSON `json:"version"`
		AgentID string             `json:"agentId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Item.Status != "draft" || result.Item.Kind != "social_post" || result.Item.Title != "Winter care" || result.AgentID != "content_creator" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Version.VersionNumber != 1 || result.Version.Body == nil || !strings.Contains(*result.Version.Body, "maintenance") {
		t.Fatalf("missing version: %+v", result.Version)
	}
	if len(runner.calls) != 1 || !strings.Contains(runner.calls[0].User, "social_post") || !strings.Contains(runner.calls[0].User, "Winter maintenance") {
		t.Fatalf("unexpected prompts: %+v", runner.calls)
	}
	if len(ws.savedDrafts) != 1 || ws.savedDrafts[0].WorkspaceID != agentWsID || ws.savedDrafts[0].Input.Metadata["source"] != "agent" {
		t.Fatalf("unexpected saves: %+v", ws.savedDrafts)
	}
	detail := doJSON(t, h, http.MethodGet, "/api/v1/workspaces/"+agentWsID+"/content/"+result.Item.ID, "")
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "Winter care") {
		t.Fatalf("draft not readable: %s", detail.Body.String())
	}
	if len(ws.auditEvents) != 1 || ws.auditEvents[0].Action != "agent.action.file_draft.completed" {
		t.Fatalf("unexpected audit: %+v", ws.auditEvents)
	}
}

func TestContentCreatorRequestFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"bad JSON", `{`, 400},
		{"bad kind", `{"kind":"podcast"}`, 400},
		{"long focus", `{"focus":"` + strings.Repeat("x", 2001) + `"}`, 400},
		{"oversize", strings.Repeat("x", maxAgentPayloadBytes+1), 413},
		{"missing item", `{"contentItemId":"missing"}`, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws := workspaceDeps(nil, nil)
			runner := &fakeAgentRunner{text: creatorReply}
			h := NewRouter(Deps{Logger: testLogger, Workspaces: ws, AgentRunner: runner})
			rec := doJSON(t, h, http.MethodPost, agentRunPath(agentWsID, "content_creator"), tc.body)
			if rec.Code != tc.status {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if len(runner.calls) != 0 || len(ws.insertedContent) != 0 {
				t.Fatal("invalid request called model or created content")
			}
		})
	}
}

func TestContentCreatorFailureDoesNotCreatePlaceholder(t *testing.T) {
	for _, runner := range []*fakeAgentRunner{
		{text: `{"title":"missing body"}`},
		{fail: errors.New("secret upstream payload")},
		{fail: &aiprovider.ProviderError{Kind: aiprovider.FailTimeout}},
	} {
		ws := workspaceDeps(nil, nil)
		h := NewRouter(Deps{Logger: testLogger, Workspaces: ws, AgentRunner: runner})
		rec := doJSON(t, h, http.MethodPost, agentRunPath(agentWsID, "content_creator"), `{}`)
		if rec.Code != 502 || strings.Contains(rec.Body.String(), "secret upstream") {
			t.Fatalf("unsafe response: %d %s", rec.Code, rec.Body.String())
		}
		if len(ws.insertedContent) != 0 || len(ws.savedDrafts) != 0 {
			t.Fatal("failed model run wrote content")
		}
		if len(ws.auditEvents) != 1 || ws.auditEvents[0].Action != "agent.run.failed" {
			t.Fatal("missing failure audit")
		}
	}
}

func TestContentCreatorExistingItemBoundaries(t *testing.T) {
	for _, tc := range []struct {
		status, workspace string
		code              int
	}{
		{"draft", agentWsID, 201}, {"approved", agentWsID, 201}, {"rejected", agentWsID, 201},
		{"review", agentWsID, 409}, {"queued", agentWsID, 409}, {"published", agentWsID, 409},
		{"draft", "other-workspace", 404},
	} {
		t.Run(tc.status+tc.workspace, func(t *testing.T) {
			ws := workspaceDeps(nil, nil)
			ws.contentItemsByID = map[string]store.ContentItem{"item": {ID: "item", WorkspaceID: tc.workspace, Kind: "snippet", Status: tc.status, Title: "Before"}}
			runner := &fakeAgentRunner{text: creatorReply}
			h := NewRouter(Deps{Logger: testLogger, Workspaces: ws, AgentRunner: runner})
			rec := doJSON(t, h, http.MethodPost, agentRunPath(agentWsID, "content_creator"), `{"contentItemId":"item"}`)
			if rec.Code != tc.code {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if tc.code == 201 {
				if !strings.Contains(runner.calls[0].User, "Content kind for this piece: snippet") || ws.contentItemsByID["item"].Status != "draft" {
					t.Fatal("existing kind or draft reset lost")
				}
			} else if len(runner.calls) != 0 {
				t.Fatal("model called for inaccessible or frozen content")
			}
			if len(ws.insertedContent) != 0 {
				t.Fatal("existing-item run inserted content")
			}
		})
	}
}

func TestContentCreatorUnavailable(t *testing.T) {
	for _, deps := range []Deps{{Logger: testLogger}, {Logger: testLogger, Workspaces: workspaceDeps(nil, nil)}} {
		rec := doJSON(t, NewRouter(deps), http.MethodPost, agentRunPath(agentWsID, "content_creator"), `{}`)
		if rec.Code != 501 {
			t.Fatalf("expected 501, got %d", rec.Code)
		}
	}
}

func TestContentCreatorOfflineProvider(t *testing.T) {
	ws := workspaceDeps(nil, nil)
	h := NewRouter(Deps{Logger: testLogger, Workspaces: ws, AgentRunner: aiprovider.StubProvider{}})
	rec := doJSON(t, h, http.MethodPost, agentRunPath(agentWsID, "content_creator"), `{}`)
	if rec.Code != 201 || !strings.Contains(rec.Body.String(), "[Stub]") {
		t.Fatalf("offline path failed: %d %s", rec.Code, rec.Body.String())
	}
}
