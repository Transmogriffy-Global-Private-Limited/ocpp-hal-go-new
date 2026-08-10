# OCPP HAL for ev-cms-backend-new

This is the independent OCPP 1.6 HAL for `ev-cms-backend-new`. It owns OCPP
transport, charger connection/status, charger command delivery, exact
charger-originated transaction truth, and raw meter facts. CMS owns customer
identity, commercial policy, settlement, and customer/CPO projections.

The legacy `OCPPHAL_Go` repository and old CMS relationship are separate and
untouched. This runtime does not register legacy CMS `/api/*` compatibility,
callbacks, frontend WebSockets, external charger-directory access, or
single-session routing.

## Runtime

PostgreSQL and `HAL_V1_CMS_BEARER_TOKEN` are required. Charger connection is
allowed only for an enabled v1 mapping. CMS uses the authenticated v1 command,
runtime, and reconciliation routes; HAL sends immutable facts to the configured
authenticated `/v1/hal-facts` receiver.

`cpconsole` is the retained OCPP-native virtual charge point. Use
`scripts/build-all.ps1` and `scripts/regression-local.ps1` from any checkout
directory; both derive the repository root from their own location.

Read [docs/README.md](docs/README.md) for the contract, architecture, audit,
and verification map. Do not commit, push, deploy, or modify the legacy or CMS
repositories without explicit human permission.
