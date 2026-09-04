# WI-20260904-cpo-charger-operations

Status: In Progress
Owner: Codex
Collaborators: Anubhab Dey (CMS/HAL boundary owner)
Started: 2026-09-04
Last updated: 2026-09-04 (source implementation complete; environment validation pending)

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

Adds a source-only forward migration for `v1_charger_operations`.

## Current state

Implemented source: a dedicated v1 operation ledger/migration, exact lookup,
scoped mapping validation, typed OCPP dispatch, guarded configuration reads and
changes, and an allowlisted TriggerMessage. Existing remote-command records
remain Start/Stop-only.

## Verification

Memory-store idempotency/one-time-claim tests and focused v1 HTTP checks pass;
repository vet and diff checks pass. PostgreSQL and hardware checks remain
blocked on an explicitly selected disposable environment.

## Handoff

Never redeliver an operation left ambiguous after physical dispatch. Exact CMS
operation-ID lookup is the only reconciliation path.

## Completion

In progress.
