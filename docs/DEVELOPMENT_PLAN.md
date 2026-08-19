# Development Plan

## Objective

Deliver one independently deployable OCPP HAL for `ev-cms-backend-new`, with
no shared database and no supported old-CMS compatibility runtime.

## Current Execution

Current phase: v1 consumer-boundary verification and legacy retirement.

Completed in this slice:

- v1-only process startup, mapping-based charger admission, authenticated HTTP
  command/query boundary, durable OCPP truth, facts, and reconciliation;
- generic contract-receiver test covering real PostgreSQL, HAL HTTP, OCPP
  remote commands, StartTransaction, MeterValues, StopTransaction, and fact
  delivery;
- retirement of legacy `/api/*`, callback/max-kWh, directory, frontend
  WebSocket, single-session, automatic offline-auth policy, and legacy smoke
  runtime; and
- Go module identity correction.
- Current connection-liveness slice: accepted current-connection Heartbeats
  renew durable `ONLINE` evidence and ordered facts without changing generation;
  the requested OCPP cadence is configurable with a five-minute default.
- Software-charger automation slice: `cpconsole` now models Boot-driven
  Heartbeats and an optional one-shot normal charging session without creating
  a second protocol or state-machine path.
- Current cross-service hardening: exact transaction lookup responses have
  stable snake_case fields for CMS recovery, while fact delivery exposes only
  bounded receiver classifications and retains the same immutable retry body.

## Next Approved Work

1. Expand real-device acceptance coverage for energy/time stop races and
   charger-specific MeterValues variants before production rollout.
2. Implement CMS-owned consumer projections, commercial flow, and UI in
   `ev-cms-backend-new` using the frozen v1 contract.

## Open Decisions

- token rotation and production transport topology;
- RFID/offline authorization and future OCPP control commands;
- commercial overshoot/debt policy; and
- whether historical legacy database tables warrant a separate destructive
  retirement migration.
