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
| `apps/web/.env.example` | `apps/web/.env.local` | Server-only API proxy and operator access settings |
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

One command for the whole local stack — API on http://localhost:8081 (database up, migrations applied) plus the dashboard on http://localhost:3000. Stop both with Ctrl-C.

```bash
make dev
```

Ports are a negotiation, not an assumption — other projects on the same machine may hold any port at any moment. The launcher (`apps/web/e2e/run-stack.sh`, used by both `make dev` and the Playwright config) does this: the dashboard takes the requested web port or the next free one of `{3000, 3001, 3002}`, and the API takes the requested port or the other of `{8080, 8081}` — but only the free one, or one already serving a ready marketing-ops API for the dashboard's origin. A server from another project keeping one of these ports therefore never breaks the run and is never silently paired into the dashboard; every picked port is printed. Overrides stay available, e.g. `make dev API_BASE=http://localhost:8081 WEB_PORT=3002`. The Playwright suite is driven by `PLAYWRIGHT_WEB_PORT` plus `PLAYWRIGHT_API_BASE_URL` (or `PLAYWRIGHT_API_PORT`) and honors those ports exactly, because some e2e specs read the API address from that environment variable themselves.

It reuses an already-running, ready API untouched when that API accepts the dashboard's origin, shuts only the API it started when the dashboard stops, and reports the exact ports it picked. The same script backs the Playwright config, so any e2e web server always has its paired API up behind it. The launcher binds the dashboard to loopback and explicitly disables web login for local use; do not expose this development server to the network.

### Operator access outside local development

The dashboard uses HTTP Basic authentication for a single operator and forwards requests through its server-side API proxy. Configure `OPERATOR_USERNAME`, a long unique `OPERATOR_PASSWORD`, exact external `WEB_ORIGIN` (for example `https://ops.example.com`, no trailing slash), `API_BASE_URL`, and `API_KEY` in the web server's private environment. `API_KEY` must match the API's key. The app fails closed if credentials or API proxy settings are missing. Never set `WEB_AUTH_DISABLED=true` for an accessible deployment. Use HTTPS for the dashboard and a private network or HTTPS for the API; Basic credentials are otherwise exposed in transit. Protect the API from direct public access, restrict access to the web service and secrets, and enforce login rate limiting at the deployment edge; the web login does not implement rate limiting itself. This is a single shared operator login, not user-level authorization or a complete production security audit. Basic authentication requires restarting the service to rotate credentials and browsers may retain login credentials until closed.

Existing `NEXT_PUBLIC_API_KEY` and `NEXT_PUBLIC_API_BASE_URL` settings are no longer used by the dashboard. Remove any previously exposed key and rotate it on both servers. For an individual local `npm run dev`, set `WEB_AUTH_DISABLED=true` and `API_BASE_URL` in `apps/web/.env.local` (and `API_KEY` if the API requires one).

Individually:

```bash
make api-tidy          # first run only: resolves dependencies and creates go.sum
make api-run           # API on http://localhost:8080 (or API_PORT in apps/api/.env)
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

## Website lead intake

Each workspace can create one or more **Website lead connections** under Campaigns & leads. This is a reusable workflow for any business with a website backend, not a pre-connected website or CRM integration:

1. Select the correct business workspace and create a connection named for its website or form. Create a separate connection for every website server.
2. Copy the one-time secret when it appears; only its hash is stored and it cannot be displayed again. Put it in that website server's secret store. If it is lost or compromised, revoke the connection and create another. Revocation does not delete existing leads.
3. Open **Setup instructions** on the connection and give the website developer its submission and verification paths, plus the externally reachable API origin. Transfer the secret through an appropriate secure channel, never in ordinary email, source code or browser code.
4. The website developer configures the server-side form handler to call the endpoint below. The website may use any framework; websites without a server need a separate server-side integration. Existing CRMs need a dedicated connector or import workflow.
5. Verify the server credentials using the read-only request below. Then send an authorized sample submission through the website's form and confirm that it arrives in the correct workspace. A successful credential check alone does not establish form delivery or verify leads. Agree on handling of sample records before testing.

A website's **server**, not browser JavaScript, sends an HTTPS POST directly to the API origin plus the path shown in the dashboard. Configure that origin to reach the Go API or an authenticated-capable reverse proxy that forwards this route and the `Authorization` header. The Next.js dashboard proxy is for operators and is not the public intake endpoint. Keep the connection secret in the website server's secret store, never in public assets, URLs, logs or a `NEXT_PUBLIC_` variable. Do not use the operator API key as the website secret.

### Read-only connection check

From the **website server**, POST to the connection's `/verify` path using the same secret; it creates no lead:

```bash
curl --request POST "$LEAD_VERIFY_URL" \
  --header "Authorization: Bearer $LEAD_INTAKE_SECRET"
```

HTTP 200 and `{"verified":true}` mean the connection secret and route were accepted. A 401 means the secret, workspace, connection or route is wrong or revoked; 429 means rate limited; 5xx means the API is unavailable. This check does not validate write delivery, website form handling, or a human enquiry.

### Submission request

Example server-side request (replace placeholders in your server environment; never put actual values into source control):

```bash
curl --request POST "$LEAD_INTAKE_URL" \
  --header "Authorization: Bearer $LEAD_INTAKE_SECRET" \
  --header 'Content-Type: application/json' \
  --data '{"submissionId":"example-unique-submission-1","contactName":"Example","contactEmail":"person@example.test","message":"Please contact me","utmSource":"website"}'
```

`submissionId` is required: generate a unique stable ID once per enquiry (letters, digits, `.`, `_`, `:`, `-`, maximum 128 characters) and reuse the **identical JSON bytes** on retries. A successful first submission returns 201 and `{id, duplicate:false}`; an identical retry returns 200 and the same ID with `duplicate:true`; reusing an ID with different bytes returns 409. At least one of `contactEmail` or `contactPhone` is required. Optional fields: `contactName`, `contactEmail`, `contactPhone`, `message`, `utmSource`, `utmMedium`, `utmCampaign`. Attribution values are recorded as provided, not verified or automatically linked to campaigns. A 401 indicates invalid or revoked connection credentials; 429 or 5xx should be retried with backoff. Never silently discard a failed submission: the website needs durable delivery/retry or a clear failure message to its user.

### Production deployment checklist

- Deploy the API and dashboard with the production environment and migration 0008 applied. Protect the dashboard/operator API behind operator authentication; expose only the two connection-specific `POST .../lead-connections/{connectionId}/verify` and `/submissions` routes through a TLS-terminating reverse proxy. Forward the website server's `Authorization` header unchanged; do not expose the other API routes publicly or route these requests through the operator-only Next.js proxy. Validate the reverse-proxy rules in the deployed environment.
- Use a publicly reachable HTTPS API origin if the website server is external; `localhost` means the website server itself and will not connect two separate servers. Provision and renew TLS certificates through your deployment environment. Test reachability from the website server.
- Keep the API, worker and PostgreSQL running; configure backups and monitoring for API availability, rate-limit responses, database health and failed website deliveries. This project does not yet provide a full production deployment, shared rate limiter or delivery alerting.
- Use a distinct secret per website; never put it in browser JavaScript, query strings, logs, source control or `NEXT_PUBLIC_*`. Revoke and replace it if exposed. Only the hash is stored on the platform.
- Each website must handle form consent and privacy notices, input validation, spam/bot protection, durable retries or visible failure to the submitter, and its own PII retention obligations. The platform receives the details the website sends; set a retention and deletion process before using real customer data.

The intake endpoint accepts server-to-server requests only by convention; the secret authenticates the calling website server, not the human submitting its form. Configure consent/privacy notices, validation, spam/bot protection and retention on the website and use HTTPS. The built-in per-IP 60/minute limit is in-memory and per API process, not distributed abuse protection; behind a reverse proxy clients may share one IP. Apply appropriate edge rate limits and monitoring before exposing it. No automatic customer discovery, CRM integration or delivery notifications are included. Configure and test the website's own form separately with authorized test data.

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

- **Dashboard says "API unavailable":** run `make dev` (starts both). Running the pieces separately: start the API, and confirm server-side `API_BASE_URL` matches its port and `API_KEY` matches the API when required.
- **A port is already in use by another program:** `make dev` falls back to the next free port in the same set ({3000, 3001, 3002} for the dashboard, {8080, 8081} for the API) and logs the port it picked. Free the wanted port `lsof -iTCP:<port> -sTCP:LISTEN` and restart, or target another one explicitly:

```bash
make dev API_BASE=http://localhost:8081 WEB_PORT=3001   # dashboard via Playwright launcher, dev mode
PLAYWRIGHT_WEB_PORT=3001 PLAYWRIGHT_API_PORT=8081 npm run test:e2e   # e2e suite on explicit ports (from apps/web/)
```
- **Database shows "Not configured":** `DATABASE_URL` is empty in `apps/api/.env`.
- **Database shows "Unavailable":** PostgreSQL is not running or the URL's host, port, user, or password is wrong. The API logs only that the ping failed, by design.
- **`docker compose` says a variable is required:** create `infra/.env` from the example.
