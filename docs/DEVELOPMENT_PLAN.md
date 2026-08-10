# Development Plan

## Objective

Deliver an independent, recoverable OCPP HAL integration for
`ev-cms-backend-new` by 2026-08-14 without sharing a database or preserving
legacy CMS behavior as the new contract.

## Permanent Direction

- HAL owns OCPP and charger truth; CMS owns business/customer/financial truth.
- Remote command acknowledgement is not transaction truth; charger-originated
  StartTransaction and StopTransaction are.
- PostgreSQL is production durable truth. CMS/HAL integration is authenticated
  HTTP with exact identities and reconciliation queries.

## Current Execution

Current phase: first HAL-side v1 vertical.

Completed implementation: mapping enrollment, opaque bearer service
authentication, PostgreSQL commands/credentials/transactions/stop workflows,
OCPP RemoteStart/Authorize/StartTransaction/MeterValues/StopTransaction
wiring, runtime and reconciliation sockets, immutable fact outbox/delivery,
RFC 8785 canonical fact digests, expired fact-lease recovery, and OpenAPI.

## Next Approved Work

1. Complete real PostgreSQL/OCPP/fact-receiver lifecycle and remaining
   crash/recovery torture coverage for the HAL vertical, including manual,
   energy-limit, time-limit/restart, and natural-stop completion scenarios.
2. Produce the CMS integration handoff with the verified contract behavior and
   receiver expectations.
3. Implement CMS-owned durable operational/session projections, financial
   flow, and customer/CPO surfaces in `ev-cms-backend-new`.

## Open Decisions

- Service-token rotation and production transport/deployment topology.
- Commercial overshoot/debt policy and any future charger-capability-supported
  predictive stop guard.
- RFID/offline authorization, generalized realtime, and legacy retirement.

No item above authorizes a CMS business API, shared database, broker, or legacy
contract adoption.
