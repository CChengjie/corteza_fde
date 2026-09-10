# Verified email replacement

## Original specification correspondence

This backend delivery implements original provision **9.1.2(b)** of
`CRM-311-implementer-specification.2dev-fe-be.en-2026-08-24.docx`:

> verified email replacement MUST require verification of the new address.

It belongs to **Developer 1 | Backend, integrations, and runtime**, whose
original responsibility is “Owns all server-side, persistence, security,
external-integration, and runtime capabilities.” The relevant scope wording is
“Constituents, service requests, relationships, notes, reminders, attachments,
geolocation, audit, and custom fields: Chapter 9.” The document assigns no
numbered implementation stage to this provision.

The merge rule remains “Merge the backend data, permissions, and APIs before
the corresponding frontend flows.” Developer 2 owns the browser form,
verification landing page, loading/error/success states, accessibility, and
responsive acceptance. Developer 1 owns runtime and database acceptance; both
developers own joint end-to-end regression.

## API and browser handoff

| Operation | Authentication | Success |
| --- | --- | --- |
| `POST /api/v1/account/email-replacement` | City 311 constituent session | 202 `{"accepted":true}` |
| `POST /api/v1/auth/email-replacement/confirm` | Public; possession of the token is proof of control | 200 `{"verified_email":"new@example.test"}` |

The request body is `{"email":"new@example.test"}`. A syntactically valid
address always receives the same 202 acknowledgement. If the address is the
current verified email or is already claimed by another verified account, the
server creates no verification token and does not disclose that fact. Invalid
email syntax returns 422 `VALIDATION_ERROR`. Missing sessions return 401
`UNAUTHENTICATED`; a non-constituent session returns 403 `FORBIDDEN`.

The confirmation body is `{"token":"<one-time token>"}`. Tokens last 30
minutes and are single use. Missing or malformed input returns 422
`VALIDATION_ERROR`; unknown, used, superseded, deleted-account, or newly
unavailable tokens return 422 `INVALID_EMAIL_VERIFICATION_TOKEN`; expired
tokens return 422 `EXPIRED_EMAIL_VERIFICATION_TOKEN`. Both endpoints and their
errors use `Cache-Control: no-store`. The published contract does not require
`Idempotency-Key` or `If-Match` for this two-step token command.

The old email remains the account's verified email, Corteza user email, profile
email, and valid login identifier until confirmation succeeds. A newer valid
request invalidates every older pending token and cancels any corresponding
delivery that has not yet been sent. Confirmation atomically:

- changes the local account and Corteza user to the normalized new email;
- keeps the Corteza `EmailConfirmed` projection true;
- replaces only the old verified address in the current constituent profile and
  increments its revision;
- consumes all outstanding replacement tokens;
- writes `VERIFIED_EMAIL_CHANGED` audit evidence; and
- queues a security notice to the old address.

Existing sessions remain valid and resolve the new projections on the next
request. Sign-in by the old email stops working after confirmation; sign-in by
the new email works with the unchanged password. Historical request contact
snapshots and prior audit values are not rewritten.

## Persistence, security, and recovery

`compose_city311_email_replacement_token` stores only an HMAC token digest, the
account ID, normalized pending email, creation/expiry timestamps, and use time.
The raw token is never stored. The transactional identity-notification outbox
contains an authenticated-encryption ciphertext so delivery survives process
restart without making the bearer token readable at rest. Delivery uses the
existing bounded retry worker and a stable delivery key.

Request and confirmation acquire the database account-row lock used by the
other identity/profile writers. Token issuance, supersession, audit, outbox,
account, user, and profile writes use database transactions. A unique verified
email constraint remains the final collision guard; a race that makes the
address unavailable before confirmation fails closed as an invalid token with
no partial update. Account deletion consumes pending replacement tokens and
scrubs their queued notifications.

The sole supported contract is version `2.1.0`; there is no legacy replacement
mode or backwards-compatibility branch. `contract.json` and `openapi.json`
publish the DTOs, capability, privacy rule, errors, token protocol, and
deterministic success/failure examples.

Automated tests cover old-address retention, normalization, all identity/profile
projections, sign-in behavior, audit and security notice, one-time use, expiry,
newest-request wins, claimed-address privacy, encrypted restart recovery,
account-deletion invalidation, REST authentication/validation/cache behavior,
contract/OpenAPI completeness, and generated-store CRUD. Clean/migrated
PostgreSQL container startup, retained-volume restart, real mail delivery, and
the browser flow remain final integration acceptance rather than claims of the
isolated unit suite.
