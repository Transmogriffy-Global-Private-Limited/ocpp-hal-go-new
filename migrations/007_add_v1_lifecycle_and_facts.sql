-- Complete the HAL-owned v1 lifecycle without creating a CMS database surface.
-- Commands record their pre-network delivery attempt; stop coordination is one
-- transaction-level workflow regardless of whether the trigger is CMS or HAL.

CREATE TABLE IF NOT EXISTS v1_stop_workflows (
    hal_transaction_id UUID PRIMARY KEY REFERENCES v1_transactions(hal_transaction_id),
    requested_stop_initiator TEXT NOT NULL CHECK (requested_stop_initiator IN ('CUSTOMER','CPO','ENERGY_LIMIT','TIME_LIMIT','SYSTEM_RECOVERY')),
    requested_stop_reason TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('PERSISTED','PENDING_DELIVERY','DELIVERY_ATTEMPTED','OCPP_ACCEPTED','OCPP_REJECTED','AMBIGUOUS','COMPLETED','SUPERSEDED')),
    delivery_attempts INTEGER NOT NULL DEFAULT 0,
    last_ocpp_result TEXT NULL,
    last_error_category TEXT NULL,
    last_error_detail TEXT NULL,
    claimed_until TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ NULL
);

ALTER TABLE v1_remote_commands
    ADD COLUMN IF NOT EXISTS claimed_until TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS stop_workflow_transaction_id UUID NULL REFERENCES v1_stop_workflows(hal_transaction_id);

ALTER TABLE v1_fact_outbox
    ADD COLUMN IF NOT EXISTS schema_version INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS producer TEXT NOT NULL DEFAULT 'ocpp-hal-go-new',
    ADD COLUMN IF NOT EXISTS claimed_until TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS delivery_status_code INTEGER NULL;

CREATE INDEX IF NOT EXISTS idx_v1_stop_workflows_due
ON v1_stop_workflows (state, updated_at)
WHERE state IN ('PERSISTED', 'PENDING_DELIVERY', 'DELIVERY_ATTEMPTED', 'OCPP_ACCEPTED', 'AMBIGUOUS');

CREATE INDEX IF NOT EXISTS idx_v1_fact_outbox_claim
ON v1_fact_outbox (status, next_retry_at, claimed_until);
