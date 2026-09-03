# WI-20260901-charging-transaction-trace

Status: In Progress
Owner: Codex
Collaborators: None
Started: 2026-09-01
Last updated: 2026-09-03 (trace completeness/correctness source slice in progress)

Development-plan reference: HAL v1 transaction lifecycle and CMS fact delivery
Detailed-plan reference: User-approved first-class charging transaction trace / waterfall specification
Issue/PR reference: None

## Outcome

Provide durable, connector-aware HAL diagnostic evidence for the OCPP path of
a charging transaction, safely correlated with CMS trace IDs and delivered to
CMS through the isolated diagnostic outbox.

## Scope

- Additive HAL trace schema/store, sanitizer, lifecycle/OCPP evidence,
  connector-aware correlation, isolated delivery, retention policy, and tests.
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

- Uses the dedicated CMS-authenticated trace-event ingress. Trace events remain
  diagnostic only and are not emitted as CMS business facts.

## Data and migration impact

- Additive HAL migration only; it must not be applied during this work.

## Current state

- Baseline: HAL `main` at `b853d4e`.
- Additive migration 018, durable root/event storage, storage-boundary
  sanitization, cursor reads, private CMS bearer API, RemoteStart/
  StartTransaction/MeterValues/RemoteStop/StopTransaction/StatusNotification
  evidence, failure evidence, connector-aware bounded post-stop association,
  and configurable bounded retention are implemented in the current dirty
  worktree.
- The existing StopTransaction deadlock behavior retains its CALLERROR
  semantics. The current source slice corrects unbound status phase to
  `STARTING`, distinguishes OCPP wire request/confirmation arrows, adds
  credential-free correlated Authorize and automatic-stop evidence, and emits
  HAL-to-CMS only after fact acknowledgement. Final paired verification and
  documentation reconciliation remain.

## Verification

- Focused trace/OCPP tests plus prescribed build and regression checks. Any
  PostgreSQL integration test remains skipped without `TEST_DATABASE_URL`.

## Handoff

- Continue only from this paired CMS/HAL trace boundary; do not repurpose the
  existing CMS fact outbox for diagnostic trace events.

## Completion

- The prior trace implementation remains verified. This focused completeness
  slice remains uncommitted and unpublished; migration application,
  deployment, and database mutation remain out of scope.
