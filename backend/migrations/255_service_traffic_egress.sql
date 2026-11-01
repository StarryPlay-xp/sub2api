-- Application payload only. No proxy endpoints, credentials or WG configuration.
CREATE TABLE IF NOT EXISTS service_traffic_egress_buckets (
 runtime_id TEXT NOT NULL REFERENCES service_traffic_runtimes(runtime_id) ON DELETE CASCADE,
 bucket_start TIMESTAMPTZ NOT NULL,
 route_type TEXT NOT NULL CHECK(route_type IN ('direct','proxy','wireguard','unidentified')),
 route_id BIGINT NOT NULL,
 -- 0: route total; -1: missing/overflow account; positive: upstream account ID.
 account_id BIGINT NOT NULL CHECK(account_id>=-1),
 sequence BIGINT NOT NULL,
 upstream_out NUMERIC(30,0) NOT NULL,
 upstream_in NUMERIC(30,0) NOT NULL,
 attempts BIGINT NOT NULL,
 peak_out DOUBLE PRECISION,peak_out_at TIMESTAMPTZ,peak_out_window DOUBLE PRECISION,
 peak_in DOUBLE PRECISION,peak_in_at TIMESTAMPTZ,peak_in_window DOUBLE PRECISION,
 peak_total DOUBLE PRECISION,peak_total_at TIMESTAMPTZ,peak_total_window DOUBLE PRECISION,
 first_sample_at TIMESTAMPTZ,last_sample_at TIMESTAMPTZ,
 valid_samples BIGINT NOT NULL,invalid_samples BIGINT NOT NULL,
 overflow BOOLEAN NOT NULL DEFAULT FALSE,
 PRIMARY KEY(runtime_id,bucket_start,route_type,route_id,account_id)
);
CREATE INDEX IF NOT EXISTS service_traffic_egress_time_idx ON service_traffic_egress_buckets(bucket_start,account_id);
CREATE INDEX IF NOT EXISTS service_traffic_egress_route_idx ON service_traffic_egress_buckets(route_type,route_id,bucket_start);
CREATE TABLE IF NOT EXISTS service_traffic_egress_metadata (
 id BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK(id),started_at TIMESTAMPTZ NOT NULL
);
