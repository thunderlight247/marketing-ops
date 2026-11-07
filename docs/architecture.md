# Architecture

The platform is a modular monolith: one Go API, one Next.js dashboard, one PostgreSQL database, and one optional Node WhatsApp dispatcher. The full product direction and live status are in [`remember.md`](../remember.md).

```
Browser
  └─ Cloudflare Worker (optional stable URL, no auth)
       └─ Next.js dashboard (apps/web)
            ├─ HTTP Basic operator auth (middleware)
            ├─ same-origin /api/v1 proxy (adds API key server-side)
            └─ Go API (apps/api)
                 ├─ pgx ──> PostgreSQL
                 ├─ publishing worker
                 ├─ email worker
                 └─ lead-engine scheduler ──> WhatsApp dispatcher
```

## API (`apps/api`)

```
cmd/api/              process entry: config, logging, workers, scheduler, graceful shutdown
cmd/migrate/          migration CLI: up | status
internal/config/      environment loading and validation (never prints secrets)
internal/health/      health report logic
internal/httpapi/     routes, thin handlers, middleware, workspace-scoped endpoints
internal/store/       data access; every client-owned query filters by workspace_id
internal/migrate/     forward-only embedded SQL migration runner
internal/publishworker/  due publishing jobs, retries, artifact records
internal/emailworker/    queued outbound email
internal/leadengine/     discovery, outreach, scheduler
internal/integration/    publisher/search-console adapters and credential checks
internal/email/          provider client, webhook verification
internal/agents/         specialist prompt registry
internal/aiprovider/     OpenRouter adapter and offline stub
internal/secretcipher/   AES-GCM encryption for stored credentials
migrations/           embedded SQL files
```

Dependencies point inward: handlers depend on small interfaces (`HealthChecker`, `WorkspaceStore`, `AnalyticsStore`, `OperatorProvider`), so tests use fakes instead of a database. Optional interfaces keep features such as delivery totals or analytics from forcing every fake to grow.

### Request flow and auth

The operator reaches Next.js, not the API. Next middleware enforces Basic auth and rejects cross-origin writes; the server-side `/api/v1` proxy attaches `API_KEY` and forwards the request. The API's own auth middleware accepts `/health` and `/ready` without a key and requires the Bearer key for everything else when configured. Provider webhooks and website intake use their own per-workspace secrets instead of the operator key.

A production build refuses to serve while `WEB_AUTH_DISABLED=true`. The bypass is development-only by design.

### Analytics

`GET /api/v1/analytics/summary` computes the dashboard in three aggregate queries: one per-workspace rollup, one CRM status breakdown, and one engine status breakdown. The browser previously fetched every workspace's campaigns, leads, engine leads, and delivery log and counted them client-side; the endpoint keeps the response small and its cost flat as tables grow. Figures are current-state counts, not historical trends.

### Health semantics

| Endpoint | Status | Purpose |
| --- | --- | --- |
| `GET /api/v1/health` | 200 while the process is up | Dashboard details; body carries `status` and database state |
| `GET /api/v1/ready` | 200 only when healthy, otherwise 503 | Deploy/load-balancer check |

The database check is a bounded `Ping`; failures are reported with fixed strings and never leak connection details.

### Migrations and data isolation

Migrations are embedded, validated, ordered, and applied one transaction at a time under a PostgreSQL advisory lock. They are forward-only. Every table owned by a workspace carries `workspace_id`, and store methods filter on it in SQL, so a mismatched ID cannot read another workspace's data.

### Background work

The publishing worker drains due and retryable jobs with bounded exponential backoff. The email worker sends queued lead replies and alerts through each workspace's own provider connection and records provider webhooks. The lead-engine scheduler discovers and dispatches for enabled workspaces on an interval; `LEAD_ENGINE_AUTOMATION=off` disables it. All loops start only when a database is configured.

## Dashboard (`apps/web`)

```
src/app/              routes: overview, workspaces, content, calendar, integrations, analytics, settings
src/components/       app shell, panels, workspace/content/integration/email/lead-engine UI
src/lib/              typed API clients, validation, and pure logic + Vitest tests
src/app/api/v1/       server-only proxy to the Go API
src/middleware.ts     operator auth and origin checks
```

Responses are validated before display; malformed payloads become an explicit unavailable/error state rather than zeros. Analytics and calendar expose refresh, last-updated time, and partial-failure warnings. The calendar filters by workspace, highlights overdue follow-ups, and exports an all-day `.ics` locally.

The two largest workspace components are still large (lead engine and draft editor); extracting shared data-fetching and UI primitives is a maintainability follow-up, not a functional gap.

## Security posture

- Basic auth is a single shared operator login, not per-user authorization. Team roles are recorded; the last active admin is protected, but roles do not yet gate API actions.
- The API key stays server-side. Browser code never receives it.
- CORS allows only configured origins; responses carry `nosniff` and `no-referrer`.
- Secrets are encrypted at rest with AES-GCM; logs and health output redact credentials.
- Rate limits for AI, intake, and email test/send paths are in-memory per API process, not distributed edge limits.
- The optional Cloudflare Worker is a proxy only. It must not be treated as an authentication layer.
