# Project State

`ocpp-hal-go-new` is a PostgreSQL-backed OCPP 1.6 HAL for
`ev-cms-backend-new`. It accepts only enabled durable v1 charger mappings and
only exposes the authenticated v1 service boundary.

## Implemented

- Source now supports strict mapped charger admission for both `/{identity}`
  and `/{identity}/{serial}`. OCPP identity remains canonical; optional serial
  and Boot metadata are stored only as physical evidence. Migrations 012 and
  013 are additive and have not been applied to a database in this slice.
- Every accepted BootNotification schedules a bounded, asynchronous,
  generation-fenced configuration reconciliation. Standard heartbeat/meter
  sampling keys are configured by default; the legacy vendor-only profile is
  explicit and disabled by default. No live charger or deployment was changed.

- Fact delivery leases are now fenced by a durable secure claim token. A
  delayed worker can no longer transition a fact after an expired lease was
  reclaimed; only the current `fact_id` plus token owner may record delivery.
  The source change requires additive migration `010` and has not been applied
  to a database. Delivery remains serial so the 15-second HTTP timeout fits
  the 30-second lease budget.
- Explicit process or local configuration values, including empty values, are
  now validated as supplied values rather than silently replaced by defaults.
  Only process `HAL_ENVIRONMENT` selects whether local `.env` can be read.

- Startup configuration now fails closed: only `development`, `test`, and
  `production` are valid environments; malformed explicit booleans, ports,
  heartbeat values, log levels, URLs, and coupled fact-delivery credentials
  cannot silently fall back. The production process never reads local `.env`.
  Durable HAL-generated UUIDs propagate entropy failure before a write rather
  than fabricating a timestamp/random-string identity.

- v1 OCPP evidence now has deliberate acknowledgement boundaries. Start/stop,
  discrete mapped status, and known-valid correlated meter facts cannot be
  acknowledged after an unpersisted store failure. Unsupported/stale/unknown
  meter input remains a normal confirmation without projection. Heartbeat is
  refreshable and physical connect/disconnect projection retries the same
  observation; restart conservatively resets any unresolved durable runtime to
  `UNKNOWN`. Connector aggregate fault state derives from all connectors.

- Terminal fact delivery is recoverable through authenticated
  `POST /v1/facts/{fact_id}/requeue`: it records a reconciliation audit and
  returns the same fact identity/payload/digest to `PENDING`, never a new fact.
  This source change requires migration `009` and has not been applied.

- HAL v1 now treats charger timestamps as bounded protocol evidence while
  persisting trusted HAL receipt times for start expiry, duration limits, and
  completion ordering. Start requires a positive connector, OCPP transaction
  ID, and nonnegative meter. Completion rejects meter rollback, pre-start or
  implausible timestamps, and conflicting duplicate evidence before a durable
  completed state or fact can exist. Stop workflows in `PERSISTED` state are
  drained at startup and periodically; attempted/ambiguous workflows are never
  replayed. This source change requires migration `008` and has not been
  deployed or applied to a database.

- The v1 HTTP boundary now requires canonical nonzero UUIDs for mutation
  headers and identities, rejects trailing JSON values, validates mappings
  before runtime persistence, and returns an OCPP CALLERROR when a
  StopTransaction cannot be durably persisted. OpenAPI now exposes mapping,
  transaction, and stop-workflow response schemas and state vocabularies.

- HAL v1 HTTP responses now use explicit transport views rather than directly
  serializing store records. Command start, stop, and exact lookup emit the
  canonical snake_case command object with required identity/state/timestamp
  fields; transaction IDs are explicit nullable fields. Mapping, transaction,
  stop-workflow, and runtime views are also explicit, and credential/customer,
  request-digest, correlation, and raw-error persistence fields are not
  exposed. This source change has not been deployed.

- Exact `GET /v1/transactions?cms_start_intent_id={uuid}` responses expose
  stable snake_case v1 JSON fields, allowing CMS to consume the existing
  recovery socket as authoritative transaction truth. HAL fact delivery records
  only bounded receiver status/code diagnostics and never changes OCPP truth
  because CMS returns an error.

- Mapping-based charger admission, process-scoped generation-safe connection
  runtime with a restart-reset durable baseline and monotonic durable sequence,
  Heartbeat-driven durable liveness renewal, connector OCPP status, exact credential authorization, charger-originated
  transaction start/completion, integer-Wh meter progression, energy/time stop
  workflows, recovery queries, and immutable fact delivery.
- `cpconsole` is an OCPP-native interactive virtual charger with Boot-driven
  automatic Heartbeats, an explicit cadence override, one-shot normal local
  startup sessions, and optional periodic metering. One shared rounded integer
  Wh conversion is used for StartTransaction, MeterValues, and StopTransaction.
  It remains a test client, not HAL runtime behavior or a durable source of
  truth.
- V1 completion persists `meter_stop_wh` as its effective/billable cumulative
  meter and may retain raw stop, adjustment, and evidence metadata for a
  temporally eligible one-Wh register discrepancy. Larger rollback evidence
  remains invalid; periodic one-Wh regressions are counted without regressing
  the authoritative meter.
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
