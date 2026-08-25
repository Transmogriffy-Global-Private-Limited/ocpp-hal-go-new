-- Optional charger-observed SoC telemetry. Historical transactions retain
-- NULL evidence; the independent sequence is zero until HAL accepts SoC.
ALTER TABLE v1_transactions
    ADD COLUMN IF NOT EXISTS initial_soc_percent NUMERIC(6,3) NULL,
    ADD COLUMN IF NOT EXISTS latest_soc_percent NUMERIC(6,3) NULL,
    ADD COLUMN IF NOT EXISTS soc_observed_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS soc_sequence BIGINT NOT NULL DEFAULT 0,
    ADD CONSTRAINT chk_v1_transactions_initial_soc_percent
        CHECK (initial_soc_percent IS NULL OR (initial_soc_percent >= 0 AND initial_soc_percent <= 100)),
    ADD CONSTRAINT chk_v1_transactions_latest_soc_percent
        CHECK (latest_soc_percent IS NULL OR (latest_soc_percent >= 0 AND latest_soc_percent <= 100)),
    ADD CONSTRAINT chk_v1_transactions_soc_observation
        CHECK ((latest_soc_percent IS NULL AND soc_observed_at IS NULL) OR (latest_soc_percent IS NOT NULL AND soc_observed_at IS NOT NULL)),
    ADD CONSTRAINT chk_v1_transactions_soc_sequence
        CHECK (soc_sequence >= 0);
