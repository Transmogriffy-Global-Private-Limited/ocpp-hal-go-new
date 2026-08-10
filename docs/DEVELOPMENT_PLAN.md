# Development Plan

## Objective

Evolve this repository into the independently owned OCPP HAL for `ev-cms-backend-new`, preserving reliable charger-side OCPP behavior while establishing an explicit authenticated boundary to the CMS. The old `OCPPHAL_Go` and old-CMS relationship are not migration targets of this plan.

## Permanent Direction

- HAL owns charger/OCPP truth and command delivery; CMS owns identity, eligibility, tariffs, wallets, billing, and customer-facing projections.
- CMS and HAL have separate databases and cross an authenticated service boundary.
- Charger-originated StartTransaction/StopTransaction remain the source of OCPP start/completion truth; remote command acknowledgements do not replace those events.
- New public or service surfaces require an approved contract, authentication, authorization, durable identity/auditability, failure/retry/recovery design, and matching documentation.

## Current Execution

Current phase: v1 first-vertical-slice implementation.

Required implementation-complete target: 2026-08-14. The first vertical slice
must favor direct, boring, recoverable mechanisms and avoid infrastructure that
does not serve its complete operational path.

Active feature: approved v1 CMS/HAL charging contract.

Current implementation slice: consumer-demand matrix, production durability
guard, additive v1 schema foundation, and focused in-memory command/credential/
transaction state verification.

Last completed slice: decision-ready joint CMS/HAL analysis.

Next expected slice: implement PostgreSQL v1-store operations and wire durable
commands, credentials, live state, meter/stop coordination, fact delivery, and
authenticated service routes into the existing OCPP handler path.

Blocked by: exact service-auth mechanism remains deliberately unresolved;
PostgreSQL v1-store operations, OCPP wiring, and service-route implementation
are not yet complete. Overshoot/debt policy and energy guard formula remain CMS
or capability-evidence decisions.

## Feature Registry

### Architecture Bootstrap and Inherited-System Audit

Status: Verified documentation foundation; v1 contract approved later

Phase: architecture bootstrap + inherited-system audit

Objective: establish accurate ownership, project rules, inherited behavior evidence, and deliberate open decisions before changing runtime integration.

Scope:

- repository operating contract;
- permanent HAL/CMS boundary;
- KEEP/MODIFY/REPLACE/REMOVE/INVESTIGATE audit;
- present-state, README, and changelog correction;
- safe verification of the documentation-only change.

Non-goals:

- changing Go runtime behavior;
- approving a new API, event, callback, schema, auth mechanism, or migration;
- modifying `OCPPHAL_Go` or `ev-cms-backend-new`.

Acceptance criteria:

- future agents can locate authoritative project memory and distinguish permanent decisions from inherited facts and open architecture questions;
- inherited components and their correctness/recovery properties are inventoried;
- documentation does not represent legacy compatibility as the new project's purpose or as an approved new-CMS contract;
- the source worktree remains runtime-code unchanged.

Verification:

- complete documentation diff review;
- `git diff --check`;
- the build/regression scripts resolve their own checkout root and preserve their existing command sequence;
- residue scan for legacy-purpose claims in changed project-memory files.
- decision-ready CMS/HAL analysis paired with read-only CMS wallet, tariff,
  session, identity, CPO-lifecycle, and User App evidence.

### New CMS/HAL Contract Design

Status: In Progress; v1 foundation added, service path not implemented

Phase: contract and integration design

Depends on:

- completion of the detailed inherited-HAL audit;
- relevant `ev-cms-backend-new` wallet, tariff, session, identity, and CPO requirements.

Objective: define the authenticated service boundary and the minimum coherent command, OCPP-truth, and recovery integration needed by the new CMS.

Non-goals:

- copying the legacy REST/callback/foreground WebSocket contracts by default;
- shared-database integration;
- changing the legacy compatibility surface during v1 implementation.

Acceptance criteria before implementation starts:

- approved ownership, identities, command/fact idempotency, duplicate/out-of-order,
  recovery, tariff/hold, settlement, energy-limit, CPO lifecycle, live
  connection/connector/meter projection, and customer state semantics are
  recorded in `docs/contracts/CMS_HAL_CHARGING_V1.md`;
- v1 HTTP paths, payload vocabulary, fact identity, reconciliation reads, and
  first-vertical-slice acceptance criteria are authoritative;
- additive v1 data/configuration foundations preserve legacy runtime behavior;
- v1 service routes, OCPP integration, fact delivery, and deployment remain to
  be implemented and verified.

## Next Approved Work

1. Design and implement the approved first vertical slice from
   `docs/contracts/CMS_HAL_CHARGING_V1.md`, beginning with durable HAL command
   and fact/outbox state plus the mutually authenticated service boundary.
2. Add the matching CMS durable intent/hold/credential/session/fact-receipt
   implementation in its owning repository without sharing databases.
3. Verify the complete failure/recovery and live operational projection matrix
   before exposing User App charging operations.

## Deferred Remediation

- Add an implementation-aligned machine-readable contract and interactive
  documentation with the first routed v1 service implementation; inherited
  routes are not a suitable authoritative specification.
- Plan any Go module/import identity change separately because it has broad repository impact.

## Completed Housekeeping

- 2026-08-10: build and regression scripts now derive the repository root from
  their script location, removing the legacy absolute-path defect without
  changing their test sequence.
