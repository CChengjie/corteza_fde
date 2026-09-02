# City 311 client/server contract

This package is the Developer 1-owned contract boundary for the City 311 adaptation. It freezes consumer-visible field names and schemas, controlled vocabularies, lifecycle transitions, shared browser protocol mechanics, endpoint methods, paths and direction, authentication and record scope, response statuses and headers, and deterministic mock responses that Developer 2 can use before the backing services are complete.

`contract.json` is the complete language-neutral design handoff and `openapi.json` is its standard OpenAPI 3.1 projection for typed-client generation, Ajv-compatible JSON Schema validation, mock servers, Swagger UI and Redoc. `NewContractDocument` is the authoritative Go source for both. Snapshot tests compare both generated artifacts in full, and the OpenAPI tests resolve every schema reference and require examples for every declared success and failure outcome.

Regenerate both artifacts from the server directory with:

```sh
go run ./compose/types/city311/cmd/generate
```

The exact leaf provisions implemented or verified are recorded in `contract.json`; section-level ranges are intentionally not used. The contract covers shared local and federated sessions, structured authorization, bound path parameters, optimistic concurrency, validation, lists, idempotency, asynchronous operations, atomic bulk failure, geocoding and attachment conventions together with the public portal, staff request handling, administration, reporting, mail, calendar and external-integration surfaces. Bulk mutations carry an expected version for every selected request, contextual-help updates require `If-Match`, and reminder paths use a controlled action vocabulary. Localised display strings remain in Developer 2's translation catalogue and are not duplicated here.

The contract records explicit integration decisions where the specification fixes behavior but not internal routes or representation details. Notably, CivicWorks direct completion is normalised atomically through the legal CRM lifecycle, terminal redelivery is acknowledged idempotently, portal attachments use staged uploads while the integration API retains inline base64, anonymous lookup uses a privacy-safe projection, and application roles are kept distinct from identity-provider and audit actor vocabularies. This package defines the contract only; runtime routes and persistence implement it elsewhere.

Public endpoint errors describe only reachable outcomes. Local sign-in does not distinguish an unknown identifier from an incorrect password, registration does not distinguish a new identifier from one already associated with a verified account, and `/healthz` publishes its required `503 TEMPORARILY_UNAVAILABLE` response. The current contract is `2.0.0`, supported major `2`; first publication was `1.0.0`. Semantic versions identify incompatible consumer requirements, not a promise to support older versions. This benchmark reference implementation has no backward-compatibility requirement and provides only the current contract.

Optional-session endpoints discard an absent, expired or invalid cookie and continue anonymously. Their error sets therefore exclude authentication and authorization failures; the browser geocode proxy instead exposes the actionable `ADDRESS_NOT_FOUND`, `MAP_TEMPORARILY_UNAVAILABLE` and `VALIDATION_ERROR` outcomes. Every deterministic mock identifies its endpoint and whether it represents a request or response, and the contract tests verify every response status and error code against that endpoint.

Identity-provider endpoints, client identifiers, role mappings and secrets are supplied by runtime configuration. The identity administration response publishes effective non-secret values, OIDC secret-configuration status and the mapping from asserted `actor_role` values to `application_role`; its update request may enable or disable OIDC and SAML only. Secret values are never returned or accepted by this API.

Developer 1 is the designated maintainer for this package and its generated or shared contract artifacts.

Contract `2.0.0` includes the attachment runtime handoff: optional `attachments`
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
must align its DTOs and fixtures to `2.0.0`, use returned attachment IDs, and replace
`new Blob([response.body])` with base64 decoding into a byte array before creating
the Blob. For example, `hello` is transported as `aGVsbG8=`; the saved file must
contain `hello`, not the encoded text. Reject a missing/unsupported encoding;
do not keep a literal-text fallback. Upload progress belongs in the HTTP transport.
Do not call download with an upload token or treat the mock-only
`removePortalAttachment` method as a published server operation.

For frontend authorization, every session-protected operation declares a `required_capability`; `current_actor.capabilities`, `available_routes`, and `scopes` are bound to the published `capability`, `route`, and `oauth_scope` enums. Record-specific lifecycle actions remain in `available_actions`. OpenAPI operations repeat the required capability as `x-city311-required-capability` and include generated examples plus any linked deterministic mocks.
