# AI-assisted changelog

## 2026-08-10 - v1 HAL meter, stop, completion, and fact vertical

- Added additive migration `007_add_v1_lifecycle_and_facts.sql` and HAL-owned
  durable stop workflows, command delivery-attempt state, fact delivery leases,
  recovery state, and immutable fact metadata.
- Wired exact v1 OCPP MeterValues into persistent integer-Wh current meter
  state and monotonic sequence facts; regressive/non-integral samples do not
  become accepted truth. Energy-limit and actual-start-time deadline triggers
  converge with customer/CPO requests on one RemoteStop workflow.
- Added authenticated idempotent `POST /v1/remote-commands/stop`, exact
  transaction/mapping validation, pre-network delivery-attempt recording,
  ambiguity recovery, and charger-originated StopTransaction completion with
  final meter/OCPP reason preserved separately from requested stop provenance.
- Added atomic immutable fact creation for command, transaction, meter,
  connection, and connector transitions plus the opt-in PostgreSQL outbox HTTP
  worker using independent outbound fact credentials.
- Updated the v1 OpenAPI source, local environment reference/generator,
  contract, plan, state, architecture, and work record.

Compatibility: inherited legacy routes, callbacks, WebSockets, and transaction
paths remain untouched. No CMS or legacy repository was modified.

Verification: source-level `go test ./...` passed; migration `001` through
`007` applied to the disposable PostgreSQL database; focused PostgreSQL store
test passed. Full virtual charger + fact receiver completion coverage remains
in progress and is not claimed by this entry.

## 2026-08-10 - v1 HAL mapping/start/runtime vertical

- Added additive migration `006_add_v1_mapping_and_runtime.sql`, PostgreSQL
  mapping/audit state, durable start commands/credentials/transactions, command
  state transitions, and charger/connector runtime projections.
- Added opaque `HAL_V1_CMS_BEARER_TOKEN` authentication, mandatory mutation
  idempotency/correlation headers, conflict-safe mapping enrollment, v1 start
  and reconciliation/runtime routes, OpenAPI, and a loopback interactive API
  explorer controlled by `API_DOCS_ENABLED`.
- Wired the v1 credential namespace into inherited `ocpp-go` handlers:
  `RemoteStartTransaction` only records command delivery, `Authorize` validates
  persisted short-lived credentials, and `StartTransaction` atomically creates
  exact durable v1 OCPP truth. Connection generation and StatusNotification
  now project to mapped durable runtime state.
- Added local `.env` precedence/loading, corrected the existing environment
  generator to operate from the checkout root, and replaced inherited
  production callback defaults in `.env.example` with safe loopback placeholders.

Compatibility: inherited legacy REST, callbacks, WebSockets, transaction
store, meter handling, and stop behavior remain in place. No CMS or legacy
repository was modified. Meter/fact/stop completion for v1 remains deferred.

Verification: applied migrations `001` through `006` to the disposable
PostgreSQL database; `go test ./...`; focused PostgreSQL durability/concurrency
test; and PostgreSQL-backed virtual OCPP HTTP-to-RemoteStart-to-Authorize-to-
StartTransaction integration test passed. No secret values were printed.

## 2026-08-10 - v1 HAL foundation and production durability guard

- Added the v1 consumer-demand matrix that derives generic HAL sockets/plugs
  from read-only User App and CPO evidence. It separates must-ship live
  operational capabilities from deferred CPO controls and inherited-only
  compatibility routes.
- Added an additive PostgreSQL foundation migration for v1 remote commands,
  app start credentials, authoritative transactions, charger/connector runtime
  projections, and durable fact outbox records. The migration is not applied by
  this repository automatically.
- Added typed v1 in-memory state and focused tests for command idempotency,
  credential charger/connector binding, replay-safe StartTransaction
  materialization, integer-Wh meter updates, actual-start-based time deadlines,
  and convergent stop ownership.
- Added `HAL_ENVIRONMENT`. With `HAL_ENVIRONMENT=production`, startup now fails
  when PostgreSQL is not configured instead of silently selecting in-memory
  transaction state. Non-production development retains the explicit memory
  fallback.

Compatibility: legacy REST, callback, transaction, OCPP, WebSocket, and smoke
behavior is not changed. The v1 foundation is not yet wired to OCPP handlers,
service routes, PostgreSQL store operations, fact delivery, or CMS integration.

Verification: `go test ./internal/store` passed. PostgreSQL migration execution,
full build, regression, and v1 OCPP integration remain unverified.

## 2026-08-10 - Approved CMS/HAL charging integration v1 contract

- Recorded the human-approved v1 architecture and authoritative service
  contract for CMS start intents, sessions, short-lived app credentials,
  durable HAL commands, immutable HAL facts, idempotency, reconciliation,
  tariff/tax snapshots, wallet holds, settlement, integer-Wh energy limits,
  production PostgreSQL durability, CPO suspension, and customer-state
  ownership.
- Chose v1 service-only HTTP paths for CMS commands/reconciliation and HAL fact
  ingestion, plus concrete command/fact JSON vocabulary and immutable fact
  identity/digest rules.
- Added the approved first-v1 live operational requirement: immutable HAL facts
  for generation-safe charger connection state, exact connector OCPP status,
  and selected near-live integer-Wh MeterValues; CMS polling remains the
  customer authority and no customer realtime transport is added.
- Preserved explicit open decisions for service-auth implementation, overshoot
  debt policy, energy guard formula, RFID, generalized realtime, legacy-surface
  retirement, migrations, and deployment topology.

Compatibility: the approved contract is documentation only. No Go runtime,
route, callback, database schema, migration, service credential, deployment,
CMS repository, or legacy repository behavior changed.

Verification: documentation-only diff, whitespace, scope, and artifact checks
are required for this slice. Full runtime regression is not required because no
runtime behavior changed.

## 2026-08-10 - CMS/HAL charging-integration analysis

- Added a decision-ready, evidence-based analysis of the first
  `ev-cms-backend-new` to `ocpp-hal-go-new` charging lifecycle.
- Recorded authority and identifier separation; start/stop truth; credential,
  wallet/hold, energy-limit, transport, recovery, duplicate, CPO suspension,
  realtime, and first-vertical-slice decisions requiring human approval.
- Refined the inherited audit to retain controlled/reconcilable limit-stop
  behavior without freezing its current retry count, separate removal of the
  single-session callback-routing mechanism from any future business policy,
  and distinguish test/dev memory storage from the production durability
  question.

Compatibility: no Go runtime behavior, API, callback, database schema,
authentication mechanism, or cross-repository integration changed. All CMS
inspection was read-only; no CMS or legacy-repository file was modified.

Verification: reviewed the complete documentation-only diff and ran the
repository documentation checks recorded below. No full regression was needed
or run because this slice changes no runtime behavior.

## 2026-08-10 - Script checkout-root remediation

- Updated `scripts/build-all.ps1` and `scripts/regression-local.ps1` to derive
  the repository root from `$PSScriptRoot` rather than an absolute
  `OCPPHAL_Go` path.
- Preserved the existing formatter, test, build, mock, PostgreSQL, job, and
  regression command sequence; no Go runtime behavior changed.

Verification: reviewed both script entry paths and confirmed no absolute
`OCPPHAL_Go` `Set-Location` remains. Full build/regression execution remains
subject to the existing Go memory and local PostgreSQL limitations.

## 2026-08-10 - New-CMS architecture bootstrap and inherited HAL audit

- Reframed this repository as the independently evolving OCPP HAL for `ev-cms-backend-new`, separate from the legacy `OCPPHAL_Go` and old-CMS integration.
- Replaced inherited project rules, overview, development plan, project state, and documentation navigation that treated legacy CMS/frontend compatibility as this repository's purpose.
- Added the canonical HAL/CMS architecture boundary record and an evidence-based inherited subsystem audit, including preservation properties for transaction truth, stale-disconnect protection, callbacks, retries, recovery, metering, and local tooling.
- Recorded that no new CMS/HAL endpoint, event, callback, table schema, or authentication design is approved yet, and sequenced a joint audit with `ev-cms-backend-new` wallet, tariff, and charging-session requirements.
- Recorded the inherited legacy-root paths in build/regression automation as a bootstrap defect so future work does not accidentally operate on the legacy repository.

Compatibility: no Go runtime behavior, route, database schema, callback payload, deployment, or external service was changed. The legacy repository and `ev-cms-backend-new` were not modified.

Verification: source paths and representative OCPP, persistence, recovery, callback, API, configuration, migration, and tooling call sites were inspected. `go test ./...` was terminated after 124 seconds with no diagnostic output. `go build ./...` reached `cmd/cpconsole` and failed because the Go linker could not allocate memory. Inherited scripts were intentionally not run because their absolute legacy path would operate outside this checkout. The final documentation diff and `git diff --check` were also required for this slice.

## 2026-08-03 — Terminal-controlled virtual charger

- Added `cmd/cpconsole`, a standalone configurable OCPP 1.6J virtual EV charger.
- Added coherent terminal-controlled local/remote transactions and meter progression.
- Added fault, suspension, remote-policy, diagnostics, firmware and trigger behavior.
- Added Windows build registration and Linux amd64/arm64 cross-build tooling.
- Added focused parser tests and the complete operator/integration guide.
- Registered canonical project-memory files and recorded existing OpenAPI remediation.

Compatibility: no HAL server route, database schema, callback payload, or runtime
ownership boundary changed. The new program is an additional test client.

Verification: focused tests, `go test ./...`, all Windows builds, Linux amd64
cross-build, and a loopback memory-store charge flow passed. The canonical
PostgreSQL regression could not run non-interactively because its password prompt
received no value; this entry does not claim that regression passed.
