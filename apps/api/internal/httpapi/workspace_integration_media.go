package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"marketingops/api/internal/integration"
	"marketingops/api/internal/secretcipher"
	"marketingops/api/internal/store"
)

// maxMediaUploadBytes is the accepted request size for one image upload.
const maxMediaUploadBytes = 8 << 20 // 8 MB

// integrationConfigString reads one non-secret display value from an
// integration config map.
func integrationConfigString(config map[string]any, key string) string {
	value, _ := config[key].(string)
	return value
}

// uploadIntegrationMediaHandler proxies one image upload to the workspace's
// connected Sanity project. The write token never leaves the server: the
// browser sends the file, the server attaches the decrypted credentials, and
// the response carries only the asset reference and its public CDN URL.
//
// POST /api/v1/workspaces/{id}/integrations/{integrationId}/media
// Body: multipart/form-data with one "file" part (JPEG, PNG, WebP, or GIF).
func uploadIntegrationMediaHandler(ws WorkspaceStore, key []byte, apiBase string, client *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "POST required")
			return
		}
		workspaceID, integrationID := r.PathValue("id"), r.PathValue("integrationId")

		if _, err := ws.GetWorkspaceByID(r.Context(), workspaceID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "no such workspace")
			} else {
				writeJSONError(w, http.StatusInternalServerError, "workspace lookup failed")
			}
			return
		}

		row, err := ws.GetIntegrationInWorkspace(r.Context(), workspaceID, integrationID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "no such integration in this workspace")
			} else {
				writeJSONError(w, http.StatusInternalServerError, "integration lookup failed")
			}
			return
		}
		if row.AdapterKind != "sanity" {
			writeJSONError(w, http.StatusConflict, "media uploads are only supported for Sanity integrations")
			return
		}

		credRow, err := ws.GetCredentialSet(r.Context(), row.ID)
		if err != nil || len(key) == 0 {
			writeJSONError(w, http.StatusConflict, integration.ErrPublisherCredentialsMissing.Error())
			return
		}
		credentials := map[string]string{}
		opened, err := secretcipher.Decrypt(key, credRow.Ciphertext)
		if err != nil || json.Unmarshal(opened, &credentials) != nil {
			writeJSONError(w, http.StatusConflict, integration.ErrPublisherCredentialsMissing.Error())
			return
		}
		token := credentials["token"]
		projectID := integrationConfigString(row.Config, "projectId")
		dataset := integrationConfigString(row.Config, "dataset")
		if token == "" || projectID == "" || dataset == "" {
			writeJSONError(w, http.StatusConflict, integration.ErrPublisherNotConfigured.Error())
			return
		}

		if err := r.ParseMultipartForm(maxMediaUploadBytes + 1); err != nil {
			writeJSONError(w, http.StatusBadRequest, "expected a multipart form with a \"file\" part")
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "the multipart form has no \"file\" part")
			return
		}
		defer func() { _ = file.Close() }()
		if header.Size > maxMediaUploadBytes {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "image is too large (max 8 MB)")
			return
		}

		// Sniff the first bytes; browser-claimed content types are not trusted.
		sniff := make([]byte, 512)
		n, _ := io.ReadFull(file, sniff)
		kind := http.DetectContentType(sniff[:n])
		switch kind {
		case "image/jpeg", "image/png", "image/webp", "image/gif":
		default:
			writeJSONError(w, http.StatusUnsupportedMediaType, "only JPEG, PNG, WebP, or GIF images are supported")
			return
		}

		ops := integration.SanityOps{
			Creds:  integration.SanityCredentials{ProjectID: projectID, Dataset: dataset, Token: token},
			Base:   apiBase,
			Client: client,
		}
		upload, err := ops.UploadImage(r.Context(), header.Filename, file)
		if err != nil {
			writePublisherFailure(w, err)
			return
		}

		audit(r.Context(), ws, store.AuditEvent{
			WorkspaceID: &workspaceID,
			Action:      "integration.media_uploaded",
			EntityType:  "integration",
			EntityID:    &row.ID,
			Details: map[string]any{
				"integrationId": row.ID,
				"assetRef":      upload.Ref,
				"kind":          strings.TrimPrefix(kind, "image/"),
			},
		})

		writeJSON(w, http.StatusCreated, map[string]any{
			"ref": upload.Ref,
			"url": upload.URL,
		})
	}
}
