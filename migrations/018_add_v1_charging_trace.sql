CREATE TABLE v1_charging_traces (
    trace_id uuid PRIMARY KEY,
    cpo_id uuid NOT NULL,
    cms_start_intent_id uuid NULL,
    cms_charging_session_id uuid NULL,
    cms_command_id uuid NULL,
    hal_transaction_id uuid NULL,
    ocpp_transaction_id bigint NULL,
    charger_ocpp_identity text NOT NULL,
    ocpp_connector_number integer NOT NULL CHECK (ocpp_connector_number > 0),
    created_at timestamptz NOT NULL DEFAULT NOW(),
    CHECK (ocpp_transaction_id IS NULL OR ocpp_transaction_id > 0)
);

CREATE UNIQUE INDEX uq_v1_charging_trace_cms_start_intent
    ON v1_charging_traces (cms_start_intent_id) WHERE cms_start_intent_id IS NOT NULL;
CREATE UNIQUE INDEX uq_v1_charging_trace_hal_transaction
    ON v1_charging_traces (hal_transaction_id) WHERE hal_transaction_id IS NOT NULL;
CREATE INDEX ix_v1_charging_trace_connector
    ON v1_charging_traces (charger_ocpp_identity, ocpp_connector_number, created_at DESC);
CREATE INDEX ix_v1_charging_trace_retention
    ON v1_charging_traces (created_at);

CREATE TABLE v1_charging_trace_events (
    event_id uuid PRIMARY KEY,
    trace_id uuid NOT NULL REFERENCES v1_charging_traces(trace_id) ON DELETE CASCADE,
    source varchar(32) NOT NULL,
    target varchar(32) NOT NULL,
    category varchar(48) NOT NULL,
    protocol varchar(24) NOT NULL,
    phase varchar(24) NOT NULL,
    summary varchar(200) NOT NULL,
    occurred_at timestamptz NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT NOW(),
    state_before varchar(64) NOT NULL DEFAULT '',
    state_after varchar(64) NOT NULL DEFAULT '',
    correlation_id varchar(128) NOT NULL DEFAULT '',
    data jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX ix_v1_charging_trace_events_cursor
    ON v1_charging_trace_events (trace_id, occurred_at DESC, event_id DESC);
