# WI-20260831-cpconsole-multi-connector

Status: Complete
Owner: Codex
Collaborators: None
Started: 2026-08-31
Last updated: 2026-08-31

Development-plan reference: `docs/DEVELOPMENT_PLAN.md`
Detailed-plan reference: None
Issue/PR reference: None

## Outcome

Make cpconsole one OCPP charge point/WebSocket with independently modeled
connectors, rather than requiring a second process for a second connector.

## Scope

- `cmd/cpconsole/` state machine, console, remote command routing, tests.
- `docs/SOFTWARE_CHARGER.md` and project-memory status for the simulator-only
  behavior.

## Non-goals

- HAL server behavior, CMS, durable schema/migrations, deployment, or a second
  WebSocket for the same charge point identity.

## Claimed surfaces

- `cmd/cpconsole/`
- `docs/SOFTWARE_CHARGER.md`
- simulator project-memory entries

## Current state

- Map-backed per-connector transaction/status/meter/automatic-worker state is
  implemented. One BootNotification and one status notification per configured
  connector occur on startup. Remote start routes explicitly or to the lowest
  eligible connector; remote stop resolves its exact transaction owner.

## Verification

- Passed: `go test ./cmd/cpconsole -count=1`,
  `go test -race ./cmd/cpconsole -count=1`, `go test ./...`, `go vet ./...`,
  `go build ./...`, and `scripts/build-all.ps1`.
- `scripts/regression-local.ps1 -SkipBuild` correctly stopped before its
  PostgreSQL suite because `TEST_DATABASE_URL` was not supplied. No database
  was selected or modified.
- Hosted simulator verification connected as `8d2bd0`, booted once, and emitted
  `Available` for connectors 1 and 2. No transaction was started.

## Handoff

No migration, deployment, commit, or push is authorized.

## Completion

Complete and archived after the approved publication reconciliation.
