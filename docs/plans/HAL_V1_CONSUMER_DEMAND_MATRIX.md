# HAL v1 Consumer-Demand Matrix

## Status

Implementation-oriented demand evidence for
`docs/work/active/WI-20260810-v1-hal-first-vertical.md`. This document derives
HAL runtime capabilities from read-only `ev-cms-backend-new` evidence and the
human-approved charging v1 contract. It does not make HAL a User App or CPO
business service.

## Evidence

- User App charger discovery currently returns the literal `UNKNOWN` for
  charger and connector availability. It has customer inventory and wallet
  reads, but no charging start, progress, or session route yet.
- CPO `GET/PUT /chargers/{charger_id}/status` owns static CMS administrative
  status. It neither establishes a HAL connection nor proves live OCPP status.
- CMS owns customer/CPO identity, authorization, tariffs, holds, settlement,
  and its durable customer/CPO projections. HAL owns OCPP transport, exact
  charger-originated transaction truth, operational facts, meter facts, and
  charger-command delivery.

## Matrix

| Consumer need | HAL-owned fact or capability | Socket / plug | Durability and freshness | Deadline class | Notes / non-goals |
| --- | --- | --- | --- | --- | --- |
| User App charger browsing with honest availability | Current charger connection state and last observation | charger runtime query; `charger.connection.updated` | Current state survives HAL restart in PostgreSQL; connected/offline observation time and sequence disclose freshness | MUST SHIP BY 2026-08-14 | CMS combines this with its static publication/admin status. Do not expose a customer HAL route. |
| User App and CPO live charger online/offline | Connection existence, generation-safe transitions, last meaningful activity | charger runtime query; `charger.connection.updated` | Generation guard prevents stale disconnect overwrite; `OFFLINE` makes last connector state stale | MUST SHIP BY 2026-08-14 | `UNKNOWN` is allowed where HAL has no observation. |
| User App live connector availability | Latest charger-originated OCPP `StatusNotification` | connector runtime query; `connector.status.updated` | Preserve observed time, sequence, status freshness | MUST SHIP BY 2026-08-14 | CMS determines customer presentation. |
| CPO connector faults | OCPP status, error code, info, vendor fields when sent | connector runtime query; `connector.status.updated` | Last authoritative status retained, explicitly stale while charger offline | MUST SHIP BY 2026-08-14 | Do not invent diagnostics interpretation. |
| Customer starts charging | Persisted app credential and idempotent RemoteStart command | start-command socket; command query; `command.updated` only where useful | Command and credential durable before OCPP delivery | MUST SHIP BY 2026-08-14 | CMS authorizes and supplies opaque start-intent/correlation values. HAL does not accept customer tokens. |
| STARTING versus actual ACTIVE | Durable command lifecycle separated from charger-originated start | command/transaction reconciliation; `transaction.started` | RemoteStart acknowledgement is never ACTIVE | MUST SHIP BY 2026-08-14 | CMS projects customer-safe progress. |
| CPO start/stop progress | Generic command and transaction state | command and transaction queries | Durable state with attempts, response, error, correlation | MUST SHIP BY 2026-08-14 | CMS CPO RBAC remains outside HAL. |
| Active session visibility | Exact OCPP transaction ID, HAL transaction, start time, connector | active-transaction and transaction query; `transaction.started` | Start is authoritative only after `StartTransaction` | MUST SHIP BY 2026-08-14 | No latest-row inference. |
| Near-live energy and consumed Wh | Latest usable cumulative MeterValues energy sample | transaction meter query; `transaction.meter` | Integer Wh, sample observation time, sequence, staleness; no interpolation | MUST SHIP BY 2026-08-14 | Optional power/current/voltage/SoC only when charger supplies it; no telemetry platform. |
| Customer elapsed time | Actual start timestamp | transaction query | CMS derives display time from HAL actual start; no HAL fabricated clock samples | MUST SHIP BY 2026-08-14 | CMS owns customer projection. |
| Energy limit | Persisted approved `energy_limit_wh`, exact Wh comparison | start socket; unified stop coordinator; command/transaction query | Threshold and stop workflow durable and restart-safe | MUST SHIP BY 2026-08-14 | Actual final energy may exceed limit; commercial treatment is CMS policy. |
| Time limit | Persisted `max_duration_seconds`, deadline from `StartTransaction` | start socket; unified stop coordinator; transaction query | Deadline persisted and rebuilt after restart | MUST SHIP BY 2026-08-14 | Not measured from user request or RemoteStart acknowledgement. |
| Customer stop | Generic durable RemoteStop against exact transaction identity | stop-command socket; command query | One idempotent stop workflow, not completion | MUST SHIP BY 2026-08-14 | CMS authorizes customer ownership before calling HAL. |
| CPO stop | Same generic durable RemoteStop | stop-command socket; command query | Same coordinator and recovery semantics as customer stop | MUST SHIP BY 2026-08-14 | No CPO-specific HAL endpoint. |
| Stop provenance | Requested stop initiator and OCPP `StopTransaction.reason` | transaction query; `transaction.completed` | Both preserved durably | MUST SHIP BY 2026-08-14 | Never rewrite the charger reason to the HAL initiator. |
| Actual completion and final energy | Charger-originated StopTransaction and final meter | transaction query; `transaction.completed` | Exact final integer Wh and completion reason retained | MUST SHIP BY 2026-08-14 | RemoteStop acknowledgement is not completion. |
| Lost response, duplicate command, restart, reconnect | Idempotent command identity, authoritative queries, durable fact outbox, recovery | command/transaction/runtime queries; all required facts | Deduplication, retries, restart reconstruction, ordered operational sequences | MUST SHIP BY 2026-08-14 | CMS consumes facts idempotently and reconciles by query. |
| CPO availability, reset, unlock, diagnostics, firmware/configuration | Existing inherited OCPP controls | None in new v1 initially | Existing behavior only | USEFUL BUT DEFER | Add a generic protocol-control socket only after CMS CPO workflow, authorization, audit, and recovery requirements are approved. |
| Legacy `/api/*` REST and frontend WebSocket compatibility | Retired copied surface | None | None | RETIRED | Neither defines new-CMS contracts nor substitutes for v1 queries/facts. |
| RFID/local auth list, reservations, smart charging, roaming | None | None | None | DEFER AFTER HAL HANDOFF | Not required for first vertical slice. |

## Must-Ship HAL Socket Families

1. Durable idempotent `RemoteStart` with pre-existing start credential,
   `energy_limit_wh`, and `max_duration_seconds` where approved by CMS.
2. Durable idempotent `RemoteStop` against an exact HAL/OCPP transaction
   correlation, shared by User App and CPO requests.
3. Command lookup by CMS command identity and transaction lookup by durable
   correlation; no inferred latest transaction.
4. Charger, connector, active-transaction, and latest-meter operational queries
   that disclose observation and freshness.
5. Durable start-credential state used by `Authorize` and `StartTransaction`.

## Must-Ship HAL Plugs

- `charger.connection.updated`
- `connector.status.updated`
- `transaction.started`
- `transaction.meter`
- `transaction.completed`
- `command.updated` only for meaningful lifecycle transitions; command query
  remains the recovery source.

Fact delivery uses authenticated HTTP and a PostgreSQL durable outbox. Delivery
is replayable and idempotent; push is not the only recovery path.

## Deliberately Preserved Boundary

HAL exposes generic OCPP/runtime capabilities. CMS maps CMS charger UUIDs,
connector UUIDs, customer identity, CPO authorization, financial decisions, and
customer/CPO display states. No customer or CPO business endpoint belongs in
HAL.

## Current Implementation Boundary

Opaque bearer service authentication, mapping enrollment, durable start/stop
commands, StartTransaction materialization, exact integer-Wh MeterValues,
deadline/energy-limit stop coordination, StopTransaction completion,
reconciliation reads, charger/connector runtime, and durable fact delivery are
implemented on the HAL side. No inherited API key is reused for v1. CMS
consumer projections, commercial behavior, and production deployment remain
separate work.
