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
- [Living development plan](DEVELOPMENT_PLAN.md)
- [Agent-assisted changelog](AI_CHANGELOG.md)

Read the architecture boundary before treating any copied behavior as a new-CMS contract. Read the inherited audit before removing or redesigning an existing subsystem, because it records the correctness and recovery property that the subsystem currently provides.

## Inherited Operational and Test Documentation

The following documents describe copied implementation behavior and tooling. They are useful audit evidence, but do not approve legacy CMS/frontend payloads or routes as the new HAL contract.

- [Local regression](LOCAL_REGRESSION.md)
- [Terminal-controlled OCPP 1.6J virtual charger](SOFTWARE_CHARGER.md)
- [Inherited frontend transaction WebSocket](FRONTEND_TRANSACTION_WEBSOCKET.md)
- [Inherited user frontend live transaction integration](USER_FRONTEND_LIVE_TRANSACTION_INTEGRATION.md)

## Contract Documentation

The v1 human-readable service contract is
[CMS/HAL Charging Integration v1](contracts/CMS_HAL_CHARGING_V1.md). It is
approved architecture with a partially implemented HAL-side start/runtime
vertical. The machine-readable source is `internal/httpapi/v1_openapi.json`;
when `API_DOCS_ENABLED=true`, it is served as `/openapi.json` and a loopback
interactive explorer is served at `/docs`. Meter/fact/stop paths remain target
contract work. Do not use inherited REST, callback, or WebSocket documentation
as a substitute for v1.

The [consumer-demand matrix](plans/HAL_V1_CONSUMER_DEMAND_MATRIX.md) is the
implementation prioritization source for the active first vertical slice. It
separates User App and CPO needs from HAL-owned sockets and plugs, and labels
must-ship, deferred, and inherited-legacy capabilities.
