"""Real-terminal journeys for input routing, persistence, and clean shutdown."""

import json
from pathlib import Path
import tempfile
import unittest

from support import FIXTURE, Terminal


class TerminalTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory(prefix="plous-terminal-")
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        self.state = self.root / "state"
        self.config = self.root / "kubeconfig"
        self.original = FIXTURE.read_bytes()
        self.config.write_bytes(self.original)

    def start(self, state=None, ready=True):
        app = Terminal(state or self.state, self.config)
        self.addCleanup(app.close)
        if ready:
            app.expect("AI assistant")
        return app

    def history(self, state=None):
        return [json.loads(path.read_text()) for path in (state or self.state).glob("workspaces/*/revisions/*/change.json")]

    def create_workspace(self, app, name, state=None):
        state = state or self.state
        count = len(self.history(state))
        app.send(":n\r")
        app.expect("Create workspace")
        app.send(name + "\r\r")
        app.wait(lambda: len(self.history(state)) == count + 1, "workspace creation")
        return next(c["workspace_id"] for c in self.history(state) if c["after"]["name"] == name)

    def test_workspace_kubernetes_activity_and_restart(self):
        app = self.start()
        first = self.create_workspace(app, "Payments")
        app.send("\r")
        app.expect("Add tool")
        app.send("\r")
        app.expect("Kubeconfig path")
        app.send("\r\r")
        app.expect("Choose kubecontexts")
        app.send(" \x1b[B \r")
        app.wait(lambda: len(self.history()) == 2, "two saved kubecontexts")
        added = max(self.history(), key=lambda c: c["revision"])
        self.assertEqual([c["settings"]["context"] for c in added["after"]["connections"]], ["dev", "prod"])

        app.send(":settings\r")
        app.send("\r")
        app.expect("Rename workspace")
        app.send("\x15Payments renamed\r\r")
        app.wait(lambda: len(self.history()) == 3, "workspace rename")
        renamed = max(self.history(), key=lambda c: c["revision"])
        self.assertEqual(renamed["before"]["name"], "Payments")
        self.assertEqual(renamed["after"]["name"], "Payments renamed")
        app.send(":activity\r")
        app.send("\r")
        app.expect("BEFORE")
        for _ in range(6):
            if "AFTER" in app.text():
                break
            app.send("\x1b[6~")
        app.expect("AFTER")
        app.send("\x1b")
        draft = "Draft :q and /commands stay literal\n:4"
        app.send("\t\x1b[200~" + draft + "\x1b[201~")
        app.wait_json(self.state / "session.json", lambda s: s["drafts"].get(first) == draft)
        app.send("\x1b[Z")
        second = self.create_workspace(app, "Platform")
        app.send(":q\r")
        app.assert_exit()
        app.close()

        restarted = self.start()
        restarted.expect("Platform")
        restarted.send(":quit\r")
        restarted.assert_exit()
        saved = json.loads((self.state / "session.json").read_text())
        self.assertEqual(saved["workspace"], second)
        self.assertEqual(saved["mode"], 3)
        self.assertEqual(saved["drafts"][first], draft)
        self.assertEqual(len(self.history()), 4)
        self.assertEqual(self.config.read_bytes(), self.original)
        for path in self.state.rglob("*.json"):
            self.assertNotIn("fixture-only-never-copy-this-token", path.read_text())

    def test_colon_encodings_and_mode_aliases(self):
        app = self.start()
        self.create_workspace(app, "Commands")
        app.send(":o\r")
        for sequence in [":", "\x1b[58u", "\x1b[58;2u", "\x1b[59:58;2u", "\x1b[59;2;58u", "\x1b[27;2;58~"]:
            with self.subTest(sequence=repr(sequence)):
                start = len(app.output)
                app.send(sequence)
                app.expect(":▏", start)
                self.assertIn("╭", app.text(start))
                start = len(app.output)
                app.send("\x1b")
                app.expect(":1", start)
        for command, mode in [("r", 1), ("deployment", 2), ("4", 3), ("overview", 0)]:
            app.send(":" + command + "\r")
            app.wait_json(self.state / "session.json", lambda s: s["mode"] == mode)
        app.send("\x03")
        app.assert_exit()

    def test_ctrl_c_in_each_panel_and_editor(self):
        for state_name in ["first-run", "main", "tree", "chat", "command", "form", "small-chat"]:
            with self.subTest(state=state_name):
                state = self.root / state_name
                app = self.start(state)
                if state_name != "first-run":
                    self.create_workspace(app, "Stable name", state)
                    app.send(":o\r")
                if state_name == "tree":
                    app.send("\x1b[Z")
                elif state_name in ["chat", "small-chat"]:
                    app.send("\tSaved draft")
                elif state_name == "command":
                    app.send(":new")
                    app.expect(":new▏")
                elif state_name == "form":
                    app.send(":s\r")
                    app.send("\r")
                    app.send("\x15Unsubmitted name")
                if state_name == "small-chat":
                    app.resize(60, 10)
                    app.expect("Resize terminal")
                app.send("\x03")
                app.assert_exit()
                app.close()
                saved = json.loads((state / "session.json").read_text())
                if state_name in ["chat", "small-chat"]:
                    self.assertIn("Saved draft", saved["drafts"].values())
                if state_name != "first-run":
                    self.assertEqual(len(self.history(state)), 1)
                    self.assertEqual(self.history(state)[0]["after"]["name"], "Stable name")
                else:
                    self.assertEqual(self.history(state), [])

    def test_lock_and_corrupt_session_report_errors(self):
        first = self.start()
        second = self.start(ready=False)
        second.assert_exit(expected=1)
        self.assertIn("another Plous process", second.text())
        second.close()
        first.send("\x03")
        first.assert_exit()
        first.close()
        session = self.state / "session.json"
        session.write_text("{corrupt")
        broken = self.start(ready=False)
        broken.assert_exit(expected=1)
        self.assertIn("session", broken.text())
        self.assertEqual(session.read_text(), "{corrupt")
