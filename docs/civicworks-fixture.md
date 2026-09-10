# Deterministic CivicWorks fixture

The root Compose project supplies a fixed-output CivicWorks service at
`http://civicworks:8080` for the City 311 backend. Host port `8081` is available
for acceptance tooling. The authoritative protocol is the checked-in benchmark
complement
[`civicworks-consumer-system.implementer-input.md`](civicworks-consumer-system.implementer-input.md).

The consumer API supports health, idempotent work-order creation, lookup by
work-order ID, and lookup by CRM source-case ID. It requires the configured API
token and benchmark run ID. Work-order IDs begin at `WO-000001`, event IDs begin
at `EVT-000031`, and timestamps advance from the fixed
`2026-08-20T10:00:00Z` logical clock.

The fixture has a separate evaluator-only API:

- `POST /__fixture/v1/reset`
- `POST /__fixture/v1/work-orders/{work_order_id}/status`
- `POST /__fixture/v1/events/{event_id}/redeliver`
- `GET /__fixture/v1/deliveries`
- `PUT /__fixture/v1/outage`

These routes require `CIVICWORKS_CONTROL_TOKEN`, which must differ from
`CIVICWORKS_API_TOKEN`. Compose never passes the control token to the app.
Product and browser code must not call or proxy the control API.

Status advancement applies the declared CivicWorks lifecycle, increments the
work-order version, signs the exact event body with HMAC-SHA256, and runs the
immediate/one-second/five-second callback retry schedule. Redelivery reuses the
same event ID, bytes, and signature. Delivery inspection never returns tokens or
the webhook secret.

The CRM constructs its callback from the server-only
`CIVICWORKS_CALLBACK_BASE_URL`. In Compose this is `http://app:80`, yielding
`http://app:80/integrations/civicworks/events`; the browser-facing
`APP_BASE_URL` is deliberately not used for container-to-container callbacks.
