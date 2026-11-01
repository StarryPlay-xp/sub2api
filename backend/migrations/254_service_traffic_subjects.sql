-- Attribution is additive and intentionally has no FK to mutable/deleted identities.
CREATE TABLE IF NOT EXISTS service_traffic_subject_buckets (
 runtime_id TEXT NOT NULL REFERENCES service_traffic_runtimes(runtime_id) ON DELETE CASCADE,
 bucket_start TIMESTAMPTZ NOT NULL,
 subject_type TEXT NOT NULL CHECK(subject_type IN ('user','api_key','unattributed')),
 subject_id BIGINT NOT NULL,
 user_id BIGINT NOT NULL,
 sequence BIGINT NOT NULL,
 client_in NUMERIC(30,0) NOT NULL,
 client_out NUMERIC(30,0) NOT NULL,
 upstream_out NUMERIC(30,0) NOT NULL,
 upstream_in NUMERIC(30,0) NOT NULL,
 requests BIGINT NOT NULL,
 active_seconds BIGINT NOT NULL,
 peak_in DOUBLE PRECISION, peak_in_at TIMESTAMPTZ, peak_in_window DOUBLE PRECISION,
 peak_out DOUBLE PRECISION, peak_out_at TIMESTAMPTZ, peak_out_window DOUBLE PRECISION,
 peak_total DOUBLE PRECISION, peak_total_at TIMESTAMPTZ, peak_total_window DOUBLE PRECISION,
 first_sample_at TIMESTAMPTZ, last_sample_at TIMESTAMPTZ,
 valid_samples BIGINT NOT NULL, invalid_samples BIGINT NOT NULL,
 subject_overflow BOOLEAN NOT NULL DEFAULT FALSE,
 PRIMARY KEY(runtime_id,bucket_start,subject_type,subject_id)
);
CREATE INDEX IF NOT EXISTS service_traffic_subject_time_idx ON service_traffic_subject_buckets(bucket_start,subject_type);
CREATE INDEX IF NOT EXISTS service_traffic_subject_identity_idx ON service_traffic_subject_buckets(subject_type,subject_id,bucket_start);
-- Preserve the start of attribution across 90-day pruning and process restarts.
CREATE TABLE IF NOT EXISTS service_traffic_attribution_metadata (
 id BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK(id), started_at TIMESTAMPTZ NOT NULL
);
