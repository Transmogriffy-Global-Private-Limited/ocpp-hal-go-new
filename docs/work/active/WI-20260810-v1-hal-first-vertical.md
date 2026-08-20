# WI-20260810-v1-hal-first-vertical

Status: In Progress
Owner: Codex
Collaborators: None
Started: 2026-08-10
Last updated: 2026-08-20

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

- User App or CPO business APIs, RBAC, tariffs, wallets, billing, customer
  notifications, customer realtime transport, generalized RFID, reservations,
  smart charging, a telemetry platform, or broker infrastructure.
- Changes in `ev-cms-backend-new` or the legacy `OCPPHAL_Go` repository.

## Claimed surfaces

- `internal/ocpp16hal/`
- `internal/store/` and `migrations/`
- `internal/httpapi/` and service-boundary documentation
- explicit HTTP response DTOs and contract tests for every externally emitted
  v1 store projection
- `internal/v1facts/` receiver-error classification diagnostics for the active
  CMS fact-projection incident; the existing exact transaction lookup remains
  the recovery socket.
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
- Fact creation now normalizes the immutable envelope `occurred_at` to UTC
  microsecond precision before hashing and persistence; reload normalizes the
  same field before delivery. A PostgreSQL round-trip regression covers a
  non-microsecond-aligned timestamp.
- Startup connection recovery now resets the process-scoped generation baseline
  after marking prior runtime `UNKNOWN`. Durable connection sequence remains the
  cross-restart ordering value, while the live tracker still fences a
  superseded same-process disconnect.
- Accepted current-connection Heartbeats now renew the durable `ONLINE`
  observation through the same ordered connection-fact stream without changing
  generation. The v1 contract deliberately uses Heartbeats only for renewal to
  bound fact volume; CMS owns the longer independent stale horizon.
- The retained OCPP-native `cpconsole` now has Boot-driven automatic
  Heartbeats, an optional one-shot local session, and optional periodic
  MeterValues while preserving the manual terminal state machine. This is test
  tooling only and does not alter the HAL service contract.
- 2026-08-19 joint boundary repair: CMS has observed repeated 500 responses
  after HAL persisted `transaction.started`, leaving charger truth without a
  CMS session. HAL must retain durable retry semantics and add only safe,
  bounded receiver error-class diagnostics; it must not stop a physical
  transaction or change charger/OCPP truth because CMS projection failed.
- 2026-08-20 confirmed a second joint-boundary defect: direct serialization of
  `store.V1RemoteCommand` produced Go field names, so CMS decoded a 2xx
  command response with zero UUID identities. This slice replaces accidental
  store serialization with explicit snake_case HTTP views across the v1
  response boundary; no storage migration is expected.
- Remaining work is end-to-end verification expansion: real full lifecycle
  with a fact receiver, manual/energy/time/natural stop, concurrent stop races,
  and restart/crash scenarios. CMS consumer work is not in scope.

## Verification

- Before runtime changes: inspect call sites, migrations, current stores, and
  existing test/smoke tools.
- After runtime changes: focused Go tests and virtual-charger verification,
  followed by `scripts/build-all.ps1`, `scripts/regression-local.ps1 -SkipBuild`,
  `git diff --check`, and a complete diff review.
- This pass still requires final full-suite, vet, build-script, regression
  script, and complete-diff verification after the v1-only retirement edits.
- 2026-08-14 fact timestamp hardening: focused `internal/store` and
  `internal/v1facts` tests, `go test ./...`, and `go vet ./...` pass with
  database environment variables removed. The new PostgreSQL round-trip
  regression is skipped until a clearly disposable `TEST_DATABASE_URL` is
  supplied; it has not been run against a runtime database.
- 2026-08-14 connection restart hardening: focused tracker and store tests,
  `go test ./...`, and `go vet ./...` pass with database environment variables
  removed. The disposable PostgreSQL regression is skipped without
  `TEST_DATABASE_URL` and has not run against a runtime database.
- 2026-08-14 connection liveness hardening: focused tracker/config/store tests,
  `go test ./...`, and `go vet ./...` pass with database environment variables
  removed. The disposable PostgreSQL renewal regression is skipped without
  `TEST_DATABASE_URL`. The physical charger procedure is documented in the CMS
  boundary guide and requires a separately approved deployment.
- 2026-08-14 cpconsole automation: focused and race-enabled cpconsole tests,
  `go test ./...`, `go vet ./...`, and `scripts/build-all.ps1` pass. The
  PostgreSQL regression script and physical charger check remain unrun because
  no disposable database or deployment approval was supplied.
- 2026-08-20 explicit response-boundary repair: endpoint-level command JSON
  tests, OpenAPI required-field test, `go test ./internal/httpapi
  ./internal/store`, full `go test ./...`, `go vet ./...`, and
  `git diff --check` pass. PostgreSQL and dual-service deployment acceptance
  remain unrun; this source-only slice did not mutate a database.

## Handoff

Complete the remaining verification matrix before archiving this item. Do not
claim CMS integration or customer-facing completion; this repository only owns
the HAL side and the receiver contract.

## Completion

Not complete.
