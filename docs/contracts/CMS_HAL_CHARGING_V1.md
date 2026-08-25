# CMS/HAL Charging Integration v1 Contract

## Status

**Human-approved architecture and partially implemented contract.** This is
the authoritative human-readable v1 contract for charging integration between
`ev-cms-backend-new` (CMS) and `ocpp-hal-go-new` (HAL).

Implemented in this repository: authenticated mapping enrollment, durable
PostgreSQL start/stop commands and one-use `appv1_` credentials, RemoteStart
delivery, `Authorize` validation, charger-originated StartTransaction
materialization, exact MeterValues and StopTransaction truth, command/
transaction reconciliation, mapped charger/connector runtime, and durable fact
outbox delivery are implemented on the HAL side. CMS-side consumption remains a
separate repository slice. This status statement outranks older target-only
wording below.

This contract does not replace a legacy route or callback merely by existing.
The legacy `OCPPHAL_Go` to old-CMS integration remains outside its scope.

## 1. Ownership and Trust Boundaries

| Responsibility | Authority |
| --- | --- |
| Customer, CPO, group identity; eligibility; tariff/tax; wallet hold; settlement; customer charging-session projection and REST state | CMS |
| OCPP transport, charger connection/reconnection, charger-originated connector status, protocol recovery, RemoteStart/RemoteStop delivery, exact OCPP transaction truth, and raw/live meter facts | HAL |
| CMS operational projection and customer polling response | CMS, derived from HAL facts | CMS does not synchronously ask HAL for ordinary reads. |
| Business conclusion, price, GST, wallet mutation, final billed money | CMS only |
| OCPP command acknowledgement and charger-originated protocol facts | HAL only |

CMS and HAL have separate databases. Neither service reads or writes the
other's tables. Every request and fact crosses an authenticated,
service-to-service HTTP boundary. A customer bearer token is never HAL service
authentication, and HAL does not receive authority to make customer, tariff,
tax, wallet, or billing decisions.

The implemented v1 service identity is `Authorization: Bearer <opaque token>`
using `HAL_V1_CMS_BEARER_TOKEN`. It is never a customer token, `API_KEY`, or
`APIAUTHKEY`; comparison is constant-time and the value is never logged.
Mutating calls require `Idempotency-Key` and `X-Correlation-ID`; both must be
canonical lowercase, nonzero UUIDs and the idempotency header must exactly
equal the body `cms_command_id`. Production
transport TLS/private-network topology and token rotation remain deployment
decisions; production must provide the opaque secret outside the repository.

## 2. Truth and Identifier Rules

`RemoteStartTransaction` acceptance does not prove charging started. Only a
charger-originated `StartTransaction` proves actual OCPP start. Likewise,
`RemoteStopTransaction` acceptance does not prove completion; only a
charger-originated `StopTransaction` proves actual OCPP completion.

When HAL accepts a charger-originated `StartTransaction`, the Central System
allocates the exact OCPP `transactionId`. The charger subsequently carries that
exact ID in `MeterValues`, `StopTransaction`, and related OCPP traffic. CMS
never generates, substitutes, or guesses this ID.

| Identifier | Owner | Meaning and rule |
| --- | --- | --- |
| `cms_charger_id` | CMS | Durable CMS charger UUID. |
| `public_charger_id` | CMS | Customer-facing charger identifier; it is not an OCPP identity. |
| `charger_ocpp_identity` | CMS mapping / HAL connection | Charge-point protocol identity. HAL validates it against the approved mapping. |
| `expected_serial` | CMS inventory evidence | Optional physical serial. It supplements but never replaces the canonical OCPP identity. |
| `cms_connector_id` | CMS | Durable CMS connector UUID. |
| `ocpp_connector_number` | Charger/HAL | OCPP connector address; CMS owns its approved mapping to `cms_connector_id`. |
| `cpo_id`, `customer_id`, `customer_group_id` | CMS | Trusted commercial scope; group is optional. |
| `cms_start_intent_id` | CMS | Durable request-to-start identity, distinct from a session. |
| `cms_charging_session_id` | CMS | Durable business projection materialized only after a start fact. |
| `cms_command_id` | CMS | UUID for one CMS command and its idempotency identity. |
| `app_charging_credential` / `id_tag` | CMS credential; HAL protocol evidence | One-use v1 credential, not a customer UUID. |
| `hal_command_id` | HAL | Durable remote-command UUID. |
| `hal_transaction_id` | HAL | Durable HAL transaction UUID. |
| `ocpp_transaction_id` | HAL/Central System | Exact OCPP transaction ID allocated on StartTransaction acceptance. |

No correlation may use "latest session", charger plus time, a public charger ID
alone, or a guessed connector. A fact that cannot prove its approved identifier
chain is retained as auditable evidence and put into reconciliation; it does not
materialize or settle a CMS session.

HAL rejects a zero, malformed, uppercase/noncanonical, or conflicting identity
at the HTTP boundary. It does not normalize one representation and compare a
different raw representation elsewhere.

## 3. App Charging Credential

The first QR/app charging vertical uses a short-lived, one-use application
charging credential as the OCPP `idTag`.

- CMS creates it with the start intent in the same durable business operation.
- It is CPO-scoped, customer-bound, start-intent-bound, charger/connector bound
  where appropriate, expiring, auditable, replay-resistant, and durably
  recoverable.
- CMS must not expose the customer UUID as an idTag.
- HAL persists the delivered idTag in its durable start command and uses it only
  as protocol/correlation evidence. It is not a customer identity.
- Credential values are sensitive operational data: logs, outbox diagnostics,
  and client errors must redact them.

RFID is a separate later credential type. This contract does not define RFID
storage or offline authorization policy.

## 4. CMS Lifecycle

### 4.1 Start intent

CMS creates a start intent only in a transaction that derives trusted scope,
resolves effective tariff and active tax inputs, freezes the commercial
snapshot, assesses affordability, creates the wallet hold, and creates or
associates the one-use credential. A start intent is not an active session.

| State | Meaning | Allowed next state |
| --- | --- | --- |
| `REQUESTED` | Commercial prerequisites, hold, snapshot, and credential are durable. No HAL command is accepted yet. | `ACCEPTED_FOR_DELIVERY`, `REJECTED`, `EXPIRED`, `RECONCILIATION_REQUIRED` |
| `ACCEPTED_FOR_DELIVERY` | HAL durably persisted the logical start command before OCPP transmission. | `PROTOCOL_ACKNOWLEDGED`, `REJECTED`, `EXPIRED`, `RECONCILIATION_REQUIRED`, `ACTUALLY_STARTED` |
| `PROTOCOL_ACKNOWLEDGED` | Charger accepted RemoteStart only at the protocol layer. | `ACTUALLY_STARTED`, `EXPIRED`, `RECONCILIATION_REQUIRED` |
| `ACTUALLY_STARTED` | Durable `transaction.started` evidence was correlated. CMS materializes one active session. | Terminal through session lifecycle |
| `REJECTED` | The request cannot proceed and no actual-start fact exists. | `RECONCILIATION_REQUIRED` only for contrary late evidence |
| `EXPIRED` | No actual-start fact arrived before the approved start window ended. | `RECONCILIATION_REQUIRED` for later evidence |
| `RECONCILIATION_REQUIRED` | Timeout, conflict, late evidence, or mapping failure makes truth incomplete. | Evidence-backed terminal/active state only |

A RemoteStart timeout after transmission is `RECONCILIATION_REQUIRED`, not
`REJECTED` or `ACTUALLY_STARTED`.

### 4.2 Charging session

There is no CMS charging session before `transaction.started` is durably
accepted.

| State | Entry requirement | Meaning |
| --- | --- | --- |
| Absent | No valid start fact | No active customer charging-session projection exists. |
| `ACTIVE` | Exact start-intent/command/credential/HAL-transaction correlation validates | One CMS session is materialized from authoritative HAL start evidence. |
| `RECONCILIATION_REQUIRED` | Active-session facts conflict, predecessor evidence is missing, or final financial result is unsafe | CMS retains facts and prevents guesswork or duplicate settlement. |
| `COMPLETED` | Completion fact is processed in CMS settlement transaction | Final energy, frozen commercial terms, ledger effect, and completion facts are immutable/auditable. |

A RemoteStop acknowledgement leaves the session `ACTIVE`; it cannot complete it.

## 5. HAL Remote-Command Lifecycle

HAL persists a remote command before transmitting RemoteStart or RemoteStop. Its
durable state contains the CMS request identity, kind, OCPP charger identity,
connector number, idTag when relevant, exact OCPP transaction ID for stop,
energy/time-limit instructions, expiry, state, attempts/result/error/timestamps,
and resulting HAL transaction correlation when materialized.

| State | Meaning | Transition rule |
| --- | --- | --- |
| `PERSISTED` | Durable command exists; no OCPP transmission occurred. | Must exist before transmit. |
| `PENDING_DELIVERY` | A dispatcher has claimed a known pre-network window. | Startup recovery returns it to `PERSISTED`; it is the only additional replayable state. |
| `DELIVERY_ATTEMPTED` | HAL attempted OCPP transmission and awaits an outcome. | Missing response becomes `AMBIGUOUS`. |
| `OCPP_ACCEPTED` | Charger accepted RemoteStart/RemoteStop. | Never proves start/completion. |
| `OCPP_REJECTED` | Charger explicitly rejected the command. | Contradictory later fact requires reconciliation. |
| `AMBIGUOUS` | Delivery timeout, lost response, or uncertain outcome. | Query/reconcile; a controlled retry is the same command only. |
| `EXPIRED` | Command became ineligible before a safe result. | It cannot be automatically revived. |
| `SUPERSEDED` | State advanced and this command/retry must no longer dispatch. | Stale retries stop. |
| `MATERIALIZED` | A resulting HAL transaction is correlated. | It records protocol truth, not CMS business completion. |

The same `cms_command_id` with the same immutable body returns the existing
`hal_command_id`. Different immutable content is an idempotency conflict. A
retry can act only on that same durable command, never a new logical command.

## 6. HTTP Boundary and Error Semantics

All paths are service-internal logical routes. `v1` is the contract-major
version; additive optional fields may be introduced compatibly, while
incompatible semantics require a new major version. Every request must use
mutually authenticated service identity and carry a trace/audit correlation ID.
V1 uses the opaque bearer mechanism defined above. Rotation and deployment
topology remain operational decisions.

`Idempotency-Key` is required on every mutating request. For CMS commands it is
the canonical textual UUID form of `cms_command_id`; HAL rejects a header/body
mismatch. For HAL facts it is the canonical textual UUID form of `fact_id`. The
header makes retries explicit and the durable body identifier supports
cross-system audit and lookup.

### 6.1 CMS to HAL

| Method and path | Semantics | Retry safety |
| --- | --- | --- |
| `POST /v1/remote-commands/start` | Persist and schedule one RemoteStart command. `202` only after durable HAL command creation/recovery; it is not OCPP acceptance or actual start. | Safe with the same idempotency key and immutable body. |
| `PUT /v1/mappings/chargers/{cms_charger_id}` | Enroll the durable CMS CPO/charger/connector to OCPP mapping used for command and runtime validation. `expected_serial` is optional physical evidence; it does not replace or mutate the OCPP identity. | Safe with the same immutable mapping. |
| `POST /v1/remote-commands/stop` | Persist and schedule one RemoteStop command. `202` only after durable HAL command creation/recovery; it is not completion. | Safe with the same idempotency key and immutable body. |
| `GET /v1/remote-commands?cms_command_id={uuid}` | Reconcile a CMS command after a lost response or delayed fact. `200` returns one durable command; `404` means HAL never accepted it. | Safe read. |
| `GET /v1/transactions?cms_start_intent_id={uuid}` | Reconcile a start intent without a "latest" lookup. `200` returns the one authoritative transaction snapshot; `404` means HAL has no materialized transaction for that exact intent and must never be treated as a session. The `transaction` object uses snake_case v1 field names. | Safe read. |
| `GET /v1/transactions/{hal_transaction_id}` | Reconcile a known HAL transaction. `200` returns authoritative HAL state; `404` means HAL does not know it. | Safe read. |

### 6.2 HAL to CMS

| Method and path | Semantics | Retry safety |
| --- | --- | --- |
| `POST /v1/hal-facts` | Deliver one durable versioned HAL protocol fact. CMS returns `204` only after durable receipt/dedupe evidence and processing or safe queuing. | Safe with the same fact ID, immutable digest, and body. |

HAL uses a durable outbox for fact delivery. It retries the same immutable fact
until CMS durably acknowledges it, records terminal delivery state when needed,
and supports reconciliation. Delivery can be duplicate or out of order.
An unexpired delivery lease is exclusive to its current worker. Every lease
claim carries a durable secure claim token, and a delivery transition compares
both fact ID and claim token. After a worker crash, an expired `DELIVERING`
lease becomes reclaimable with a new token as the same durable fact ID, digest,
and payload; a delayed former worker cannot overwrite the newer lease result.
HAL never creates a replacement fact for retry. Delivery is intentionally one
outbound request at a time: its 15-second HTTP timeout remains within the
30-second lease budget.

| Status | Category | Retry rule |
| --- | --- | --- |
| `400` | Malformed envelope, ID, header, timestamp, or integer value | Correct request; do not blind retry. |
| `401` / `403` | Invalid service identity or unauthorized CPO/route scope | Repair credentials/authorization first. |
| `404` | Unknown mapping, command, or HAL transaction | Reconcile; do not guess. |
| `409` | Idempotency mismatch, changed fact content, invalid correlation, unsafe state transition | Stop automation and reconcile. |
| `410` | Expired command, credential, or window | Do not revive automatically. |
| `422` | Semantically invalid transaction, connector, or limit | Correct source state; do not blind retry. |
| `429`, `500`, `502`, `503`, `504` | Transient capacity or transport failure | Retry same identity with observable policy and reconciliation. |

## 7. Command Payload Vocabulary

All timestamps are RFC 3339 UTC. Integer Wh fields are JSON integers, never
floating-point kWh. CMS money, tariff, tax, wallet balance, and hold data are
never sent to HAL.

### 7.1 Start command

```json
{
  "cms_command_id": "1260f4e7-7981-4c20-8c5a-8b830718a004",
  "cms_start_intent_id": "1f1d1300-2057-4f3d-b387-43909f7cd025",
  "cpo_id": "b5b4d85e-da6b-46d3-9a6a-31957654e3b2",
  "customer_id": "cd6c14f8-a98d-458c-a6f2-ba9dbbb1db76",
  "cms_charger_id": "aef9a6c9-0f64-421f-ae0e-b7c9d8cf664f",
  "cms_connector_id": "0e0ecfa0-8f45-4480-9ad2-12f02068d2ce",
  "charger_ocpp_identity": "ocpp_chargepoint_17",
  "ocpp_connector_number": 1,
  "id_tag": "appv1_7N9qK2mP",
  "credential_expires_at": "2026-08-10T12:05:00Z",
  "command_expires_at": "2026-08-10T12:06:00Z",
  "energy_limit_wh": 18000,
  "max_duration_seconds": 3600
}
```

Every shown field is required. HAL validates CPO, CMS charger/connector, OCPP
identity, and connector-number mapping before creating a deliverable command.
`energy_limit_wh` and `max_duration_seconds` are positive protocol-enforcement
instructions, not price or tariff inputs.

### 7.2 Stop command

```json
{
  "cms_command_id": "6120fa0c-8fc7-4fab-9f7c-e410c3df33d4",
  "cms_charging_session_id": "e92c060f-2b3d-487d-9ee3-7f31d5c194d0",
  "cpo_id": "b5b4d85e-da6b-46d3-9a6a-31957654e3b2",
  "cms_charger_id": "aef9a6c9-0f64-421f-ae0e-b7c9d8cf664f",
  "cms_connector_id": "0e0ecfa0-8f45-4480-9ad2-12f02068d2ce",
  "charger_ocpp_identity": "ocpp_chargepoint_17",
  "ocpp_connector_number": 1,
  "hal_transaction_id": "b4b13ecf-37ac-46a7-a620-1e505b59ef16",
  "ocpp_transaction_id": 493829,
  "requested_stop_initiator": "customer",
  "requested_stop_reason": "user_requested",
  "command_expires_at": "2026-08-10T12:35:00Z"
}
```

All shown IDs and expiry are required. Requested initiator/reason are command
evidence, not a claim about the final charger stop reason. HAL rejects a stop
whose exact transaction or mapping does not match durable OCPP state.

### 7.3 Command response and lookup

Every start, stop, and exact-command response wraps this canonical object in
`{"command": ...}`. These are the complete public command fields; store-only
request digests, correlations, credentials, customer data, expiry policy,
delivery internals, and raw error detail are not HTTP contract fields.

```json
{
  "command": {
    "hal_command_id": "e3010bb6-cc38-4df9-99d9-772d79d5ffab",
    "cms_command_id": "1260f4e7-7981-4c20-8c5a-8b830718a004",
    "kind": "START",
    "state": "PENDING_DELIVERY",
    "hal_transaction_id": null,
    "ocpp_transaction_id": null,
    "updated_at": "2026-08-10T12:00:01Z"
  }
}
```

All seven fields are required and use exact snake_case names. The two
transaction fields are nullable until HAL has authoritative transaction truth;
every returned command identity is a nonzero UUID. CMS rejects a successful
HTTP response that omits the wrapper, an identity, `updated_at`, a supported
kind/state, or has mismatched `cms_command_id`. A successful command response
only means durable HAL command acceptance. A lookup may show a later command
state or correlated exact transaction IDs.

## 8. HAL Fact Model

HAL facts are protocol facts, never tariff, GST, wallet, or billing conclusions.

### 8.1 Envelope and fact identity

```json
{
  "fact_id": "c0c7ed9f-5f39-4f7f-9a3d-bd95be4d6eaf",
  "fact_type": "transaction.started",
  "schema_version": 1,
  "occurred_at": "2026-08-10T12:00:17Z",
  "producer": "ocpp-hal-go-new",
  "immutable_content_sha256": "lowercase-hex-sha256",
  "payload": {}
}
```

HAL creates `fact_id` once with the durable immutable fact/outbox record.
`immutable_content_sha256` is SHA-256 of the RFC 8785 JSON Canonical
Serialization of the complete envelope with that field omitted. CMS stores the
fact ID and digest in its durable receipt transaction.

At the durable outbox boundary, HAL normalizes the envelope `occurred_at` to
UTC microsecond precision before both hashing and PostgreSQL persistence. This
is the immutable top-level timestamp delivered on every retry; payload
timestamps retain their own contract-defined precision.

- The same fact ID and digest is an exact duplicate: CMS returns `204` without
  repeating state or financial effects.
- The same fact ID with a different digest is `409 fact_integrity_violation`.
  CMS records audit evidence and does not automatically process altered content.
- A fact body is immutable. Every redelivery uses the identical body and
  `Idempotency-Key: {fact_id}`.

This identity is distinct from CMS business idempotency, CMS-to-HAL command
idempotency, and OCPP retransmission handling.

### 8.2 `transaction.started`

```json
{
  "fact_id": "c0c7ed9f-5f39-4f7f-9a3d-bd95be4d6eaf",
  "fact_type": "transaction.started",
  "schema_version": 1,
  "occurred_at": "2026-08-10T12:00:17Z",
  "producer": "ocpp-hal-go-new",
  "immutable_content_sha256": "lowercase-hex-sha256",
  "payload": {
    "hal_transaction_id": "b4b13ecf-37ac-46a7-a620-1e505b59ef16",
    "ocpp_transaction_id": 493829,
    "hal_command_id": "e3010bb6-cc38-4df9-99d9-772d79d5ffab",
    "cms_command_id": "1260f4e7-7981-4c20-8c5a-8b830718a004",
    "cms_start_intent_id": "1f1d1300-2057-4f3d-b387-43909f7cd025",
    "charger_ocpp_identity": "ocpp_chargepoint_17",
    "ocpp_connector_number": 1,
    "id_tag": "appv1_7N9qK2mP",
    "meter_start_wh": 102345,
    "started_at": "2026-08-10T12:00:16Z"
  }
}
```

Every shown payload field is required for the v1 app-start flow. HAL correlates
the fact to the persisted start command before publishing. CMS validates the
CMS command/start-intent/credential/mapping chain before materializing one
`ACTIVE` session.

`started_at` is the charger-provided OCPP timestamp and remains protocol
evidence. HAL records a separate trusted receipt time internally, uses that
receipt time for credential/command expiry and duration deadlines, and rejects
an OCPP start timestamp outside the bounded clock-skew policy. The envelope
`occurred_at` is the trusted HAL receipt time, not a way for a charger clock to
backdate authorization.

### 8.3 `transaction.completed`

```json
{
  "fact_id": "92d9cc50-38f5-412d-b2b7-5409a6c4b30e",
  "fact_type": "transaction.completed",
  "schema_version": 1,
  "occurred_at": "2026-08-10T12:31:09Z",
  "producer": "ocpp-hal-go-new",
  "immutable_content_sha256": "lowercase-hex-sha256",
  "payload": {
    "hal_transaction_id": "b4b13ecf-37ac-46a7-a620-1e505b59ef16",
    "ocpp_transaction_id": 493829,
    "hal_command_id": "2878aa5c-b0ac-44c5-a76d-19bd65bc0415",
    "cms_command_id": "6120fa0c-8fc7-4fab-9f7c-e410c3df33d4",
    "cms_start_intent_id": "1f1d1300-2057-4f3d-b387-43909f7cd025",
    "charger_ocpp_identity": "ocpp_chargepoint_17",
    "ocpp_connector_number": 1,
    "meter_start_wh": 102345,
    "meter_stop_wh": 119840,
    "raw_meter_stop_wh": 119839,
    "meter_stop_adjustment_wh": 1,
    "meter_stop_evidence": "QUANTIZATION_NORMALIZED",
    "stopped_at": "2026-08-10T12:31:08Z",
    "ocpp_stop_reason": "Local",
    "requested_stop_initiator": "customer",
    "requested_stop_reason": "user_requested"
  }
}
```

`hal_command_id`, `cms_command_id`, and requested-stop fields are nullable when
the charger stopped locally or no stop command correlates. HAL transaction ID,
exact OCPP transaction ID, mapping, effective `meter_stop_wh`, and OCPP
timestamp remain required. New completion facts optionally retain
`raw_meter_stop_wh`, `meter_stop_adjustment_wh`, and `meter_stop_evidence`.
They are present only when HAL has raw evidence from this version; historical
facts remain valid without them and retain `schema_version: 1`. A one-Wh
`QUANTIZATION_NORMALIZED` value means the raw stop was one Wh below the latest
temporally eligible meter sample, so `meter_stop_wh` remains the authoritative
nondecreasing/billable cumulative value.

### 8.4 `command.updated`

HAL publishes this fact when a command reaches `OCPP_ACCEPTED`,
`OCPP_REJECTED`, `AMBIGUOUS`, `EXPIRED`, `SUPERSEDED`, or `MATERIALIZED` and CMS
needs result evidence for recovery. Its payload contains:

```json
{
  "hal_command_id": "e3010bb6-cc38-4df9-99d9-772d79d5ffab",
  "cms_command_id": "1260f4e7-7981-4c20-8c5a-8b830718a004",
  "kind": "START",
  "state": "OCPP_ACCEPTED",
  "charger_ocpp_identity": "ocpp_chargepoint_17",
  "ocpp_connector_number": 1,
  "delivery_attempts": 1,
  "ocpp_result": "Accepted",
  "last_error_category": null,
  "occurred_at": "2026-08-10T12:00:05Z"
}
```

The standard envelope remains required. A command fact never changes start or
completion truth by itself.

### 8.5 `transaction.meter`

`transaction.meter` is required for the first vertical slice whenever HAL
accepts an authoritative usable MeterValues energy sample for an active v1
transaction. It carries the latest cumulative value and never interpolates a
value between charger samples.

```json
{
  "fact_id": "738c642c-ae9c-4cdb-a337-58150977cc77",
  "fact_type": "transaction.meter",
  "schema_version": 1,
  "occurred_at": "2026-08-10T12:08:19Z",
  "producer": "ocpp-hal-go-new",
  "immutable_content_sha256": "lowercase-hex-sha256",
  "payload": {
    "hal_transaction_id": "b4b13ecf-37ac-46a7-a620-1e505b59ef16",
    "ocpp_transaction_id": 493829,
    "cms_start_intent_id": "1f1d1300-2057-4f3d-b387-43909f7cd025",
    "charger_ocpp_identity": "ocpp_chargepoint_17",
    "ocpp_connector_number": 1,
    "meter_sequence": 7,
    "meter_value_wh": 106220,
    "consumed_wh": 3875,
    "meter_observed_at": "2026-08-10T12:08:18Z"
  }
}
```

HAL assigns a monotonically increasing `meter_sequence` per HAL transaction
after it accepts a usable sample. `consumed_wh` is the nonnegative integer
difference from the authoritative `meter_start_wh`; a decreasing/invalid meter
value is not silently projected as valid consumption. CMS stores every accepted
fact receipt, but updates its current operational projection only when the
sequence advances. A delayed older fact must not regress the displayed meter.

### 8.6 `transaction.soc`

`transaction.soc` is an additive immutable fact emitted only when HAL accepts
valid charger-provided OCPP 1.6 `SoC` MeterValues evidence. It is separate from
`transaction.meter`: energy and SoC have independent timestamps, acceptance
rules, and sequences. A SoC-only request creates this fact without inventing an
energy reading; an energy-only request does not repeat cached SoC.

```json
{"fact_type":"transaction.soc","schema_version":1,"payload":{"hal_transaction_id":"uuid","ocpp_transaction_id":493829,"cms_start_intent_id":"uuid","cms_charger_id":"uuid","cms_connector_id":"uuid","charger_ocpp_identity":"ocpp_chargepoint_17","ocpp_connector_number":1,"soc_percent":"63.5","soc_observed_at":"2026-08-10T12:08:18Z","soc_sequence":4}}
```

`soc_percent` is a canonical decimal string in `[0,100]`, retaining up to
three observed decimal places without float equality. HAL accepts `Percent`,
`%`, or a missing unit for standard `SoC`; arbitrary units are rejected.
Missing SoC is unknown, never zero or an estimate. `soc_sequence` increments
only for accepted SoC. Older timestamps cannot regress it; equal timestamps
never create unstable regression.

## 9. Live Operational Projection

### 9.1 Authoritative dimensions

CMS must retain these dimensions separately:

| Dimension | Authority | Required projection fields | Rule |
| --- | --- | --- | --- |
| CMS administrative charger/connector status | CMS | CMS status and its own update time | It is not OCPP availability. |
| Charger connection state | HAL | `ONLINE`, `OFFLINE`, or `UNKNOWN`; observation/change time and sequence; CPO and CMS charger identity; OCPP identity | A stale disconnect must not overwrite a newer connection generation. |
| Connector OCPP status | HAL | Exact OCPP `StatusNotification` status, observation time and sequence, CMS connector ID, connector number, freshness | Do not collapse this into CMS administrative state. |
| Active-session meter progression | HAL | Latest cumulative Wh, consumed Wh, meter observation time, meter sequence, freshness | No fabricated/interpolated sample. |
| Active-session SoC | HAL | First/latest observed percentage, SoC observation time, independent SoC sequence, freshness | Optional; never inferred from energy, capacity, or completion. |

`UNKNOWN` means HAL has no reliable current connection observation. `ONLINE`
means the current registered connection exists. `OFFLINE` means the current
connection is absent after a current-generation disconnect. Connection
generation is a live-HAL-process callback fence, not an ordering value across
process restarts. Startup first projects prior connection state as `UNKNOWN`
and resets the generation baseline; the first legitimate connection in the new
process can therefore establish `ONLINE` immediately. HAL includes the
generation in its durable operational fact record. HAL also assigns a
monotonically increasing `connection_sequence` per OCPP identity; CMS applies a
connection fact only when its sequence advances, so delayed delivery cannot
regress current connection state across reconnects or restarts.

The first connection fact and each accepted OCPP Heartbeat from the current
tracked connection are durable liveness observations. Heartbeat renewal keeps
state `ONLINE` and its existing generation, advances `connection_sequence`, and
publishes `charger.connection.updated` with a new `observed_at`; it never
creates or resurrects a connection. Other charger-originated messages do not
renew this connection fact in v1, preventing MeterValues and status traffic
from amplifying connection-fact volume. The default requested OCPP heartbeat
interval is `OCPP_HEARTBEAT_INTERVAL_SECONDS=300`; CMS connection freshness
must be configured comfortably longer than that cadence and separately from
meter freshness.

When connection is `OFFLINE` or `UNKNOWN`, CMS marks the latest connector status
as `STALE` for live use. It may retain `last_ocpp_status` and its observation
time as historical data, but must not represent that old status as fresh live
truth.

### 9.2 `charger.connection.updated`

```json
{
  "fact_id": "9e6c007f-e6c7-4b71-bd76-930f9e6541f0",
  "fact_type": "charger.connection.updated",
  "schema_version": 1,
  "occurred_at": "2026-08-10T12:01:00Z",
  "producer": "ocpp-hal-go-new",
  "immutable_content_sha256": "lowercase-hex-sha256",
  "payload": {
    "cpo_id": "b5b4d85e-da6b-46d3-9a6a-31957654e3b2",
    "cms_charger_id": "aef9a6c9-0f64-421f-ae0e-b7c9d8cf664f",
    "charger_ocpp_identity": "ocpp_chargepoint_17",
    "connection_state": "ONLINE",
    "connection_generation": 42,
    "connection_sequence": 88,
    "observed_at": "2026-08-10T12:01:00Z"
  }
}
```

Every field is required. HAL publishes an operational fact only for a charger
that maps to exactly one CMS charger and CPO. The connection-generation guard
must make an obsolete same-process disconnect a no-op before it can create an
`OFFLINE` fact; CMS relies on durable `connection_sequence` rather than
generation to order facts across a HAL restart.

### 9.3 `connector.status.updated`

```json
{
  "fact_id": "bc0515a4-46ed-4d99-a2f4-3a5b99a25b39",
  "fact_type": "connector.status.updated",
  "schema_version": 1,
  "occurred_at": "2026-08-10T12:01:05Z",
  "producer": "ocpp-hal-go-new",
  "immutable_content_sha256": "lowercase-hex-sha256",
  "payload": {
    "cpo_id": "b5b4d85e-da6b-46d3-9a6a-31957654e3b2",
    "cms_charger_id": "aef9a6c9-0f64-421f-ae0e-b7c9d8cf664f",
    "cms_connector_id": "0e0ecfa0-8f45-4480-9ad2-12f02068d2ce",
    "charger_ocpp_identity": "ocpp_chargepoint_17",
    "ocpp_connector_number": 1,
    "ocpp_connector_status": "Charging",
    "connector_status_sequence": 19,
    "observed_at": "2026-08-10T12:01:04Z"
  }
}
```

`ocpp_connector_status` preserves the OCPP StatusNotification value; it is not
renamed into a business availability enum. HAL sends the latest authoritative
status notification as an immutable fact and assigns a monotonically increasing
`connector_status_sequence` for each mapped connector. CMS derives live
freshness using the latest accepted sequence, its observation, and current HAL
connection state.

### 9.4 CMS customer polling projection

CMS exposes the durable customer-owned projection; it does not make a blocking
HAL call for a session, map, or discovery read. The first User App surface is:

| Method and path | Semantics |
| --- | --- |
| `GET /api/v1/app/charging-start-intents/{cms_start_intent_id}` | Authenticated owner-only progress for requested, delivery-accepted, protocol-acknowledged, expired, rejected, or reconciliation-required start, with nullable materialized session ID. |
| `GET /api/v1/app/charging-sessions/{cms_charging_session_id}` | Authenticated owner-only durable active/completed session projection, including live operational fields when active. |

The exact User App bearer/CPO scope follows the CMS's existing authenticated
User App boundary. These are CMS routes to implement later, not HAL routes.

```json
{
  "id": "e92c060f-2b3d-487d-9ee3-7f31d5c194d0",
  "start_intent_id": "1f1d1300-2057-4f3d-b387-43909f7cd025",
  "state": "ACTIVE",
  "start_progress": "ACTUALLY_STARTED",
  "stop_progress": null,
  "connection": {
    "state": "ONLINE",
    "observed_at": "2026-08-10T12:01:00Z"
  },
  "connector_live": {
    "last_ocpp_status": "Charging",
    "observed_at": "2026-08-10T12:01:04Z",
    "freshness": "FRESH"
  },
  "meter": {
    "latest_wh": 106220,
    "consumed_wh": 3875,
    "observed_at": "2026-08-10T12:08:18Z",
    "freshness": "FRESH"
  },
  "completed_at": null
}
```

`freshness` is one of `FRESH`, `STALE`, or `UNKNOWN`. CMS defines a documented
staleness threshold for the meter projection during implementation. It must
become `STALE` after that threshold or an offline/unknown connection; it never
creates a fabricated new meter value. The achievable meter freshness is bounded
by charger sample behavior. When a charger safely supports configuration, HAL
should target a MeterValueSampleInterval around 5-10 seconds for this experience,
but this is not an SLA or a capability guarantee.

## 10. Correlation and End-to-End Flows

The exact correlation chain is:

```text
CMS start intent UUID
-> CMS command/idempotency UUID
-> one-use app credential/idTag
-> HAL RemoteCommand UUID
-> HAL transaction UUID
-> exact OCPP transactionId
-> CMS charging-session UUID
```

HAL stores the command-to-credential and command-to-transaction relationship
before publishing a start fact. CMS stores the start-intent-to-command and
start-intent-to-credential relationship before it calls HAL. CMS materializes
the session only from a start fact that proves that complete chain.

### Start

```text
authenticated User App customer
-> CMS trusted scope
-> one atomic tariff/tax snapshot + affordability + hold + start intent + credential
-> POST /v1/remote-commands/start
-> HAL persists RemoteCommand before OCPP RemoteStart
-> RemoteStart response is a separate command result
-> charger StartTransaction
-> HAL allocates exact OCPP transactionId and persists transaction/fact/outbox
-> POST /v1/hal-facts transaction.started
-> CMS durable receipt + chain validation + one ACTIVE session
-> HAL connection/status/meter facts update the CMS operational projection
-> authenticated CMS REST polling returns current operational/session state
```

### Stop and completion

```text
customer stop request or HAL energy-limit enforcement
-> exact HAL/OCPP transaction correlation
-> POST /v1/remote-commands/stop when CMS requests stop
-> HAL persists RemoteCommand before OCPP RemoteStop
-> RemoteStop response is a separate command result
-> charger StopTransaction
-> HAL persists completion fact/outbox
-> POST /v1/hal-facts transaction.completed
-> CMS locked session/hold validation + exactly-once settlement
-> COMPLETED customer session
```

For HAL energy enforcement, no CMS stop command is required where HAL acts on
the already approved persisted energy instruction. HAL still publishes final
protocol evidence and requested initiator/reason where known.

## 11. Tariff, Hold, Settlement, and Energy Invariants

Before CMS submits RemoteStart, one CMS transaction must:

1. derive trusted customer, CPO, charger, and connector scope;
2. resolve effective tariff and active tax/GST;
3. freeze tariff/tax snapshot;
4. use one canonical pricing engine for affordability and permitted-energy/spend
   policy;
5. create a durable wallet hold/reservation preventing concurrent-spend races;
6. create the start intent and one-use credential; and
7. create the CMS command identity used for HAL.

CMS must not derive permitted energy with an independent `balance /
price_per_kWh` shortcut. The canonical forward pricing semantics are authoritative
for affordability, permitted-energy inversion, and final billing. Money remains
exact; billable energy and all cross-service energy instructions use integer Wh.

On valid completion, CMS uses one idempotent transaction to store fact receipt,
lock/validate session state, validate final meter evidence, calculate the final
charge from the frozen snapshot, settle/capture the hold and release unused
value, write exactly one ledger effect, retain immutable completion evidence,
and mark the session completed only after the financial transition succeeds.

A duplicate completion returns success without a second debit. If final energy
exceeds the hold and existing CMS invariants do not support debt or a negative
balance, CMS preserves evidence and enters reconciliation; it does not invent
debt policy.

### 11.1 Energy-limit enforcement

CMS owns commercial policy. HAL enforces approved `energy_limit_wh` against
authoritative OCPP meter progression. HAL persists the limit before RemoteStart.
After StartTransaction establishes `meter_start_wh`, HAL evaluates delivered
energy from that baseline and executes a controlled idempotent RemoteStop
workflow at the approved enforcement threshold.

Sampling and stop latency create physical overshoot. V1 deliberately uses the
first accepted cumulative sample at or above the configured integer-Wh limit;
it does not invent a predictive power guard. HAL records the stop workflow and
actual final meter without clamping. A later predictive guard needs separate
charger-capability evidence. The inherited retry count is not contractual;
controlled, idempotent, observable delivery with terminal/reconciliation
semantics is.

### 11.2 Time-limit enforcement

CMS may include `max_duration_seconds` in the start command. HAL persists it
before command delivery but calculates its deadline only after the
charger-originated `StartTransaction` is durably received by HAL:

```text
trusted HAL start receipt time + max_duration_seconds = stop deadline
```

The deadline is durable, reconstructed after restart, and enters the same
idempotent stop workflow as user, CPO, and energy-limit stops. A RemoteStart
acknowledgement does not start the timer. When a charger-originated
`StopTransaction` arrives first, its actual stop reason remains authoritative.

## 12. Failure and Reconciliation Rules

### 12.0 Inbound evidence inventory

HAL classifies inbound OCPP and transport evidence by what can safely be
reconstructed. A database write failure is therefore not a uniform protocol
failure rule.

| Source | Evidence class | Acknowledgement and recovery rule |
| --- | --- | --- |
| `StartTransaction` | Irreplaceable lifecycle truth | Its accepted lifecycle state and immutable fact commit atomically; persistence failure is not hidden behind a successful protocol result. |
| `StopTransaction` | Irreplaceable lifecycle truth | Its authoritative completion and immutable fact commit atomically; persistence failure is a protocol error. |
| `StatusNotification` | Discrete operational state truth | Persist the mapped observation before confirmation; storage failure is a protocol error. Unmapped/disabled observations are unsupported or un-correlatable and receive a normal confirmation without a projection. |
| `MeterValues` | Active-transaction operational evidence | A known-valid supported sample for an active transaction is persisted before confirmation. Unsupported samples, unknown correlation, duplicates, and stale/regressive readings are normal acknowledgements; only failure to persist a known-valid sample is a protocol error. |
| `Heartbeat` | Repeated, refreshable liveness evidence | Return the normal confirmation even when one liveness renewal cannot persist; log the failure and rely on the next heartbeat or reconnect. Durable state must never be treated as fresh past its own freshness rule. |
| `Authorize` | Credential decision | Validate against durable credential state and fail closed when that state cannot be read; it does not materialize a transaction itself. |
| `BootNotification`, diagnostics, firmware, data transfer | Unsupported/bootstrap/diagnostic evidence in v1 | Use their protocol-specific normal response where no v1 authoritative durable projection exists; do not manufacture transaction/session state. |
| WebSocket open/close | Physical transport event, not an OCPP CALL | Preserve immediate local transport truth, queue the exact failed durable runtime observation for retry, and reset unreconciled durable connection state to `UNKNOWN` on restart. There is no CALL acknowledgement to reject. |

### 12.1 Charger-originated acknowledgement policy

`StartTransaction` and `StopTransaction` are irreplaceable lifecycle truth.
HAL never accepts either as successful lifecycle evidence unless its transaction
state and immutable outbox fact commit together. A failed start is returned as a
non-accepted authorization result; an unpersisted stop is a protocol error.

`StatusNotification` is a discrete mapped connector-state observation. HAL
persists its runtime/fact transaction before its confirmation. A storage error
returns a protocol error; an unmapped or disabled observation is acknowledged
without creating a local or durable projection because it cannot be correlated.

For `MeterValues`, a supported integer-Wh energy register correlated to an
active v1 transaction is persisted before acknowledgement. Store failure is a
protocol error so a valid observation cannot silently vanish. Unsupported
samples, unknown transactions, duplicate values, and stale/regressive evidence
are acknowledged without advancing durable meter state. A one-Wh periodic
rollback is counted as a bounded quantization anomaly; it does not replace the
latest meter or emit a regressive fact. A material rollback remains ignored and
acknowledged.

Heartbeat is repeatable liveness evidence, not a non-reconstructable lifecycle
event. HAL logs a failed renewal but returns the normal confirmation; a later
Heartbeat or reconnect can refresh the same `ONLINE` projection. Connection
open/close are physical transport callbacks rather than OCPP requests: HAL
keeps the local transport truth, retries the same failed durable observation in
process, and startup resets any unreconciled durable connection state to
`UNKNOWN` after a crash.

| Situation | Required behavior |
| --- | --- |
| Duplicate customer/API start | CMS returns the original intent/hold result; no second hold or command. |
| Duplicate CMS-to-HAL command | HAL returns the same `hal_command_id`; no independent OCPP command. |
| Timeout after HAL persistence/delivery | HAL records `AMBIGUOUS`; CMS queries commands/transactions and awaits or replays facts. Neither guesses result. |
| Charger reject | HAL publishes rejection. CMS safely resolves unstarted intent/hold under business policy. |
| Accepted RemoteStart without StartTransaction | Intent remains non-active until its approved window expires or reconciliation proves a transaction. |
| StartTransaction received after credential/command expiry | HAL rejects it using trusted receipt time; a charger timestamp cannot backdate authorization. CMS reconciles any contrary physical/device evidence without inventing a session. |
| Duplicate StartTransaction | HAL reuses the same exact OCPP transaction only where protocol evidence proves retransmission; CMS accepts start fact once. |
| Unknown/stale meter transaction | HAL never attaches it to a latest session; it records/quarantines/reconciles durable OCPP state. |
| Duplicate/out-of-order fact | CMS dedupes ID/hash. Completion before start is retained but not settled until predecessor correlation is recovered. |
| Same fact ID, different immutable content | CMS returns `409 fact_integrity_violation`, audits it, and stops automatic transition. |
| HAL restart with active transaction | HAL restores durable command/transaction/outbox state and conservatively replays/reconciles facts. |
| CMS restart with pending start | CMS restores intent/hold/credential/command/receipt state and queries HAL; it does not recreate work. |
| CMS unavailable for started/completed fact | HAL outbox retries same immutable fact and records delivery state; completion is never discarded. |
| CMS fact receiver returns auth/client/correlation failure | HAL marks the exact fact `RECONCILIATION_REQUIRED`, stops blind retry, and retains its ID/payload/digest. After repair, CMS calls `POST /v1/facts/{fact_id}/requeue` with a canonical correlation UUID; HAL audits the prior state and requeues that same fact only. |
| HAL unavailable to CMS | CMS preserves intent/hold and retries/reconciles same command identity; it does not create a fresh command. |
| RemoteStop ack with no StopTransaction | Session remains active/reconciliation-required. HAL retains controlled stop/recovery awaiting protocol truth. |
| Stale stop retry | HAL revalidates and marks it superseded/prevents dispatch after state advanced. |
| Charger reconnect with open transaction | HAL restores/reconciles transaction truth. CMS never bills/closes from connection status. |
| Stale disconnect | HAL generation guard suppresses it before it changes connection state or marks connector status stale. |
| Delayed/out-of-order connection or connector status fact | CMS stores receipt evidence but applies operational state only when the relevant HAL sequence advances. |
| Offline/unknown charger with old connector state | CMS retains the last OCPP status as historical but marks its freshness `STALE` or `UNKNOWN`; it does not report it as current live truth. |
| Delayed/out-of-order meter fact | CMS retains receipt evidence but updates current meter only when `meter_sequence` advances; no interpolation or regression occurs. |
| Wrong charger/connector/CPO mapping | Reject or quarantine; no command, session materialization, or settlement. |
| CPO suspended during active session | Block new starts only; stop, facts, recovery, final meter, settlement, and history continue. |

Reconciliation is normal operation. CMS uses command and transaction queries
after lost responses, missing facts, restart, or timeout. HAL replays durable
outbox facts. Neither service creates a transaction, session, or financial
effect without the approved identifier chain.

## 13. CPO Lifecycle and Customer State

CPO suspension blocks new charging starts. It does not automatically stop every
charger/session and must not block active-session stop commands, fact ingestion,
recovery, reconciliation, final meter capture, billing/settlement, or completed
history.

CMS REST session state is authoritative for User App state. HAL frontend
WebSockets are not a User App contract. Any later realtime surface is a
CPO/customer-authorized notification/invalidation channel with CMS REST
snapshot/recovery. Discovery/map requests must not synchronously depend on HAL;
live availability is a later explicit slice.

## 14. Production Durability

Production HAL requires durable PostgreSQL. Startup/readiness must fail safely
when PostgreSQL is absent or unavailable; production must not silently fall back
to memory for command, transaction, fact, or outbox state. The memory store is
allowed only for explicit tests and intentional local development.

HAL command, transaction, fact/outbox, retry, and recovery state must survive
restart. CMS start-intent, credential, hold, session, fact receipt, and
settlement state must survive restart. No database is shared.

## 15. First Vertical-Slice Acceptance Criteria

```text
authenticated User App customer
-> CMS start intent, snapshot, hold, and credential
-> durable CMS-to-HAL start command
-> RemoteStart
-> charger StartTransaction
-> durable HAL start fact
-> CMS ACTIVE session
-> live CMS projection of charger connection and connector OCPP status
-> near-live CMS projection of MeterValues/latest and consumed Wh
-> customer stop, CPO stop, energy-limit stop, or time-limit stop
-> durable HAL stop command where applicable
-> RemoteStop
-> charger StopTransaction
-> durable HAL completion fact
-> exactly-once CMS settlement
-> COMPLETED customer session
-> outage/restart reconciliation
```

Verification must cover duplicate requests; lost HTTP responses; accepted
RemoteStart without StartTransaction; duplicate StartTransaction; CMS and HAL
restart; temporary CMS and HAL unavailability; duplicate/out-of-order facts;
RemoteStop accepted without StopTransaction; charger reconnect with open
transaction; CPO suspension during active session; wrong mapping; stale retries
after state advances; stale disconnect generation; connector status while
offline; delayed/out-of-order meter fact; and no fabricated meter interpolation.
A happy-path-only proof is insufficient.

## 16. Physical Identity and Boot Configuration

HAL accepts exactly two WebSocket URL forms: `/{identity}` and
`/{identity}/{serial}`. The first segment is always the canonical OCPP mapping
identity. The optional second segment is physical evidence only. Unknown,
disabled, empty, malformed, or more-than-two-segment paths are rejected. When a
mapping supplies `expected_serial`, a conflicting suffix is rejected; absence
of a suffix remains compatible for existing installations.

BootNotification metadata is retained in HAL as observed evidence and is never
written back to CMS inventory. A serial mismatch remains observable in HAL's
durable boot-evidence projection without rewriting `charger_ocpp_identity`.

After each accepted BootNotification from the current connection generation,
HAL asynchronously reads the configured standard OCPP keys and changes only
mismatches. `HeartbeatInterval` and `MeterValueSampleInterval` are standard
keys. Unsupported, readonly, rejected, and timed-out keys are logged as
individual outcomes and do not prevent other keys from being considered. The
legacy remote-only keys are vendor extensions; they apply only when an explicit
vendor profile and exact vendor gate are configured. A reconnect supersedes the
older generation before later configuration writes.

## 17. Open Decisions

The following remain deliberately not fixed or not yet implemented:

- credential rotation and deployment topology;
- wallet overshoot, debt, or negative-balance policy;
- exact energy stop-guard formula and supporting charger capability evidence;
- RFID credential lifecycle and offline authorization policy;
- generalized customer realtime and discovery/map live-availability projection
  beyond the required active-session polling projection;
- whether historical legacy tables require a separately approved destructive
  retirement migration; and
- detailed future migration/table designs and implementation-level retry
  constants.

No implementation may present an open item as approved architecture.
