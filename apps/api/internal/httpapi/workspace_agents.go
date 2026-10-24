package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"marketingops/api/internal/agents"
	"marketingops/api/internal/aiprovider"
	"marketingops/api/internal/integration"
	"marketingops/api/internal/secretcipher"
	"marketingops/api/internal/store"
)

const (
	// AgentTimeout bounds one specialist run (data fetch + model call).
	defaultAgentTimeout = 90 * time.Second
	// gscFetchTimeout bounds only the read-only Search Console reads that
	// feed the run; the model call has its own budget.
	gscFetchTimeout = 30 * time.Second
	// Advisory data window: enough page+query rows for cannibalization
	// without shipping the whole workspace to the model.
	gscWindowDays    = 28
	maxPayloadAgents = 4 * 1024
)

// GSCLoader supplies the authorized Search Console snapshot of exactly one
// workspace, or nil with a short note when no connected data exists. Errors
// to reach Google degrade to no-data (with the note) so a specialist run never
// asserts an analysis it could not perform.
type GSCLoader interface {
	LoadSnapshot(ctx context.Context, workspaceID string) (*agents.GSCSnapshot, string, error)
}

// gscAdvisoryLoader resolves the workspace's connected Search Console
// integration and reads a bounded analytics window for it. It sees only the
// integration rows of the requested workspace.
type gscAdvisoryLoader struct {
	ws  WorkspaceStore
	key []byte
}

// NewGSCAdvisoryLoader builds the Search Console snapshot loader used by the
// advisory route. Exported for cmd wiring; tests inject fakes instead.
func NewGSCAdvisoryLoader(ws WorkspaceStore, key []byte) GSCLoader {
	return &gscAdvisoryLoader{ws: ws, key: key}
}

func (l *gscAdvisoryLoader) LoadSnapshot(ctx context.Context, workspaceID string) (*agents.GSCSnapshot, string, error) {
	integrations, err := l.ws.ListIntegrations(ctx, workspaceID)
	if err != nil {
		return nil, "", fmt.Errorf("integration listing failed: %w", err)
	}
	var gsc *store.Integration
	for i := range integrations {
		if integrations[i].AdapterKind == "search_console" {
			gsc = &integrations[i]
			break
		}
	}
	if gsc == nil {
		return nil, "no Search Console integration exists in this workspace", nil
	}
	// Only a connected integration is data the run may rely on; limited or
	// errored ones keep the agent in no-data mode.
	if gsc.Status != "connected" {
		return nil, fmt.Sprintf("the Search Console integration's last check was not connected (status %q)", gsc.Status), nil
	}
	property, _ := gsc.Config["property"].(string)
	property = strings.TrimSpace(property)
	if property == "" {
		return nil, "the Search Console integration has no property configured", nil
	}

	creds := make(map[string]string)
	credSet, credErr := l.ws.GetCredentialSet(ctx, gsc.ID)
	if credErr == nil && len(l.key) > 0 {
		if opened, decErr := secretcipher.Decrypt(l.key, credSet.Ciphertext); decErr == nil {
			_ = json.Unmarshal(opened, &creds)
		}
	}
	ops := &integration.GSCOps{ServiceAccountJSON: strings.TrimSpace(creds["serviceAccount"])}
	if ops.ServiceAccountJSON == "" {
		return nil, "the Search Console integration has no service-account key", nil
	}

	fetchCtx, cancel := context.WithTimeout(ctx, gscFetchTimeout)
	defer cancel()
	now := time.Now().UTC()
	endDate := now.Format("2006-01-02")
	startDate := now.AddDate(0, 0, -gscWindowDays).Format("2006-01-02")

	pageQueries, err := l.analyticsRows(fetchCtx, ops, property, startDate, endDate, []string{"page", "query"}, maxPageQueryRowsLoader)
	if err != nil {
		return nil, fmt.Sprintf("Search Console query failed: %s", safeGSCCause(err)), nil
	}
	topPages, err := l.analyticsRows(fetchCtx, ops, property, startDate, endDate, []string{"page"}, maxTopPagesRowsLoader)
	if err != nil {
		return nil, fmt.Sprintf("Search Console query failed: %s", safeGSCCause(err)), nil
	}

	if len(pageQueries) == 0 && len(topPages) == 0 {
		return nil, "the Search Console property returned no rows for the last 28 days", nil
	}
	snap := &agents.GSCSnapshot{
		Property:    property,
		StartDate:   startDate,
		EndDate:     endDate,
		PageQueries: toPageQueryRows(pageQueries),
		TopPages:    toPageRows(topPages),
	}
	return snap, "", nil
}

// toPageQueryRows maps adapter rows into the bounded agents snapshot type.
func toPageQueryRows(rows []integration.GSCQueryRow) []agents.GSCPageQueryRow {
	out := make([]agents.GSCPageQueryRow, 0, len(rows))
	for _, r := range rows {
		page, query := firstTwoKeys(r.Keys)
		out = append(out, agents.GSCPageQueryRow{
			Page:        page,
			Query:       query,
			Clicks:      r.Clicks,
			Impressions: r.Impressions,
			Ctr:         r.Ctr,
			Position:    r.Position,
		})
	}
	return out
}

// toPageRows maps per-page adapter rows ("page"-dimension requests put the
// page into Keys[0]).
func toPageRows(rows []integration.GSCQueryRow) []agents.GSCPageRow {
	out := make([]agents.GSCPageRow, 0, len(rows))
	for _, r := range rows {
		page, _ := firstTwoKeys(r.Keys)
		out = append(out, agents.GSCPageRow{
			Page:        page,
			Clicks:      r.Clicks,
			Impressions: r.Impressions,
			Ctr:         r.Ctr,
			Position:    r.Position,
		})
	}
	return out
}

func firstTwoKeys(keys []string) (string, string) {
	var first, second string
	if len(keys) > 0 {
		first = keys[0]
	}
	if len(keys) > 1 {
		second = keys[1]
	}
	return first, second
}

// analyticsRows fetches one dimension slice; limits are hard caps.
const (
	maxPageQueryRowsLoader = 50
	maxTopPagesRowsLoader  = 20
	maxRowLimitLoader      = 25000
)

func (l *gscAdvisoryLoader) analyticsRows(ctx context.Context, ops *integration.GSCOps, property, startDate, endDate string, dimensions []string, limit int) ([]integration.GSCQueryRow, error) {
	rows, err := ops.AnalyticsQuery(ctx, time.Now().UTC(), property, startDate, endDate, dimensions, limit)
	if err != nil {
		return nil, err
	}
	if limit > maxRowLimitLoader {
		limit = maxRowLimitLoader
	}
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

// safeGSCCause extracts the safe, credential-free message text.
func safeGSCCause(err error) string {
	var gerr interface{ Error() string }
	if errors.As(err, &gerr) {
		return gerr.Error()
	}
	return err.Error()
}

// POST /api/v1/workspaces/{id}/agents/seo/advisory runs the first curated
// specialist (SEO advisor) for one workspace and returns its validated,
// advisory-only report. It never edits content, publishes, or schedules:
// follow-up actions are always human decisions through existing flows.
func seoAdvisoryHandler(ws WorkspaceStore, runner agents.PromptRun, loader GSCLoader, timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ws == nil {
			writeJSONError(w, http.StatusNotImplemented, workspaceUnavailable)
			return
		}
		if runner == nil {
			writeJSONError(w, http.StatusNotImplemented, "agent specialists are not configured on this server")
			return
		}
		workspaceID := r.PathValue("id")
		workspace, err := ws.GetWorkspaceByID(r.Context(), workspaceID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "no such workspace")
			} else {
				writeJSONError(w, http.StatusInternalServerError, "workspace lookup failed")
			}
			return
		}
		// A workspace without a saved brand profile is still reviewable; the
		// specialist just gets less context.
		brand, err := ws.GetBrandProfile(r.Context(), workspaceID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusInternalServerError, "brand profile lookup failed")
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, maxPayloadAgents+1))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "could not read the request body")
			return
		}
		if len(body) > maxPayloadAgents {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request body is too large")
			return
		}
		var req struct {
			Focus *string `json:"focus"`
		}
		if len(body) > 0 {
			if err := json.Unmarshal(body, &req); err != nil {
				writeJSONError(w, http.StatusBadRequest, "request body is not valid JSON")
				return
			}
		}
		focus := trim(req.Focus)
		if runeCount(focus) > maxGenerateInstrRunes {
			writeJSONError(w, http.StatusBadRequest, "focus is too long (max 2000 characters)")
			return
		}

		if timeout <= 0 {
			timeout = defaultAgentTimeout
		}
		runCtx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		var snapshot *agents.GSCSnapshot
		dataNote := "Search Console data loading is not configured on this server"
		if loader != nil {
			snapshot, dataNote, err = loader.LoadSnapshot(r.Context(), workspaceID)
			if err != nil {
				// Loader failures degrade to the no-data mode, matched with
				// the specialist's own gate.
				snapshot, dataNote = nil, "Search Console data could not be loaded for this workspace"
			}
		}

		advisor := agents.SEOAdvisor{}
		runStarted := time.Now()
		report, agentErr := advisor.Advise(runCtx, runner, agents.AgentInput{
			Workspace: workspaceContext(workspace, brand),
			GSC:       snapshot,
			Focus:     focus,
		})
		runDuration := time.Since(runStarted)
		if agentErr != nil {
			audit(r.Context(), ws, store.AuditEvent{
				WorkspaceID: &workspaceID,
				Action:      "agent.seo.advisory.failed",
				Details: map[string]any{
					"agent_id":      advisor.ID(),
					"agent_version": advisor.Version(),
					"reason_kind":   safeFailureKind(agentErr),
				},
			})
			writeJSONError(w, http.StatusBadGateway, agentErr.Error())
			return
		}

		if dataNote == "" {
			dataNote = "Search Console page+query rows included"
		}
		audit(r.Context(), ws, store.AuditEvent{
			WorkspaceID: &workspaceID,
			Action:      "agent.seo.advisory.completed",
			Details: map[string]any{
				"agent_id":        report.AgentID,
				"agent_version":   report.Version,
				"provider":        report.Provider,
				"model":           report.Model,
				"data_source":     report.DataSource,
				"recommendations": len(report.Recommendations),
				"gaps":            len(report.Gaps),
			},
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"report":     report,
			"dataNote":   dataNote,
			"durationMs": runDuration.Milliseconds(),
		})
	}
}

// safeFailureKind keeps the audit row scalar and secret-free.
func safeFailureKind(err error) string {
	var perr *aiprovider.ProviderError
	if errors.As(err, &perr) {
		if perr.Kind == "" {
			return "unknown"
		}
		return perr.Kind
	}
	return "runtime_error"
}
