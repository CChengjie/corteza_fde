# Inherited Customer Constraints

`C-RQ-B2-01` Use: assigning an eligible triaged request must call the supplied
CivicWorks fixture. A local work-order substitute does not satisfy the task.

`C-RQ-B2-02` Synchronize: the local request and external work order must share
the request number, source case ID, department, status, and stable external ID.

`C-RQ-B2-03` Preserve: historical request IDs, request numbers, attachments,
public status lookup, audit records, and unchanged API behaviour remain usable.

`C-RQ-B2-04` Constrain: a staff member outside the owning department or district
must not create, read, or infer the external work order.

`C-RQ-B2-05` Reliability: a refused or unavailable CivicWorks call must leave
the request unassigned and must not create a local fake external mapping.
