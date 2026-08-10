# Project Operating Contract

## Project Identity

`ocpp-hal-go-new` is the independently evolving OCPP HAL for `ev-cms-backend-new`. It is not the legacy `OCPPHAL_Go` service and it does not own or preserve the legacy CMS-to-HAL integration by default.

The HAL owns OCPP transport and protocol behavior, charger connections and reconnections, charger-originated transaction truth, raw/live meter handling, and charger-command delivery. The CMS owns customer and CPO identity, business eligibility and authorization, tariffs, wallet/payment/billing, and customer-facing charging-session and business projections.

The old repository and its old CMS integration are outside this repository's scope. Do not edit them while working here.

## Required Reading

Before meaningful planning or implementation, read:

- `README.md`
- `docs/README.md`
- `docs/ARCHITECTURE_BOUNDARY.md`
- `docs/INHERITED_HAL_AUDIT.md`
- `docs/DEVELOPMENT_PLAN.md`
- `docs/PROJECT_STATE.md`
- `docs/AI_CHANGELOG.md`
- the focused code, tests, configuration, migrations, and integration documents for the surface being changed

Treat the architecture boundary document as the source for decided ownership and unresolved design questions. Treat the inherited audit as evidence about the current code, not as approval to preserve legacy behavior.

## Engineering Rules

1. Use `github.com/lorenzodonini/ocpp-go` for OCPP protocol framing and normal OCPP 1.6 handling. Do not manually recreate the protocol layer.
2. Preserve the decided HAL/CMS ownership boundary. CMS and HAL must not share a database; integration crosses an explicit authenticated service boundary.
3. A `RemoteStartTransaction` acknowledgement is command delivery evidence, not charging-start evidence. Only charger-originated `StartTransaction` creates normal OCPP start truth. The equivalent rule applies to `RemoteStopTransaction` and charger-originated `StopTransaction`.
4. Do not treat inherited REST routes, callback payloads, WebSocket payloads, environment names, single-session behavior, or database fields as approved new-CMS contracts. First identify the correctness or recovery property they provide, then obtain or record the replacement contract.
5. Fail safely at boundaries. Do not infer authority from malformed, inconsistent, unauthorized, duplicate, stale, out-of-order, or ambiguous state. Preserve durable identity and auditability.
6. Keep the design boring and explicit: one durable source of truth per fact, no speculative endpoints or transport abstractions, and no duplicate orchestration paths without a documented reason.
7. Preserve unrelated work. Do not commit generated `builds/` artifacts, local review workspaces, `.env`, or real secrets.

## Change Workflow

Before editing, inspect the worktree, applicable instructions, relevant call sites, route wiring, configuration, tests, migrations, and current project memory. Map the affected causal chain: input, trusted scope, domain decision, durable state, downstream command or integration, recovery, and verification.

For a contract or behavior change, update all relevant producers, consumers, fixtures, tests, scripts, and documentation in the same slice. Update `docs/ARCHITECTURE_BOUNDARY.md` when a decided boundary changes, `docs/INHERITED_HAL_AUDIT.md` when inherited behavior is newly evidenced or reclassified, `docs/DEVELOPMENT_PLAN.md` for approved sequencing and status, `docs/PROJECT_STATE.md` for verified present reality, and `docs/AI_CHANGELOG.md` for meaningful completed work.

No new CMS/HAL runtime contract may be presented as approved until it has been explicitly decided and recorded in the architecture boundary and appropriate machine-readable/human-readable contract documentation.

## Verification

Use the smallest focused check first, then the broadest appropriate check. For runtime changes, the intended repository checks are:

```powershell
.\scripts\build-all.ps1
.\scripts\regression-local.ps1 -SkipBuild
```

The inherited scripts currently contain an absolute legacy-repository path. Do not run them if that would operate on `OCPPHAL_Go`; record the exact limitation and use safe local checks that operate in this checkout. Do not claim database, charger, CMS, or end-to-end verification without executing it against the correct services.

Before completion, inspect the complete diff, run `git diff --check`, confirm `git status --short`, and verify no secrets, generated binaries, or unrelated artifacts were introduced.

## Documentation Ownership

- `docs/ARCHITECTURE_BOUNDARY.md`: permanent decisions, inherited facts, and unresolved architecture questions.
- `docs/INHERITED_HAL_AUDIT.md`: evidence-based KEEP/MODIFY/REPLACE/REMOVE/INVESTIGATE inventory and preservation properties.
- `docs/DEVELOPMENT_PLAN.md`: approved work, dependencies, status, and next work.
- `docs/PROJECT_STATE.md`: verified current system reality and known gaps.
- `docs/AI_CHANGELOG.md`: meaningful completed changes and verification facts.
- `docs/README.md`: documentation navigation.

## Coordination

For a substantial active slice, inspect `docs/work/active/` before changing a
shared surface. Create one `WI-YYYYMMDD-short-slug.md` record with status,
owner, scope, claimed surfaces, contract impact, dependencies, verification,
and handoff state. Move it to `docs/work/archive/` when complete. The active
directory is the coordination ledger; do not create a competing status table.

Use `docs/work/README.md` for the record template and protocol. Keep this
lightweight: tiny isolated corrections do not need ceremonial coordination.

## Git and Operational Safety

Never commit, stage, push, merge, rebase, deploy, modify a remote service, or make destructive database changes without explicit human permission. Keep local development services loopback-bound. Do not modify `OCPPHAL_Go` or `ev-cms-backend-new` as part of work in this repository unless the human explicitly expands the scope.
