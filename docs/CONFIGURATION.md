# HAL Configuration

`config.Load()` is a startup boundary: a default applies only when a setting is
absent. An explicitly supplied empty or malformed value stops startup before listeners,
workers, or database connections are created. Secrets are never included in
startup errors or logs.

## Environment

`HAL_ENVIRONMENT` accepts only `development` (default), `test`, or
`production`. Only a process `HAL_ENVIRONMENT` chooses the bootstrap mode: a
local `.env` cannot silently make an otherwise development process production.
An explicitly supplied `production` value prevents local `.env` loading. In
development/test, a local `.env` supplies only absent process values; an empty
process value is present, fails validation, and never falls back to `.env`.
Unknown values—including empties and misspellings—fail instead of becoming
development.

## Validated settings

- `F_SERVER_PORT`, `OCPP_LISTEN_PORT`, and `DB_PORT`: `1..65535`.
- `OCPP_HEARTBEAT_INTERVAL_SECONDS`: `1..86400`, default `300`; this is both
  the BootNotification interval and the standard configuration target.
- `OCPP_METER_VALUE_SAMPLE_INTERVAL_SECONDS`: `1..3600`, default `15`; HAL
  requests this standard OCPP key after Boot. CMS's default `30s` meter-display
  freshness window remains compatible with the requested cadence.
- `OCPP_CONFIGURATION_RECONCILE_TIMEOUT_SECONDS`: `1..120`, default `20`;
  bounded asynchronous per-boot configuration reconciliation.
- `OCPP_VENDOR_CONFIGURATION_PROFILE`: empty (default) or `legacy-remote-only`.
  A non-empty profile also requires the exact `OCPP_VENDOR_CONFIGURATION_VENDOR`.
  Vendor keys are never applied to another vendor or inferred from a URL.
- `LOG_LEVEL`: `debug`, `info`, `warn`/`warning`, or `error`; default `info`.
- `HAL_V1_FACT_DELIVERY_ENABLED` and `API_DOCS_ENABLED`: strict Go boolean
  syntax; both default to `false` only when absent.
- `HAL_V1_CMS_FACTS_URL`: absolute `http`/`https` URL without credentials or a
  fragment when fact delivery is enabled.

`HAL_V1_CMS_BEARER_TOKEN` is always required. PostgreSQL is always required:
set a valid absolute `DATABASE_URL` or every structured setting
`DB_NAME`, `DB_USER`, `DB_PASSWORD`, and `DB_HOST` (with `DB_PORT`). When
`HAL_V1_FACT_DELIVERY_ENABLED=true`, both `HAL_V1_CMS_FACTS_URL` and
`HAL_V1_CMS_FACT_BEARER_TOKEN` are also required.

Use `.env.example` as the non-secret local template. A configuration parse is
not a connectivity test; startup still verifies PostgreSQL before serving.
