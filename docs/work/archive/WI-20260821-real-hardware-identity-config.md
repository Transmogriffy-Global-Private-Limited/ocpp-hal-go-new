# WI-20260821-real-hardware-identity-config

Status: Implemented
Owner: Codex
Collaborators: Anubhab Dey
Started: 2026-08-21
Last updated: 2026-08-21

Development-plan reference: `docs/DEVELOPMENT_PLAN.md`
Detailed-plan reference: `docs/contracts/CMS_HAL_CHARGING_V1.md`
Issue/PR reference: None

## Outcome

Admit mapped physical chargers through either supported identity URL form and
reconcile a bounded, generation-fenced operating profile after BootNotification.

## Scope

- Optional serial mapping evidence and strict WebSocket path parsing.
- Boot physical-evidence observation and current-generation configuration sync.
- Configuration defaults/profile, persistence, tests, contracts, and docs.

## Non-goals

- Legacy HAL edits, Caddy rewrite, unknown charger admission, live service/DB
  mutation, or a generic plugin framework.

## Claimed surfaces

- `internal/ocpp16hal`, `internal/store`, `internal/httpapi`, `internal/config`,
  migrations, docs/contracts, and tests.

## Dependencies and blockers

- CMS companion work adds optional serial to the mapping request.
- Disposable PostgreSQL and physical charger acceptance remain unrun.

## Contract impact

- Mapping gains optional expected serial. OCPP ingress accepts only
  `/{identity}` or `/{identity}/{serial}`.

## Data and migration impact

- Additive mapping/configuration evidence only; historical mappings remain
  valid with NULL serial.

## Current state

Phase-0 evidence confirmed a one-segment-only ingress assumption and no active
configuration reconciliation after BootNotification. Source now has strict
dual-form path admission, HAL-only Boot evidence, and generation-fenced
configuration reconciliation.

## Verification

Focused serial/admission/config tests, full Go tests/vet/build, docs and
residue scans passed. Database checks require `TEST_DATABASE_URL`.

## Handoff

Keep OCPP identity authoritative and tie configuration work to the current
connection generation.

## Completion

Source implementation is complete. Migration application and physical charger
acceptance remain external and unrun.
