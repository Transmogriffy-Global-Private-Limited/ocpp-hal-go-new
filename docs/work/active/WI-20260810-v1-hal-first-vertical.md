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
- Exact service-authentication mechanism, key rotation, and deployment topology
  remain an explicit open architectural decision. Do not silently repurpose a
  legacy CMS API key as the new service identity.

## Contract impact

- Extends the approved v1 contract with the human-required time constraint and
  consumer-demand implementation matrix.
- Routed service authentication and machine-readable contract details require a
  deliberately selected service-auth mechanism.

## Data and migration impact

- Expected additive PostgreSQL state for v1 commands, start credentials,
  operational projections, integer-Wh transaction state, stop coordination, and
  fact delivery.

## Current state

- Consumer demand was inspected read-only in the CMS. Current User App
  availability is intentionally `UNKNOWN`; its charging-session read model is
  not implemented. Current CPO charger status is static administrative CMS
  status, not live OCPP state.
- HAL retains useful inherited OCPP and recovery behavior. The repository now
  has an additive v1 schema and focused in-memory command/credential/
  transaction state foundation, but no PostgreSQL v1-store implementation,
  OCPP handler wiring, v1 route, or fact delivery worker.

## Verification

- Before runtime changes: inspect call sites, migrations, current stores, and
  existing test/smoke tools.
- After runtime changes: focused Go tests and virtual-charger verification,
  followed by `scripts/build-all.ps1`, `scripts/regression-local.ps1 -SkipBuild`,
  `git diff --check`, and a complete diff review.

## Handoff

Continue by implementing in dependency order: persistence/configuration, durable
start command and credential validation, live state/facts, transaction meter and
stop coordination, then authenticated routes/OpenAPI once service authentication
is selected. The current foundation has not been wired to production paths.

## Completion

Not complete.
