-- Additive only: existing WG totals and historical migration checksums stay intact.
ALTER TABLE wireguard_traffic_buckets ADD COLUMN IF NOT EXISTS peak_data JSONB;
ALTER TABLE wireguard_traffic_hourly ADD COLUMN IF NOT EXISTS peak_data JSONB;
CREATE TABLE IF NOT EXISTS service_traffic_runtimes (
 runtime_id TEXT PRIMARY KEY,
 started_at TIMESTAMPTZ NOT NULL,
 saved_at TIMESTAMPTZ NOT NULL,
 sequence BIGINT NOT NULL,
 closed BOOLEAN NOT NULL DEFAULT FALSE,
 incomplete BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE TABLE IF NOT EXISTS service_traffic_buckets (
 runtime_id TEXT NOT NULL REFERENCES service_traffic_runtimes(runtime_id) ON DELETE CASCADE,
 interval TEXT NOT NULL CHECK (interval IN ('minute','hour')),
 bucket_start TIMESTAMPTZ NOT NULL,
 sequence BIGINT NOT NULL,
 data JSONB NOT NULL,
 PRIMARY KEY(runtime_id,interval,bucket_start)
);
CREATE INDEX IF NOT EXISTS service_traffic_buckets_time_idx ON service_traffic_buckets(interval,bucket_start);
