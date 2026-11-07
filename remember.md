# Marketing Operations Platform — Complete Project Memory and Milestone History

**Document purpose:** Preserve the project's original direction, sequential milestone roadmap, technical decisions, implementation history, verification evidence, unresolved work, and next steps so a future AI assistant can continue development without restarting, confusing projects, or claiming unfinished work is complete.

**Project:** Marketing Operations Platform
**Pilot business:** Nutrition Guide — `https://nutritionguide.xyz`
**Reference date for latest recorded progress:** 22 October 2026
**Repository location:** Must be confirmed from the current development environment
**Current implementation state:** Multiple milestones and increments have been reported as implemented. The repository must be audited before treating the entire roadmap as complete.

---

## 1. Project identity and purpose

The Marketing Operations Platform is an internal tool for managing marketing operations across multiple client businesses that the operator is authorized to manage.

Its purpose is to bring the main marketing workflow into one place:

1. Manage client workspaces and their business profiles.
2. Discover and configure available marketing integrations.
3. Plan marketing campaigns and content.
4. Generate AI-assisted marketing drafts.
5. Review and approve exact content versions.
6. Publish approved content through supported integrations.
7. Schedule publication and manage background jobs.
8. Record publishing outcomes and errors.
9. Track leads and marketing performance.
10. Produce reports and maintain an audit trail.

The initial pilot is **Nutrition Guide**, whose website is `nutritionguide.xyz`. The site uses Next.js and Sanity, with deployment connected to Vercel.

The project is intended to become a reusable operations platform rather than a one-off integration for Nutrition Guide. New integrations must fit the existing architecture instead of introducing a separate publishing system for each platform.

###

---

## 2. Core technical direction

### Frontend

- Next.js
- TypeScript
- Tailwind CSS
- Responsive operator-facing interface
- Existing frontend test and lint tooling

### Backend

- Go
- Modular API architecture
- PostgreSQL
- Database migrations
- Background jobs and workers
- Provider-neutral integration and publishing contracts

### AI

AI functionality must use a provider abstraction rather than coupling core business logic to one AI provider.

AI-generated output remains a draft until it passes the applicable review and approval workflow.

The SEO Specialist advisory feature described later is an advisory extension of the platform. It does not replace the main roadmap or authorize unrestricted agent activity.

### Database and domain concepts

The original architecture identifies the following core domain concepts:

- `Workspace` / `Client`
- `User` / `Operator`
- `Integration`
- `IntegrationCapability`
- `BrandProfile`
- `ContentItem`
- `ContentVersion`
- `Approval`
- `PublishingJob`
- `PublishedArtifact`
- `AuditEvent`

Campaigns and leads were subsequently added to the recorded implementation scope.

The current repository must be checked to establish the exact implemented table names, relationships, constraints, and migration history. The domain concepts above are the intended model, not a substitute for inspecting the live schema.

### Integration approach

Integrations should be implemented behind reusable contracts and adapters.

Examples include:

- Sanity and other content-management systems
- Client websites using APIs
- Git-connected website deployments
- Google Search Console
- Analytics providers
- Facebook and Instagram publishing
- Lead intake sources

A website may use a CMS, a custom API, a Git-based workflow, or another deployment arrangement. The platform must detect and respect each integration's actual capabilities.

### Engineering principles

1. **Workspace isolation:** One client's data must not leak into another workspace.
2. **Least privilege:** Only use the permissions required for a specific integration.
3. **Secret protection:** Do not commit tokens, expose them to the browser, or leak them through logs.
4. **Human approval:** Publishing must follow the configured approval requirements.
5. **Version correctness:** Approval must apply to the exact content version reviewed.
6. **Idempotency:** Retried jobs must not casually create duplicate publications or records.
7. **Observable failures:** Failures, retries, and external responses must be recorded appropriately.
8. **Capability-aware UI:** Do not offer actions an integration cannot support.
9. **Reusable adapters:** Add providers through existing contracts where practical.
10. **Safe verification:** Start with read-only discovery. Use production writes only when explicitly authorized and controlled.
11. **Honest status reporting:** A passing unit test is not proof of live production behavior.
12. **Incremental development:** Continue from the current architecture unless repository evidence demonstrates a compelling reason to change it.

---

## 3. Why the milestones are sequential

The milestone order is intentional. Each stage establishes capabilities that later stages depend on.

| Milestone range | Main purpose                                       | Why the order matters                                                                          |
| --------------- | -------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| M0–M1           | Engineering foundation and data model              | Later features need a working application, persistent storage, and a consistent domain model.  |
| M2–M3           | Workspace operations and integration registry      | The platform must know which business is being managed and what its integrations can do.       |
| M4–M5           | Content drafting and approvals                     | Content needs a lifecycle and a review gate before publication can be trusted.                 |
| M6–M7           | First publishing integration and reusable adapters | A working end-to-end publishing path provides the basis for other destinations.                |
| M8              | Social publishing                                  | Social channels can reuse the publishing contract, approval model, and job infrastructure.     |
| M9              | Scheduling and background processing               | Time-based publication needs reliable jobs, retries, and status tracking.                      |
| M10             | SEO and analytics                                  | Measurement is more useful after content can be published and its destination recorded.        |
| M11             | Campaigns and leads                                | Marketing activities and incoming leads need workspace ownership and persistent relationships. |
| M12             | Reporting                                          | Reports depend on trustworthy records from publishing, analytics, campaigns, and leads.        |
| M13             | Security, reliability, and usability hardening     | Cross-cutting risks must be addressed before a broader operational rollout.                    |
| M14             | Deployment and pilot release                       | The integrated product must be validated as a complete operational workflow.                   |

This table explains the roadmap's dependency structure. It does **not** mean that every milestone is fully complete or that every dependency has already been verified.

---

# Part I — Original roadmap

## Milestone 0 — Project setup and engineering foundation

**Purpose:** Establish the application structure and local development environment so later work can proceed consistently.

### Planned deliverables

- Frontend application using Next.js, TypeScript, and Tailwind CSS.
- Go backend/API with configuration and a health endpoint.
- PostgreSQL connection and migration mechanism.
- `.env.example` documenting required environment variables without real secrets.
- `.gitignore` that excludes local credentials and generated files.
- Basic formatting, linting, and testing conventions.
- README with local setup and development instructions.
- Simple health or status view.
- Initial tests for foundational behavior.

### Acceptance criteria

- A developer can follow the README to start the application.
- Frontend and backend start successfully.
- The API health endpoint responds correctly.
- The backend can verify its database connection.
- Database migrations can be applied to a clean database.
- No credentials are committed to source control.

### Recorded status

The roadmap originally identified this as the next implementation stage. Later progress indicates that the application evolved substantially beyond initial setup.

### What must still be verified

- Current repository structure and startup commands.
- Current health/readiness endpoints.
- Whether a clean database can be migrated from scratch.
- Whether setup instructions still match the repository.
- Whether CI or local checks cover the actual current code.

**Important:** Do not restart M0 automatically. Audit the current code first and only reopen it if a specific foundation requirement is missing or broken.

---

## Milestone 1 — Domain model and database foundation

**Purpose:** Establish the persistent data model needed to manage multiple businesses, their integrations, content, approvals, publishing jobs, and audit history.

### Planned deliverables

- Database migrations for the core domain.
- Repository or data-access layer.
- Workspace/client ownership relationships.
- Operator/user relationships where required.
- Integration and capability records.
- Brand profiles.
- Content items and version history.
- Approval records.
- Publishing jobs and publication artifacts.
- Audit events.
- Safe seed data for local development.

### Acceptance criteria

- Migrations apply successfully to a clean database.
- The data-access layer can create and retrieve the main domain records.
- Records are scoped to their intended workspace.
- Cross-workspace access is prevented by the relevant application and database rules.
- Tests exercise important data relationships and isolation behavior.

### Why M1 precedes later milestones

Without a stable data model, the dashboard, approval system, integrations, publishing, and reporting would each invent their own representations of clients and content.

### Recorded status

Later milestones rely on this foundation, and the reported M11 work includes migrations `0001` through `0007`. This is evidence of a developed database history, but the current repository and schema still need inspection before the entire M1 acceptance checklist can be marked verified.

---

## Milestone 2 — Dashboard and workspace management

**Purpose:** Give the operator a usable interface for selecting and managing client businesses.

### Planned deliverables

- Responsive dashboard.
- Workspace list.
- Create, view, and edit workspace operations.
- Business profile fields such as:
  - Business description
  - Website
  - Target audience
  - Services
  - Brand tone
  - Brand rules
  - Prohibited claims
  - Calls to action
- Workspace-level sections for integrations and content.
- Empty, loading, success, and error states.

### Acceptance criteria

- The operator can manage multiple workspaces.
- The currently selected workspace controls the displayed data.
- Workspace changes do not accidentally expose data from another client.
- Form validation and error states are understandable.
- The main workspace workflow works without requiring direct database edits.

### Why M2 precedes integrations and content

The platform must know which business it is operating on before it can safely configure accounts, draft content, or publish anything.

### Recorded status

A later browser-based test reportedly verified workspace navigation and campaign/lead screens. That demonstrates some workspace-facing UI functionality, but the full original M2 checklist should be rechecked against the current repository.

---

## Milestone 3 — Integration registry and capability detection

**Purpose:** Represent available integrations consistently and determine what each integration can actually do.

### Planned deliverables

- Integration records associated with workspaces.
- Provider identification and configuration.
- Connection status.
- Capability information.
- Provider-neutral integration interfaces.
- Safe connection checks where supported.
- Error reporting for invalid or incomplete configurations.
- UI that distinguishes configured integrations from unavailable or unsupported capabilities.

### Acceptance criteria

- Integrations can be associated with the correct workspace.
- Provider capabilities are represented explicitly.
- Missing or invalid configuration produces a useful error.
- Unsupported actions are not presented as if they were available.
- Integration failures do not expose credentials.
- A provider can be added without rewriting the whole application.

### Why M3 precedes content publishing

A publishing action must know which destination is configured, whether it is authorized, and which operations that destination supports.

### Recorded status

Later work on Sanity and Meta uses provider-specific integration paths and a shared publishing contract. The exact status of the entire integration registry must still be verified in the repository.

---

## Milestone 4 — Content planning and AI-assisted drafting

**Purpose:** Help the operator create marketing content while keeping business-specific requirements and editorial control.

### Planned deliverables

- Content creation and editing workflow.
- Content types and relevant metadata.
- Content associated with a workspace.
- Draft and version lifecycle.
- Business profile and brand guidance supplied as context.
- AI provider abstraction.
- AI-generated drafts saved as drafts rather than published automatically.
- Validation and understandable error handling.

### Acceptance criteria

- Content belongs to the correct workspace.
- Drafts can be saved and retrieved.
- The system distinguishes drafts from approved content.
- AI output can be reviewed and edited.
- Provider errors do not destroy existing content.
- Sensitive integration credentials are not sent to an AI provider as unnecessary context.

### Why M4 precedes M5

The approval system needs a defined content object and version lifecycle. Approving an undefined or mutable draft would not provide a reliable publishing gate.

### Recorded status

The original roadmap includes AI-assisted marketing content. The SEO Specialist advisory extension was implemented later, but that extension does not, by itself, prove that every M4 content-planning requirement is complete.

---

## Milestone 5 — Review and approvals

**Purpose:** Ensure that content is reviewed before publication and that approval refers to the exact version being published.

### Planned deliverables

- Review status and approval records.
- A review interface.
- Clear distinction between draft, submitted, approved, rejected, and other supported states.
- Version-aware approval.
- Audit history for approval actions.
- Protection against publishing an edited version using an approval granted to an earlier version.

### Acceptance criteria

- The operator can review content before publication.
- Approval applies to the exact content version.
- Editing content after approval invalidates or otherwise correctly handles the previous approval.
- Publishing actions respect the approval gate.
- Approval outcomes are traceable.

### Why M5 precedes M6 and M8

Publishing integrations must not independently bypass the central approval policy. The first website publisher and later social publishers should reuse the same approval rules.

### Recorded status

The recorded M8 implementation uses an approval-gated publishing workflow. The precise completeness of the entire approval lifecycle still needs repository-level verification.

---

## Milestone 6 — First website publishing integration: Sanity

**Purpose:** Prove that approved content can travel through the platform into a real supported content destination.

### Planned deliverables

- Initial Sanity publishing adapter.
- Connection and configuration validation.
- Mapping between platform content and Sanity documents.
- Publishing through the approved content workflow.
- Recording destination identifiers and publication results.
- Error handling and verification.
- A safe test procedure for the pilot site.

### Acceptance criteria

- A configured Sanity integration can be checked safely.
- An approved content version can be submitted through the adapter.
- The result is recorded, including a destination identifier where supported.
- Failures are visible and do not falsely report success.
- The workflow does not bypass approval.
- A real publishing test is conducted only with authorization and an appropriate test document.

### Pilot site technical context

The Nutrition Guide website was previously audited with the following reported details:

- Next.js `14.2.15`.
- Vercel Git-connected deployment.
- Sanity packages, including `sanity`, `next-sanity`, and `@sanity/image-url`.
- Sanity Studio at `/studio`.
- Blog-related queries, pages, slugs, and sitemap.
- Relevant source files included:
  - `src/sanity/client.ts`
  - `src/sanity/queries.ts`
  - `src/sanity/schemas/post.ts`
  - `src/app/blog/`
  - `src/app/sitemap.ts`

The environment was reported to have the required configuration, and a read-only query found an existing post titled **“5 Simple Ways to Enjoy Ghanaian Foods While Losing Weight”**.

This establishes that the site has a Sanity-backed content workflow and that read-only discovery was possible. It does **not** prove that the Marketing Operations Platform successfully created or published a document through the write API.

### Recorded status

The project has treated Sanity as its first website-publishing integration. A controlled write/publish verification remains an important item unless the current repository and execution records prove that it has since been completed.

---

## Milestone 7 — Reusable website adapters and provider-neutral publishing

**Purpose:** Avoid building an isolated publishing workflow for every client website.

### Planned deliverables

- A provider-neutral publishing contract.
- Reusable adapter conventions.
- Consistent input and output models.
- Common validation and error mapping.
- A way to determine which operations each provider supports.
- A foundation for adding additional website and CMS providers.

### Acceptance criteria

- Provider-specific logic is contained in adapters.
- The application uses a common publishing interface.
- A new adapter can be added without duplicating the entire job and approval workflow.
- Provider-specific limitations are reported accurately.
- Unsupported actions are not treated as successful publications.

### Recorded implementation

The roadmap update reported that the **first increment of M7 — the provider-neutral publishing contract — was implemented**.

The recorded status refers to the first increment, not necessarily every possible website adapter or every provider-specific capability.

### Important distinction

A common publishing contract makes it possible to support multiple destinations. It does not mean all websites can be published automatically. Some may require a CMS API, others a custom API, and others a controlled Git-based deployment workflow.

---

## Milestone 8 — Social publishing

**Purpose:** Extend the existing content publishing workflow to supported social platforms while reusing approval, validation, and job infrastructure.

### Planned deliverables

- Facebook Page publishing.
- Instagram professional-account publishing where supported.
- Workspace-specific social integration configuration.
- Channel selection.
- Approval-gated publishing.
- Immediate and scheduled publication through the shared pipeline.
- Duplicate protection and error mapping.
- Fake-server contract tests.
- Clear reporting of platform limitations.

### Recorded implementation

The first Meta publishing increment was reported as implemented and tested.

Reported behavior includes:

- Use of the existing adapter/job pipeline.
- A configured Facebook Page or Instagram professional account per integration.
- Social publishing UI.
- Channel selection.
- Approval-gated **Publish post now** action.
- Scheduled publishing using UTC through the same scheduler.
- Fake-server tests covering connection checks, missing/rejected tokens, invalid configuration, Facebook and Instagram request shapes, duplicate reuse, error mapping, draftless-content handling, and publisher validation.

### Reported verification

- `go vet ./...`
- `go test -count=1 ./...`
- Frontend Vitest: 152/152 tests passed in the recorded run.
- ESLint passed.
- `tsc --noEmit` passed.
- Prettier passed.

These results establish the reported test status for that implementation increment. They do not establish successful live publishing to a real client account.

### Known limitations at the time of the report

1. Real Meta credentials, app review, live publication, and public visibility were not verified.
2. Facebook duplicate scanning covered only the most recent 25 posts.
3. An interrupted Instagram retry after container/media creation could risk a duplicate.
4. Insights were not supported in this increment.
5. Destination-side scheduling was not supported.
6. Token configuration was manual; a complete OAuth onboarding flow was not implemented.

### Current status

**Implemented increment; live-account validation and multi-client authorization remain open unless subsequently verified.**

Do not describe Meta publishing as production-verified simply because fake-server tests passed.

---

## Milestone 9 — Scheduler, background workers, and retries

**Purpose:** Make scheduled publishing reliable without depending on an operator keeping a browser window open.

### Planned deliverables

- Persistent publishing jobs.
- Scheduling and due-job selection.
- Background worker processing.
- Retry policy.
- Job states and failure reporting.
- Duplicate protection and idempotency.
- Recovery from temporary provider failures.
- Operator-facing scheduling controls.
- Clear visibility into scheduled and failed jobs.

### Acceptance criteria

- A scheduled job is persisted.
- Due jobs can be processed independently of the frontend.
- Failures are recorded and retried according to defined rules.
- Duplicate job execution is handled safely.
- The operator can understand whether a job is queued, running, successful, failed, or awaiting retry.
- Time zones are handled consistently.
- Approval and content-version rules are enforced when jobs execute.

### Recorded implementation

The project update reports that the **first scheduler/worker/retry increment was implemented**.

Media support was also reported as implemented, including:

- Main image support.
- Body media support.

The recorded roadmap still identified the **operator-facing scheduled-publish UI** as open at that point.

### Current status

Backend scheduling and worker behavior have a reported implementation, but the completeness of the operator-facing scheduling workflow must be verified.

The UI may have progressed after the earlier status report. Inspect the repository before assuming it remains missing.

### Special caution

Idempotency must be assessed separately for each external provider. A job record being unique in the local database does not guarantee that a retry cannot duplicate an external post if the provider accepted the first request but the response was lost.

---

## Milestone 10 — SEO and analytics

**Purpose:** Connect published marketing activity to search visibility and measurable performance.

### Planned deliverables

The original roadmap places SEO and analytics after the platform has a publishing workflow. The exact complete M10 acceptance checklist must be recovered from the original roadmap or verified against the repository before treating this milestone as fully specified.

The intended capability area includes:

- SEO-related information and workflows.
- Google Search Console integration.
- Analytics integration.
- Retrieval and display of relevant performance information.
- A relationship between published content and its performance records.
- Useful error and connection states.

### Recorded status

Google Search Console and analytics work were part of the integration plan. However, **live OAuth-backed Google Search Console data was not verified** in the latest recorded context.

### What is not yet established

- That OAuth authorization is complete.
- That the correct property is selected and bound to the correct workspace.
- That live Search Console data is retrieved and displayed.
- That analytics data is retrieved and displayed.
- That reporting is based on verified production data rather than mock or fixture data.

### Next verification

Inspect the integration and authorization implementation, confirm its permissions and workspace binding, then run a safe read-only test using an account and property the operator is authorized to access.

---

## Milestone 11 — Campaigns and leads

**Purpose:** Track marketing campaigns and the leads associated with them within the correct client workspace.

### Planned deliverables

- Campaign creation and listing.
- Campaign updates.
- Lead creation and listing.
- Lead status updates.
- Lead-to-campaign association.
- Workspace isolation.
- Input validation.
- Duplicate and conflict handling.
- Audit events for relevant operations.
- Operator-facing campaign and lead screens.

### Recorded verification — 22 October 2026

A first increment was tested against a local PostgreSQL-backed environment.

#### Database and API

- Local PostgreSQL Compose environment reported healthy.
- Migrations `0001` through `0007` were applied.
- Database-backed campaign and lead tests passed.
- Development API ran on port `8081`.
- `GET /api/v1/ready` returned HTTP 200.

Reported API smoke-test outcomes for the `demo-cafe` workspace:

| Operation                           | Reported result |
| ----------------------------------- | --------------- |
| Create campaign                     | HTTP 201        |
| List campaigns                      | HTTP 200        |
| Update campaign                     | HTTP 200        |
| Duplicate campaign name             | HTTP 409        |
| Create lead                         | HTTP 201        |
| List leads                          | HTTP 200        |
| Update lead                         | HTTP 200        |
| Invalid lead source                 | HTTP 400        |
| Cross-workspace campaign assignment | HTTP 400        |

#### Browser/UI

Headless Chromium testing reportedly verified:

- Workspace navigation.
- Campaign and lead empty states.
- Campaign creation.
- Campaign persistence after refresh.
- Lead creation associated with a campaign.
- Lead status update to `Contacted`.

The recorded Playwright end-to-end tests passed across three runs.

#### Cleanup

Temporary campaign, lead, and audit records created during the smoke tests were removed.

The reported final counts were:

- Campaigns: `0`
- Leads: `0`
- Smoke-test rows: `0`

The report stated that demo records were not altered and that the API/UI remained available after cleanup.

### What this proves

This is evidence that the tested campaign/lead API and UI paths worked in the local environment at the time of testing.

### What this does not prove

- Real external lead intake.
- Complete marketing attribution.
- Production deployment behavior.
- Every possible campaign/lead validation case.
- The entire original roadmap being complete.

### Current status

**First increment verified in the recorded local test environment.** Continue from the existing implementation and avoid recreating the same CRUD features without identifying a specific gap.

---

## Milestone 12 — Reporting

**Purpose:** Turn the platform's recorded marketing activity into useful summaries for the operator and client businesses.

### Dependency

Reporting relies on the quality and consistency of the records produced by earlier milestones, including:

- Published content and publication results.
- Campaigns.
- Leads.
- Search and analytics data.
- Integration/job statuses.
- Workspace relationships.

### Work to establish

The exact original M12 acceptance checklist is not established by the latest progress summary. Recover it from the original roadmap or repository documentation before expanding the scope.

At minimum, audit whether the existing data can support useful reports without mixing workspaces or presenting unverified metrics as facts.

### Acceptance criteria to verify

- Reports are scoped to the selected workspace.
- Reported figures have identifiable source records.
- Missing integration data is distinguished from a genuine zero.
- Date ranges and time zones are handled consistently.
- Failures and stale data are communicated.
- Any export or report-generation behavior matches the intended product scope.

### Recorded status (updated 2026-10-24)

**First increment implemented and verified.** The smallest operator-facing
report now exists end to end:

- `GET /api/v1/workspaces/{id}/reports?since=&until=` returns current-state
  content/campaign/lead counts plus range-filtered publishing figures with
  recent failures (all workspace-scoped in SQL, nothing fabricated).
- Web typed client `apps/web/src/lib/reports.ts` + report panel section wired
  into the workspace detail panel; Vitest coverage in
  `apps/web/src/lib/reports.test.ts`.
- Store aggregation tests in `apps/api/internal/store/reports_store_test.go`
  and router tests in `apps/api/internal/httpapi/workspace_reports_test.go`.
- **Live verification found and fixed a real bug** the fake-store router
  tests could not catch: the publishing-jobs query joined
  `publishing_jobs` to `content_items` and selected bare
  `status`/`GROUP BY status` (ambiguous across both tables,
  SQLSTATE 42702), so every report request returned 500. Fixed by
  qualifying `pj.status` in `apps/api/internal/store/reports.go`; a probe
  against the live local database confirmed all workspaces failed before
  the fix and render 200 after it.
- Verified live on the running local API: valid request → 200 with all
  sections populated (empty map values are genuine zeros for an empty
  workspace), ranged request → 200 with echoed bounds, inverted range →
  400 `until must be after since`, malformed timestamp → 400.
- Full API checks re-run green after the fix: `gofmt -s`, `go vet`,
  `go test -race ./...` (with DATABASE_URL configured).

**Still open in M12:** date-range presentation in the UI beyond the panel,
export/report-generation behavior, stale-data/failure surfacing UX, and the
final documented acceptance checklist from the original roadmap.

**Purpose:** Make the integrated platform safer and more dependable before expanding its operational use.

### Areas to audit

- Workspace isolation and authorization.
- Integration secret storage and redaction.
- OAuth state validation and callback security where OAuth exists.
- Token handling and revocation.
- Audit events.
- Publishing approval enforcement.
- Content-version correctness.
- Idempotency and retry behavior.
- Error visibility and recovery.
- Database constraints and migration safety.
- UI validation, empty states, and error states.
- Test coverage for high-risk workflows.
- Operational documentation.

### Acceptance criteria to verify

- Unauthorized workspace access is rejected.
- Credentials are never exposed to browser code or routine logs.
- Publishing cannot bypass required approval.
- Retries have documented behavior and appropriate duplicate protection.
- Sensitive operations are auditable.
- Important failure modes have tests.
- The application can be diagnosed without exposing secrets.

### Recorded status

Security and reliability principles are part of the original architecture. That does not mean the entire hardening milestone has been completed.

**M13 remains a milestone to verify and close systematically**, even if earlier milestones already implemented individual protections.

### M13 systematic audit (recorded 2026-10-24, first pass)

Verified-strong areas (with evidence):

- **Workspace isolation:** every route is workspace-scoped — the campaigns,
  leads, and report handlers use the shared `workspaceScope` check; the rest
  look up parent workspaces/integrations/items through store methods that
  filter on `workspace_id` and return `ErrNotFound` cross-workspace, and the
  store integration tests assert that behavior (e.g. `workflow_test.go`,
  `campaigns_leads_test.go`).
- **Credential handling:** AES-256-GCM at rest (`secretcipher`), key only
  from env, key version stored for rotation; credentials are never returned
  to the browser (only `hasCreds` booleans); worker/handler logs use
  `error_class`/`safeErrorText` instead of error details; config validation
  never echoes `DATABASE_URL`.
- **Approval + version correctness:** publish is gated on the newest version
  being approved (store returns `ErrNotApproved` on insert and at retry,
  mapped to 409); idempotency key embeds `(workspace, item, version,
  integration)` with a DB `UNIQUE` constraint; duplicate publish → 409;
  resume avoids duplicate inserts.
- **Transport/logging:** CORS strict allowlist with preflight handling,
  security headers, request logger without query strings/headers, panic
  recovery, body-size limits on all JSON routes, RFC3339/UTC validation.
- **DB constraints:** status CHECK constraints on campaigns/leads/publishing
  rows, FK CASCADE, `UNIQUE (workspace_id, name)` (now several places),
  `idempotency_key UNIQUE`.

Gaps found (ranked, all small and actionable, none drop existing behavior):

1. ~~HIGH — AI generate error surface~~ **FIXED 2026-10-24**
   (`workspace_content_generate.go`): the 502 handler wrote `err.Error()`
   unconditionally. Now it surfaces only classified `ProviderError` text
   (asserted with `errors.As`, matching the agent-route convention) and
   returns the generic "the AI provider could not be reached" otherwise.
   Router test added: a non-provider error must not leak its details.
2. ~~MEDIUM — missing audit rows~~ **FIXED 2026-10-24**: `workspace.created`
   is now audited, and operator actions `publishing_job_cancelled` and
   `publishing_job_retried` (with jobId + attempts) record audit rows;
   router tests assert each.
3. ~~LOW — silent decrypt failure~~ **FIXED 2026-10-24**: the check handler
   now names the exact reason (no server key / decrypt failed / payload did
   not parse) in `Diagnostic` with a `LifecycleError` result and records the
   reason class as `credentialProblem` in the `integration.checked` audit
   row; the publish path gained a distinct `ErrPublisherCredentialsUnreadable`
   sentinel → 409 "re-enter them" instead of the misleading "missing".
   Router tests cover both paths; the web client type carries the
   machine-readable `credentialProblem` field.
4. ~~MEDIUM — no authentication layer~~ **FIXED 2026-10-24 (M13)**:
   Added operator API key authentication middleware (`auth.go`, `auth_test.go`)
   with constant-time comparison, strict bearer token / header support, and
   token-bucket rate limiting (`ratelimit.go`, `ratelimit_test.go`). Integrated
   `withAuth()` into all frontend client libraries (`config.ts`, `config.test.ts`,
   `workspaces.ts`, `content.ts`, `campaigns.ts`, `agents.ts`, `seo.ts`,
   `integrations.ts`, `reports.ts`).
5. **LOW — worker concurrency:** the publishing worker runs in-process;
   horizontal scaling would need a leader-election or lock guard. Fine for
   the local/single-instance deployment in M14 increment 1; document it.

---

### M13 & UI/UX Mobile Overhaul & Production Cleanup Update (25 October 2026)

- **M13 Security & Hardening completed and verified (commit `55e4719`):**
  - Authentication middleware (`auth.go`) and rate limiting (`ratelimit.go`) implemented and tested.
  - Required API keys enforced in production; optional in development.
  - All web clients include authorization headers via `withAuth()`.

- **UI/UX Mobile Overhaul and Production Cleanup completed and verified (commits `489d374`, `6d86814`):**
  - Upgraded design system across 18+ components from legacy teal to unified indigo theme.
  - Mobile responsiveness: card headers, 16px touch-friendly input fields (prevents iOS auto-zoom), responsive navigation drawer with SVG icons, responsive tables.
  - Production readiness cleanup:
    - Removed demo workspace badges from `overview-workspaces.tsx`.
    - Removed `plannedFor` and milestone references from `navigation.ts` and UI views.
    - Cleaned up developer comments across `seo.ts`, `content.ts`, `campaigns.ts`, `reports.ts`, and `draft-editor.tsx`.
    - Modified migration `0003_seed_demo.sql` so fresh databases do not seed demo workspaces.
    - Purged all 10 demo/test workspaces from the PostgreSQL database, leaving only the real pilot workspace (`nutritionguide-studio`).
  - Verification results:
    - Go test suite: all packages passing (`go test ./...`).
    - Web test suite: 14 test files, 171 tests passing (`vitest run`).
    - Typecheck: 0 TypeScript errors (`tsc --noEmit`).
    - Linter: 0 ESLint warnings (`eslint .`).
    - Pushed cleanly to `origin/main`.
---

## Milestone 14 — Deployment and pilot release

**Purpose:** Validate the integrated product as an operational tool for the pilot and prepare a controlled path to managing additional authorized client businesses.

### Work to establish

- Deployment architecture and environment configuration.
- Database migration and recovery procedure.
- Secrets management.
- Health/readiness monitoring.
- Worker operation and recovery.
- Backup and restore expectations.
- Deployment and rollback instructions.
- Pilot acceptance workflow.
- Operator documentation.
- A clear record of integrations verified in live environments.

### Pilot scenario

The original end-to-end scenario is:

1. Select the Nutrition Guide workspace.
2. Load its business profile and authorized integrations.
3. Draft a marketing article.
4. Save and review the draft.
5. Approve the exact content version.
6. Publish through the Sanity adapter.
7. Record the destination document ID, public URL, time, and result.
8. Verify the publication.
9. Later display performance information when a verified analytics integration is available.

This scenario is a target workflow, not proof that all steps have already passed in a live environment.

### Acceptance criteria to verify

- A new operator can follow the documented process.
- The pilot workflow works from beginning to end.
- Live credentials and permissions are managed securely.
- Publishing results are verified rather than assumed.
- Failures and recovery paths are documented.
- No unapproved production changes occur.
- The operator can distinguish tested integrations from planned or simulated integrations.

### Recorded status

**Full M14 completion has not been established in the latest recorded project context.**

---

# Part II — Cross-cutting implementation history

## 4. Nutrition Guide pilot audit

Nutrition Guide was chosen as the first integration target because its existing site and CMS provide a concrete path to test the platform against a real client workflow.

The recorded audit established the Next.js/Sanity structure and a successful read-only content query.

### Continuing rules

- Treat read-only discovery as distinct from write access.
- Do not assume that environment variables being present means credentials are valid.
- Do not assume that access to the Sanity Studio automatically proves the backend publishing integration works.
- Do not edit, publish, or deploy client content without authorization.
- Prefer a clearly identified test draft and a controlled verification process.
- Verify the resulting document and public page after a write operation.

### Next action

Inspect the current Sanity adapter and determine whether a real, authorized test publication has ever succeeded. If not, plan a safe test without changing existing production content.

---

## 5. Agency Agents / SEO Specialist advisory extension

Reference project:

`https://github.com/msitarzewski/agency-agents`

The project later incorporated the concept of agency-style specialist agents into the Marketing Operations Platform.

### Recorded implementation

- The SEO Specialist was selected as the first specialist.

- A curated, internal advisory prompt profile was implemented.

- It uses the existing provider interface.

- Upstream agent files were not copied wholesale.

- The specialist is advisory-only.

- It does not have unrestricted network, filesystem, or general tool authority.

- An endpoint was recorded:

  `POST /api/v1/workspaces/{id}/agents/seo/advisory`

- Audit rows were added for advisory activity.

- A typed frontend panel was implemented.

- Go and frontend tests were reported clean.

### Limitations

- Live AI-provider execution was not verified in the latest status.
- OAuth-backed Google Search Console data was not verified.
- A dedicated advisory run-history UI was not established as complete.

### Architectural rule

Treat this as a cross-cutting extension, not a replacement for the original milestone roadmap.

Do not restart the platform around an agent framework. Do not grant the specialist tools or permissions beyond its documented purpose. Keep its recommendations distinguishable from executed actions.

**Later direction (26 October 2026):** the user expanded this from a single advisory specialist to a broader agency-style agent layer inside the platform. See Part VI, section 15.

---

## 6. Multi-client Meta account authorization

The current Meta implementation has a path for publishing through a configured account, but multi-client authorization remains a major area to verify and extend.

The intended direction is to support both:

- Client-assisted onboarding, where the client completes official account authorization.
- Admin-assisted onboarding, where an authorized operator helps connect the client's account without asking for or storing the client's password.

### Proposed design

1. Use Meta's official OAuth flow where supported.
2. Bind the authorization request to the correct workspace.
3. Validate OAuth `state`.
4. Use registered redirect URIs.
5. Request only necessary permissions.
6. Store access credentials securely on the server.
7. Never expose tokens to the frontend.
8. Avoid secrets in prompts, errors, and logs.
9. Support connection status and account selection.
10. Support reconnect and disconnect flows.
11. Audit authorization and connection changes.
12. Reuse the existing publishing, approval, and scheduler pipeline.

### Important status distinction

This is the recorded target design, **not a claim that OAuth is already implemented**.

Before coding, inspect the existing integration schema, token configuration, adapter, API routes, and tests. Reuse existing structures where appropriate.

### Suggested acceptance criteria

- A user cannot connect an account to a workspace they are not authorized to manage.
- OAuth state and callback validation work.
- The selected account is associated with the intended workspace.
- Tokens are protected at rest and never returned to ordinary frontend responses.
- Expired or revoked authorization is reported clearly.
- Disconnecting an account prevents future publishing through that connection.
- Publishing still follows the central approval and job pipeline.
- Tests cover invalid state, denied authorization, missing permissions, token failure, and workspace mismatch.

---

# Part III — Consolidated status

## 7. Recorded status by milestone

The table below distinguishes reported implementation from verified end-to-end production behavior.

| Milestone              | Recorded status                                                 | Important qualification                                              |
| ---------------------- | --------------------------------------------------------------- | -------------------------------------------------------------------- |
| M0 — Foundation        | Earlier foundation established; audit required                  | Verify clean setup and current instructions.                         |
| M1 — Domain model      | Core domain and migrations developed                            | Audit current schema, constraints, and isolation.                    |
| M2 — Workspace UI      | Workspace-facing UI exists in recorded testing                  | Verify the full original checklist.                                  |
| M3 — Integrations      | Integration work exists                                         | Verify the registry and complete capability model.                   |
| M4 — Drafting          | AI-assisted content is part of the architecture                 | Do not infer full completion from the SEO advisory feature.          |
| M5 — Approvals         | Approval-gated publishing is reported                           | Audit exact-version approval behavior across providers.              |
| M6 — Sanity            | Pilot integration is established as the first publishing target | Real write/publish success remains unconfirmed in the latest status. |
| M7 — Adapter contract  | First increment implemented                                     | Do not assume all website adapters exist.                            |
| M8 — Social publishing | First Meta increment implemented and fake-server tested         | Live publishing and OAuth onboarding remain unverified.              |
| M9 — Scheduler         | First worker/retry increment implemented                        | Verify operator scheduling UI and real provider retry behavior.      |
| M10 — SEO/analytics    | Planned and partially explored                                  | Live OAuth-backed data remains unverified.                           |
| M11 — Campaigns/leads  | First increment tested locally                                  | External intake and production behavior remain unverified.           |
| M12 — Reporting        | Not confirmed complete                                          | Recover exact acceptance criteria and inspect implementation.        |
| M13 — Hardening        | Cross-cutting requirements identified                           | Full security/reliability closure is not established.                |
| M14 — Pilot release    | Full completion not established                                 | Validate end-to-end deployment and operational workflow.             |

**These are historical status notes.** The next assistant must check the current repository, because some UI or integration work may have advanced since these entries were recorded.

---

## 8. Testing and verification evidence

The following evidence was reported in the project history.

### Meta publishing increment

- `go vet ./...`
- `go test -count=1 ./...`
- Fake-server contract tests for connection checks, token errors, invalid configuration, provider request shapes, duplicate handling, and error mapping.
- Frontend Vitest: 152/152.
- ESLint.
- `tsc --noEmit`.
- Prettier.

**Interpretation:** These are reported test results for that increment. They do not establish real Meta account authorization or live public posting.

### Campaigns and leads increment

- Local PostgreSQL Compose environment healthy.
- Migrations `0001`–`0007` applied.
- Database-backed campaign and lead tests passed.
- Readiness endpoint returned HTTP 200.
- API smoke tests produced the reported status codes.
- Browser-based campaign/lead workflow was tested.
- Playwright end-to-end tests passed across three runs.
- Temporary test records were cleaned up.

**Interpretation:** The tested local workflow worked at the time of the recorded test. This is not evidence of complete production readiness.

### SEO Specialist advisory increment

- Go tests reportedly passed.
- Frontend tests reportedly passed.
- Advisory endpoint and audit behavior were implemented.
- Live AI-provider execution and live Google Search Console retrieval remain unverified.

### General verification policy

For every feature, distinguish among:

1. **Implemented:** Code exists.
2. **Unit-tested:** Isolated logic passes tests.
3. **Integration-tested:** Components work together in a controlled environment.
4. **Locally smoke-tested:** A real local application path was exercised.
5. **Externally verified:** A real authorized external integration was exercised.
6. **Production-verified:** The intended deployed workflow was tested under the applicable operational conditions.

Do not collapse these into one “done” status.

---

# Part IV — Remaining work and next steps

## 9. Priority 1: Audit the current repository

Before making changes, establish the actual state of the project.

### Audit checklist

- [ ] Confirm the correct repository and working directory.
- [ ] Read the current README and project instructions.
- [ ] Inspect Git status and recent commits.
- [ ] Inspect the frontend and backend structure.
- [ ] Inspect database migrations and schema.
- [ ] Identify implemented API routes.
- [ ] Identify integration adapters and their capabilities.
- [ ] Inspect the approval and content-version workflow.
- [ ] Inspect scheduler/worker startup and retry behavior.
- [ ] Inspect campaign and lead models, routes, UI, and tests.
- [ ] Inspect the SEO advisory endpoint and frontend panel.
- [ ] Inspect current Meta configuration and publishing paths.
- [ ] Inspect any OAuth implementation before designing a new one.
- [ ] Run the existing test and quality-check commands.
- [ ] Record what passes, fails, or cannot be run.
- [ ] Compare the actual implementation against each milestone's acceptance criteria.

Do not replace working code just because an older memory file says a milestone is incomplete.

## 10. Priority 2: Close the most consequential integration gaps

After the audit, determine the smallest high-value next change.

### A. Multi-client Meta connection lifecycle

If the repository confirms that OAuth onboarding is still missing, implement it through the existing integration architecture.

Do not rewrite the Meta publisher or scheduler unnecessarily.

### B. Live Meta verification

Once authorization and permissions are properly configured, conduct a controlled test using an account the operator is authorized to manage.

Verify the external result rather than relying only on a successful API response from a mock server.

### C. Sanity write/publish verification

Determine whether a real Sanity publication has already been completed. If not, arrange a safe, authorized test using a new test draft and verify the resulting document and public page.

### D. Scheduling workflow

Audit the operator-facing scheduled-publish UI and worker status reporting. Implement only the missing portion.

### E. SEO and analytics

Verify the existing OAuth and data-retrieval path before expanding the feature. Do not display mock data as if it came from a live client property.

## 11. Priority 3: Complete the roadmap in dependency order

Once the immediate integration gaps are addressed:

1. Reassess M0–M7 using the original acceptance criteria.
2. Finish any remaining M8–M9 workflow gaps.
3. Verify M10 against live authorized data.
4. Complete the required M11 campaign/lead scope.
5. Establish and deliver the exact M12 reporting requirements.
6. Conduct M13 security, reliability, and UX hardening.
7. Validate M14 deployment and the pilot workflow.

A milestone may be closed only when its required criteria have evidence. If the project deliberately defers a requirement, document the deferral instead of silently marking it complete.

---

# Part V — How a future AI assistant must continue

## 12. Operating instructions

The assistant continuing this project must follow these rules.

### First: establish the current truth

- Read this memory file.
- Read the repository's current README and local agent/developer instructions.
- Inspect the code and Git status.
- Run appropriate checks where practical.
- Compare the repository with the recorded milestone status.
- Treat the repository and fresh test results as the primary evidence of current implementation.

### Second: preserve project identity

This is the **Marketing Operations Platform**, not LightDesk.

Do not import LightDesk-specific architecture, paths, features, or milestone statuses unless the user explicitly asks for a cross-project comparison.

### Third: make the smallest useful change

- Continue from existing working code.
- Reuse current contracts and adapters.
- Avoid architecture rewrites without a demonstrated reason.
- Do not duplicate features that already work.
- Identify the milestone and acceptance criterion addressed by each change.

### Fourth: protect client data and production systems

- Do not expose credentials.
- Do not commit real tokens.
- Do not assume permission to publish or modify a client website.
- Prefer read-only tests until write access and authorization are confirmed.
- Use controlled test content for external write verification.
- Never claim that a live integration works when only mocks or local tests have passed.

### Fifth: maintain milestone history

After meaningful work, update this memory file with:

- Date.
- Milestone and increment.
- Files or components changed.
- Behavior implemented.
- Commands and tests executed.
- Actual results.
- External verification performed, if any.
- Known limitations.
- Remaining acceptance criteria.
- Next recommended step.

Keep historical facts and current status separate. Do not erase earlier milestones when adding a new one.

### Sixth: report status clearly

Use a consistent status vocabulary:

- **Complete and verified:** Acceptance criteria have supporting evidence.
- **Implemented, verification pending:** Code exists but sufficient verification is missing.
- **In progress:** Only part of the milestone is implemented.
- **Blocked:** Progress depends on credentials, permissions, an external service, or a decision.
- **Not started:** No meaningful implementation has been established.
- **Deferred:** Explicitly postponed, with a reason.

Do not mark an entire milestone complete because its first increment passes tests.

---

## 13. Decisions that must not be silently changed

1. The platform is an internal multi-client marketing operations tool.
2. Nutrition Guide is the first pilot.
3. The frontend uses Next.js, TypeScript, and Tailwind CSS.
4. The backend uses Go.
5. PostgreSQL is the primary database.
6. Integrations should use reusable provider contracts and adapters.
7. Approval and content-version correctness are central publishing requirements.
8. Scheduled work must be persisted and observable.
9. Client workspaces must remain isolated.
10. AI specialist functionality is advisory unless an explicitly authorized workflow says otherwise.
11. The project should progress incrementally, without unnecessary rewrites.
12. Live integration claims require live evidence.
13. The roadmap should retain its historical milestone sequence.
14. This project remains separate from LightDesk.

If a change to one of these decisions appears necessary, explain the reason and seek the user's agreement when it materially affects the architecture or project direction.

---

# Part VI — Agent agency and social-first posting direction

## 15. Approved direction (recorded 26 October 2026)

The user approved the following platform direction. All of it is to be built **on top of what already exists** — the adapter contract, integration registry, approval pipeline, scheduler, publish worker, and audit trail are the foundation; nothing here is a rewrite.

### A. Agency-style agent layer inside the platform

The agency-agents concept (`msitarzewski/agency-agents`) moves from one advisory specialist (SEO, section 5) to an integrated agent layer the site itself uses. Relevant specialist roles include marketing, project management, and testing, plus any other role that aligns with the platform's purpose.

Implementation rules:

1. Specialists are **curated internal prompt profiles** on the existing provider abstraction (as the SEO Specialist was), not vendored crates of the upstream repository. No separate agent runtime or framework dependency is introduced.
2. Recommended operating model: **advisory + limited actions**. Specialists generate guidance and drafts; a defined, explicitly enumerated set of *in-platform* actions (e.g. create a draft, propose content for approval, open a scheduled-publish job) may be executed directly. Anything client-facing or destructive still requires human approval.
3. Every agent action — advisory or executed — is recorded in the audit trail, and agent-produced content remains distinguishable from operator actions.
4. The enumerated action set is a design decision per specialist; it is documented here when added and never expanded silently.
5. New specialists reuse the existing route shape (`/api/v1/workspaces/{id}/agents/...`), typed frontend clients, and persona/test conventions established by the SEO extension.

#### A.1 Agent-layer registry design (recorded 26 October 2026)

The design that turns section 15 A into pluggable agency agents, grounded in the existing implementation (`internal/agents` `SEOAdvisor` + `ContentCreator`, duplicated `seoAdvisoryHandler`/`contentCreatorDraftHandler` routes, per-specialist web clients). Generalizing rule: every existing security property of the two specialists stays — bounded context only, validated replies, advisory-only unless an explicitly registered action class, no secrets in prompts, per-run audit.

`internal/agents` package:

1. New `Specialist` interface replaces per-struct conventions:
   - `ID()`, `Version()`, `DisplayName()`, `Description()` — identity, shown in the UI and audit rows.
   - `OutputContract()` — closed set: `"advisory"` (→ validated `AdvisorReport`) or `"drafting"` (→ validated `ContentPlan`). Extended only by documenting a new validated output contract here first.
   - `WantsData()` — declares which authorized data snapshots the run consumes (`"search_console"` first). Anything not declared must not reach the prompt.
   - `Run(ctx, PromptRun, SpecialistInput) (RunOutput, error)` — one bounded pass; `SpecialistInput{Workspace, Focus, Data}` carries only declared snapshots; the specialist never sees stores, credentials, or request plumbing.
2. New `Registry` (same conventions as `integration.Registry`): `Register` (rejects duplicate IDs), `Get`, `ListDescriptors` (sorted: id, version, display name, description, output contract, declared data needs, allowed action classes), `NewDefaultRegistry` registering `seo_specialist` and `content_creator` unchanged in behavior.
3. `SnapshotLoader` registry keyed by data need (`"search_console"` → the existing GSC loader). New loaders (e.g. campaigns/leads summaries for a project-manager specialist) register without touching handlers.

HTTP layer:

4. One generic dispatch, replacing per-specialist handlers:
   - `GET /api/v1/agents` — descriptor list for the web agents panel.
   - `POST /api/v1/workspaces/{id}/agents/{agentId}/run` — unknown agent → 404; runner not configured → 501; else the shared pipeline: payload/focus caps → workspace + brand context → declared-snapshot loaders (failures degrade to no-data with a note) → timeout → run → audit (`agent.run.completed` / `agent.run.failed` with agent_id, version, provider, model, data_source, output stats, reason_kind) → `{agent, report?|plan?, dataNote, durationMs}`.
   - The existing two routes migrate to the generic handler in one release (web client and tests updated together); no dual-route period.
5. Same AI rate-limit middleware guards the generic route.

Governance (the "advisory + limited actions" rule made executable):

6. Allowed action classes are a closed set on the registry descriptor (`advisory_only`, then `file_draft`, later e.g. `open_publish_job`). A specialist's scheme output may propose actions; a dedicated `ActionExecutor` (not the model) validates payloads and executes only classes listed for that agent, through the store, each class audited as `agent.action.<class>`. Executing an undeclared class is refused and audited. `file_draft` is the first executor action (the existing content-creator draft filing path, unchanged).

Web:

7. `lib/agents.ts` grows a registry client (`fetchAgentRegistry`, generic `requestAgentRun`) with strict per-contract response parsing; the existing SEO advisory function becomes a thin wrapper on it. A generic specialists section renders from descriptors, reusing the existing report and plan result views.

Increment sequence:

1. API registry + generic route with the two existing specialists migrated (behavior-preserving).
2. Web generic agents panel.
3. First new specialists (marketing, project management, testing roles for section 15 A) as curated profiles over registered data loaders — no framework change per agent.
4. Action executor classes beyond `file_draft`, each designed, listed in a descriptor, and recorded here before implementation.

Guardrails carried into the design: duplicate registration is an error; changing a specialist's prompt requires a version bump; input/output caps stay per contract; audit rows stay scalar and secret-free; no agent ever receives channel credentials.


### B. Visible, automatic social posting

Social media posting becomes a first-class, visible flow in the dashboard:

1. An operator composes a post once and targets one or more connected social channels.
2. The post can be published immediately or scheduled.
3. After approval (see C), the background worker delivers it automatically to each connected channel at its due time — no one needs to keep a browser open.
4. Per-channel job and outcome records are visible to the operator: scheduled, running, delivered, failed, awaiting-retry, plus the permalink/external ID of the delivered post.
5. Delivery always goes through the existing publishing-job pipeline and one adapter per channel, preserving idempotency, retry/backoff, and reconnection flagging per channel.

### C. Automatic-posting decision

**Recorded decision:** "automatic posting" means fully unattended delivery **after approval**. Every social post passes the existing human-approval gate before scheduling; once approved and scheduled, the worker posts without further involvement. The recorded approval-gating decision is unchanged.

### D. Social channel set

The existing channels (Meta/Facebook, Instagram via Meta, YouTube) are kept. The next adapters to build are:

1. **X** (formerly Twitter)
2. **LinkedIn**
3. **TikTok**

Each is implemented as a provider under the existing adapter contract (capability detection, credential encryption at rest, connection check diagnostics, idempotent writes — each provider's own idempotency semantics assessed separately), with OAuth-based onboarding for the channels that require it.

### First increment: X (Twitter) adapter + web flow (recorded 26 October 2026)

The first social-increment per section 15 is implemented. The X (Twitter) channel now works through the existing adapter contract with no changes to the publish pipeline, approvals, scheduler, or worker.

API (`apps/api`):

- `internal/integration/x.go` — new X adapter (`kind: "x"`, display name "X (Twitter)") on the same pattern as Meta:
  - Connection check is read-only: `GET /2/users/me` with the saved token, under the default base `https://api.x.com/2`.
  - Credentials: one OAuth 2.0 user access token (`accessToken`), sent in the Authorization header only, never in URLs.
  - Publishing: approval-gated `POST /2/tweets` with `tweet.write` scope; no destination-side draft (the shared `ErrNoDestinationDraft` sentinel). Post text comes from the `x` channel copy with title/excerpt fallback, an optional `postLinkPrefix` config tags the article link (prefix + slug), and the whole text is truncated to the documented 280-character cap.
  - Duplicate prevention mirrors Facebook: the account's recent tweets are scanned read-only and an exact text match is reused instead of re-posting; a failed identity read skips the scan without blocking the publish.
  - Error classification: 401 → `token_expired` (operator-safe message, reconnection instruction), 403 forbidden, 429 rate-limited, 5xx unavailable, others validation. Diagnostics never echo the token.
  - The resulting status URL (`https://x.com/{handle}/status/{id}`) is recorded as the artifact URL.
- `internal/integration/registry.go` — `XAdapter` and `XPublisherAdapter` registered in the default registry, so the capability check endpoint, publish endpoint, scheduler, and background worker all serve the new kind without further wiring.
- `internal/integration/publisher.go` — `PublisherNeedsReconnection` now also treats a rejected/expired X token as a credential problem (flagged for reconnection like Meta/YouTube/Sanity).
- Tests: `internal/integration/x_test.go` — fake-server contract tests: read-only check (token missing → limited, connected diagnostic naming handle + scope, rejected token → error with no token echo, server body never surfaced verbatim), publisher validation, draftless sentinel, request shape (JSON body, Bearer auth), duplicate reuse with zero writes, title/excerpt fallback, empty-message validation, 280-rune truncation, 401→`token_expired`→reconnection mapping, network failure not a reconnection problem.

Web (`apps/web`):

- `src/components/workspace-integrations-section.tsx` — X appears in the connect form (optional article link prefix), credentials payload saves the token as `accessToken` (previously it would have fallen through to the generic `apiKey` key), and the credentials label/hint describe the X token correctly.
- `src/components/draft-editor.tsx` — the social publish picker (`meta`/`youtube`/`x`) includes X integrations, so operators can publish now or schedule a tweet through the existing approval-gated flow; per-channel job/outcome records appear in the existing jobs panel.

Verification level (per the section 8 policy):

1. **Implemented / unit + contract tested:** all Go packages build, vet, and pass (`go test ./internal/... -count=1`, integration package including the new fake-server contract tests, with `gofmt -s` clean and store/integration database-backed tests re-run). Web: typecheck clean, all 171 Vitest tests pass, ESLint clean, Prettier clean on the edited files.
2. **Not yet verified and not claimed:** no live X account, no real tweet, and no real public page were exercised. External verification should follow the recorded controlled-test pattern (a clearly identified test post on an authorized account) before any "live posting" claim. Reuse the M15/Meta caution: a local duplicate scan is not proof a retried delivery cannot duplicate a post when the provider accepted a request whose response was lost.

Only X is built; LinkedIn and TikTok remain planned under section 15 D, following this same adapter pattern. The general agent layer of section 15 A is still planned, not started.

Separate observation: `npx prettier --check src` reports 15 pre-existing web files with formatting drift (excluding this increment's edits); they were left untouched to keep this change scoped.

### Status

Direction approved. Implemented: the X channel increment and the agent-layer registry increments 1–3 (below). Designed, not implemented: the LinkedIn/TikTok channels (section D below). Each future increment is recorded here with the evidence level it reached.

---

## 15.2 Agent-layer registry — increment 1 implemented (recorded 26 October 2026)

Per section A.1, the API-side pluggable agent layer is implemented for both existing specialists, strictly behavior-preserving for callers except the two documented renames below.

`internal/agents/registry.go` (new):

- `Specialist` interface: `ID/Version/DisplayName/Description`, `OutputContract()`, `AllowedActions()`, `WantsData()`, and one bounded `Run(ctx, PromptRun, RunInput)` where `RunInput{Workspace, Focus, Data}` carries only declared snapshots and `RunOutput` unions `*AdvisorReport`/`*ContentPlan`.
- `Registry` with `Register` (duplicate ID is an error), `Get`, sorted `ListDescriptors`, `NewDefaultRegistry` (both built-ins registered; built-in registration failure is a panic, i.e. a programming error).
- Governance is executable: `Register` rejects unknown contracts, contracts must be one of `advisory` (zero actions) or `drafting` (exactly `file_draft`), and declared data needs/actions must be known names. Closed sets: contracts `advisory|drafting`, action `file_draft`, data need `search_console`.

Migration of the specialists (behavior-identical): `SEOAdvisor` and `ContentCreator` keep `Advise`/`Create` (still used by package tests) and gained `Run` methods plus the dashboard-facing metadata; validation, field caps, and the GSC data-honesty gate are untouched.

`internal/httpapi` (per-specialist handlers deleted):

- `GET /api/v1/agents` — descriptor list for the agents panel (200 even before a runner is configured).
- `POST /api/v1/workspaces/{id}/agents/{agentId}/run` — the single shared pipeline: unknown agent → 404; store/runner unconfigured → 501; payload cap (8 KiB) and 2000-rune focus cap; workspace + brand context (missing brand tolerated); declared data loaders only (the GSC loader behind `AdaptGSCLoader`, missing/failed loads degrade to no-data with a note — same note strings); run timeout; contract check on the output; audit events `agent.run.failed` / `agent.run.completed` (scalar, secret-free details); advisory contract → `{report, dataNote, durationMs}` 200, drafting contract → file_draft executor filing the draft with the legacy checks (kind validated and existing-item access/status/match checked **before** any model call) and the same 201 envelope `{item, version, provider, model, agentId, keyPoints, durationMs, slugSuggestion}`.
- The two legacy routes (`/agents/seo/advisory`, `/agents/content-creator/draft`) do not exist anymore; unknown agent paths answer 404 (there is a regression test). The AI rate limiter now guards the generic run route.
- `cmd/api/main.go` wires `AgentLoaders["search_console"]` from the workspace store when a database is configured.

Web (`apps/web`): the two client functions keep their names/interfaces (components unchanged) but now hit the generic run path: `requestSeoAdvisory` → `.../agents/seo_specialist/run`, `requestContentDraft` → `.../agents/content_creator/run`. Vitest expectations and the Playwright content-creator stub updated to match.

Documented renames (the only intentional caller-visible changes):

1. Request/response shapes are unchanged, but audit action names changed: `agent.seo.advisory.*` → `agent.run.*` and `agent.content.draft.failed` → `agent.run.failed`; the drafting success action is now `agent.action.file_draft.completed` (was `agent.content.draft.created`), with the same details plus the action framing. Historical audit rows are unchanged.
2. Non-`ProviderError` run failures now answer 502 with a fixed safe message ("the agent could not complete this run") for the advisory contract too; previously the advisory handler echoed `err.Error()` verbatim (the drafting handler already had the safe pattern).

Verification level (section 8 policy): all Go packages build, vet, format, and pass `go test ./...`, including the migrated httpapi suite (no-data/prompt/GSC-snapshot/failure/negative cases), the migrated creator suite (draft filing, pre-run validation 400/413/404/409, failure-does-not-create-placeholder with unsafe-secret assertion, existing-item boundaries, offline stub), new registry unit tests (duplicate ID, capability gating, sorted descriptors), the unknown-agent/404 regression test, and the new `GET /api/v1/agents` test. Web: typecheck clean, 171/171 Vitest tests pass, ESLint clean, edited files Prettier-clean. Database-backed store tests were skipped in this environment (no `DATABASE_URL` configured); nothing in this increment touches the store schema. Not yet done at that point: first new specialist profiles (increment 3), action-executor classes beyond `file_draft` (increment 4).

---

## 15.3 Agent-layer registry — increment 2 implemented: the dashboard agents panel (recorded 26 October 2026)

The registry itself did not change; this increment is web-only and consumes `GET /api/v1/agents` so a newly registered specialist appears in the dashboard with no further UI work.

Web (`apps/web`):

- `src/lib/agents.ts` — `fetchAgentRegistry(baseUrl)` returns `AgentsRegistryState` (`loading | available {agents} | unavailable {message}`) and parses `GET /api/v1/agents` strictly: the envelope must have an `agents` array and every descriptor must carry non-empty `id` and `version`; a single malformed entry rejects the whole list as "unavailable" with a safe message, never a partially parsed descriptor. Missing display strings only fall back to the id.
- Format helpers `formatAgentContract`/`formatAgentDataNeed`/`formatAgentAction` map the registered capability names to operator-facing sentences ("Advisory only — returns a report, never edits", "Drafts into the human approval workflow", "Reads: Search Console data", "May file drafts in the approval workflow") and pass unknown names through verbatim.
- `src/components/workspace-agents-section.tsx` (new) — an "Enabled agents (registry)" card on the workspace page, mounted in `workspace-detail-panel.tsx` below the SEO Advisor section: per agent it shows display name, stable id, version chip, description, contract sentence, and per-data-need/per-action sentences; loading/unavailable render as plain text; an empty registry renders nothing.
- `e2e/content-creator.spec.ts` — the `/api/v1/**` route stub now answers `GET /api/v1/agents` with both built-in descriptors; without it the spec's "Unexpected API request" guard would throw.

Verification level (section 8 policy, per the scope of this increment): web typecheck clean; 176/176 Vitest tests pass (14 files), including new `fetchAgentRegistry` cases (list parse, malformed descriptor entry, non-array envelope, fixed server-error/network-failure messages) and descriptor format-helper cases; ESLint clean; Prettier-clean on the files this increment created or touched. The previously recorded pre-existing formatting drift in other files (including `workspace-detail-panel.tsx`) was left untouched to keep the change scoped. No API/Go changes were needed; API suites remain as verified in increment 1. Not yet verified: a live browser session (no Playwright run executed in this environment). Not yet done at that point: first new specialist profiles (increment 3), action-executor classes beyond `file_draft` (increment 4).

---

## 15.4 Agent-layer registry — increment 3 implemented: the third specialist proves the add-one-agent claim (recorded 26 October 2026)

The claim under test: a third agent costs a prompt profile + registration + declared data needs — no handler, no route, no loader, no store, no web client. Result: **confirmed.** The only files changed outside `internal/agents` are the offline stub branch and (test-only) assertions that counted exactly two agents.

The new specialist (`apps/api/internal/agents/searchgapwriter.go`, new):

- `SearchGapWriter` — id `search_gap_writer` v1, drafting contract, wants `search_console`, actions `{file_draft}`. It drafts one piece aimed at a real demand gap: among the provided Search Console page+query rows it targets the query where searchers saw the workspace most while clicking least, and never writes any metric into the prose. With no rows it drafts from the operator brief only and states the data basis in the excerpt.
- Data honesty enforced once, for all drafting agents, in `internal/agents/contentcreator.go`: the shared plan validator is now `planFromReply(reply, rows)` (previously `ContentCreator.validateReply`, a private method). `creatorReplyShape` gained the optional `targetQuery` hook; a run without rows must not name any query, and a run with rows must name a query that is verbatim among the provided rows. A valid data-informed target query becomes the first key point ("Target query: …", exported const `TargetQueryKeyPointPrefix`), so the operator sees the evidence in the filed draft. `SearchGapWriter.Run` calls the validator with the loaded snapshot; `ContentCreator` still calls it with nil.
- `registry.go` — `SearchGapWriter{}` added to `NewDefaultRegistry`. Nothing else changed in the registry or route code.

`internal/aiprovider/aiprovider.go` — the offline stub's drafting branch now also matches system prompts starting "You are the internal search-gap writer "; it deliberately returns no `targetQuery` (it cannot honestly confirm a query from rows it does not read, and the validator makes an empty value legal in no-data runs). Consequence worth recording: in an offline-stub deployment **with** a connected real Search Console loader, the gap writer correctly fails closed (502, malformed data-claim) rather than inventing a query.

Web: no client or component changes were needed — the agents panel lists the new descriptor automatically (increment 2 working as designed). The `content-creator` e2e route stub was updated to list all three descriptors so its "Unexpected API request" guard keeps matching the real registry.

Tests: new `TestSearchGapWriterDataHonesty` (verbatim/provided query accepted; reworded, fabricated, missing-with-rows, and query-without-rows rejected), a no-data full-plan assertion in the agents-package tests' spirit plus identity check, `TestSearchGapWriterRunThroughGenericRoute` (201, honest no-data marker, drafting brief present, single `agent.action.file_draft.completed` audit, and the offline stub end-to-end), the registry test now expects three valid built-ins, and `GET /api/v1/agents` asserts the third descriptor.

Verification level (section 8 policy): Go — full `go test ./...` passes (13 packages), `go vet` and `gofmt` clean; database-backed store tests remain skipped in this environment (no `DATABASE_URL`). Web — typecheck clean, 176/176 Vitest tests, ESLint clean, edited files Prettier-clean (no drift added). Not yet verified: a live Playwright session and any real model run of the new persona (via a real provider key and a connected Search Console integration). Not yet done: action-executor classes beyond `file_draft` (increment 4).

---

## 15.5 Agent-layer registry — increment 4 design recorded before implementation

Per section A.4, executor classes are designed and recorded here before they exist. Two classes are designed now; the second is recorded as the designed-next work, not built in this pass.

1. **`create_campaign` (campaign proposals) — implemented this increment (15.6).** New output contract `proposal`: the run's validated reply is a `CampaignProposal` (name, utmCampaign value, notes/rationale, keyPoints) that the executor files straight into the existing campaigns store with the status forced to `planned`. Activation (`planned → active`), pausing, edits, and deletion stay ordinary operator PATCH/DELETE actions; no proposal is ever active on arrival. Duplicate names surface the store's existing per-workspace uniqueness as the same 409 the manual form gets. Audit event: `agent.action.create_campaign.completed` (scalar details incl. resulting id, name, forced status, agent identity, provider/model).
2. **`file_channel_copy` (per-channel copy variants) — designed next, NOT yet implemented.** A drafting-adjacent specialist drafts the per-channel copy variants (e.g. X/Meta/YouTube message text) for an **existing** draft content item, filed into the item's newest version's `channel_copy` JSONB bead through the existing draft-save flow. It files nothing new, never creates items, and must refuse frozen items exactly like text edits; it inherits the approval workflow untouched. Open design point left for its implementing pass: whether it declares a fourth contract or extends `drafting` (the `drafting` capabilities rule currently pins exactly one action), and how the tool input selects the target item within the existing 8 KiB payload and pre-run validation pattern.

Data honesty rule carried into the new contract: a proposal may only use workspace context and the operator brief; it must never cite Search Console or other performance data it was not given, so the strategist declares no data needs.

---

## 15.6 Agent-layer registry — increment 4 implemented: the second action-executor class `create_campaign` (recorded 26 October 2026)

What landed, end to end:

**API.** `internal/agents/registry.go` adds the `proposal` output contract (`ContractProposal`) and the `create_campaign` action, with the capabilities check extended so a `proposal` specialist MUST license exactly `create_campaign` (licensing foreign actions is also rejected). `internal/agents/campaignstrategist.go` implements `CampaignStrategist` — `Run` wraps one `Propose` pass; the prompt bounds the specialist to workspace context plus the operator brief and forbids any performance data claim; the reply validator requires a non-empty `name`, accepts an optional `utm_campaign` and free-text rationale `notes`, caps name/utm/notes to the same rune budget as the campaigns forms, and carries provider/model identity. `internal/httpapi/workspace_agents_run.go` gains the second executor: a validated `CampaignProposal` is filed via `WorkspaceStore.InsertCampaign` with status forced unconditionally to `planned` (the operator alone activates), duplicate names surface `ErrSlugTaken` as the same 409 the manual form gets, and the run answers 201 with the proposal envelope (`campaign` + provider/model/agentId/keyPoints/durationMs). Audit event `agent.action.create_campaign.completed` records scalar details (resulting id, name, forced status, agent id/version, provider/model, has_utm, keypoints). `internal/aiprovider/aiprovider.go`'s offline stub answers the strategist's prompt with a clearly-labelled plumbing-check proposal (`[Stub]` notes; no performance claims), so the whole flow works with no provider key (mirroring `file_draft`).

**Web.** `src/lib/agents.ts` labels the new contract/action, adds `requestCampaignProposal` + `parseProposalEnvelope` (shape-checked envelope; server decides the status), and `src/lib/agents.test.ts` covers proposal formatting and the run-route round-trip. New `src/components/workspace-campaign-strategist-section.tsx` asks for one proposal (focus optional, 2000 chars), shows the proposed planned campaign, and calls back into the workspace detail panel so the campaigns list re-reads; it mounts in `workspace-detail-panel.tsx` above the campaigns panel.

**Tests.** API: `registry_test.go` expects four valid built-ins and pins the proposal↔`create_campaign` capabilities rule; `campaignstrategist_test.go` covers bounded prompt, identity/metadata, malformed-map rejection (FailMalformed), and over-cap trimming; `workspace_agents_test.go` covers the executor through the generic run route (planned status forced in the stored row, envelope shape, 201, audit details). Web: unit tests plus a new route-stubbed Playwright spec `e2e/campaign-strategist.spec.ts` (propose → planned proposal shown in both the specialist section and the campaigns panel).

Verification level (section 8 policy): Go — full `go test ./...` passes, `go vet` and `gofmt` clean. Web — typecheck clean, 181/181 Vitest tests, ESLint clean, Prettier applied. Not yet verified: a live Playwright session (real API + DB) for the new spec, and any real model run of the strategist persona. Proposed follow-on (unchanged from 15.5): `file_channel_copy` remains designed-not-implemented.

---

## 15.7 Agent-layer registry — increment 5 design recorded before implementation

The `file_channel_copy` executor of 15.5 (2) is now designed, resolving the two open points of that record:

1. **A fourth output contract, `channel_copy`** (not a `drafting` extension). The `drafting` capabilities rule stays frozen — it still pins exactly `file_draft` — so no existing semantics move. The new contract licenses exactly the new action `file_channel_copy` and declares no data needs, mirroring how `proposal` licenses exactly `create_campaign`.
2. **Target item via the existing run payload's `contentItemId`** (no new payload channel, 8 KiB cap unchanged). The pipeline pre-validates before any model call: the item must exist in the workspace (404 otherwise) and must be in status `draft` (409 otherwise). This is deliberately stricter than the text-edit frozen-refusal: filing copy into an `approved` or `rejected` item would silently reset it to `draft` and invalidate the human decision, which contradicts "inherits the approval workflow untouched" — so the agent files only into `draft` items and any status change stays an operator action.
3. **Closed channel set** `{facebook, instagram, x, youtube}` — exactly the social channels the consuming publishers read (`x.go` `ChannelCopy["x"]`, `metapublisher`/worker per-channel keys, `youtubepublisher` `ChannelCopy["youtube"]`). Reply keys are lowercase-normalized; unknown keys are rejected as malformed, blank values dropped, each message capped (280 runes — the X cap — with overflow refused, not truncated, so the operator never sees silently-shortened copy), and at most four entries with at least one non-empty. Previous channel copy is preserved: the executor merges new values over the newest version's existing channels, then files through the existing `SaveContentDraft` (which appends one new immutable version and keeps the item in `draft` unchanged). Prompt context = workspace/brand context + the draft's title and a bounded body excerpt + the operator focus; no, and never, any performance data.
4. **Audit** event `agent.action.file_channel_copy.completed` (item id/kind, channel count, agent identity, provider/model). Envelope: 201 with `{item, version, channels, provider, model, agentId, durationMs}`.
5. Offline stub answers the strategist-style labelled placeholder for all four channels; web gains a `Channel copy (agent)` workspace section (pick one draft item, optional focus, run → shows filed copy) and lib labels.

**Implemented and verified (registry increment 5).**
- `agents`: `ContractChannelCopy` ("channel_copy") licensing exactly `ActionFileChannelCopy` ("file_channel_copy"), no data needs; registry gate + descriptor tests. `ChannelCopyWriter` (id `channel_copy_writer`, v1): brief = workspace/brand context + target draft's title and a 1200-rune body excerpt + operator focus; validates the reply's `channelCopy` map against the closed set {facebook, instagram, x, youtube} (unknown keys malformed, blanks dropped, 280-rune cap with overflow refused). Tests cover the brief, reply validation, caps, and the Run wrapper.
- `httpapi`: the generic run route pre-validates `contentItemId` before any model call (400 no target / 404 unknown or foreign item / 409 non-draft item — draft-only is stricter than SaveContentDraft's editable rule so an approved/rejected human decision can never be reset by a run); the `file_channel_copy` executor merges the plan over the newest version's existing channel copy and files through `SaveContentDraft` (title carried, excerpt/body/CTA nil = carried forward, metadata = agent provenance), responds 201 `{item, version, channels(run's own), provider, model, agentId, durationMs}`, and audits `agent.action.file_channel_copy.completed` (EntityType content_item, item id, channel count of the run's new copy). Fake-store tests cover filing, merge-over-previous, provenance, brief contents, all pre-run failures with zero model calls, and store-error mapping.
- `aiprovider`: StubProvider answers the channel-copy system prompt with four in-cap placeholder channels; verified end-to-end through the route in tests and the offline e2e.
- Web: `lib/agents.ts` labels for the contract/action + `requestChannelCopy`/`ChannelCopyEnvelope`/`ChannelCopyState` (parse-checked; refuses to run without a picked item); new `WorkspaceChannelCopySection` (loads `/content`, lists draft items only, optional focus, shows per-channel filed copy + version) mounted in the workspace detail panel after the content-creator agent section, refreshing the content section on file; extended registry/labels tests (`requestChannelCopy` suite). `agents-panel-live` e2e still proves registry-driven listing (new specialist appears with no dashboard change).
- Checks: `gofmt` clean, `go vet ./...` clean, full `go test ./...` green (agents/httpapi/aiprovider included), `tsc --noEmit` clean, vitest 187 green, eslint + prettier clean on touched files, and the full Playwright suite (5 specs, including both live-API specs) green against a locally migrated Postgres + API on :8081.

---

## Operator access and API proxy — 31 October 2026

The dashboard now uses server-side operator Basic authentication and a same-origin API proxy. The browser no longer receives the API bearer key; the proxy reads `API_KEY` and `API_BASE_URL` from its server environment. Missing operator credentials fail closed, and unsafe API requests require the configured `WEB_ORIGIN`. `make dev` explicitly disables login on its loopback-bound dashboard for local testing and passes its selected API address to the proxy. Configuration and the required HTTPS/private-network precautions are documented in `docs/local-development.md` and `apps/web/.env.example`. Previously exposed `NEXT_PUBLIC_API_KEY` values must be rotated.

Verification: 189 web unit tests passed, including proxy and authentication tests; production build passed; a local production-server HTTP check confirmed 401 without login, 200 with valid credentials, 403 for cross-origin POST, and 502 when the upstream was unavailable. No live client data or external publication was exercised. This is single-operator access, not a complete production security audit; deploy only with HTTPS, protected API access, and appropriate login rate limiting at the deployment edge. Existing work toward real-account verification and pilot deployment remains open.

---

## Publishing job item-boundary fix — 31 October 2026

Cancel and retry operations previously checked workspace and job but not whether the job belonged to the content item named in the URL. The store now checks all three identifiers in the mutation/locked lookup; mismatches return 404 without changing the job or recording a successful audit. Existing correctly scoped actions remain available.

Verification: focused HTTP handler tests passed, including mismatched cancel/retry requests. Focused PostgreSQL store tests passed against a newly created isolated temporary PostgreSQL 16 container, including unchanged job state for item mismatches and successful retry with the correct item. No client database or external publishing destination was used. Pilot deployment and live publishing verification remain open.

---

## Multi-workspace Sanity onboarding and agent assessment — 31 October 2026

The operator confirmed that they have previously tested Sanity publishing. The specific account and end-to-end conditions of that test were not independently rechecked in this increment.

Sanity onboarding now collects integration name, project ID, dataset, document type, public article path and a project API token together inside a workspace. It creates the integration, separately encrypts the token, and automatically performs a read-only Sanity dataset query with the token. A failed or incomplete setup leaves the created connection visible for recovery; the token is not returned to the browser. The check never writes to Sanity and reports write permissions and site schema as unverified. New Sanity publishes and schedules require a connected lifecycle status; existing connections previously marked connected by the old configuration-only check should be rechecked. Scheduled publish requests reject past timestamps and attempts to silently reschedule an existing job; the approved-content scheduler and worker remain in use.

Assessment of agency agents: the five registered specialists advise, draft content, create planned campaign proposals or file channel copy; tests exercise their contracts, but no live model quality evaluation or real lead generation was performed in this increment. The workspace supports recording and tracking leads with source and campaign metadata, but there is no automatic website lead intake or lead prospecting source. AI must not invent contacts or claim verified lead results. A real lead pipeline needs an explicitly selected authorized source and workspace-specific intake/authentication, consent/privacy, spam controls, provenance and validation. No external publish, lead collection, or real client data change was performed.

Verification: Go integration, httpapi, agents and publishworker suites passed; web TypeScript check and 97 focused integration, agent, campaign and content tests passed. Sanity checks used a fake server, not a live client account. New guided form was not browser-tested. Read-only Sanity access does not establish write privileges, site-specific schema compatibility, or scheduled production delivery. The backend must remain running for queued jobs.

---

## Workspace website lead intake — 31 October 2026

Implemented an initial server-to-server website lead intake connector. Each workspace can create named connections and see the randomly generated secret once; only its SHA-256 hash is stored. Connection management requires operator authentication and revocation is workspace-scoped. The dedicated submission route bypasses operator auth only for that exact POST path and requires the connection secret, even in development. Submission IDs and payload hashes are stored in migration 0008; a transaction locks the connection and returns the same lead for identical retries, rejects different payloads under the same ID, and checks revocation. Leads are stored as form_api with the connection name as source; supplied UTM values are recorded without inferring campaign attribution. An in-process per-IP limiter is present; it is not distributed spam protection.

Workspace UI provides connection creation/revocation and one-time secret display. Server-side request format, retry behavior, HTTPS, privacy and abuse-control limitations are documented in docs/local-development.md. An individual website must still implement its own form forwarding, secure secret storage, durable retry/failure handling, consent and bot protection; no external website was changed or real lead sent. API authentication/lifecycle tests and isolated PostgreSQL workspace/concurrent retry tests passed, along with frontend typecheck and 16 campaign/connection client tests. Browser UI and a live website integration were not exercised.

---

## Multi-website lead connection workflow — 31 October 2026

Extended the workspace lead connection UI with per-connection submission and verification paths and reusable step-by-step instructions. A one-time secret must be hidden before another connection can be created, preventing inadvertent loss. The UI explicitly says that creating a connection does not connect a website or prove delivery.

Added POST `/api/v1/workspaces/{id}/lead-connections/{connectionId}/verify`, sharing the submission route's connection-secret authentication and rate limiter. It is read-only, creates no lead, and confirms only credential/route access; operator credentials, other workspaces and revoked secrets are rejected. Detailed production deployment, TLS, narrowly exposed API routes, secret handling, website form delivery, retries, consent and operational limitations are documented in docs/local-development.md. No website has been connected or deployed by this increment; websites without a backend and CRM integrations still need a suitable server-side connector.

Verification: focused API lead-intake and authentication tests passed; web typecheck and 16 campaign/lead connection client tests passed. No live website, deployment, browser UI or external connection was tested.

---

## Lead reply drafting agent — 31 October 2026

Closed the response half of the lead loop. Website intake stored enquiries that nobody could answer from inside the platform; a new specialist now drafts the first reply to one **existing** lead, stored on the lead for the operator to review, edit and send themselves.

- Migration `0009_lead_reply_drafts` adds `reply_draft_subject`, `reply_draft_body`, `reply_drafted_at` to `leads`, with a CHECK constraint keeping the three columns all-set or all-null. The draft is reviewable text only: `SaveLeadReplyDraft` updates nothing else, so the lead's status, source, campaign attribution and private notes are untouched. A drafted reply is **not** a contact, and the platform sends nothing.
- Registry increment 6: contract `lead_reply` licensing exactly the action `file_lead_reply`, no data needs (`internal/agents/registry.go`, capabilities gate + `LeadResponder` registration). `LeadResponder` (`lead_responder` v1, `internal/agents/leadresponder.go`) answers the lead's own words plus workspace/brand context and validates the reply before anything is stored: subject required (≤200 runes), body required (≤1500 runes), overflow refused rather than truncated, and no email address other than the lead's own — an invented address would send a human reply nowhere, so it fails as malformed. The prompt forbids invented prices, availability, dates, staff or commitments, any claim that something was sent or answered, and any performance data.
- `httpapi`: the generic run route accepts `leadId`; `prepareLeadReplyFiling` runs before any model call (400 no target, 404 unknown or foreign lead, 409 lead already `won`/`lost`, 400 lead with neither enquiry text nor a name) and builds a bounded brief from the lead's recorded fields only — operator notes are never included. The `file_lead_reply` executor stores the validated draft, audits `agent.action.file_lead_reply.completed` (lead id, unchanged status, agent identity, provider/model, lengths), and responds 201 `{lead, subject, body, provider, model, agentId, durationMs}`. `leadJSON` now carries the draft fields so the leads list shows them. The offline stub answers the new prompt with a labelled, address-free placeholder.
- Web: `lib/agents.ts` labels both new strings and adds `requestLeadReply` (refuses to run without a picked lead, shape-checks the envelope); new `WorkspaceLeadReplySection` lists open leads, shows the selected enquiry, takes an optional focus, and displays the stored subject/body in a read-only field with an explicit "nothing was sent, status unchanged" note; the campaigns panel marks a lead whose reply is drafted and shows the draft inline. Mounted in the workspace detail panel after the campaigns panel, refreshing it after a run.

Verification: `gofmt`/`go vet` clean; full `go test ./...` green (agents, httpapi, aiprovider included); the new store test passed against a freshly migrated isolated PostgreSQL 16 database (`TestLeadReplyDraftLifecycle`, temp database dropped afterwards); migration 0009 applied to the local development database. Web: typecheck clean, 197/197 Vitest tests, ESLint clean, Prettier applied; the full Playwright suite passes (6 specs, including a new route-stubbed `e2e/lead-reply.spec.ts`).

Two pre-existing harness defects surfaced while running that suite and are fixed: the production dashboard server failed every same-origin POST to the API proxy with 403 because Next derives its request origin as `localhost` while the suite and its bind address use `127.0.0.1` (the launchers now pass an explicit `WEB_ORIGIN`), and three route-stubbed specs did not answer the `/lead-connections` request added with website intake (plus one campaign-row selector that matched the lead-connections list first). Both had been red before this increment, which is why earlier records could not claim a green browser suite against the current dashboard.

Not verified: any live model run (a real provider key), sending a reply from the platform (deliberately out of scope — there is no email integration), and a real lead arriving from a connected website. Lead-reply drafts were per-lead text only at that point: sending and delivery tracking were added the same day (next section), and CRM sync remains future work.

---

## Per-workspace outbound email: alerts and sending replies — 31 October 2026

Closed the send half of the lead loop without centralising sending: every workspace connects **its own** email account, so each client keeps its own sending domain and reputation, and one client's key can never send another's mail.

- Migration `0010_email_delivery`: `workspace_email_settings` (one row per workspace: provider, from name/address, alert address, encrypted API key + key version, status `unverified|connected|error`, last verified), `email_deliveries` (kind `lead_notification|lead_reply`, recipient, subject, body, status `queued|sending|sent|failed`, attempts, next attempt, last safe error, provider message id, sent timestamp) and `leads.reply_sent_at`. Two partial unique indexes enforce the real-world invariants: **one new-lead alert per lead** (a replayed form submission cannot alert twice) and **at most one live reply per lead** (a double click cannot email a person twice, while a failed attempt stays retryable as a fresh delivery).
- `internal/email` is the provider client (Resend's single `POST /emails` call, `Idempotency-Key` per delivery). Failures are classified rather than flattened: a rejected key, an unverified from-address or a malformed address are terminal; 429/5xx/transport are retryable; a **timeout is terminal and explicitly "outcome unknown"**, because the message may already have been accepted and a retry would duplicate it. Provider response text never reaches an operator or a row — only a declared safe message does. From-headers strip line breaks and quote unsafe display names; display-name forms are refused in address fields.
- `internal/emailworker` drains queued deliveries exactly like the publishing worker (SKIP LOCKED claim, bounded exponential backoff, audit events `email_delivery_sent`/`email_delivery_failed`, one post-crash sweep). A credential rejection flags the workspace connection as `error` so the dashboard asks for a working key instead of failing quietly. An interrupted send becomes terminal with "check with the provider before sending again" rather than being replayed.
- `httpapi`: `GET/PUT/DELETE /workspaces/{id}/email-settings`, `POST .../email-settings/test`, `GET .../email-deliveries`, `POST .../leads/{leadId}/reply-send`. The key is encrypted with the platform credential key, never returned, and the API-omitted key means "keep the stored one". **Connected is only ever set by a provider-accepted test message** — no configuration-shaped shortcut. Sending requires a recorded lead with a usable address, a stored connection, and a subject/body (typed or the stored draft); the exact sent text is written back onto the lead so record and delivery agree, and the response is `202` with the queued delivery. A provider-accepted reply sets `reply_sent_at` and moves a `new` lead to `contacted` — the one status change the platform makes on its own, because a person explicitly asked for that message.
- New-lead alerts are queued best effort from both capture paths (manual/import creation and website intake), never blocking the capture, and only when the workspace set an alert address. There is deliberately **no platform-wide email credential**: with none configured, draft and connect flow all behave as before.
- Web: `lib/email.ts` (write-only key, strict response parsing, `Connected` only from a verified response) plus a new **Email sending (per workspace)** panel that collects from-name/from-address/alert-address/API key, explains the production steps (provider account, domain verification in DNS, key paste), sends the test message, disconnects, and lists recent outbound mail with its real provider outcome. The lead-reply panel now holds an **editable** subject/body and a send button (disabled until email is connected), shows the workspace's sending identity, and states that a queued reply is not yet a sent one; the campaigns list shows "reply sent <date>" once the provider accepted it.

Verification: `gofmt`/`go vet` clean; full `go test ./...` green, including new `internal/email` classification tests, `internal/emailworker` send/retry/credential-failure tests, `internal/httpapi` settings + test-send + reply-send tests, and store integration tests (dedupe, claim-once, backoff, terminal states, stuck recovery, lead marking) run against a freshly migrated isolated PostgreSQL 16 database (temp database dropped afterwards); migration 0010 applied to the local development database by the e2e stack. Web: typecheck clean, 214/214 Vitest tests (new `lib/email.test.ts`), ESLint clean, Prettier applied; full Playwright suite **6/6 passing**, with `e2e/lead-reply.spec.ts` extended to cover the connected-email state, the editable draft, the send request carrying the reviewed text, the queued notice, the lead's "reply sent" badge and the delivery log (the three other route-stubbed specs now answer the two new email requests).

Not verified: a real provider send (no live Resend account/key was used), delivery webhooks or bounce handling (status reflects the provider's acceptance of the API call, not inbox delivery), a scheduled follow-up sequence, CRM sync, and inbound reply capture. Sending is one reviewed message per lead, by design.

## Provider delivery webhooks: delivered, delayed, bounced — 31 October 2026

Sending only ever proved that the provider *accepted* a message. This increment closes the reporting half: the provider now tells the platform what happened afterwards, per workspace, and the delivery log shows it.

- Migration `0011_email_delivery_webhooks`: `workspace_email_settings.webhook_secret_ciphertext` (encrypted signing secret, NULL until connected), `email_deliveries.delivery_state` (`unknown|delivered|delayed|bounced|complained`), `delivery_state_at`, `delivery_detail`, and `email_provider_events` (PK `(workspace_id, provider_event_id)`) as the callback ledger. **`status` and `delivery_state` stay separate facts**: `sent` means the provider accepted the message, `delivery_state` means what the provider later proved. Messages sent before this increment keep `unknown` and are not guessed at.
- `internal/email/webhook.go` verifies Resend's Svix signature over `id.timestamp.body` (HMAC-SHA256 under the base64-decoded `whsec_…` secret, any of several space-separated `v1,` versions, constant-time compare) and refuses anything older or newer than the 5-minute tolerance, so a captured callback cannot be replayed. Payloads are decoded strictly; an unreadable or untyped payload is `ErrWebhookMalformed`, a bad signature/secret/stale timestamp is `ErrWebhookUnauthorized`. Bounce text is sanitized to one short control-free line before it can reach a row. Unknown event types map to nothing (`DeliveryStateFor`), so `email.sent` cannot overwrite a real delivery fact.
- Store `ApplyEmailProviderEvent`: one transaction that finds the delivery by `(workspace, provider_message_id)`, records the provider event id (an `ON CONFLICT DO NOTHING` insert makes retries idempotent), and moves `delivery_state` only when the new report is at least as new as the stored one. A late bounce arriving after a delivery report is recorded but cannot rewrite the newer fact, and a provider message id belonging to another workspace is never matched. `SetEmailWebhookSecret` stores/clears only the secret and never touches the sending connection or its verified status.
- `httpapi`: `PUT/DELETE /workspaces/{id}/email-settings/webhook` (encrypts the secret, never returns it, 409 when email is not connected) and `POST /api/v1/email/webhooks/{provider}/{workspaceID}`. The callback is the **only** route exempt from operator auth besides health and website intake — the provider has no operator key — and it authenticates itself with that workspace's own secret (`isEmailWebhookRequest`). It answers 401 on a bad signature, 404 for an unknown provider or a workspace with no stored secret, and 200 `{ignored:true}` for unsupported event types or unknown messages, because a non-2xx would only make the provider retry something nothing here can use. A real change is audited as `email_delivery_state_changed`.
- Web: the per-workspace email panel gained a **Delivery reports (provider webhook)** block that shows this client's own callback URL (`/api/v1/email/webhooks/resend/<workspace>` + the API base already configured), takes the `whsec_…` signing secret, allows removing it, and explains the provider-side steps (add endpoint, subscribe to delivered/delayed/bounced/complained, paste secret). `lib/email.ts` validates the secret's shape before any round trip, parses `webhookConfigured`/`webhookPath`/`deliveryState`, and the delivery list shows a **Delivered / Delayed / Bounced / Marked as spam** badge with the provider's detail, next to the unchanged send status.

Verification: `gofmt`/`go vet` clean and full `go test ./...` green, including new `internal/email` webhook tests (verified event, tampered body, wrong secret, stale replay, malformed payloads, detail sanitizing, event mapping), router tests (secret save/remove/409, delivered then retried then bounced, ignored unknown message/unknown event type, 401/404 refusals that never reach the store, operator auth still enforced while the callback works without a key), and the store test `TestEmailProviderEventsMoveDeliveryStateForward` (record → duplicate → out-of-order → newer bounce → cross-workspace → unknown message) run against the local PostgreSQL 16 database after migration 0011 applied. Web: typecheck clean, `src/lib/email.test.ts` 24/24 Vitest tests, ESLint and Prettier clean, and `e2e/lead-reply.spec.ts` passing (it now asserts the delivery-reports block and this workspace's own callback URL against the real dashboard build).

Not verified: a real Resend webhook delivery (no live provider account was used — the signature scheme is exercised with locally signed requests), inbound reply capture, and scheduled follow-up sequences.

## Retrying a failed lead reply from the delivery log — 31 October 2026

A failed send used to be a dead end: the log explained the failure, but the only way to try again was to open the lead and send from the reply panel. The log now owns that action, so the operator fixes the provider problem and retries where they see it.

- Store `GetEmailDeliveryInWorkspace` reads one delivery scoped to its workspace, so a foreign id is `ErrNotFound` rather than a cross-client read.
- `POST /api/v1/workspaces/{id}/email-deliveries/{deliveryId}/retry` queues a **fresh** delivery (`202`) from the failed row's logged recipient, subject and body — never the lead's current draft, which may have changed. The failed row stays as history. Refused with an explanation: anything that is not a `lead_reply` (a new-lead alert is deliberately once per enquiry), anything not in `failed` state (queued/sending/sent are not failures), a reply whose lead was deleted, a workspace with no email connection, and a lead that already has a live reply — that last check is the database's existing one-live-reply index, so a retry can never email a person twice. Audited as `email_delivery_retried` with both delivery ids.
- Web: `lib/email.ts` gained `retryEmailDelivery`; the delivery log shows a **Retry send** button on failed lead replies only, disables it while working, and reports the queued retry or the server's refusal. Refreshing the list afterwards shows the new row.

Verification: `gofmt`/`go vet` clean, full `go test ./...` green with new router tests (queued retry copies the logged text and audits it, sent/notification/lead-less/unknown/foreign refusals queue nothing, disconnected account, already-live reply) and the store test extended to read a failed row back within its own workspace only (run against the local PostgreSQL 16 database). Web: typecheck clean, `src/lib/email.test.ts` 27/27 Vitest, ESLint/Prettier clean, and `e2e/lead-reply.spec.ts` passing with the failed-row fixture, the retry click and the queued-retry confirmation.

Not verified: a live retry against a real provider (no provider account was used), and retrying a message whose delivery later bounced (the retry action is limited to `failed` sends).

## Resending a bounced reply — 31 October 2026

The retry action covered sends the provider refused. A bounce is the other way a reply never reaches the person: the provider accepted it and the recipient's server then refused it. The log now offers a resend for that case too.

- Migration `0012_bounced_reply_resend` narrows the one-live-reply index instead of inventing a new status: the predicate is now `status IN ('queued','sending') OR (status = 'sent' AND delivery_state <> 'bounced')`. A bounced message stays recorded as what it was — `sent` + `bounced`, with its provider id and bounce detail intact — but it no longer counts as live, so the resend can be queued. Everything else still blocks a second copy: queued, sending, delivered, delayed and complained all remain live. The index is strictly more permissive, so applying it cannot fail on existing rows.
- The retry endpoint accepts `failed` **or** `sent` + `bounced` and refuses everything else with "only a delivery that failed or bounced can be retried". It still copies the logged recipient/subject/body, still requires a connected account and an existing lead, and the audit event now records `wasBounced` so the resend is distinguishable from a retry of a rejected send.
- Web: `canRetryDelivery` decides the button (a bounced reply shows **Resend**, a failed send shows **Retry send**) and the confirmation is worded per case ("Resend queued to …" / "Retry queued to …"). Bounced replies that get resent stay in the log with their bounce badge, so the history is not rewritten.

Verification: `gofmt`/`go vet` clean, full `go test ./...` green (with the local PostgreSQL) including new router tests (a bounced reply resends, the queued resend then blocks another, a merely delivered reply is still refused) and the store test `TestBouncedReplyCanBeResent` (bounce → resend allowed → queued blocks → delivered blocks → bounce row untouched) after migration 0012 applied to the local database. Web: typecheck clean, `src/lib/email.test.ts` 28/28 Vitest, ESLint/Prettier clean, and `e2e/lead-reply.spec.ts` passing with a bounced fixture, the **Resend** click and its confirmation.

Not verified: a live bounce and resend against a real provider account; a resend of a reply that was marked as spam (complained is not treated as bounced — the person did receive it).

## Per-lead send history — 31 October 2026

A lead's alerts and reply attempts were only visible scattered through the workspace-wide delivery log. The lead reply panel now shows every message the platform tried to send about the selected enquiry, newest first, so the whole contact history reads in one place.

- Store `ListLeadEmailDeliveries(workspace, lead, limit)` filters `email_deliveries` by workspace **and** lead, newest first — the alert the enquiry produced plus each reply attempt, whatever its state. No migration: the existing `email_deliveries` rows already carry `lead_id` (and `ON DELETE SET NULL` means a deleted lead simply drops out).
- `GET /api/v1/workspaces/{id}/leads/{leadId}/email-deliveries` resolves the lead first, so an unknown or foreign id is a `404` instead of an empty list that would look like "nothing was ever sent to this person". Limit parsing matches the workspace log.
- Web: `fetchLeadEmailDeliveries` + a **Sending history for this enquiry** block in the lead-reply panel, loaded on selection and re-read after a reply is queued. Each row shows the kind, the send status and the provider's delivery report (`Bounced`, `Delivered`, …) as separate badges, the recipient, when it was sent, the subject and any bounce detail or safe error. Retrying/resending remains in the delivery log, where those actions and their confirmations live.

Verification: `gofmt`/`go vet` clean, full `go test ./...` green (with the local PostgreSQL) including new router tests (both attempts for one enquiry returned, another lead's message not mixed in, unknown/foreign lead 404, quiet lead returns an empty list) and the store test `TestLeadEmailHistoryIsOneEnquirysAttempts` (same workspace two leads + another workspace, empty history). Web: typecheck clean, `src/lib/email.test.ts` 31/31 Vitest, ESLint/Prettier clean, and `e2e/lead-reply.spec.ts` passing with the history request mocked and asserted (bounced and failed attempts visible under the selected enquiry).

Not verified: a live history against a real provider account (the data path is exercised with seeded rows and signed test requests).

## Delivery problems on the workspace overview — 31 October 2026

A failed send or a bounce only appeared inside one workspace's email panel, so an operator who never opened that workspace never saw it. The overview now flags every workspace whose log holds a send that failed or a message the provider reported as bounced.

- Store `EmailDeliveryAttention(ctx)` returns `map[workspaceID]{Failed, Bounced}` for workspaces with `status = 'failed'` or `delivery_state = 'bounced'`; clean workspaces are omitted so "nothing to look at" is an absent entry. A failed row and a bounced row cannot be the same message (`failed` means the provider never accepted it), so the two counts never overlap.
- `GET /api/v1/workspaces` gained an optional `deliveryAttention` field, present only when a workspace has problems. The listing treats it as a convenience banner: if the counting query fails, the workspaces are still returned without flags rather than the whole list 500ing. The field is filled through an optional interface, so stores that cannot count (fakes, minimal setups) simply report nothing.
- Web: `Workspace.deliveryAttention` is parsed (malformed counts reject the body like any other field, zero counts are dropped) and `describeDeliveryAttention` renders e.g. "2 failed to send · 1 bounced". The overview row shows a red pill linking to that workspace's delivery log; the log's container now carries `id="email-deliveries"` so the link lands on it.

Verification: `gofmt`/`go vet` clean, full `go test ./...` green against the local PostgreSQL, including `TestWorkspaceListFlagsDeliveryProblems` (one flagged workspace, clean workspace unflagged, list still complete) and `TestEmailDeliveryAttentionCountsOnlyProblems` (failed and bounced counted, delivered/delayed/queued ignored, clean workspace omitted). Web: `tsc --noEmit` clean, `src/lib/workspaces.test.ts` 25/25 Vitest (attention parsed, zero counts dropped, malformed counts rejected, label wording), ESLint/Prettier clean, and `e2e/overview-delivery-alerts.spec.ts` passing against the real stack with the workspaces list stubbed (badge text, link target with the log anchor, clean workspace unflagged).

Not verified: the badge against a real provider account that produces a real bounce (counts come from seeded deliveries).

## Dismissing acknowledged delivery problems — 31 October 2026

The badge was permanent: once a workspace had ever failed or bounced a message, the overview flagged it forever, so real problems drowned in history. An operator can now acknowledge the problems they have seen, and the badge only returns for problems recorded afterwards.

- Migration `0013_email_delivery_ack.sql` adds `email_delivery_acknowledgements (workspace_id PK, acknowledged_at)`. It is a timestamp, not a deletion — the delivery log keeps every message, so retry/resend still work.
- `AcknowledgeEmailDeliveryAttention(workspace)` upserts the timestamp and returns it. `EmailDeliveryAttention` now excludes problems whose own timestamp (`GREATEST(updated_at, delivery_state_at)` — when the send failed, or when the provider reported the bounce) is not newer than the acknowledgement, so a new failure or bounce raises the badge again.
- `POST /api/v1/workspaces/{id}/delivery-attention/acknowledge` validates the workspace (404 otherwise), records an `email_delivery_attention_acknowledged` audit event and answers `{acknowledgedAt}`. Local `emailStore` interface gained the method, so stores without it still answer 501 elsewhere.
- Web: `acknowledgeDeliveryAttention` in `lib/email.ts`; the overview badge is now a link to `#email-deliveries` plus a **Dismiss** button. On success the badge is dropped for that workspace and a status line says the problems were marked as seen and that new failures or bounces will show again; a refusal or unreachable API surfaces as an alert and leaves the badge in place.

Verification: `gofmt`/`go vet` clean, full `go test ./...` green against the local PostgreSQL, including `TestAcknowledgingDeliveryProblemsHidesOnlyOlderOnes` (older failure and bounce clear on ack, log still lists both, a later failure counts again, second ack clears it) and `TestAcknowledgingClearsDeliveryProblemsUntilTheNextOne` (list flag before ack, gone after, untouched deliveries, new failure re-raises, unknown workspace 404). Web: `tsc --noEmit` clean, `src/lib/email.test.ts` 35/35 Vitest (post target, server refusal, unreadable body, unreachable API), ESLint/Prettier clean, and `e2e/overview-delivery-alerts.spec.ts` passing against the real stack (badge + link, Dismiss posts, badge gone, status line explains new problems reappear).

Not verified: dismissing against a real provider bounce (the timestamp boundary is exercised with seeded rows).

## Delivery-problem count in the status panel — 31 October 2026

The status panel reported API and database health only, so a workspace with failing email was invisible until an operator scrolled to the workspace list.

- `countDeliveryAttentionWorkspaces(workspaces)` in `lib/workspaces.ts` counts flagged workspaces; the panel reuses `fetchWorkspaces` (same optional `deliveryAttention` field) rather than inventing a second endpoint.
- The panel gained an **Outbound email** row: green “No delivery problems” plus *nothing failed in N workspaces*, or an amber *“2 workspaces need attention”* with *“2 of 5 have a send that failed or a message the provider bounced”* and a **Review below** anchor to the list. A list that cannot be read says so instead of claiming all clear, and the count never blocks the health rows.
- Acknowledging in the list dispatches `DELIVERY_ATTENTION_CHANGED_EVENT` (`notifyDeliveryAttentionChanged`); the panel listens and re-reads, so the count follows a dismissal instead of going stale on the same screen.

Verification: typecheck clean, `src/lib/workspaces.test.ts` 26/26 Vitest (count helper: empty, unflagged, mixed), ESLint/Prettier clean, and `e2e/overview-delivery-alerts.spec.ts` passing against the real stack with both the list and the status panel reading the stubbed payload: the count and copy appear, and after Dismiss the count flips to “No delivery problems”.

Not verified: the count in a browser against a live provider generating a real bounce.

## Delivery-problem flag on the workspaces list — 31 October 2026

The `/workspaces` page listed every workspace with no signal about failing email, so an operator managing clients there had to open each one. Each row now carries the same red *“Email needs attention: 1 failed to send · 3 bounced”* pill the overview uses, linking to that workspace's `#email-deliveries` log. It reuses the already-parsed `deliveryAttention` field and `describeDeliveryAttention`, and a workspace with a clean log shows nothing.

Verification: typecheck clean (the change is rendering only; parsing is covered by the existing `workspaces.test.ts` suite), ESLint/Prettier clean, and the new `e2e/workspaces-delivery-flag.spec.ts` passes against the real stack with the list stubbed: badge text, link target with the log anchor, and no badge for a clean workspace.

Not verified: the flag driven by a real provider bounce (the payload is stubbed).

## Showing when delivery problems were last acknowledged — 31 October 2026

A dismissed badge said nothing about *what* had been dismissed, so an operator returning later could not tell whether the cleared state was recent or months old. The acknowledgement time is now part of the workspace payload and shown on both lists.

- Store `EmailDeliveryAcknowledgements(ctx)` returns `map[workspaceID]time.Time` from `email_delivery_acknowledgements`. `GET /api/v1/workspaces` gained an optional `deliveryAcknowledgedAt` (RFC3339) per workspace, filled through another optional interface so stores and fakes without the table simply omit it; a lookup failure drops the field instead of failing the list.
- Web: `Workspace.deliveryAcknowledgedAt` is parsed (absent/null dropped, empty string or non-string rejects the body). Both the overview rows and the `/workspaces` rows show a muted *“Last acknowledged 31/10/2026, 09:00 — the badge/flag shows what failed or bounced since”*, or *“nothing new has failed or bounced since”* when the badge is clear. A `Dismiss` writes the returned `acknowledgedAt` into local state, so the line appears immediately without a refetch.

Verification: `gofmt`/`go vet` clean, full `go test ./...` green against the local PostgreSQL, including the store test now asserting `EmailDeliveryAcknowledgements` returns the exact acknowledgement, and the router test asserting the field is absent before the first acknowledgement and present after it. Web: `tsc --noEmit` clean, 62/62 Vitest across `workspaces.test.ts` (acknowledgement parsing) and `email.test.ts`, ESLint/Prettier clean, and both `e2e/overview-delivery-alerts.spec.ts` (line appears after Dismiss) and `e2e/workspaces-delivery-flag.spec.ts` (acknowledged workspace explains what the flag covers, clean workspace has no line) passing against the real stack.

Not verified: the timestamp in a browser produced by a real acknowledgement against a live provider.

## 16. Immediate handover summary

The project has progressed beyond its original foundation stage. The recorded work includes a provider-neutral publishing contract, a first Meta social-publishing increment, a scheduler/worker/retry increment, media support, a campaign-and-lead increment tested locally, and an SEO Specialist advisory extension.

The approved forward direction (section 15) adds an agency-style agent layer and a visible, approval-gated automatic social-posting flow, with X, LinkedIn, and TikTok as the next channel adapters — all built on top of the existing architecture.

However, the latest recorded status does not establish that every milestone is complete. In particular, live Meta publishing, a complete multi-client Meta OAuth lifecycle, live Sanity write/publish verification, live OAuth-backed Google Search Console data, full reporting, complete hardening, and the final pilot release still require verification or implementation unless the current repository shows that the work has since been completed.

**The next action is a repository audit—not a rewrite.** Confirm the current state, run the relevant checks, identify the highest-priority missing acceptance criterion, implement the smallest appropriate change, verify it, and update this document.

This file is intended to preserve the reasoning behind the roadmap as well as its implementation history, so future assistants can continue the project without losing earlier work or confusing planned capabilities with verified behavior.
## Stable public entrypoint via Cloudflare Worker — 1 November 2026

Platform deployed on Neon (branch `production`, all 16 migrations applied) and now served behind the
stable URL **https://marketing-ops-entry.marketing-ops-team.workers.dev** — a Cloudflare Worker in the
new `marketing-ops` Cloudflare account (free `workers.dev` subdomain `marketing-ops-team` registered via the
API because wrangler v4 dropped the `subdomain` command). The Worker (source under
`infra/cloudflare-worker/`, deployable with `./update-target.sh <tunnel-url>`) proxies to the rotating
trycloudflare quick tunnel that fronts Next.js :3000, which proxies `/api/v1` to the Go API :8081 on
Neon. `WEB_ORIGIN` on the web process must equal the stable URL or non-GET API writes 403. A named
cloudflared tunnel exists for the future (bind a real domain with `cloudflared tunnel route dns
marketing-ops <host>`); without a domain it cannot get a public hostname. Full details:
`docs/stable-entrypoint.md`.

Not yet production-hardened: the entrypoint is unauthenticated (`WEB_AUTH_DISABLED=true`, API has no
`API_KEY`). Turning on real auth requires `OPERATOR_USERNAME`/`OPERATOR_PASSWORD` on web and
`APP_ENV=production` + `API_KEY` on the API.
