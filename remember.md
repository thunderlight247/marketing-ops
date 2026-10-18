# Marketing Operations Platform --- Project Memory and Milestones

**Project status:** Milestone 0 COMPLETE (verified, committed); Milestone 1 first increment in progress\
**Current milestone:** Milestone 1 --- Domain model and database foundation (decisions settled; schema + store landed)\
**Project type:** Private internal tool for managing marketing work
across client businesses\
**Primary first integration:** Nutrition Guide (`nutritionguide.xyz`), a
Next.js website using Sanity for blog content\
**Owner/operator:** The founder manages only businesses and accounts for
which they have authorized access.

------------------------------------------------------------------------

## 1. Project purpose

Build one internal platform that helps the operator plan, create,
approve, publish, and measure marketing content for multiple authorized
client businesses from one dashboard.

The platform should reduce repetitive work without assuming every client
website or social platform uses the same technology. It should detect
and record the capabilities available for each connected integration and
use reusable adapters rather than creating a one-off publishing
implementation for every client.

This is an internal operations tool, not a public SaaS product. A client
portal is not part of the initial scope.

## 2. Core product capabilities

1.  **Client workspaces:** Separate each business's profile, brand
    guidelines, integrations, content, campaigns, publishing history,
    and reports.
2.  **Integration management:** Connect supported websites, CMSs,
    Git-based publishing workflows, and social platforms through
    authorized credentials.
3.  **Capability detection:** Record whether an integration supports
    reading content, creating drafts, publishing, scheduling,
    deleting/updating, analytics, or lead tracking. Never claim a
    capability is supported until the adapter verifies it.
4.  **AI-assisted content:** Draft SEO-oriented articles, social posts,
    titles, descriptions, calls to action, and campaign ideas using each
    client's business context and brand rules.
5.  **Human approval:** Generated content remains a draft until an
    authorized operator explicitly approves it for publishing. No silent
    auto-publishing in the initial release.
6.  **Publishing:** Publish approved content through the appropriate
    adapter; record status, destination URL/ID, timestamps, errors, and
    retries.
7.  **Scheduling:** Queue approved content for future publication, with
    safe retries and duplicate prevention.
8.  **SEO and performance:** Track available search, website, social,
    content, and enquiry metrics. Clearly label unavailable or estimated
    metrics.
9.  **Lead/enquiry tracking:** Capture enquiries only through configured
    and authorized integrations; record source and status without
    inventing attribution.
10. **Reports:** Summarize work completed, content published,
    traffic/engagement metrics available, and leads recorded for a
    client and period.
11. **Audit and security:** Record important actions, protect
    credentials, isolate client data, and provide understandable
    integration/error states.

## 3. Agreed technical direction

-   **Frontend:** Next.js, TypeScript, Tailwind CSS.
-   **Backend:** Go, with clear separation between transport/handlers,
    application services, domain logic, integration adapters, and
    persistence.
-   **Database:** PostgreSQL.
-   **Asynchronous work:** A background job/worker mechanism for
    scheduled publishing, retries, and longer-running tasks. Choose a
    maintainable implementation during the architecture phase; do not
    add infrastructure without a demonstrated need.
-   **AI:** Provider interface so a model provider can be changed
    without rewriting content workflows. Keep model keys server-side.
-   **Integrations:** Adapter/interface pattern with capability
    discovery and explicit per-integration status.
-   **Deployment:** Local development first; production deployment
    decisions can be made after the core workflow is working.
-   **Quality:** Migrations, configuration validation, structured logs,
    tests for business-critical behavior, and documented local setup.

Do not introduce microservices, Kubernetes, or unnecessary
infrastructure for the first version. A modular Go backend and a single
web application are sufficient unless evidence later shows otherwise.

## 4. Architecture principles

-   **Client data isolation:** Every client-owned record must be scoped
    to a workspace. Enforce this in application queries and tests.
-   **Least privilege:** Request only the permissions needed for each
    integration.
-   **Secrets:** Never commit tokens or credentials; never send
    server-side secrets to the browser; redact them from logs. Use
    environment variables locally and a suitable secret store in
    deployment.
-   **Approval gate:** Publishing jobs must verify that the content
    version being published has been approved. Editing approved content
    invalidates that approval until re-approved.
-   **Idempotency:** Retried jobs must not accidentally publish
    duplicate posts or duplicate records.
-   **Observable failures:** Store meaningful failure state and safe
    diagnostics; provide retry controls where appropriate.
-   **Capability-aware UI:** Show supported, unsupported, unconfigured,
    and failed capabilities accurately.
-   **Adapter reuse:** Support different client stacks through
    interfaces/adapters, not by forcing all websites into one CMS.
-   **No destructive discovery:** Integration checks must be read-only
    unless the operator explicitly starts an authorized write test.
-   **Human control:** Do not publish automatically without the
    operator's approval in the initial release.

## 5. Milestone roadmap

Milestones are ordered deliverables, not promises of a particular
calendar duration. Complete each milestone with working code, tests
appropriate to its scope, and an update to this file. Do not skip ahead
by implementing every integration before the core workflow is reliable.

### Milestone 0 --- Project setup and engineering foundation

**Status: IMPLEMENTED, VERIFICATION PENDING** (see section 9 for exactly what
was and was not verified)

Deliver: - Create repository and documented directory structure. -
Initialize Next.js + TypeScript + Tailwind frontend. - Initialize Go
backend module with health endpoint and configuration
loading/validation. - Set up PostgreSQL connection and first migration
mechanism. - Add `.env.example`, `.gitignore`, formatter/linter
conventions, and basic tests. - Add local development instructions and a
simple health/status view. - Add a project README linking to this
milestone file.

Acceptance: - A developer can follow the README to run the app
locally. - Frontend and backend start successfully. - Backend health
endpoint responds. - Database connection can be checked without exposing
credentials. - No real client credentials are committed.

### Milestone 1 --- Domain model and database foundation

Deliver: - Define core entities: Workspace/Client, User/Operator
(initially single-operator-friendly but with sound authorization
boundaries), Integration, IntegrationCapability, BrandProfile,
ContentItem, ContentVersion, Approval, PublishingJob, PublishedArtifact,
AuditEvent. - Add PostgreSQL migrations and repository/data-access
layer. - Define ownership relationships and workspace scoping. - Add
seed/demo data that contains no real client secrets.

Acceptance: - Migrations run from a clean database and are documented. -
Domain and database rules are tested. - Cross-workspace access tests
show one workspace cannot access another workspace's records.

### Milestone 2 --- Initial dashboard and workspace management

Deliver: - Responsive dashboard shell and navigation. -
List/create/view/edit client workspaces. - Client profile with business
description, website, audience, services, tone, brand rules, prohibited
claims, and calls to action. - Workspace-specific integrations and
content sections.

Acceptance: - Operator can create and manage multiple workspaces. - Each
view is clearly scoped to the selected workspace. - Empty, loading,
success, and error states exist.

### Milestone 3 --- Integration registry and capability checks

Deliver: - Integration interface and registry. - Connection
configuration/status lifecycle: unconfigured, connected, limited, error,
disconnected. - Capability model: read content, create draft, publish,
update, schedule, analytics, lead/enquiry intake. - Read-only
test/health checks by default. - Secure credential handling and redacted
diagnostic messages.

Acceptance: - UI can show what each integration supports and why a
capability is unavailable. - Failed checks do not damage client sites or
expose secrets. - A new adapter can be added without changing core
content workflow code.

### Milestone 4 --- Content planning and AI-assisted drafting

Deliver: - Content calendar/planning records and content briefs. - AI
provider abstraction and server-side model invocation. - Generate blog
articles and social posts from workspace context. - Editable title,
slug, excerpt, body, metadata, channel-specific copy, CTA, and suggested
schedule. - Store prompt/configuration metadata needed for debugging
without unnecessarily retaining sensitive input.

Acceptance: - Drafts are stored before any publishing action. -
Generated content is editable and associated with the correct
workspace. - Model failure, timeout, or malformed output is handled
safely. - No AI output is published automatically.

### Milestone 5 --- Review, versioning, and approval workflow

Deliver: -
Draft/review/approved/rejected/queued/publishing/published/failed
lifecycle, with transitions defined centrally. - Content version history
and approval records. - Explicit approval action and approval
invalidation when approved content changes. - Preview of content before
approval. - Audit events for approvals and publishing actions.

Acceptance: - A publishing job cannot publish unapproved or changed
content. - The operator can compare versions and understand current
status. - Approval decisions are traceable.

### Milestone 6 --- First real publishing adapter: Sanity

**Pilot integration:** Nutrition Guide's existing Next.js + Sanity blog.

Deliver: - Sanity adapter using the supported Sanity APIs and
least-privilege server-side credentials. - Connection check and
capability reporting. - Create or update a draft, publish only after
approval, and retrieve the resulting document/slug/URL. - Handle image
upload if required and supported by the chosen implementation. - Safe
retries, duplicate prevention, and useful error handling. - Verification
step that checks the resulting public article URL when possible.

Acceptance: - Existing website content and routes continue to work. - A
test draft can be created without appearing publicly before approval. -
An approved article can be published once, with the destination
recorded. - Credentials remain server-side and are not logged. - Test
first with a clearly identified test article or other safe test content
agreed by the operator; do not alter/delete existing client content
without explicit permission.

### Milestone 7 --- Reusable website publishing adapters

Deliver adapters based on actual client needs, prioritizing: -
Sanity/CMS API adapter pattern. - Git-based publishing adapter for
repositories where content is committed as files and deployed through an
authorized workflow. - Generic/custom API adapter where a client
provides an authorized content API. - Clear manual/export workflow when
safe automation is not possible. - Adapter documentation and contract
tests.

Acceptance: - Core content/approval workflows do not depend on a
particular CMS. - Unsupported sites are identified clearly instead of
receiving a false promise of automatic publishing. - Every adapter
declares its supported capabilities and limitations.

### Milestone 8 --- Social publishing integrations

Deliver: - Integration framework for selected social platforms, based on
official APIs and platform permissions. - Channel-specific post previews
and formatting. - Approval, scheduling, publication status, post
IDs/URLs, and retry handling. - Clear handling of expired/revoked
permissions and platform limitations.

Acceptance: - Only platforms with valid authorized access can be
connected. - No password scraping or unofficial automation that violates
platform rules. - Unsupported actions are disclosed and may fall back to
a manual checklist/export.

### Milestone 9 --- Scheduler, workers, retries, and operations

Deliver: - Background worker and persistent job queue. - Schedule
approved content using timezone-aware timestamps. - Retry policy with
backoff, idempotency keys, lock/concurrency handling, and terminal
failure state. - Operator controls to cancel or retry eligible jobs. -
Operational logs and job history.

Acceptance: - Restarting the app does not silently lose scheduled
jobs. - Duplicate job execution does not create duplicate posts. -
Timezones and daylight-saving behavior are handled explicitly where
relevant. - A failed job is visible and diagnosable without exposing
secrets.

### Milestone 10 --- SEO and website analytics

Deliver: - Integrations for available data sources such as Google Search
Console and analytics providers, where the operator has authorization. -
Track clicks, impressions, queries, landing pages, traffic and content
metrics that the connected source actually provides. - SEO metadata
checks for generated articles. - Baseline and date-range comparisons.

Acceptance: - Metrics show source, time period, and last successful
sync. - Missing permissions/data are reported rather than fabricated. -
SEO recommendations are distinguishable from measured results.

### Milestone 11 --- Lead/enquiry tracking and campaign management

Deliver: - Campaigns linked to content and publishing channels. -
Capture enquiries through configured forms/APIs or authorized imports. -
Lead status, source, notes, and follow-up date. - UTM conventions and
attribution only where tracking data supports it. - Respect data
minimization and client-specific access.

Acceptance: - Lead records have a traceable source or are marked
unknown. - Data is workspace-isolated. - No claim of perfect attribution
where the underlying tracking cannot support it.

### Milestone 12 --- Reporting and content performance

Deliver: - Workspace and date-range reports. - Published content,
planned vs completed items, available analytics, leads, and integration
health. - Export/shareable reports for the operator to send to
clients. - Clear definitions for metrics and caveats.

Acceptance: - Reports only include data actually available. - Report
calculations are tested and reproducible. - Client-specific data cannot
leak into another workspace's report.

### Milestone 13 --- Security, reliability, and UX hardening

Deliver: - Review authentication and authorization for the actual
deployment model. - Rate limits and validation for sensitive
endpoints. - Secure secret storage, rotation/revocation instructions,
log redaction, backups, restore procedure, and data-retention
decisions. - Accessibility, mobile responsiveness, loading/error states,
and audit review. - Dependency and configuration review.

Acceptance: - Security-sensitive flows have tests and documented
operational procedures. - Backups can be restored in a tested
environment. - No known cross-workspace data leakage in the tested
paths. - Operator can disconnect an integration and revoke its
credentials.

### Milestone 14 --- Deployment, pilot, and release

Deliver: - Deployment configuration and environment documentation. -
Production database migration and backup plan. - Monitoring/alerts for
failed jobs and integrations. - Pilot with Nutrition Guide and a small
number of other authorized clients. - Feedback-driven fixes and final
operator guide.

Acceptance: - Production deployment is repeatable. - Pilot workflows
work end-to-end from content brief through approval, publication,
verification, and reporting. - Recovery and rollback steps are
documented. - Only then consider additional advanced automation.

## 6. Out of scope until the core system works

-   Public SaaS subscriptions and billing.
-   Client-facing self-service portal.
-   Automatic publishing of unreviewed AI content.
-   Unsupported unofficial social media automation.
-   Complex multi-agent workflows, microservices, Kubernetes, or
    expensive infrastructure without a proven need.
-   Claims that every website/platform can be fully automated.

## 7. Initial end-to-end success scenario

1.  Operator opens the dashboard and selects Nutrition Guide.
2.  The system uses its saved workspace profile and authorized
    integration.
3.  Operator creates a content brief or asks AI to draft an article.
4.  The article is stored as a draft and reviewed/edited.
5.  Operator approves the exact version.
6.  The Sanity adapter publishes the approved version.
7.  The system records the Sanity document ID, public URL, time, and
    result.
8.  The operator verifies the page and later sees available performance
    data.

This scenario is the first practical definition of a successful release.
Build the general foundation first, then implement this pilot without
hard-coding Nutrition Guide into the platform.

## 8. How any AI assistant/developer should continue

At the beginning of a session: 1. Read this file and `README.md`. 2.
Inspect the actual repository and current Git status before changing
files. 3. Check completed milestones against code, migrations, tests,
and working behavior; do not trust status labels alone. 4. Identify the
current milestone and its acceptance criteria. 5. Work on the smallest
useful next step. Do not repeat discovery questions already answered
here. 6. Do not rewrite working architecture or make unrelated changes.
7. Never expose or request secrets in chat. 8. Run relevant tests and
report the exact commands and results. 9. Update the milestone status,
decisions, files changed, tests, and next action in this file. 10. Ask
the operator only when a decision genuinely blocks progress or would
create a consequential external/production change.

## 9. Current state and next action

### 9.1 Project history before Milestone 0

**Completed** - Product purpose and internal-only scope established. -
Initial stack selected: Next.js/TypeScript/Tailwind, Go, PostgreSQL. -
Reusable integration/adapter direction established. - Nutrition Guide
identified as the first pilot: Next.js frontend with Sanity blog. -
Read-only local Sanity query succeeded and found one post: - Title:
`5 Simple Ways to Enjoy Ghanaian Foods While Losing Weight` - Slug:
`5-simple-ways-to-enjoy-ghanaian-foods-while-losing-weight` - This
confirms read access from the local project; it does not prove
write/publish permissions or production deployment behavior. - No
marketing-platform repository has been created yet. - No
marketing-platform code has been written. - No changes have been made to
the Nutrition Guide production site as part of this investigation.

### 9.2 Milestone 0 --- what was built

The repository foundation exists in the project root (monorepo). Files created:

- Root: `README.md`, `remember.md`, `Makefile`, `.gitignore`, `.editorconfig`
- `infra/`: `docker-compose.yml` (local PostgreSQL only), `.env.example`
- `docs/`: `architecture.md`, `local-development.md`, `decisions.md`
- `apps/api/` (Go 1.23, module `marketingops/api`, only dependency `pgx/v5`):
  - `cmd/api/main.go` (server, graceful shutdown), `cmd/migrate/main.go` (`up`, `status`)
  - `internal/config` (validated env config, credential-safe output) + tests
  - `internal/health` (status report logic) + tests
  - `internal/httpapi` (routes `GET /api/v1/health`, `GET /api/v1/ready`; recover/log/security-header/CORS middleware) + tests
  - `internal/database` (lazy pgx pool), `internal/migrate` (forward-only runner) + loader tests
  - `migrations/0001_app_metadata.sql` (baseline only), `migrations/embed.go`, `.env.example`
- `apps/web/` (Next.js 15 App Router, React 19, TypeScript, Tailwind v4):
  - Overview page with live API/database status panel; placeholder routes with empty states for Workspaces, Content, Calendar, Integrations, Analytics, Settings
  - `src/lib/{config,health,navigation}.ts` with Vitest tests
  - ESLint, Prettier, Vitest, TypeScript config; `.env.example`

Key decisions are in `docs/decisions.md` (D1 to D15). Notable: `/health` always returns
200 with a status body and `/ready` returns 503 when degraded; `DATABASE_URL` is optional
outside production; migrations are forward-only and embedded; no animation libraries and
no authentication in this milestone.

### 9.3 Milestone 0 --- verification status (honest record)

**VERIFIED COMPLETE on 2026-10-18 on the operator's machine (Go toolchain, Node,
network, Docker all available).** No compile or test fixes were needed.

Verification results:

- `make api-tidy` produced `go.sum`; `api-fmt-check` reports no unformatted files.
- `make api-vet` and `make api-test`: pass (config, health, httpapi, migrate tests).
- `make web-fmt-check`, `web-lint`, `web-typecheck`: pass. Real Vitest run: 26 tests
  in 3 files (`config`, `health`, `navigation`) pass.
- `make web-build` (`next build` 15.5.28): succeeds; all 8 routes + not-found build.
- `make db-up`: PostgreSQL 16 container healthy. `make migrate-status` shows
  `0001 app_metadata` applied; `make migrate-up` reports nothing pending. The
  migration runner has connected to PostgreSQL.
- Live API check (run on `API_PORT=8081`, see note below): `GET /api/v1/health` →
  200 with `status: ok`, database `connected`, latencyMs 14; `GET /api/v1/ready` →
  200. Structured request logs contain no credentials.
- Degraded-mode check: starting the API with a wrong `DATABASE_URL` password gives
  `GET /api/v1/ready` → 503 with `status: degraded`, database `unavailable`, and no
  credentials in the response (lint-style redaction claim confirmed at runtime).
- Initial git repository created (`git init -b main`) and root commit `53f3823`
  "Milestone 0: project foundation and verification" made with `go.sum` and
  `package-lock.json` committed; `apps/api/.env`, `apps/web/.env.local`,
  `infra/.env` confirmed untracked.

Note: **port 8080 is occupied on this machine by an unrelated server**
(`/app/schoolhub-server`, root-owned), so the API was verified on `API_PORT=8081`
instead. `make api-run` on this machine will fail to bind 8080 until either that
other server is stopped or `API_PORT` in `apps/api/.env` is changed; the code is
fine, this is purely a local-port conflict.

### 9.4 Verification commands run (2026-10-18)

```bash
make db-up && make migrate-status && make migrate-up   # healthy; 0001 applied
API_PORT=8081 make api-run        # health 200, ready 200 (8080 taken by other app)
API_PORT=8082 (bad DATABASE_URL)  # ready -> 503 degraded, no secrets in body/logs
make web-build                    # next build succeeds, 10/10 pages
make check                        # all format/lint/type/test checks green
```

### 9.5 Milestone 0 acceptance checklist

| Criterion | Status |
| --- | --- |
| README lets a developer run the app locally | Done (docs exist; flow executed via ARLS migration) |
| Frontend and backend start successfully | API verified live on :8081; `next build` + dev routes build; dashboard page itself not manually clicked through in a browser |
| Backend health endpoint responds | Verified live: `/health` 200, `/ready` 200 when DB up, 503 degraded when DB unreachable |
| DB connection checkable without exposing credentials | Verified: migration applied against PostgreSQL; degraded response and JSON logs contain no credentials |
| No real client credentials committed | Verified: `.env`/.env.local/infra .env untracked; only `.env.example` files committed |
| Tests and format/lint documented | Done: gofmt clean, vet + Go tests pass, Vitest 26/26, lint/typecheck/prettier pass |
| Git repository | Done: initialized, root commit `53f3823` |

### 9.6 Current milestone and next action

**Current milestone:** Milestone 1 --- domain model and database foundation.

**Deferred decisions settled (2026-10-18, recorded as D16/D17 in `docs/decisions.md`):**

1. Operator authentication: none for now; middleware stays the single choke point so a
   login layer can wrap requests later without touching handlers (revisit in Milestone 13).
2. Client integration credentials: encrypted at rest in PostgreSQL (AES-GCM ciphertext
   without the nonce, key from env, `key_version` column for rotation). Plaintext is the
   failure mode; the key never enters the database.

**Milestone 1 first increment (implemented 2026-10-18):**

- `apps/api/migrations/0002_domain.sql`: full core schema --- operators, workspaces,
  brand_profiles, integrations, integration_capabilities, integration_credential_sets
  (ciphertext bytes + key_version), content_items, content_versions, approvals,
  publishing_jobs (idempotency_key unique), published_artifacts, audit_events. Status and
  kind columns are text with CHECK constraints; every client-owned table carries
  `workspace_id` with a foreign key and index.
- `apps/api/migrations/0003_seed_demo.sql`: two clearly-labelled demo workspaces
  (`demo-studio`, `demo-cafe`) with sample brand profiles; no real client names, no secrets.
- `apps/api/internal/store`: repository foundation (pgx pool wrapper). Workspace-scoped
  content queries filter on `workspace_id` in SQL. Sentinel errors `ErrNotFound` and
  `ErrSlugTaken` returned to callers instead of driver errors.
- `apps/api/internal/store/store_test.go`: integration tests that self-apply migrations and
  skip when no database is configured. Tests cover workspace lifecycle plus the
  **cross-workspace isolation acceptance test**: workspace B cannot list or read workspace
  A's content even with a valid item id; deleting a workspace cascades its content away.
  Also verifies new content items must start as drafts.
- `Makefile`: `api-test` now sources `apps/api/.env`, so `make check` runs the integration
  tests against the local Docker PostgreSQL when it is up.

**Milestone 1 second increment (implemented 2026-10-18) --- the remaining repositories:**

- `internal/store/brands.go`: workspace-scoped brand profile get/upsert.
- `internal/store/integrations.go`: scoped integration create/list/get, lifecycle status
  updates, deletes; capability upsert/list (validates value pairs in code before SQL);
  encrypted credential set put/get/delete (ciphertext + key_version only, never plaintext).
- `internal/store/workflow.go`: content versions inserted as numbered snapshots per item
  (order and reads joined through `content_items` for scoping); record approval
  only when item+version+workspace all match (cross-workspace or cross-item approval is
  impossible at the SQL level); secret-free audit events with per-workspace listing.
- `internal/store/publishing.go`: publishing job creation guarded by joins through the
  owning item + version + workspace integration (so unapproved/mismatched or
  cross-workspace job creation cannot happen at this layer), idempotency key lookup,
  published artifact recording (waiting Milestone 9 for the workflow that drives them).
- `internal/secretcipher`: AES-256-GCM encrypt/decrypt with hex-encoded 32-byte key from
  env (D17), nonce-per-message, errors deliberately non-distinguishing, key loading
  validated in unit tests (no database needed).
- `internal/store` now has 8 integration tests: workspace lifecycle, cross-workspace
  isolation (content), integrations scoped to workspace, capabilities + credential
  roundtrip, version numbering, approval guards, audit scoping, and publishing job
  scoping/idempotency/artifacts. All pass against the Docker database.

**Verified:** `make migrate-up` applied 0002 + 0003; both increments confirmed by
`make check` exit 0 (all Go packages including store integration tests and secretcipher
unit tests; all web checks 26/26). Tests still skip cleanly without DATABASE_URL.

**Next action:** Milestone 1 is essentially delivered. Remaining M1 polish before M2:
expose a small operator read API for workspaces on the Go side (list/get endpoints on
`/api/v1/workspaces`, header-only CORS from the dashboard) so Milestone 2 dashboard UI
has real data. Then Milestone 2 --- workspace management in the dashboard.

Local-environment note carried forward: port 8080 on this machine is occupied by an
unrelated server, so local API runs use `API_PORT=8081` in the untracked local `.env`;
the committed `.env.example` keeps 8080 as the default.
