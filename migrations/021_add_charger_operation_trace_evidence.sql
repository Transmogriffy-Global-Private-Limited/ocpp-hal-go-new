-- Additive operation-root identity for the existing diagnostic trace/outbox.
ALTER TABLE v1_charging_traces
    ADD COLUMN cms_charger_operation_id uuid NULL,
    ADD COLUMN hal_charger_operation_id uuid NULL;
ALTER TABLE v1_charger_operations
    ADD COLUMN trace_id uuid NULL,
    ADD COLUMN configuration_keys text[] NOT NULL DEFAULT '{}';
ALTER TABLE v1_charger_operations
    ADD CONSTRAINT v1_charger_operations_trace_id_unique UNIQUE(trace_id);
ALTER TABLE v1_charger_operations
    DROP CONSTRAINT IF EXISTS v1_charger_operations_kind_check;
ALTER TABLE v1_charger_operations
    ADD CONSTRAINT v1_charger_operations_kind_check
    CHECK (kind IN ('RESET','UNLOCK_CONNECTOR','CHANGE_AVAILABILITY','CLEAR_CACHE','CHANGE_CONFIGURATION','TRIGGER_MESSAGE','GET_CONFIGURATION'));
ALTER TABLE v1_charging_traces
    DROP CONSTRAINT IF EXISTS v1_charging_traces_ocpp_connector_number_check;
ALTER TABLE v1_charging_traces
    ADD CONSTRAINT v1_charging_traces_ocpp_connector_number_check
    CHECK (ocpp_connector_number > 0 OR cms_charger_operation_id IS NOT NULL);
CREATE UNIQUE INDEX uq_v1_charging_trace_cms_charger_operation
    ON v1_charging_traces(cms_charger_operation_id) WHERE cms_charger_operation_id IS NOT NULL;
CREATE UNIQUE INDEX uq_v1_charging_trace_hal_charger_operation
    ON v1_charging_traces(hal_charger_operation_id) WHERE hal_charger_operation_id IS NOT NULL;
CREATE INDEX ix_v1_charging_trace_operation
    ON v1_charging_traces(cpo_id, cms_charger_operation_id)
    WHERE cms_charger_operation_id IS NOT NULL;
