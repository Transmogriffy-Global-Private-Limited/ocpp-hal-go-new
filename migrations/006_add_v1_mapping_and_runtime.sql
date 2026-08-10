-- Additive v1 mapping and command-audit state. CMS remains authoritative for
-- business mapping; these tables are only HAL-owned integration projections.

CREATE TABLE IF NOT EXISTS v1_charger_mappings (
    cms_charger_id UUID PRIMARY KEY,
    cpo_id UUID NOT NULL,
    charger_ocpp_identity TEXT NOT NULL UNIQUE,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (cpo_id, cms_charger_id)
);

CREATE TABLE IF NOT EXISTS v1_connector_mappings (
    cms_connector_id UUID PRIMARY KEY,
    cpo_id UUID NOT NULL,
    cms_charger_id UUID NOT NULL,
    charger_ocpp_identity TEXT NOT NULL,
    ocpp_connector_number INTEGER NOT NULL CHECK (ocpp_connector_number >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (charger_ocpp_identity, ocpp_connector_number),
    UNIQUE (cpo_id, cms_connector_id),
    FOREIGN KEY (cpo_id, cms_charger_id)
        REFERENCES v1_charger_mappings (cpo_id, cms_charger_id)
);

CREATE TABLE IF NOT EXISTS v1_mapping_audit (
    id UUID PRIMARY KEY,
    cpo_id UUID NOT NULL,
    cms_charger_id UUID NOT NULL,
    correlation_id TEXT NOT NULL,
    request_digest TEXT NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('ENROLLED', 'ENABLEMENT_CHANGED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE v1_remote_commands
    ADD COLUMN IF NOT EXISTS customer_id UUID NULL,
    ADD COLUMN IF NOT EXISTS correlation_id TEXT NULL;

ALTER TABLE v1_transactions
    ADD COLUMN IF NOT EXISTS customer_id UUID NULL;

CREATE INDEX IF NOT EXISTS idx_v1_runtime_mapping_identity
ON v1_charger_mappings (charger_ocpp_identity);
