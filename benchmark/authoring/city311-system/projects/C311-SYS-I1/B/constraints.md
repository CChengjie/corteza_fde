# Constraints

Preserve P1 and D1 behavior. Business code must not call mapping directly;
credentials are server-only. Missing configuration, upstream timeout, malformed
responses, and upstream authentication failures must use the one safe mapping
unavailable response without leaking fixture details.
