# Development Plan

## Objective

Evolve this repository into the independently owned OCPP HAL for `ev-cms-backend-new`, preserving reliable charger-side OCPP behavior while establishing an explicit authenticated boundary to the CMS. The old `OCPPHAL_Go` and old-CMS relationship are not migration targets of this plan.

## Permanent Direction

- HAL owns charger/OCPP truth and command delivery; CMS owns identity, eligibility, tariffs, wallets, billing, and customer-facing projections.
- CMS and HAL have separate databases and cross an authenticated service boundary.
- Charger-originated StartTransaction/StopTransaction remain the source of OCPP start/completion truth; remote command acknowledgements do not replace those events.
- New public or service surfaces require an approved contract, authentication, authorization, durable identity/auditability, failure/retry/recovery design, and matching documentation.

## Current Execution

Current phase: architecture bootstrap + inherited-system audit.

Active feature: CMS/HAL charging integration analysis.

Current implementation slice: decision-ready CMS/HAL charging-integration analysis. No Go runtime behavior is changed.

Last completed slice: repository-purpose bootstrap, inherited-HAL inventory, and script checkout-root remediation.

Next expected slice: human review and approval of the recommended boundary decisions before any contract or runtime implementation.

Blocked by: no new CMS/HAL runtime contract has been approved.

## Feature Registry

### Architecture Bootstrap and Inherited-System Audit

Status: Implemented (documentation analysis only; no runtime contract approved)

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

Status: Analysis complete; human approval required before implementation

Phase: contract and integration design

Depends on:

- completion of the detailed inherited-HAL audit;
- relevant `ev-cms-backend-new` wallet, tariff, session, identity, and CPO requirements.

Objective: define the authenticated service boundary and the minimum coherent command, OCPP-truth, and recovery integration needed by the new CMS.

Non-goals:

- copying the legacy REST/callback/foreground WebSocket contracts by default;
- shared-database integration;
- prematurely selecting endpoint, event, or table schemas.

Acceptance criteria before implementation starts:

- ownership, identities, authorization, idempotency, duplicate/out-of-order handling, and recovery semantics are approved;
- authoritative contracts and migration/rollout approach are identified;
- all affected HAL and CMS consumers/producers are mapped.

## Next Approved Work

1. Obtain human approval for the recommended identity, hold/pricing, transport,
   mapping, recovery, and first-vertical-slice decisions in
   `docs/plans/CMS_HAL_CHARGING_INTEGRATION_ANALYSIS.md`.
2. Write the selected authoritative contract and rollout/reconciliation design.
3. Implement only the first approved vertical slice, including contract, authorization, durable state, recovery, verification, and documentation.

## Deferred Remediation

- Establish machine-readable contract and interactive documentation only after the first new service/API contract is approved; the inherited routes are not a suitable authoritative new-CMS specification.
- Plan any Go module/import identity change separately because it has broad repository impact.

## Completed Housekeeping

- 2026-08-10: build and regression scripts now derive the repository root from
  their script location, removing the legacy absolute-path defect without
  changing their test sequence.
