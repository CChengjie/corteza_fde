# Delivery Request

When an authorised staff member assigns an eligible triaged City 311 service
request, create or associate a work order in the supplied CivicWorks system.
Persist the returned external work-order ID and expose the synchronization state
in the staff request detail. CivicWorks status callbacks must update the request
through the existing legal City 311 lifecycle.

Use the supplied CivicWorks base URL, credential, callback secret, and fixture
control API. Do not replace CivicWorks with local data. Preserve all inherited
customer constraints in `../B/constraints.md`.
