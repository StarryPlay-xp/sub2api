import json
from pathlib import Path
import tempfile
import threading
import unittest
from unittest.mock import patch

import updater


class HostUpdaterTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.host = updater.Updater.__new__(updater.Updater)
        self.host.state_dir = Path(self.temp.name)
        self.host.directory = Path(self.temp.name)
        self.host.project = "sub2api"
        self.host.compose_files = [Path(self.temp.name) / "compose.yaml"]
        self.host.env_file = Path(self.temp.name) / ".env"
        self.host.override = Path(self.temp.name) / "compose.image-version.json"
        self.host.mutex = threading.RLock()
        self.host.job = {"enabled": True, "id": "job", "phase": "queued", "version": "1.2.4", "nodes": []}
        self.host.latest = lambda: "1.2.4"
        self.app = {"Id": "app", "Image": "old-image", "Config": {"Image": "old:1.2.3", "Env": ["DATABASE_HOST=postgres"]},
                    "Mounts": [{"Type": "bind", "Destination": "/app/data", "Source": "/opt/sub2api/data", "RW": True}],
                    "State": {"Running": True, "ExitCode": 0, "Health": {"Status": "healthy"}}}
        self.pg = {"Id": "pg"}
        self.redis = {"Id": "redis"}
        self.host.preflight = lambda: (self.app, self.pg, self.redis, [], True)
        self.host.check_compose = lambda app: None
        self.host.snapshot = lambda pg: {"accounts": "4:unchanged", "wireguard_nodes": "1:unchanged"}
        self.backups = []

        def backup(directory, *args):
            self.backups.append(directory)
            directory.mkdir()
        self.host.backup = backup
        self.calls = []
        self.started_new = False
        self.host.container = self.container
        self.host.sql = lambda pg, sql: "f" if "EXISTS" in sql else "7"
        phase = self.host.phase

        def complete_verification(name, **extra):
            phase(name, **extra)
            if name == "verifying":
                self.host.verify({"id": "job", "version": "1.2.4", "ok": True})
        self.host.phase = complete_verification

    def container(self, name):
        if name == "postgres":
            return self.pg
        if name == "redis":
            return self.redis
        app = dict(self.app)
        if self.started_new:
            app["Image"] = "new-image"
        return app

    def command(self, args, **kwargs):
        self.calls.append(args)
        if "-version" in args:
            return b"Sub2API 1.2.3 (commit: test)"
        if args[:3] == ["docker", "image", "inspect"]:
            return json.dumps([{"Id": "new-image", "RepoDigests": [updater.IMAGE + "@sha256:abc"], "Config": {"Labels": {
                "org.opencontainers.image.version": "1.2.4", "org.opencontainers.image.source": "https://github.com/" + updater.REPO}}}]).encode()
        if args[:2] == ["docker", "inspect"]:
            return json.dumps([self.app]).encode()
        if "up" in args:
            self.started_new = True
        return b""

    def test_happy_path_only_recreates_app_and_pins_digest(self):
        with patch.object(updater, "command", side_effect=self.command), patch.object(updater, "trusted_file", side_effect=Path):
            self.host.run("1.2.4")
        self.assertEqual(self.host.status()["phase"], "succeeded")
        self.assertEqual(self.host.status()["nodes"], [7])
        self.assertEqual(len(self.backups), 1)
        up = next(c for c in self.calls if "up" in c)
        self.assertEqual(up[-6:], ["up", "-d", "--no-deps", "--pull", "never", "sub2api"])
        self.assertIn("@sha256:", json.loads(self.host.override.read_text())["services"]["sub2api"]["image"])
        self.assertFalse(any("down" in c or "rm" in c for c in self.calls))

    def test_unreleased_version_never_stops_service(self):
        with patch.object(updater, "command", side_effect=self.command):
            self.host.run("1.2.5")
        self.assertEqual(self.host.status()["phase"], "failed")
        self.assertFalse(self.calls)

    def test_failed_backup_restarts_old_app_without_recreation(self):
        self.host.backup = lambda *args: (_ for _ in ()).throw(updater.UpdateError("Backup failed"))
        with patch.object(updater, "command", side_effect=self.command):
            self.host.run("1.2.4")
        self.assertEqual(self.host.status()["phase"], "needs_attention")
        self.assertTrue(any("start" in c for c in self.calls))
        self.assertFalse(self.started_new)

    def test_failed_migrated_app_does_not_automatically_roll_back(self):
        original = self.command

        def fail(args, **kwargs):
            if "up" in args:
                raise updater.UpdateError("New image failed")
            return original(args, **kwargs)
        with patch.object(updater, "command", side_effect=fail), patch.object(updater, "trusted_file", side_effect=Path):
            self.host.run("1.2.4")
        self.assertEqual(self.host.status()["phase"], "needs_attention")
        self.assertFalse(any("start" in c for c in self.calls))

    def test_unreleased_wg_lease_blocks_recreation(self):
        self.host.sql = lambda *args: "t"
        with patch.object(updater, "command", side_effect=self.command):
            self.host.run("1.2.4")
        self.assertFalse(self.started_new)
        self.assertFalse(self.backups)
        self.assertEqual(self.host.status()["phase"], "needs_attention")

    def test_fingerprint_change_does_not_report_success(self):
        count = 0

        def snapshot(pg):
            nonlocal count
            count += 1
            return {"accounts": str(count)}
        self.host.snapshot = snapshot
        with patch.object(updater, "command", side_effect=self.command), patch.object(updater, "trusted_file", side_effect=Path):
            self.host.run("1.2.4")
        self.assertEqual(self.host.status()["phase"], "needs_attention")

    def test_same_job_is_idempotent_and_other_versions_blocked(self):
        self.assertEqual(self.host.begin("1.2.4")["id"], "job")
        with self.assertRaises(updater.UpdateError):
            self.host.begin("1.2.5")

    def test_invalid_versions_and_unreviewed_failure_rejected(self):
        for version in ["latest", "1.2.3; id", "../file", "1.2.3-rc1", None, "01.2.3"]:
            with self.assertRaises(updater.UpdateError):
                self.host.begin(version)
        self.host.job["phase"] = "needs_attention"
        with self.assertRaises(updater.UpdateError):
            self.host.begin("1.2.5")

    def test_stale_verification_cannot_finish_job(self):
        self.host.job["phase"] = "verifying"
        with self.assertRaises(updater.UpdateError):
            self.host.verify({"id": "old-job", "version": "1.2.4", "ok": True})
        self.assertEqual(self.host.status()["phase"], "verifying")
        result = self.host.verify({"id": "job", "version": "1.2.4", "ok": False, "error": "secret"})
        self.assertEqual(result["phase"], "needs_attention")
        self.assertNotIn("secret", json.dumps(result))

    def test_compose_mount_drift_rejected_before_mutation(self):
        model = {"services": {"sub2api": {"environment": {"DATABASE_HOST": "postgres"}, "volumes": [
            {"type": "bind", "source": "/new/empty/data", "target": "/app/data"}]}}}
        with patch.object(updater, "command", return_value=json.dumps(model).encode()):
            with self.assertRaisesRegex(updater.UpdateError, "mounts differ|bind source is missing"):
                updater.Updater.check_compose(self.host, self.app)

    def test_host_sys_recursive_bind_matches_inspect_and_rejects_drift(self):
        model = {"services": {"sub2api": {"volumes": [
            {"type": "bind", "source": "/opt/sub2api/data", "target": "/app/data"},
            {"type": "bind", "source": "/sys", "target": "/host/sys", "read_only": True,
             "bind": {"create_host_path": False}}]}}}
        self.app["Mounts"].append({"Type": "bind", "Source": "/sys", "Destination": "/host/sys",
                                   "RW": False, "Propagation": "rprivate", "Mode": "ro"})
        with patch.object(updater, "command", return_value=json.dumps(model).encode()), \
                patch.object(Path, "exists", lambda path: str(path) in ("/sys", "/opt/sub2api/data")):
            updater.Updater.check_compose(self.host, self.app)
            before = self.host.mounts(self.app)
            # Recreated container retains the same externally visible bind.
            recreated = json.loads(json.dumps(self.app))
            self.assertEqual(before, self.host.mounts(recreated))
            for field, value in [("RW", True), ("Source", "/sys/class/net")]:
                changed = json.loads(json.dumps(self.app))
                changed["Mounts"][-1][field] = value
                self.assertNotEqual(before, self.host.mounts(changed))
                with self.assertRaisesRegex(updater.UpdateError, "mounts differ"):
                    updater.Updater.check_compose(self.host, changed)
            self.app["Mounts"].pop()
            with self.assertRaisesRegex(updater.UpdateError, "mounts differ"):
                updater.Updater.check_compose(self.host, self.app)

    def test_compose_database_drift_rejected_before_mutation(self):
        model = {"services": {"sub2api": {"environment": {"DATABASE_HOST": "empty-database"}}}}
        with patch.object(updater, "command", return_value=json.dumps(model).encode()):
            with self.assertRaisesRegex(updater.UpdateError, "environment differs"):
                updater.Updater.check_compose(self.host, self.app)

    def test_existing_container_names_and_service_aliases_are_accepted(self):
        app = {"NetworkSettings": {"Networks": {"existing-network": {}}}}
        dependency = {"NetworkSettings": {"Networks": {"existing-network": {
            "Aliases": ["sub2api-postgres", "postgres"]}}}}
        for host in ["sub2api-postgres", "postgres"]:
            self.assertTrue(updater.Updater.service_host(app, dependency, host))

    def test_foreign_database_hosts_are_rejected(self):
        app = {"NetworkSettings": {"Networks": {"existing-network": {}}}}
        dependency = {"NetworkSettings": {"Networks": {"existing-network": {
            "Aliases": ["sub2api-postgres", "postgres"]}}}}
        for host in ["external.example", "another-postgres", "", None]:
            self.assertFalse(updater.Updater.service_host(app, dependency, host))

    def test_alias_on_unshared_network_is_rejected(self):
        app = {"NetworkSettings": {"Networks": {"existing-network": {}}}}
        dependency = {"NetworkSettings": {"Networks": {"another-network": {
            "Aliases": ["postgres"]}}}}
        self.assertFalse(updater.Updater.service_host(app, dependency, "postgres"))


if __name__ == "__main__":
    unittest.main()
