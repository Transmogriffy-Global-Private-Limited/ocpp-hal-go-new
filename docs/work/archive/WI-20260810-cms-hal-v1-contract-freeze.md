# WI-20260810-cms-hal-v1-contract-freeze

Status: Completed
Owner: Codex
Collaborators: None
Started: 2026-08-10
Last updated: 2026-08-10

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` - New CMS/HAL Contract Design
Detailed-plan reference: `docs/contracts/CMS_HAL_CHARGING_V1.md`
Issue/PR reference: None

## Outcome

Promoted the human-approved v1 CMS/HAL charging architecture, including live
operational projection, into canonical project memory and defined the
authoritative human-readable service contract.

## Scope

- approved ownership, identifier, credential, command, fact, idempotency,
  durability, tariff/hold, settlement, recovery, CPO lifecycle, live
  connection/connector/meter projection, and first vertical-slice semantics;
- v1 HTTP endpoint and JSON fact/command/read-model shapes; and
- contract/project-memory reconciliation.

## Non-goals

- runtime Go, endpoint, migration, callback, configuration, deployment, or
  cross-repository implementation changes;
- changes to `ev-cms-backend-new` or legacy `OCPPHAL_Go`; and
- promotion of decisions outside the approved v1 contract.

## Claimed surfaces

- `docs/contracts/`, architecture/project-memory documentation, and work ledger.

## Dependencies and blockers

The v1 architecture is approved. Runtime implementation remains a separate
first vertical slice targeted for 2026-08-14. It still requires implementation
work for service authentication, overshoot/debt policy, energy guard, migrations,
and deployment design.

## Contract impact

Defined the human-approved v1 contract on paper, including live operational
facts and CMS polling projections. No route, payload, or service behavior is
implemented by this work item.

## Data and migration impact

None in this slice. The contract specifies future durable requirements; it does
not add a schema or migration.

## Current state

The contract is authoritative for v1 implementation planning. The existing
HAL/CMS runtime remains unchanged.

## Verification

- reviewed the complete documentation-only diff;
- ran `git diff --check`;
- confirmed only HAL documentation/work-ledger files changed;
- confirmed no runtime Go, migration, configuration, generated binary, secret,
  CMS, or legacy-repository file changed;
- no full regression was required or run because no runtime behavior changed.

## Handoff

Implement the first vertical slice from `docs/contracts/CMS_HAL_CHARGING_V1.md`
without re-opening approved decisions. Escalate only the explicitly open
decisions listed there.

## Completion

Completed as a documentation-only contract-freeze slice. No commit, push,
deployment, pull request, or remote change was made.
