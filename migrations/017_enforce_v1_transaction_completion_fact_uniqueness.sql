-- A transaction completion is a single terminal fact. It intentionally has
-- no sequence, so the legacy (fact_type, aggregate_key, sequence) unique
-- constraint cannot prevent duplicate NULL-sequence completion rows.
--
-- Keep historical outbox rows immutable: a direct partial unique index could
-- fail on an already-affected deployment. This additive key ledger backfills
-- one canonical existing fact per aggregate and makes every future terminal
-- completion insertion reserve the aggregate before writing its outbox row.
CREATE TABLE IF NOT EXISTS v1_transaction_completion_fact_keys (
    aggregate_key TEXT PRIMARY KEY,
    fact_id UUID NULL UNIQUE REFERENCES v1_fact_outbox(fact_id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO v1_transaction_completion_fact_keys (aggregate_key, fact_id)
SELECT DISTINCT ON (aggregate_key) aggregate_key, fact_id
FROM v1_fact_outbox
WHERE fact_type = 'transaction.completed'
ORDER BY aggregate_key, created_at, fact_id
ON CONFLICT (aggregate_key) DO NOTHING;
