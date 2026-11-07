# Marketing Operations Platform

A private, multi-workspace operations tool for planning, approving, publishing, and measuring marketing work for client businesses. It is an internal single-operator product: no billing, no client portal, and no public sign-up.

**Current status, roadmap, and operational notes live in [`remember.md`](remember.md).** This README describes the checked-in system; where they disagree, `remember.md` is the source of truth.

## What is implemented

| Area | Working behavior |
| --- | --- |
| Workspaces | Per-client records, brand context, archive/delete, strict workspace-scoped queries |
| Content | Drafts, version history, approval decisions, audit trail, AI drafting and specialist agents |
| Publishing | Scheduled publishing worker, per-workspace Sanity connection, retries, publish history |
| Campaigns & leads | Campaigns, CRM leads, lead sources, follow-up dates, website intake with signed per-connection secrets |
| Outbound email | Per-workspace Resend connection, queued sends, retries, delivery webhooks, failure/bounce attention |
| Lead engine | Google Places discovery, WhatsApp outreach (QR scan or phone pairing code), blacklist, per-workspace limits/delays, follow-up rounds |
| Calendar | Monthly view, local-day semantics, workspace filter, overdue follow-ups, `.ics` export |
| Analytics | One SQL-backed aggregate endpoint: CRM/engine counts, status breakdowns, per-workspace email totals |
| Settings | System-wide health, database state, team records and roles, service/runbook notes; workspace-specific settings live inside each workspace |

Two background loops run inside the API process: the publishing worker and the email worker. The lead-engine scheduler is also started there unless `LEAD_ENGINE_AUTOMATION=off`. The WhatsApp dispatcher is a separate Node service.

## Layout

| Path | What it is |
| --- | --- |
| `apps/web/` | Next.js 15 + TypeScript + Tailwind dashboard, with a same-origin API proxy |
| `apps/api/` | Go HTTP API, PostgreSQL access, migrations, workers, scheduler |
| `services/whatsapp-dispatcher/` | Node/WhatsApp transport used by the lead engine |
| `infra/` | Local PostgreSQL, Caddy/nginx examples, Cloudflare Worker entrypoint |
| `docs/` | Architecture, local development, decisions, stable entrypoint |
| `remember.md` | Scope, milestones, decisions and live operational state |

## Quick start

Prerequisites: Go 1.23+, Node.js 20.9+ with npm, and PostgreSQL 14+ (or Docker).

```bash
# 1. Configuration (examples contain placeholders only)
cp apps/api/.env.example apps/api/.env          # edit DATABASE_URL
cp apps/web/.env.example apps/web/.env.local
cp infra/.env.example infra/.env                # only for the Docker database

# 2. Database (optional; the API reports "unconfigured" without one)
make db-up
make migrate-up

# 3. API + dashboard
make dev                                        # picks free ports and disables login locally

# 4. Checks
make check
```

Individually: `make api-run`, `make web-dev`, `make migrate-status`, `make help`.

## Access and security

- The dashboard uses **HTTP Basic authentication** for a single operator. Set `OPERATOR_USERNAME`, `OPERATOR_PASSWORD`, `WEB_ORIGIN`, `API_BASE_URL`, and `API_KEY` in the web server's private environment. Use HTTPS.
- The Go API requires `Authorization: Bearer <API_KEY>` whenever `API_KEY` is set, and production startup refuses to run without one.
- `WEB_AUTH_DISABLED=true` is honored **only by a development server**. A production build returns 503 while that variable is set, because that is how a public URL silently loses its login.
- Team records have `admin`/`editor`/`viewer` labels, but they are **not yet enforced as authorization**: the shared API key is the actual access boundary. The final active admin cannot be demoted or deactivated.
- All client-owned queries are workspace-scoped in SQL. Integration and email credentials are encrypted at rest with `CREDENTIAL_ENCRYPTION_KEY`.
- The Cloudflare Worker in `infra/cloudflare-worker/` is only a pass-through proxy; it does not add authentication. Keep the origin and API private, and read [`docs/stable-entrypoint.md`](docs/stable-entrypoint.md) before exposing it.
- This is not a complete production security program: no per-user sessions, MFA, distributed rate limiting, or audit-backed authorization yet.

## Verification

```bash
make check       # gofmt, go vet, Go tests, Prettier check, ESLint, tsc, Vitest
make web-build   # production Next.js build
```

CI runs Go formatting/vet/tests against PostgreSQL, the Go build, and the web lint/typecheck/tests/build (`.github/workflows/ci.yml`). A separate workflow runs the campaigns/leads Playwright suite.

## Known limits

- Analytics is aggregate-only: no date ranges, trends, or traffic metrics yet.
- Calendar reminders are not pushed; the `.ics` export is the reminder mechanism.
- The lead engine and email path need real provider credentials to verify end-to-end delivery.
- No automated retention/pagination for every large table yet; list endpoints already cap their pages.
