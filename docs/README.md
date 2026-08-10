# Documentation Index

## Start Here

- [Repository overview](../README.md)
- [HAL and CMS architecture boundary](ARCHITECTURE_BOUNDARY.md)
- [Inherited HAL audit](INHERITED_HAL_AUDIT.md)
- [CMS/HAL charging integration analysis (decision-ready, not approved)](plans/CMS_HAL_CHARGING_INTEGRATION_ANALYSIS.md)
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

No new `ev-cms-backend-new` service/API/event contract is approved yet. When a contract is approved, add its authoritative machine-readable schema and its human-readable integration guide here. Do not use inherited REST, callback, or WebSocket documentation as a substitute for that decision.
