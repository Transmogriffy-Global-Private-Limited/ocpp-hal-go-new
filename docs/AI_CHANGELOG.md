# AI-assisted changelog

## 2026-08-31 - cpconsole one charge point with multiple connectors

- Replaced cpconsole's single scalar connector/transaction/meter state with one
  OCPP Charge Point/WebSocket and independently locked connector states. Startup
  boots once and reports `Available` for every configured connector.
- Added `-connectors` / `CP_SIM_CONNECTORS`, retained `-connector` as the
  initial terminal selection, and added `use <connector>` plus `state all`.
  Each connector has isolated metering, SoC, pending remote actions, and
  automatic-meter lifecycle.
- Remote start now validates an explicit connector or deterministically selects
  the lowest eligible `Available`/`Preparing` connector. Remote stop resolves
  the exact active OCPP transaction owner and cannot stop another connector.

Compatibility: HAL server, CMS, persistence, migrations, and deployment are
unchanged. This is cpconsole simulator behavior only.

Verification: focused cpconsole tests and race coverage pass. Broader source
verification is recorded with the active work item; no remote charger session,
database, migration, deployment, commit, or push was performed.

## 2026-08-26 - Preserve independent limit provenance through HAL

- Extended the existing V1 start command, durable command/transaction records,
  facts, query response, and automatic stop paths with independent energy and
  duration source fields. HAL can now distinguish `WALLET_LIMIT` from customer
  energy, time, and money limits without interpreting tariff or wallet policy.
- Added additive migration 016, backward inference for legacy omitted source
  fields, OpenAPI/contract documentation, and focused source/stop regression
  coverage. No additional command or stop worker exists.

Verification: focused HTTP, store, and OCPP HAL tests pass. Migration 016 and
live CMS/HAL/charger acceptance remain unrun.

## 2026-08-26 - Preserve customer limit type through the V1 HAL path

- Extended the existing V1 RemoteStart command, command/transaction store,
  facts, and query response with durable `limit_type`. Energy and duration
  limits are independently optional; zero means absent at the HTTP boundary.
- The existing meter/deadline stop workflows now use `MONEY_LIMIT` for a
  MONEY-derived threshold. No second command or stop worker was introduced.

Verification: focused store, HTTP, and OCPP HAL tests pass. Migration 015 and
real CMS/HAL/charger execution were not run.

## 2026-08-25 - Optional charger SoC telemetry

- MeterValues parsing now independently selects valid OCPP `SoC` percentage
  observations alongside existing integer-Wh energy. SoC-only packets persist
  and create a separate immutable `transaction.soc` fact; energy-only facts do
  not replay stale SoC.
- Added migration 014 for nullable initial/latest SoC, timestamp, and separate
  sequence. Latest SoC cannot regress on older observation time; missing SoC is
  unknown, never estimated, and does not affect lifecycle or billing.

Verification: focused OCPP/store/HTTP tests pass. PostgreSQL migration and
CMS-to-HAL-to-cpconsole acceptance require an explicitly disposable topology
and were not run.

## 2026-08-21 - Real-hardware identity and boot-configuration hardening

- Mapping enrollment now carries optional expected serial evidence. HAL admits
  only mapped enabled chargers through exactly `/{identity}` or
  `/{identity}/{serial}` and preserves canonical OCPP identity behind the
  ocpp-go final-path-segment transport detail.
- Added migrations 012 and 013 for expected serial and observed Boot metadata.
  Boot evidence is HAL-only and can expose a serial conflict without mutating
  CMS inventory.
- Every accepted Boot starts bounded generation-fenced reconciliation of the
  standard heartbeat/meter sample keys. Unsupported, readonly, rejected, and
  failed values are isolated; vendor-only legacy keys require an explicit
  exact-vendor profile and are disabled by default.

Verification: focused config, OCPP, HTTP, and store tests pass. PostgreSQL
migration and physical-charger acceptance remain unrun without an explicitly
disposable database and hardware session; no deployment or live mutation ran.

## 2026-08-21 - Meter quantization evidence and coherent simulator register

- Added migration 011 and explicit raw/effective stop-meter evidence. The HAL
  retains a raw stop reading, adjustment, and classification while preserving
  `meter_stop_wh` as the nondecreasing authoritative meter. Only a one-Wh
  StopTransaction rollback with eligible temporal evidence normalizes; larger
  rollbacks still reject completion. Periodic one-Wh rollbacks are durably
  counted without moving the meter or emitting a regressive fact.
- Completion facts keep schema version 1 and add the raw/adjustment/evidence
  fields only when available. CMS remains compatible because its receiver uses
  `meter_stop_wh` as the billing/session value and ignores additive fields.
- `cpconsole` now uses one checked rounded conversion for StartTransaction,
  MeterValues, and StopTransaction, removing its former rounded-versus-
  truncated register disagreement.

Verification: focused store, OCPP, and simulator tests pass. PostgreSQL
lifecycle verification requires `TEST_DATABASE_URL` and was not run without a
clearly disposable database; no database or deployment was changed.

## 2026-08-20 - Fact-delivery claim fencing and explicit config presence

- Added migration 010 and a secure fact-delivery claim token. Delivery state
  transitions now compare the exact fact and active token, preventing a stale
  worker from overwriting a reclaimed lease; one outbound delivery remains
  within the documented lease budget.
- Configuration now preserves explicit empty process/local values as supplied
  input and validates them rather than silently falling back. Only process
  `HAL_ENVIRONMENT` selects bootstrap environment/local-file behavior.

Verification: focused config, store, fact-worker, and HTTP tests pass. The
new disposable PostgreSQL fencing test skips without `TEST_DATABASE_URL`; no
database or deployment was changed.

## 2026-08-20 - HAL-wide fail-closed configuration, evidence, and fact recovery hardening

- Replaced silent malformed-configuration fallbacks with validated startup
  parsing, explicit environment vocabulary, range/coupling/URL checks, and
  production-safe local `.env` handling. Secure UUID generation now returns an
  error to every runtime persistence operation; it never manufactures a
  non-UUID fallback.
- Classified OCPP evidence rather than applying a blanket DB-error policy:
  irreplaceable transaction truth, mapped discrete status, and valid correlated
  meter observations cannot be acknowledged before durable persistence;
  unsupported/uncorrelatable meter input and refreshable Heartbeats preserve
  protocol interoperability. Physical connection projection retries exact
  observations and remains conservative across restart. Charger fault
  aggregation now derives from every connector.
- Added migration `009_add_v1_fact_reconciliation_audit.sql` and an
  authenticated exact-fact requeue operation. It audits the terminal receiver
  evidence and requeues the original immutable fact, never a replacement.

Verification: focused configuration, store, registry, OCPP, fact-worker, HTTP,
and main-package tests passed. PostgreSQL migration/lifecycle tests remain
unrun without a clearly disposable `TEST_DATABASE_URL` or `DATABASE_URL`; no
migration, database, deployment, or physical charger was used.

## 2026-08-20 - HAL v1 transaction-evidence and stop-recovery hardening

- Added migration `008` plus HAL receipt timestamps for started/completed v1
  transactions. HAL now uses receipt time, not a charger-provided timestamp,
  for credential/command expiry and duration enforcement; it retains protocol
  timestamps only as bounded evidence.
- Enforced positive OCPP/connector identities, nonnegative start meters,
  monotonic meter/completion evidence, conflicting-completion rejection, and
  mapping validation before connector runtime persistence. A persistence
  failure now makes the OCPP StopTransaction handler return an error instead
  of acknowledging data that HAL did not store.
- Added bounded periodic/startup dispatch for only proven pre-network stop
  workflows. DELIVERY_ATTEMPTED and AMBIGUOUS workflows remain non-replayable.
  The authenticated HTTP boundary now uses canonical nonzero UUID parsing and
  rejects trailing JSON values. OpenAPI documents the expanded v1 response
  schemas and state vocabulary.

Verification: focused store, HTTP, and OCPP tests, full `go test ./...`,
`go vet ./...`, and `scripts/build-all.ps1` passed. `regression-local.ps1
-SkipBuild` correctly stopped before running because `DATABASE_URL` is unset;
PostgreSQL lifecycle checks remain unrun without an explicitly selected
disposable database. No migration, runtime database, deployment, or physical
charger was used.

## 2026-08-20 - Explicit HAL v1 response boundary

- Replaced direct HTTP serialization of v1 store records with explicit response
  views. Start, stop, and exact-command lookup now return the required
  snake_case command contract, while mapping, transaction, stop-workflow, and
  runtime responses no longer accidentally expose persistence-only fields.
- Added endpoint-level JSON regression coverage for start, stop, and exact
  command lookup plus OpenAPI required-field coverage. No database migration,
  runtime data mutation, or deployment occurred.

Verification: focused `go test ./internal/httpapi ./internal/store`, full
`go test ./...`, `go vet ./...`, and `git diff --check` passed. No runtime
service, database, or physical charger was used.

## 2026-08-19 - CMS fact-receiver diagnostics and exact lookup compatibility

- Added explicit snake_case JSON field names to the existing exact transaction
  lookup response so the CMS recovery client can consume HAL-authoritative
  transaction truth by `cms_start_intent_id`.
- HAL fact delivery now safely records a bounded receiver error code with fact
  ID/type/status for retry and terminal-reconciliation diagnosis. It never
  reads or stores an error message/body, and CMS failure does not stop or alter
  a physical OCPP transaction.

Verification: `go test ./internal/v1facts ./internal/httpapi ./internal/store`
passed. Full PostgreSQL/dual-service acceptance remains separate.

## 2026-08-14 - cpconsole heartbeat and startup automation

- Added BootNotification-driven periodic Heartbeats to the OCPP-native
  `cpconsole` simulator. `-heartbeat-interval` and
  `CP_SIM_HEARTBEAT_INTERVAL` use `0` for the accepted server interval and a
  positive value for an explicit process-lifetime override.
- Added optional one-shot `-auto-start-id-tag`, `-auto-power-kw`, and
  `-auto-meter-interval` startup behavior. It invokes the existing plug,
  authorization, transaction, status, and meter operations, then preserves
  manual terminal control of that same state.
- Made simulator `HeartbeatInterval` configuration coherent with the effective
  scheduler. A valid remote reconfiguration reschedules an unoverridden worker;
  an explicit CLI/env override rejects the change deterministically.

Compatibility: HAL runtime, durable state, database schema, CMS, and legacy
repositories are unchanged.

Verification: focused cpconsole scheduler/configuration/automation tests,
full Go tests, vet, repository build, and diff checks are recorded with this
slice. No runtime service, database, or live charger was used.

## 2026-08-14 - durable Heartbeat connection liveness

- Added `OCPP_HEARTBEAT_INTERVAL_SECONDS` with a backward-compatible `300`
  second default and used it in accepted BootNotification confirmations.
- Accepted Heartbeats now renew only the current tracked, enabled mapping's
  existing `ONLINE` runtime. Renewal preserves generation, advances durable
  connection sequence and observation time, and emits the normal immutable
  connection fact. It cannot create or resurrect a connection.
- Kept connection fact volume bounded: v1 renews on Heartbeat only, not on
  high-frequency MeterValues or other charger-originated messages. The contract
  records the separate CMS connection-freshness responsibility and acceptance
  procedure.

Compatibility: no schema migration or legacy behavior change. Existing fact
digest canonicalization, startup `UNKNOWN`, process-scoped generation fencing,
and durable fact delivery remain unchanged.

Verification: `gofmt`; focused config, tracker, and store tests; `go test
./...`; and `go vet ./...` passed with database environment variables removed.
The disposable PostgreSQL renewal regression is skipped without
`TEST_DATABASE_URL`; no runtime database was used.

## 2026-08-14 - connection runtime restart generation recovery

- Defined connection generation as a live-process callback fence rather than a
  cross-restart ordering value. Startup now resets the durable generation
  baseline before projecting prior connection state as `UNKNOWN`, allowing the
  first legitimate new-process connection to publish `ONLINE` immediately.
- Tightened persistence fencing so only a newer generation, or the current
  generation's `ONLINE` to `OFFLINE` transition, can advance runtime state.
  Durable `connection_sequence` remains the CMS ordering value across restarts.
- Added focused tracker coverage for superseded and duplicate disconnects, and
  a disposable-`TEST_DATABASE_URL` regression covering stale durable generation,
  startup `UNKNOWN`, immediate new-process `ONLINE`, fact sequencing, and a
  stale prior-generation disconnect.

Verification: `gofmt`; focused `go test ./internal/ocpp16hal ./internal/store`;
`go test ./...`; and `go vet ./...` pass with database environment variables
removed. The PostgreSQL regression is skipped unless a clearly disposable
`TEST_DATABASE_URL` is supplied; it is not run against a runtime database.

## 2026-08-14 - durable fact timestamp canonicalization

- Normalized v1 fact-envelope `occurred_at` to UTC microsecond precision at
  the PostgreSQL outbox boundary before immutable-content hashing and
  persistence, then normalized the reloaded value before delivery.
- Made the durable fact model the shared owner of the delivery-envelope shape,
  so digest construction, PostgreSQL round-trip coverage, and worker delivery
  use the same immutable fields.
- Added a focused unit test and a disposable-`TEST_DATABASE_URL` PostgreSQL
  regression that creates a fact with non-microsecond nanoseconds, reloads it
  through the real claim path, reconstructs the delivered envelope, and
  verifies the receiver-side canonical digest equals `content_digest`.

Compatibility: the v1 fact contract and CMS integrity validation are unchanged.
Existing outbox rows are not rewritten; fact IDs, sequences, retries, and
reconciliation behavior are unchanged.

Verification: `gofmt`; focused `go test ./internal/store ./internal/v1facts`;
`go test ./...`; and `go vet ./...` passed with `DATABASE_URL` and
`TEST_DATABASE_URL` removed from the process environment. The PostgreSQL
round-trip regression requires a clearly disposable `TEST_DATABASE_URL` and
was not run against a runtime database.

## 2026-08-10 - v1-only HAL runtime and lifecycle proof

- Replaced the inherited old-CMS process wiring with the authenticated v1
  mapping, command, runtime, reconciliation, and immutable-fact boundary.
  Enabled durable mappings now admit charge points; unknown charge points cannot
  create runtime state.
- Retired the runtime registrations and tooling for legacy `/api/*` routes,
  callbacks, callback-derived `max_kwh`, external charger directory, frontend
  WebSockets, single-session routing, remote-only auth configuration, and old
  smoke binaries. The retained `cpconsole` remains an OCPP-native simulator.
- Retained `ocpp-go`, charger connection-generation safety,
  charger-originated StartTransaction/StopTransaction truth, integer-Wh meter
  facts, durable command/stop recovery, and the PostgreSQL fact outbox.
- Corrected the module identity to
  `github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new`, made
  scripts/configuration checkout-local and v1-only, and tightened mapped
  connector validation to connector numbers greater than zero.
- Added a generic authenticated `/v1/hal-facts` contract receiver integration
  test that drives real PostgreSQL, HAL HTTP, OCPP RemoteStart/RemoteStop,
  charger-originated start/meter/stop facts, reconciliation reads, and
  idempotency conflict handling. The shared-outbox test is run sequentially by
  `scripts/regression-local.ps1`, not concurrently with package-wide tests.

Compatibility: this intentionally changes this repository's runtime from the
inherited legacy CMS/frontend surface to v1 only. `OCPPHAL_Go` and
`ev-cms-backend-new` were not modified.

Verification: `gofmt`; focused HTTP/OCPP/store tests; `go test ./...`; `go vet
./...`; `scripts/build-all.ps1`; and
`scripts/regression-local.ps1 -SkipBuild` with a local PostgreSQL
`DATABASE_URL` all passed. The regression includes the real PostgreSQL/OCPP
fact-receiver lifecycle and fact-outbox tests. Real physical-charger coverage
for vendor-specific meter variants and energy/time stop races remains pending.

## 2026-08-10 - v1 fact durability and contract-conformance hardening

- Fixed v1 fact-outbox crash recovery: an expired `DELIVERING` lease is now
  reclaimable as the same durable fact without stealing an unexpired claim.
- Replaced the local map-key canonicalizer with RFC 8785 JCS and added a
  canonicalization vector covering nested values, Unicode, escaping, booleans,
  nulls, and numbers.
- Corrected v1 fact payload drift: `transaction.started` now emits
  `started_at`; `command.updated` includes nullable error evidence; and
  `transaction.completed` emits nullable stop-command correlation or the
  correlated CMS/HAL command IDs when present.
- Added PostgreSQL lease/concurrency tests and HTTP fact-worker response,
  transport-loss, immutable-redelivery, and retry classification tests.

Compatibility: additive hardening of the approved v1 outbox and fact contract.
No legacy route, CMS repository, or legacy repository was modified.

Verification: `go test ./internal/store ./internal/v1facts`; PostgreSQL v1
store, outbox lease/concurrency, and real virtual-OCPP start integration;
`go test ./...`; `go vet ./...`; and `scripts/build-all.ps1` passed. The full
manual-stop, energy-limit, time-limit/restart, natural-stop, and fact-receiver
lifecycle matrix remains in progress and is not claimed by this entry.

## 2026-08-10 - Go 1.23 hook-test verification compatibility

- Replaced the inherited Go-1.24-only `testing.T.Context()` calls in the
  hook callback test with a test-scoped cancellable standard-library context.
  The module remains at `go 1.23.0`.

Compatibility: test-only change; no runtime behavior, API, persistence, or
external contract changed. No CMS or legacy repository was modified.

Verification: ran `gofmt` on the changed Go test, `go test ./internal/hooks`,
`go test ./...`, and `go vet ./...`; all passed. The prior inherited
Go-1.24-only test incompatibility is no longer a verification blocker.

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
