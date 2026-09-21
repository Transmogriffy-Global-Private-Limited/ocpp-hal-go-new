-- Bounded restart/reconnect recovery scans only definitely unattempted rows.
CREATE INDEX idx_v1_charger_operations_dispatchable
    ON v1_charger_operations(charger_ocpp_identity, created_at, cms_operation_id)
    WHERE state = 'PERSISTED';
