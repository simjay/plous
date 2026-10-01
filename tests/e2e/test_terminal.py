"""Terminal tests for input routing, persistence, and shutdown."""

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
        return [json.loads(path.read_text()) for path in (state or self.state).glob("workspaces/*/revisions/*/change.json") if not path.parent.name.startswith(".")]

    def create_workspace(self, app, name, state=None):
        state = state or self.state
        count = len(self.history(state))
        app.send(":n\r")
        app.expect("Create workspace")
        app.send(name + "\r\r")
        app.wait(lambda: len(self.history(state)) == count + 1, "workspace creation")
        return next(c["workspace_id"] for c in self.history(state) if c["after"]["name"] == name)

    def test_panel_search_keeps_escape_and_drafts_local(self):
        app = self.start()
        workspace = self.create_workspace(app, "Commerce")
        app.send("\x1b")
        app.send(":1\r")
        app.send("\x1b[Z/commerce")
        app.expect("Search tree")
        app.send("\x15ii11:q")
        app.expect("No matches.")
        self.assertIsNone(app.process.poll())
        app.send("\x1b")
        app.send("\t/connections")
        app.expect("Find")
        app.send("\x1b")
        app.send(":H\r/")
        app.expect("Search conversations")
        app.send("missing")
        app.expect("No matching conversations.")
        app.send("\x1b")
        app.expect("Conversations")
        app.send("\x1b")
        app.send("cc/keep this draft")
        app.send("\x03")
        app.assert_exit()
        session = json.loads((self.state / "session.json").read_text())
        self.assertEqual(session["drafts"][workspace], "/keep this draft")
        self.assertNotIn("ii11:q", json.dumps(session))
        self.assertEqual(self.config.read_bytes(), self.original)

    def test_workspace_kubernetes_activity_and_restart(self):
        app = self.start()
        first = self.create_workspace(app, "Payments")
        app.expect("Add tool")
        app.send("\r")
        app.expect("Add Kubernetes contexts")
        app.send(" \x1b[B \x1b[B\r")
        app.wait(lambda: len(self.history()) == 2, "two saved kubecontexts")
        added = max(self.history(), key=lambda c: c["revision"])
        self.assertEqual([c["settings"]["context"] for c in added["after"]["connections"]], ["dev", "prod"])

        app.send(":settings\r")
        app.send("\x1b[B\r")
        app.expect("Rename workspace")
        app.send("\x15Payments renamed\r\r")
        app.wait(lambda: len(self.history()) == 3, "workspace rename")
        renamed = max(self.history(), key=lambda c: c["revision"])
        self.assertEqual(renamed["before"]["name"], "Payments")
        self.assertEqual(renamed["after"]["name"], "Payments renamed")
        app.send(":3\r")
        app.send("\r")
        app.expect("Workspace name")
        app.expect("Before  Payments")
        for _ in range(6):
            if "After   Payments renamed" in app.text():
                break
            app.send("\x1b[6~")
        app.expect("After   Payments renamed")
        app.send("\x1b")
        draft = "Draft :q and /commands stay literal\n:4"
        app.send(":c\r\x1b[200~" + draft + "\x1b[201~")
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
        self.assertEqual(saved["mode"], 2)
        self.assertEqual(saved["drafts"][first], draft)
        self.assertEqual(len(self.history()), 4)
        self.assertEqual(self.config.read_bytes(), self.original)
        for path in self.state.rglob("*.json"):
            self.assertNotIn("fixture-only-never-copy-this-token", path.read_text())

    def test_workspace_tool_actions_and_another_kubeconfig(self):
        app = self.start()
        self.create_workspace(app, "Connections")
        app.send("\x1b")
        app.expect("Enter  Add tool")
        app.send("\r\r")
        app.expect("Add Kubernetes contexts")
        app.expect("Kubeconfig file")
        app.expect("~/.kube/config")
        app.expect("Refresh contexts")
        app.send(" \x1b[B\x1b[B\r")
        app.wait(lambda: len(self.history()) == 2, "default context added")
        app.expect("+ Add context")

        app.send("\x1b[B\r")
        app.expect("Already added")
        start = len(app.output)
        app.send("\x1b[H\x15")
        extra = self.root / "another.yaml"
        extra.write_text("contexts:\n- name: another-context\n  context: {cluster: separate, namespace: example}\n")
        app.send("\x1b[200~" + str(extra) + "\x1b[201~")
        app.expect("Refresh required", start)
        app.expect("disabled", start)
        app.send("\x1b[B\x1b[B\r")
        app.expect("Refresh contexts before adding")
        self.assertEqual(len(self.history()), 2)
        app.send("\x1b[A\r")
        app.expect("[ ] another-context")
        app.send(" \x1b[B\r")
        app.wait(lambda: len(self.history()) == 3, "context from another file added")
        saved = max(self.history(), key=lambda change: change["revision"])["after"]["connections"]
        self.assertEqual([c["settings"]["context"] for c in saved], ["dev", "another-context"])
        self.assertEqual(saved[1]["settings"]["kubeconfig"], str(extra))
        self.assertEqual(self.config.read_bytes(), self.original)
        app.send("\x03")
        app.assert_exit()

    def test_colon_encodings_and_numeric_modes(self):
        app = self.start()
        self.create_workspace(app, "Commands")
        app.send(":1\r")
        for sequence in [":", "\x1b[58u", "\x1b[58;2u", "\x1b[59:58;2u", "\x1b[59;2;58u", "\x1b[27;2;58~"]:
            with self.subTest(sequence=repr(sequence)):
                start = len(app.output)
                app.send(sequence)
                app.expect(":▏", start)
                self.assertIn("╭", app.text(start))
                start = len(app.output)
                app.send("\x1b")
                app.expect("Overview  Enter Add tool", start)
        for command, mode in [("2", 1), ("3", 2), ("1", 0)]:
            app.send(":" + command + "\r")
            app.wait_json(self.state / "session.json", lambda s: s["mode"] == mode)
        app.send("\x03")
        app.assert_exit()

    def test_global_help_from_each_panel_and_back(self):
        app = self.start()
        for focus, move, command in [(0, "\x1b[Z", "hh"), (1, "\t", ":h\r"), (2, "\t", ":help\r")]:
            with self.subTest(focus=focus):
                app.send(move)
                app.wait_json(self.state / "session.json", lambda s: s["focus"] == focus)
                start = len(app.output)
                app.send(command)
                app.expect("4 Global", start)
                app.expect(":stopchat", start)
                app.send("\t\x1b[Z\x1b[C\x1b[6~")
                app.send("\x1b")
                app.wait_json(self.state / "session.json", lambda s: s["focus"] == focus)
                self.assertEqual(self.history(), [])
        app.send(":s\r")
        app.expect("AI connections")
        app.send(":q\r")
        app.assert_exit()

    def test_settings_and_help_trap_tab(self):
        app = self.start()
        app.send(":s\r\t\r")
        app.expect("Vendor")
        app.send("\x1b[Btest\t-key\x1b[Z-end")
        self.assertNotIn("test-key-end", app.text())
        app.send("\x1b")
        self.assertFalse((self.state / "credentials.json").exists())
        app.send("\x1b")
        app.send(":h\r")
        app.expect("4 Global")
        for key in ["\t", "\x1b[Z"]:
            app.send(key)
            app.read(0.7)
            self.assertEqual(json.loads((self.state / "session.json").read_text())["focus"], 1)
        app.send("\x03")
        app.assert_exit()

    def test_panel_toggles_focus_and_restart(self):
        app = self.start()
        workspace = self.create_workspace(app, "Layout")
        app.send(":c\rKeep this draft")
        app.send("\x1b")
        app.send(":i\r:b\r\t\x1b[Z")
        app.wait_json(self.state / "session.json", lambda s: s.get("left_hidden") and s.get("right_hidden") and s["focus"] == 1)
        app.send("\x03")
        app.assert_exit()
        app.close()

        restarted = self.start(ready=False)
        restarted.expect("plous")
        restarted.send("\t\x1b[Z")
        restarted.wait_json(self.state / "session.json", lambda s: s.get("left_hidden") and s.get("right_hidden") and s["focus"] == 1)
        restarted.send(":sidebar\r\x1b[Z")
        restarted.wait_json(self.state / "session.json", lambda s: not s.get("left_hidden") and s["focus"] == 0)
        restarted.send(":chat\r")
        restarted.wait_json(self.state / "session.json", lambda s: not s.get("right_hidden") and s["focus"] == 2)
        restarted.send("\x1b")
        restarted.send(":aiinteraction\r")
        restarted.wait_json(self.state / "session.json", lambda s: s.get("right_hidden") and s["focus"] == 1)
        restarted.send("\x03")
        restarted.assert_exit()
        saved = json.loads((self.state / "session.json").read_text())
        self.assertEqual(saved["drafts"][workspace], "Keep this draft")
        self.assertEqual(len(self.history()), 1)
        self.assertEqual(self.config.read_bytes(), self.original)

    def test_chat_entry_from_form_action_and_pending_command(self):
        app = self.start()
        app.send(":i\r:c\r")
        app.wait_json(self.state / "session.json", lambda s: s["focus"] == 2 and not s.get("right_hidden"))
        self.assertEqual(self.history(), [])
        app.send("\x1b")
        workspace = self.create_workspace(app, "Chat entry")
        app.send(":s\r\x1b[B\r")
        app.expect("Rename workspace")
        app.send("\x15Unsubmitted name\r:c\rTyped from form")
        app.wait_json(self.state / "session.json", lambda s: s["focus"] == 2 and s["drafts"].get(workspace) == "Typed from form")
        self.assertEqual(len(self.history()), 1)
        self.assertEqual(self.history()[0]["after"]["name"], "Chat entry")
        app.send("\x1b")
        app.send("\x1b[Z")
        app.send("\x1b")
        app.send("\x1b")
        app.send(":s\x1b[Z:c\r and from tree")
        app.wait_json(self.state / "session.json", lambda s: s["focus"] == 2 and s["drafts"].get(workspace) == "Typed from form and from tree")
        app.send("\x03")
        app.assert_exit()

    def test_double_keys_and_literal_input(self):
        app = self.start()
        workspace = self.create_workspace(app, "Shortcuts")
        app.send(":1\r")
        start = len(app.output)
        app.send("b")
        app.expect("b → b toggle sidebar", start)
        app.send("bii")
        app.wait_json(self.state / "session.json", lambda s: s.get("left_hidden") and s.get("right_hidden"))
        app.send("ss")
        app.expect("AI connections")
        app.send("ccbb ii ss cc hh 11 22 33 44")
        app.wait_json(self.state / "session.json", lambda s: s["focus"] == 2 and not s.get("right_hidden") and s["drafts"].get(workspace) == "bb ii ss cc hh 11 22 33 44")
        app.send("\x1b")
        app.send(":sidebar\r")
        app.wait_json(self.state / "session.json", lambda s: not s.get("left_hidden"))
        app.send("b")
        app.send("\x1b")
        app.send("b")
        app.send("\x1b")
        app.send("\x1b[200~bb ii ss cc hh 11 22 33 44\x1b[201~")
        app.send(":aiinteraction\r")
        app.wait_json(self.state / "session.json", lambda s: s.get("right_hidden") and not s.get("left_hidden"))
        app.send("ii")
        app.wait_json(self.state / "session.json", lambda s: not s.get("right_hidden"))
        for sequence, mode in [("22", 1), ("33", 2), ("11", 0)]:
            app.send(sequence)
            app.wait_json(self.state / "session.json", lambda s: s["mode"] == mode and s["focus"] == 1)
        app.send("b\x03")
        app.assert_exit()
        saved = json.loads((self.state / "session.json").read_text())
        self.assertFalse(saved.get("left_hidden"))
        self.assertEqual(saved["drafts"][workspace], "bb ii ss cc hh 11 22 33 44")

    def test_ctrl_c_in_each_panel_and_editor(self):
        for state_name in ["first-run", "main", "tree", "chat", "command", "form", "small-chat"]:
            with self.subTest(state=state_name):
                state = self.root / state_name
                app = self.start(state)
                if state_name != "first-run":
                    self.create_workspace(app, "Stable name", state)
                    app.send(":1\r")
                if state_name == "tree":
                    app.send("\x1b[Z")
                elif state_name in ["chat", "small-chat"]:
                    app.send(":c\rSaved draft")
                elif state_name == "command":
                    app.send(":new")
                    # Rendering can update only the newly typed character, so the
                    # byte stream need not contain the whole command contiguously.
                    app.expect("Enter Run · Esc Cancel")
                elif state_name == "form":
                    app.send(":s\r")
                    app.send("\x1b[B\r")
                    app.send("\x15Unsubmitted name")
                if state_name == "small-chat":
                    app.wait_json(state / "session.json", lambda s: "Saved draft" in s.get("drafts", {}).values())
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
