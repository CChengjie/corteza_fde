# CivicWorks External Work-Order API Contract

## Purpose

This document defines the binding public CivicWorks integration contract and the deterministic
benchmark-fixture profile: service configuration, authentication, resources, lifecycle, API
operations, event delivery, errors, and evaluator-only controls. Required CRM features and
acceptance outcomes are specified in a separate task document.

You do not need to identify, install, or emulate any real municipal software.

## Contract scope

This contract defines:

- runtime service discovery and authentication;
- the external work-order resource and status lifecycle;
- creation, lookup, and idempotency semantics;
- optional callback delivery and status retrieval;
- event signing, acknowledgement, and retry semantics; and
- deterministic error responses.

This contract does not define CRM pages, user operations, reporting, notification policy, internal
storage, or evaluation criteria.

The evaluator-only fixture controls defined below are not CRM operations. Application and browser
code MUST NOT call them, receive their control token, or expose them through a product API.

## Service configuration

The runtime environment supplies:

```text
CIVICWORKS_BASE_URL=http://civicworks:8080
CIVICWORKS_API_TOKEN=<run-specific token>
CIVICWORKS_WEBHOOK_SECRET=<run-specific secret>
CIVICWORKS_CALLBACK_BASE_URL=http://application:8080
BENCHMARK_RUN_ID=<run identifier>
```

Do not hard-code these values.

The CRM appends `/integrations/civicworks/events` to `CIVICWORKS_CALLBACK_BASE_URL` and sends the
resulting absolute URL as `callback_url`. The base URL must be reachable from the CivicWorks
container and is distinct from any public browser-facing application URL.

Every consumer API request other than `GET /healthz` must include:

```http
Authorization: Bearer <CIVICWORKS_API_TOKEN>
X-Benchmark-Run-Id: <BENCHMARK_RUN_ID>
Content-Type: application/json
```

All timestamps use UTC and RFC 3339 format. All request and response bodies use UTF-8 JSON.

## CivicWorks work-order representation

```json
{
  "work_order_id": "WO-000001",
  "source_case_id": "case-7c58d2",
  "service_request_number": "SR-2026-00041",
  "service_type": "TREE_MAINTENANCE",
  "summary": "Fallen branch obstructing pavement",
  "department_code": "PUBLIC_WORKS",
  "fulfilment_source": "CIVICWORKS",
  "status": "ASSIGNED",
  "location": {
    "address": "100 Example Street",
    "latitude": 42.9001,
    "longitude": -78.8801
  },
  "external_status_url": "http://civicworks:8080/ui/work-orders/WO-000001",
  "version": 1,
  "created_at": "2026-08-20T10:00:00Z",
  "updated_at": "2026-08-20T10:00:00Z"
}
```

`location` is optional unless the originating case contains location information. Do not transmit
sensitive personal information to CivicWorks.

## Status lifecycle

The CivicWorks status vocabulary is:

| Status | Meaning within CivicWorks |
|---|---|
| `ASSIGNED` | The external work order exists and has been assigned. |
| `IN_PROGRESS` | Work is in progress. |
| `PARTIALLY_COMPLETED` | Some work is complete, but the work order remains active. |
| `COMPLETED` | The external work is complete. |

Allowed transitions are:

```text
ASSIGNED -> IN_PROGRESS
ASSIGNED -> COMPLETED
IN_PROGRESS -> PARTIALLY_COMPLETED
IN_PROGRESS -> COMPLETED
PARTIALLY_COMPLETED -> IN_PROGRESS
PARTIALLY_COMPLETED -> COMPLETED
```

`COMPLETED` is terminal. `version` increases whenever CivicWorks changes the work order. A lower
version represents older state.

## Consumer API

### Health check

```http
GET /healthz
```

Successful response:

```json
{
  "status": "ok"
}
```

### Create a work order

```http
POST /api/v1/work-orders
Idempotency-Key: <stable key for this CRM case and integration>
```

Example request:

```json
{
  "source_case_id": "case-7c58d2",
  "service_request_number": "SR-2026-00041",
  "service_type": "TREE_MAINTENANCE",
  "summary": "Fallen branch obstructing pavement",
  "department_code": "PUBLIC_WORKS",
  "location": {
    "address": "100 Example Street",
    "latitude": 42.9001,
    "longitude": -78.8801
  },
  "callback_url": "http://application:8080/integrations/civicworks/events"
}
```

`callback_url` is optional in the external API. When supplied, it must be an absolute HTTP or HTTPS
URL. The City 311 CRM profile always supplies it so normal status changes use signed callbacks;
polling remains available for recovery.

The initial successful response is `201 Created`:

```json
{
  "work_order_id": "WO-000001",
  "source_case_id": "case-7c58d2",
  "service_request_number": "SR-2026-00041",
  "service_type": "TREE_MAINTENANCE",
  "summary": "Fallen branch obstructing pavement",
  "department_code": "PUBLIC_WORKS",
  "fulfilment_source": "CIVICWORKS",
  "status": "ASSIGNED",
  "location": {
    "address": "100 Example Street",
    "latitude": 42.9001,
    "longitude": -78.8801
  },
  "external_status_url": "http://civicworks:8080/ui/work-orders/WO-000001",
  "version": 1,
  "created_at": "2026-08-20T10:00:00Z",
  "updated_at": "2026-08-20T10:00:00Z"
}
```

### Idempotent creation

Repeating an equivalent request with the same `Idempotency-Key` returns `200 OK` and the original
work order. It does not create a second work order.

Reusing the key with materially different content returns:

```http
HTTP/1.1 409 Conflict
```

```json
{
  "error": "IDEMPOTENCY_CONFLICT",
  "message": "The idempotency key has already been used with different content.",
  "retryable": false
}
```

Creation is also unique by `source_case_id`. An equivalent create request for an existing source
case returns `200 OK` and the original work order even when it supplies a new idempotency key. A
materially different create request for an existing source case returns `409 SOURCE_CASE_CONFLICT`.

### Read a work order

```http
GET /api/v1/work-orders/{work_order_id}
```

This returns the current work-order representation.

### Find the work order for a CRM case

```http
GET /api/v1/work-orders?source_case_id={source_case_id}
```

The response contains zero or one item:

```json
{
  "items": [
    {
      "work_order_id": "WO-000001",
      "source_case_id": "case-7c58d2",
      "status": "IN_PROGRESS",
      "version": 2,
      "updated_at": "2026-08-20T10:05:00Z"
    }
  ]
}
```

This endpoint may be used for polling or recovery after missed callback delivery.

## Status-event delivery

If a `callback_url` was supplied, CivicWorks sends status changes to that URL:

```http
POST /integrations/civicworks/events
X-CivicWorks-Event-Id: EVT-000031
X-CivicWorks-Signature: sha256=<signature>
Content-Type: application/json
```

```json
{
  "event_id": "EVT-000031",
  "event_type": "work_order.status_changed",
  "work_order_id": "WO-000001",
  "source_case_id": "case-7c58d2",
  "previous_status": "IN_PROGRESS",
  "status": "COMPLETED",
  "version": 3,
  "occurred_at": "2026-08-20T10:30:00Z"
}
```

The signature is calculated as:

```text
HMAC-SHA256(CIVICWORKS_WEBHOOK_SECRET, exact_request_body)
```

CivicWorks treats any `2xx` response as acknowledgement of the event. A non-`2xx` response or
connection failure is treated as an unsuccessful delivery attempt.

CivicWorks uses at-least-once event delivery. It may repeat an event using the same `event_id` and
body. Consumers must therefore process events idempotently.

### Delivery schedule

Callback delivery uses the following schedule:

- first attempt: immediately;
- second attempt: one second after the first failure; and
- third and final attempt: five seconds after the second failure.

The same `event_id`, signature, and body are used for each retry.

## Error contract

Errors use this form:

```json
{
  "error": "VALIDATION_ERROR",
  "message": "department_code is required",
  "retryable": false
}
```

| Condition | HTTP status and error code |
|---|---|
| Missing or invalid token | `401 UNAUTHORIZED` |
| Incorrect benchmark run | `403 RUN_SCOPE_MISMATCH` |
| Unknown work order | `404 WORK_ORDER_NOT_FOUND` |
| Unknown fixture event | `404 EVENT_NOT_FOUND` |
| Reused idempotency key with different content | `409 IDEMPOTENCY_CONFLICT` |
| Existing source case with materially different content | `409 SOURCE_CASE_CONFLICT` |
| Invalid status transition | `409 INVALID_STATUS_TRANSITION` |
| Missing or malformed required field | `422 VALIDATION_ERROR` |
| Temporary service outage | `503 TEMPORARILY_UNAVAILABLE` with `retryable: true` |

Clients may safely retry creation after an uncertain response by reusing the same
`Idempotency-Key`.

## Deterministic benchmark-fixture profile

The supplied benchmark fixture implements the consumer API above and the control API in this
section. This section removes fixture-state ambiguity; it does not add product behavior.

### Deterministic allocation and time

After reset, the first newly created work order is `WO-000001` and subsequent work orders use the
next six-digit decimal value. The first generated event is `EVT-000031` and subsequent events use
the next six-digit decimal value. Allocation is serialized, so simultaneous creates are ordered by
the fixture's acquisition of its state lock.

The first work order is created at `2026-08-20T10:00:00Z`; each subsequent work order is created one
second later. Each successful status transition occurs five minutes after that work order's current
`updated_at`. These timestamps are logical fixture time and do not depend on wall-clock time.

The request fingerprint used for idempotency includes every create-request field after JSON decode
and canonical re-encoding, including `location` and `callback_url`. Object member order and JSON
whitespace are therefore immaterial.

### Control authentication

The benchmark harness receives a separate run-specific `CIVICWORKS_CONTROL_TOKEN`. Every control
request includes:

```http
Authorization: Bearer <CIVICWORKS_CONTROL_TOKEN>
X-Benchmark-Run-Id: <BENCHMARK_RUN_ID>
Content-Type: application/json
```

The CRM receives only `CIVICWORKS_API_TOKEN`; the two tokens MUST differ. Missing or invalid control
credentials return the same `401 UNAUTHORIZED` or `403 RUN_SCOPE_MISMATCH` form as the consumer API.

### Reset

```http
POST /__fixture/v1/reset
```

Reset atomically removes all work orders, idempotency keys, generated events, delivery attempts and
fault state for the configured benchmark run, and resets the deterministic sequences and logical
clock. It returns:

```json
{
  "status": "reset",
  "next_work_order_id": "WO-000001",
  "next_event_id": "EVT-000031"
}
```

### Advance a work order

```http
POST /__fixture/v1/work-orders/{work_order_id}/status
```

```json
{
  "status": "IN_PROGRESS"
}
```

The target status must follow the public lifecycle. The fixture updates the work order, increments
its version, creates one signed event, and synchronously runs the public callback retry schedule.
The response is `200 OK` and contains the updated `work_order`, generated `event`, and a `delivery`
summary with `acknowledged`, `attempts`, and the last HTTP `status` when a response was received.
No callback is attempted when the work order has no callback URL; in that case `attempts` is zero
and `acknowledged` is false. Invalid transitions return `409 INVALID_STATUS_TRANSITION`; unknown work
orders return `404 WORK_ORDER_NOT_FOUND`.

Example response after advancing the first work order from `ASSIGNED` to `IN_PROGRESS` and receiving
`204 No Content` from its callback:

```json
{
  "work_order": {
    "work_order_id": "WO-000001",
    "source_case_id": "case-7c58d2",
    "service_request_number": "SR-2026-00041",
    "service_type": "TREE_MAINTENANCE",
    "summary": "Fallen branch obstructing pavement",
    "department_code": "PUBLIC_WORKS",
    "fulfilment_source": "CIVICWORKS",
    "status": "IN_PROGRESS",
    "location": {
      "address": "100 Example Street",
      "latitude": 42.9001,
      "longitude": -78.8801
    },
    "external_status_url": "http://civicworks:8080/ui/work-orders/WO-000001",
    "version": 2,
    "created_at": "2026-08-20T10:00:00Z",
    "updated_at": "2026-08-20T10:05:00Z"
  },
  "event": {
    "event_id": "EVT-000031",
    "event_type": "work_order.status_changed",
    "work_order_id": "WO-000001",
    "source_case_id": "case-7c58d2",
    "previous_status": "ASSIGNED",
    "status": "IN_PROGRESS",
    "version": 2,
    "occurred_at": "2026-08-20T10:05:00Z"
  },
  "delivery": {
    "acknowledged": true,
    "attempts": 1,
    "status": 204
  }
}
```

### Redeliver an event

```http
POST /__fixture/v1/events/{event_id}/redeliver
```

Redelivery reuses the event's exact UTF-8 body, `event_id`, and HMAC signature and runs the same
immediate/one-second/five-second attempt schedule. It returns the resulting delivery summary. This
operation exists so acceptance tests can verify idempotent callback handling. An unknown event
returns `404 EVENT_NOT_FOUND`.

Example successful response:

```json
{
  "acknowledged": true,
  "attempts": 1,
  "status": 204
}
```

### Inspect deliveries

```http
GET /__fixture/v1/deliveries
```

The response is `{"items": [...]}` in attempt order. Each item contains `event_id`, `attempt`,
`acknowledged`, optional HTTP `status`, and `attempted_at`. It never returns either token or the
webhook secret.

```json
{
  "items": [
    {
      "event_id": "EVT-000031",
      "attempt": 1,
      "acknowledged": true,
      "status": 204,
      "attempted_at": "2026-08-20T10:05:00Z"
    }
  ]
}
```

### Deterministic outage

```http
PUT /__fixture/v1/outage
```

```json
{
  "enabled": true
}
```

While enabled, consumer create and lookup operations return `503 TEMPORARILY_UNAVAILABLE`; health
and control operations remain available. Setting `enabled` to false restores normal operation.
The response echoes `{"enabled": true}` or `{"enabled": false}`. Reset disables the outage.
