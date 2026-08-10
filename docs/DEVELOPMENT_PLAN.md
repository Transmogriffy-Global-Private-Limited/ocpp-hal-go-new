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

Completed slice: mapping enrollment, opaque bearer service authentication,
PostgreSQL commands/credentials/transactions, inherited OCPP RemoteStart and
Authorize wiring, StartTransaction materialization, runtime state/query
sockets, OpenAPI, and PostgreSQL/virtual-charger verification.

## Next Approved Work

1. Near-live MeterValues projection with integer Wh, observation time,
   monotonic sequence, and no interpolation.
2. `transaction.meter` durable fact plug and authenticated delivery/recovery.
3. Durable HAL-to-CMS fact delivery for command/start/runtime facts.
4. One unified durable stop coordinator.
5. User-request stop through the coordinator.
6. CPO-request stop through the coordinator.
7. Energy-limit stop through the coordinator.
8. Time-limit stop through the coordinator.
9. Charger-originated StopTransaction completion truth.
10. `transaction.completed` plug.
11. Restart/reconciliation torture tests for the complete lifecycle.

## Open Decisions

- Service-token rotation and production transport/deployment topology.
- Commercial overshoot/debt policy and charger-capability-supported stop guard.
- RFID/offline authorization, generalized realtime, and legacy retirement.

No item above authorizes a CMS business API, shared database, broker, or legacy
contract adoption.
