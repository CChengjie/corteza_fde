# Anonymous public status runtime

This self-contained unit implements **7.9.3(b), 7.9.4 and 12.1.3(a)–(b)** of
`CRM-311-implementer-specification.2dev-fe-be.en-2026-08-24.docx`, within the
original ownership-index wording **“Submission, lookup, drafts, attachments,
location”** (the lookup backend only). Developer 1's original role is **“Backend,
integrations, and runtime”** and includes **“Role and record-scope enforcement,
REST APIs, OAuth scopes, and error responses.”** There is no numbered original
stage assigned to this feature.

`POST /api/v1/public/service-request-status` uses the already-published
`anonymous_status_lookup_request` and `anonymous_status_lookup_response`:

```json
{"request_number":"SR-2026-00041","email":"alex@example.invalid"}
```

An exact request number and matching submitted email return 200 with the public
request summary/status/department/timestamp and complete chronological public
history. Email comparison uses the same trim/case normalisation as submission;
request numbers are exact. The lookup credential is the request's submitted
snapshot, not an editable current constituent profile.

Every mismatch, malformed credential body, unknown field or body over 4 KiB
returns the identical 404 body `{"request_detail":null}`. Drafts are never exposed,
even if accidentally assigned a number. Responses use `Cache-Control: no-store`.
Only the dedicated public-history records are returned; private contact details,
description, location, attachments, audit, staff assignments and internal notes
are not part of this projection. All history pages are read and ordered by event
time, with ID as a stable tie-breaker. No mutation or audit event is created by a
lookup, and no pagination or idempotency header is needed for this operation.

No schema, contract, seed, generated store, identity, attachment or frontend file
is changed. The branch builds independently from merged mainline, alongside
attachment #13, profile #15 and the mock-only FE-05 #14. Developer 1 owns the
single route registration; Developer 2 owns the status form and visible states.

The original merge rule remains **“Merge the backend data, permissions, and APIs
before the corresponding frontend flows.”** Tests cover matched/mismatched and
malformed input, privacy, immutable submitted email despite profile edits,
non-public drafts, reconstruction and 252 history records spanning pages.

Run from `server`:

```sh
go test ./compose/... ./app/... ./store/tests/...
go test -race ./compose/service/city311 ./compose/rest/city311 -run TestPublicStatus
go vet ./compose/service/city311 ./compose/rest/city311
```

Under the original acceptance matrix, **“Check status”** requires matching and
mismatching browser tests and a uniform not-found result. These API tests provide
the backend half; both developers still owe joint real-backend browser acceptance.
Developer 1 retains final runtime/database acceptance and Developer 2 final
browser-experience acceptance. This does not implement authenticated My requests,
linking, notes, reopen, drafts, or claim Docker/PostgreSQL restart acceptance.
