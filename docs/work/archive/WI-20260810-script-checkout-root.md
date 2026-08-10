# WI-20260810-script-checkout-root

Status: Completed
Owner: Codex
Collaborators: None
Started: 2026-08-10
Last updated: 2026-08-10

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` - completed housekeeping
Detailed-plan reference: None
Issue/PR reference: None

## Outcome

Build and local regression automation now use the checkout containing the script
instead of the legacy repository's absolute path.

## Scope

- `scripts/build-all.ps1`;
- `scripts/regression-local.ps1`;
- affected project-memory and audit records.

## Non-goals

- changing formatter, test, build, mock, job, PostgreSQL, or regression steps;
- changing Go runtime behavior;
- modifying `OCPPHAL_Go` or `ev-cms-backend-new`.

## Claimed surfaces

- build/regression script entry paths and their documentation.

## Dependencies and blockers

No design dependency. Full regression requires the existing local PostgreSQL
setup and remains outside the script-root change itself.

## Contract impact

None.

## Data and migration impact

None.

## Current state

Both scripts resolve the parent of their own `scripts` directory before
executing their unchanged command sequence.

## Verification

- PowerShell parser validation passed for both scripts;
- calculated root for both scripts resolved to this checkout;
- the complete script diff contains only the root-resolution replacement;
- no `Set-Location` to `OCPPHAL_Go` remains in either script;
- `git diff --check` passed;
- full build/regression execution was not repeated because it remains subject
  to the known Go memory and local PostgreSQL limitations, not this path fix.

## Handoff

The scripts are now safe to invoke from another PowerShell working directory.
Future runtime work should run the existing full regression when the required
local PostgreSQL environment is available.

## Completion

Completed without changing test semantics, Go runtime behavior, external
services, or the legacy/new CMS repositories.
