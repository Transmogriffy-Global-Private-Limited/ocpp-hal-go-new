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

- Implements the approved HAL mapping/start/stop/reconciliation/runtime/fact
  portion of the contract. CMS receiving projections and commercial behavior
  remain outside this repository.

## Data and migration impact

- Expected additive PostgreSQL state for v1 commands, start credentials,
  operational projections, integer-Wh transaction state, stop coordination, and
  fact delivery.

## Current state

- PostgreSQL mapping, command, credential, transaction, stop-workflow, runtime,
  and fact-outbox state is implemented and OCPP-wired. Start commands record
  durable pre-network delivery attempts; recovery returns only pre-attempt
  windows to `PERSISTED` and marks attempted windows `AMBIGUOUS`.
- MeterValues updates exact active v1 transactions in integer Wh, emits a
  monotonic meter fact, and creates the shared energy-limit workflow. Durable
  deadline enforcement creates the same workflow. CMS customer/CPO stop joins
  it by exact transaction identity; StopTransaction is authoritative completion.
- The opt-in fact worker delivers immutable facts to the configured CMS receiver
  with separate outbound credentials and retained retry/reconciliation state;
  an expired `DELIVERING` lease is reclaimed as the same durable fact.
- This hardening pass corrected contract field drift (`started_at`, nullable
  completion command correlation, and command error evidence), replaced the
  local fact canonicalizer with RFC 8785 JCS, and added PostgreSQL lease/
  contention plus HTTP delivery classification tests.
- Remaining work is end-to-end verification expansion: real full lifecycle
  with a fact receiver, manual/energy/time/natural stop, concurrent stop races,
  and restart/crash scenarios. CMS consumer work is not in scope.

## Verification

- Before runtime changes: inspect call sites, migrations, current stores, and
  existing test/smoke tools.
- After runtime changes: focused Go tests and virtual-charger verification,
  followed by `scripts/build-all.ps1`, `scripts/regression-local.ps1 -SkipBuild`,
  `git diff --check`, and a complete diff review.
- This pass ran focused store/fact-worker tests, PostgreSQL lease/concurrency
  tests, the existing real PostgreSQL OCPP-start integration, `go test ./...`,
  `go vet ./...`, and `scripts/build-all.ps1`; it did not complete the
  remaining lifecycle matrix.

## Handoff

Complete the remaining verification matrix before archiving this item. Do not
claim CMS integration or customer-facing completion; this repository only owns
the HAL side and the receiver contract.

## Completion

Not complete.
