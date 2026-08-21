-- Hardware metadata is an observed HAL record. It intentionally has no write
-- path back into CMS charger inventory.
CREATE TABLE IF NOT EXISTS v1_charger_boot_evidence (
    charger_ocpp_identity TEXT PRIMARY KEY REFERENCES v1_charger_mappings(charger_ocpp_identity) ON DELETE RESTRICT,
    path_serial TEXT NULL,
    charge_box_serial_number TEXT NULL,
    charge_point_serial_number TEXT NULL,
    charge_point_vendor TEXT NULL,
    charge_point_model TEXT NULL,
    firmware_version TEXT NULL,
    serial_conflict BOOLEAN NOT NULL DEFAULT FALSE,
    observed_at TIMESTAMPTZ NOT NULL
);
