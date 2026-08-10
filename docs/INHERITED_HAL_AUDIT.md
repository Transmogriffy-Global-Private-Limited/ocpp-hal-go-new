# Inherited HAL Audit

## Disposition

This is the evidence record for the clone cleanup. `RETAINED` and `ADAPTED`
identify code still used by the new HAL runtime. `RETIRED` is absent from the
supported runtime. `DEFERRED WITH REASON` is not a supported surface but
remains as historical migration/code material until a separately safe data
retirement is approved.

| Inherited subsystem | Final disposition | Evidence and preserved property |
| --- | --- | --- |
| OCPP central-system handlers | RETAINED | `ocpp-go` still owns protocol framing; boot, status, authorize, start, meter, and stop handlers preserve charger-originated transaction truth. |
| Charger authorization | ADAPTED | Every authorization now validates a durable v1 credential; unknown and ambiguous idTags fail closed. |
| Charger connection generation guard | RETAINED | The connection tracker still ignores stale disconnects, while durable v1 runtime records the generation and sequence. |
| Transaction persistence | ADAPTED | V1 PostgreSQL commands, credentials, transactions, stop workflows, and facts own new-runtime truth. Exact OCPP IDs remain durable and only StopTransaction completes them. |
| MeterValues handling | ADAPTED | V1 accepts exact integer Wh only, rejects non-integral/regressive samples, preserves sequence/observation time, and never fabricates samples. |
| Legacy `max_kwh` callback policy | RETIRED | Callback-derived kWh limits and their retry loop were removed. Persisted `energy_limit_wh` and the unified v1 stop workflow are the sole HAL enforcement path. |
| Callback/outbox machinery | RETIRED | Legacy start/completion callback worker, payloads, auth, and routes were removed. Immutable v1 fact outbox delivery preserves durable retry, idempotency, terminal reconciliation, and lease recovery. |
| Boot/reconnect recovery | ADAPTED | V1 recovery reconstructs only durable command/stop/deadline state; it never turns a RemoteStop acknowledgement into completion. |
| CMS-facing `/api/*` REST facade | RETIRED | Static-key, flexible-alias legacy routes are not registered. Only authenticated v1 mapping, command, runtime, and reconciliation routes remain. |
| Charger directory | RETIRED | External CMS directory lookup/cache was removed. Enabled v1 mapping is the connection-time admission authority; unmapped chargers cannot create runtime state. |
| Frontend WebSockets | RETIRED | Direct frontend status/transaction WebSockets were removed. CMS owns customer/CPO projections and realtime. |
| Single-session compatibility | RETIRED | Process-local pending-start routing and callback selection were removed; no replacement business distinction was invented. |
| Remote-only/local-auth sync | RETIRED | Unapproved automatic charger configuration policy was removed. Future offline/RFID policy requires an explicit contract. |
| Configuration/environment model | ADAPTED | Runtime requires PostgreSQL and `HAL_V1_CMS_BEARER_TOKEN`; only v1 fact-delivery and API-doc settings remain. |
| PostgreSQL schema/migrations | ADAPTED | Migrations `005`-`007` are active HAL-owned v1 state. Historical `transactions`/`callback_outbox` tables remain migration history but have no new-runtime caller. |
| Memory store | DEFERRED WITH REASON | The legacy in-memory implementation remains test-only source material and is not selectable by the production v1 process. Removing historical test code has no runtime safety benefit. |
| Virtual charger tooling | RETAINED | `cpconsole` remains an OCPP-native virtual charge point; legacy REST smoke binaries were retired. |
| Regression/build tooling | ADAPTED | Build produces only `ocpphal` and `cpconsole`; local regression runs the PostgreSQL HTTP/OCPP/fact-receiver v1 tests. |
| Go module identity | ADAPTED | Module and all repository-local imports now use `github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new`. |

## Historical Data

No landed migration was rewritten or dropped. Historical legacy tables are
not a supported integration surface and must not be read by CMS. Any physical
schema removal needs its own additive, backup-aware migration decision.
