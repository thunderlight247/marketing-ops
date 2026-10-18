# Decision log

Short records of choices made in Milestone 0, so later work does not have to rediscover the reasoning.

| # | Decision | Why |
| --- | --- | --- |
| D1 | Monorepo with `apps/web`, `apps/api`, `docs`, `infra` | Matches the structure in the build brief; one repo keeps the AI and human workflow simple |
| D2 | Go standard library `net/http` with `ServeMux` method patterns, `log/slog` | No router or logging dependency needed for a handful of routes |
| D3 | `pgx/v5` with `pgxpool` is the only Go dependency | Standard PostgreSQL driver; nothing else is needed yet |
| D4 | Own small embedded forward-only migration runner instead of golang-migrate or goose | Avoids an extra tool or dependency for one table's worth of need; logic is small and tested. Revisit if down-migrations or tooling become necessary |
| D5 | The baseline migration creates only `app_metadata` | Proves the pipeline end to end. Business tables belong to Milestone 1 |
| D6 | `/health` always returns 200 with a status body; `/ready` returns 503 when degraded | The dashboard needs details even when the database is down; deploy checks need a pass/fail code |
| D7 | `DATABASE_URL` is optional outside production | Lets the dashboard and API run without PostgreSQL and show an honest "not configured" state |
| D8 | Only the `postgres://` URL form is accepted | Allows safe validation and credential-free display; key=value DSNs are harder to redact |
| D9 | Environment is loaded by the shell (`Makefile` sources `.env`), not a dotenv library | One fewer dependency; the Go code reads plain environment variables |
| D10 | Tailwind CSS v4 (`@tailwindcss/postcss`), Next.js 15 App Router, React 19 | Current stable stack; v4 needs no config file |
| D11 | System font stack, no `next/font/google` | Builds work offline and without a build-time network fetch |
| D12 | No animation or smooth-scroll libraries (Lenis, GSAP, React Bits) in this milestone | The build brief asks for minimal animation on an internal operations dashboard. These suit public marketing sites and can be added to any client-facing site separately |
| D13 | Vitest in a Node environment for frontend tests, covering pure logic only | No jsdom or Testing Library dependency until components have behavior worth testing in isolation |
| D14 | No authentication in Milestone 0 | No client data exists yet. Decision deferred to Milestone 1, revisited in Milestone 13 |
| D15 | Frontend calls the API directly from the browser | Simple, and a stopped API shows a clear error. A server-side proxy can be added if cookies or secrets ever require it |
