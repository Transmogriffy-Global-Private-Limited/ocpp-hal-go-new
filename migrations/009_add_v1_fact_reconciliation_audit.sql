-- Preserve the immutable fact itself while recording each explicit operator
-- recovery request. A repaired CMS credential may requeue only an existing
-- RECONCILIATION_REQUIRED fact; it never creates a replacement fact.

CREATE TABLE IF NOT EXISTS v1_fact_reconciliation_audit (
    id UUID PRIMARY KEY,
    fact_id UUID NOT NULL REFERENCES v1_fact_outbox(fact_id),
    correlation_id UUID NOT NULL,
    previous_status TEXT NOT NULL CHECK (previous_status = 'RECONCILIATION_REQUIRED'),
    previous_error TEXT NULL,
    requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_v1_fact_reconciliation_audit_fact
ON v1_fact_reconciliation_audit (fact_id, requested_at);
