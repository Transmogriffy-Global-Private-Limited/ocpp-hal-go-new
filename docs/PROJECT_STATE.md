# Project State

## Current Purpose

This repository is now the independently evolving OCPP HAL for `ev-cms-backend-new`. The architecture bootstrap establishes that purpose; it does not change the copied Go runtime in this slice.

## Inherited Functionality That Currently Exists

The copied service currently contains:

- an OCPP 1.6 central system implemented through `ocpp-go`;
- charger connection tracking with a stale-disconnect generation guard;
- charger-originated StartTransaction/StopTransaction persistence in PostgreSQL when configured, with a memory fallback for non-durable local use;
- live energy-register handling, local transaction snapshots, and max-kWh RemoteStop retry logic;
- a durable callback outbox with retry, dedupe, and reconciliation behavior;
- boot/reconnect recovery of local open transactions;
- an optional external charger-directory validator and cache;
- legacy REST command/status routes, frontend status and transaction WebSockets, virtual chargers, smoke clients, mock hooks, and regression scripts.

These are implementation facts, not a promise that each behavior is retained or exposed to the new CMS. The evidence and preservation properties are in `INHERITED_HAL_AUDIT.md`.

## Legacy-Shaped Functionality Still Present

- `/api/*` commands, static API-key protection, flexible legacy JSON aliases, and status semantics originate from the old integration.
- Config contains old CMS callback URL names/defaults and an optional legacy charger-directory endpoint.
- Start/completion callback payloads and the `max_kwh` response are legacy integration behavior.
- Single-session flags and alternate callback routing are inherited and process-local.
- Frontend WebSockets expose legacy status/transaction shapes without new-CMS customer/CPO authorization.
- The Go module/import path still refers to the legacy repository identity. Build and regression scripts now derive this checkout root from their own script location.

None of these are an approved permanent new-CMS contract.

## Approved v1 Architecture and Foundation

`docs/contracts/CMS_HAL_CHARGING_V1.md` is the human-approved v1 CMS/HAL
architecture and service contract. It defines the service-only HTTP command and
fact paths, identity chain, durable command/fact model, tariff/hold/settlement
invariants, production durability, live charger/connector/meter projection,
and recovery semantics. An additive migration and focused in-memory v1 state
foundation now exist, along with a production PostgreSQL configuration guard.
No v1 route, PostgreSQL v1-store operation, service-authentication mechanism,
or User App charging operation exists in runtime.

## Not Yet Implemented for `ev-cms-backend-new`

- an explicit authenticated HAL-to-CMS and CMS-to-HAL service boundary;
- CPO/customer/service identity, authorization, and audit context propagation;
- the approved v1 command, result, OCPP-start, meter, OCPP-stop, recovery, or
  billing contract in runtime;
- approved identifier correlation in durable CMS/HAL records and CMS
  charging-session projections;
- CMS-owned wallet/tariff/eligibility decision integration in the v1 flow;
- HAL connection, connector StatusNotification, and MeterValues fact delivery
  plus CMS durable operational/session polling projections;
- approved source of truth and access policy for charger directory/status and customer realtime projections;
- new-CMS machine-readable API/event contracts, interactive documentation, and contract-drift checks;
- migration/rollout plan for replacing legacy callbacks, routes, and frontend surfaces.

The earlier analysis remains historical evidence in
`docs/plans/CMS_HAL_CHARGING_INTEGRATION_ANALYSIS.md`. The v1 contract is now
approved. The v1 foundation changes only production store selection behavior:
`HAL_ENVIRONMENT=production` now requires PostgreSQL configuration.

## Verification Status

The architecture-bootstrap, analysis, contract-freeze, and script-root
remediation slices did not change runtime behavior. The v1 foundation adds a
production durability guard and unintegrated additive state/schema code;
`go test ./internal/store` passed. It does not claim v1 route, database,
charger, CMS, or end-to-end verification. `go test ./...` was previously
terminated after 124 seconds with no diagnostic output, and `go build ./...`
previously failed while linking `cmd/cpconsole` because the Go runtime could not
allocate memory. The build/regression scripts now target this checkout, but
their database-backed execution remains unverified in this environment.
