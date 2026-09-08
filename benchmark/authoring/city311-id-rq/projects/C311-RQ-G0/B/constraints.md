# Customer Constraints

`C-G0-01` Use the supplied mapping service for location validation. Do not
replace it with a local geocoder or accept unvalidated coordinates.

`C-G0-02` Persist request data and attachments across application restart.

`C-G0-03` Assign a stable, externally safe request number. Anonymous public
lookup requires the request number and the submitted email, and returns no PII,
internal notes, attachments, or staff-only fields.

`C-G0-04` Staff queue reads and operations enforce department and district
scope. An out-of-scope staff account must receive no request data.

`C-G0-05` Submission is idempotent for the same idempotency key and payload;
the same key with a different payload is rejected without creating a record.

`C-G0-06` Invalid input, unavailable mapping, failed attachment persistence,
and forbidden state transitions leave no partial request or orphaned file.
