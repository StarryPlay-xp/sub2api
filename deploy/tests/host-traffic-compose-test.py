"""Offline checks: no Docker daemon or server access required."""
import json
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2]


class HostTrafficComposeTests(unittest.TestCase):
    def test_all_compose_variants_include_safe_sys_mount(self):
        # docker compose config is also checked separately without a daemon.
        for name in ['docker-compose.yml', 'docker-compose.local.yml',
                     'docker-compose.dev.yml', 'docker-compose.standalone.yml',
                     'compose.host-traffic.yml']:
            text = (ROOT / 'deploy' / name).read_text()
            app = text.split('  sub2api:', 1)[1].split('\n  postgres:', 1)[0]
            block = app.split('        source: /sys\n', 1)[1]
            self.assertTrue(block.startswith('        target: /host/sys\n        read_only: true\n'
                                             '        bind:\n          create_host_path: false'))
            self.assertNotIn('source: /proc', text)

    def test_updater_default_files_include_host_traffic(self):
        config = json.loads((ROOT / 'deploy/image-updater/config.example.json').read_text())
        self.assertIn('/opt/sub2api/compose.host-traffic.yml', config['compose_files'])


if __name__ == '__main__':
    unittest.main()
