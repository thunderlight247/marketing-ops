# Stable public entrypoint — 1 November 2026

The deployed platform now sits behind a **stable HTTPS entrypoint**:

- **Stable URL:** https://marketing-ops-entry.marketing-ops-team.workers.dev
  (Cloudflare Worker `marketing-ops-entry`, deployed under the
  `marketing-ops` Cloudflare account, account ID `10b55f924abec4caa9b7367a4c606446`).
- **Architecture:** browser → Worker (stable URL) → quick-tunnel URL that serves
  Next.js :3000 on this host → same-origin `/api/v1` proxy → Go API :8081 →
  Neon Postgres (production branch, pooled endpoint).
- A Cloudflare *named* tunnel (`marketing-ops`) was prepared under the default
  account with ingress → `http://localhost:3000`, but cloudflared tunnels
  cannot get a free hostname without a verified domain zone, so the Worker is
  the entrypoint; it is also registered in `~/.cloudflared/config.yml` should a
  domain be added later (`cloudflared tunnel route dns marketing-ops <host>`).

## Files

- `infra/cloudflare-worker/wrangler.toml` — Worker config; `TARGET` holds the
  current quick-tunnel URL (set via the Worker's `[vars]`, not a secret).
- `infra/cloudflare-worker/src/worker.js` — method/path/header pass-through
  proxy; keeps the browser Origin so the web middleware sees the stable URL.
- `infra/cloudflare-worker/update-target.sh <new-tunnel-url>` — one-command
  re-decoration of `TARGET` + redeploy when the trycloudflare URL rotates.

## Environment coupling

- `apps/web` must run with `OPERATOR_USERNAME`, a long unique
  `OPERATOR_PASSWORD`, `WEB_ORIGIN=https://marketing-ops-entry.marketing-ops-team.workers.dev`,
  `API_BASE_URL=http://localhost:8081`, and an `API_KEY` shared with the Go API.
  `WEB_ORIGIN` must exactly match the URL users browse from, or every non-GET
  `/api/v1/*` request 403s.
- `WEB_AUTH_DISABLED=true` is **development-only** now: a production build
  answers 503 while it is set. Remove it before starting the deployed build.

## Verification (1 Nov 2026, ~19:28 UTC)

- Home and `/workspaces` over the stable URL: 200.
- `/api/v1/health` over the stable URL: `{"status":"ok", database ok}` (Neon,
  ~430 ms round trip).
- `POST /api/v1/workspaces` over the stable URL: created
  `worker-verify-27753` (id `5ad8093f-…`) — a real Neon write through
  Cloudflare Workers → quick tunnel → Next → Go API.

## Current posture / next steps

- **Check the deployed environment before treating this URL as private.** The
  code now refuses the auth bypass in a production build, but the running
  process is only safe once it has been started with operator credentials and
  an API key. Until then it will answer 503 rather than serve unauthenticated
  data, which is the intended failure mode.
- Required deployment changes: `OPERATOR_USERNAME` / `OPERATOR_PASSWORD` on
  web, remove `WEB_AUTH_DISABLED`, set `APP_ENV=production` + `API_KEY` on the
  API, and put the same `API_KEY` in the web proxy environment. Rotate any key
  that was exposed while the URL was public.
- The Worker itself does not authenticate requests; it only forwards them.
  Keep the quick-tunnel origin private and add Cloudflare Access or equivalent
  in front of the Worker if the URL is reachable by others.
- Subdomain registration: the account's free workers.dev subdomain
  `marketing-ops-team` was registered during this work (wrangler v4 removed
  the `subdomain` command; it was done via the Cloudflare API).
