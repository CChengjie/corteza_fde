# City 311 implementation audit

## Scope and evidence

This audit assesses the working tree based on the requested upstream `2024.9.x` revision `de7eba4c82970b8a6011ab740e713b9621ebf4ae`, plus the account-deletion and CI coverage changes in this submission.

The controlling source is `implementer-specification-draft-2026-08-23.docx` and its Chinese two-developer restatement. The Buffalo RFP comparison is used only as background traceability, not as a replacement for the controlling specification.

Evidence collected on 2026-09-14:

- PASS: `go test ./compose/service/city311 ./compose/rest/city311 ./compose/types/city311`.
- PASS: `./scripts/runtime-acceptance.sh --static`.
- PASS: `python3 -m py_compile tools/c311-browser/fe02_matrix.py tools/c311-browser/fe09_matrix.py`.
- PASS: static provider-to-router scan after removal of browser calls to external-only operations.
- NOT RUN: Compose and Admin C311 Jest tests. The workstation has no `corepack`, `yarn`, or local `node_modules`.
- INCOMPLETE: `./scripts/runtime-acceptance.sh`. The isolated build reaches the deterministic Docker frontend dependency step, but does not finish in this workstation execution window and creates no service containers. This is not a passing end-to-end result.
- Static route tracing confirms browser provider -> `/api/v1` -> frontend Nginx `/api/` proxy -> application `/api/v1` City311 router.

Legend: `[x]` verified by executed evidence; `[-]` implemented but only static/unit evidence or incomplete surface coverage; `[ ]` missing, broken, or unverified.

## Requirement checklist

### Baseline, roles, and platform requirements

- [-] 3.1-3.4 baseline preservation, repeatable migration, seeded data, restart persistence: runtime script has explicit checks, but the full runtime check did not complete.
- [-] 4.1-4.3 actor roles, department/district scope, platform override audit, API 401/403, and UI denial states: server tests and route guards exist; browser and clean-runtime verification is incomplete.
- [-] 6.1 and 13.1-13.5 container launch, health, configuration, volumes, deterministic seed/time: compose and static contract pass; full launch and restart acceptance remains unverified.
- [-] 6.2-6.4 responsive widths, supported browsers, WCAG 2.2 AA and keyboard/focus behavior: component and browser-matrix scripts exist, but their tests were not runnable locally.
- [-] 6.5-6.7 server-side authorisation, cookies, sanitisation, secret confinement, timezone and public languages: implementation exists; no complete browser/security regression was run.

### Functional requirements

- [-] 7.1 branding and public content: REST endpoints and partial management UI exist. The deployed Compose administration workspace omits logo/favicon/wallpaper/font selection, preview at three widths, version history and rollback controls.
- [-] 7.2 refresh and unsaved-edit continuity: dirty-form helper and route handling exist, but no executed browser regression proves refresh/route behavior.
- [-] 7.3 workflow administration: backend, protected manual-action route, Admin editor, and execution log exist. Compose now builds and starts the Admin client; full browser/runtime evidence remains outstanding.
- [-] 7.4 ICS import/export: backend routes and Admin extension UI exist; no end-to-end fixture verification was completed.
- [-] 7.5 federated identity: authenticated users explicitly confirm a link before the federated redirect; the server validates that confirmation in the protected start/callback flow. Supplied OIDC/SAML fixtures are not proven end-to-end.
- [-] 7.6 service-request submission and idempotency: portal, staff, and integration REST operations exist and backend tests pass; browser-to-runtime confirmation is incomplete.
- [-] 7.7 attachments and mapping: API and portal controls exist; mapping fixture and binary upload behavior were not run end-to-end.
- [-] 7.8 constituents, notes, reminders, linked parties: service/REST coverage exists and partial staff UI exists; constituent search/detail and full collaboration journeys are not delivered in the standard Compose frontend.
- [-] 7.9 assignment, drafts, status lookup, duplicate grouping, bulk: backend and selected public/staff UI flows exist; the full interaction and atomicity matrix was not run.
- [-] 7.10 audit, origin class, mail, contact export: backend routes exist; the standard Compose UI does not expose a complete audit/mail/export workspace.
- [-] 7.11 account maintenance: profile, password, identifier, replacement, deletion, and protected federated linking services exist. Full browser regression remains outstanding.
- [-] 7.12 reports, custom fields, data export: backend and Admin-only extensions UI exist; standard delivery does not build that Admin frontend, and no full report/export workflow was executed.
- [-] 7.13 contextual help: public/admin endpoints and public help drawer exist; no complete administrator editing/localisation/keyboard regression was run.

### Customer experience, data, workflow, and integrations

- [-] 8.1-8.4 public/staff navigation, feedback, status visibility, localisation and responsive continuity: selected surfaces are implemented; all supported browser and viewport assertions remain unverified locally.
- [-] 9.1-9.7 constituent/request schemas, links, attachments, locations, custom fields, audit and retention: City311 types, store adapters, services, and REST tests exist. Runtime persistence and legacy-data migration were not completed.
- [-] 9.8 reports and CSV export: service/REST and Admin-only UI exist; delivered frontend and end-to-end evidence are incomplete.
- [-] 10.1-10.7 lifecycle, CivicWorks, bulk atomicity, reminders, password/mail lifecycle, and concurrency: service/REST tests pass; fixture callbacks and application restarts were not completed in this environment.
- [-] 11.1 CivicWorks and 11.2 mapping: endpoint/configuration code and deterministic fixture are present; full fixture exchange did not run.
- [-] 11.3 OIDC/SAML: protected confirmation, start, and callback routes are implemented; no fixture run was completed.
- [-] 11.4 workflow OAuth action: the protected CRM action route and OAuth client are implemented and covered by service/REST tests; no fixture run was completed.
- [-] 11.5 mail and 11.6-11.7 request/data-export APIs: backend routes/services exist; no completed runtime execution against mail fixture or integration OAuth client.
- [-] 12.1 public views: Home, catalogue, submit, status, help, sign-in, register, password, account, and my-request routes exist in Compose.
- [ ] 12.2-12.5 complete staff/admin surfaces and interaction contract: only a subset is in the standard Compose image; no dedicated delivered interface for the full required queue/detail/constituent/report/workflow/admin scope.

## Frontend-to-backend connectivity

- [x] Transport path: `C311FetchTransport` sends same-origin requests; the production config uses `/api`; `docker/frontend-nginx.conf` proxies `/api/` to `app:80`; `server/app/servers.go` mounts City311 under `/api/v1`.
- [x] Backend contract: service and REST package tests pass, covering the mounted City311 handlers.
- [-] Provider-to-router coverage is broad for sessions, portal submission, status, drafts, staff requests, admin configuration, reports, mail, calendar, and exports, but it is not complete.
- [x] INT-01 resolved upstream: federated start requires an authenticated session and explicit `link_confirmed=true`; REST tests cover the flow.
- [x] INT-02 resolved upstream: `/api/v1/actions` is mounted behind identity authorization and delegates to the workflow service; REST tests cover success, idempotency, and denial.
- [x] INT-03 resolved upstream: Compose builds and exposes both `frontend` and `admin` services from the shared frontend Dockerfile.

## Defects and missing work

| ID | Priority | Finding | Requirement impact | Recommended correction |
| --- | --- | --- | --- | --- |
| INT-01 | Resolved upstream | Account linking now has an explicit authenticated confirmation contract. | 7.5.3-7.5.4, 11.3.6, 12.1 | Retain REST and browser regression coverage. |
| INT-02 | Resolved upstream | Workflow action route is now mounted and authorised. | 7.3.5, 11.4, 12.2 | Retain REST and browser regression coverage. |
| INT-03 | Resolved upstream | The supported Compose launch now includes the Admin client. | 2.1.4, 7.3, 7.10, 7.12, 12.2, 13.1 | Complete full runtime acceptance. |
| VAL-01 | P1 | Full runtime acceptance cannot currently be claimed: frontend build stalled before service creation. | 3.3, 6.1, 13.1-13.3, 14 | Make dependency installation deterministic/cacheable in the execution environment; then run the complete isolated acceptance script and retain its result. |
| VAL-02 | P1 | Frontend Jest and browser matrices were not runnable because local package tooling/dependencies are absent. | 6.2-6.4, 8, 12, 14 | Provision the documented Node/Corepack/Yarn toolchain, install lockfile dependencies, and run Compose/Admin C311 tests plus FE-01 through FE-10 matrices. |
| UI-01 | P1 | Deployed Compose AdminWorkspace is materially narrower than the stated administration contract; several administrator capabilities are only in the non-deployed Admin client or not surfaced. | 2.1.4, 7.1, 7.3, 7.10, 7.12, 12.2 | Inventory each administrator requirement against a deployed route and add missing controls, including preview/version/rollback and operational views. |
| VAL-03 | Resolved | FE-08 and FE-09 browser matrices were not invoked by CI. | 6.2-6.4, 8, 12 | Both matrices and their artifacts are now included in the C311 release-gate job. |

## Acceptance status

The current frontend and backend have no remaining known browser calls to unmounted CRM routes: the transport/proxy/server mounting path and City311 service, REST, and contract tests pass. The implementation is still **not ready to close** until the full container and browser acceptance suites complete in a provisioned environment. The remaining blockers are VAL-01 and VAL-02.
