# WI-20260820-fact-delivery-fencing

Status: Completed
Owner: Codex
Collaborators: Anubhab Dey
Started: 2026-08-20
Last updated: 2026-08-20

Development-plan reference: `docs/DEVELOPMENT_PLAN.md`
Detailed-plan reference: `docs/ARCHITECTURE_BOUNDARY.md`
Issue/PR reference: None

## Outcome

Fence durable fact-delivery leases and make explicit configuration empties
invalid without changing OCPP or CMS truth ownership.

## Scope

- Add claim-token lease ownership to the fact outbox and worker.
- Preserve explicit empty environment values and bootstrap environment policy.
- Update tests, contracts, and project memory.

## Non-goals

- Database deployment, OCPP handler changes, physical charger operation, or
  changing CMS receiver semantics.

## Claimed surfaces

- `internal/store`, `internal/v1facts`, `internal/config`, migrations, tests,
  contracts, and project memory.

## Dependencies and blockers

- CMS requeue consumer work is tracked in its paired work item.
- PostgreSQL fencing tests require `TEST_DATABASE_URL`.

## Contract impact

- No wire fact-envelope change; lease ownership is internal durable delivery
  state. Existing fact requeue remains the explicit recovery socket.

## Data and migration impact

- Adds an additive nullable claim-token column migration.

## Current state

Implemented in source. Migration 010 remains unapplied because no database
mutation was authorized.

## Verification

Passed: focused config/store/fact-worker/HTTP tests, serial `go test -p 1
./...`, serial `go vet -p 1 ./...`, and `scripts/build-all.ps1`. The local
regression script correctly stopped before database work because `DATABASE_URL`
is unset. Claim-fencing tests skip without `TEST_DATABASE_URL`.

## Handoff

Do not claim dual-service acceptance without a configured disposable topology.

## Completion

Complete; archive after publication.
