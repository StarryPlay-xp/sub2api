-- WireGuard secrets are separate from ordinary proxy exports and scheduler data.
CREATE TABLE IF NOT EXISTS wireguard_nodes (
    proxy_id BIGINT PRIMARY KEY REFERENCES proxies(id) ON DELETE RESTRICT,
    config_ciphertext TEXT NOT NULL,
    config_version BIGINT NOT NULL DEFAULT 1 CHECK (config_version > 0),
    public_summary JSONB NOT NULL,
    paused BOOLEAN NOT NULL DEFAULT FALSE,
    pause_reason TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Revision survives WG detach/reattach and invalidates account connection pools.
CREATE TABLE IF NOT EXISTS account_egress_revisions (
    account_id BIGINT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    revision BIGINT NOT NULL DEFAULT 1
);

-- Durable fail-closed intent survives a missing proxy object/cache entry.
CREATE TABLE IF NOT EXISTS account_wireguard_bindings (
    account_id BIGINT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    proxy_id BIGINT NOT NULL REFERENCES wireguard_nodes(proxy_id) ON DELETE RESTRICT,
    revision BIGINT NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS wireguard_traffic_totals (
    proxy_id BIGINT PRIMARY KEY REFERENCES wireguard_nodes(proxy_id) ON DELETE RESTRICT,
    tx_bytes NUMERIC(20,0) NOT NULL DEFAULT 0 CHECK(tx_bytes >= 0),
    rx_bytes NUMERIC(20,0) NOT NULL DEFAULT 0 CHECK(rx_bytes >= 0),
    saved_at TIMESTAMPTZ,
    incomplete BOOLEAN NOT NULL DEFAULT FALSE,
    metric_basis TEXT NOT NULL DEFAULT 'wg_udp_payload_v1'
);
CREATE TABLE IF NOT EXISTS wireguard_traffic_runtimes (
    runtime_id TEXT PRIMARY KEY,
    proxy_id BIGINT NOT NULL REFERENCES wireguard_nodes(proxy_id) ON DELETE RESTRICT,
    config_version BIGINT NOT NULL,
    sequence BIGINT NOT NULL DEFAULT -1,
    tx_bytes NUMERIC(20,0) NOT NULL DEFAULT 0,
    rx_bytes NUMERIC(20,0) NOT NULL DEFAULT 0,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    saved_at TIMESTAMPTZ,
    closed BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE TABLE IF NOT EXISTS wireguard_traffic_buckets (
    runtime_id TEXT NOT NULL REFERENCES wireguard_traffic_runtimes(runtime_id) ON DELETE CASCADE,
    bucket_start TIMESTAMPTZ NOT NULL,
    tx_bytes NUMERIC(20,0) NOT NULL CHECK(tx_bytes >= 0),
    rx_bytes NUMERIC(20,0) NOT NULL CHECK(rx_bytes >= 0),
    PRIMARY KEY(runtime_id,bucket_start)
);
CREATE TABLE IF NOT EXISTS wireguard_traffic_hourly (
    proxy_id BIGINT NOT NULL REFERENCES wireguard_nodes(proxy_id) ON DELETE RESTRICT,
    bucket_start TIMESTAMPTZ NOT NULL,
    tx_bytes NUMERIC(20,0) NOT NULL DEFAULT 0,
    rx_bytes NUMERIC(20,0) NOT NULL DEFAULT 0,
    PRIMARY KEY(proxy_id,bucket_start)
);
CREATE TABLE IF NOT EXISTS egress_observations (
    id BIGSERIAL PRIMARY KEY,
    route_key TEXT NOT NULL,
    family TEXT NOT NULL CHECK(family IN ('ipv4','ipv6')),
    snapshot JSONB NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS egress_observations_route_idx ON egress_observations(route_key,family,observed_at DESC);
CREATE TABLE IF NOT EXISTS upstream_egress_attempts (
    attempt_id TEXT PRIMARY KEY,
    request_id TEXT,
    correlation_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    account_id BIGINT,
    route_key TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    snapshot JSONB NOT NULL,
    outcome TEXT NOT NULL DEFAULT 'started'
);
CREATE INDEX IF NOT EXISTS upstream_egress_attempts_request_idx ON upstream_egress_attempts(request_id,started_at);
CREATE INDEX IF NOT EXISTS upstream_egress_attempts_time_idx ON upstream_egress_attempts(started_at);

CREATE OR REPLACE FUNCTION guard_wireguard_proxy() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND (OLD.protocol = 'wireguard' OR NEW.protocol = 'wireguard') AND NEW.protocol <> OLD.protocol THEN
        RAISE EXCEPTION 'WireGuard node protocol is immutable';
    END IF;
    IF NEW.protocol = 'wireguard' AND (NEW.fallback_mode <> 'none' OR NEW.backup_proxy_id IS NOT NULL
        OR COALESCE(NEW.username,'') <> '' OR COALESCE(NEW.password,'') <> '') THEN
        RAISE EXCEPTION 'WireGuard does not support fallback or proxy credentials';
    END IF;
    IF NEW.backup_proxy_id IS NOT NULL AND EXISTS(SELECT 1 FROM proxies WHERE id=NEW.backup_proxy_id AND protocol='wireguard') THEN
        RAISE EXCEPTION 'WireGuard cannot be selected by proxy fallback';
    END IF;
    IF NEW.deleted_at IS NOT NULL AND EXISTS(SELECT 1 FROM account_wireguard_bindings WHERE proxy_id=NEW.id) THEN
        RAISE EXCEPTION 'WireGuard node is in use';
    END IF;
    RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS wireguard_proxy_guard ON proxies;
CREATE TRIGGER wireguard_proxy_guard BEFORE INSERT OR UPDATE ON proxies FOR EACH ROW EXECUTE FUNCTION guard_wireguard_proxy();

CREATE OR REPLACE FUNCTION guard_account_wireguard_binding() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE old_node BIGINT; new_protocol TEXT; previous_explicit TEXT;
BEGIN
    IF TG_OP='INSERT' OR NEW.proxy_id IS DISTINCT FROM OLD.proxy_id THEN
        INSERT INTO account_egress_revisions(account_id,revision) VALUES(NEW.id,1)
        ON CONFLICT(account_id) DO UPDATE SET revision=account_egress_revisions.revision+1;
    END IF;
    SELECT proxy_id INTO old_node FROM account_wireguard_bindings WHERE account_id=NEW.id;
    IF old_node IS NOT NULL AND NEW.proxy_id IS DISTINCT FROM old_node
       AND current_setting('sub2api.explicit_egress_change', TRUE) IS DISTINCT FROM NEW.id::TEXT
       AND NOT (COALESCE(NULLIF(current_setting('sub2api.explicit_egress_accounts', TRUE),''),'[]')::jsonb ? NEW.id::TEXT)
       AND (NEW.parent_account_id IS NULL OR current_setting('sub2api.explicit_egress_change', TRUE) IS DISTINCT FROM NEW.parent_account_id::TEXT) THEN
        RAISE EXCEPTION 'WireGuard binding requires an explicit administrator egress change';
    END IF;
    IF NEW.proxy_id IS NOT NULL THEN
        SELECT protocol INTO new_protocol FROM proxies WHERE id=NEW.proxy_id AND deleted_at IS NULL FOR SHARE;
        IF new_protocol IS NULL THEN
            RAISE EXCEPTION 'Selected egress node is missing';
        END IF;
        IF new_protocol='wireguard' THEN
            IF NEW.platform='anthropic' AND NEW.type IN ('oauth','setup-token') AND COALESCE(NEW.extra->>'custom_base_url_enabled','false')='true' THEN
                RAISE EXCEPTION 'WireGuard cannot be combined with an external Anthropic relay';
            END IF;
            IF NEW.platform='openai' AND NEW.type='oauth' AND EXISTS(SELECT 1 FROM sub2api_plugin_bindings WHERE enabled AND rollout_percent>0 AND capability='openai.oauth.outbound_transport.v1') THEN
                RAISE EXCEPTION 'Disable the external OpenAI OAuth transport plugin before binding WireGuard';
            END IF;
            IF NOT EXISTS(SELECT 1 FROM wireguard_nodes WHERE proxy_id=NEW.proxy_id) THEN
                RAISE EXCEPTION 'WireGuard configuration is missing';
            END IF;
            INSERT INTO account_wireguard_bindings(account_id,proxy_id) VALUES(NEW.id,NEW.proxy_id)
            ON CONFLICT(account_id) DO UPDATE SET proxy_id=EXCLUDED.proxy_id,
                revision=account_wireguard_bindings.revision+CASE WHEN account_wireguard_bindings.proxy_id<>EXCLUDED.proxy_id THEN 1 ELSE 0 END;
        ELSIF old_node IS NOT NULL THEN
            DELETE FROM account_wireguard_bindings WHERE account_id=NEW.id;
        END IF;
    ELSIF old_node IS NOT NULL THEN
        DELETE FROM account_wireguard_bindings WHERE account_id=NEW.id;
    END IF;
    IF (old_node IS NOT NULL OR new_protocol='wireguard') AND NEW.parent_account_id IS NOT NULL
       AND NOT EXISTS(SELECT 1 FROM accounts WHERE id=NEW.parent_account_id AND proxy_id IS NOT DISTINCT FROM NEW.proxy_id) THEN
        RAISE EXCEPTION 'WireGuard shadow binding must follow its parent';
    END IF;
    IF NEW.parent_account_id IS NULL AND (old_node IS NOT NULL OR new_protocol='wireguard') THEN
        -- Parent and shadows change atomically. The existing service propagation
        -- still refreshes their scheduler snapshots after this transaction.
        previous_explicit := current_setting('sub2api.explicit_egress_change',TRUE);
        PERFORM set_config('sub2api.explicit_egress_change',NEW.id::TEXT,true);
        UPDATE accounts SET proxy_id=NEW.proxy_id,updated_at=NOW()
        WHERE parent_account_id=NEW.id AND deleted_at IS NULL AND proxy_id IS DISTINCT FROM NEW.proxy_id;
        PERFORM set_config('sub2api.explicit_egress_change',COALESCE(previous_explicit,''),true);
    END IF;
    RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS account_wireguard_binding_guard ON accounts;
CREATE TRIGGER account_wireguard_binding_guard AFTER INSERT OR UPDATE OF proxy_id ON accounts
FOR EACH ROW EXECUTE FUNCTION guard_account_wireguard_binding();

-- Only one application process may own a node at a time. The local watchdog
-- stops packet I/O after 10s without renewal; takeover requires 30s expiry.
CREATE TABLE IF NOT EXISTS wireguard_runtime_leases (
    proxy_id BIGINT PRIMARY KEY REFERENCES wireguard_nodes(proxy_id) ON DELETE RESTRICT,
    runtime_id TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE OR REPLACE FUNCTION guard_wireguard_plugin_binding() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.enabled AND NEW.rollout_percent>0 AND NEW.capability='openai.oauth.outbound_transport.v1'
       AND EXISTS(SELECT 1 FROM accounts a JOIN account_wireguard_bindings b ON b.account_id=a.id WHERE a.deleted_at IS NULL AND a.platform='openai' AND a.type='oauth') THEN
        RAISE EXCEPTION 'External OpenAI OAuth transport cannot be enabled while OAuth accounts use WireGuard';
    END IF;
    RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS wireguard_plugin_guard ON sub2api_plugin_bindings;
CREATE TRIGGER wireguard_plugin_guard BEFORE INSERT OR UPDATE ON sub2api_plugin_bindings FOR EACH ROW EXECUTE FUNCTION guard_wireguard_plugin_binding();
