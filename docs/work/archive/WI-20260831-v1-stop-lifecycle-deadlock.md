# WI-20260831-v1-stop-lifecycle-deadlock

Status: Complete locally; publication authorized
Owner: Codex
Collaborators: Anubhab Dey
Started: 2026-08-31
Last updated: 2026-08-31

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` — v1 lifecycle durability
Detailed-plan reference: None
Issue/PR reference: None

## Outcome

Eliminate the proven PostgreSQL stop-lifecycle lock-order inversion so valid
charger-originated StopTransaction evidence is durably accepted even when a
RemoteStop confirmation is persisted concurrently.

## Scope

- `internal/store/` v1 transaction/workflow/command/fact ordering and bounded
  local PostgreSQL abort retry.
- Additive outbox uniqueness protection for one logical transaction completion.
- PostgreSQL-gated concurrency regression, source-level retry tests, focused
  lifecycle documentation and project memory.

## Non-goals

- Sending another RemoteStopTransaction, changing cpconsole timing, CMS changes,
  deployment, migration application, or manual repair of existing rows.

## Claimed surfaces

- `v1_transactions`, `v1_stop_workflows`, `v1_remote_commands`,
  `v1_fact_outbox`; their store transactions, migrations, tests, and lifecycle
  documentation.

## Dependencies and blockers

- PostgreSQL concurrency coverage is gated on an existing disposable
  `TEST_DATABASE_URL`; this work will not create or request one.

## Contract and data impact

- No CMS/HAL wire contract change. The additive source migration protects one
  `transaction.completed` outbox fact per HAL transaction and is not applied by
  this work.

## Current state

- The source now uses transaction -> workflow -> remote commands -> facts for
  all concurrent STOP lifecycle mutations. `MarkV1StopDelivery`, completion,
  stop-command creation, workflow creation, claim/begin, and energy-limit
  workflow creation retain that order. Recovery changes only workflow rows and
  therefore cannot form the former two-row cycle.
- Migration 017 backfills a terminal-completion key ledger from existing facts
  without deleting historical outbox rows. New completion writes reserve the
  key and create the outbox fact atomically.

## Verification

- `go test -count=1 ./internal/store ./internal/ocpp16hal ./cmd/cpconsole`,
  `go test ./...`, `go vet ./...`, `go build ./...`, and `git diff --check`
  pass locally.
- The focused PostgreSQL race regression is present and reports `SKIP` without
  `TEST_DATABASE_URL`; no disposable database was provisioned or modified.

## Handoff

- Preserve the distinction between RemoteStop acceptance and completion. Retry
  only locally aborted PostgreSQL transactions; never use retry to resend an
  OCPP remote command.

## Completion

- Complete locally. Archive after publication with the resulting source
  checkpoint; no migration, deployment, service restart, or manual data repair
  occurred.
