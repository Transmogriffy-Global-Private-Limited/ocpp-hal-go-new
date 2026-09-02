# Development Plan

## Objective

Deliver one independently deployable OCPP HAL for `ev-cms-backend-new`, with
no shared database and no supported old-CMS compatibility runtime.

## Current Execution

Current phase: v1 consumer-boundary verification and legacy retirement.

Current trace/migration-ownership slice:

- source implements a connector-aware, durable diagnostic transaction trace
  readable only by CMS over the service bearer, with bounded retention and no
  authority over OCPP, transactions, connector state, facts, or CMS commerce;
- additive migration 018 is source-only and guarded by the new configurable
  application-role migration path. Runtime starts only after a read-only check
  confirms the application role can access the required v1 relations,
  including the migration-017 completion ledger. No migration or deployment
  has occurred.

Verified source-only continuation:

- migration 019 adds a diagnostic trace-delivery outbox distinct from
  `v1_fact_outbox`; a separate worker posts immutable, sanitized trace events
  to CMS `POST /v1/hal-trace-events` under its own bearer, capacity, claim,
  retry, and timeout policy. The prior private trace GET is retired because no
  supported product consumer remains. Delivery is evidence-only, preserves
  complete valid trace events subject to bounded mechanics/retention, and never
  changes OCPP or fact behaviour. This source work is not deployed and no
  migration has been applied; see
  `docs/work/archive/WI-20260902-trace-delivery-push-pipeline.md`.

Completed in this slice:

- customer-selected/wallet-derived limit contract: existing V1 start,
  transaction, fact, and automatic-stop paths retain independent customer
  intent plus energy/duration source provenance; no parallel command or worker
  was added;

- v1-only process startup, mapping-based charger admission, authenticated HTTP
  command/query boundary, durable OCPP truth, facts, and reconciliation;
- generic contract-receiver test covering real PostgreSQL, HAL HTTP, OCPP
  remote commands, StartTransaction, MeterValues, StopTransaction, and fact
  delivery;
- retirement of legacy `/api/*`, callback/max-kWh, directory, frontend
  WebSocket, single-session, automatic offline-auth policy, and legacy smoke
  runtime; and
- Go module identity correction.
- Current connection-liveness slice: accepted current-connection Heartbeats
  renew durable `ONLINE` evidence and ordered facts without changing generation;
  the requested OCPP cadence is configurable with a five-minute default.
- Software-charger automation slice: `cpconsole` now models Boot-driven
  Heartbeats and an optional one-shot normal charging session without creating
  a second protocol or state-machine path.
- Software-charger multi-connector slice: one `cpconsole` Charge Point/WebSocket
  now owns independently selectable connector state, exact remote-start/stop
  routing, and independent transaction-bound automatic meter workers using real
  elapsed sample time; no HAL server or CMS path is changed.
- Current cross-service hardening: all v1 HTTP response projections now use
  explicit snake_case transport views, command responses have a typed OpenAPI
  schema, and fact delivery exposes only bounded receiver classifications while
  retaining the same immutable retry body. Deployment and dual-service
  acceptance remain required.
- HAL-wide fail-closed hardening: strict startup configuration, error-returning
  durable UUID generation, classified OCPP acknowledgement boundaries,
  connection-projection retry, multi-connector fault aggregation, and audited
  exact-fact requeue are implemented in source. The additive reconciliation
  audit migration still needs disposable-PostgreSQL verification.
- Fact-delivery lease fencing and explicit-empty configuration semantics are
  implemented in source. Migration 010 adds the nullable claim token; a
  delayed former lease owner is rejected instead of overwriting a reclaimed
  fact. PostgreSQL confirmation remains pending a disposable database.
- Meter quantization evidence hardening is implemented in source. Migration
  011 preserves raw stop readings while an effective nondecreasing stop meter
  may normalize only a coherent one-Wh register discrepancy. `cpconsole` now
  uses one rounded OCPP meter conversion at every transaction boundary.
- Real-hardware identity and operating-profile hardening is implemented in
  source: strict optional serial admission, durable Boot metadata evidence, and
  per-boot generation-fenced standard configuration reconciliation. Migrations
  012 and 013 plus physical-device acceptance remain pending.
- Optional charger SoC telemetry is implemented in source: MeterValues now
  independently accepts valid percentage SoC, persists first/latest evidence
  with a separate sequence, and delivers additive `transaction.soc` facts.
  Migration 014 and cross-service/cpconsole acceptance remain pending.
- STOP lifecycle deadlock hardening is implemented in source: every concurrent
  mutation now locks transaction, workflow, commands, and facts in that order;
  only uncommitted PostgreSQL deadlock/serialization aborts retry locally.
  Migration 017 adds a backfilled terminal-completion key ledger so new
  `transaction.completed` facts are one logical fact per HAL transaction
  without deleting historical outbox evidence.

## Next Approved Work

1. Apply and verify additive migrations 010 through 017 against a
   clearly disposable PostgreSQL database, including stop-worker crash and
   recovery behavior.
2. Expand real-device acceptance coverage for energy/time stop races and
   charger-specific MeterValues variants before production rollout.
3. Apply and verify migration 018 through `cmd/migrate` with the configured
   application/schema role and a disposable PostgreSQL database, including
   relation-owner/privilege checks.
4. Implement CMS-owned consumer projections, commercial flow, and UI in
   `ev-cms-backend-new` using the frozen v1 contract.

## Open Decisions

- token rotation and production transport topology;
- RFID/offline authorization and future OCPP control commands;
- commercial overshoot/debt policy; and
- whether historical legacy database tables warrant a separate destructive
  retirement migration.
