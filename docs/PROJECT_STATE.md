# Project State

`ocpp-hal-go-new` is a PostgreSQL-backed OCPP 1.6 HAL for
`ev-cms-backend-new`. It accepts only enabled durable v1 charger mappings and
only exposes the authenticated v1 service boundary.

## Implemented

- Mapping-based charger admission, process-scoped generation-safe connection
  runtime with a restart-reset durable baseline and monotonic durable sequence,
  Heartbeat-driven durable liveness renewal, connector OCPP status, exact credential authorization, charger-originated
  transaction start/completion, integer-Wh meter progression, energy/time stop
  workflows, recovery queries, and immutable fact delivery.
- `POST /v1/hal-facts` delivery supports stable fact identity/digest,
  idempotent retry, transient/terminal classification, lost acknowledgement,
  expired-lease reclaim, and UTC-microsecond durable envelope timestamps so
  the persisted fact digest remains valid after PostgreSQL reload. API docs
  are served only when `API_DOCS_ENABLED=true`.
- The Go module identity is
  `github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new`.

## Retired Legacy Runtime

No legacy `/api/*` route, callback worker, callback-derived `max_kwh` policy,
external charger directory, frontend WebSocket, single-session path, legacy
smoke binary, or automatic offline-authorisation configuration path is
registered by `ocpphal`.

Historical legacy schema and test-only store code remain unreferenced by the
new runtime; they are not a CMS integration surface.

## Outside This Repository

CMS customer/CPO identity, authorization, tariffs, wallet holds, billing,
settlement, projections, and customer realtime remain CMS-owned work. No CMS
consumer implementation is included here.
