# City 311 runtime acceptance

The root Compose project is the supported local backend runtime. It builds the
current repository source, waits for PostgreSQL, applies all store upgrades,
imports the standard source-tree provision bundle, and serves the City 311 API
below `/api/v1`.

## Start the runtime

```sh
cp .env.example .env
docker compose up --build --detach
curl --fail http://localhost:8080/healthz
```

The ready response is exactly the following two-field JSON object (field order
is not significant):

```json
{"status":"ok","database":"ok"}
```

`postgres_data` owns the PostgreSQL data directory and `attachment_data` owns
the Corteza object-store path. `docker compose down` retains both volumes;
`docker compose down --volumes` intentionally removes them.

`DATABASE_URL` is copied to Corteza's `DB_DSN`. If the PostgreSQL credentials
are changed, update `DATABASE_URL` at the same time. Every placeholder secret
in `.env.example` must be replaced before using the runtime outside a local
workstation. The `.invalid` integration endpoints are intentionally inert and
should be replaced with deployment-specific endpoints.

The public `BENCHMARK_*`, `CRM_API_CLIENT_*`, `MAIL_*`, `MAP_*`,
`CIVICWORKS_*`, `WORKFLOW_*`, `OIDC_*`, and `SAML_*` inputs are passed directly
to the source-built server. Readiness returns HTTP 503 when a required value is
missing or malformed. City 311 mail settings are also mapped to Corteza's
`SMTP_*` settings so account-security and domain mail use the same fixture.

## Run acceptance checks

Static checks do not require a running Docker daemon:

```sh
./scripts/runtime-acceptance.sh --static
```

The full check uses an isolated `city311-acceptance` Compose project and port
`18080`. It removes only that project's old volumes, starts from a clean
database, creates representative data through the HTTP API, restarts only the
application twice, and verifies:

- `/healthz` reaches the exact ready contract within 120 seconds;
- clean and already-migrated databases both start successfully;
- repeated `UPGRADE_ALWAYS=true` upgrades are idempotent;
- the PostgreSQL container is neither replaced nor restarted;
- account/session, request, draft, workflow, audit, attachment, integration,
  and object-store volume state remain readable after each restart.

```sh
./scripts/runtime-acceptance.sh
```

The script removes the isolated runtime and its volumes after completion. Set
`KEEP_RUNTIME=1` to leave it running for inspection. `CITY311_ACCEPTANCE_PORT`
and `CITY311_ACCEPTANCE_PROJECT` may be used to select another isolated port
or a project name beginning with `city311-acceptance`.
