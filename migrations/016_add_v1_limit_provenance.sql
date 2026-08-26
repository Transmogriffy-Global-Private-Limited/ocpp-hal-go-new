-- A customer-selected execution boundary and a wallet-safety boundary are
-- independent facts. Preserve source per physical dimension so HAL reports
-- why it stopped without interpreting tariff or wallet policy itself.
ALTER TABLE v1_remote_commands
    ADD COLUMN IF NOT EXISTS energy_limit_source TEXT NOT NULL DEFAULT 'NONE',
    ADD COLUMN IF NOT EXISTS duration_limit_source TEXT NOT NULL DEFAULT 'NONE';

ALTER TABLE v1_transactions
    ADD COLUMN IF NOT EXISTS energy_limit_source TEXT NOT NULL DEFAULT 'NONE',
    ADD COLUMN IF NOT EXISTS duration_limit_source TEXT NOT NULL DEFAULT 'NONE';

UPDATE v1_remote_commands
SET energy_limit_source = CASE
        WHEN energy_limit_wh IS NULL OR energy_limit_wh = 0 THEN 'NONE'
        WHEN limit_type = 'ENERGY' THEN 'CUSTOMER_ENERGY'
        WHEN limit_type = 'MONEY' THEN 'CUSTOMER_MONEY'
        WHEN limit_type = 'AUTO' THEN 'WALLET'
        ELSE 'NONE'
    END,
    duration_limit_source = CASE
        WHEN max_duration_seconds IS NULL OR max_duration_seconds = 0 THEN 'NONE'
        WHEN limit_type = 'TIME' THEN 'CUSTOMER_TIME'
        WHEN limit_type = 'MONEY' THEN 'CUSTOMER_MONEY'
        WHEN limit_type = 'AUTO' THEN 'WALLET'
        ELSE 'NONE'
    END;

UPDATE v1_transactions
SET energy_limit_source = CASE
        WHEN energy_limit_wh IS NULL OR energy_limit_wh = 0 THEN 'NONE'
        WHEN limit_type = 'ENERGY' THEN 'CUSTOMER_ENERGY'
        WHEN limit_type = 'MONEY' THEN 'CUSTOMER_MONEY'
        WHEN limit_type = 'AUTO' THEN 'WALLET'
        ELSE 'NONE'
    END,
    duration_limit_source = CASE
        WHEN max_duration_seconds IS NULL OR max_duration_seconds = 0 THEN 'NONE'
        WHEN limit_type = 'TIME' THEN 'CUSTOMER_TIME'
        WHEN limit_type = 'MONEY' THEN 'CUSTOMER_MONEY'
        WHEN limit_type = 'AUTO' THEN 'WALLET'
        ELSE 'NONE'
    END;

ALTER TABLE v1_remote_commands
    ADD CONSTRAINT chk_v1_remote_commands_energy_limit_source CHECK (energy_limit_source IN ('NONE','CUSTOMER_ENERGY','CUSTOMER_TIME','CUSTOMER_MONEY','WALLET')),
    ADD CONSTRAINT chk_v1_remote_commands_duration_limit_source CHECK (duration_limit_source IN ('NONE','CUSTOMER_ENERGY','CUSTOMER_TIME','CUSTOMER_MONEY','WALLET')),
    ADD CONSTRAINT chk_v1_remote_commands_energy_limit_provenance CHECK ((energy_limit_wh IS NULL AND energy_limit_source = 'NONE') OR (energy_limit_wh > 0 AND energy_limit_source IN ('CUSTOMER_ENERGY','CUSTOMER_MONEY','WALLET'))),
    ADD CONSTRAINT chk_v1_remote_commands_duration_limit_provenance CHECK ((max_duration_seconds IS NULL AND duration_limit_source = 'NONE') OR (max_duration_seconds > 0 AND duration_limit_source IN ('CUSTOMER_TIME','CUSTOMER_MONEY','WALLET')));

ALTER TABLE v1_transactions
    ADD CONSTRAINT chk_v1_transactions_energy_limit_source CHECK (energy_limit_source IN ('NONE','CUSTOMER_ENERGY','CUSTOMER_TIME','CUSTOMER_MONEY','WALLET')),
    ADD CONSTRAINT chk_v1_transactions_duration_limit_source CHECK (duration_limit_source IN ('NONE','CUSTOMER_ENERGY','CUSTOMER_TIME','CUSTOMER_MONEY','WALLET')),
    ADD CONSTRAINT chk_v1_transactions_energy_limit_provenance CHECK ((energy_limit_wh IS NULL AND energy_limit_source = 'NONE') OR (energy_limit_wh > 0 AND energy_limit_source IN ('CUSTOMER_ENERGY','CUSTOMER_MONEY','WALLET'))),
    ADD CONSTRAINT chk_v1_transactions_duration_limit_provenance CHECK ((max_duration_seconds IS NULL AND duration_limit_source = 'NONE') OR (max_duration_seconds > 0 AND duration_limit_source IN ('CUSTOMER_TIME','CUSTOMER_MONEY','WALLET')));

ALTER TABLE v1_stop_workflows
    DROP CONSTRAINT IF EXISTS v1_stop_workflows_requested_stop_initiator_check,
    ADD CONSTRAINT v1_stop_workflows_requested_stop_initiator_check CHECK (requested_stop_initiator IN ('CUSTOMER','CPO','ENERGY_LIMIT','TIME_LIMIT','MONEY_LIMIT','WALLET_LIMIT','SYSTEM_RECOVERY'));
