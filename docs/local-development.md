# Local development

## Prerequisites

| Tool | Version | Notes |
| --- | --- | --- |
| Go | 1.23 or newer | Builds and tests the API |
| Node.js | 20.9 or newer | Includes npm; runs the dashboard |
| PostgreSQL | 14 or newer | Local install, or use the Docker setup below |
| Docker with Compose | Any recent version | Optional; only for `make db-up` |
| make | Any | Optional; every target is a thin wrapper you can run by hand |

## Configuration

Three example files hold placeholders only. Copy each and edit the copy. Real `.env` files are git-ignored.

| Copy | To | Purpose |
| --- | --- | --- |
| `apps/api/.env.example` | `apps/api/.env` | API settings and `DATABASE_URL` |
| `apps/web/.env.example` | `apps/web/.env.local` | `NEXT_PUBLIC_API_BASE_URL` (public; never put secrets here) |
| `infra/.env.example` | `infra/.env` | Credentials for the optional Docker database |

### API variables

| Variable | Default | Rules |
| --- | --- | --- |
| `APP_ENV` | `development` | `development`, `test`, or `production` |
| `API_PORT` | `8080` | Integer 1 to 65535 |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |
| `DATABASE_URL` | empty | `postgres://` URL form. Empty means the API runs without a database. Required in production |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:3000` | Comma-separated origins, no wildcards. Required in production |

The API refuses to start if any value is invalid and lists every problem at once. Error messages never repeat the value you supplied, so a bad `DATABASE_URL` does not leak its password.

## Database

Option A: Docker.

```bash
cp infra/.env.example infra/.env     # choose a user, password, and database name
make db-up
```

Then set `DATABASE_URL` in `apps/api/.env` to match, for example `postgres://myuser:mypassword@localhost:5432/mydb?sslmode=disable`.

Option B: an existing local PostgreSQL. Create an empty database and point `DATABASE_URL` at it.

## Migrations

Migrations are plain SQL files in `apps/api/migrations/`, named `NNNN_description.sql` (four digits, starting at `0001`, no gaps). They are embedded into the binary, applied in order, each in its own transaction, under a PostgreSQL advisory lock. The mechanism is forward-only: to undo a change, add a new migration.

```bash
make migrate-status    # list migrations and whether each is applied
make migrate-up        # apply pending migrations
```

Without `make`: `cd apps/api && set -a && . ./.env && set +a && go run ./cmd/migrate up`.

## Run the services

```bash
make api-tidy          # first run only: resolves dependencies and creates go.sum
make api-run           # API on http://localhost:8080
make web-install       # first run only: installs packages and creates package-lock.json
make web-dev           # dashboard on http://localhost:3000
```

Commit `go.sum` and `package-lock.json` after the first run.

Check the API directly:

```bash
curl -s localhost:8080/api/v1/health    # always 200 while the process is up; body shows dependency state
curl -si localhost:8080/api/v1/ready    # 200 only when the database answers, otherwise 503
```

`/api/v1/health` reports the database as `ok`, `unavailable`, or `unconfigured`, with fixed messages that never include hostnames or credentials.

## Tests, linting, and formatting

```bash
make check             # everything below, in order
```

| Command | What it does |
| --- | --- |
| `make api-fmt` / `make api-fmt-check` | Format Go code / fail if unformatted |
| `make api-vet` | `go vet ./...` |
| `make api-test` | `go test ./...` (no database needed) |
| `make web-fmt` / `make web-fmt-check` | Prettier write / check |
| `make web-lint` | ESLint (Next.js core-web-vitals and TypeScript rules) |
| `make web-typecheck` | `tsc --noEmit` |
| `make web-test` | Vitest unit tests |

On a fresh checkout, run `make api-fmt` and `make web-fmt` once, then commit, so the check targets start green.

## Troubleshooting

- **Dashboard says "API unavailable":** start the API, and confirm `NEXT_PUBLIC_API_BASE_URL` matches its port. The browser origin must be listed in `CORS_ALLOWED_ORIGINS`.
- **Database shows "Not configured":** `DATABASE_URL` is empty in `apps/api/.env`.
- **Database shows "Unavailable":** PostgreSQL is not running or the URL's host, port, user, or password is wrong. The API logs only that the ping failed, by design.
- **`docker compose` says a variable is required:** create `infra/.env` from the example.
