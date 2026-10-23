"""Failure reports distinguish early exits from an unresponsive process."""

from pathlib import Path
import hashlib
import sys
import tempfile
import unittest

from support import FIXTURE, Terminal


class TerminalSupportTests(unittest.TestCase):
    def test_large_input_drains_output_without_losing_bytes(self):
        payload = (b"\x1b[<64;146;13M" * 1000 + "界".encode()) * 10
        script = '''
import hashlib, os, sys, tty
tty.setraw(0)
print("ready", flush=True)
remaining = int(sys.argv[1])
digest = hashlib.sha256()
while remaining:
    chunk = os.read(0, min(4096, remaining))
    digest.update(chunk)
    remaining -= len(chunk)
    output = memoryview(b"x" * (4 * len(chunk)))
    while output:
        output = output[os.write(1, output):]
print("\\r\\nreceived=" + digest.hexdigest(), flush=True)
'''
        with tempfile.TemporaryDirectory(prefix="plous-terminal-io-") as directory:
            app = Terminal(Path(directory) / "state", FIXTURE,
                           command_prefix=[sys.executable, "-c", script, str(len(payload))])
            self.addCleanup(app.close)
            app.expect("ready")
            app.send_bytes(payload)
            app.expect("received=" + hashlib.sha256(payload).hexdigest())
            app.wait(lambda: app.process.poll() == 0, "fixture exit")

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
