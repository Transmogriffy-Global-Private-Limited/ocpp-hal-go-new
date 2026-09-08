# WI-20260904-cpo-charger-operations

Status: In Progress
Owner: Codex
Collaborators: Anubhab Dey (CMS/HAL boundary owner)
Started: 2026-09-04
Last updated: 2026-09-08 (per-operation protocol-evidence source slice implemented; verification and source review in progress; uncommitted and not deployed)

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

Adds source-only forward migrations for `v1_charger_operations` and operation
trace-root evidence (`021`); neither is applied.

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

## Verification

Focused trace-sanitizer tests plus OCPP/store/v1 HTTP compile checks pass.
PostgreSQL, CMS delivery, and hardware checks remain blocked on an explicitly
selected disposable environment.

## Handoff

Never redeliver an operation left ambiguous after physical dispatch. Exact CMS
operation-ID lookup is the only reconciliation path.

## Completion

In progress.
