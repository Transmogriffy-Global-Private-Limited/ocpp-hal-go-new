# OCPP HAL for ev-cms-backend-new

This repository is the independently evolving OCPP 1.6 HAL for `ev-cms-backend-new`. It was copied with Git history from `OCPPHAL_Go`, but it does not exist to preserve the old CMS/frontend compatibility surface. The old HAL and its old CMS relationship remain separate and out of scope.

## Architecture Boundary

The HAL owns OCPP transport/protocol behavior, charger connectivity and reconnection, charger-originated transaction truth, raw/live meter handling, and charger-command delivery. `ev-cms-backend-new` owns CPO/customer identity, business eligibility, tariffs/pricing, wallets/payments/billing, and customer-facing charging-session/business projections.

CMS and HAL must not share a database. They will integrate through an explicit authenticated service boundary. A remote start or stop acknowledgement means that a charger accepted a command; only charger-originated StartTransaction or StopTransaction establishes OCPP start or completion truth.

The complete record of decided boundaries, inherited facts, and unresolved decisions is [docs/ARCHITECTURE_BOUNDARY.md](docs/ARCHITECTURE_BOUNDARY.md).

## Current Inherited Implementation

The current Go code contains useful inherited OCPP behavior:

- an `ocpp-go` OCPP 1.6 central system;
- generation-guarded charger connections;
- local PostgreSQL transaction/outbox persistence with a memory fallback;
- live meter updates, max-kWh RemoteStop retry behavior, and boot recovery;
- an optional charger directory/cache;
- copied REST command/status APIs, callback URLs/payloads, frontend WebSockets, virtual chargers, smoke clients, and regression scripts.

These are observations, not approved new-CMS contracts. See [docs/INHERITED_HAL_AUDIT.md](docs/INHERITED_HAL_AUDIT.md) for each subsystem's KEEP/MODIFY/REPLACE/REMOVE/INVESTIGATE classification and the property that a future replacement must preserve.

## Repository Layout

| Path | Purpose |
| --- | --- |
| `cmd/ocpphal` | Main OCPP HAL binary. |
| `cmd/cpconsole` | Interactive OCPP 1.6J virtual charge point. |
| `cmd/cpsmoke`, `cmd/cplimitsmoke`, `cmd/cpsinglesmoke` | Inherited OCPP smoke clients. |
| `cmd/frontendwssmoke` | Inherited frontend WebSocket smoke client. |
| `cmd/mockhooks` | Legacy-shaped local callback/directory mock. |
| `internal/ocpp16hal` | OCPP central handlers, connection behavior, recovery, and outbound charger commands. |
| `internal/httpapi` | Inherited REST and frontend WebSocket surfaces. |
| `internal/store` | HAL-local PostgreSQL and memory transaction/outbox implementations. |
| `internal/hooks` | Inherited durable callback-outbox worker. |
| `internal/chargerdir` | Inherited external charger-directory client/cache. |
| `internal/config` | Environment configuration. |
| `migrations` | HAL-local PostgreSQL migrations. |
| `scripts` | Build and local regression automation. |
| `docs` | Architecture, audit, planning, state, and operational documentation. |

## Development Status

The current phase is architecture bootstrap plus inherited-system audit. No new CMS/HAL runtime contract is approved. The next work is a detailed audit paired with relevant `ev-cms-backend-new` wallet, tariff, and charging-session requirements before implementation begins.

Read [docs/README.md](docs/README.md) for the canonical documentation map and [docs/DEVELOPMENT_PLAN.md](docs/DEVELOPMENT_PLAN.md) for approved sequencing.

## Local Development

Use `.env.example` only as a reference; do not commit `.env` or real secrets. Keep local listeners on loopback. The inherited source exposes build and regression scripts, but both currently contain a hard-coded legacy repository path and must not be run if that would operate on `OCPPHAL_Go`.

For safe source-level checks in this checkout:

```powershell
go test ./...
go build ./...
git diff --check
```

Do not commit, push, deploy, or modify `OCPPHAL_Go` or `ev-cms-backend-new` without explicit human permission.
