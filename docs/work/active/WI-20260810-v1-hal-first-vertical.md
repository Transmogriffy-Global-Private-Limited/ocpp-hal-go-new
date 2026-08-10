# WI-20260810-v1-hal-first-vertical

Status: In Progress
Owner: Codex
Collaborators: None
Started: 2026-08-10
Last updated: 2026-08-10

Development-plan reference: `docs/DEVELOPMENT_PLAN.md`
Detailed-plan reference: `docs/plans/HAL_V1_CONSUMER_DEMAND_MATRIX.md`
Issue/PR reference: None

## Outcome

Implement the smallest complete HAL-side first charging vertical slice for
`ev-cms-backend-new` by 2026-08-14: durable commands and start credentials,
charger-originated transaction truth, live operational facts, near-live integer
Wh progression, unified stop enforcement, query/reconciliation sockets, and
durable fact delivery.

## Scope

- HAL OCPP, persistence, recovery, HTTP service boundary, configuration,
  migrations, verification tooling, OpenAPI/integration documentation, and
  project memory in this repository only.
- Read-only consumer-demand evidence from `ev-cms-backend-new`.
- Preserve useful inherited OCPP, connection-generation, transaction-truth,
  outbox, recovery, and smoke-tooling properties.

## Non-goals

- Changes in `ev-cms-backend-new` or `OCPPHAL_Go`.
- User App or CPO business APIs, RBAC, tariffs, wallets, billing, customer
  notifications, customer realtime transport, generalized RFID, reservations,
  smart charging, a telemetry platform, or broker infrastructure.
- Broad module-path renaming or legacy-surface removal without a separate
  migration plan.

## Claimed surfaces

- `internal/ocpp16hal/`
- `internal/store/` and `migrations/`
- `internal/httpapi/` and service-boundary documentation
- `internal/config/`, `cmd/ocpphal/`, regression/smoke tooling as necessary
- `docs/contracts/`, `docs/plans/`, project memory, and API contract material

## Dependencies and blockers

- The v1 charging contract is human-approved.
- Opaque bearer service authentication is implemented. Token rotation and
  production transport topology remain operational decisions.

## Contract impact

- Implements the approved mapping/start/reconciliation/runtime portion of the
  contract. Meter/fact/stop completion surfaces remain explicitly deferred.

## Data and migration impact

- Expected additive PostgreSQL state for v1 commands, start credentials,
  operational projections, integer-Wh transaction state, stop coordination, and
  fact delivery.

## Current state

- PostgreSQL mapping, command, credential, transaction, and runtime state is
  implemented and OCPP-wired. A virtual OCPP charger verifies RemoteStart,
  Authorize, StartTransaction materialization, and runtime state.
- Meter projection, fact delivery, unified stop coordination, StopTransaction
  completion, and CMS consumer work are not implemented.

## Verification

- Before runtime changes: inspect call sites, migrations, current stores, and
  existing test/smoke tools.
- After runtime changes: focused Go tests and virtual-charger verification,
  followed by `scripts/build-all.ps1`, `scripts/regression-local.ps1 -SkipBuild`,
  `git diff --check`, and a complete diff review.

## Handoff

Continue in the dependency order in `docs/DEVELOPMENT_PLAN.md`: meter
projection, durable fact delivery, unified stop coordination, completion truth,
then lifecycle torture tests. Do not treat this partial HAL vertical as the full
charging lifecycle.

## Completion

Not complete.
