"""Failure reports distinguish early exits from an unresponsive process."""

from pathlib import Path
import sys
import tempfile
import unittest

from support import FIXTURE, Terminal


class TerminalSupportTests(unittest.TestCase):
    def test_failed_wait_reports_process_state(self):
        cases = (
            ("raise SystemExit(0)", "Process exited with code 0"),
            ("raise SystemExit(17)", "Process exited with code 17"),
            ("import os, signal; os.kill(os.getpid(), signal.SIGTERM)", "Process terminated by SIGTERM"),
            ("import time; time.sleep(30)", "process is still running"),
        )
        for script, expected in cases:
            with self.subTest(expected), tempfile.TemporaryDirectory(prefix="plous-terminal-report-") as directory:
                app = Terminal(Path(directory) / "state", FIXTURE,
                               command_prefix=[sys.executable, "-c", script])
                try:
                    with self.assertRaisesRegex(AssertionError, expected):
                        app.wait(lambda: False, "fixture readiness", timeout=1)
                finally:
                    app.close()
