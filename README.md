# Marketing Operations Platform

A private internal tool for one operator to plan, approve, publish, and measure marketing work for multiple authorized client businesses. It is not a public SaaS product: there is no billing and no client portal.

**Project memory, milestone roadmap, and current status live in [`remember.md`](remember.md).** Read it first. This repository is at the end of **Milestone 0** (engineering foundation).

## Layout

| Path | What it is |
| --- | --- |
| `apps/web/` | Next.js + TypeScript + Tailwind dashboard |
| `apps/api/` | Go HTTP API, PostgreSQL access, migrations |
| `docs/` | [Architecture](docs/architecture.md), [local development](docs/local-development.md), [decision log](docs/decisions.md) |
| `infra/` | Docker Compose for a local PostgreSQL only |
| `remember.md` | Source of truth for scope, milestones, and status |

## Quick start

Prerequisites: Go 1.23+, Node.js 20.9+ with npm, and PostgreSQL 14+ (or Docker). Full details are in [docs/local-development.md](docs/local-development.md).

```bash
# 1. Configuration (placeholders only in the .example files)
cp apps/api/.env.example apps/api/.env          # then edit DATABASE_URL
cp apps/web/.env.example apps/web/.env.local
cp infra/.env.example infra/.env                # only if using the Docker database

# 2. Database (optional: the API runs without one and reports it as unconfigured)
make db-up
make migrate-up

# 3. API and dashboard, in two terminals
make api-tidy          # first run only: creates go.sum
make api-run           # http://localhost:8080/api/v1/health
make web-install       # first run only: creates package-lock.json
make web-dev           # http://localhost:3000

# 4. Checks
make check
```

Run `make help` to list every command.

## What Milestone 0 includes

- Dashboard shell with navigation placeholders (Workspaces, Content, Calendar, Integrations, Analytics, Settings), empty states, and a live API and database status panel.
- Go API with validated configuration, `GET /api/v1/health`, `GET /api/v1/ready`, graceful shutdown, and structured logs that never contain credentials.
- PostgreSQL connection, a forward-only migration runner, and one small baseline migration.

## What it deliberately does not include

No client data, credentials, integrations, publishing, Sanity access, AI calls, analytics, or authentication yet. Nothing on the dashboard is sample data. See the roadmap in `remember.md` for when each arrives.
