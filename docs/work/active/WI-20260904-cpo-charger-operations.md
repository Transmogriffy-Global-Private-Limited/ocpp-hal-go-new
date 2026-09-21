# WI-20260904-cpo-charger-operations

Status: In Progress
Owner: Codex
Collaborators: Anubhab Dey (CMS/HAL boundary owner)
Started: 2026-09-04
Last updated: 2026-09-21 (source-only restart recovery hardening; not deployed)

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` — v1 consumer boundary
Detailed-plan reference: `docs/contracts/CMS_HAL_CHARGING_V1.md` (to be extended)
Issue/PR reference: None

## Outcome

Expose a typed, authenticated, durable HAL v1 charger-operation boundary for
the CMS CPO control vertical without altering charging Start/Stop semantics.

## Scope

- Dedicated durable operation identity/delivery state, exact lookup, mapping
  validation, typed OCPP dispatch, guarded configuration reads/mutations, and
  bounded TriggerMessage allowlist.
- Stable trace roots for durable operations plus narrow CALL/CALLRESULT/
  CALLERROR evidence tied by the library-generated OCPP unique ID.
- A separate 60-second accepted TriggerMessage follow-on diagnostic lane,
  matched only by identity/action/time/(when scoped) connector.

## Non-goals

- Legacy HAL, direct browser access, generic OCPP passthrough, replaying
  ambiguous physical commands, RemoteStart/RemoteStop, firmware, diagnostics,
  database application, deployment, or charger acceptance.

## Claimed surfaces

- `internal/httpapi`, `internal/ocpp16hal`, `internal/store`, migrations,
  v1 OpenAPI/contract docs, tests, and project memory.

## Dependencies and blockers

- CMS counterpart uses the existing authenticated v1 service boundary.
- Disposable PostgreSQL and a real mapped OCPP charge point are absent, so
  lifecycle and hardware verification cannot run.

## Contract impact

Adds typed v1 charger-operation endpoints. Durable HAL acceptance, OCPP
confirmation, and later charger evidence are deliberately distinct.

## Data and migration impact

Adds source-only forward migrations for `v1_charger_operations`, operation
trace-root evidence (`021`), and the persisted-operation recovery index (`023`);
none is applied.

## Current state

Implemented source: a dedicated v1 operation ledger/migration, exact lookup,
scoped mapping validation, typed OCPP dispatch, guarded configuration reads and
changes, and an allowlisted TriggerMessage. Existing remote-command records
remain Start/Stop-only.

Implemented but uncommitted/source-only: operation trace-root identities, a
unique-ID-only OCPP observer, strict action-specific durable evidence
sanitation, and audited GET_CONFIGURATION with an optional transient safe
configuration response. The legacy configuration-read endpoint remains
unmodified.

Implemented but uncommitted/source-only: PostgreSQL normalizes an omitted
operation `configuration_keys` slice to an explicit empty array before the
insert binds it. This prevents an explicit SQL `NULL` from violating the
`NOT NULL` column introduced by migration `021`; it does not alter the
database schema or any OCPP/CMS operation semantics.

Implemented but uncommitted/source-only: PostgreSQL operation readback now
uses pgx v5's database/sql-compatible array scanner for `configuration_keys`.
This preserves the durable non-null empty-array representation through POST
readback, duplicate lookup, and exact GET.

Implemented but uncommitted/source-only: HAL commits migration `022`'s indexed
durable diagnostic window atomically with persisted TriggerMessage `CALLRESULT`
`Accepted` evidence and its trace outbox record, before later operation
completion. Later matching traffic and the existing trace worker's closure
serialize only while a window is `OPEN`, committing one final `OBSERVED` or
`CLOSED` state. This remains non-causal diagnostic evidence; overlapping
windows are allowed and trace failure cannot alter operation, OCPP,
transaction, connector, fact, or worker behavior. Migration `022` is
unapplied.

Implemented source-only: startup turns only possibly sent
`DELIVERY_ATTEMPTED` rows into `RECONCILIATION_REQUIRED`. A mapped charger
connection performs a bounded, indexed scan of definitely unattempted
`PERSISTED` rows; a separate sequential recovery worker fairly revisits one
active charger per pass so batches drain and post-connect rows get a later
opportunity without delaying charging deadline or stop recovery. Offline rows
remain `PERSISTED`; recovery never blindly replays ambiguity. Final operation
persistence survives CMS request cancellation only within a bounded,
HAL-shutdown-cancelled context. A failed final write intentionally remains
ambiguous. Duplicate `PERSISTED` POSTs reuse the same state-gated claim;
attempted and terminal duplicate rows never resend. Migration `023` is
unapplied.

## Verification

Focused configuration-key normalization and CMS HAL reconciliation package
checks pass. The PostgreSQL persistence regression is covered but skipped
without `TEST_DATABASE_URL`; CMS delivery and hardware checks remain blocked
on an explicitly selected disposable environment.

Focused DB-free TriggerMessage follow-on store/OCPP/worker/sanitizer tests and
full Go test, vet, build, and diff checks pass. PostgreSQL, paired CMS, and
mapped-charger verification remain blocked on an explicit disposable
environment; no deployment/restart occurred.

Focused DB-free charger-operation recovery tests cover persisted restart,
racing claimants, attempted/terminal non-replay, mixed-batch isolation,
request-cancelled finalization, bounded-batch draining, offline-to-connected
recovery, cancellation, duplicate state gating, lifecycle isolation, sequential
worker execution, and lifecycle wiring. PostgreSQL lifecycle and hardware
checks remain blocked on a selected disposable environment.

## Handoff

Never redeliver an operation left ambiguous after physical dispatch. Exact CMS
operation-ID lookup is the only reconciliation path. The `PERSISTED` recovery
path is bounded per pass but eventually revisits active chargers; it is not an
offline poll or a replay path for ambiguous delivery.

## Completion

In progress.
