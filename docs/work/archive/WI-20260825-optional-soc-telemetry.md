# WI-20260825-optional-soc-telemetry

Status: Completed (source verified; database and hardware acceptance unrun)
Owner: Codex
Collaborators: Anubhab Dey (CMS/HAL boundary owner)
Started: 2026-08-25
Last updated: 2026-08-25

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` — v1 consumer-boundary verification
Detailed-plan reference: `docs/contracts/CMS_HAL_CHARGING_V1.md`
Issue/PR reference: None

## Outcome

Durably retain valid optional OCPP SoC observations independently from energy
and deliver immutable evidence to CMS without deriving missing telemetry.

## Scope

- MeterValues parsing, transaction telemetry persistence, migration, exact
  transaction response, immutable fact outbox, tests, contract, and simulator
  acceptance guidance.

## Non-goals

- Generic persistence of every OCPP measurand, commercial/billing policy,
  deployment, live database migration, or physical charger changes.

## Claimed surfaces

- `internal/ocpp16hal`, `internal/store`, `internal/httpapi`, migrations,
  v1 contract and project documentation. The CMS counterpart owns the
  customer-facing projection.

## Dependencies and blockers

- The existing v1 energy meter path and durable fact outbox. Disposable
  PostgreSQL plus CMS/cpconsole topology remain required for full acceptance.

## Contract impact

- Adds additive `transaction.soc` facts with an independent `soc_sequence`;
  energy-only `transaction.meter` facts remain unchanged.

## Data and migration impact

- Adds nullable v1 transaction SoC columns plus a non-null sequence counter.
  No backfill or runtime database action is authorized.

## Current state

- HAL parses independently selected valid energy and SoC observations, stores
  nullable first/latest SoC with its own timestamp and sequence, returns it in
  transaction state, and enqueues an additive immutable `transaction.soc`
  fact. Missing, invalid, stale, and equal-time evidence cannot fabricate or
  regress durable truth.

## Verification

- Passed: focused OCPP/store tests, `go test ./...`, `go vet ./...`, and
  `git diff --check`.
- Not run: migration on a disposable PostgreSQL database, cpconsole hardware
  acceptance, and cross-service CMS projection. No database or runtime was
  mutated.

## Handoff

- Keep SoC observational and independent: absent means unknown; it must never
  affect lifecycle, pricing, limits, wallet, or completion meter semantics.

## Completion

Source implementation complete. The separate CMS work item records the
counterpart durable projection and API verification.
