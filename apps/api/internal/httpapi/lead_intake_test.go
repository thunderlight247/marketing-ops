package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"marketingops/api/internal/store"
)

type intakeFake struct {
	*fakeWorkspaceStore
	connections map[string]store.LeadIntakeConnection
	receipts    map[string][]byte
	calls       int
}

func (f *intakeFake) CreateLeadIntakeConnection(_ context.Context, ws, name string, hash []byte) (store.LeadIntakeConnection, error) {
	c := store.LeadIntakeConnection{ID: "22222222-2222-2222-2222-222222222222", WorkspaceID: ws, Name: name, SecretHash: hash, CreatedAt: time.Now()}
	f.connections[c.ID] = c
	return c, nil
}
func (f *intakeFake) ListLeadIntakeConnections(_ context.Context, ws string) ([]store.LeadIntakeConnection, error) {
	var out []store.LeadIntakeConnection
	for _, c := range f.connections {
		if c.WorkspaceID == ws {
			out = append(out, c)
		}
	}
	return out, nil
}
func (f *intakeFake) GetLeadIntakeConnection(_ context.Context, ws, id string) (store.LeadIntakeConnection, error) {
	c, ok := f.connections[id]
	if !ok || c.WorkspaceID != ws {
		return c, store.ErrNotFound
	}
	return c, nil
}
func (f *intakeFake) DeleteLeadIntakeConnection(_ context.Context, ws, id string) error {
	if _, err := f.GetLeadIntakeConnection(context.Background(), ws, id); err != nil {
		return err
	}
	delete(f.connections, id)
	return nil
}
func (f *intakeFake) RecordLeadIntake(_ context.Context, c store.LeadIntakeConnection, id string, hash []byte, l store.Lead) (string, bool, error) {
	key := c.ID + id
	if prev, ok := f.receipts[key]; ok {
		if string(prev) != string(hash) {
			return "", false, store.ErrSubmissionConflict
		}
		return "lead-1", true, nil
	}
	f.receipts[key] = hash
	f.calls++
	if l.ContactEmail == nil {
		return "", false, errors.New("missing email")
	}
	return "lead-1", false, nil
}

func TestLeadIntakeAuthenticationAndLifecycle(t *testing.T) {
	const ws = "11111111-1111-1111-1111-111111111111"
	const other = "33333333-3333-3333-3333-333333333333"
	f := &intakeFake{fakeWorkspaceStore: &fakeWorkspaceStore{byID: map[string]store.Workspace{ws: {ID: ws}, other: {ID: other}}}, connections: map[string]store.LeadIntakeConnection{}, receipts: map[string][]byte{}}
	h := NewRouter(Deps{Logger: testLogger, Workspaces: f, APIKey: "operator-secret"})
	request := func(method, path, auth, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if auth != "" {
			r.Header.Set("Authorization", "Bearer "+auth)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	base := "/api/v1/workspaces/" + ws + "/lead-connections"
	if w := request("POST", base, "", `{"name":"Website"}`); w.Code != 401 {
		t.Fatalf("management without operator key: %d", w.Code)
	}
	created := request("POST", base, "operator-secret", `{"name":"Website"}`)
	if created.Code != 201 {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	var data struct {
		Secret     string `json:"secret"`
		Connection struct {
			ID string `json:"id"`
		} `json:"connection"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &data); err != nil || len(data.Secret) != 64 {
		t.Fatalf("bad creation response: %v", err)
	}
	hash := sha256.Sum256([]byte(data.Secret))
	if string(f.connections[data.Connection.ID].SecretHash) != string(hash[:]) {
		t.Fatal("stored hash mismatch")
	}
	listed := request("GET", base, "operator-secret", "")
	if strings.Contains(listed.Body.String(), data.Secret) || strings.Contains(listed.Body.String(), "secret") {
		t.Fatal("secret leaked by list")
	}
	verify := base + "/" + data.Connection.ID + "/verify"
	for _, tc := range []struct {
		path, secret string
		status       int
	}{
		{verify, "", 401}, {verify, "operator-secret", 401},
		{"/api/v1/workspaces/" + other + "/lead-connections/" + data.Connection.ID + "/verify", data.Secret, 401},
		{verify, data.Secret, 200},
	} {
		w := request("POST", tc.path, tc.secret, "")
		if w.Code != tc.status {
			t.Fatalf("verify status = %d, want %d", w.Code, tc.status)
		}
	}
	if f.calls != 0 {
		t.Fatal("verification must not create a lead")
	}
	submit := base + "/" + data.Connection.ID + "/submissions"
	payload := `{"submissionId":"abc-1","contactEmail":"person@example.com","message":"Hello"}`
	for _, tc := range []struct {
		name, path, auth, body string
		want                   int
	}{
		{"no secret", submit, "", payload, 401}, {"operator key not accepted", submit, "operator-secret", payload, 401}, {"wrong workspace", "/api/v1/workspaces/" + other + "/lead-connections/" + data.Connection.ID + "/submissions", data.Secret, payload, 401},
		{"bad email", submit, data.Secret, `{"submissionId":"bad","contactEmail":"invalid"}`, 400},
		{"success", submit, data.Secret, payload, 201}, {"retry", submit, data.Secret, payload, 200}, {"conflicting retry", submit, data.Secret, `{"submissionId":"abc-1","contactEmail":"another@example.com"}`, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := request("POST", tc.path, tc.auth, tc.body)
			if w.Code != tc.want {
				t.Fatalf("got %d want %d body %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
	if f.calls != 1 {
		t.Fatalf("created %d leads", f.calls)
	}
	if w := request("DELETE", base+"/"+data.Connection.ID, "operator-secret", ""); w.Code != 204 {
		t.Fatalf("revoke: %d", w.Code)
	}
	if w := request("POST", submit, data.Secret, payload); w.Code != 401 {
		t.Fatalf("revoked connection: %d", w.Code)
	}
	if w := request("POST", verify, data.Secret, ""); w.Code != 401 {
		t.Fatalf("revoked verification: %d", w.Code)
	}
}
