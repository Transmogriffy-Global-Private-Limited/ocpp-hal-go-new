# Documentation Index

## Start Here

- [Repository overview](../README.md)
- [HAL and CMS architecture boundary](ARCHITECTURE_BOUNDARY.md)
- [Inherited HAL audit](INHERITED_HAL_AUDIT.md)
- [CMS/HAL charging integration v1 contract](contracts/CMS_HAL_CHARGING_V1.md)
- [Machine-readable v1 OpenAPI source](../internal/httpapi/v1_openapi.json)
- [v1 consumer-demand to HAL-capability matrix](plans/HAL_V1_CONSUMER_DEMAND_MATRIX.md)
- [CMS/HAL charging integration analysis (historical decision evidence)](plans/CMS_HAL_CHARGING_INTEGRATION_ANALYSIS.md)
- [Current project state](PROJECT_STATE.md)
- [Configuration semantics](CONFIGURATION.md)
- [Living development plan](DEVELOPMENT_PLAN.md)
- [Agent-assisted changelog](AI_CHANGELOG.md)

Read the architecture boundary before treating any copied behavior as a new-CMS contract. Read the inherited audit before removing or redesigning an existing subsystem, because it records the correctness and recovery property that the subsystem currently provides.

## Operational Documentation

- [Terminal-controlled OCPP 1.6J virtual charger](SOFTWARE_CHARGER.md)

Legacy local-regression, frontend-WebSocket, and frontend-transaction guides
were retired with their runtime surfaces. Use the v1 contract and
`scripts/regression-local.ps1` instead.

## Contract Documentation

The v1 human-readable service contract is
[CMS/HAL Charging Integration v1](contracts/CMS_HAL_CHARGING_V1.md). It is
approved architecture with the HAL-side lifecycle implemented. The
machine-readable source is `internal/httpapi/v1_openapi.json`;
when `API_DOCS_ENABLED=true`, it is served as `/openapi.json` and a loopback
interactive explorer is served at `/docs`. The contract covers mapping, start,
stop, exact reconciliation, live meter/runtime facts, and fact delivery. Do
not use inherited REST, callback, or WebSocket documentation as a substitute
for v1.

The [consumer-demand matrix](plans/HAL_V1_CONSUMER_DEMAND_MATRIX.md) is the
implementation prioritization source for the active first vertical slice. It
separates User App and CPO needs from HAL-owned sockets and plugs, and labels
must-ship, deferred, and inherited-legacy capabilities.
