"""Run the documented Python procedure with all external operations mocked."""
import contextlib
import io
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
GUIDE = (ROOT / 'deploy/HOST_TRAFFIC_CN.md').read_text()
SCRIPT = GUIDE.split("sudo python3 - <<'PY'\n", 1)[1].split('\nPY\n', 1)[0]


class HostTrafficGuideTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.files = [self.root / name for name in (
            'compose.yml', 'compose.image-updater.yml',
            'compose.wireguard.yml', 'compose.host-traffic.yml')]
        self.env = self.root / '.env'
        self.version = self.root / 'compose.image-version.json'
        for path in [*self.files, self.env, self.version]:
            path.write_text('fixture')
        self.config = {'project_directory': str(self.root),
                       'compose_files': list(map(str, self.files)),
                       'env_file': str(self.env), 'project_name': 'fixture'}
        self.calls = []

    def run_guide(self, state='inactive', preflight_fails=False, writable=False):
        read_text = Path.read_text
        is_dir = Path.is_dir

        def read(path, *args, **kwargs):
            if str(path) == '/etc/sub2api-updater.json':
                return json.dumps(self.config)
            return read_text(path, *args, **kwargs)

        def run(args, **kwargs):
            self.calls.append(args)
            if args == ['systemctl', 'is-active', 'sub2api-updater']:
                return subprocess.CompletedProcess(args, 3 if state == 'inactive' else 0,
                                                   stdout=state + '\n')
            if '--check' in args and preflight_fails:
                raise subprocess.CalledProcessError(1, args)
            self.assertTrue(kwargs['check'])
            self.assertTrue(args[0] in ('docker', 'python3', 'systemctl'))
            return subprocess.CompletedProcess(args, 0)

        def output(args, **kwargs):
            self.calls.append(args)
            if args[-3:] == ['ps', '-q', 'sub2api']:
                return 'fixture-container\n'
            self.assertEqual(args[-2:], ['inspect', 'fixture-container'])
            return json.dumps([{'Mounts': [{'Destination': '/host/sys', 'Source': '/sys',
                                           'Type': 'bind', 'RW': writable}]}])

        with patch.object(os, 'geteuid', return_value=0), \
                patch.object(Path, 'read_text', read), \
                patch.object(Path, 'is_dir', lambda p: str(p) == '/sys/class/net' or is_dir(p)), \
                patch.object(subprocess, 'run', side_effect=run), \
                patch.object(subprocess, 'check_output', side_effect=output), \
                contextlib.redirect_stdout(io.StringIO()):
            exec(compile(SCRIPT, 'HOST_TRAFFIC_CN.md', 'exec'), {})

    def test_rebuild_checks_then_starts_updater_with_new_config(self):
        self.run_guide()
        up = next(c for c in self.calls if 'up' in c)
        self.assertEqual(up[-5:], ['up', '-d', '--no-deps', '--force-recreate', 'sub2api'])
        # Preserve every existing override and the pinned image version.
        files = [up[i + 1] for i, arg in enumerate(up) if arg == '-f']
        self.assertEqual(files, list(map(str, [*self.files, self.version])))
        self.assertEqual(self.calls[-3:], [
            ['python3', '/usr/local/lib/sub2api-updater/updater.py',
             '--config', '/etc/sub2api-updater.json', '--check'],
            ['systemctl', 'start', 'sub2api-updater'],
            ['systemctl', 'is-active', '--quiet', 'sub2api-updater'],
        ])
        self.assertNotIn('down', [arg for call in self.calls for arg in call])

    def test_running_or_failed_updater_blocks_container_changes(self):
        for state in ('active', 'activating', 'failed', 'unknown'):
            with self.subTest(state=state):
                self.calls.clear()
                with self.assertRaises(SystemExit):
                    self.run_guide(state=state)
                self.assertEqual(len(self.calls), 1)

    def test_preflight_failure_leaves_updater_stopped(self):
        with self.assertRaises(subprocess.CalledProcessError):
            self.run_guide(preflight_fails=True)
        self.assertIn('--check', self.calls[-1])
        self.assertFalse(any('start' in c for c in self.calls))

    def test_bad_mount_does_not_start_updater(self):
        with self.assertRaises(SystemExit):
            self.run_guide(writable=True)
        self.assertFalse(any('start' in c or '--check' in c for c in self.calls))


if __name__ == '__main__':
    unittest.main()
