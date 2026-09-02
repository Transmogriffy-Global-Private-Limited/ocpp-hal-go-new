-- Diagnostic trace delivery is deliberately independent of v1_fact_outbox.
-- Trace rows remain evidence only and must never delay authoritative facts.
CREATE TABLE v1_trace_delivery_outbox (
    event_id uuid PRIMARY KEY REFERENCES v1_charging_trace_events(event_id) ON DELETE CASCADE,
    payload jsonb NOT NULL,
    content_sha256 varchar(64) NOT NULL,
    status varchar(32) NOT NULL DEFAULT 'PENDING',
    retries integer NOT NULL DEFAULT 0,
    next_retry_at timestamptz NOT NULL DEFAULT NOW(),
    claimed_until timestamptz NULL,
    claim_token uuid NULL,
    delivery_status_code integer NULL,
    last_error varchar(500) NULL,
    sent_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    CHECK (status IN ('PENDING','DELIVERING','RETRY','DELIVERED','RECONCILIATION_REQUIRED'))
);
CREATE INDEX ix_v1_trace_delivery_outbox_due
    ON v1_trace_delivery_outbox (next_retry_at, created_at)
    WHERE status IN ('PENDING','RETRY','DELIVERING');
