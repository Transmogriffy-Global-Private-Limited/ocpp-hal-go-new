# HAL and CMS Architecture Boundary

## Status and Scope

This is the canonical architecture record for `ocpp-hal-go-new` and
`ev-cms-backend-new`. The approved v1 human-readable service contract is
[CMS/HAL Charging Integration v1](contracts/CMS_HAL_CHARGING_V1.md).

The v1 architecture is approved. The HAL-side first vertical implements
PostgreSQL state, opaque bearer service authentication, mapping enrollment,
durable start/stop command coordination, charger-originated start/completion
truth, integer-Wh meter facts, live runtime projections, reconciliation
queries, and PostgreSQL outbox delivery. The inherited legacy runtime was
retired from this process. CMS-side projection and deployment topology remain
outside this work.

## Permanent Approved Invariants

- HAL uses `github.com/lorenzodonini/ocpp-go` for OCPP protocol handling; it
  must not manually reimplement OCPP framing.
- HAL owns OCPP transport/protocol behavior, charger connections and
  reconnections, command delivery, exact charger-originated transaction truth,
  raw/live meter facts, and protocol recovery.
- CMS owns CPO/customer/group identity, eligibility, tariff/tax, wallet,
  charging-session business projections, settlement/billing, and
  customer-facing state.
- CMS and HAL never share a database. They communicate across an authenticated
  service boundary.
- A RemoteStart acknowledgement is not charging truth; charger-originated
  StartTransaction establishes actual OCPP start. A RemoteStop acknowledgement
  is not completion truth; charger-originated StopTransaction establishes it.
- HAL/Central System allocates the exact OCPP transaction ID only when accepting
  charger-originated StartTransaction. CMS never generates, substitutes, or
  guesses that ID.
- Charger timestamps are protocol evidence, not the authority for credential
  expiry or time-limit enforcement. HAL retains the protocol timestamp and a
  trusted receipt timestamp, accepts only bounded clock skew, and bases
  security/deadline decisions on the trusted receipt.
- HAL accepts a completion only when its exact OCPP transaction identity,
  connector, protocol ordering, and receipt ordering validate. Its effective
  cumulative meter remains nondecreasing: a raw stop one Wh below the latest
  eligible meter sample may be recorded and normalized solely as a documented
  register-quantization discrepancy; all other rollbacks are rejected. A
  conflicting duplicate completion is rejected and does not create a completed
  fact.
- CMS charger UUID, public charger ID, OCPP identity, CMS connector UUID, OCPP
  connector number, CPO/customer/group IDs, CMS start intent/session/command,
  app credential/idTag, HAL command/transaction IDs, and OCPP transaction ID
  remain distinct identities.
- The first QR/app flow uses a short-lived, one-use, CPO-scoped,
  customer/start-intent/charger/connector-bound, expiring, auditable, replay-
  resistant, durably recoverable credential as OCPP idTag. It is not a customer
  UUID. RFID is a separate later credential type.
- A user start creates a durable CMS start intent, not an active session. CMS
  materializes an active charging session only after an authoritative HAL
  StartTransaction fact.
- Before submitting start, CMS atomically derives trusted scope, resolves and
  freezes tariff/tax, assesses affordability with one canonical pricing engine,
  creates a wallet hold, creates the start intent, and creates/associates the
  one-use credential. Money remains exact and cross-service billable-energy
  policy uses integer Wh.
- HAL persists a durable remote command before transmitting RemoteStart or
  RemoteStop. Duplicate CMS command identity resolves to that same command, not
  a new OCPP dispatch. Timeout after delivery is ambiguous and requires
  reconciliation.
- CMS to HAL is synchronous authenticated HTTP command/query. HAL to CMS is
  durable-outbox-backed authenticated HTTP delivery of versioned HAL facts with
  fact identity, dedupe, retry, and reconciliation. No v1 broker is introduced.
- HAL facts are protocol evidence, never tariff, wallet, GST, or final-billing
  conclusions. CMS performs final completion and settlement transactionally and
  idempotently from durable completion evidence and frozen commercial terms.
- CMS owns commercial energy policy. HAL enforces the approved integer-Wh limit
  after persisting it before RemoteStart, with controlled, idempotent,
  observable stop delivery and explicit terminal/reconciliation semantics.
- Production HAL requires PostgreSQL and must fail safely when it is absent or
  unavailable. Memory storage is only for explicit tests and intentional local
  development.
- CPO suspension blocks new starts, but never blocks active-session stop,
  fact ingestion, recovery, reconciliation, final meter capture, settlement, or
  completed history. It does not imply a mass stop without separate approval.
- CMS REST state is authoritative for User App charging sessions. HAL frontend
  WebSockets are not a User App contract; future realtime is
  notification/invalidation with CMS REST recovery.
- HAL is authoritative for current charger connection state, charger-originated
  per-connector OCPP StatusNotification state, and accepted live meter facts.
  CMS keeps those as a durable operational/session projection with observation
  time and explicit freshness. Offline or unknown connection makes historical
  connector status stale, never fresh live truth.
- Connection generation fences superseded callbacks only within one live HAL
  process. Startup first marks prior connection state `UNKNOWN`, resets the
  durable generation baseline, and then lets the first new-process connection
  establish `ONLINE`. Durable connection sequence, not generation, orders CMS
  projection facts across process restarts.
- An accepted OCPP Heartbeat from the current tracked connection is durable
  liveness evidence: it preserves `ONLINE` and generation, advances connection
  sequence, and publishes the corresponding fact. Heartbeat renewal cannot
  create or resurrect a connection. The default requested cadence is five
  minutes; CMS owns a separately configured, longer stale horizon.
- HAL treats charger input by evidence class: StartTransaction and
  StopTransaction are irreplaceable lifecycle truth and must commit with their
  durable outbox evidence before acceptance; mapped StatusNotification and
  supported active-transaction MeterValues persist before acknowledgement;
  unsupported, stale, and uncorrelatable observations do not advance a
  projection. Heartbeat is refreshable liveness and physical socket callbacks
  are transport facts, so failed durable connection projection is retried with
  the same observation and becomes `UNKNOWN` on restart rather than invented
  freshness.
- Boundary handling is fail-safe: malformed, unauthorized, stale, duplicate,
  out-of-order, inconsistent, or ambiguous input never authorizes a business
  outcome by guesswork. Durable identity and auditability are required.
- The legacy `OCPPHAL_Go` to old-CMS integration remains untouched and outside
  this work.

## Approved v1 Contract Surface

The contract fixes these logical, service-only paths and payload semantics:

- CMS to HAL: `POST /v1/remote-commands/start`,
  `POST /v1/remote-commands/stop`, command lookup, and exact transaction
  reconciliation lookups.
- HAL to CMS: `POST /v1/hal-facts` for immutable, versioned
  `charger.connection.updated`, `connector.status.updated`,
  `transaction.started`, `transaction.meter`, `transaction.completed`, and
  required `command.updated` facts.
- Fact identity is a durable HAL `fact_id` plus a canonical immutable-content
  SHA-256. CMS treats repeated identical facts as success and same-ID/different-
  content as a durable integrity violation.
- `transaction.meter`, connection, and connector status are first-vertical-
  slice facts. CMS User App polling is the resulting read model; no customer
  WebSocket/SSE is introduced for v1.

Full request/response shapes, state tables, idempotency layers, failure rules,
and first-slice criteria are authoritative only in
[CMS/HAL Charging Integration v1](contracts/CMS_HAL_CHARGING_V1.md).

## Current Inherited Facts

These describe the cloned code before the current v1-only process retirement.
They explain why behavior was preserved or replaced; they are not current
runtime contracts.

- Central handlers already created/closed local transactions only from
  charger-originated StartTransaction/StopTransaction and carried a generation
  guard. V1 retains both correctness properties in its own state.
- MeterValues selected usable energy-register samples and legacy max-kWh
  handling triggered a stop. V1 replaces it with persisted integer-Wh meter
  facts and the shared v1 energy-limit stop workflow.
- Callback URLs/payloads, `max_kwh` response coupling, `POST /api/*`, optional
  charger-directory behavior, and frontend WebSockets were legacy behavior.
  They are no longer registered by this process.
- The copied module/import identity named the legacy repository. The module and
  repository-local imports now identify this repository; scripts derive the
  checkout root from their own location.

## Open Decisions

The following remain deliberately unresolved and must not be invented during
implementation:

1. Credential rotation and deployment topology.
2. Wallet overshoot, debt, or negative-balance policy.
3. Any future predictive energy stop-guard formula and supporting charger
   power/sampling evidence. V1 uses the approved simple accepted-sample
   threshold and preserves actual overshoot.
4. RFID lifecycle and offline authorization policy.
5. Generalized realtime/live-availability projection.
6. Whether historical legacy tables require a separately approved destructive
   retirement migration.
7. Detailed future migration/table design and implementation retry constants.

## Required Implementation Method

Implement the approved first vertical slice only through the v1 contract. Before
runtime work, map every affected CMS/HAL producer, consumer, durable state,
migration, authentication boundary, retry, recovery, test, and documentation
surface. Do not reinterpret an open decision as a license to bypass a v1
invariant.
