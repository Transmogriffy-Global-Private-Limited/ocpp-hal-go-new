-- Durable diagnostic-only coverage for accepted TriggerMessage follow-on windows.
-- It is deliberately separate from charger-operation, transaction, and fact state.
CREATE TABLE v1_trigger_message_follow_on_windows (
    trace_id uuid PRIMARY KEY REFERENCES v1_charging_traces(trace_id) ON DELETE CASCADE,
    expected_message varchar(64) NOT NULL,
    charger_ocpp_identity text NOT NULL,
    ocpp_connector_number integer NOT NULL CHECK (ocpp_connector_number >= 0),
    accepted_at timestamptz NOT NULL,
    deadline_at timestamptz NOT NULL,
    state varchar(16) NOT NULL DEFAULT 'OPEN',
    observed_at timestamptz NULL,
    closed_at timestamptz NULL,
    CHECK (expected_message IN ('BootNotification','DiagnosticsStatusNotification','FirmwareStatusNotification','Heartbeat','MeterValues','StatusNotification')),
    CHECK (deadline_at = accepted_at + INTERVAL '60 seconds'),
    CHECK (state IN ('OPEN','OBSERVED','CLOSED')),
    CHECK ((state = 'OPEN' AND observed_at IS NULL AND closed_at IS NULL) OR (state = 'OBSERVED' AND observed_at IS NOT NULL AND closed_at IS NULL) OR (state = 'CLOSED' AND observed_at IS NULL AND closed_at IS NOT NULL)),
    CHECK (observed_at IS NULL OR (observed_at > accepted_at AND observed_at <= deadline_at)),
    CHECK (closed_at IS NULL OR closed_at >= deadline_at)
);
-- Bounds each inbound OCPP handler lookup to windows that can still contain
-- its observation, rather than scanning historical charger operations. CLOSED
-- is terminal and deliberately excluded: a closure cannot be invalidated.
CREATE INDEX ix_v1_trigger_message_follow_on_match
    ON v1_trigger_message_follow_on_windows (charger_ocpp_identity, expected_message, deadline_at, accepted_at, ocpp_connector_number)
    WHERE state = 'OPEN';
-- Lets the existing diagnostic trace worker close only a bounded due batch.
CREATE INDEX ix_v1_trigger_message_follow_on_due
    ON v1_trigger_message_follow_on_windows (deadline_at, trace_id)
    WHERE state = 'OPEN';
