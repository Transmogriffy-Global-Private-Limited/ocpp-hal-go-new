# HAL and CMS Architecture Boundary

## Status and Scope

This document is the canonical boundary record for `ocpp-hal-go-new` and `ev-cms-backend-new`. It separates already-decided invariants from facts found in the copied implementation and from work that still needs a deliberate contract decision.

It does not approve inherited legacy REST routes, callbacks, WebSockets, database fields, or environment variables as the new CMS/HAL contract.

## Permanent Invariants

- The HAL uses `github.com/lorenzodonini/ocpp-go` for OCPP protocol handling; it must not manually reimplement OCPP framing.
- The HAL owns OCPP transport/protocol behavior, charger connections and reconnections, exact charger-originated transaction truth, charger-originated protocol state, raw/live meter handling, and charger command delivery.
- `ev-cms-backend-new` owns customer/CPO identity, business eligibility and authorization, tariffs/pricing, wallet/payment/billing, and customer-facing charging-session and business projections.
- A `RemoteStartTransaction` acknowledgement confirms only charger-command acceptance. A charger-originated `StartTransaction` establishes actual OCPP start truth.
- A `RemoteStopTransaction` acknowledgement confirms only charger-command acceptance. A charger-originated `StopTransaction` establishes actual OCPP completion truth.
- HAL and CMS do not share a database. Every integration crosses an explicit authenticated service boundary.
- The legacy `OCPPHAL_Go` to old-CMS integration remains untouched and outside this repository's work.
- Inherited implementation is prior art, not automatically a permanent new-CMS contract. A replacement must retain the correctness, recovery, identity, and audit properties it needs.
- Boundary handling is fail-safe: malformed, inconsistent, unauthorized, duplicate, stale, out-of-order, or ambiguous input does not authorize a business outcome by guesswork.
- Durable identity and auditability are preserved.

## Current Inherited Facts

These are code observations as of the architecture-bootstrap audit, not future contract promises.

- The service is an OCPP 1.6 central system backed by `ocpp-go`; its current central handlers create and close local transaction rows only from charger-originated StartTransaction and StopTransaction messages.
- A connection-generation tracker prevents a stale disconnect from marking a newer connection for the same charger offline.
- PostgreSQL can persist a local transaction row and callback-outbox task; without database configuration the process falls back to an in-memory store.
- MeterValues currently selects one energy-register-like sample, updates a local live meter value, and can initiate a local max-kWh stop workflow.
- The inherited callback worker posts legacy-shaped start/completion payloads to configured URLs, derives `max_kwh` from the start callback response, and retries from an outbox. Those URLs and payloads are legacy integration facts.
- Boot recovery loads local open rows, force-closes a narrow "Available" ghost case, and otherwise hydrates in-memory state before retrying RemoteStop and Unlock. It does not make a remote-stop acknowledgement a local completion.
- Existing `/api/*` and `/frontend/ws/*` surfaces use one shared API key or no frontend authentication. Their schemas and authorization are not an approved new-CMS interface.
- The copied module/import identity still names the legacy repository. Build and regression scripts now derive this checkout root from their script location; that remediation did not change test semantics or runtime behavior.

## Unresolved Design Decisions

The following require a jointly reviewed new-CMS/HAL design before runtime work. No endpoint, event, callback, table, or payload schema is fixed here.

1. The authenticated service-boundary mechanism, service identities, key or certificate rotation, authorization context, replay protection, and audit fields.
2. How CMS submits charger commands, obtains command acceptance/timeout status, retries idempotently, and later correlates charger-originated truth with a business charging-session projection.
3. The stable cross-system identifiers for CPO, charger, connector, customer, authorization attempt, OCPP transaction, CMS session, and command intent.
4. The CMS-to-HAL eligibility decision and the HAL-to-CMS start, meter, stop, recovery, and command-result information. This includes which data is durable, raw, sampled, aggregated, or privacy-restricted.
5. The tariff/wallet limit decision flow: CMS owns pricing and eligibility; HAL may execute an approved charger stop command, but the source, units, timing, and acknowledgement/reconciliation semantics are undecided.
6. Whether a durable outbox/event mechanism, synchronous service calls, or a combination is used across the service boundary, including retries, deduplication, ordering, terminal failures, and reconciliation.
7. The new owner and source of truth for charger enrollment/directory data, availability and status projections, customer-facing realtime updates, and access control.
8. Which inherited database fields and migrations remain HAL-local OCPP audit state, how they are evolved additively, and how no CMS database coupling is introduced.
9. The fate of legacy single-session routing, the legacy REST compatibility API, permissive frontend WebSockets, remote-only configuration policy, and all legacy callback URLs/payloads.
10. The authoritative machine-readable contracts, interactive documentation, contract-drift checks, and migration/rollout sequence for any new external surface.

## Required Decision Method

Before resolving an item above, inspect both sides of the proposed interaction: the CMS business requirement and data authority, the HAL OCPP/recovery requirement, existing consumers, failure modes, duplicate delivery, restart behavior, authorization, and auditability. Record the approved result here and in the appropriate machine-readable and integration contract documents.
