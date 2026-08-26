# WI-20260826-wallet-derived-customer-limits

Status: In Progress
Owner: Codex
Collaborators: None
Started: 2026-08-26
Last updated: 2026-08-26

## Outcome

Accept the CMS's existing start command with optional energy and/or duration
limits, persist the requested limit classification, and retain that
classification when HAL's existing meter/deadline stop workflows fire.

## Claimed surfaces

- `internal/httpapi/` v1 start contract
- `internal/store/` v1 command/transaction persistence and lifecycle
- `internal/ocpp16hal/` automatic stop classification
- `migrations/`, OpenAPI, focused tests, and HAL architecture/project memory

## Contract impact

`energy_limit_wh` and `max_duration_seconds` become independently optional
non-negative fields. `limit_type` is `AUTO`, `ENERGY`, `TIME`, or `MONEY`.
No new command, worker, or delivery path is introduced.

## Verification

Focused store/http tests, then repository build/regression scripts where the
local environment permits.
