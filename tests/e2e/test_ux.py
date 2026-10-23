"""Current-screen UX acceptance with temporary profiles and no live providers."""

import base64
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

from support import FIXTURE, ROOT, Terminal


class UXTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory(prefix="plous-ux-")
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        self.state = self.root / "state"
        self.config = self.root / "kubeconfig"
        self.config.write_bytes(FIXTURE.read_bytes())

    def start(self):
        app = Terminal(self.state, self.config)
        self.addCleanup(app.close)
        app.expect_screen("AI assistant")
        return app

    def test_fresh_and_saved_startup_renders_without_input(self):
        for attempt, (width, height) in enumerate(((80, 16), (120, 30), (240, 60))):
            state = self.root / f"startup-{attempt}"
            for launch in range(2):
                with self.subTest(size=(width, height), launch=launch):
                    app = Terminal(state, self.config, width=width, height=height)
                    try:
                        app.expect_screen("AI assistant")
                        app.expect_screen("Current Workspace")
                        if launch == 0:
                            app.send(":n\rStartup check\r\r")
                            app.expect_screen("Add Connection")
                        app.send("\x03")
                        app.assert_exit()
                    finally:
                        app.close()

    def test_screen_replay_tracks_overwrites_erases_resizes_and_unicode(self):
        def data(text):
            return {"Data": base64.b64encode(text.encode()).decode()}

        events = [
            {"Width": 20, "Height": 6},
            data("\x1b[?1049h\x1b[2J\x1b[Hobsolete\x1b[2;1Hchart-00"),
            data("\x1b[2;7H12\x1b[1;1H\x1b[2KReady 界"),
            {"Width": 24, "Height": 8},
            data("\x1b[8;1Hfooter"),
        ]
        result = subprocess.run(
            [str(ROOT / "bin/test-terminalscreen")], input=json.dumps(events),
            capture_output=True, text=True, check=True,
        )
        rows = json.loads(result.stdout)["rows"]
        self.assertEqual(len(rows), 8)
        self.assertEqual(rows[0].rstrip(), "Ready 界")
        self.assertEqual(rows[1].rstrip(), "chart-12")
        self.assertEqual(rows[-1].rstrip(), "footer")
        self.assertNotIn("obsolete", "\n".join(rows))

    def test_connection_expansion_keeps_tool_branches_collapsed(self):
        from test_helm import HelmAPI
        api = HelmAPI(self.root)
        self.addCleanup(api.close)
        original = FIXTURE.read_text().replace("https://127.0.0.1:1", api.url).encode()
        self.config.write_bytes(original)
        app = self.start()
        app.send(":n\rTree workspace\r\r")
        app.expect_screen("Add Connection")
        app.send("\r")
        app.expect_screen("Add Context")
        app.send(" \x1b[B\x1b[B\r")
        app.expect_screen("Added 1 Kubernetes")
        app.send(":1\r")
        app.send("\x1b")
        app.send("/dev\r")

        def expect_collapsed_tools(terminal):
            terminal.expect_screen("▸ Kubernetes")
            terminal.expect_screen("▸ Helm")
            sidebar = "\n".join(row[:30] for row in terminal.screen()["rows"])
            for section in ("Releases", "Charts", "Discover"):
                self.assertNotIn(section, sidebar)

        expect_collapsed_tools(app)
        app.send("\x1b[D\x1b[C")
        expect_collapsed_tools(app)
        app.send("\x03")
        app.assert_exit()
        app.close()

        connection = json.loads((self.state / "session.json").read_text())["connection"]
        restarted = self.start()
        restarted.expect_screen("Current Workspace")
        restarted.send("\x1b[1;3D\x1b[C")
        expect_collapsed_tools(restarted)
        restarted.send("\x1b[B\x1b[B\x1b[C")
        restarted.expect_screen("▾ Helm")
        restarted.expect_screen("Releases")
        restarted.expect_screen("Charts")
        self.assertNotIn("Discover", "\n".join(restarted.screen()["rows"]))
        restarted.expect_screen("▸ Kubernetes")
        restarted.send("\x1b[A\x1b[A\x1b[D\x1b[C")
        expect_collapsed_tools(restarted)
        restarted.send("\x03")
        restarted.assert_exit()
        restarted.close()

        # Earlier sessions can have an open Helm branch under a closed connection.
        session_path = self.state / "session.json"
        saved = json.loads(session_path.read_text())
        saved["connection"], saved["tool"] = connection, "kubernetes"
        saved["expanded"][connection] = False
        saved["expanded"][connection + "/view/helm"] = True
        saved["expanded"][connection + "/view/kubernetes"] = False
        saved["tree_key"], saved["focus"] = connection, 0
        session_path.write_text(json.dumps(saved))
        restored = self.start()
        restored.send("\x1b[1;3D\x1b[C")
        expect_collapsed_tools(restored)
        restored.capture_screen("saved-helm-connection-reopened")
        restored.send("\x03")
        restored.assert_exit()
        self.assertEqual(self.config.read_bytes(), original)

    def test_startup_current_context_keeps_connection_draft_without_reusing_it(self):
        app = self.start()
        app.expect_screen("Current Workspace")
        app.send(":n\rStartup workspace\r\r")
        app.expect_screen("Add Connection")
        app.send("\r")
        app.expect_screen("Add Context")
        app.send(" \x1b[B\x1b[B\r")
        app.expect_screen("Added 1 Kubernetes")
        app.send(":1\r")
        app.send("\x1b")
        app.send("/dev\r")
        app.send("ccKeep this connection draft")
        app.wait_json(self.state / "session.json", lambda s: "Keep this connection draft" in s["drafts"].values())
        app.send("\x03")
        app.assert_exit()
        app.close()
        session_path = self.state / "session.json"
        saved = json.loads(session_path.read_text())
        scope = next(key for key, value in saved["drafts"].items() if value == "Keep this connection draft")
        self.assertTrue(scope.startswith("conn-"), "draft must belong to the saved connection")
        for mode in range(3):
            with self.subTest(previous_tab=mode):
                saved["mode"], saved["focus"] = mode, 2
                session_path.write_text(json.dumps(saved))
                restored = self.start()
                try:
                    restored.expect_screen("Current Workspace")
                    self.assertNotIn("Keep this connection draft", "\n".join(restored.screen()["rows"]))
                    restored.expect_screen("▸ dev")
                    for width, height in [(80, 16), (180, 42), (120, 30)]:
                        restored.resize(width, height)
                        restored.expect_screen("Current Workspace")
                        rows = restored.capture_screen(f"startup-logo-{mode}-{width}x{height}")
                        self.assertEqual(len(rows), height)
                        self.assertEqual(rows[-2].count("╯"), 3)
                        self.assertNotIn("Workspace overview", "\n".join(rows))
                    restored.send("\x03")
                    restored.assert_exit()
                finally:
                    restored.close()
                after = json.loads(session_path.read_text())
                self.assertEqual(after["mode"], 0)
                self.assertEqual(after["connection"], "")
                self.assertEqual(after["tool"], "")
                self.assertEqual(after["workspace"], "")
                self.assertEqual(after["drafts"][scope], "Keep this connection draft")

    def test_launch_resize_help_form_and_restart_preserve_visible_draft(self):
        app = self.start()
        app.send("ccUX draft")
        app.send("\x1b")
        for width, height in [(80, 16), (80, 24), (120, 30), (180, 42), (240, 60)]:
            app.resize(width, height)
            app.expect_screen("Current Workspace")
            app.expect_screen("UX draft")
            rows = app.capture_screen(f"terminal-launch-{width}x{height}")
            self.assertEqual(len(rows), height)
            self.assertTrue(rows[0].lstrip().startswith("plous"))
            self.assertEqual(rows[-2].count("╯"), 3, "a panel lost its bottom border")
            self.assertIn("AI assistant", rows[-1], "footer left its reserved row")

        app.resize(60, 12)
        app.expect_screen("Resize terminal to at least 80 x 16")
        app.resize(120, 30)
        app.expect_screen("UX draft")
        app.send(":n\rUnfinished UX\r")
        app.expect_screen("Create Workspace")
        app.send(":h\r")
        app.expect_screen("Global")
        app.resize(80, 16)
        app.expect_screen("Global")
        app.capture_screen("terminal-help-over-form-80x16")
        app.send("\x1b")
        app.expect_screen("Unfinished UX")
        app.capture_screen("terminal-restored-form-80x16")
        app.send("\x03")
        app.assert_exit()
        app.close()

        restarted = self.start()
        restarted.expect_screen("Current Workspace")
        restarted.expect_screen("UX draft")
        self.assertNotIn("Unfinished UX", "\n".join(restarted.screen()["rows"]))
        self.assertFalse(list(self.state.glob("workspaces/*/revisions/*")))
        self.assertEqual(self.config.read_bytes(), FIXTURE.read_bytes())
        restarted.send(":f\r")
        restarted.expect_screen("Restore layout")
        self.assertNotIn("AI assistant", "\n".join(restarted.screen()["rows"]))
        restarted.send("\x1b")
        restarted.expect_screen("UX draft")
        restarted.send("\x03")
        restarted.assert_exit()
