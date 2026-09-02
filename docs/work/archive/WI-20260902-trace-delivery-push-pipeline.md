# WI-20260902-trace-delivery-push-pipeline

Status: Verified (source only)
Owner: Codex
Collaborators: None
Started: 2026-09-02
Last updated: 2026-09-02

Development-plan reference: `docs/DEVELOPMENT_PLAN.md`
Detailed-plan reference: Task 2 — isolated HAL to CMS diagnostic trace pipeline
Issue/PR reference: None

## Outcome

Delivered persisted diagnostic trace events to CMS through an independent durable
outbox and worker without sharing authoritative fact delivery machinery.

## Scope

- Trace outbox persistence/lease/retry worker, dedicated endpoint and bearer,
  configuration, retention, tests, and matching documentation.

## Non-goals

- Changes to `v1_fact_outbox`, `ClaimV1Facts`, authoritative facts, OCPP
  acknowledgement semantics, deployment, or database mutation.

## Claimed surfaces

- `internal/store/`, trace delivery worker/configuration/main wiring, migration
  019, HTTP-contract documentation, and tests.

## Dependencies and blockers

- The matching CMS trace-ingress envelope and dedicated bearer are implemented
  in the counterpart source worktree.
- PostgreSQL integration verification remains gated on `TEST_DATABASE_URL`,
  which was unavailable for this work.

## Contract impact

- HAL diagnostic trace events POST independently to CMS `/v1/hal-trace-events`.
- The obsolete private HAL trace GET is removed because no supported product
  consumer remains.

## Data and migration impact

- Additive diagnostic migration 019 creates `v1_trace_delivery_outbox`. It was
  not applied.

## Current state

- Source was recovered from HAL `88f10f4e` and the corresponding CMS baseline.
- Trace delivery is durable, independent from facts, evidence-only, and keeps
  failures observable without changing OCPP or business authority.

## Verification

- Focused trace/outbox/worker/store/OCPP/fact/migration tests, full Go test,
  vet, build, OpenAPI JSON parsing, and diff checks pass locally.
- PostgreSQL-gated cases were skipped because `TEST_DATABASE_URL` was unset.

## Handoff

- Preserve separate diagnostic delivery. Apply migration 019 and CMS 000060
  only together in an explicitly authorized coordinated rollout.

## Completion

- Source implementation and applicable local verification complete. No commit,
  push, deployment, database mutation, migration application, or service
  restart was performed.
