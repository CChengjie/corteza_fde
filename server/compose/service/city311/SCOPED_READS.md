# City 311 scoped constituent and audit reads

## Original specification correspondence

This is a self-contained Developer 1 backend unit, not a newly numbered stage.
The original two-developer specification names the role **“Developer 1 |
Backend, integrations, and runtime”**, with ownership of **“Roles, permissions,
and department and district boundaries: Chapter 4”** and **“Constituents, service
requests, relationships, notes, reminders, attachments, geolocation, audit, and
custom fields: Chapter 9.”** Its required deliverable is **“Role and record-scope
enforcement, REST APIs, OAuth scopes, and error responses.”**

This unit implements the constituent search/detail and audit-browsing API parts
of 4.1.1, 4.2, 9.7.3 and 12.2.1, supporting the baseline find/open behavior in
3.3.3. It does not claim the export portion of 9.7.3, report creation, constituent
editing, or completion of Chapter 9. No new domain fields, enums, migrations or
seed values are introduced. The existing contract's constituent-read permission
is narrowed to the four CRM staff roles; workflow-designer-only access does not
grant CRM record access. The existing fixtures already include these roles.

## Implemented routes

All routes use the existing staff authentication middleware and resolve the
current persisted role profile on every call. Responses have `Cache-Control:
no-store`. Missing authentication returns 401; disallowed roles return 403.

| Route | Authorized readers | Response |
| --- | --- | --- |
| `GET /api/v1/staff/constituents` | Agent/supervisor in department and district; manager in department; platform administrator globally | Published list envelope with constituent DTOs |
| `GET /api/v1/staff/constituents/{constituent_id}` | Same scope | Constituent DTO, 403 outside scope, 404 if absent |
| `GET /api/v1/staff/audit-events` | Department manager or platform administrator | Published list envelope with audit-event DTOs |

Non-administrators require an assigned department. Constituents without a
district follow the existing department-wide visibility rule. Constituent IDs
come from the indexed persisted column, not the profile JSON. Projections omit
private profile keys, preserve custom fields, and return empty arrays rather
than null contact collections.

For manager audit browsing, ownership is resolved from the **current persisted
request**, or the current constituent for a constituent event without a request.
The manager's district does not limit their department. Identity/configuration
events without a record owner, missing target records, and unowned constituents
are administrator-only. Payload text and the event actor never grant scope.
This is current-record authorization, not historical-department authorization.
Reads do not change records or append audit events.

## Query contract

The authoritative contract assembly and generated OpenAPI describe filter
properties, sort fields and role requirements. Both JSON `filters` and exploded
filter parameters are accepted; do not mix them. For example:

```text
GET /api/v1/staff/constituents?page_size=25&department=STREETS&q=resident&sort=display_name
GET /api/v1/staff/constituents?filters=%7B%22department%22%3A%22STREETS%22%7D
GET /api/v1/staff/audit-events?entity_type=service_request&request_id=42&sort=-occurred_at
```

- Constituent filters: `q` (case-insensitive name/email/phone substring), `email`
  (case-insensitive exact match), `department`, `district`, `category`.
- Audit filters: `entity_type`, `entity_id`, `event_type`, `actor_id`, `request_id`,
  `source_channel`, `from`, `to`. Dates are RFC 3339, with inclusive boundaries.
- `page_size`: default 50, range 1–100. `page_token`: opaque continuation token.
- `sort`: up to three comma-separated fields, optional `-` for descending.
  Constituent fields: `constituent_id` (default), `display_name`,
  `primary_category`, `updated_at`. Audit fields: `occurred_at` (default),
  `entity_type`, `entity_id`, `event_type`, `actor_id`. String fields use exact
  lexical ordering; time fields use chronological ordering; persisted row ID
  breaks ties deterministically.
- Unknown/duplicate query parameters, invalid filters, repeated sort fields,
  oversized page sizes and invalid continuation tokens return 422
  `VALIDATION_ERROR` with RFC 6901 field pointers.

The response contains `items`, `next_page_token`, `total_count`, `applied_filters`
and `sort`. All storage pages are considered before permission-filtered counting
and pagination. Tokens bind the operation, current actor/role/scope, normalized
filters and sort. Each page rechecks authorization. A token is not a grant and
is not a frozen database snapshot: concurrent authorized record edits may change
the result ordering. This bounded benchmark implementation scans stored rows;
large-scale indexed query optimization is not an acceptance claim.

## Verification and handoff

Service and HTTP tests cover role denials, department/district boundaries,
251 inaccessible records preceding visible records, filtered counts and
continuations, service reconstruction, query/scope-bound tokens, deterministic
sorting, invalid inputs, private-key exclusion, null collection normalization,
corrupt projection failures, audit ownership and side-effect-free reads.

Developer 1 maintains REST registration and contract generation. Developer 2
retains constituent/audit browser surfaces and interaction acceptance. The
original merge rule remains **“Merge the backend data, permissions, and APIs
before the corresponding frontend flows.”** Runtime/database acceptance belongs
to Developer 1; browser acceptance to Developer 2; end-to-end, responsive,
accessibility and baseline-regression checks remain joint. Local SQLite tests
are not PostgreSQL, container-restart or browser acceptance.

## Parallel boundary assessment

The profile, public-status and scoped-read units consume merged identity and
record models, and can be reviewed independently of attachment PR #13 and
mock-only frontend FE-05 (#14). The remaining families need coherent model or
integration units rather than additional read-only endpoint shells:

| Family | Required foundation before an independent consumer unit |
| --- | --- |
| Drafts, linking, notes, assignment and bulk operations | Shared request/relationship model and transactional attachment ownership; coordinate with #13 before changing those write paths |
| CivicWorks | Durable work-order delivery/recovery and callback state, idempotent transitions, signature/version handling and fixture acceptance |
| Reminders and ICS | Reminder/recipient lifecycle model, audit and scheduling behavior |
| Broader mail and workflow actions | Durable delivery/operation model, template and attachment rules, retry/terminal-failure handling; identity notices alone are not the mail feature |
| Federation | Account linking/JIT policy, external identity fixture validation and shared session/account projections; coordinate with #15 |
| Reports and exports | Complete requested entities (including follow-up actions), saved report/operation model, export audit, OAuth scope and rate-limit enforcement |
| Branding, public content and help | Baseline content/page integration plus shared versioned draft/publication/history/rollback model and localized catalogue ownership; a settings-only or static public getter would not implement 7.1/7.13 |

These are still Developer 1 responsibilities, not waived requirements or a
claim that frontend implementation must finish first. They are the next coherent
model/integration work, to sequence against the existing shared-maintainer and
merge boundaries instead of presenting partial consumers as finished features.
