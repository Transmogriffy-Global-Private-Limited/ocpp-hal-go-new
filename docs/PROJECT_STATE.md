# Project State

## Purpose

`ocpp-hal-go-new` is the independent OCPP HAL for `ev-cms-backend-new`.
It owns OCPP transport, charger connection/status, charger command delivery,
and charger-originated protocol truth. It does not own customer identity,
eligibility, pricing, wallets, billing, or customer-facing projections.

## Verified v1 HAL Functionality

- `.env` loading uses process environment over local `.env` over defaults;
  `.env` remains ignored and `scripts/generate-env.ps1` creates local values.
- Additive migrations `005`, `006`, and `007` create HAL-owned v1 command,
  credential, mapping, transaction, stop-workflow, runtime, and fact-outbox
  state.
- `HAL_V1_ENABLED=true` requires PostgreSQL and
  `HAL_V1_CMS_BEARER_TOKEN`; production still requires PostgreSQL.
- `PUT /v1/mappings/chargers/{cms_charger_id}` persists auditable,
  conflict-safe CPO/charger/connector mappings.
- Authenticated v1 start persists an idempotent command and short-lived,
  one-use `appv1_` credential before inherited RemoteStart dispatch.
- `Authorize` rejects unknown/expired/wrong-charger v1 credentials while
  preserving the inherited non-v1 credential path.
- Charger-originated `StartTransaction` atomically creates exactly one v1
  transaction, preserves retransmission identity, stores integer Wh start,
  limit/duration, and derives deadline from actual start time.
- Authenticated command/transaction reconciliation and mapped charger/connector
  runtime queries exist. Connection reset on startup makes historical ONLINE
  state `UNKNOWN`; stale disconnect persistence is generation-guarded.
- `StatusNotification` preserves exact OCPP status and supplied fault details.
  Runtime responses expose freshness as false unless connection state is ONLINE.
- Exact v1 MeterValues identify the transaction by charger OCPP identity plus
  OCPP transaction ID, convert exact integer Wh (including integral kWh), keep
  an accepted monotonic meter sequence, reject regressive values, and never
  fabricate meter samples.
- V1 energy limits use the approved accepted-sample threshold. Time limits use
  the durable deadline derived from charger-originated actual start time. Both,
  CMS customer stop, and CPO stop converge on one transaction stop workflow.
- `POST /v1/remote-commands/stop` validates the complete mapping/transaction
  correlation, is idempotent by CMS command ID, and does not treat RemoteStop
  acknowledgement as completion. Charger-originated StopTransaction records
  final meter and OCPP reason separately from requested stop provenance.
- Immutable `transaction.started`, `transaction.meter`,
  `transaction.completed`, `charger.connection.updated`,
  `connector.status.updated`, and meaningful `command.updated` facts are
  written atomically with the corresponding HAL state transition.
- Optional `HAL_V1_FACT_DELIVERY_ENABLED=true` starts a PostgreSQL-backed HTTP
  fact worker using the separate outbound URL/token configuration. It retries
  transient receiver failures with the same fact ID and marks non-retryable
  receiver responses for reconciliation.
- `API_DOCS_ENABLED=true` serves `/openapi.json` and `/docs` from the same
  v1 OpenAPI source and loopback request explorer.

## Inherited Functionality Still Present

The copied OCPP 1.6 system, legacy transaction/outbox store, callback worker,
max-kWh stop behavior, boot recovery, charger directory, `/api/*` routes, and
frontend WebSockets remain. They are not v1 CMS contracts and were not retired
or redirected by this slice.

## Not Implemented for the New CMS

- CMS consumer implementation, CMS operational/session projections, business
  authorization, tariffs, holds, billing, settlement, customer APIs, and
  customer realtime.
- Credential rotation/deployment topology, RFID, smart charging, and legacy
  retirement sequencing.

## Verification Status

Source-level `go test ./...` passes. The migration chain through `007` was
applied to the disposable PostgreSQL database, and focused PostgreSQL store
tests pass there. The virtual-charger start/runtime integration is retained;
full fact/stop lifecycle coverage remains the final verification work for this
active slice.
