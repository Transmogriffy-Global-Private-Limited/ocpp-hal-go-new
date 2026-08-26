-- The CMS remains the commercial authority. HAL stores this immutable
-- classification only so its existing automatic stop workflows can preserve
-- whether an energy/deadline threshold was enforcing a money request.
ALTER TABLE v1_remote_commands
    ADD COLUMN IF NOT EXISTS limit_type TEXT NOT NULL DEFAULT 'AUTO',
    ADD CONSTRAINT chk_v1_remote_commands_limit_type CHECK (limit_type IN ('AUTO','ENERGY','TIME','MONEY'));

ALTER TABLE v1_transactions
    ADD COLUMN IF NOT EXISTS limit_type TEXT NOT NULL DEFAULT 'AUTO',
    ADD CONSTRAINT chk_v1_transactions_limit_type CHECK (limit_type IN ('AUTO','ENERGY','TIME','MONEY'));

ALTER TABLE v1_stop_workflows
    DROP CONSTRAINT IF EXISTS v1_stop_workflows_requested_stop_initiator_check,
    ADD CONSTRAINT v1_stop_workflows_requested_stop_initiator_check CHECK (requested_stop_initiator IN ('CUSTOMER','CPO','ENERGY_LIMIT','TIME_LIMIT','MONEY_LIMIT','SYSTEM_RECOVERY'));
