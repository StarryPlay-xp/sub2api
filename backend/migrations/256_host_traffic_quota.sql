-- Single-host counters and monthly quota. No external agent or registration.
CREATE TABLE IF NOT EXISTS host_traffic_state (
 id SMALLINT PRIMARY KEY CHECK (id=1), data JSONB NOT NULL
);
INSERT INTO host_traffic_state(id,data) VALUES(1,'{}') ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS host_traffic_samples (
 boot_id TEXT NOT NULL, interface_name TEXT NOT NULL, ifindex INTEGER NOT NULL,
 sampled_at TIMESTAMPTZ NOT NULL, data JSONB NOT NULL,
 PRIMARY KEY(boot_id,interface_name,ifindex,sampled_at)
);
CREATE INDEX IF NOT EXISTS host_traffic_samples_retention ON host_traffic_samples(sampled_at);
-- Hour containers retain minute differences and indivisible recovery spans.
-- Keeping differences permits 15-minute / timezone boundary recalculation.
CREATE TABLE IF NOT EXISTS host_traffic_hours (
 hour TIMESTAMPTZ NOT NULL, interface_name TEXT NOT NULL, spans JSONB NOT NULL,
 PRIMARY KEY(hour,interface_name)
);
CREATE TABLE IF NOT EXISTS host_traffic_calibrations (
 id BIGSERIAL PRIMARY KEY, at TIMESTAMPTZ NOT NULL, period_start TIMESTAMPTZ NOT NULL,
 accounting_key TEXT NOT NULL, adjustment TEXT, data JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS host_traffic_calibrations_period ON host_traffic_calibrations(period_start,at DESC);
CREATE TABLE IF NOT EXISTS host_traffic_periods (
 period_start TIMESTAMPTZ NOT NULL, period_end TIMESTAMPTZ NOT NULL, data JSONB NOT NULL,
 PRIMARY KEY(period_start,period_end)
);
ALTER TABLE ops_alert_rules ADD COLUMN IF NOT EXISTS managed_by VARCHAR(64);
CREATE UNIQUE INDEX IF NOT EXISTS ops_alert_rules_managed_host_traffic
 ON ops_alert_rules(managed_by) WHERE managed_by IS NOT NULL;
