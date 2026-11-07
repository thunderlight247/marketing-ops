// Analytics summary endpoint. The dashboard uses one aggregate call instead
// of fetching every workspace's leads, campaigns, and delivery log. The store
// computes the counts in SQL; this handler only enforces availability and
// encodes the result.
package httpapi

import (
	"context"
	"net/http"
	"strings"

	"marketingops/api/internal/store"
)

// AnalyticsStore computes the cross-workspace summary. It is optional so the
// router can still serve everything else when no database is configured.
type AnalyticsStore interface {
	AnalyticsSummary(ctx context.Context, workspaceID string) (store.AnalyticsSummary, error)
}

// GET /api/v1/analytics/summary?workspaceId=...
//
// An omitted or empty workspaceId returns every active workspace. A supplied
// id narrows all three aggregates, so a workspace page cannot show another
// workspace's counts.
func analyticsSummaryHandler(analytics AnalyticsStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if analytics == nil {
			writeJSONError(w, http.StatusNotImplemented, "analytics summary is not configured")
			return
		}
		workspaceID := strings.TrimSpace(r.URL.Query().Get("workspaceId"))
		summary, err := analytics.AnalyticsSummary(r.Context(), workspaceID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "analytics summary failed")
			return
		}
		writeJSON(w, http.StatusOK, summary)
	}
}
