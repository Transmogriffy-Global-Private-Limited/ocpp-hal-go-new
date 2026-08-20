-- A delivery result belongs only to the lease that issued it. Reclaimed facts
-- receive a new runtime-generated token, so a delayed former worker cannot
-- overwrite the newer worker's durable delivery state.
ALTER TABLE v1_fact_outbox
    ADD COLUMN IF NOT EXISTS claim_token UUID NULL;

CREATE INDEX IF NOT EXISTS idx_v1_fact_outbox_claim_token
    ON v1_fact_outbox (fact_id, claim_token)
    WHERE claim_token IS NOT NULL;
