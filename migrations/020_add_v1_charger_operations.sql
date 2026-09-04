-- Dedicated control-command ledger. It intentionally never shares the
-- commercial Start/Stop remote-command lifecycle.
CREATE TABLE v1_charger_operations (
    id UUID PRIMARY KEY,
    cms_operation_id UUID NOT NULL UNIQUE,
    request_digest CHAR(64) NOT NULL,
    cpo_id UUID NOT NULL,
    cms_charger_id UUID NOT NULL,
    cms_connector_id UUID NULL,
    charger_ocpp_identity TEXT NOT NULL,
    ocpp_connector_number INTEGER NOT NULL CHECK (ocpp_connector_number >= 0),
    kind TEXT NOT NULL CHECK (kind IN ('RESET','UNLOCK_CONNECTOR','CHANGE_AVAILABILITY','CLEAR_CACHE','CHANGE_CONFIGURATION','TRIGGER_MESSAGE')),
    parameters JSONB NOT NULL,
    correlation_id TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('PERSISTED','DELIVERY_ATTEMPTED','OCPP_CONFIRMED','RECONCILIATION_REQUIRED')),
    delivery_attempts INTEGER NOT NULL DEFAULT 0,
    ocpp_result TEXT NULL,
    error_category TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ NULL
);
CREATE INDEX idx_v1_charger_operations_cpo_created ON v1_charger_operations(cpo_id, created_at DESC);
CREATE INDEX idx_v1_charger_operations_reconciliation ON v1_charger_operations(state, updated_at) WHERE state='RECONCILIATION_REQUIRED';
