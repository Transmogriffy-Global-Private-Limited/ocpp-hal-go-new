# WI-20260901-charging-transaction-trace

Status: Verified
Owner: Codex
Collaborators: None
Started: 2026-09-01
Last updated: 2026-09-01 (implementation and local verification complete)

Development-plan reference: HAL v1 transaction lifecycle and CMS fact delivery
Detailed-plan reference: User-approved first-class charging transaction trace / waterfall specification
Issue/PR reference: None

## Outcome

Provide durable, connector-aware HAL diagnostic evidence for the OCPP path of
a charging transaction, safely correlated with CMS trace IDs and available to
CMS through a private authenticated read contract.

## Scope

- Additive HAL trace schema/store, sanitizer, lifecycle/OCPP evidence,
  connector-aware correlation, private read API, retention policy, and tests.
- Preserve fact-outbox semantics and the existing StopTransaction durability
  behavior.

## Non-goals

- Replacing transaction, connector, or OCPP truth with trace state.
- Changing CMS-owned commercial decisions; applying migrations, deployment, or
  publishing changes.

## Claimed surfaces

- `internal/store/`, `internal/ocpp16hal/`, `internal/httpapi/`, migrations,
  OCPP lifecycle tests, and HAL API documentation.

## Dependencies and blockers

- CMS propagates a pre-generated trace ID for CMS-originated starts. HAL binds
  that root when `StartTransaction` supplies the OCPP transaction identity and
  creates a HAL-owned root only where no CMS root exists. No external blocker
  known.

## Contract impact

- Adds a private CMS-authenticated trace read surface. Trace events remain
  diagnostic only and are not emitted as CMS business facts.

## Data and migration impact

- Additive HAL migration only; it must not be applied during this work.

## Current state

- Baseline: HAL `main` at `daaa63abffcf6b2e4fcea642eea74be0d5b77339`.
- Additive migration 018, durable root/event storage, storage-boundary
  sanitization, cursor reads, private CMS bearer API, RemoteStart/
  StartTransaction/MeterValues/RemoteStop/StopTransaction/StatusNotification
  evidence, failure evidence, connector-aware bounded post-stop association,
  and configurable bounded retention are implemented in the current dirty
  worktree.
- The existing StopTransaction deadlock behavior retains its CALLERROR
  semantics. Final cross-repository verification and documentation
  reconciliation remain.

## Verification

- Focused trace/OCPP tests plus prescribed build and regression checks. Any
  PostgreSQL integration test remains skipped without `TEST_DATABASE_URL`.

## Handoff

- Continue only from this paired CMS/HAL trace boundary; do not repurpose the
  existing CMS fact outbox for diagnostic trace events.

## Completion

- Implementation and local verification are complete. Publication is
  authorized; migration application, deployment, and database mutation remain
  out of scope.
