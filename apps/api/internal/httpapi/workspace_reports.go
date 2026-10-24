// Report endpoint (Milestone 12, first increment). A GET on the workspace's
// report returns the counts behind an operator-facing summary. There is no
// export in this increment; the JSON body is the artifact.
//
// Honesty rules carried over from the roadmap:
//   - Content/campaign/lead figures are current-state counts, labelled as
//     such ("now") in the response envelope.
//   - Publishing figures respect the optional since/until range.
//   - No metric is fabricated: sections that a workspace has no verified
//     source for (search analytics) simply never appear.
package httpapi

import (
	"net/http"
	"strings"
	"time"

	"marketingops/api/internal/store"
)

// GET /api/v1/workspaces/{id}/reports?since=RFC3339&until=RFC3339
//
// Both bounds are optional. An empty `since` means "the beginning"; an empty
// `until` means "now". until must be strictly after since when both are set.
// Timestamps are parsed from RFC3339 and treated as UTC instants.
func getWorkspaceReportHandler(ws WorkspaceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspaceID, ok := workspaceScope(w, r, ws)
		if !ok {
			return
		}

		rg := store.ReportRange{}
		if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
			t, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				writeJSONError(w, http.StatusBadRequest, "since must be an RFC3339 timestamp")
				return
			}
			rg.Since = &t
		}
		if raw := strings.TrimSpace(r.URL.Query().Get("until")); raw != "" {
			t, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				writeJSONError(w, http.StatusBadRequest, "until must be an RFC3339 timestamp")
				return
			}
			rg.Until = &t
		}
		if rg.Since != nil && rg.Until != nil && !rg.Until.After(*rg.Since) {
			writeJSONError(w, http.StatusBadRequest, "until must be after since")
			return
		}

		report, err := ws.WorkspaceReport(r.Context(), workspaceID, rg)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "report generation failed")
			return
		}

		rangeJSON := map[string]any{"since": nil, "until": nil}
		if rg.Since != nil {
			rangeJSON["since"] = rg.Since.UTC().Format(time.RFC3339)
		}
		if rg.Until != nil {
			rangeJSON["until"] = rg.Until.UTC().Format(time.RFC3339)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"workspaceId": report.WorkspaceID,
			"generatedAt": report.GeneratedAt.UTC().Format(time.RFC3339),
			"range":       rangeJSON,
			// Labels make the two time bases explicit so a reader cannot
			// mistake a current-state figure for range-filtered history.
			"contentStatusNow": report.ContentStatus,
			"campaignsNow":     report.CampaignsBy,
			"leadsNow":         report.LeadsByStatus,
			"leadSourcesNow":   report.LeadsBySource,
			"publishingRange": map[string]any{
				"jobsByStatus":   report.PublishingJobs,
				"publishedCount": report.PublishedCount,
				"failedCount":    report.FailedCount,
				"recentFailures": report.RecentFailures,
			},
		})
	}
}
