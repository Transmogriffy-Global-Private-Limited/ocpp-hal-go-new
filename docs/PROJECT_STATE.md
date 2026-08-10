# Project State

## Purpose

`ocpp-hal-go-new` is the independent OCPP HAL for `ev-cms-backend-new`.
It owns OCPP transport, charger connection/status, charger command delivery,
and charger-originated protocol truth. It does not own customer identity,
eligibility, pricing, wallets, billing, or customer-facing projections.

## Verified v1 HAL Functionality

- `.env` loading uses process environment over local `.env` over defaults;
  `.env` remains ignored and `scripts/generate-env.ps1` creates local values.
- Additive migrations `005` and `006` create HAL-owned v1 command,
  credential, mapping, transaction, and runtime state.
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
- `API_DOCS_ENABLED=true` serves `/openapi.json` and `/docs` from the same
  v1 OpenAPI source and loopback request explorer.

## Inherited Functionality Still Present

The copied OCPP 1.6 system, legacy transaction/outbox store, callback worker,
max-kWh stop behavior, boot recovery, charger directory, `/api/*` routes, and
frontend WebSockets remain. They are not v1 CMS contracts and were not retired
or redirected by this slice.

## Not Implemented for the New CMS

- MeterValues projection/current consumed Wh and `transaction.meter` facts.
- Durable HAL-to-CMS fact delivery and its outbox worker.
- Unified stop coordinator, user/CPO/energy/time stop requests, RemoteStop
  orchestration, StopTransaction v1 completion, and `transaction.completed`.
- CMS consumer implementation, CMS operational/session projections, business
  authorization, tariffs, holds, billing, settlement, customer APIs, and
  customer realtime.
- Credential rotation/deployment topology, RFID, smart charging, and legacy
  retirement sequencing.

## Verification Status

`go test ./...`, focused PostgreSQL store durability/concurrency test, clean
PostgreSQL migration application, and the PostgreSQL-backed virtual OCPP
HTTP-to-RemoteStart-to-Authorize-to-StartTransaction test pass. Full legacy
regression and complete meter/stop/fact lifecycle verification remain pending.
