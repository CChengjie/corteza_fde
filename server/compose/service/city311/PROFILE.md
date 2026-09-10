# Constituent profile and language runtime

## Original specification scope

This is the account/profile backend portion of **Developer 1 | Backend,
integrations, and runtime** in
`CRM-311-implementer-specification.2dev-fe-be.en-2026-08-24.docx`. The original
wording is “Owns all server-side, persistence, security, external-integration,
and runtime capabilities” and “Constituents, service requests, relationships,
notes, reminders, attachments, geolocation, audit, and custom fields: Chapter 9.”
The document does not assign a numbered implementation stage to this feature.

Implemented or directly verified provisions:

- 4.1.1 and 4.3.1–4.3.2: a constituent maintains only their own account;
  unauthenticated callers receive 401 and non-constituent callers receive 403.
- 6.7.2(c): an authenticated constituent's language selection persists to the
  account. The accepted vocabulary is EN, ES, VI (6.7.1).
- 7.11.3, 9.1.1 and 9.1.2(a): maintain the five editable profile fields with
  the published cardinalities and validation. The separate verified email
  replacement flow in 9.1.2(b) is documented in `EMAIL_REPLACEMENT.md`.
- 9.1.3: updates do not rewrite historical request snapshots or prior audit
  values.
- 9.7.1–9.7.2: successful effective changes write an immutable audit event with
  actor, UTC time, source channel, and changed-field before/after values.
- 3.2.3 and 3.4.2: repeat-upgrade and service-reconstruction tests retain saved
  state and identifiers. These tests are not container-restart acceptance.

The existing contract's `profile_get`, `profile_update`, and `language_update`
operations are implemented with their existing If-Match precondition. OpenAPI
and contract metadata publish the missing success ETag, matching session-cookie
name and deterministic profile success/conflict examples. JSON Schema, enums,
seed data, migrations and generated store files are unchanged. Existing constituent JSON, local
account, Corteza user, and audit records are used. There is one current contract,
with no legacy response mode or compatibility-only branch.

## API and frontend handoff

| Operation | Authentication | Success |
| --- | --- | --- |
| `GET /api/v1/account/profile` | City 311 constituent session | 200 `constituent` |
| `PATCH /api/v1/account/profile` | City 311 constituent session | 200 complete saved `constituent` |
| `PATCH /api/v1/preferences/language` | Optional City 311 session | 200 `language_preference` |

Profile identity is derived from the authenticated session, never a submitted
constituent ID. A Corteza identity alone is not a City 311 constituent session.
Responses are marked `Cache-Control: no-store`.

Profile PATCH supports only:

- `display_name`: 1–120 Unicode characters, trimmed on save;
- `phone_numbers`: zero to three objects, MOBILE/HOME/WORK labels and E.164 values;
- `addresses`: zero to five structured addresses, at most one primary;
- `primary_category`: the existing published contact-category vocabulary;
- `preferred_language`: EN, ES or VI.

Omitted fields retain their current values. Empty arrays explicitly clear a
collection. Explicit null, unknown fields, invalid nested fields, and attempts
to set ID, emails, login identifier, email opt-out, or record scope are rejected
without a partial save. `{}` and an equivalent PATCH using the current ETag are
successful no-ops without extra audit entries. Arrays are replaced as a whole,
not merged. GET and successful PATCH return `ETag: "<revision>"`; send that
value as `If-Match` on PATCH. Missing/malformed preconditions return 428
`EXPECTED_VERSION_REQUIRED`; stale versions return 409 `VERSION_CONFLICT`
with `current_version`, without writes or extra audits. Reload after a conflict
before resubmitting. Even an equivalent body with a stale version is rejected.
The existing contract does not require pagination or an idempotency key here.

Example authenticated request:

```http
PATCH /api/v1/account/profile
Content-Type: application/json
Cookie: city311_session=<session token>
If-Match: "1"

{"display_name":"Alex Resident","preferred_language":"ES","phone_numbers":[{"label":"MOBILE","value":"+17165550100"}],"addresses":[]}
```

The 200 body is the full `constituent`, including unchanged `constituent_id`,
`login_identifier`, `emails`, `primary_category`, and `email_opt_out`. A subsequent
`GET /api/v1/session` reflects the saved display name and language without a new
sign-in. Login-identifier and password maintenance remain their distinct,
already implemented re-authenticated operations.

For example, `{"phone_numbers":[{"label":"HOME","value":"123"}]}` returns
422 `VALIDATION_ERROR`, with field `/phone_numbers/0/value` and validation code
`INVALID_FORMAT`. Missing/expired sessions return 401 `UNAUTHENTICATED`;
authenticated staff without the constituent role receive 403 `FORBIDDEN`.
A missing persisted profile returns 404 `NOT_FOUND`. Unavailable identity
configuration returns retryable 503 `TEMPORARILY_UNAVAILABLE`. All use the existing
API error envelope; no error enum is added.

Language selection uses `{"language":"VI"}` and returns that same shape.
For authenticated constituents it uses the same audited transaction as profile
PATCH. For anonymous visitors or staff it acknowledges a browser-session
preference without creating or editing a constituent account. Developer 2 owns
browser-session storage, selector UI, translations, fallback, and post-sign-in
preference application under 6.7.2(a)–(b), 6.7.3 and 8.4.

The optional-session language endpoint has no If-Match requirement. A language
change still increments the profile revision, as does an identifier change;
reload the profile and its ETag before saving an already-open profile form.

## Persistence and concurrency

Profile, Corteza user name/language, local-account language, and audit are saved
in one database transaction. Existing user metadata is retained. A failed audit
insert rolls all projections back. A database account-row write lock is acquired
before reading profile/account projections; identifier maintenance uses the same
lock and fresh database values, including its audit before-value. This is not a
process-local mutex. Future account/profile writers must use the same lock order.
The private revision is stored as a decimal string in constituent JSON, starts
at 1 for an unmodified profile, and is not exposed as an extra constituent field.
It advances in the same transaction as effective edits; rollback preserves it.

The handwritten store extension uses a no-value-change UPDATE, including on
SQLite where SELECT FOR UPDATE is unavailable, and fails closed for adapters
without this operation. No shared generated interface is edited.

## Acceptance and parallel integration

Automated tests cover owned/foreign account isolation, staff/anonymous/expired
session boundaries, cardinalities, Unicode, E.164, null and protected fields,
omitted-field preservation, clearing, equivalent retries, current session
projections, unrelated user metadata, audit rollback, missing records, ETag
preconditions, two-service competing writes, language/identifier invalidation, repeat
upgrade, and reconstruction over retained data. Two independent service instances
with stale resolved-session snapshots verify fresh projection reads. SQLite
rollback tests explicitly enable real SQL transactions; this is not evidence of
concurrent PostgreSQL acceptance.

Run from `server`:

```sh
go test ./compose/... ./app/... ./store/tests/...
go test -race ./compose/service/city311 ./compose/rest/city311 ./compose/types/city311 -run 'TestProfile|Test.*Identifier'
go vet ./compose/service/city311 ./compose/rest/city311 ./compose/types/city311 ./store ./store/adapters/rdbms
make -C pkg/locale src/en
go test ./tests/...
```

This unit is independently based on merged backend identity/persistence. It does
not depend on the pending attachment implementation (#13), edit frontend files,
or implement all of FE-05 (#14). Developer 1 maintains shared REST registration;
the other additions are separate profile/store files. Sonar coverage includes
the handwritten store lock, without exclusions or a weakened quality threshold.

The original merge wording applies: “Merge the backend data, permissions, and
APIs before the corresponding frontend flows.” FE-05 is currently mock-only;
its profile/language views can switch to these APIs after backend approval.
Developer 1 owns final runtime/database acceptance and Developer 2 owns final
browser-experience acceptance; both own joint end-to-end regression. Real
PostgreSQL contention, clean/migrated container startup and retained-volume
restart, browser persistence, responsive and accessibility acceptance remain
required integration work, not claims of this isolated backend delivery.

Remaining work listed for this profile unit includes linked request access,
drafts, constituent notes/reopen, and category administration; verified email
replacement is implemented as the separate flow in `EMAIL_REPLACEMENT.md`.
CivicWorks, broader mail flows, federation, calendar and exports are additional
backend candidates, but need their own transactional models/fixture acceptance
and coordination of shared schema/route ownership before parallel PRs.
