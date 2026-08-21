-- New completion writes retain exact charger evidence separately from the
-- monotonic authoritative stop meter. Historical raw evidence is unknown and
-- deliberately remains NULL.
ALTER TABLE v1_transactions
    ADD COLUMN IF NOT EXISTS raw_meter_stop_wh BIGINT NULL,
    ADD COLUMN IF NOT EXISTS meter_stop_adjustment_wh BIGINT NULL,
    ADD COLUMN IF NOT EXISTS meter_stop_evidence TEXT NULL,
    ADD COLUMN IF NOT EXISTS meter_quantization_anomaly_count BIGINT NOT NULL DEFAULT 0;

ALTER TABLE v1_transactions
    ADD CONSTRAINT v1_transactions_raw_stop_meter_nonnegative
        CHECK (raw_meter_stop_wh IS NULL OR raw_meter_stop_wh >= 0),
    ADD CONSTRAINT v1_transactions_stop_meter_adjustment_range
        CHECK (meter_stop_adjustment_wh IS NULL OR meter_stop_adjustment_wh BETWEEN 0 AND 1),
    ADD CONSTRAINT v1_transactions_stop_meter_evidence_known
        CHECK (meter_stop_evidence IS NULL OR meter_stop_evidence IN ('EXACT', 'FORWARD', 'QUANTIZATION_NORMALIZED')),
    ADD CONSTRAINT v1_transactions_stop_meter_raw_effective_consistent
        CHECK (raw_meter_stop_wh IS NULL OR (meter_stop_wh IS NOT NULL AND meter_stop_adjustment_wh IS NOT NULL AND meter_stop_wh = raw_meter_stop_wh + meter_stop_adjustment_wh));
