# Delivery Request

Deliver the first City 311 service-request workflow on the supplied customer
system.

Residents must be able to submit a pothole service request with requester
details, address/coordinates, and attachments. The system must validate the
location through the supplied mapping boundary, issue a stable public request
number, and persist the request and files.

Staff must be able to list and inspect requests only within their authorised
department and district scope. A resident must be able to retrieve a minimal
public status using the request number and submitted email.

Implement the requirement without violating the customer constraints in
`../B/constraints.md`. Keep implementation choices open; the supplied contract
defines interfaces and observable behaviour, not a required architecture.
