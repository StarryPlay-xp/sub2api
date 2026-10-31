-- Usage and operations records may carry a prefixed local/client request ID.
-- Keep alias lookup indexed as attempt history grows.
CREATE INDEX IF NOT EXISTS upstream_egress_attempts_correlation_idx
    ON upstream_egress_attempts USING GIN (correlation_ids);
