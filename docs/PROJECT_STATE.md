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

## Not Yet Implemented for `ev-cms-backend-new`

- an explicit authenticated HAL-to-CMS and CMS-to-HAL service boundary;
- CPO/customer/service identity, authorization, and audit context propagation;
- approved command, result, OCPP-start, meter, OCPP-stop, recovery, or billing contracts;
- approved correlation between OCPP transaction truth and CMS charging-session projections;
- CMS-owned wallet/tariff/eligibility decision integration;
- approved source of truth and access policy for charger directory/status and customer realtime projections;
- new-CMS machine-readable API/event contracts, interactive documentation, and contract-drift checks;
- migration/rollout plan for replacing legacy callbacks, routes, and frontend surfaces.

The decision-ready but unapproved analysis for these gaps is in
`docs/plans/CMS_HAL_CHARGING_INTEGRATION_ANALYSIS.md`. It records evidence and
recommendations only; it does not change current runtime behavior or approve a
CMS/HAL transport contract.

## Verification Status

The architecture-bootstrap slice did not change runtime behavior, and the subsequent script-root remediation did not change test semantics. Neither slice claims new runtime, database, charger, CMS, or end-to-end verification. `go test ./...` was terminated after 124 seconds with no diagnostic output, and `go build ./...` failed while linking `cmd/cpconsole` because the Go runtime could not allocate memory. The build/regression scripts now target this checkout, but their database-backed execution remains unverified in this environment.
