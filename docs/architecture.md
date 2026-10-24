# Architecture (Milestone 0)

The platform is a modular monolith: one Go API, one Next.js dashboard, and one PostgreSQL database. There are no microservices, queues, or other infrastructure until a milestone demonstrates a need. The full product direction is in [`remember.md`](../remember.md).

```
Browser --> Next.js dashboard (apps/web) --HTTP/JSON--> Go API (apps/api) --pgx--> PostgreSQL
```

## API (`apps/api`)

```
cmd/api/              process entry point: config, logging, wiring, graceful shutdown
cmd/migrate/          migration CLI: up | status
internal/config/      environment loading and validation (no secrets in output)
internal/health/      status report logic, no HTTP code
internal/httpapi/     routes, thin handlers, middleware (recover, log, headers, CORS)
internal/database/    pgx connection pool
internal/migrate/     forward-only SQL migration runner
internal/store/       data-access layer; every client-owned query is workspace-scoped
migrations/           embedded SQL files
```

Dependencies point inward: `httpapi` depends on a small `HealthChecker` interface, `health` depends on a small `Pinger` interface, and neither knows about pgx. This keeps handlers thin and lets tests use fakes instead of a database.

Later milestones add `domain`, application services, repositories, integration adapters, and workers beside these packages without reshaping them.

### Health semantics

| Endpoint | Status code | Purpose |
| --- | --- | --- |
| `GET /api/v1/health` | 200 while the process is up | Dashboard status. The body carries `status` (`ok` or `degraded`) and a `database` object (`ok`, `unavailable`, or `unconfigured`) |
| `GET /api/v1/ready` | 200 only when `status` is `ok`, otherwise 503 | Deploy and load-balancer checks |

The database check is a `Ping` with a timeout. Failure messages are fixed strings. Driver errors are never returned or logged in full, because they can contain connection details; only the failure class (`timeout` or `error`) is logged.

### Startup and shutdown

The pool is created lazily, so the API starts even when PostgreSQL is down and reports that state. Invalid configuration stops startup with a list of every problem. `SIGINT` and `SIGTERM` trigger `http.Server.Shutdown` with a timeout, then the pool closes.

### Migrations

Files are embedded with `go:embed`, validated (name pattern, contiguous versions, non-empty), and applied in order. Each migration and its `schema_migrations` row commit in one transaction. A PostgreSQL advisory lock prevents concurrent runners. The name check and ordering logic are unit tested without a database.

## Dashboard (`apps/web`)

```
src/app/              dashboard routes for overview, workspaces, content, integrations, and placeholders
src/components/       app shell, status panel, workspace/content/integration panels, batch publisher
src/lib/              config, health, workspace, content, integration, navigation API clients + tests
```

The status panel calls the API from the browser, so a stopped API produces a clear "API unavailable" state instead of a broken page. Responses are validated before display; anything that is not a well-formed health report is treated as unavailable. The overview loads workspace records and marks seeded `demo-*` records as demos. Its batch publisher presents approved posts from all workspaces, confirms the selected list, then sends independent concurrent publish requests through each post's chosen registered publisher; outcomes are displayed per post, with no claim of cross-post atomicity. The shell is responsive (sidebar on wide screens, menu button on narrow ones) and includes a skip link, landmarks, `aria-current`, and visible focus rings.

### Publishing adapter boundary

The `integration.Publisher` contract accepts an immutable content-version payload and exposes draft creation, publication, and optional read-only URL verification. `integration.Registry` keeps destination capability adapters separate from publisher implementations: Sanity is registered for both, while generic REST and manual export currently have no automated publisher. The API decrypts credentials server-side and dispatches through the publisher registry; unknown/unimplemented destinations are rejected before creating a publishing job or performing an external write. Provider failures are exposed or persisted only when they implement the explicit safe-message interface. Adding another CMS requires a provider implementation and its contract tests; merely saving a base URL/token never claims verified publish support.

## Security posture in Milestone 0

- No secrets in the repository. `.env.example` files contain placeholders; real env files are git-ignored.
- Configuration errors, health responses, and logs never contain credentials.
- CORS allows only configured origins. Responses carry `nosniff` and `no-referrer` headers.
- The dashboard sends `noindex` metadata.
- There is no authentication yet because no client data exists. Deciding how the operator signs in is an open item for Milestone 1 and is revisited in Milestone 13.
