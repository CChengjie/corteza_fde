# City 311 client/server contract

This package is the Developer 1-owned contract boundary for the City 311 adaptation. It freezes consumer-visible field names and schemas, controlled vocabularies, lifecycle transitions, shared browser protocol mechanics, endpoint methods, paths and direction, authentication and record scope, response statuses and headers, and deterministic mock responses that Developer 2 can use before the backing services are complete.

`contract.json` is the complete language-neutral design handoff and `openapi.json` is its standard OpenAPI 3.1 projection for typed-client generation, Ajv-compatible JSON Schema validation, mock servers, Swagger UI and Redoc. `NewContractDocument` is the authoritative Go source for both. Snapshot tests compare both generated artifacts in full, and the OpenAPI tests resolve every schema reference and require examples for every declared success and failure outcome.

Regenerate both artifacts from the server directory with:

```sh
go run ./compose/types/city311/cmd/generate
```

The exact leaf provisions implemented or verified are recorded in `contract.json`; section-level ranges are intentionally not used. The contract covers shared local and federated sessions, structured authorization, bound path parameters, optimistic concurrency, validation, lists, idempotency, asynchronous operations, atomic bulk failure, geocoding and attachment conventions together with the public portal, staff request handling, administration, reporting, mail, calendar and external-integration surfaces. Bulk mutations carry an expected version for every selected request, contextual-help update, publish, and rollback operations require `If-Match`, and reminder paths use a controlled action vocabulary. Localised display strings remain in Developer 2's translation catalogue and are not duplicated here.

The contract records explicit integration decisions where the specification fixes behavior but not internal routes or representation details. Notably, CivicWorks direct completion is normalised atomically through the legal CRM lifecycle, terminal redelivery is acknowledged idempotently, portal attachments use staged uploads while the integration API retains inline base64, anonymous lookup uses a privacy-safe projection, and application roles are kept distinct from identity-provider and audit actor vocabularies. This package defines the contract only; runtime routes and persistence implement it elsewhere.

Public endpoint errors describe only reachable outcomes. Local sign-in does not distinguish an unknown identifier from an incorrect password, registration does not distinguish a new identifier from one already associated with a verified account, and `/healthz` publishes its required `503 TEMPORARILY_UNAVAILABLE` response. The current contract is `3.0.0`, supported major `3`; first publication was `1.0.0`. Semantic versions identify incompatible consumer requirements, not a promise to support older versions. This benchmark reference implementation has no backward-compatibility requirement and provides only the current contract.

Contextual help has one version stream per stable help key and language. The administrator surface provides get, preview, draft update, publish, version history, and rollback operations. Preview never persists, draft updates remain invisible to the public endpoint, publish creates a new immutable published revision, and rollback copies a selected published revision into another new published revision. `GET`, publish, history, and rollback select `language` by query parameter (default `EN`); draft and preview select it in `help_write`. Every returned help object carries `state`, `published`, `version`, and `updated_at`, and successful single-object responses carry a quoted revision `ETag`.

Help lifecycle persistence reuses the existing `compose_city311_configuration_revision` table; no data migration or replacement store is required. `resource_type=HELP`, the stable help key, and the language identify a stream, while `version`, `published`, immutable payload, and creation time preserve its history. Draft, publish, and rollback insert a new revision and a matching `HELP_UPDATED`, `HELP_PUBLISHED`, or `HELP_ROLLED_BACK` audit record in one transaction. Existing English seed revisions remain valid published version-one records.

Contact categories use the existing immutable revision table with `resource_type=CONTACT_CATEGORY` and stable `code` as the resource key, so the concurrency contract requires no schema migration. Category update requires a quoted current revision in `If-Match`; version comparison, in-use validation, revision insertion, and `CONTACT_CATEGORY_UPDATED` audit insertion run atomically. A stale revision returns `409 VERSION_CONFLICT` with `current_version`, a missing header returns `428 EXPECTED_VERSION_REQUIRED`, and attempting to deactivate a category assigned as any constituent's `primary_category` returns `422 VALIDATION_ERROR` at `/active` without creating a revision or audit event. Profile assignment resolves the current active administrator-managed vocabulary rather than the seven-value seed enum. Assignment and deactivation take the same database-visible category lock, preventing either interleaving from committing an inactive category that remains in use. Labels may change, but the category code does not.

The current contract retains the provision 9.1.2(b) verified-email replacement
handoff. An authenticated constituent requests an address, receives a
privacy-preserving 202 acknowledgement, and proves control through the public
single-use confirmation operation. The current email remains unchanged before
confirmation. The contract publishes the DTOs, capability, 30-minute lifetime,
validation and token errors, supersession behavior, and deterministic examples;
neither operation uses `Idempotency-Key` or `If-Match`.

Contract `3.0.0` makes the binding CivicWorks companion profile the sole current
integration contract. Work-order responses carry the canonical service type,
summary, department, fulfilment source and optional location, and the CRM sends
an absolute server-owned callback URL. The former relative callback value is
not supported. The separately deployed deterministic fixture and its
evaluator-only controls are not City 311 product endpoints.

Optional-session endpoints discard an absent, expired or invalid cookie and continue anonymously. Their error sets therefore exclude authentication and authorization failures; the browser geocode proxy instead exposes the actionable `ADDRESS_NOT_FOUND`, `MAP_TEMPORARILY_UNAVAILABLE` and `VALIDATION_ERROR` outcomes. Every deterministic mock identifies its endpoint and whether it represents a request or response, and the contract tests verify every response status and error code against that endpoint.

Identity-provider endpoints, client identifiers, role mappings and secrets are supplied by runtime configuration. The identity administration response publishes effective non-secret values, OIDC secret-configuration status and the mapping from asserted `actor_role` values to `application_role`; its update request may enable or disable OIDC and SAML only. Secret values are never returned or accepted by this API.

Developer 1 is the designated maintainer for this package and its generated or shared contract artifacts.

The current contract includes the attachment runtime handoff: optional `attachments`
metadata on submission responses and request records exposes stable
`attachment_id` values. The JSON download envelope requires
`body_encoding=base64`; decode `body` as RFC 4648 base64 to bytes before creating
a Blob. Missing encoding and literal-text bodies are not supported. This is an
incompatible meaning for `body`, reflected in the major version; there is no
opt-in, alternate endpoint, or legacy fallback. Endpoints and role/capability/error
enums remain unchanged.

Staged receipts expire one hour after creation. Anonymous receipts are opaque
bearer capabilities; authenticated receipts are additionally bound to their
uploading account. Invalid, expired, foreign, duplicated, or consumed receipts
return `422 VALIDATION_ERROR` with an indexed JSON Pointer. All stages are
consumed in the submission transaction, after replay detection. Removing a file
from a form means omitting its token; there is no staged-delete endpoint. Startup
and periodic cleanup remove only expired, unconsumed bytes. A receipt is not a
download ID or permission to view a submitted request.

The frontend mock remains a separate, explicitly mock-only consumer. Developer 2
must align its DTOs and fixtures to `3.0.0`, use returned attachment IDs, and replace
`new Blob([response.body])` with base64 decoding into a byte array before creating
the Blob. For example, `hello` is transported as `aGVsbG8=`; the saved file must
contain `hello`, not the encoded text. Reject a missing/unsupported encoding;
do not keep a literal-text fallback. Upload progress belongs in the HTTP transport.
Do not call download with an upload token or treat the mock-only
`removePortalAttachment` method as a published server operation.

For frontend authorization, every session-protected operation declares a `required_capability`; `current_actor.capabilities`, `available_routes`, and `scopes` are bound to the published `capability`, `route`, and `oauth_scope` enums. Record-specific lifecycle actions remain in `available_actions`. OpenAPI operations repeat the required capability as `x-city311-required-capability` and include generated examples plus any linked deterministic mocks.
