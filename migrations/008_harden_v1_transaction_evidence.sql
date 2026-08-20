-- Preserve protocol timestamps as evidence while recording HAL receipt times
-- for expiry, deadlines, and durable ordering. Existing historical rows stay
-- readable; all newly materialized v1 transactions populate these fields.

ALTER TABLE v1_transactions
    ADD COLUMN IF NOT EXISTS observed_started_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS observed_completed_at TIMESTAMPTZ NULL;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM v1_transactions WHERE meter_start_wh < 0) THEN
        RAISE EXCEPTION 'cannot add v1 transaction meter_start_wh constraint: invalid existing rows';
    END IF;
    IF EXISTS (SELECT 1 FROM v1_transactions WHERE ocpp_transaction_id <= 0) THEN
        RAISE EXCEPTION 'cannot add v1 transaction OCPP identity constraint: invalid existing rows';
    END IF;
    IF EXISTS (SELECT 1 FROM v1_transactions WHERE ocpp_connector_number <= 0) THEN
        RAISE EXCEPTION 'cannot add v1 transaction connector constraint: invalid existing rows';
    END IF;
    IF EXISTS (SELECT 1 FROM v1_transactions WHERE meter_stop_wh IS NOT NULL AND meter_stop_wh < meter_start_wh) THEN
        RAISE EXCEPTION 'cannot add v1 transaction completion meter constraint: invalid existing rows';
    END IF;
    IF EXISTS (SELECT 1 FROM v1_transactions WHERE completed_at IS NOT NULL AND completed_at < actual_started_at) THEN
        RAISE EXCEPTION 'cannot add v1 transaction completion time constraint: invalid existing rows';
    END IF;
END $$;

ALTER TABLE v1_transactions
    ADD CONSTRAINT v1_transactions_meter_start_nonnegative CHECK (meter_start_wh >= 0),
    ADD CONSTRAINT v1_transactions_ocpp_transaction_positive CHECK (ocpp_transaction_id > 0),
    ADD CONSTRAINT v1_transactions_connector_positive CHECK (ocpp_connector_number > 0),
    ADD CONSTRAINT v1_transactions_completion_meter_monotonic CHECK (meter_stop_wh IS NULL OR meter_stop_wh >= meter_start_wh),
    ADD CONSTRAINT v1_transactions_completion_after_start CHECK (completed_at IS NULL OR completed_at >= actual_started_at),
    ADD CONSTRAINT v1_transactions_completion_receipt_after_start_receipt CHECK (observed_completed_at IS NULL OR observed_started_at IS NULL OR observed_completed_at >= observed_started_at);

CREATE INDEX IF NOT EXISTS idx_v1_stop_workflows_dispatchable
ON v1_stop_workflows (created_at, hal_transaction_id)
WHERE state = 'PERSISTED';
