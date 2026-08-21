-- CMS inventory remains authoritative. The optional serial is only physical
-- admission evidence and can be absent for installed chargers.
ALTER TABLE v1_charger_mappings
    ADD COLUMN IF NOT EXISTS expected_serial TEXT NULL;

ALTER TABLE v1_charger_mappings
    DROP CONSTRAINT IF EXISTS chk_v1_charger_mappings_expected_serial;
ALTER TABLE v1_charger_mappings
    ADD CONSTRAINT chk_v1_charger_mappings_expected_serial
        CHECK (expected_serial IS NULL OR (char_length(expected_serial) BETWEEN 1 AND 100 AND expected_serial !~ '[[:space:]/]'));
