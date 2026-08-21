# WI-20260821-meter-quantization-evidence

Status: Completed
Owner: Codex
Collaborators: Anubhab Dey
Started: 2026-08-21
Last updated: 2026-08-21

Development-plan reference: `docs/DEVELOPMENT_PLAN.md`
Detailed-plan reference: `docs/contracts/CMS_HAL_CHARGING_V1.md`
Issue/PR reference: None

## Outcome

Preserve raw charger meter evidence while normalizing only a coherent one-Wh
StopTransaction quantization discrepancy into a nondecreasing authoritative
transaction projection.

## Scope

- v1 transaction schema/store, OCPP meter/stop handling, completion facts,
  cpconsole conversion, tests, and documentation.

## Non-goals

- CMS changes, arbitrary tolerance configuration, database deployment, or
  broad transaction-lifecycle redesign.

## Claimed surfaces

- `internal/store`, `internal/ocpp16hal`, `cmd/cpconsole`, migration 011,
  contracts, tests, and project memory.

## Dependencies and blockers

- CMS receiver compatibility must remain additive; disposable PostgreSQL is
  needed for lifecycle confirmation.

## Contract impact

- Completion facts retain `meter_stop_wh` as authoritative effective meter and
  add optional raw/adjustment/evidence metadata for new facts.

## Data and migration impact

- Additive migration only; historical raw stop evidence remains NULL.

## Current state

Implemented raw/effective completion meter evidence, a fixed one-Wh classifier,
periodic anomaly counting, a single cpconsole OCPP conversion, and additive
completion-fact metadata. CMS remains unchanged because it accepts additive
facts and continues to bill from `meter_stop_wh`.

## Verification

Focused store/OCPP/simulator tests, full Go tests/vet/race, and repository
build passed. PostgreSQL lifecycle tests correctly require `TEST_DATABASE_URL`
and were not run without a clearly disposable database.

## Handoff

Do not accept arbitrary meter rollbacks or claim real-charger acceptance
without a deliberate disposable topology.

## Completion

Complete. Archive this work item after publication.
