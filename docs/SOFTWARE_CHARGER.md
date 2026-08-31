# Terminal-controlled OCPP 1.6J virtual charger

## Purpose and boundary

`cpconsole` is a standalone software EV charger. It behaves as one OCPP 1.6J
Charge Point with one outbound WebSocket and one or more independently modeled
connectors. It does not run inside the HAL server and does not call the HAL
database or CMS directly.

```text
terminal commands or HAL remote commands
→ cpconsole charge-point state machine
→ OCPP 1.6J WebSocket messages
→ this HAL (or another OCPP 1.6J Central System)
→ durable HAL v1 transaction and fact flow
→ authenticated CMS fact receiver
```

The simulator is useful only when connected to a Central System. Running it
without one would create local pretend state but would not test any real
integration, so connection and an accepted `BootNotification` are startup
requirements.

## Charger identity and target URL

Set the charge point identity with `-id`. `ocpp-go` appends that identity to the
base URL, matching the HAL's `/{charger_id}` WebSocket route.

```powershell
.\builds\cpconsole.exe -id CP-SIM-001 -url ws://127.0.0.1:18081
```

For a hosted TLS endpoint:

```powershell
.\builds\cpconsole.exe -id CP-SIM-001 -url wss://ocpp.example.com
```

The ID must be enrolled as an enabled v1 mapping in the target HAL. An unmapped
or disabled identity is rejected during the WebSocket handshake, exactly as
unknown hardware should be.

The URL must be an absolute `ws://` or `wss://` base URL and may include a base
path. Do not include the charger ID, query string, or fragment; the selected
`-id` is appended as the final path segment. A trailing slash is normalized.

Supported flags:

| Flag | Environment fallback | Default | Meaning |
| --- | --- | --- | --- |
| `-url` | `CP_SIM_URL` | `ws://127.0.0.1:18081` | Central System WebSocket base URL. |
| `-id` | `CP_SIM_ID` | `CP-SIM-001` | OCPP charge point identity. |
| `-model` | `CP_SIM_MODEL` | `TransEV-Simulator` | Boot model (1–20 characters). |
| `-vendor` | `CP_SIM_VENDOR` | `TransEV` | Boot vendor (1–20 characters). |
| `-connectors` | `CP_SIM_CONNECTORS` | `1` | Number of connectors on the one simulated charge point. Must be at least one. |
| `-connector` | `CP_SIM_CONNECTOR` | `1` | Initial selected connector ID for terminal/startup automation. It must be within `1..connectors`; this retains the original one-connector default. |
| `-meter-start-wh` | `CP_SIM_METER_START_WH` | `100000` | Initial cumulative energy register. |
| `-voltage` | `CP_SIM_VOLTAGE` | `230` | Voltage used for coherent current calculation. |
| `-soc` | `CP_SIM_SOC` | `35` | Initial EV state of charge. |
| `-heartbeat-interval` | `CP_SIM_HEARTBEAT_INTERVAL` | `0` | Simulator Heartbeat cadence in seconds. `0` honors the accepted `BootNotification.conf.interval`; a positive value is an explicit override. |
| `-auto-start-id-tag` | `CP_SIM_AUTO_START_ID_TAG` | empty | Run one normal local session after boot using this idTag. |
| `-auto-power-kw` | `CP_SIM_AUTO_POWER_KW` | `7.2` | Constant power used by configured transaction-bound automatic metering. |
| `-auto-meter-interval` | `CP_SIM_AUTO_METER_INTERVAL` | `0` | Automatic MeterValues cadence in seconds after every successful local or remote transaction. `0` disables it. |

Flags take precedence over environment values. These variables are simulator
client settings; they are not consumed by the HAL server.

`cpconsole` starts automatic Heartbeats only after an accepted BootNotification.
It prints both the server-requested interval and, when configured, the explicit
effective override. A valid Central System `ChangeConfiguration` for
`HeartbeatInterval` reschedules the live worker when no override is set. An
explicit `-heartbeat-interval` or `CP_SIM_HEARTBEAT_INTERVAL` wins for the
process lifetime, so that remote configuration request is rejected rather than
silently reporting one cadence while using another.

## Build and run on Windows

```powershell
Set-Location C:\path\to\ocpp-hal-go-new
.\scripts\build-all.ps1
.\builds\cpconsole.exe -id CP-SIM-001 -url ws://127.0.0.1:18081
```

Hosted development examples:

```powershell
# Manual charger with the server-requested Heartbeat cadence.
go run ./cmd/cpconsole -url wss://dev-ocpphal-new.transev.site -id bd9099 -connectors 2 -connector 1

# Manual charger with a 60-second simulator Heartbeat override.
go run ./cmd/cpconsole -url wss://dev-ocpphal-new.transev.site -id bd9099 -connectors 2 -connector 1 -heartbeat-interval 60

# One realistic local session, then retain the interactive cp> prompt.
go run ./cmd/cpconsole -url wss://dev-ocpphal-new.transev.site -id bd9099 -connectors 2 -connector 1 -heartbeat-interval 60 -auto-start-id-tag USER001 -auto-power-kw 7.2 -auto-meter-interval 10
```

## Optional SoC acceptance

`cpconsole` already puts the configured `-soc` value in each MeterValues
sample as OCPP `SoC` with `Percent`. For a disposable CMS/HAL topology, start
with `-soc 35`, begin a CMS-controlled session, and send `tick 60 7.2`.
Verify HAL transaction `initial_soc_percent`, `latest_soc_percent`,
`soc_observed_at`, and `soc_sequence`; then verify the durable
`transaction.soc` fact, CMS charging-session projection, customer session
detail, and history. After another tick, latest SoC may advance while initial
SoC remains unchanged. After `stop`, final/history SoC must remain the last
actual SoC timestamp, never the stop timestamp. A charger that sends no `SoC`
remains supported and exposes unknown/absent SoC rather than `0`.

## Build for Linux VPS2 from Windows

For the usual x86-64 VPS:

```powershell
Set-Location C:\path\to\ocpp-hal-go-new
.\scripts\build-cpconsole-linux.ps1 -Architecture amd64
```

For an ARM64 VPS, use `-Architecture arm64`. The output is ignored by Git:

```text
builds/cpconsole-linux-amd64
builds/cpconsole-linux-arm64
```

Copy the applicable file to VPS2, then run:

```text
chmod 700 ./cpconsole-linux-amd64
./cpconsole-linux-amd64 -id CP-SIM-001 -url wss://ocpp.example.com
```

Alternatively, build directly on Linux from the repository:

```text
go build -trimpath -o ./builds/cpconsole ./cmd/cpconsole
./builds/cpconsole -id CP-SIM-001 -url wss://ocpp.example.com
```

No database, CMS credentials, or inbound public listener is required by the
simulator. VPS2 only needs outbound access to the selected WebSocket endpoint.

## Normal charging flow

Startup automatically performs:

```text
WebSocket connect
→ one BootNotification (must be Accepted)
→ StatusNotification Available for each configured connector (1 through N)
→ terminal prompt
```

A realistic locally initiated session is:

```text
plug
→ StatusNotification Preparing

start USER001
→ Authorize USER001
→ StartTransaction with cumulative meterStart in Wh
→ server-assigned transactionId retained exactly
→ StatusNotification Charging

tick 60 7.2
→ add 120 Wh (7.2 kW × 60 seconds)
→ MeterValues with the exact transactionId
→ cumulative energy, power, current, voltage and SoC samples

stop Local
→ StopTransaction with the same transactionId and cumulative meterStop
→ StatusNotification Finishing

unplug
→ StatusNotification Available
```

Each connector retains its own fractional internal meter, transaction, SoC,
status, pending remote action, and automatic meter worker. `StartTransaction`,
every `MeterValues` sample, and `StopTransaction` for that connector all use
the same finite, non-negative, rounded integer-Wh conversion. The same physical
register therefore cannot be rounded for a periodic sample and truncated for
StopTransaction.

`tick` never invents unrelated readings. The simulator calculates:

- energy delta from power multiplied by elapsed time;
- current from power divided by configured voltage;
- SoC from a documented 60 kWh reference battery;
- consumed session energy from cumulative `meterStop - meterStart`.

The energy register is monotonic and is transmitted in Wh. Transaction IDs are
assigned by the Central System and never fabricated by the simulator.

When `-auto-start-id-tag` is present, cpconsole performs this exact flow once
after boot reports `Available`: `plug`, `Authorize`, `StartTransaction`, and
`StatusNotification Charging`. It retains the Central-System-assigned
transaction ID. If `-auto-meter-interval` is positive, it starts the same
periodic meter worker used by `auto <seconds> <power-kW>` only after that start
has succeeded. The configured worker also starts for normal terminal and
accepted remote starts, is bound to that exact connector transaction, and uses
actual elapsed time between samples. A failed startup step is printed with its exact phase and leaves
the terminal prompt usable; automatic startup never retries after a stop or
failure.

## Remote CMS flow

By default, valid `RemoteStartTransaction` and `RemoteStopTransaction` requests
are accepted and executed after their confirmation is returned. This models a
normal connected charger for the authenticated v1 remote-command boundary.

For manual failure and timing tests:

```text
policy auto-remote off
policy remote-start accept
```

The next valid remote start is acknowledged and retained as pending. Inspect it
with `state`, then run `remote-start`. The equivalent command is `remote-stop`.
Future remote commands may be rejected deliberately with:

```text
policy remote-start reject
policy remote-stop reject
```

For `RemoteStartTransaction`, an explicit connector must exist and be eligible.
With no connector supplied, cpconsole chooses the lowest eligible connector in
`Available` or `Preparing` state. A remote start never creates another WebSocket
or uses a connector with an active/pending transaction. `RemoteStopTransaction`
searches the configured connector states by exact server-assigned transaction
ID, stops only that owner, cancels only its meter worker, reports `Finishing`,
then returns it to `Available` (or scheduled `Unavailable`). Other connectors
continue unchanged.

## Terminal command reference

Run `help` inside the program for the canonical compact command list.

- `use <connector>` selects the connector used by all connector-local commands.
- `state` prints charge-point connection/boot/policy state; `state all` prints
  every connector's separate status, meter, transaction, pending command, and
  automatic-meter state. `state <connector>` prints just that connector.
- `heartbeat` sends an on-demand OCPP heartbeat while automatic Heartbeats run.
- `plug`, `unplug`, `finish`, `suspend ev|evse`, and `resume` model normal states.
- `authorize`, `start`, `tick`, `meter`, and `stop` drive the transaction lifecycle.
- `auto <seconds> <power-kW>` sends periodic coherent readings for the selected
  connector; `auto off` stops only that connector's worker.
- Automatic metering pauses without adding energy while a manual command leaves
  the connector outside `Charging` (for example `suspend ev` or `fault`), and
  resumes only when the same state machine returns to `Charging`.
- `fault <error-code>` and `clear-fault` test valid OCPP fault behavior.
- `status <ocpp-status>` is an explicit escape hatch for protocol edge testing.
- `policy ...` controls responses and automatic execution of remote commands.
- `quit` closes the client cleanly.

Invalid state transitions are rejected for normal commands. The explicit
`status` command intentionally permits raw valid OCPP statuses so a tester can
exercise unusual Central System behavior.

## Other HAL commands

The virtual charger handles the active HAL remote-command surface:

- ChangeAvailability, including `Scheduled` while a transaction is active;
- ChangeConfiguration and GetConfiguration using in-memory charger configuration;
- ClearCache, UnlockConnector, Reset and DataTransfer;
- GetDiagnostics with coherent Uploading then Uploaded notifications;
- UpdateFirmware with Downloading, Downloaded, Installing and Installed notifications;
- TriggerMessage for BootNotification, Heartbeat, StatusNotification and MeterValues.

Configuration and pending actions intentionally live only for the lifetime of
the simulator process; durable business truth remains in the Central System.

## Failure, restart, and recovery semantics

- If the WebSocket or BootNotification fails, startup exits non-zero.
- OCPP request errors are printed and do not fabricate success.
- The underlying library does not automatically reconnect. Restart `cpconsole`
  to model charger reconnection and boot recovery.
- An open transaction is not locally persisted across simulator process exit.
  This is deliberate: HAL recovery behavior should be tested by keeping the
  durable transaction in HAL and reconnecting the same charger ID, then using
  the HAL's recovery commands.
- `Ctrl+C` or `quit` stops the client. Use an explicit `stop` first when the test
  requires a normal completed transaction and HAL fact delivery.
- Automatic Heartbeats continue while the terminal waits at `cp>`. A Heartbeat
  failure is printed and the worker continues so transient transport errors do
  not invent success or terminate manual control. `quit`, Ctrl+C, `auto off`,
  and normal transaction stop cancel the applicable worker; shutdown waits for
  active simulator workers before closing the Charge Point.

## Verification

Focused compile/test:

```powershell
go test .\cmd\cpconsole
go build -o .\builds\cpconsole.exe .\cmd\cpconsole
.\scripts\build-cpconsole-linux.ps1 -Architecture amd64
```

Full repository verification:

```powershell
.\scripts\build-all.ps1
.\scripts\regression-local.ps1 -SkipBuild
```

The complete PostgreSQL fact-receiver lifecycle test is deliberately enabled by
`regression-local.ps1`. It consumes a shared durable outbox and therefore is
not run during a concurrent package-wide `go test ./...` pass.

For an end-to-end manual check, enroll an enabled mapping for the selected
charger ID, run the HAL, start `cpconsole`, execute the normal charging flow,
then verify the HAL v1 transaction/runtime rows and immutable fact delivery.
