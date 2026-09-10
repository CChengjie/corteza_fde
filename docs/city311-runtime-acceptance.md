# City 311 runtime acceptance

The root Compose project is the supported local backend runtime. It builds the
current repository source, waits for PostgreSQL, applies all store upgrades,
imports the standard source-tree provision bundle, and serves the City 311 API
below `/api/v1`. It also starts the binding deterministic CivicWorks fixture on
the internal `http://civicworks:8080` service address and host port `8081` by
default.

## Start the runtime

```sh
cp .env.example .env
docker compose up --build --detach
curl --fail http://localhost:8080/healthz
curl --fail http://localhost:8081/healthz
```

The ready response contains the following required fields; additive non-secret
readiness fields are permitted:

```json
{"status":"ok","database":"ok"}
```

`postgres_data` owns the PostgreSQL data directory and `attachment_data` owns
the Corteza object-store path. `docker compose down` retains both volumes;
`docker compose down --volumes` intentionally removes them.

`DATABASE_URL` is copied to Corteza's `DB_DSN`. If the PostgreSQL credentials
are changed, update `DATABASE_URL` at the same time. Every placeholder secret
in `.env.example` must be replaced before using the runtime outside a local
workstation. Non-CivicWorks `.invalid` integration endpoints are intentionally
inert and should be replaced with deployment-specific endpoints.

The public `BENCHMARK_*`, `CRM_API_CLIENT_*`, `MAIL_*`, `MAP_*`, consumer-side
`CIVICWORKS_*`, `WORKFLOW_*`, `OIDC_*`, and `SAML_*` inputs are passed directly
to the source-built server. `CIVICWORKS_CONTROL_TOKEN` is passed only to the
fixture, never the app or browser. Readiness returns HTTP 503 when a required
value is missing or malformed. City 311 mail settings are also mapped to
Corteza's `SMTP_*` settings so account-security and domain mail use the same
fixture.

## Run acceptance checks

Static checks do not require a running Docker daemon:

```sh
./scripts/runtime-acceptance.sh --static
```

The full check uses an isolated `city311-acceptance` Compose project, app port
`18080`, and fixture port `18081`. It removes only that project's old volumes,
starts from a clean database, creates representative data through the HTTP API,
restarts only the application twice, and verifies:

- `/healthz` reports `status=ok` and `database=ok` within 120 seconds;
- the CivicWorks fixture passes deterministic reset, create, idempotent replay,
  and source-case lookup checks;
- a CRM request is assigned to CivicWorks, signed status callbacks advance it
  through `IN_PROGRESS` to `RESOLVED`, and exact-event redelivery is idempotent;
- clean and already-migrated databases both start successfully;
- repeated `UPGRADE_ALWAYS=true` upgrades are idempotent;
- the eight canonical public requests `SR-2026-00033` through
  `SR-2026-00040`, their seed audit/history rows, and all eight seed actor
  identities are installed exactly once and remain unchanged after each
  application restart;
- the PostgreSQL container is neither replaced nor restarted;
- account/session, request, draft, workflow, audit, attachment, integration,
  and object-store volume state remain readable after each restart.

```sh
./scripts/runtime-acceptance.sh
```

The script removes the isolated runtime and its volumes after completion. Set
`KEEP_RUNTIME=1` to leave it running for inspection. `CITY311_ACCEPTANCE_PORT`,
`CITY311_ACCEPTANCE_CIVICWORKS_PORT`, and `CITY311_ACCEPTANCE_PROJECT` may be
used to select other isolated ports or a project name beginning with
`city311-acceptance`.
