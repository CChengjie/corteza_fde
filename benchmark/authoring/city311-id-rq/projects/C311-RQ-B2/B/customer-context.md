# Customer System B: City 311

City 311 receives resident service requests. Residents can submit a request,
look up its public status with the request number and submitted email, and see
only the permitted public history. Staff can operate requests only within their
department and district authority.

The supplied customer environment contains historical service requests,
attachments, request numbers, audit records, department-scoped staff accounts,
and a City-owned CivicWorks work-order system. CivicWorks is outside the agent
workspace. The task runtime supplies it through a deterministic fixture, not a
local replacement table inside City 311.

The B seed must support submission, triage, staff detail, public status lookup,
and the CivicWorks integration boundary. It must not create a work order or
persist an external work-order mapping. That missing capability is R.
