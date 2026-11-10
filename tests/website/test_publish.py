"""Check publication identity, latest selection, aliases, and archive safety."""

import hashlib
import importlib.util
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("publish", Path(__file__).parents[2] / "website/scripts/publish.py")
publish = importlib.util.module_from_spec(spec)
spec.loader.exec_module(publish)


class PublicationTests(unittest.TestCase):
    def snapshot(self, root, version, base):
        root.mkdir()
        (root / "docs").mkdir()
        (root / "index.html").write_text("home")
        (root / "docs/index.html").write_text("guide")
        (root / "404.html").write_text("not found")
        (root / "sitemap.xml").write_text("sitemap")
        (root / "site.json").write_text(json.dumps({"version": version, "base": base, "routes": ["", "docs/"]}))
        return root

    def test_retains_versions_and_selects_latest_numerically(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            dev = self.snapshot(root / "dev", "Development", "/plous/dev/")
            releases = [(tag, self.snapshot(root / tag, tag, f"/plous/{tag}/")) for tag in ("v0.0.3", "v0.0.10")]
            output = root / "published"
            self.assertEqual(publish.assemble(dev, releases, output, "/plous/"), "v0.0.10")
            self.assertEqual(json.loads((output / "versions.json").read_text())["latest"], "v0.0.10")
            self.assertIn('/plous/v0.0.10/docs/', (output / "docs/index.html").read_text())
            self.assertTrue((output / "v0.0.3/docs/index.html").exists())
            self.assertEqual((output / "dev/docs/index.html").read_text(), "guide")
            self.assertIn('location.search+location.hash', (output / "index.html").read_text())

    def test_first_deployment_and_identity_validation(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            dev = self.snapshot(root / "dev", "Development", "/dev/")
            self.assertEqual(publish.assemble(dev, [], root / "out", "/"), "Development")
            release = self.snapshot(root / "release", "v0.0.3", "/wrong/")
            with self.assertRaises(ValueError):
                publish.assemble(dev, [("v0.0.3", release)], root / "out", "/")

    def test_rejects_corruption_and_path_escape(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            archive = root / "documentation-v0.0.3.tar.gz"
            with tarfile.open(archive, "w:gz") as output:
                member = tarfile.TarInfo("../escape")
                member.size = 1
                output.addfile(member, io.BytesIO(b"x"))
            checksum = root / "checksum"
            checksum.write_text(f'{hashlib.sha256(archive.read_bytes()).hexdigest()}  {archive.name}\n')
            with self.assertRaisesRegex(ValueError, "Unsafe"):
                publish.unpack(archive, checksum, root / "out")
            checksum.write_text(f'{"0" * 64}  {archive.name}\n')
            with self.assertRaisesRegex(ValueError, "checksum mismatch"):
                publish.unpack(archive, checksum, root / "out")


if __name__ == "__main__":
    unittest.main()
