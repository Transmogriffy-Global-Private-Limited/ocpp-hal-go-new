# Work Coordination

`docs/work/active/` is the canonical ledger for substantial active work. It is
for ownership, overlap, blockers, and handoff; it is not a second development
plan, project-state file, or changelog.

Before materially changing a shared surface, inspect active work items. Create
one `WI-YYYYMMDD-short-slug.md` for a substantial slice, then move it to
`docs/work/archive/` after completion.

## Work Item Template

```markdown
# WI-YYYYMMDD-short-name

Status: In Progress | Blocked | Completed
Owner:
Collaborators: None
Started:
Last updated:

Development-plan reference:
Detailed-plan reference: None
Issue/PR reference: None

## Outcome

## Scope

## Non-goals

## Claimed surfaces

## Dependencies and blockers

## Contract impact

## Data and migration impact

## Current state

## Verification

## Handoff

## Completion
```

Keep claims narrow. If work overlaps, record the dependency or collaboration in
both records and use the approved contract/boundary rather than competing
implementations. A blocker must state the exact missing decision or evidence.
The handoff must let another engineer continue from repository state alone.
