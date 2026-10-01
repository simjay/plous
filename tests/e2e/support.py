"""Run the executable with temporary storage and a pseudo-terminal."""

import errno
import fcntl
import json
import os
from pathlib import Path
import pty
import re
import select
import signal
import struct
import subprocess
import termios
import time

ROOT = Path(__file__).resolve().parents[2]
BINARY = Path(os.environ.get("PLOUS_TEST_BINARY", ROOT / "bin/plous")).resolve()
FIXTURE = ROOT / "tests/fixtures/kubeconfig.yaml"
# Used only to inspect emitted text; geometry is checked by the Go layout tests.
ANSI = re.compile(rb"\x1b\][^\x07]*(?:\x07|\x1b\\)|\x1b\[[0-?]*[ -/]*[@-~]")


class Terminal:
    def __init__(self, data_dir, kubeconfig, width=120, height=30):
        self.master, self.slave = pty.openpty()
        self.output = bytearray()
        self.closed = False
        fcntl.ioctl(self.slave, termios.TIOCSWINSZ, struct.pack("HHHH", height, width, 0, 0))
        self.initial_flags = termios.tcgetattr(self.slave)[3]
        home = Path(data_dir).parent / "home"
        default_config = home / ".kube" / "config"
        default_config.parent.mkdir(parents=True, exist_ok=True)
        if not default_config.exists():
            default_config.symlink_to(Path(kubeconfig).resolve())
        env = dict(os.environ, TERM="xterm-256color", HOME=str(home), KUBECONFIG=str(kubeconfig))
        try:
            self.process = subprocess.Popen(
                [str(BINARY), "--data-dir", str(data_dir)],
                stdin=self.slave, stdout=self.slave, stderr=self.slave,
                env=env, start_new_session=True,
            )
        except BaseException:
            os.close(self.master)
            os.close(self.slave)
            raise

    def read(self, duration=0.05):
        deadline = time.monotonic() + duration
        while time.monotonic() < deadline:
            if not select.select([self.master], [], [], max(0, deadline - time.monotonic()))[0]:
                break
            try:
                data = os.read(self.master, 65536)
            except OSError as error:
                if error.errno == errno.EIO:
                    break
                raise
            if not data:
                break
            self.output.extend(data)

    def send(self, text):
        data = text.encode("utf-8")
        # Drain output so a large paste cannot fill both PTY directions.
        while data:
            written = os.write(self.master, data[:512])
            data = data[written:]
            self.read(0.01)
        self.read(0.08)

    def text(self, start=0):
        return ANSI.sub(b"", bytes(self.output[start:])).decode("utf-8", "replace")

    def wait(self, predicate, description, timeout=8):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            self.read()
            if predicate():
                return
            if self.process.poll() is not None:
                break
        raise AssertionError(f"Timed out waiting for {description}\n{self.text()[-6000:]}")

    def expect(self, text, start=0):
        self.wait(lambda: text in self.text(start), repr(text))

    def wait_json(self, path, predicate):
        result = None

        def matches():
            nonlocal result
            try:
                result = json.loads(Path(path).read_text())
                return predicate(result)
            except (FileNotFoundError, json.JSONDecodeError):
                return False

        self.wait(matches, f"saved state in {path.name}")
        return result

    def resize(self, width, height):
        fcntl.ioctl(self.slave, termios.TIOCSWINSZ, struct.pack("HHHH", height, width, 0, 0))
        os.kill(self.process.pid, signal.SIGWINCH)
        self.read(0.1)

    def assert_exit(self, expected=0):
        # Shutdown includes a bounded wait for the terminal reader to stop.
        self.wait(lambda: self.process.poll() is not None, "process exit")
        self.read(0.1)
        if self.process.returncode != expected:
            raise AssertionError(f"Exit {self.process.returncode}, expected {expected}\n{self.text()}")
        if expected == 0:
            if b"\x1b[?1049h" not in self.output or b"\x1b[?1049l" not in self.output:
                raise AssertionError("Alternate screen was not entered and restored")
            mask = termios.ECHO | termios.ICANON
            if termios.tcgetattr(self.slave)[3] & mask != self.initial_flags & mask:
                raise AssertionError("Terminal echo/canonical input was not restored")

    def close(self):
        if self.closed:
            return
        self.closed = True
        try:
            if self.process.poll() is None:
                self.process.kill()
            self.process.wait(timeout=5)
        finally:
            os.close(self.master)
            os.close(self.slave)
