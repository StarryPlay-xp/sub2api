#!/usr/bin/env python3
"""Host-only Compose updater. No shell/paths/images are accepted from the app.

Run as a root-owned systemd service; configure only root-owned files. The app
gets a Unix socket, never Docker access. Backups and state stay outside the
socket directory, and command output (which may contain secrets) is not logged.
"""
import argparse
import fcntl
import gzip
import http.server
import json
import os
from pathlib import Path
import re
import shutil
import socketserver
import stat
import subprocess
import threading
import time
import urllib.request
import uuid

REPO = "StarryPlay-xp/sub2api"
IMAGE = "ghcr.io/starryplay-xp/sub2api"
VERSION = re.compile(r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\Z")
ACTIVE = {"queued", "preflight", "pulling", "stopping", "backing_up", "recreating", "verifying"}


class UpdateError(Exception):
    pass


def atomic_json(path, data, mode=0o600):
    path = Path(path)
    temporary = path.with_suffix(".tmp")
    fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_TRUNC | os.O_NOFOLLOW, mode)
    with os.fdopen(fd, "w") as stream:
        json.dump(data, stream)
        stream.flush()
        os.fsync(stream.fileno())
    os.replace(temporary, path)
    fd = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


def trusted_file(path):
    p = Path(path)
    if not p.is_absolute() or p.is_symlink():
        raise UpdateError("Configuration paths must be absolute regular files")
    info = p.stat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o022:
        raise UpdateError("Updater configuration must be root-owned and not group/world writable")
    for parent in p.parents:
        info = parent.stat()
        if info.st_uid != 0 or info.st_mode & 0o022 or parent.is_symlink():
            raise UpdateError("Updater configuration directory is not trusted")
    return p


def command(args, timeout=120, input_data=None, include_stderr=False):
    if args and args[0] == "docker":
        args = ["docker", "--host", "unix:///var/run/docker.sock", *args[1:]]
    try:
        result = subprocess.run(args, input=input_data, stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, timeout=timeout, check=False)
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise UpdateError("Host command could not complete; inspect deployment locally") from exc
    if result.returncode:
        raise UpdateError("Host command failed; inspect deployment locally (command output withheld)")
    return result.stdout + (result.stderr if include_stderr else b"")


class Updater:
    def __init__(self, config):
        self.config = config
        self.directory = Path(config["project_directory"])
        self.state_dir = Path(config["state_directory"])
        self.socket_dir = Path(config["socket_directory"])
        self.compose_files = [trusted_file(p) for p in config["compose_files"]]
        self.env_file = trusted_file(config["env_file"])
        self.project = config["project_name"]
        if not re.fullmatch(r"[a-z0-9][a-z0-9_-]*", self.project):
            raise UpdateError("Invalid Compose project name")
        if self.directory != self.directory.resolve() or not self.directory.is_dir():
            raise UpdateError("Project directory must be an existing canonical path")
        for path in [self.state_dir, self.socket_dir]:
            if not path.is_absolute() or path.is_symlink():
                raise UpdateError("Updater directories must be absolute and not symlinks")
            path.mkdir(parents=True, exist_ok=True, mode=0o700)
            for directory in [path, *path.parents]:
                info = directory.stat()
                if directory.is_symlink() or info.st_uid != 0 or info.st_mode & 0o022:
                    raise UpdateError("Updater directories must be root-owned and not group/world writable")
        if self.state_dir == self.socket_dir or self.socket_dir in self.state_dir.parents:
            raise UpdateError("State and backups must not be inside the mounted socket directory")
        os.chmod(self.state_dir, 0o700)
        os.chown(self.socket_dir, 0, int(config.get("app_gid", 1000)))
        os.chmod(self.socket_dir, 0o750)
        self.lock_file = open(self.state_dir / "daemon.lock", "a")
        fcntl.flock(self.lock_file, fcntl.LOCK_EX | fcntl.LOCK_NB)
        self.override = self.directory / "compose.image-version.json"
        self.mutex = threading.RLock()
        self.job = {"enabled": True, "phase": "idle"}
        state = self.state_dir / "status.json"
        if state.exists():
            self.job = json.loads(state.read_text())
            if self.job["phase"] in ACTIVE:
                self.job.update(phase="needs_attention", error="Updater restarted during a job; inspect the deployment before retrying")
                self.save()

    def save(self):
        atomic_json(self.state_dir / "status.json", self.job)

    def phase(self, phase, **extra):
        with self.mutex:
            self.job.update(phase=phase, **extra)
            self.save()

    def status(self):
        with self.mutex:
            return {k: v for k, v in self.job.items()
                    if k in {"enabled", "id", "phase", "version", "error", "nodes"}}

    def compose(self, *args, override=True):
        files = list(self.compose_files)
        if override and self.override.exists():
            files.append(trusted_file(self.override))
        return ["docker", "compose", "--project-directory", str(self.directory),
                "--env-file", str(self.env_file), "-p", self.project,
                *[part for f in files for part in ("-f", str(f))], *args]

    def container(self, service):
        ids = command(self.compose("ps", "-a", "-q", service)).decode().split()
        if len(ids) != 1:
            raise UpdateError("Expected exactly one container per service")
        return json.loads(command(["docker", "inspect", ids[0]]))[0]

    def sql(self, postgres, query):
        return command(["docker", "exec", "-i", postgres, "sh", "-c",
                        'exec psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At'],
                       input_data=query.encode()).decode().strip()

    def snapshot(self, postgres):
        tables = {
            "users": "id,password_hash",
            "api_keys": "id,key,user_id",
            "accounts": "id,proxy_id",
            "proxies": "id,protocol,host,port,username,password",
            "wireguard_nodes": "proxy_id,config_ciphertext,config_version",
            "account_wireguard_bindings": "account_id,proxy_id,revision",
        }
        result = {}
        for table, columns in tables.items():
            if self.sql(postgres, f"SELECT to_regclass('public.{table}') IS NOT NULL") != "t":
                continue
            # Only aggregate fingerprints are retained, never raw credentials.
            result[table] = self.sql(postgres, f"SELECT COUNT(*)::text || ':' || md5(COALESCE(string_agg(row_to_json(t)::text, ',' ORDER BY {columns.split(',')[0]}),'')) FROM (SELECT {columns} FROM {table}) t")
        return result

    def begin(self, version):
        if not isinstance(version, str) or not VERSION.fullmatch(version):
            raise UpdateError("Only stable three-part versions are supported")
        with self.mutex:
            if self.job["phase"] in ACTIVE:
                if self.job.get("version") == version:
                    return self.status()
                raise UpdateError("An image update is already running")
            if self.job["phase"] == "needs_attention":
                raise UpdateError("Resolve the previous job on the server before another upgrade")
            self.job = {"enabled": True, "phase": "queued", "version": version,
                        "id": uuid.uuid4().hex, "nodes": []}
            self.save()
            threading.Thread(target=self.run, args=(version,), daemon=True).start()
            return self.status()

    def verify(self, data):
        with self.mutex:
            if self.job["phase"] != "verifying" or data.get("id") != self.job["id"] or data.get("version") != self.job["version"]:
                raise UpdateError("Verification does not match the active job")
            if data.get("ok") is True:
                self.phase("succeeded", error="")
            else:
                # Do not trust arbitrary text from the application as a log or UI message.
                self.phase("needs_attention", error="Application/WireGuard verification failed; inspect nodes. No automatic database rollback was performed")
            return self.status()

    def latest(self):
        req = urllib.request.Request(f"https://api.github.com/repos/{REPO}/releases/latest",
                                     headers={"User-Agent": "Sub2API-Host-Updater", "Accept": "application/vnd.github+json"})
        with urllib.request.urlopen(req, timeout=30) as response:
            data = json.loads(response.read(1024 * 1024))
        if data.get("draft") or data.get("prerelease"):
            raise UpdateError("Release is not stable")
        return data["tag_name"].removeprefix("v")

    @staticmethod
    def mounts(container):
        # Docker inspect exposes one bind entry for /sys; recursive kernel
        # submounts are not separate Mounts entries. Preserve strict equality.
        return sorted((m["Destination"], m["Type"], m["Source"], m["RW"]) for m in container["Mounts"])

    @staticmethod
    def service_host(app, dependency, host):
        """Accept only DNS aliases of the inspected dependency on a shared network."""
        app_networks = app.get("NetworkSettings", {}).get("Networks", {})
        dependency_networks = dependency.get("NetworkSettings", {}).get("Networks", {})
        return bool(host) and any(
            host in (network.get("Aliases") or [])
            for name, network in dependency_networks.items() if name in app_networks
        )

    def preflight(self):
        app, postgres, redis = (self.container(s) for s in ("sub2api", "postgres", "redis"))
        if not all(c["State"]["Running"] for c in (app, postgres, redis)):
            raise UpdateError("Application, PostgreSQL and Redis must already be running")
        if not any(m["Destination"] == "/app/data" for m in app["Mounts"]):
            raise UpdateError("Application data is not mounted persistently")
        self.check_compose(app)
        for mount in app["Mounts"]:
            source = Path(mount["Source"])
            if self.state_dir == source or source in self.state_dir.parents:
                raise UpdateError("Updater backups must not be accessible from the application")
        env = dict(e.split("=", 1) for e in app["Config"]["Env"] if "=" in e)
        if not self.service_host(app, postgres, env.get("DATABASE_HOST")) or not self.service_host(app, redis, env.get("REDIS_HOST")):
            raise UpdateError("This updater requires the existing Compose PostgreSQL and Redis services")
        pg_env = dict(e.split("=", 1) for e in postgres["Config"]["Env"] if "=" in e)
        if env.get("DATABASE_DBNAME") != pg_env.get("POSTGRES_DB") or env.get("DATABASE_USER") != pg_env.get("POSTGRES_USER"):
            raise UpdateError("Application and backup database identity differ")
        pgdata = pg_env.get("PGDATA", "")
        if not pgdata or not any(pgdata == m["Destination"] or pgdata.startswith(m["Destination"] + "/") for m in postgres["Mounts"]):
            raise UpdateError("PostgreSQL data directory is not mounted")
        if env.get("IMAGE_UPDATER_SOCKET") != "/run/sub2api-updater/updater.sock":
            raise UpdateError("Install the updater Compose override before enabling updates")
        if not any(m["Destination"] == "/run/sub2api-updater" and m["Source"] == str(self.socket_dir) for m in app["Mounts"]):
            raise UpdateError("Updater socket mount does not match host configuration")
        keys = []
        wg_exists = self.sql(postgres["Id"], "SELECT to_regclass('public.wireguard_nodes') IS NOT NULL") == "t"
        has_nodes = wg_exists and self.sql(postgres["Id"], "SELECT EXISTS(SELECT 1 FROM wireguard_nodes)") == "t"
        key_target = env.get("WIREGUARD_KEY_FILE")
        if key_target:
            matches = [m for m in app["Mounts"] if m["Type"] == "bind" and m["Destination"] == key_target and not m["RW"]]
            if len(matches) != 1:
                raise UpdateError("WG key must be a separate read-only file mount")
            key = Path(matches[0]["Source"])
            info = key.stat()
            if not key.is_file() or key.is_symlink() or info.st_mode & 0o077 or not re.fullmatch(r"[0-9a-fA-F]{64}\s*", key.read_text()):
                raise UpdateError("WG key file has invalid format or permissions")
            keys.append(key)
        elif has_nodes:
            raise UpdateError("Existing WG nodes require their original key file")
        # Estimate database + application data backup space, leaving room for image layers.
        db_size = int(self.sql(postgres["Id"], "SELECT pg_database_size(current_database())"))
        app_size = int(command(["docker", "exec", app["Id"], "du", "-sk", "/app/data"]).split()[0]) * 1024
        if shutil.disk_usage(self.state_dir).free < db_size * 2 + app_size * 2 + 2 * 1024**3:
            raise UpdateError("Insufficient disk space for backup and image update")
        return app, postgres, redis, keys, wg_exists

    def check_compose(self, app):
        """Reject deployment drift BEFORE starting a new image against data."""
        model = json.loads(command(self.compose("config", "--format", "json")))
        service = model["services"]["sub2api"]
        actual_env = dict(e.split("=", 1) for e in app["Config"]["Env"] if "=" in e)
        for key, value in service.get("environment", {}).items():
            if value is not None and str(value) != actual_env.get(key):
                raise UpdateError("Compose environment differs from the running application; reconcile before updating")
        expected = []
        for mount in service.get("volumes", []):
            source = mount["source"]
            if mount["type"] == "volume":
                name = model["volumes"][source]["name"]
                source = json.loads(command(["docker", "volume", "inspect", name]))[0]["Mountpoint"]
            elif mount["type"] == "bind" and not Path(source).exists():
                raise UpdateError("A configured bind source is missing; refusing to create an empty replacement")
            expected.append((mount["target"], mount["type"], source, not mount.get("read_only", False)))
        if sorted(expected) != self.mounts(app):
            raise UpdateError("Compose data mounts differ from the running application; reconcile before updating")

    def backup(self, directory, app, postgres, keys):
        directory.mkdir(mode=0o700)
        for index, path in enumerate([*self.compose_files, self.env_file, *keys]):
            shutil.copyfile(path, directory / f"config-{index}-{path.name}")
        if self.override.exists():
            shutil.copyfile(self.override, directory / self.override.name)
        atomic_json(directory / "deployment.json", {"image": app["Config"]["Image"], "image_id": app["Image"],
                                                    "mounts": self.mounts(app), "environment": app["Config"]["Env"]})
        # The application is stopped before copying its data or dumping the DB.
        command(["docker", "cp", app["Id"] + ":/app/data", str(directory / "app-data")], timeout=600)
        path = directory / "database.sql.gz"
        args = ["docker", "--host", "unix:///var/run/docker.sock", "exec", postgres["Id"], "sh", "-c",
                'exec pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" --no-owner --no-acl --clean --if-exists']
        # Stream to disk, never hold a production database in memory.
        with open(path, "wb") as output, subprocess.Popen(args, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL) as process:
            timer = threading.Timer(600, process.kill)
            timer.start()
            try:
                with gzip.GzipFile(fileobj=output, mode="wb") as archive:
                    shutil.copyfileobj(process.stdout, archive)
                if process.wait() != 0:
                    raise UpdateError("Database backup failed; old application will be restarted")
            finally:
                timer.cancel()
        with gzip.open(path, "rb") as archive:
            size = 0
            while chunk := archive.read(1024 * 1024):
                size += len(chunk)
        if size < 100:
            raise UpdateError("Database backup is empty")

    def wait_for_verification(self, seconds=900):
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline:
            if self.status()["phase"] != "verifying":
                return
            time.sleep(3)
        raise UpdateError("New application did not confirm version and WG verification in time")

    def run(self, version):
        stopped = False
        recreated = False
        try:
            self.phase("preflight")
            if self.latest() != version:
                raise UpdateError("Requested version is no longer the latest stable release")
            app, postgres, redis, keys, wg_exists = self.preflight()
            current = command(["docker", "exec", app["Id"], "/app/sub2api", "-version"], include_stderr=True)
            match = re.search(rb"Sub2API v?([0-9]+\.[0-9]+\.[0-9]+)(?:[\s(])", current)
            if not match:
                raise UpdateError("Cannot identify the running application version")
            old_version = match[1].decode()
            if tuple(map(int, version.split('.'))) <= tuple(map(int, old_version.split('.'))):
                raise UpdateError("Image updater only supports forward upgrades")
            self.phase("pulling")
            command(["docker", "pull", f"{IMAGE}:{version}"], timeout=900)
            image = json.loads(command(["docker", "image", "inspect", f"{IMAGE}:{version}"]))[0]
            labels = image["Config"].get("Labels") or {}
            if labels.get("org.opencontainers.image.version") != version or labels.get("org.opencontainers.image.source", "").lower() != f"https://github.com/{REPO}".lower():
                raise UpdateError("Image provenance labels do not match this release")
            digests = [d for d in image.get("RepoDigests", []) if d.startswith(IMAGE + "@sha256:")]
            if len(digests) != 1:
                raise UpdateError("Cannot pin the downloaded image digest")
            # Re-check configuration immediately before the first service mutation.
            if self.mounts(self.container("sub2api")) != self.mounts(app):
                raise UpdateError("Deployment changed during image download")
            self.check_compose(app)
            self.phase("stopping")
            stopped = True
            command(self.compose("stop", "-t", "120", "sub2api"), timeout=150)
            old = json.loads(command(["docker", "inspect", app["Id"]]))[0]
            if old["State"].get("OOMKilled") or old["State"].get("ExitCode") != 0:
                raise UpdateError("Old application did not shut down cleanly; inspect WG nodes")
            if wg_exists and self.sql(postgres["Id"], "SELECT EXISTS(SELECT 1 FROM wireguard_runtime_leases)") == "t":
                raise UpdateError("WG runtime ownership was not released; inspect nodes before retrying")
            baseline = self.snapshot(postgres["Id"])
            nodes = []
            if wg_exists:
                rows = self.sql(postgres["Id"], "SELECT p.id FROM proxies p JOIN wireguard_nodes w ON w.proxy_id=p.id WHERE p.deleted_at IS NULL AND p.status='active' AND NOT w.paused AND (p.expires_at IS NULL OR p.expires_at>NOW()) ORDER BY p.id")
                nodes = [int(row) for row in rows.splitlines() if row]
            self.phase("backing_up", nodes=nodes)
            directory = self.state_dir / self.job["id"]
            self.backup(directory, app, postgres, keys)
            atomic_json(directory / "fingerprints.json", baseline)
            self.phase("recreating")
            atomic_json(self.override, {"services": {"sub2api": {"image": digests[0], "stop_grace_period": "2m"}}})
            # From this point the new program may migrate the database. Never
            # blindly restore the old image or an old database snapshot.
            recreated = True
            command(self.compose("up", "-d", "--no-deps", "--pull", "never", "sub2api"), timeout=180)
            deadline = time.monotonic() + 600
            while time.monotonic() < deadline:
                new = self.container("sub2api")
                if new["State"].get("Health", {}).get("Status") == "healthy":
                    break
                time.sleep(3)
            else:
                raise UpdateError("New application did not become healthy")
            if new["Image"] != image["Id"] or self.mounts(new) != self.mounts(app):
                raise UpdateError("Running image or retained mounts do not match")
            for service, original in [("postgres",postgres),("redis",redis)]:
                if self.container(service)["Id"] != original["Id"]:
                    raise UpdateError("Database or Redis container changed unexpectedly")
            after = self.snapshot(postgres["Id"])
            if any(after.get(table) != value for table, value in baseline.items()):
                raise UpdateError("Account, proxy or WG configuration fingerprints changed; inspect before accepting upgrade")
            self.phase("verifying")
            self.wait_for_verification()
        except Exception as exc:
            message = str(exc) if isinstance(exc, UpdateError) else "Update failed; inspect the host deployment (details withheld)"
            if stopped and not recreated:
                try:
                    command(self.compose("start", "sub2api"))
                except Exception:
                    message += "; restarting the old application also failed"
            self.phase("needs_attention" if stopped or recreated else "failed", error=message)


class Server(socketserver.ThreadingMixIn, socketserver.UnixStreamServer):
    daemon_threads = True


class Handler(http.server.BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_GET(self):
        self.dispatch()

    def do_POST(self):
        self.dispatch()

    def dispatch(self):
        self.connection.settimeout(10)
        try:
            updater = self.server.updater
            if self.command == "GET" and self.path == "/status":
                result = updater.status()
            elif self.command == "POST" and self.path in {"/update", "/verified"}:
                size = int(self.headers.get("Content-Length", "0"))
                if not 0 < size <= 4096:
                    raise UpdateError("Invalid request size")
                data = json.loads(self.rfile.read(size))
                if not isinstance(data, dict):
                    raise UpdateError("Invalid request")
                result = updater.begin(data.get("version")) if self.path == "/update" else updater.verify(data)
            else:
                self.send_error(404)
                return
            code = 200
        except Exception:
            code, result = 409, {"enabled": True, "phase": "rejected", "error": "Request rejected"}
        body = json.dumps(result).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--config", required=True)
    parser.add_argument("--check", action="store_true", help="Read-only deployment preflight")
    parser.add_argument("--acknowledge-failure", action="store_true", help="Clear a reviewed failure; does not restore data or containers")
    args = parser.parse_args()
    if os.geteuid() != 0:
        raise SystemExit("The host updater must run as root")
    os.umask(0o077)
    updater = Updater(json.loads(trusted_file(args.config).read_text()))
    if args.acknowledge_failure:
        if updater.job["phase"] not in {"failed", "needs_attention"}:
            raise SystemExit("No failed job to acknowledge")
        updater.phase("idle", error="")
        return
    if args.check:
        updater.preflight()
        print("Deployment preflight passed")
        return
    path = updater.socket_dir / "updater.sock"
    path.unlink(missing_ok=True)
    with Server(str(path), Handler) as server:
        server.updater = updater
        os.chown(path, 0, int(updater.config.get("app_gid", 1000)))
        os.chmod(path, 0o660)
        server.serve_forever()


if __name__ == "__main__":
    main()
