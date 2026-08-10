# CMS/HAL Charging Integration Analysis

## Status

**Decision-ready analysis only.** This document records evidence and
recommendations for human approval. It does not approve an endpoint, event,
table, callback payload, authentication mechanism, or runtime change.

## 1. Current State Causal Map

### HAL today

```text
charger WebSocket connection
-> ocpp-go central system and connection-generation registration
-> charger-originated Boot/Authorize/StartTransaction/MeterValues/StopTransaction
-> HAL-local transaction, live-meter, and callback-outbox state
-> legacy callback delivery and legacy REST/WebSocket projections
-> boot/reconnect recovery and command retries where applicable
```

`StartTransaction` creates the local HAL transaction and supplies the OCPP
transaction ID. `StopTransaction` finalizes it. `RemoteStartTransaction` and
`RemoteStopTransaction` acknowledgements only report command acceptance.
`MeterValues` updates the current usable energy-register value; it does not
persist a raw meter-sample history. The generation guard prevents an old socket
close from taking a replacement connection offline.

The current start/completion callback, `/api/*` REST facade, optional charger
directory, `max_kwh` callback response, and frontend WebSockets are
legacy-shaped integration behavior. They are evidence, not a new-CMS contract.

### CMS today

```text
authenticated customer principal (customer UUID + CPO UUID)
-> CPO-scoped published hub/charger/connector discovery
-> informational tariff/GST resolution or wallet/recharge operations
-> PostgreSQL business records and audit/outbox behavior
-> REST views; no HAL call and no live charging-session route
```

CMS already has durable CPO, customer, group, charger, connector, tariff, GST,
wallet, wallet transaction, charging-session, payment, and audit record shapes.
Customer discovery intentionally reports connector availability as `UNKNOWN`
until a HAL contract exists. The price resolver is informational, not a charge
commitment. Wallet recharge verifies provider evidence, takes row locks, and
credits exactly once through a durable idempotent transaction.

The CMS has no implemented HAL client, service-to-service authentication,
charging command route, durable live-session ingestion, session settlement, or
customer charging-session API. Its customer-app plan explicitly defers
credential/idTag policy and the HAL lifecycle until design approval.

CMS currently exposes a configured `CHARGER_CONNECTION_URL` only when it builds
displayed WS/WSS charger connection URLs from its OCPP mapping; it is not a HAL
control or callback client. The User App is an authenticated HTTP surface and
the established local-development guidance is loopback-oriented. A new
service-to-service path must therefore be separately configured, authenticated,
and operationally verified rather than inferred from that display setting.

## 2. Authority Map

| Fact or decision | Sole authority | Notes |
| --- | --- | --- |
| Customer and CPO identity | CMS | Trusted customer principal is CPO-scoped. |
| CMS charger and connector identity | CMS | CMS charger UUID and connector UUID are business identities. |
| Public charger ID and OCPP identity mapping | CMS | Mapping is not proof of a live OCPP connection. |
| OCPP charge-point identity and connection generation | HAL | Must be validated against an approved CMS mapping, not inferred from a client value. |
| OCPP connector number | HAL protocol fact | CMS owns the mapping from connector number to its connector UUID. |
| Customer eligibility and authorization | CMS | HAL must consume an authenticated, scoped decision and fail closed on ambiguity. |
| idTag credential lifecycle | CMS, after approval | Current CMS plan leaves its model and storage unresolved. |
| Tariff, tax, wallet, holds, settlement, and billing | CMS | HAL must not calculate price or mutate CMS financial state. |
| Command intent and its business lifecycle | CMS | HAL owns delivery attempt and protocol response, not business completion. |
| OCPP transaction ID, actual start/stop, and live meter truth | HAL | Created/closed only by charger-originated OCPP events. |
| CMS charging-session and customer-facing projection | CMS | Derived from authenticated HAL facts with durable replay/reconciliation. |
| Final energy evidence | HAL | CMS may validate and snapshot received final facts for billing. |
| Final monetary outcome | CMS | Derived from an approved tariff/tax policy and final energy evidence. |
| Customer realtime | CMS | Realtime is derived notification; CMS REST projection is the recovery authority. |

Open mapping ambiguity: a durable, CPO-scoped one-to-one rule for CMS charger,
CMS connector, OCPP identity, and connector number has not been approved.

## 3. Identifier Model

The first contract must carry stable identifiers without treating these distinct
meanings as interchangeable:

| Identifier | Current or required owner | Meaning |
| --- | --- | --- |
| CMS charger UUID | CMS | Durable business-network row identity. |
| Public charger ID | CMS | Customer/CPO-facing charger reference. |
| OCPP identity | CMS mapping; HAL connection fact | Configured correlation value, not a session ID. |
| CMS connector UUID | CMS | Durable business connector identity. |
| OCPP connector number | Charger/HAL | Protocol connector address. |
| CPO UUID, customer UUID, customer group UUID | CMS | Authorized commercial scope. |
| Credential/idTag identifier | CMS after approval | Authorization credential, never a substitute for customer identity. |
| CMS start intent ID | CMS after approval | Idempotency and business audit identity for a requested start. |
| CMS charging-session ID | CMS after approval | Customer/business projection identity. |
| CMS command ID/idempotency key | CMS after approval | A command delivery correlation distinct from the start intent. |
| HAL durable transaction identity | HAL | HAL-local durable row identity. |
| OCPP TransactionId | HAL/charger protocol | Exact number returned in StartTransaction confirmation and used by later OCPP messages. |

Do not derive an OCPP TransactionId in CMS, collapse a CMS session ID into an
OCPP transaction ID, or use an idTag as a durable customer identity.

## 4. Credential and idTag Options

### Evidence

The HAL currently accepts every `Authorize` idTag. CMS has customer principals
and a planned, but unimplemented, customer credential/idTag lifecycle. Neither
system currently provides a safe new-CMS authorization decision.

### Options

1. **Customer UUID as idTag:** easiest mapping, but leaks a stable business
   identifier to chargers/logs and has poor rotation and replay characteristics.
2. **Durable RFID/idTag credential:** supports physical RFID and familiar OCPP
   authorization, but needs CPO-scoped uniqueness, revoke/block lifecycle,
   secure presentation rules, audit, offline policy, and reconciliation.
3. **Short-lived one-use application credential bound to a CMS start intent:**
   avoids exposing the customer UUID, limits replay, and can bind charger,
   connector, CPO, customer, expiry, and intent. It still needs replay tracking,
   restart recovery, charger retransmission handling, and a future RFID path.

### Recommendation - not approved

Use a short-lived, one-use CMS authorization credential bound to the approved
start intent for the first app-start vertical. Design durable RFID/idTag as a
separate subsequent capability. The authorization decision must be CPO-scoped,
auditable, minimally disclosed to HAL/charger, replay-safe across restart, and
reconcilable when an event is delayed or retransmitted.

## 5. Start Lifecycle State Machine

The CMS must represent a request separately from OCPP truth:

```text
CMS start requested
-> CMS eligibility/tariff/hold decision recorded
-> command accepted for HAL delivery (or rejected before delivery)
-> HAL delivery attempt
-> charger RemoteStart response accepted/rejected/timeout
-> charger-originated StartTransaction
-> HAL durable OCPP transaction fact
-> CMS durable session projection becomes actually started
-> live meter facts may update the projection
```

`RemoteStartTransaction` acceptance is neither actual charging nor financial
commitment. A command timeout after delivery is ambiguous, not a failed start;
the CMS must wait for/reconcile authoritative HAL facts. Duplicate CMS requests
must resolve through the CMS start-intent identity. Duplicate OCPP starts use
HAL's existing same-transaction retransmission handling. Restarts require both
sides to reload durable pending/open state and reconcile rather than recreate a
session.

Recommended CMS state names are intentionally omitted: the state vocabulary and
legal transitions require approval. The required semantic distinctions are
requested, submitted for delivery, protocol acknowledged, actually started,
terminally rejected/failed, and reconciled.

## 6. Stop Lifecycle State Machine

```text
CMS stop requested or policy limit reached
-> CMS records command intent or HAL receives approved enforcement instruction
-> HAL delivery attempt
-> charger RemoteStop response accepted/rejected/timeout
-> charger-originated StopTransaction
-> HAL durable completion fact
-> CMS final-energy projection, financial settlement, and customer completion
```

The following must remain true in every path: a RemoteStop acknowledgement is
not completion; a rejected/timeout delivery result is not proof that charging
continued or ended; and a charger-originated `StopTransaction` may arrive after
reconnect, recovery, an external stop, or an earlier timeout. User cancellation,
remote stop, limit enforcement, charger-local stop, fault, and recovery each
need separately auditable reason evidence without guessing a final outcome.

## 7. Wallet and Tariff Integrity

CMS already uses decimal financial fields, exact integer minor units for
recharge provider amounts, CPO/customer scopes, transactional locks, and
idempotent wallet credits. Existing `ChargingSession` fields support tariff/tax
snapshots and integer Wh start/stop evidence but do not make a live billing
workflow implemented.

### Recommendation - not approved

Before a start is submitted, CMS should atomically select the applicable tariff
and tax, snapshot the commercial terms, assess affordability, and create a
durable financial hold/reservation with an explicit release/settlement policy.
One CMS pricing calculation must serve both affordability/allowed-energy policy
and final billing, rather than separately calculating a HAL `max_kwh` and a
final bill. Preserve money in exact decimal/minor-unit arithmetic and energy in
integer Wh; define rounding, tax, idle-fee, and insufficient-fund rules before
implementation.

The precise hold amount, authorization horizon, idle-fee policy, and settlement
model remain open.

## 8. Energy Enforcement

Inherited HAL can ingest a usable energy register, compare it with legacy
`max_kwh`, durable-claim a stop workflow, and send RemoteStop until an OCPP stop
fact is observed or its current routine reaches a terminal condition. This is
valuable protocol enforcement, not a tariff engine.

### Recommendation - not approved

CMS should own the approved limit/policy calculation and provide an explicit,
unit-defined enforcement instruction to HAL. HAL should enforce only a
well-formed authorized instruction against live OCPP energy facts. The design
must state Wh/kWh conversion, meter monotonicity/rollback handling, allowed
overshoot, duplicate meter guard, charger reconnect behavior, CMS unavailability
behavior, and reconciliation after a terminal delivery failure.

Preserve a controlled, idempotent, observable stop-delivery process with an
explicit terminal outcome and reconciliation path. Do not elevate the inherited
bounded retry count into a permanent requirement.

## 9. Cross-Boundary Transport

| Option | Strength | Main risk |
| --- | --- | --- |
| Synchronous authenticated HTTP only | Simple commands/queries and immediate errors | Lost responses, delayed facts, and recovery need separate durable work. |
| Durable events only | Durable fact fanout/replay | More infrastructure and weaker command feedback without a query path. |
| Hybrid | Synchronous command/eligibility decisions plus durable fact publication/replay | Must make authority, dedupe, and reconciliation explicit. |

### Recommendation - not approved

Adopt the hybrid shape only if the contract supplies one authenticated command
path and one durable/replayable HAL-fact path with a CMS reconciliation query.
This is the smallest apparent design that distinguishes command intent from
charger-originated truth. Whether the durable fact path is an outbox-backed HTTP
delivery, broker, or another mechanism is open; do not select a product or
schema yet.

## 10. HAL-to-CMS Facts

HAL facts should be protocol evidence, not business conclusions. The required
fact classes are:

- charger admission/connection and recovered connection context when approved;
- charger-originated actual start, exact OCPP TransactionId, meter start, idTag
  evidence, connector number, and timestamp;
- selected live meter progression or an approved aggregation, with sequence or
  monotonicity evidence;
- command delivery result/ambiguity, separately from transaction completion;
- charger-originated actual stop, final meter/stop reason/timestamp; and
- recovery/reconciliation findings, including the narrow inherited ghost case.

CMS derives session status, customer messaging, holds, billing, and final
monetary outcome. HAL must not turn a protocol acknowledgement into any of
those business facts.

## 11. Duplicate, Out-of-Order, and Ambiguous Evidence

| Situation | Authoritative evidence | Safe action | Forbidden guess | Reconciliation |
| --- | --- | --- | --- | --- |
| Duplicate CMS start | CMS start intent/idempotency | Return/continue same intent | Create a second hold or command | Query intent and HAL facts. |
| Command response lost | HAL delivery correlation plus later OCPP fact | Mark response ambiguous | Treat as failed or started | Reconcile command and transaction facts. |
| RemoteStart accepted without StartTransaction | Charger-originated start absent | Keep pending/expire by approved policy | Mark session active | Query/replay and recover. |
| Duplicate StartTransaction | HAL durable transaction + same protocol evidence | Reuse existing transaction fact | Create a second session | Publish/replay idempotently. |
| Meter for unknown/stale transaction | OCPP TransactionId and connector context | Reject/quarantine/record safely | Attach to latest session | Reconcile against HAL open transaction. |
| Out-of-order meter before CMS start projection | HAL start fact and meter ordering evidence | Store/replay fact after start correlation | Bill a guessed session | CMS replay/rebuild projection. |
| Duplicate or delayed StopTransaction | HAL stop idempotency | Preserve one completion fact | Reopen or double-settle | Replay completion to CMS. |
| RemoteStop accepted without StopTransaction | Stop fact absent | Continue controlled terminal/reconciliation flow | Mark completed | Query/recover charger state. |
| CMS unavailable during HAL fact delivery | HAL durable outbox/fact store | Retry/record terminal condition | Discard fact or invent billing | CMS replay/reconciliation. |
| CPO suspended during active charge | Existing actual transaction | Continue stop/fact/finalization pathways | Drop completion or bill nothing | Complete projection and settlement. |

## 12. CPO Suspension

CMS suspension revokes CPO sessions and records lifecycle audit/outbox evidence.
For the new contract, suspension must block *new* customer starts and related
new tenant operations, but it must never prevent an existing active session's
stop command, HAL fact ingestion, recovery, final energy capture, billing,
wallet settlement, or customer history completion. Charger administration and
whether a suspension triggers a deliberate stop policy remain separate human
decisions; no implicit stop should be inferred from a suspended CPO status.

## 13. Customer Realtime Ownership

CMS owns customer REST session/wallet/history projections and their
authorization. Realtime may notify or invalidate a CPO/customer-scoped view,
but cannot be the durable session authority. The client must be able to reload a
CMS REST snapshot after reconnect or missed notification. HAL frontend
WebSockets remain inherited evidence and must not become a direct customer
surface.

## 14. Legacy Isolation and Migration

The old `OCPPHAL_Go` to old-CMS integration remains untouched. This repository
must not dual-post the legacy callback and a new CMS contract as an accidental
permanent architecture, share databases, or copy CMS business logic into HAL.
When migration is approved, preserve the current correctness properties:
charger-originated start/stop truth, exact OCPP IDs, durable outbox/retry,
generation-safe connection state, conservative recovery, and testable virtual
charger flows. Temporary compatibility requires explicit consumers, precedence,
telemetry, and removal criteria.

## 15. First Vertical Slice

### Recommended scope - not approved

The first production-shaped slice should support one authenticated customer app
start intent for one mapped CMS charger/connector, one eligibility/tariff/hold
decision, one HAL command delivery attempt, one charger-originated start fact,
one CMS session projection, and a REST-readable customer status. It should also
support the corresponding charger-originated stop and a reconciliation path.

### Preconditions

- approved CPO/charger/connector mapping and service authentication;
- approved credential/idTag, eligibility, tariff snapshot, hold, and command
  idempotency semantics;
- HAL durable store/outbox availability policy and CMS durable projection model;
- approved reconciliation query/replay and audit fields.

### Non-goals

- legacy REST/callback/unauthenticated frontend-WebSocket compatibility;
- RFID, roaming, reservations, smart charging, raw meter-history export,
  generalized billing, and CPO-suspension-triggered mass stop behavior.

### Acceptance and failure evidence

Verify authorized and denied start, duplicate start, lost command response,
charger rejection, accepted command without StartTransaction, duplicate start,
meter before/after start projection, remote stop without completion, local stop,
duplicate/delayed stop, HAL/CMS restart, HAL/CMS temporary unavailability,
outbox terminal failure and replay, wrong CPO mapping, malformed identity, and
CPO suspension while a session is active. Verify that neither database is shared
and that the OCPP TransactionId is preserved exactly.

## 16. Decision Ledger

### Approved Existing Invariants

- `ocpp-go` remains the OCPP protocol implementation.
- HAL owns charger/OCPP truth, command delivery, raw/live meter behavior, and
  reconnection/recovery; CMS owns identity, commercial policy, and customer
  projections.
- The databases remain separate behind an authenticated service boundary.
- OCPP `StartTransaction` and `StopTransaction`, not RemoteStart/RemoteStop
  acknowledgements, establish actual start and completion truth.
- The legacy repository and old-CMS integration are outside this work.

### Recommended for Human Approval

- Short-lived one-use start credential for the first app-start flow, with RFID
  deliberately deferred.
- CMS-owned atomic tariff/tax snapshot and financial hold before HAL delivery.
- One approved commercial calculation for affordability, energy policy, and
  final billing, using exact financial arithmetic and integer Wh evidence.
- Hybrid service boundary: authenticated command path plus durable/replayable
  HAL fact path and reconciliation query.
- First vertical slice limited to one start/stop lifecycle with recovery tests.

### Still Open / Needs Evidence

- Exact service-auth mechanism, key/certificate rotation, message versioning,
  payloads, endpoint/event schemas, and delivery technology.
- Final mapping uniqueness rules, enrolment ownership, charger-directory
  replacement, and treatment of CMS `ocpp_identity` prefixes.
- Credential storage, expiry, encryption, RFID lifecycle, authorization latency,
  offline behavior, and charger retransmission semantics.
- Hold amount, rounding/tax/idle policy, wallet ledger/settlement model, tariff
  change timing, and financial failure handling.
- Meter sampling/aggregation, allowed overshoot, terminal stop state, retry
  policy, ghost-session policy, and reconciliation cadence.
- CPO suspension operational policy, customer realtime transport, legacy surface
  retirement order, and the production rule for no-database HAL startup.

## Evidence Sources

- HAL: `internal/ocpp16hal`, `internal/hooks`, `internal/store`,
  `internal/httpapi`, `internal/chargerdir`, `internal/config`, migrations, and
  local smoke/regression tooling.
- CMS (read-only): `src/models/schema.go`, `src/customerauth`, `src/cpo`,
  `docs/integrations/ocpp-hal-boundary.md`, and
  `docs/plans/customer-app-experience.md`.
