# AI-assisted changelog

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
