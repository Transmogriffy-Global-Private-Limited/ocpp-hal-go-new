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
- `OCPP_HEARTBEAT_INTERVAL_SECONDS`: `1..86400`, default `300`.
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
