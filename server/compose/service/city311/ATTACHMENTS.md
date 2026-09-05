# City 311 attachment runtime

## Original specification correspondence

This is the backend attachment portion of the original Option A provision
ownership entry **“Submission, lookup, drafts, attachments, location”**
(chapters **7.6–7.9, 9.1–9.5, 10.1**). The original document does not assign a
numbered implementation stage to this unit. Its Developer 1 wording is:

> Owns all server-side, persistence, security, external-integration, and runtime capabilities.

The applicable scope includes **“Constituents, service requests, relationships,
notes, reminders, attachments, geolocation, audit, and custom fields: Chapter 9.”**
The merge rule remains **“Merge the backend data, permissions, and APIs before
the corresponding frontend flows.”** FE-03/FE-04 were merged as mock-only work;
this backend does not certify their real browser integration.

Implemented/verified provisions: 3.2.2, 3.2.3, 3.3.4, 4.2.2, 4.2.3, 4.3.1,
4.3.2, 6.5.1, 7.6.1, 7.6.4, 7.7.1, 9.1.3, 9.4.1, 9.4.2, 9.4.3, 9.4.4(a),
9.7.1, 9.7.2, 11.6.5(c), 14.1.1 (Upload attachments), 14.3.2.

## Public operations

- `POST /api/v1/portal/attachments`: optional session, multipart fields `file`,
  `filename`, `media_type`, returning `201` and the existing upload receipt.
  Exactly one file, one byte through 10 MiB inclusive, an allowed media type,
  and a 1–120 Unicode-character basename are
  required. The entire body is bounded; multipart temporary files are removed.
- Portal and staff request submission accept up to five receipt tokens. Every
  token is checked before request creation; its final file ID is claimed through
  a database primary-key constraint. Request, attachment, audit, stage consumption,
  sequence, and idempotency writes share one transaction. Integration submissions
  retain inline base64 and also expose the resulting metadata.
- Successful submission responses expose `attachments[].attachment_id`;
  staff details expose the same metadata at `request.attachments`.
- `GET /api/v1/attachments/{attachment_id}` requires authentication and current
  request-view permission. Current primary-requester ownership and staff
  department/district scope are enforced. Workflow-design permission alone is
  not a CRM record grant; a cumulative CRM record role is required for staff access.
  Unauthenticated, forbidden, and missing
  results are `401`, `403`, and `404`. Bytes are base64 in the existing JSON
  envelope with required `body_encoding=base64`, `Cache-Control: no-store`, and
  `X-Content-Type-Options: nosniff`. The disposition is safely encoded as attachment.

Staging lasts 3,600 seconds according to the service clock (`BENCHMARK_NOW` when
supplied). Only token hashes are stored. Anonymous receipts can be consumed by
their bearer; authenticated receipts require the uploader's account. All invalid
receipt cases use `422 VALIDATION_ERROR`, indexed at `/attachment_tokens/0` or
`/request/attachment_tokens/0`. Equivalent portal replay keeps the original `201`
response even after receipt consumption/expiry; a changed body is `409
IDEMPOTENCY_CONFLICT`. Replay is actor-bound, with or without attachments;
another actor using the same key receives that conflict. Failed validation never
consumes otherwise-valid tokens.

No staging delete API is introduced. Omitting a token abandons it; startup and
minute-interval cleanup traverse every raw cursor page in bounded batches, deleting
stages whose expiry is at or before the effective clock, including stages behind
live-only pages. Submitted files
are never part of that cleanup. Upload progress belongs to the browser transport.
Pre-submission preview must use the locally selected file, not a download call
with an upload token.

The sole supported API contract is `2.1.0`. The benchmark reference implementation
does not provide backward-compatible download modes. Developer 2 must update
literal-body DTOs, mocks, and Blob construction to require and decode RFC 4648
base64; absence of `body_encoding` is invalid, not a legacy-text signal. Durable
attachment IDs and the decoded bytes must be used during real browser integration.

## Persistence and boundaries

The additive `compose_city311_staged_attachment` DAL table and generated store
operations use the existing startup schema upgrade. Repeated upgrade preserves
stages and submitted files. Committed bytes remain in the existing request
attachment table, backed by the deployment's PostgreSQL volume. No baseline
attachment storage, required configuration, or health checks are replaced.

Authenticated portal requests bind primary-requester access to the current
persisted constituent ID while retaining submitted contact snapshots. Entering
another account's email does not grant that account access. Additional request
relationships, drafts, and their public detail APIs are separate backend units;
this delivery does not claim those services or full frontend acceptance.

Regression tests cover bounded validation, five-file cardinality, receipt scope,
atomic rollback, two service instances, replay, immutable attachment audit,
repeat upgrade, service reconstruction, expiry batches, and authorized binary
download. SQLite attachment service tests explicitly enable SQL transactions;
the baseline SQLite driver otherwise disables them. The two-service test shares
a one-connection SQLite pool and does not prove concurrent PostgreSQL transaction
behavior. Full PostgreSQL concurrency/container restart acceptance and the real
backend/browser path remain distinct runtime gates.
