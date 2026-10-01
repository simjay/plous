"""CLI tests with isolated configuration."""

import subprocess
import tempfile
import unittest
from pathlib import Path

from support import BINARY, ROOT


class CommandLineTests(unittest.TestCase):
    def run_cli(self, *args):
        return subprocess.run([str(BINARY), *args], capture_output=True, text=True, timeout=10)

    def test_version_matches_single_source(self):
        result = self.run_cli("--version")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout, (ROOT / "cmd/plous/VERSION").read_text())
        self.assertEqual(result.stderr, "")

    def test_help_lists_supported_options(self):
        result = self.run_cli("--help")
        self.assertEqual(result.returncode, 0)
        self.assertIn("-version", result.stderr)
        self.assertIn("-data-dir", result.stderr)

    def test_invalid_arguments_fail_with_usage(self):
        for argument in ["unexpected", "--unknown"]:
            with self.subTest(argument=argument):
                result = self.run_cli(argument)
                self.assertEqual(result.returncode, 2)
                self.assertIn("Usage", result.stderr)

    def test_non_terminal_fails_without_creating_storage(self):
        with tempfile.TemporaryDirectory(prefix="plous-cli-") as directory:
            state = Path(directory) / "state"
            result = self.run_cli("--data-dir", str(state))
            self.assertEqual(result.returncode, 1)
            self.assertIn("interactive terminal", result.stderr)
            self.assertFalse(state.exists())
