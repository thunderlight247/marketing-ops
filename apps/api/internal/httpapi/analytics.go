// Analytics summary endpoint. The dashboard uses one aggregate call instead
// of fetching every workspace's leads, campaigns, and delivery log. The store
// computes the counts in SQL; this handler only enforces availability and
// encodes the result.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"marketingops/api/internal/store"
)

// AnalyticsStore computes the cross-workspace summary. It is optional so the
// router can still serve everything else when no database is configured.
type AnalyticsStore interface {
	AnalyticsSummary(ctx context.Context, workspaceID string) (store.AnalyticsSummary, error)
	AnalyticsBreakdown(ctx context.Context, workspaceID string, from, to time.Time) (store.AnalyticsBreakdown, error)
}

// GET /api/v1/analytics/breakdown?workspaceId=...&from=YYYY-MM-DD&to=YYYY-MM-DD
//
// Returns the daily lead trend and UTM breakdown for [from, to). Dates are
// inclusive days in UTC; the default range is the last 30 days. The range is
// capped at 366 days so one request cannot scan unbounded history.
func analyticsBreakdownHandler(analytics AnalyticsStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if analytics == nil {
			writeJSONError(w, http.StatusNotImplemented, "analytics breakdown is not configured")
			return
		}
		q := r.URL.Query()
		now := time.Now().UTC()
		to := now.Truncate(24*time.Hour).AddDate(0, 0, 1)
		from := to.AddDate(0, 0, -30)
		if v := strings.TrimSpace(q.Get("from")); v != "" {
			t, err := time.Parse("2006-01-02", v)
			if err != nil {
				writeJSONError(w, http.StatusBadRequest, "from must be YYYY-MM-DD")
				return
			}
			from = t
		}
		if v := strings.TrimSpace(q.Get("to")); v != "" {
			t, err := time.Parse("2006-01-02", v)
			if err != nil {
				writeJSONError(w, http.StatusBadRequest, "to must be YYYY-MM-DD")
				return
			}
			to = t.AddDate(0, 0, 1)
		}
		if !from.Before(to) || to.Sub(from) > 366*24*time.Hour {
			writeJSONError(w, http.StatusBadRequest, "date range must be 1 to 366 days")
			return
		}
		workspaceID := strings.TrimSpace(q.Get("workspaceId"))
		breakdown, err := analytics.AnalyticsBreakdown(r.Context(), workspaceID, from, to)
		if err != nil {
			slog.Default().Error("analytics breakdown failed", "workspace_id", workspaceID, "err", err)
			writeJSONError(w, http.StatusInternalServerError, "analytics breakdown failed")
			return
		}
		writeJSON(w, http.StatusOK, breakdown)
	}
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
			slog.Default().Error("analytics summary failed", "workspace_id", workspaceID, "err", err)
			writeJSONError(w, http.StatusInternalServerError, "analytics summary failed")
			return
		}
		writeJSON(w, http.StatusOK, summary)
	}
}
