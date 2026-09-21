# Project State

## 2026-09-21 - Charger-operation restart recovery source hardening

- HAL startup converts `DELIVERY_ATTEMPTED` operation rows to
  `RECONCILIATION_REQUIRED`, preserving the fact that their OCPP call may have
  crossed the network boundary. It never replays them.
- A mapped charger connection triggers a bounded scan of only durable
  `PERSISTED` rows. Each candidate uses the same atomic claim and typed OCPP
  dispatcher as HTTP acceptance. Offline rows remain `PERSISTED`; an operation
  that cannot be safely classified as pre-delivery is never sent by recovery.
- Migration `023_add_v1_charger_operation_recovery_index` adds the partial
  dispatchable-row index. It is source-only and unapplied.

Focused store and HAL recovery tests pass. PostgreSQL lifecycle, paired CMS,
and physical charger checks remain unrun without `TEST_DATABASE_URL` and a
mapped test charger. No database mutation, deployment, restart, commit, or
push occurred.

## 2026-09-10 - TriggerMessage Accepted atomicity and final-closure source correction

- HAL commits an allowlisted TriggerMessage `CALLRESULT` `Accepted` trace,
  its trace-delivery outbox record, and migration `022`'s indexed durable
  60-second window in one transaction before later operation bookkeeping. It
  searches that window for
  matching charger-originated BootNotification, DiagnosticsStatusNotification,
  FirmwareStatusNotification, Heartbeat, MeterValues, or StatusNotification
  traffic. MeterValues and StatusNotification require the matching connector.
- Each match produces only a separately sanitized
  `CHARGER_OPERATION_FOLLOW_ON` trace event. The existing trace worker writes
  `CHARGER_OPERATION_FOLLOW_ON_CLOSED` for an expired unmatched window. Positive
  matching and closure serialize on an `OPEN` row, producing exactly one final
  transition: `OBSERVED` or `CLOSED`. This is temporal rather than causal:
  overlapping acceptance windows may record the same incoming frame. Trace
  append/delivery failure is logged and never changes the OCPP acknowledgement,
  durable operation result, transaction, connector, worker, or CMS fact path.

Focused store/OCPP/worker/sanitizer tests and full Go test, vet, build, and
diff checks pass. Migration `022` is source-only and unapplied; no database
mutation, deployment, restart, commit, or push occurred. PostgreSQL, paired
CMS, and physical charger validation remain unrun without `TEST_DATABASE_URL`
and a mapped test charger.

## 2026-09-08 - PostgreSQL charger-operation array readback source fix

- `GetV1ChargerOperation` now scans the durable `configuration_keys text[]`
  column through pgx v5's database/sql-compatible `pgtype` scanner rather
  than directly into a Go slice. A successful empty array remains a usable
  empty Go slice; selected keys retain their exact ordering and values.
- The change corrects immediate POST readback, duplicate-request lookup, and
  exact GET lookup without changing migration `021`, the request digest,
  mapping validation, OCPP dispatch, or operation-state meanings.

The DB-free pgx scanner regression passes. The create/read PostgreSQL
regression remains gated by `TEST_DATABASE_URL` and was skipped because no
disposable database was selected. No database mutation, migration, deployment,
restart, commit, or push occurred.

## 2026-09-08 - PostgreSQL charger-operation configuration-key regression source fix

- The v1 PostgreSQL operation insert now normalizes an omitted
  `configuration_keys` Go slice to an explicit empty array before binding it.
  The database default remains unchanged: PostgreSQL does not apply that
  default to an explicit SQL `NULL`. Empty continues to mean all keys, while
  requested keys are retained exactly.
- The correction is limited to the PostgreSQL persistence boundary. It does
  not change the CMS contract, OCPP dispatch, operation state machine, trace
  evidence, migration `021`, or ambiguous-delivery recovery policy.

The DB-free regression test passes. A real PostgreSQL insert/read regression
test is intentionally gated by `TEST_DATABASE_URL` and was skipped because no
disposable database was selected. No database mutation, migration, deployment,
restart, commit, or push occurred.

## 2026-09-08 - Per-operation OCPP protocol-evidence source slice

- Uncommitted source binds each CMS-owned CPO operation to its trace root and
  writes action-specific, bounded OCPP CALL/CALLRESULT/CALLERROR evidence only
  after the pinned OCPP library accepts the websocket write. Pairing is solely
  by OCPP unique ID; trace rows remain diagnostic and never assert a later
  physical charger effect.
- `GET_CONFIGURATION` now uses the same audited operation path. Its immediate
  v1 response can include a safe, redacted configuration projection only when
  the synchronous OCPP confirmation arrives. ChangeConfiguration values cannot
  cross the final trace persistence boundary.
- Migration `021_add_charger_operation_trace_evidence` is source-only. No
  migration, database mutation, deployment, restart, commit, or push occurred.

Focused trace-sanitizer tests and compile checks pass. PostgreSQL lifecycle,
CMS delivery, websocket write behaviour, and a physical mapped-charge-point
check remain unrun because no disposable environment or charger was selected.

## 2026-09-04 - CPO charger-operation source vertical, locally verified and not deployed

- The uncommitted new-HAL source adds a dedicated `v1_charger_operations`
  ledger and typed authenticated REST boundary for CMS-owned Reset,
  UnlockConnector, ChangeAvailability, ClearCache, Get/ChangeConfiguration,
  and allowlisted TriggerMessage. It does not reuse Start/Stop records or alter
  their charging semantics.
- Mapping validation retains CPO, CMS charger/connector, OCPP identity, and
  connector-number scope. A durable operation is claimed once before OCPP
  dispatch; any uncertain physical delivery becomes
  `RECONCILIATION_REQUIRED` and is never replayed. Exact CMS operation-ID GET
  is the recovery authority. OCPP results are stored as separate evidence, not
  a claim of later physical effect.
- Migration `020_add_v1_charger_operations` is source-only. No migration,
  database mutation, deployment, restart, commit, or push occurred.

Verification: focused memory-store and v1 HTTP tests, full `go test -p 1
./...`, and `go vet -p 1 ./...` pass. PostgreSQL lifecycle and a real OCPP
mapped-charge-point check are unrun because no disposable `TEST_DATABASE_URL`
or hardware environment was selected.

## 2026-09-03 - Charging-trace completeness source work, verified locally and not deployed

- Connector status diagnostics now classify an unbound CMS root as `STARTING`,
  an active associated HAL transaction as `CHARGING`, and a completed
  transaction as `POST_STOP`. The durable transaction—not a `Finishing` or
  `Available` status string—remains the only lifecycle input.
- The HAL trace now distinguishes RemoteStart/RemoteStop wire requests from
  charger confirmations, records safely-correlated Authorize request and
  confirmation evidence without credential material, records automatic-stop
  creation only for automatic workflow creation, and labels local start/stop
  persistence as HAL-to-HAL. Successful `transaction.started` and
  `transaction.completed` fact delivery additionally records the actual
  HAL-to-CMS boundary after the authoritative fact outbox is marked delivered.
- Trace append failures remain diagnostic-only and cannot alter OCPP
  confirmations, runtime projection, command/fact delivery, or transaction
  authority. No migration, database mutation, deployment, restart, commit, or
  push occurred in this source worktree.

## 2026-09-02 - Post-stop connector-status trace phase correction source verified, not deployed

- This historical source pass introduced transaction-based status
  classification. Its unbound-root `CHARGING` fallback was superseded by the
  2026-09-03 source correction above: an unbound pre-materialization root is
  `STARTING`, while active/completed transactions remain `CHARGING`/
  `POST_STOP`. It never infers phase from `Finishing`, `Available`, or another
  connector status string, and skips only the diagnostic append if a bound
  transaction cannot be read.
- The trace summary now records the sanitized OCPP status (for example,
  `Connector status: Available`), while `data.status` and
  `data.connector_id` are unchanged. Connector runtime persistence, registry
  updates, fact delivery, and StatusNotification acknowledgement semantics are
  unchanged. No migration, deployment, restart, or database mutation occurred.

## 2026-09-02 - Trace delivery PostgreSQL type fix source verified, not deployed

- `MarkV1TraceDelivery` now explicitly casts every reused status parameter to
  `varchar(32)`, matching `v1_trace_delivery_outbox.status`. This prevents the
  PostgreSQL `42P08` type-inference failure observed after a trace-delivery
  claim. It does not alter claim, retry, delivery, fact, charging, or OCPP
  semantics.
- The current pending and expired-delivery-lease rows remain reclaimable by the
  existing claim predicate. No queue repair, migration, database mutation,
  deployment, or restart was performed.

## 2026-09-02 - Diagnostic trace delivery push pipeline source verified, not deployed

- The current source adds additive migration 019 and an independent durable
  trace-delivery outbox. A dedicated worker posts immutable sanitized trace
  envelopes to CMS under its own bearer, retry/lease/timeout policy, without
  sharing `v1_fact_outbox` or changing OCPP acknowledgement semantics.
- The obsolete private HAL trace read has been retired. HAL keeps diagnostic
  evidence local until delivery succeeds or reaches explicit reconciliation;
  trace append/delivery failure is observable but never changes transaction,
  connector, command, fact, or commercial truth. No migration was applied,
  service restarted, or deployment performed.

## 2026-09-01 - Charging transaction trace source implementation verified, not deployed

- The current source adds additive migration 018, durable
  connector-aware diagnostic trace roots/events, private CMS-only trace reads,
  sanitizer-at-persistence, bounded cursor reads/retention, and OCPP lifecycle
  evidence. CMS-created roots bind to the eventual StartTransaction identity;
  charger-only roots are created only when no such root exists.
- The source also adds a guarded migration command that runs reviewed DDL under
  a configurable application/schema role and verifies resulting ownership, plus
  a non-mutating startup privilege gate for required v1 relations including the
  migration-017 completion ledger. No migration, database repair, deployment,
  service restart, or charger acceptance was performed.

`ocpp-hal-go-new` is a PostgreSQL-backed OCPP 1.6 HAL for
`ev-cms-backend-new`. It accepts only enabled durable v1 charger mappings and
only exposes the authenticated v1 service boundary.

## Implemented

- STOP lifecycle persistence now has one canonical row order: transaction,
  stop workflow, remote commands, then outbox facts. RemoteStop acceptance and
  charger StopTransaction completion therefore serialize instead of acquiring
  transaction/workflow rows in opposite orders. Only uncommitted PostgreSQL
  deadlock or serialization failures retry, never an OCPP remote command.
  Migration 017 adds an additive, backfilled terminal-completion key ledger:
  new `transaction.completed` facts reserve one HAL transaction aggregate while
  preserving any historical outbox rows. The migration and PostgreSQL race
  regression remain unrun without an explicit disposable `TEST_DATABASE_URL`.

- V1 command/transaction records now retain customer selection independently
  from energy and duration threshold provenance. CMS may supply both physical
  limits without HAL predicting one dimension from the other. Meter/deadline
  stops report customer ENERGY/TIME/MONEY versus WALLET truthfully; migration
  016 is additive and source-only/unrun.

- V1 start commands now persist an immutable CMS limit classification (`AUTO`,
  `ENERGY`, `TIME`, `MONEY`) with independently optional energy and duration
  thresholds. The existing meter/deadline stop workers remain the only
  enforcement path; a MONEY-derived threshold records `MONEY_LIMIT` rather
  than falsely presenting an energy/time customer selection. Migration 015 is
  additive and has not been applied in this source-only slice.

- Optional OCPP 1.6 SoC is now retained as charger-observed telemetry when
  supplied. HAL accepts only `SoC` in Percent (or its omitted standard-unit
  form), persists nullable first/latest percentage, observation time, and a
  separate accepted SoC sequence, and emits immutable `transaction.soc` facts.
  Missing evidence remains unknown and never affects energy, limits, billing,
  or completion. Additive migration 014 is source-only and unrun.

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
  startup sessions, and optional periodic metering. One process now models one
  charge point/WebSocket with independent per-connector transaction, meter,
  status, pending remote command, and automatic-meter state. One shared rounded
  integer Wh conversion is used for StartTransaction, MeterValues, and
  StopTransaction on each connector. It remains a test client, not HAL runtime
  behavior or a durable source of truth.
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
