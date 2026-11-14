"""Real mouse protocol through the executable using temporary application data."""

import base64
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

from support import FIXTURE, Terminal


class MouseTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory(prefix="plous-mouse-")
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        self.state = self.root / "state"
        self.config = self.root / "kubeconfig"
        self.config.write_bytes(FIXTURE.read_bytes())

    def start(self, env_overrides=None):
        app = Terminal(self.state, self.config, env_overrides=env_overrides)
        self.addCleanup(app.close)
        app.expect("AI assistant")
        return app

    def visible_point(self, app, text):
        for y, row in enumerate(app.screen()["rows"]):
            x = row.find(text)
            if x >= 0:
                return x, y
        self.fail("Missing visible text: " + text)

    def test_sgr_release_and_exit_cleanup(self):
        app = self.start()
        self.assertIn(b"\x1b[?1002h", app.output)
        self.assertIn(b"\x1b[?1006h", app.output)
        # The 120x30 layout is covered by the application geometry tests.
        app.mouse_press(54, 1)
        app.mouse_release(54, 1)
        app.mouse_click(110, 24)
        app.send("mouse draft")
        app.mouse_wheel(110, 8, up=True)
        app.send("\x03")
        app.assert_exit()
        session = json.loads((self.state / "session.json").read_text())
        self.assertEqual(session["mode"], 1)
        self.assertIn("mouse draft", session["drafts"].values())
        self.assertIn(b"\x1b[?1002l", app.output)
        self.assertIn(b"\x1b[?1006l", app.output)
        self.assertNotIn(b"plous-mouse:", app.output)
        self.assertEqual(self.config.read_bytes(), FIXTURE.read_bytes())

    def test_saved_off_and_keyboard_reenable(self):
        app = self.start()
        app.send("ss")
        app.expect("Mouse support: On")
        app.send("\x1b[B\r")
        app.expect("Mouse support: Off")
        app.send("\x03")
        app.assert_exit()

        app.close()
        app = self.start()
        self.assertNotIn(b"\x1b[?1002h", app.output)
        app.send(":s\r")
        app.expect("Mouse support: Off")
        app.send("\x1b[B\r")
        app.expect("Mouse support: On")
        self.assertIn(b"\x1b[?1002h", app.output)
        app.send("\x03")
        app.assert_exit()


    def test_composer_cursor_drag_and_word_selection(self):
        app = self.start()
        app.send(":c\ralpha beta")
        # The composer text begins at column 88 in the 120-column frame.
        app.mouse_click(90, 24)
        app.send("X")
        app.wait_json(self.state / "session.json", lambda s: s["drafts"].get("") == "alXpha beta")
        app.send("\x05\x15one cafe\u0301 two")
        app.mouse_press(92, 24)
        app.mouse_drag(95, 24)
        app.mouse_release(95, 24)
        app.send("tea")
        app.wait_json(self.state / "session.json", lambda s: s["drafts"].get("") == "one tea two")
        app.mouse_double_click(89, 24)
        app.send("new")
        app.send("\x03")
        app.assert_exit()
        session = json.loads((self.state / "session.json").read_text())
        self.assertEqual(session["drafts"].get(""), "new tea two")

    def test_local_clipboard_helpers_mouse_copy_and_forwarded_paste(self):
        helpers = self.root / "helpers"
        helpers.mkdir()
        clipboard = self.root / "clipboard"
        for name in ("pbcopy", "wl-copy", "pbpaste", "wl-paste"):
            script = helpers / name
            body = '/bin/cat > "$PLOUS_TEST_CLIPBOARD"' if name.endswith("copy") else '/bin/cat "$PLOUS_TEST_CLIPBOARD"'
            script.write_text("#!/bin/sh\n" + body + "\n")
            script.chmod(0o700)
        app = self.start({
            "PATH": str(helpers) + os.pathsep + os.environ.get("PATH", ""),
            "PLOUS_TEST_CLIPBOARD": str(clipboard),
            "SSH_CONNECTION": "", "SSH_TTY": "", "MOSH_IP": "",
            "WAYLAND_DISPLAY": "fixture-wayland", "DISPLAY": "",
        })
        app.send(":c\ralpha beta")
        app.mouse_press(88, 24)
        app.mouse_drag(92, 24)
        app.mouse_release(92, 24)
        app.expect_screen("Copy selection")
        x, y = self.visible_point(app, "Copy selection")
        app.mouse_press(x, y)
        app.mouse_drag(x + 1, y)
        app.mouse_release(x + 1, y)
        app.expect_screen("Copied to clipboard.")
        self.assertEqual(clipboard.read_text(), "alpha")
        clipboard.write_text("界\n:quit")
        app.send("\x1b[118;9u")
        app.wait_json(self.state / "session.json", lambda s: s["drafts"].get("") == "界\n:quit beta")
        app.send("\x03")
        app.assert_exit()

    def test_forwarded_copy_shortcuts_and_native_paste(self):
        for modifier in (9, 6):  # Command and Ctrl+Shift in the enhanced key protocol.
            with self.subTest(modifier=modifier):
                app = self.start()
                app.send(":c\r\x05\x15one cafe\u0301 two")
                app.mouse_press(92, 24)
                app.mouse_drag(95, 24)
                app.mouse_release(95, 24)
                app.expect("Copy selection")
                app.send(f"\x1b[99;{modifier}u")
                app.expect("terminal clipboard")
                encoded = base64.b64encode("cafe\u0301".encode())
                self.assertIn(b"\x1b]52;c;" + encoded, app.output)
                app.send("\x1b[200~:quit /model\x1b[201~")
                app.wait_json(self.state / "session.json", lambda s: s["drafts"].get("") == "one :quit /model two")
                app.send("\x03")
                app.assert_exit()
                app.close()

    def test_legacy_click_and_cancelled_press(self):
        app = self.start()
        app.mouse_press(75, 1)
        app.resize(130, 32)
        app.mouse_release(75, 1)
        app.resize(120, 30)
        # X10 press and release, with the legacy unspecified release button.
        app.send_bytes(b"\x1b[M" + bytes((32, 54 + 33, 1 + 33)))
        app.send_bytes(b"\x1b[M" + bytes((35, 54 + 33, 1 + 33)))
        app.send("\x03")
        app.assert_exit()
        session = json.loads((self.state / "session.json").read_text())
        self.assertEqual(session["mode"], 1)

    def test_screen_selection_keeps_settings_mouse_controls(self):
        app = self.start()
        app.send("ss")
        app.expect_screen("Mouse support: On")

        text = "Mouse support: On"
        x, y = self.visible_point(app, text)
        app.mouse_press(x, y)
        app.mouse_drag(x + len(text) - 1, y)
        app.mouse_release(x + len(text) - 1, y)
        app.expect_screen("Copy selection")
        clear_x, clear_y = self.visible_point(app, "Esc Clear selection")
        app.mouse_click(clear_x, clear_y)
        app.expect_screen("Mouse support: On")
        self.assertNotIn("Copy selection", "\n".join(app.screen()["rows"]))
        app.mouse_press(x, y)
        app.mouse_drag(x + len(text) - 1, y)
        app.mouse_release(x + len(text) - 1, y)
        app.expect_screen("Copy selection")
        x, y = self.visible_point(app, "Copy selection")
        app.mouse_press(x, y)
        app.mouse_drag(x + 1, y)
        app.mouse_release(x + 1, y)
        encoded = base64.b64encode(text.encode())
        self.assertIn(b"\x1b]52;c;" + encoded, app.output)
        self.assertIn("Mouse support: On", "\n".join(app.screen()["rows"]))
        app.send("\x1b")
        x, y = self.visible_point(app, "Mouse support: On")
        app.mouse_click(x, y)
        app.expect_screen("Mouse support: Off")
        app.send("\x03")
        app.assert_exit()

    def test_screen_selection_copies_footer_error(self):
        app = self.start()
        app.send(":unknown\r")
        text = "Unknown command."
        app.expect_screen(text)
        x, y = self.visible_point(app, text)
        app.mouse_press(x, y)
        app.mouse_drag(x + len(text) - 1, y)
        app.mouse_release(x + len(text) - 1, y)
        app.expect_screen("Copy selection")
        self.assertIn(text, app.screen()["rows"][-1])
        x, y = self.visible_point(app, "Copy selection")
        self.assertEqual(y, 0)
        app.mouse_click(x, y)
        encoded = base64.b64encode(text.encode())
        self.assertIn(b"\x1b]52;c;" + encoded, app.output)
        app.send("\x03")
        app.assert_exit()

    @unittest.skipUnless(shutil.which("tmux"), "tmux is not installed")
    def test_mouse_through_isolated_tmux(self):
        socket = self.root / "tmux.sock"
        config = self.root / "tmux.conf"
        config.write_text("set -g mouse on\nset -g status off\n")
        command = [shutil.which("tmux"), "-S", str(socket), "-f", str(config)]
        self.addCleanup(lambda: subprocess.run(command + ["kill-server"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL))
        app = Terminal(self.state, self.config, env_overrides={"TMUX": ""}, command_prefix=command + ["new-session", "-s", "mouse-test"])
        self.addCleanup(app.close)
        app.expect("AI assistant")
        app.mouse_click(54, 1)
        app.mouse_click(110, 24)
        app.send("tmux draft")
        app.mouse_wheel(110, 8, up=True)
        app.send("\x03")
        app.assert_exit()
        session = json.loads((self.state / "session.json").read_text())
        self.assertEqual(session["mode"], 1)
        self.assertIn("tmux draft", session["drafts"].values())
