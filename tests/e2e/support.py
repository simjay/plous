"""Run the executable with temporary storage and a pseudo-terminal."""

import base64
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
# Historical output checks complement screen() assertions against the VT grid.
ANSI = re.compile(rb"\x1b\][^\x07]*(?:\x07|\x1b\\)|\x1b\[[0-?]*[ -/]*[@-~]")


class Terminal:
    def __init__(self, data_dir, kubeconfig, width=120, height=30, env_overrides=None, command_prefix=None, stderr=None, cwd=None):
        self.master, self.slave = pty.openpty()
        self.output = bytearray()
        self.screen_events = [{"Width": width, "Height": height}]
        self.closed = False
        self.started_at = time.monotonic()
        self.first_output_after = None
        fcntl.ioctl(self.slave, termios.TIOCSWINSZ, struct.pack("HHHH", height, width, 0, 0))
        self.initial_flags = termios.tcgetattr(self.slave)[3]
        home = Path(data_dir).parent / "home"
        default_config = home / ".kube" / "config"
        default_config.parent.mkdir(parents=True, exist_ok=True)
        if not default_config.exists():
            default_config.symlink_to(Path(kubeconfig).resolve())
        env = dict(os.environ, TERM="xterm-256color", HOME=str(home), KUBECONFIG=str(kubeconfig),
                   SSH_CONNECTION="test-client 1234 test-server 22")
        if env_overrides:
            env.update(env_overrides)
        if cwd is None:
            cwd = Path(data_dir).parent / "launch"
            cwd.mkdir(parents=True, exist_ok=True)
        try:
            self.process = subprocess.Popen(
                [*(command_prefix or []), str(BINARY), "--data-dir", str(data_dir)],
                stdin=self.slave, stdout=self.slave, stderr=self.slave if stderr is None else stderr,
                env=env, start_new_session=True, cwd=cwd,
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
            if self.first_output_after is None:
                self.first_output_after = time.monotonic() - self.started_at
            self.output.extend(data)
            self.screen_events.append({"Data": base64.b64encode(data).decode("ascii")})

    def send(self, text):
        self.send_bytes(text.encode("utf-8"))

    def mouse_press(self, x, y):
        self.send(f"\x1b[<0;{x + 1};{y + 1}M")

    def mouse_release(self, x, y):
        self.send(f"\x1b[<0;{x + 1};{y + 1}m")

    def mouse_drag(self, x, y):
        self.send(f"\x1b[<32;{x + 1};{y + 1}M")

    def mouse_click(self, x, y):
        self.mouse_press(x, y)
        self.mouse_release(x, y)

    def mouse_double_click(self, x, y):
        # Send one gesture without read waits between its clicks.
        self.send(f"\x1b[<0;{x + 1};{y + 1}M\x1b[<0;{x + 1};{y + 1}m" * 2)

    def mouse_wheel(self, x, y, up=False):
        self.send(f"\x1b[<{64 if up else 65};{x + 1};{y + 1}M")

    def send_bytes(self, data):
        # Drain output so a large paste cannot fill both PTY directions.
        while data:
            written = os.write(self.master, data[:512])
            data = data[written:]
            self.read(0.01)
        self.read(0.08)

    def text(self, start=0):
        return ANSI.sub(b"", bytes(self.output[start:])).decode("utf-8", "replace")

    def screen(self):
        result = subprocess.run(
            [str(ROOT / "bin/test-terminalscreen")],
            input=json.dumps(self.screen_events), capture_output=True, text=True, timeout=5,
        )
        if result.returncode:
            raise AssertionError("Cannot reconstruct terminal screen: " + result.stderr)
        return json.loads(result.stdout)

    def expect_screen(self, text):
        self.wait(lambda: text in "\n".join(self.screen()["rows"]), "visible " + repr(text))

    def capture_screen(self, name):
        screen = self.screen()
        directory = os.environ.get("PLOUS_REVIEW_DIR")
        if directory:
            target = Path(directory)
            target.mkdir(parents=True, exist_ok=True)
            (target / (name + ".ansi")).write_text(screen["ansi"])
            (target / (name + ".txt")).write_text("\n".join(screen["rows"]))
        return screen["rows"]

    def wait(self, predicate, description, timeout=8):
        started = time.monotonic()
        deadline = started + timeout
        while time.monotonic() < deadline:
            self.read()
            if predicate():
                return
            if self.process.poll() is not None:
                # Exit can complete between the predicate and this poll.
                if predicate():
                    return
                break
        returncode = self.process.poll()
        if returncode is None:
            reason = f"Timed out waiting for {description}; process is still running"
        elif returncode < 0:
            reason = f"Process terminated by {signal.Signals(-returncode).name} while waiting for {description}"
        else:
            reason = f"Process exited with code {returncode} while waiting for {description}"
        diagnostic = dict(reason=reason, pid=self.process.pid, returncode=returncode,
                          wait_seconds=round(time.monotonic() - started, 3),
                          process_seconds=round(time.monotonic() - self.started_at, 3),
                          output_bytes=len(self.output), first_output_after=self.first_output_after)
        name = f"failed-terminal-{self.process.pid}"
        directory = os.environ.get("PLOUS_REVIEW_DIR")
        if directory:
            target = Path(directory)
            target.mkdir(parents=True, exist_ok=True)
            (target / (name + ".json")).write_text(json.dumps(diagnostic, indent=2))
            (target / (name + ".events.json")).write_text(json.dumps(self.screen_events))
        visible = "\n".join(self.capture_screen(name))
        raise AssertionError(f"{reason}\nDiagnostics: {json.dumps(diagnostic)}\nCurrent screen:\n{visible}\nRecent output:\n{self.text()[-1500:]}")

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
        self.screen_events.append({"Width": width, "Height": height})
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
