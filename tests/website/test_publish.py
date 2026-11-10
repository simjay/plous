"""Check publication identity, latest selection, aliases, and archive safety."""

import hashlib
import importlib.util
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest.mock import patch

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

    def test_corrections_preserve_the_original_and_ignore_partial_uploads(self):
        tag = "v0.0.3"
        original = f"documentation-{tag}"
        first = f"documentation-{tag}-docs-{'a' * 40}"
        second = f"documentation-{tag}-docs-{'b' * 40}"
        release = {"tag_name": tag, "assets": [
            {"name": original + ".tar.gz"}, {"name": original + ".sha256"},
            {"name": first + ".tar.gz", "created_at": "2026-11-10T00:00:00Z"},
            {"name": first + ".sha256"},
            {"name": second + ".tar.gz", "created_at": "2026-11-11T00:00:00Z"},
        ]}
        self.assertEqual(publish.select_snapshot(release), (first, "a" * 40))
        release["assets"].append({"name": second + ".sha256"})
        self.assertEqual(publish.select_snapshot(release), (second, "b" * 40))
        release["assets"] = release["assets"][:2]
        self.assertEqual(publish.select_snapshot(release), (original, None))
        release["assets"].pop()
        with self.assertRaisesRegex(ValueError, "Incomplete"):
            publish.select_snapshot(release)

    def test_upload_adds_commit_assets_without_overwriting_release_assets(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            revision = "c" * 40
            stem = f"documentation-v0.0.3-docs-{revision}"
            archive = root / (stem + ".tar.gz")
            data = json.dumps({"version": "v0.0.3", "revision": revision, "source": revision}).encode()
            with tarfile.open(archive, "w:gz") as output:
                member = tarfile.TarInfo("site.json")
                member.size = len(data)
                output.addfile(member, io.BytesIO(data))
            (root / (stem + ".sha256")).write_text(f"{hashlib.sha256(archive.read_bytes()).hexdigest()}  {archive.name}\n")
            original = {"isDraft": False, "isPrerelease": False, "assets": [{"name": "documentation-v0.0.3.tar.gz"}]}
            with patch.object(publish, "gh", side_effect=[json.dumps(original), "", ""]) as call:
                publish.upload_correction("v0.0.3", root)
                uploads = [args.args for args in call.call_args_list if args.args[:2] == ("release", "upload")]
                self.assertEqual(len(uploads), 2)
                self.assertTrue(all("--clobber" not in args for args in uploads))
                self.assertTrue(all(stem in args[-1] for args in uploads))
            archived_bytes = archive.read_bytes()
            partial = {**original, "assets": [*original["assets"], {"name": archive.name}]}
            def finish_partial(*args):
                if args[:2] == ("release", "view"):
                    return json.dumps(partial)
                if args[:2] == ("release", "download"):
                    destination = Path(args[args.index("--dir") + 1])
                    (destination / archive.name).write_bytes(archived_bytes)
                return ""
            with patch.object(publish, "gh", side_effect=finish_partial) as call:
                publish.upload_correction("v0.0.3", root)
                uploads = [args.args for args in call.call_args_list if args.args[:2] == ("release", "upload")]
                self.assertEqual(len(uploads), 1)
                self.assertTrue(uploads[0][-1].endswith(".sha256"))
            with patch.object(publish, "gh", return_value=json.dumps({**original, "isDraft": True})) as call:
                with self.assertRaisesRegex(ValueError, "published"):
                    publish.upload_correction("v0.0.3", root)
                self.assertEqual(call.call_count, 1)

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
