package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"marketingops/api/internal/integration"
	"marketingops/api/internal/secretcipher"
	"marketingops/api/internal/store"
)

type capabilityJSON struct {
	Capability string  `json:"capability"`
	Status     string  `json:"status"`
	Details    *string `json:"details,omitempty"`
	VerifiedAt *string `json:"verifiedAt,omitempty"`
}

type integrationJSON struct {
	ID             string           `json:"id"`
	WorkspaceID    string           `json:"workspaceId"`
	AdapterKind    string           `json:"adapterKind"`
	Name           string           `json:"name"`
	Status         string           `json:"status"`
	Config         map[string]any   `json:"config"`
	HasCredentials bool             `json:"hasCredentials"`
	Capabilities   []capabilityJSON `json:"capabilities"`
	CreatedAt      string           `json:"createdAt"`
	UpdatedAt      string           `json:"updatedAt"`
}

func toIntegrationJSON(in store.Integration, hasCreds bool, caps []store.IntegrationCapability) integrationJSON {
	capsJSON := make([]capabilityJSON, 0, len(caps))
	for _, c := range caps {
		var verifiedAt *string
		if c.VerifiedAt != nil {
			v := c.VerifiedAt.UTC().Format(time.RFC3339)
			verifiedAt = &v
		}
		capsJSON = append(capsJSON, capabilityJSON{
			Capability: c.Capability,
			Status:     c.Status,
			Details:    c.Details,
			VerifiedAt: verifiedAt,
		})
	}

	return integrationJSON{
		ID:             in.ID,
		WorkspaceID:    in.WorkspaceID,
		AdapterKind:    in.AdapterKind,
		Name:           in.Name,
		Status:         in.Status,
		Config:         in.Config,
		HasCredentials: hasCreds,
		Capabilities:   capsJSON,
		CreatedAt:      in.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:      in.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

const maxIntegrationPayloadBytes = 16 * 1024

// GET /api/v1/integrations/adapters returns available adapter types.
func listAdaptersHandler(reg *integration.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if reg == nil {
			writeJSONError(w, http.StatusNotImplemented, "integration registry unavailable")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"adapters": reg.ListDescriptors()})
	}
}

// GET /api/v1/workspaces/{id}/integrations returns all integrations for one workspace.
func listWorkspaceIntegrationsHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		workspaceID := r.PathValue("id")
		if workspaceID == "" {
			writeJSONError(w, http.StatusBadRequest, "workspace id is required")
			return
		}
		if _, err := ws.GetWorkspaceByID(r.Context(), workspaceID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "no such workspace")
			} else {
				writeJSONError(w, http.StatusInternalServerError, "workspace lookup failed")
			}
			return
		}

		list, err := ws.ListIntegrations(r.Context(), workspaceID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "integrations listing failed")
			return
		}

		out := make([]integrationJSON, 0, len(list))
		for _, in := range list {
			caps, _ := ws.ListCapabilities(r.Context(), in.ID)
			_, credErr := ws.GetCredentialSet(r.Context(), in.ID)
			hasCreds := credErr == nil
			out = append(out, toIntegrationJSON(in, hasCreds, caps))
		}

		writeJSON(w, http.StatusOK, map[string]any{"integrations": out})
	}
}

// POST /api/v1/workspaces/{id}/integrations creates an integration.
func createWorkspaceIntegrationHandler(ws WorkspaceStore, reg *integration.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil || reg == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		workspaceID := r.PathValue("id")
		if _, err := ws.GetWorkspaceByID(r.Context(), workspaceID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "no such workspace")
			} else {
				writeJSONError(w, http.StatusInternalServerError, "workspace lookup failed")
			}
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, maxIntegrationPayloadBytes+1))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "could not read request body")
			return
		}
		if len(body) > maxIntegrationPayloadBytes {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request body is too large")
			return
		}

		var req struct {
			Name        *string        `json:"name"`
			AdapterKind *string        `json:"adapterKind"`
			Config      map[string]any `json:"config"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "request body is not valid JSON")
			return
		}

		var problems []string
		name := trim(req.Name)
		if name == "" {
			problems = append(problems, "name is required")
		} else if runeCount(name) > 120 {
			problems = append(problems, "name is too long (max 120 characters)")
		}

		adapterKind := trim(req.AdapterKind)
		adapter, ok := reg.Get(adapterKind)
		if adapterKind == "" {
			problems = append(problems, "adapterKind is required")
		} else if !ok {
			problems = append(problems, "unsupported adapterKind: "+adapterKind)
		}

		if len(problems) > 0 {
			writeJSONError(w, http.StatusBadRequest, strings.Join(problems, "; "))
			return
		}

		cfg := req.Config
		if cfg == nil {
			cfg = map[string]any{}
		}

		created, err := ws.InsertIntegration(r.Context(), store.Integration{
			WorkspaceID: workspaceID,
			AdapterKind: adapterKind,
			Name:        name,
			Status:      integration.LifecycleUnconfigured,
			Config:      cfg,
		})
		if errors.Is(err, store.ErrSlugTaken) {
			writeJSONError(w, http.StatusConflict, "an integration with that name already exists in this workspace")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "integration creation failed")
			return
		}

		// Seed initial default capabilities
		defaults := adapter.DefaultCapabilities()
		for _, d := range defaults {
			details := d.Details
			_ = ws.UpsertCapability(r.Context(), created.ID, store.IntegrationCapability{
				IntegrationID: created.ID,
				Capability:    d.Capability,
				Status:        d.Status,
				Details:       &details,
			})
		}

		audit(r.Context(), ws, store.AuditEvent{
			WorkspaceID: &workspaceID,
			Action:      "integration.created",
			Details: map[string]any{
				"id":          created.ID,
				"adapterKind": created.AdapterKind,
				"name":        created.Name,
			},
		})

		caps, _ := ws.ListCapabilities(r.Context(), created.ID)
		writeJSON(w, http.StatusCreated, toIntegrationJSON(created, false, caps))
	}
}

// GET /api/v1/workspaces/{id}/integrations/{integrationId} returns details for one integration.
func getWorkspaceIntegrationHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		workspaceID := r.PathValue("id")
		integrationID := r.PathValue("integrationId")

		in, err := ws.GetIntegrationInWorkspace(r.Context(), workspaceID, integrationID)
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "no such integration")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "integration lookup failed")
			return
		}

		caps, _ := ws.ListCapabilities(r.Context(), in.ID)
		_, credErr := ws.GetCredentialSet(r.Context(), in.ID)
		hasCreds := credErr == nil

		writeJSON(w, http.StatusOK, toIntegrationJSON(in, hasCreds, caps))
	}
}

// PUT /api/v1/workspaces/{id}/integrations/{integrationId}/credentials saves encrypted credentials and runs check.
func putIntegrationCredentialsHandler(ws WorkspaceStore, reg *integration.Registry, key []byte, keyVersion int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil || reg == nil || len(key) == 0 {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		workspaceID := r.PathValue("id")
		integrationID := r.PathValue("integrationId")

		in, err := ws.GetIntegrationInWorkspace(r.Context(), workspaceID, integrationID)
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "no such integration")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "integration lookup failed")
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, maxIntegrationPayloadBytes+1))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "could not read request body")
			return
		}
		if len(body) > maxIntegrationPayloadBytes {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request body is too large")
			return
		}

		var req struct {
			Credentials map[string]string `json:"credentials"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "request body is not valid JSON")
			return
		}
		if len(req.Credentials) == 0 {
			writeJSONError(w, http.StatusBadRequest, "credentials object is required and cannot be empty")
			return
		}

		// Encrypt credentials JSON
		credJSON, err := json.Marshal(req.Credentials)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to serialize credentials")
			return
		}
		ciphertext, err := secretcipher.Encrypt(key, credJSON)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to encrypt credentials")
			return
		}

		if err := ws.PutCredentialSet(r.Context(), store.CredentialSet{
			IntegrationID: integrationID,
			KeyVersion:    keyVersion,
			Ciphertext:    ciphertext,
		}); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to store credentials")
			return
		}

		// Run read-only capability check with new credentials
		adapter, ok := reg.Get(in.AdapterKind)
		if ok {
			checkRes := adapter.Check(r.Context(), in.Config, req.Credentials)
			_ = ws.SetIntegrationStatus(r.Context(), workspaceID, integrationID, checkRes.LifecycleStatus)
			for _, c := range checkRes.Capabilities {
				details := c.Details
				verifiedAt := checkRes.CheckedAt
				_ = ws.UpsertCapability(r.Context(), integrationID, store.IntegrationCapability{
					IntegrationID: integrationID,
					Capability:    c.Capability,
					Status:        c.Status,
					Details:       &details,
					VerifiedAt:    &verifiedAt,
				})
			}
		}

		audit(r.Context(), ws, store.AuditEvent{
			WorkspaceID: &workspaceID,
			Action:      "integration.credentials_updated",
			Details: map[string]any{
				"id":         integrationID,
				"keyVersion": keyVersion,
			},
		})

		// Return fresh state
		updated, _ := ws.GetIntegrationInWorkspace(r.Context(), workspaceID, integrationID)
		caps, _ := ws.ListCapabilities(r.Context(), integrationID)
		writeJSON(w, http.StatusOK, toIntegrationJSON(updated, true, caps))
	}
}

// POST /api/v1/workspaces/{id}/integrations/{integrationId}/check triggers a read-only capability check.
func checkIntegrationCapabilitiesHandler(ws WorkspaceStore, reg *integration.Registry, key []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil || reg == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		workspaceID := r.PathValue("id")
		integrationID := r.PathValue("integrationId")

		in, err := ws.GetIntegrationInWorkspace(r.Context(), workspaceID, integrationID)
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "no such integration")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "integration lookup failed")
			return
		}

		adapter, ok := reg.Get(in.AdapterKind)
		if !ok {
			writeJSONError(w, http.StatusBadRequest, "unsupported adapter kind: "+in.AdapterKind)
			return
		}

		// Load credentials if present. A stored set that cannot be opened is a
		// diagnosable problem in itself (M13): the check still runs read-only,
		// but the result says why the credentials did not apply instead of
		// silently looking like no credentials were entered.
		creds := make(map[string]string)
		credProblem := ""
		credSet, err := ws.GetCredentialSet(r.Context(), integrationID)
		switch {
		case err != nil:
			// Nothing stored: the adapter's own diagnostic explains what is missing.
		case len(key) == 0:
			credProblem = "no_server_key"
		default:
			opened, derr := secretcipher.Decrypt(key, credSet.Ciphertext)
			if derr != nil {
				credProblem = "decrypt_failed"
			} else if json.Unmarshal(opened, &creds) != nil || len(creds) == 0 {
				credProblem = "invalid_format"
			}
		}

		checkRes := adapter.Check(r.Context(), in.Config, creds)
		if credProblem != "" {
			prefix := "Credential error: stored credentials could not be applied ("
			switch credProblem {
			case "no_server_key":
				prefix += "the server has no credential key configured; supply one and re-enter the credentials)"
			case "decrypt_failed":
				prefix += "decryption with the current server key failed, the key may have changed; re-enter the credentials)"
			default:
				prefix += "the stored credential payload did not parse as expected; re-enter the credentials)"
			}
			// The adapter's own diagnostic (e.g. "Waiting for the ... token")
			// would mislead when credentials actually exist but are unreadable.
			checkRes.LifecycleStatus = integration.LifecycleError
			checkRes.Diagnostic = prefix + ". " + checkRes.Diagnostic
		}
		_ = ws.SetIntegrationStatus(r.Context(), workspaceID, integrationID, checkRes.LifecycleStatus)
		for _, c := range checkRes.Capabilities {
			details := c.Details
			verifiedAt := checkRes.CheckedAt
			_ = ws.UpsertCapability(r.Context(), integrationID, store.IntegrationCapability{
				IntegrationID: integrationID,
				Capability:    c.Capability,
				Status:        c.Status,
				Details:       &details,
				VerifiedAt:    &verifiedAt,
			})
		}

		audit(r.Context(), ws, store.AuditEvent{
			WorkspaceID: &workspaceID,
			Action:      "integration.checked",
			Details: map[string]any{
				"id":                integrationID,
				"status":            checkRes.LifecycleStatus,
				"credentialProblem": credProblem,
			},
		})

		writeJSON(w, http.StatusOK, checkRes)
	}
}

// DELETE /api/v1/workspaces/{id}/integrations/{integrationId} removes an integration.
func deleteWorkspaceIntegrationHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		workspaceID := r.PathValue("id")
		integrationID := r.PathValue("integrationId")

		in, err := ws.GetIntegrationInWorkspace(r.Context(), workspaceID, integrationID)
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "no such integration")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "integration lookup failed")
			return
		}

		if err := ws.DeleteIntegration(r.Context(), workspaceID, integrationID); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to delete integration")
			return
		}

		audit(r.Context(), ws, store.AuditEvent{
			WorkspaceID: &workspaceID,
			Action:      "integration.deleted",
			Details: map[string]any{
				"id":          in.ID,
				"adapterKind": in.AdapterKind,
				"name":        in.Name,
			},
		})

		w.WriteHeader(http.StatusNoContent)
	}
}
