-- Additive v1 HAL state. These tables are HAL-owned and are never a CMS
-- database integration surface.

CREATE TABLE IF NOT EXISTS v1_remote_commands (
    id UUID PRIMARY KEY,
    cms_command_id UUID NOT NULL UNIQUE,
    kind TEXT NOT NULL CHECK (kind IN ('START', 'STOP')),
    request_digest TEXT NOT NULL,
    cpo_id UUID NOT NULL,
    cms_charger_id UUID NOT NULL,
    cms_connector_id UUID NOT NULL,
    charger_ocpp_identity TEXT NOT NULL,
    ocpp_connector_number INTEGER NOT NULL CHECK (ocpp_connector_number >= 0),
    cms_start_intent_id UUID NULL,
    cms_charging_session_id UUID NULL,
    id_tag TEXT NULL,
    credential_expires_at TIMESTAMPTZ NULL,
    command_expires_at TIMESTAMPTZ NOT NULL,
    energy_limit_wh BIGINT NULL CHECK (energy_limit_wh > 0),
    max_duration_seconds BIGINT NULL CHECK (max_duration_seconds > 0),
    requested_stop_initiator TEXT NULL,
    requested_stop_reason TEXT NULL,
    hal_transaction_id UUID NULL,
    ocpp_transaction_id BIGINT NULL,
    state TEXT NOT NULL,
    delivery_attempts INTEGER NOT NULL DEFAULT 0,
    last_ocpp_result TEXT NULL,
    last_error_category TEXT NULL,
    last_error_detail TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_v1_remote_commands_start_intent
ON v1_remote_commands (cms_start_intent_id)
WHERE kind = 'START';

CREATE INDEX IF NOT EXISTS idx_v1_remote_commands_dispatch
ON v1_remote_commands (state, command_expires_at, created_at);

CREATE TABLE IF NOT EXISTS v1_start_credentials (
    id_tag TEXT PRIMARY KEY,
    cms_start_intent_id UUID NOT NULL UNIQUE,
    cpo_id UUID NOT NULL,
    cms_charger_id UUID NOT NULL,
    cms_connector_id UUID NOT NULL,
    charger_ocpp_identity TEXT NOT NULL,
    ocpp_connector_number INTEGER NOT NULL CHECK (ocpp_connector_number >= 0),
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ NULL,
    hal_transaction_id UUID NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_v1_start_credentials_lookup
ON v1_start_credentials (charger_ocpp_identity, ocpp_connector_number, expires_at);

CREATE TABLE IF NOT EXISTS v1_transactions (
    hal_transaction_id UUID PRIMARY KEY,
    cms_start_intent_id UUID NOT NULL UNIQUE,
    cms_command_id UUID NOT NULL UNIQUE REFERENCES v1_remote_commands(cms_command_id),
    cpo_id UUID NOT NULL,
    cms_charger_id UUID NOT NULL,
    cms_connector_id UUID NOT NULL,
    charger_ocpp_identity TEXT NOT NULL,
    ocpp_connector_number INTEGER NOT NULL CHECK (ocpp_connector_number >= 0),
    id_tag TEXT NOT NULL,
    ocpp_transaction_id BIGINT NOT NULL UNIQUE,
    actual_started_at TIMESTAMPTZ NOT NULL,
    meter_start_wh BIGINT NOT NULL,
    latest_meter_wh BIGINT NULL,
    meter_observed_at TIMESTAMPTZ NULL,
    meter_sequence BIGINT NOT NULL DEFAULT 0,
    energy_limit_wh BIGINT NULL CHECK (energy_limit_wh > 0),
    max_duration_seconds BIGINT NULL CHECK (max_duration_seconds > 0),
    stop_deadline_at TIMESTAMPTZ NULL,
    stop_state TEXT NOT NULL DEFAULT 'NONE',
    requested_stop_initiator TEXT NULL,
    requested_stop_reason TEXT NULL,
    ocpp_stop_reason TEXT NULL,
    completed_at TIMESTAMPTZ NULL,
    meter_stop_wh BIGINT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_v1_transactions_active
ON v1_transactions (charger_ocpp_identity, ocpp_connector_number, actual_started_at)
WHERE completed_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_v1_transactions_stop_deadline
ON v1_transactions (stop_deadline_at)
WHERE completed_at IS NULL AND stop_deadline_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS v1_charger_runtime (
    charger_ocpp_identity TEXT PRIMARY KEY,
    connection_state TEXT NOT NULL CHECK (connection_state IN ('ONLINE', 'OFFLINE', 'UNKNOWN')),
    connection_generation BIGINT NOT NULL DEFAULT 0,
    connection_sequence BIGINT NOT NULL DEFAULT 0,
    connected_at TIMESTAMPTZ NULL,
    last_observed_at TIMESTAMPTZ NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS v1_connector_runtime (
    charger_ocpp_identity TEXT NOT NULL,
    ocpp_connector_number INTEGER NOT NULL,
    status TEXT NULL,
    error_code TEXT NULL,
    info TEXT NULL,
    vendor_id TEXT NULL,
    vendor_error_code TEXT NULL,
    observed_at TIMESTAMPTZ NULL,
    status_sequence BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (charger_ocpp_identity, ocpp_connector_number)
);

CREATE TABLE IF NOT EXISTS v1_fact_outbox (
    fact_id UUID PRIMARY KEY,
    fact_type TEXT NOT NULL,
    aggregate_key TEXT NOT NULL,
    sequence BIGINT NULL,
    payload JSONB NOT NULL,
    content_digest TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',
    retries INTEGER NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMPTZ NULL,
    UNIQUE (fact_type, aggregate_key, sequence)
);

CREATE INDEX IF NOT EXISTS idx_v1_fact_outbox_due
ON v1_fact_outbox (status, next_retry_at, created_at);
