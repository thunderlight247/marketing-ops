package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"regexp"
	"strings"

	"marketingops/api/internal/store"
)

type leadIntakeStore interface {
	CreateLeadIntakeConnection(context.Context, string, string, []byte) (store.LeadIntakeConnection, error)
	ListLeadIntakeConnections(context.Context, string) ([]store.LeadIntakeConnection, error)
	GetLeadIntakeConnection(context.Context, string, string) (store.LeadIntakeConnection, error)
	DeleteLeadIntakeConnection(context.Context, string, string) error
	RecordLeadIntake(context.Context, store.LeadIntakeConnection, string, []byte, store.Lead) (string, bool, error)
}

var intakeUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var submissionIDPattern = regexp.MustCompile(`^[a-zA-Z0-9._:-]{1,128}$`)

// Only the two exact POST routes bypass operator auth; both handlers require
// the separate per-connection secret, even in development mode.
func isLeadIntakeRequest(r *http.Request) bool {
	parts := strings.Split(r.URL.Path, "/")
	return r.Method == http.MethodPost && len(parts) == 8 && parts[1] == "api" && parts[2] == "v1" && parts[3] == "workspaces" && parts[5] == "lead-connections" && (parts[7] == "submissions" || parts[7] == "verify")
}

func intakeConnectionJSON(c store.LeadIntakeConnection) map[string]any {
	return map[string]any{"id": c.ID, "workspaceId": c.WorkspaceID, "name": c.Name, "createdAt": c.CreatedAt.UTC()}
}

func manageLeadIntake(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}
		s, ok := ws.(leadIntakeStore)
		if !ok {
			writeJSONError(w, http.StatusNotImplemented, "lead intake unavailable")
			return
		}
		switch r.Method {
		case http.MethodGet:
			connections, err := s.ListLeadIntakeConnections(r.Context(), workspaceID)
			if err != nil {
				writeJSONError(w, 500, "could not list lead connections")
				return
			}
			out := make([]map[string]any, 0, len(connections))
			for _, c := range connections {
				out = append(out, intakeConnectionJSON(c))
			}
			writeJSON(w, 200, map[string]any{"connections": out})
		case http.MethodPost:
			body, problems := readJSONBody(w, r, 4096)
			if problems != nil {
				return
			}
			var input struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(body, &input) != nil || strings.TrimSpace(input.Name) == "" || len([]rune(strings.TrimSpace(input.Name))) > 120 {
				writeJSONError(w, 400, "name is required and must be at most 120 characters")
				return
			}
			var raw [32]byte
			if _, err := rand.Read(raw[:]); err != nil {
				writeJSONError(w, 500, "could not generate connection secret")
				return
			}
			secret := hex.EncodeToString(raw[:])
			hash := sha256.Sum256([]byte(secret))
			c, err := s.CreateLeadIntakeConnection(r.Context(), workspaceID, strings.TrimSpace(input.Name), hash[:])
			if errors.Is(err, store.ErrSlugTaken) {
				writeJSONError(w, 409, "a connection with that name already exists")
				return
			}
			if err != nil {
				writeJSONError(w, 500, "could not create lead connection")
				return
			}
			audit(r.Context(), ws, store.AuditEvent{WorkspaceID: &workspaceID, Action: "lead_intake.created", EntityType: "lead_intake_connection", EntityID: &c.ID})
			writeJSON(w, 201, map[string]any{"connection": intakeConnectionJSON(c), "secret": secret})
		case http.MethodDelete:
			id := r.PathValue("connectionId")
			if !intakeUUID.MatchString(id) {
				writeJSONError(w, 404, "no such connection")
				return
			}
			if err := s.DeleteLeadIntakeConnection(r.Context(), workspaceID, id); errors.Is(err, store.ErrNotFound) {
				writeJSONError(w, 404, "no such connection")
				return
			} else if err != nil {
				writeJSONError(w, 500, "could not delete lead connection")
				return
			}
			audit(r.Context(), ws, store.AuditEvent{WorkspaceID: &workspaceID, Action: "lead_intake.deleted", EntityType: "lead_intake_connection", EntityID: &id})
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func authenticateLeadIntake(w http.ResponseWriter, r *http.Request, ws WorkspaceStore) (leadIntakeStore, store.LeadIntakeConnection, bool) {
	s, ok := ws.(leadIntakeStore)
	if !ok {
		writeJSONError(w, http.StatusServiceUnavailable, "lead intake unavailable")
		return nil, store.LeadIntakeConnection{}, false
	}
	workspaceID, id := r.PathValue("id"), r.PathValue("connectionId")
	if !intakeUUID.MatchString(workspaceID) || !intakeUUID.MatchString(id) {
		writeJSONError(w, http.StatusUnauthorized, "invalid lead connection credentials")
		return nil, store.LeadIntakeConnection{}, false
	}
	c, err := s.GetLeadIntakeConnection(r.Context(), workspaceID, id)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusServiceUnavailable, "lead intake unavailable")
		return nil, store.LeadIntakeConnection{}, false
	}
	token := extractBearerToken(r)
	hash := sha256.Sum256([]byte(token))
	if err != nil || token == "" || subtle.ConstantTimeCompare(hash[:], c.SecretHash) != 1 {
		writeJSONError(w, http.StatusUnauthorized, "invalid lead connection credentials")
		return nil, store.LeadIntakeConnection{}, false
	}
	return s, c, true
}

// Verification is read-only and does not assert that the website form sends leads.
func verifyLeadIntake(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _, ok := authenticateLeadIntake(w, r, ws)
		if !ok {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"verified": true})
	}
}

func receiveLeadIntake(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, c, ok := authenticateLeadIntake(w, r, ws)
		if !ok {
			return
		}
		workspaceID, id := r.PathValue("id"), r.PathValue("connectionId")
		body, problems := readJSONBody(w, r, maxLeadPayloadBytes)
		if problems != nil {
			return
		}
		var req struct {
			SubmissionID string  `json:"submissionId"`
			ContactName  *string `json:"contactName"`
			ContactEmail *string `json:"contactEmail"`
			ContactPhone *string `json:"contactPhone"`
			Message      *string `json:"message"`
			UtmSource    *string `json:"utmSource"`
			UtmMedium    *string `json:"utmMedium"`
			UtmCampaign  *string `json:"utmCampaign"`
		}
		if json.Unmarshal(body, &req) != nil || !submissionIDPattern.MatchString(req.SubmissionID) {
			writeJSONError(w, 400, "valid submissionId is required")
			return
		}
		if blankToNil(req.ContactEmail) == nil && blankToNil(req.ContactPhone) == nil {
			writeJSONError(w, 400, "contactEmail or contactPhone is required")
			return
		}
		if email := blankToNil(req.ContactEmail); email != nil {
			parsed, err := mail.ParseAddress(strings.TrimSpace(*email))
			if err != nil || parsed.Address != strings.TrimSpace(*email) {
				writeJSONError(w, 400, "contactEmail must be a valid email address")
				return
			}
		}
		for _, field := range []*string{req.ContactName, req.ContactEmail, req.ContactPhone, req.UtmSource, req.UtmMedium, req.UtmCampaign} {
			if field != nil && len([]rune(*field)) > 255 {
				writeJSONError(w, 400, "contact and tracking fields must be at most 255 characters")
				return
			}
		}
		if req.Message != nil && len([]rune(*req.Message)) > 5000 {
			writeJSONError(w, 400, "message must be at most 5000 characters")
			return
		}
		payloadHash := sha256.Sum256(body)
		leadID, duplicate, err := s.RecordLeadIntake(r.Context(), c, req.SubmissionID, payloadHash[:], store.Lead{ContactName: blankToNil(req.ContactName), ContactEmail: blankToNil(req.ContactEmail), ContactPhone: blankToNil(req.ContactPhone), Message: blankToNil(req.Message), UtmSource: blankToNil(req.UtmSource), UtmMedium: blankToNil(req.UtmMedium), UtmCampaign: blankToNil(req.UtmCampaign)})
		if errors.Is(err, store.ErrSubmissionConflict) {
			writeJSONError(w, 409, "submissionId already used with different data")
			return
		}
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, 401, "invalid lead connection credentials")
			return
		}
		if err != nil {
			writeJSONError(w, 500, "could not record enquiry")
			return
		}
		if !duplicate {
			audit(r.Context(), ws, store.AuditEvent{WorkspaceID: &workspaceID, Action: "lead.created", EntityType: "lead", EntityID: &leadID, Details: map[string]any{"sourceKind": "form_api", "connectionId": id}})
			// A real submission alerts the team; a replayed one does not, and the
			// delivery table's unique index backs that up.
			queueNewLeadEmail(r.Context(), ws, workspaceID, leadID)
		}
		status := http.StatusCreated
		if duplicate {
			status = http.StatusOK
		}
		writeJSON(w, status, map[string]any{"id": leadID, "duplicate": duplicate})
	}
}
