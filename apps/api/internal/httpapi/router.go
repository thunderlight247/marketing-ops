// Package httpapi wires HTTP routes and middleware. Handlers stay thin: they
// call a service interface and encode the result.
package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"marketingops/api/internal/agents"
	"marketingops/api/internal/health"
	"marketingops/api/internal/integration"
)

// Default rate-limit for expensive AI/agent endpoints: 20 requests per minute
// per IP. This prevents accidental runaway calls while never impeding normal
// single-operator use.
const (
	defaultAIRateLimit  = 20
	defaultAIRateWindow = time.Minute
)

// HealthChecker is implemented by *health.Service.
type HealthChecker interface {
	Check(ctx context.Context) health.Report
}

// Deps are the collaborators the router needs.
type Deps struct {
	Health               HealthChecker
	Logger               *slog.Logger
	AllowedOrigins       []string
	Workspaces           WorkspaceStore // optional; nil disables the workspaces routes
	Integrations         *integration.Registry
	CredentialKey        []byte
	CredentialKeyVersion int

	// APIKey is the operator's Bearer token. When non-empty every request
	// except /health and /ready must supply it. Empty means development mode
	// (no auth enforced); production startup rejects an empty key via config.
	APIKey string

	// Operators resolves the acting operator in single-operator mode (D16);
	// nil makes the decision route answer 501 alongside the store check.
	Operators OperatorProvider

	// SanityOpsBase overrides the Sanity API host in development/tests only;
	// HTTPClient customizes publisher transport (and is used by safe tests).
	SanityOpsBase string
	HTTPClient    *http.Client

	// AIGenerator drafts content; nil makes the generate route answer 501.
	AIGenerator     ContentGenerator
	GenerateTimeout time.Duration

	// AgentRunner transports specialist prompts (agents.PromptRun); nil makes
	// the run route answer 501. Agents is the registry of enabled specialists
	// (a default registry is built when absent); AgentLoaders answers each
	// declared data need with the workspace's own authorized snapshot.
	AgentRunner  agents.PromptRun
	Agents       *agents.Registry
	AgentLoaders map[string]AgentSnapshotLoader
	AgentTimeout time.Duration

	// AIRateLimit / AIRateWindow set the per-IP cap on AI/agent endpoints.
	// Zero values use the package defaults (20 req/min).
	AIRateLimit  int
	AIRateWindow time.Duration
}

// NewRouter returns the API handler with middleware applied.
func NewRouter(d Deps) http.Handler {
	if d.Integrations == nil {
		d.Integrations = integration.NewDefaultRegistry()
	}
	// AI rate limiter: caps expensive model calls per IP.
	aiLimit, aiWindow := d.AIRateLimit, d.AIRateWindow
	if aiLimit <= 0 {
		aiLimit = defaultAIRateLimit
	}
	if aiWindow <= 0 {
		aiWindow = defaultAIRateWindow
	}
	aiLimiter := NewRateLimiter(aiLimit, aiWindow)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", healthHandler(d.Health))
	mux.HandleFunc("GET /api/v1/ready", readyHandler(d.Health))
	// Workspaces handlers nil-check their store and answer 501 when absent.
	mux.HandleFunc("GET /api/v1/workspaces", listWorkspacesHandler(d.Workspaces))
	mux.HandleFunc("POST /api/v1/workspaces", createWorkspaceHandler(d.Workspaces))
	mux.HandleFunc("GET /api/v1/workspaces/{id}", getWorkspaceDetailHandler(d.Workspaces))
	mux.HandleFunc("PUT /api/v1/workspaces/{id}/brand", putWorkspaceBrandHandler(d.Workspaces))
	mux.HandleFunc("PATCH /api/v1/workspaces/{id}", patchWorkspaceHandler(d.Workspaces))
	mux.HandleFunc("DELETE /api/v1/workspaces/{id}", deleteWorkspaceHandler(d.Workspaces))
	mux.HandleFunc("GET /api/v1/workspaces/{id}/content", listWorkspaceContentHandler(d.Workspaces))
	mux.HandleFunc("POST /api/v1/workspaces/{id}/content", createWorkspaceContentHandler(d.Workspaces))
	mux.HandleFunc("GET /api/v1/workspaces/{id}/content/{itemId}", getWorkspaceContentItemHandler(d.Workspaces))
	mux.HandleFunc("PATCH /api/v1/workspaces/{id}/content/{itemId}", updateWorkspaceContentItemHandler(d.Workspaces))
	mux.HandleFunc("GET /api/v1/workspaces/{id}/content/{itemId}/history", getWorkspaceContentHistoryHandler(d.Workspaces))
	mux.HandleFunc("GET /api/v1/workspaces/{id}/content/{itemId}/jobs", listContentJobsHandler(d.Workspaces))
	mux.HandleFunc("POST /api/v1/workspaces/{id}/content/{itemId}/jobs/{jobId}/cancel", cancelContentJobHandler(d.Workspaces))
	mux.HandleFunc("POST /api/v1/workspaces/{id}/content/{itemId}/jobs/{jobId}/retry", retryContentJobHandler(d.Workspaces))
	mux.HandleFunc("POST /api/v1/workspaces/{id}/content/{itemId}/decision", decideWorkspaceContentHandler(d.Workspaces, d.Operators))
	mux.HandleFunc("POST /api/v1/workspaces/{id}/content/{itemId}/status", transitionWorkspaceContentHandler(d.Workspaces))
	mux.HandleFunc("POST /api/v1/workspaces/{id}/content/{itemId}/publish", publishWorkspaceContentHandler(d.Workspaces, d.CredentialKey, d.Integrations, d.SanityOpsBase, d.HTTPClient))
	mux.Handle("POST /api/v1/workspaces/{id}/content/generate",
		aiLimiter.Middleware(http.HandlerFunc(generateWorkspaceContentHandler(d.Workspaces, d.AIGenerator, d.GenerateTimeout))))
	// Agency Agents: one set of routes for every registered specialist, each
	// behind the same AI rate limit. Nil runner answers 501; unknown agents
	// answer 404.
	if d.Agents == nil {
		d.Agents = agents.NewDefaultRegistry()
	}
	mux.HandleFunc("GET /api/v1/agents", listAgentsHandler(d.Agents))
	mux.Handle("POST /api/v1/workspaces/{id}/agents/{agentId}/run",
		aiLimiter.Middleware(http.HandlerFunc(workspaceAgentRunHandler(
			d.Workspaces, d.Agents, d.AgentRunner, d.AgentLoaders, d.AgentTimeout))))
	// Campaigns and leads (Milestone 11); workspace-scoped throughout.
	mux.HandleFunc("GET /api/v1/workspaces/{id}/campaigns", listWorkspaceCampaignsHandler(d.Workspaces))
	mux.HandleFunc("POST /api/v1/workspaces/{id}/campaigns", createWorkspaceCampaignHandler(d.Workspaces))
	mux.HandleFunc("GET /api/v1/workspaces/{id}/campaigns/{campaignId}", getWorkspaceCampaignHandler(d.Workspaces))
	mux.HandleFunc("PATCH /api/v1/workspaces/{id}/campaigns/{campaignId}", patchWorkspaceCampaignHandler(d.Workspaces))
	mux.HandleFunc("GET /api/v1/workspaces/{id}/leads", listWorkspaceLeadsHandler(d.Workspaces))
	mux.HandleFunc("POST /api/v1/workspaces/{id}/leads", createWorkspaceLeadHandler(d.Workspaces))
	mux.HandleFunc("PATCH /api/v1/workspaces/{id}/leads/{leadId}", patchWorkspaceLeadHandler(d.Workspaces))
	// Reporting (Milestone 12, first increment); workspace-scoped.
	mux.HandleFunc("GET /api/v1/workspaces/{id}/reports", getWorkspaceReportHandler(d.Workspaces))
	// Integrations handlers
	mux.HandleFunc("GET /api/v1/integrations/adapters", listAdaptersHandler(d.Integrations))
	mux.HandleFunc("GET /api/v1/workspaces/{id}/integrations", listWorkspaceIntegrationsHandler(d.Workspaces))
	mux.HandleFunc("POST /api/v1/workspaces/{id}/integrations", createWorkspaceIntegrationHandler(d.Workspaces, d.Integrations))
	mux.HandleFunc("GET /api/v1/workspaces/{id}/integrations/{integrationId}", getWorkspaceIntegrationHandler(d.Workspaces))
	mux.HandleFunc("PUT /api/v1/workspaces/{id}/integrations/{integrationId}/credentials", putIntegrationCredentialsHandler(d.Workspaces, d.Integrations, d.CredentialKey, d.CredentialKeyVersion))
	mux.HandleFunc("POST /api/v1/workspaces/{id}/integrations/{integrationId}/media", uploadIntegrationMediaHandler(d.Workspaces, d.CredentialKey, d.SanityOpsBase, d.HTTPClient))
	mux.HandleFunc("POST /api/v1/workspaces/{id}/integrations/{integrationId}/check", checkIntegrationCapabilitiesHandler(d.Workspaces, d.Integrations, d.CredentialKey))
	mux.HandleFunc("DELETE /api/v1/workspaces/{id}/integrations/{integrationId}", deleteWorkspaceIntegrationHandler(d.Workspaces))

	// Outermost first: recover -> log -> security headers -> CORS -> auth -> routes.
	return recoverer(d.Logger)(
		requestLogger(d.Logger)(
			securityHeaders(
				cors(d.AllowedOrigins)(
					authMiddleware(d.APIKey)(mux)))))
}

// healthHandler always answers 200 while the process is up. The body says
// whether dependencies are healthy, so the dashboard can show details.
func healthHandler(hc HealthChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, hc.Check(r.Context()))
	}
}

// readyHandler answers 503 unless every dependency is healthy, for load
// balancers and deploy checks.
func readyHandler(hc HealthChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report := hc.Check(r.Context())
		code := http.StatusOK
		if report.Status != health.StatusOK {
			code = http.StatusServiceUnavailable
		}
		writeJSON(w, code, report)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeJSONError answers with a fixed, non-internal error message.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// statusRecorder captures the response status for logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// requestLogger logs method, path, status, and duration. It never logs query
// strings or headers, which can carry tokens.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}

func recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered", "path", r.URL.Path, "panic", rec)
					http.Error(w, "internal server error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// cors allows only the configured browser origins. Requests from other
// origins get no CORS headers, so browsers block them.
func cors(allowed []string) func(http.Handler) http.Handler {
	set := make(map[string]struct{}, len(allowed))
	for _, origin := range allowed {
		set[origin] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if _, ok := set[origin]; ok && origin != "" {
				w.Header().Add("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Origin", origin)
				isPreflight := r.Method == http.MethodOptions &&
					r.Header.Get("Access-Control-Request-Method") != ""
				if isPreflight {
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
					w.Header().Set("Access-Control-Max-Age", "600")
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
