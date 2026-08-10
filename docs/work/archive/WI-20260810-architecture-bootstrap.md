# WI-20260810-architecture-bootstrap

Status: Completed
Owner: Codex
Collaborators: None
Started: 2026-08-10
Last updated: 2026-08-10

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` - Architecture Bootstrap and Inherited-System Audit
Detailed-plan reference: None
Issue/PR reference: None

## Outcome

Correct inherited repository purpose and establish a durable HAL/CMS boundary,
an inherited-system audit, canonical documentation navigation, and a future
work-coordination protocol.

## Scope

- root operating contract and repository overview;
- architecture boundary and inherited HAL audit;
- project plan, state, changelog, documentation index, and work protocol.

## Non-goals

- Go runtime, database schema, routes, callback payloads, deployment, or new CMS/HAL contracts;
- modifications to `OCPPHAL_Go` or `ev-cms-backend-new`.

## Claimed surfaces

- project documentation and root `AGENTS.md` only.

## Dependencies and blockers

The next implementation-design slice is blocked on the jointly reviewed
`ev-cms-backend-new` wallet, tariff, charging-session, identity, and CPO
requirements. No new CMS/HAL runtime contract is approved.

## Contract impact

No runtime contract changed. The record explicitly classifies copied legacy
routes, callbacks, WebSockets, schema, and environment behavior as inherited,
not approved new-CMS contracts.

## Data and migration impact

None.

## Current state

The repository now describes itself as the independent OCPP HAL for
`ev-cms-backend-new`. The inherited build/regression scripts have a hard-coded
legacy repository path and are documented as unsafe to run until repaired.

## Verification

- inspected representative OCPP handlers, connection generation guard,
  persistence, MeterValues, limit stops, callback/outbox processing, boot
  recovery, route wiring, directory cache, frontend WebSockets, config,
  migrations, tooling, and call sites;
- `go test ./...` was terminated after 124 seconds with no diagnostic output;
- `go build ./...` failed while linking `cmd/cpconsole` because the Go runtime
  could not allocate memory;
- reviewed complete documentation diff and ran `git diff --check`;
- did not run inherited build/regression scripts because they would
  `Set-Location` into `OCPPHAL_Go`, outside the approved repository scope.

## Handoff

Start with `docs/ARCHITECTURE_BOUNDARY.md` and
`docs/INHERITED_HAL_AUDIT.md`. Pair the next audit with the relevant
`ev-cms-backend-new` business requirements before proposing a service contract.

## Completion

Documentation-only bootstrap complete. No commit, push, deployment, or remote
change was made.
