package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"marketingops/api/internal/store"
)

// GET /api/v1/operators
func listOperatorsHandler(ops OperatorProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ops == nil {
			writeJSONError(w, http.StatusNotImplemented, "operator management is not configured")
			return
		}
		list, err := ops.ListOperators(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to list operators")
			return
		}
		if list == nil {
			list = []store.Operator{}
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// POST /api/v1/operators
func createOperatorHandler(ops OperatorProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ops == nil {
			writeJSONError(w, http.StatusNotImplemented, "operator management is not configured")
			return
		}

		var in struct {
			Name  string `json:"name"`
			Email string `json:"email"`
			Role  string `json:"role"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, maxWorkspacePayloadBytes)).Decode(&in); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		in.Name = strings.TrimSpace(in.Name)
		in.Email = strings.TrimSpace(in.Email)
		in.Role = strings.TrimSpace(strings.ToLower(in.Role))

		if in.Name == "" || in.Email == "" {
			writeJSONError(w, http.StatusBadRequest, "name and email are required")
			return
		}
		if in.Role == "" {
			in.Role = "editor"
		}
		if in.Role != "admin" && in.Role != "editor" && in.Role != "viewer" {
			writeJSONError(w, http.StatusBadRequest, "role must be admin, editor, or viewer")
			return
		}

		created, err := ops.CreateOperator(r.Context(), in.Name, in.Email, in.Role)
		if err != nil {
			writeJSONError(w, http.StatusConflict, "could not create operator (email may already exist)")
			return
		}

		writeJSON(w, http.StatusCreated, created)
	}
}

// PATCH /api/v1/operators/{id}
func updateOperatorHandler(ops OperatorProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ops == nil {
			writeJSONError(w, http.StatusNotImplemented, "operator management is not configured")
			return
		}

		id := r.PathValue("id")
		if id == "" {
			writeJSONError(w, http.StatusBadRequest, "operator id is required")
			return
		}

		var in struct {
			Role   string `json:"role"`
			Status string `json:"status"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, maxWorkspacePayloadBytes)).Decode(&in); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		in.Role = strings.TrimSpace(strings.ToLower(in.Role))
		in.Status = strings.TrimSpace(strings.ToLower(in.Status))

		if in.Role != "" && in.Role != "admin" && in.Role != "editor" && in.Role != "viewer" {
			writeJSONError(w, http.StatusBadRequest, "invalid role")
			return
		}
		if in.Status != "" && in.Status != "active" && in.Status != "inactive" {
			writeJSONError(w, http.StatusBadRequest, "invalid status")
			return
		}

		updated, err := ops.UpdateOperator(r.Context(), id, in.Role, in.Status)
		if errors.Is(err, store.ErrLastAdmin) {
			writeJSONError(w, http.StatusConflict, "at least one active admin is required")
			return
		}
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "operator not found")
			return
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "could not update operator")
			return
		}

		writeJSON(w, http.StatusOK, updated)
	}
}
