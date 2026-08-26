# WI-20260826-wallet-derived-customer-limits

Status: In Progress
Owner: Codex
Collaborators: None
Started: 2026-08-26
Last updated: 2026-08-26

## Outcome

Accept the CMS's existing start command with optional energy and/or duration
limits, persist immutable customer intent separately from each threshold's
provenance, and retain truthful source when HAL's existing meter/deadline stop
workflows fire.

## Claimed surfaces

- `internal/httpapi/` v1 start contract
- `internal/store/` v1 command/transaction persistence and lifecycle
- `internal/ocpp16hal/` automatic stop classification
- `migrations/`, OpenAPI, focused tests, and HAL architecture/project memory

## Contract impact

`energy_limit_wh` and `max_duration_seconds` remain independently optional
non-negative fields. `limit_type` remains customer intent; `energy_limit_source`
and `duration_limit_source` explain the physical boundary. No new command,
worker, or delivery path is introduced.

## Verification

Focused store/http tests, then repository build/regression scripts where the
local environment permits.
