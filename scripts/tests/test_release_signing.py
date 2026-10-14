"""Disposable keys and fixtures only; no repository secrets or real artifacts."""
import base64
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / 'sign-release-assets.sh'
VERSION = '0.1.173-dev'
FILES = [f'home-ai-core-update_{VERSION}_{a}.tar.gz' for a in ('amd64', 'arm64')] + [f'home-ai-core_0.1.173.dev_{a}.deb' for a in ('amd64', 'arm64')] + [f'home-ai-windows-client_{VERSION}_amd64.exe']

class SigningTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.key = subprocess.check_output(['openssl', 'genpkey', '-algorithm', 'ED25519'], stderr=subprocess.DEVNULL).decode()

    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.root = Path(tmp.name)
        self.dist = self.root / 'dist'
        self.dist.mkdir()
        self.env = {**os.environ, 'HOME_AI_UPDATE_SIGNING_KEY': self.key, 'GITHUB_SHA': 'a'*40, 'RUNNER_TEMP': str(self.root)}
        for name in FILES:
            (self.dist / name).write_bytes(('isolated test fixture '+name).encode())

    def invoke(self, mode='--sign'):
        args = ['bash', str(SCRIPT), mode]
        if mode == '--sign': args += [str(self.dist), VERSION]
        return subprocess.run(args, env=self.env, capture_output=True, text=True, timeout=30)

    def test_missing_key(self):
        self.env.pop('HOME_AI_UPDATE_SIGNING_KEY')
        result = self.invoke()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('not configured', result.stderr)
        self.assertEqual(len(list(self.dist.iterdir())), 5)

    def test_invalid_key(self):
        self.env['HOME_AI_UPDATE_SIGNING_KEY'] = 'not-a-key-private-test-value'
        result = self.invoke('--check-key')
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn(self.env['HOME_AI_UPDATE_SIGNING_KEY'], result.stdout + result.stderr)

    def test_wrong_algorithm(self):
        self.env['HOME_AI_UPDATE_SIGNING_KEY'] = subprocess.check_output(['openssl','genpkey','-algorithm','EC','-pkeyopt','ec_paramgen_curve:P-256'], stderr=subprocess.DEVNULL).decode()
        result = self.invoke('--check-key')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Ed25519', result.stderr)

    def test_self_check(self):
        result = self.invoke('--check-key')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(list(self.dist.iterdir())), 5)
        self.assertNotIn('PRIVATE KEY', result.stdout + result.stderr)
        self.assertFalse(list(self.root.glob('home-ai-sign.*')))

    def test_incomplete_release(self):
        (self.dist / FILES[0]).unlink()
        self.assertNotEqual(self.invoke().returncode, 0)
        self.assertFalse((self.dist / 'release-metadata.json').exists())

    def test_symlink(self):
        target = self.root / 'outside'
        target.write_text('fixture')
        artifact = self.dist / FILES[0]
        artifact.unlink()
        artifact.symlink_to(target)
        self.assertNotEqual(self.invoke().returncode, 0)

    def test_extra_asset(self):
        (self.dist / 'unexpected.txt').write_text('fixture')
        self.assertNotEqual(self.invoke().returncode, 0)

    def test_source_commit_required(self):
        self.env.pop('GITHUB_SHA')
        self.assertNotEqual(self.invoke().returncode, 0)

    def test_signed_release_and_tampering(self):
        result = self.invoke()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn('PRIVATE KEY', result.stdout + result.stderr)
        pub = self.dist / 'home-ai-core-update-trusted.pub'
        sig = self.root / 'signature'
        for name in [x + '.sha256' for x in FILES] + ['SHA256SUMS']:
            with self.subTest(name=name):
                raw = base64.b64decode((self.dist / (name + '.sig')).read_bytes())
                self.assertEqual(len(raw), 64)
                sig.write_bytes(raw)
                checked = subprocess.run(['openssl','pkeyutl','-verify','-rawin','-pubin','-inkey',str(pub),'-in',str(self.dist / name),'-sigfile',str(sig)], capture_output=True)
                self.assertEqual(checked.returncode, 0)
                if name != 'SHA256SUMS':
                    self.assertEqual((self.dist / name).read_text().split()[1], name.removesuffix('.sha256'))
        metadata = json.loads((self.dist / 'release-metadata.json').read_text())
        self.assertEqual(metadata['source_commit'], 'a'*40)
        self.assertTrue(metadata['migration']['from_172_requires_trusted_debian_installer'])
        self.assertFalse(list(self.root.glob('home-ai-sign.*')))
        (self.dist / FILES[0]).write_text('tampered')
        self.assertNotEqual(subprocess.run(['sha256sum','--check','--strict','SHA256SUMS'], cwd=self.dist, capture_output=True).returncode, 0)
        sums = self.dist / 'SHA256SUMS'
        sums.write_text(sums.read_text() + 'tampered')
        self.assertNotEqual(subprocess.run(['openssl','pkeyutl','-verify','-rawin','-pubin','-inkey',str(pub),'-in',str(sums),'-sigfile',str(sig)], capture_output=True).returncode, 0)

if __name__ == '__main__': unittest.main(verbosity=2)
