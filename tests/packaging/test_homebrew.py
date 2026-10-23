"""Release formula checks using synthetic source archives."""

import hashlib
import importlib.util
import io
from pathlib import Path
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("homebrew", ROOT / "scripts/homebrew.py")
homebrew = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(homebrew)


class HomebrewTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.archive = Path(self.temp.name) / "source.tar.gz"

    def source(self, version):
        with tarfile.open(self.archive, "w:gz") as archive:
            data = (version + "\n").encode()
            member = tarfile.TarInfo("plous-0.0.1/cmd/plous/VERSION")
            member.size = len(data)
            archive.addfile(member, io.BytesIO(data))
            data = b"module github.com/simjay/plous\n\ngo 1.26.4\n"
            member = tarfile.TarInfo("plous-0.0.1/go.mod")
            member.size = len(data)
            archive.addfile(member, io.BytesIO(data))

    def test_formula_uses_source_checksum_and_embedded_version(self):
        self.source("0.0.1")
        formula = homebrew.render("v0.0.1", self.archive)
        self.assertIn('version "0.0.1"', formula)
        self.assertIn('/archive/refs/tags/v0.0.1.tar.gz"', formula)
        self.assertIn(hashlib.sha256(self.archive.read_bytes()).hexdigest(), formula)
        self.assertNotIn("@VERSION@", formula)
        self.assertNotIn("@SHA256@", formula)
        self.assertIn('depends_on "go@1.26" => :build', formula)

    def test_mismatched_source_version_fails(self):
        self.source("0.0.1")
        with self.assertRaisesRegex(ValueError, "embedded version"):
            homebrew.render("v0.0.2", self.archive)

    def test_missing_version_fails(self):
        with tarfile.open(self.archive, "w:gz"):
            pass
        with self.assertRaisesRegex(ValueError, "exactly one"):
            homebrew.render("v0.0.1", self.archive)

    def test_invalid_tags_fail(self):
        for tag in ["0.0.1", "v0.0.1-rc.1", "v1.2", "v1.2.3\n", 'v1.2.3"']:
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                homebrew.render(tag, self.archive)

    def test_reruns_and_older_releases_preserve_published_bottles(self):
        formula = Path(self.temp.name) / "plous.rb"
        self.assertFalse(homebrew.already_published("v0.0.1", formula))
        formula.write_text('  version "0.0.10"\n  bottle do\n  end\n')
        self.assertTrue(homebrew.already_published("v0.0.10", formula))
        self.assertTrue(homebrew.already_published("v0.0.9", formula))
        self.assertFalse(homebrew.already_published("v0.0.11", formula))


if __name__ == "__main__":
    unittest.main()
