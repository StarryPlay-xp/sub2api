-- Deleted accounts retain fail-closed binding intent and traffic history, but
-- no longer keep a node in use. Active shadows still count independently.
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
    IF NEW.deleted_at IS NOT NULL AND EXISTS(
        SELECT 1 FROM account_wireguard_bindings b
        JOIN accounts a ON a.id=b.account_id
        WHERE b.proxy_id=NEW.id AND a.deleted_at IS NULL
    ) THEN
        RAISE EXCEPTION 'WireGuard node is in use';
    END IF;
    RETURN NEW;
END $$;

-- Restoring an account must revalidate its retained egress (including a node
-- deleted while the account was inactive), rather than silently reviving it.
DROP TRIGGER IF EXISTS account_wireguard_restore_guard ON accounts;
CREATE TRIGGER account_wireguard_restore_guard
AFTER UPDATE OF deleted_at ON accounts
FOR EACH ROW WHEN (OLD.deleted_at IS NOT NULL AND NEW.deleted_at IS NULL)
EXECUTE FUNCTION guard_account_wireguard_binding();
