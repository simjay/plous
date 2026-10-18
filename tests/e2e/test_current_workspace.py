"""Launch-directory journeys use temporary sources, profiles, and kubeconfig."""

from pathlib import Path
import tempfile
import unittest

from support import Terminal


class CurrentWorkspaceTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory(prefix="plous-current-workspace-")
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        self.source = self.root / "launch-project"
        self.source.mkdir()
        (self.source / "README.md").write_text("Launch directory document\n")
        (self.source / "kustomization.yaml").write_text("resources: [service.yaml]\n")
        (self.source / "service.yaml").write_text(
            "apiVersion: v1\nkind: Service\nmetadata: {name: launch-service}\n"
            "spec: {ports: [{port: 80}]}\n")
        self.config = self.root / "kubeconfig"
        self.config.write_text("apiVersion: v1\nkind: Config\n")
        self.state = self.root / "state"

    def start(self, directory=None):
        app = Terminal(self.state, self.config, width=160, height=40,
                       cwd=directory or self.source)
        self.addCleanup(app.close)
        app.expect_screen("Current Workspace")
        app.expect_screen("No current context")
        return app

    def open_source(self, app, name="launch-project"):
        app.send("\x1b[1;3D/" + name + "\r")
        app.expect_screen(name + " / Files")
        app.send("\x1b[1;3C")
        app.expect_screen("README.md")

    def test_current_directory_editor_refresh_and_restart(self):
        before = self.config.read_bytes()
        app = self.start()
        self.open_source(app)
        self.assertFalse(any(self.state.joinpath("workspaces").iterdir()))
        app.send("/README.md\r\r")
        app.expect_screen("Launch directory document")
        app.send("iSaved from Current Workspace")
        app.send("\x1b")
        app.expect_screen("NORMAL · Unsaved changes")
        app.send(":w\r")
        app.expect_screen("Saved to local file")
        app.send(":q\r")
        app.send("ccDirectory draft survives restart")
        app.send("\x1b")
        app.wait(lambda: "Enter Send" not in app.screen()["rows"][-1], "chat navigation")
        app.send(":K\r")
        app.expect_screen("Choose Kubernetes connection")
        app.send("\x1b")
        app.expect_screen("launch-project / Files")
        app.send(":g\r")
        app.expect_screen("README.md")
        for width, height in [(80, 16), (120, 30), (180, 42)]:
            app.resize(width, height)
            app.expect_screen("Files")
            rows = app.capture_screen(f"current-workspace-{width}x{height}")
            self.assertEqual(len(rows), height)
            self.assertTrue(all(len(row) <= width for row in rows))
        app.send("\x03")
        app.assert_exit()
        self.assertIn("Saved from Current Workspace", (self.source / "README.md").read_text())
        restarted = self.start()
        self.open_source(restarted)
        restarted.expect_screen("Directory draft survives restart")
        restarted.send("\x03")
        restarted.assert_exit()
        self.assertEqual(self.config.read_bytes(), before)

    def test_save_directory_without_cluster(self):
        app = self.start()
        self.open_source(app)
        app.send(":s\r" + "\x1b[B" * 4)
        app.expect_screen("› Save as Workspace")
        app.send("\r")
        app.expect_screen("Workspace Name")
        app.send("\x15Saved directory\r\r")
        app.expect_screen("Workspace saved")
        app.expect_screen("launch-project / Files")
        saved = app.wait_json(self.state / "session.json", lambda s: s.get("workspace") and not s["workspace"].startswith("dir-"))
        self.assertEqual(saved["tool"], "files")
        app.send("\x03")
        app.assert_exit()

    def test_current_directory_renders_with_shared_kustomize_view(self):
        app = self.start()
        app.send("\x1b[1;3D/launch-project\r" + "\x1b[B" * 3 + "\r")
        app.expect_screen("launch-project / Kustomize")
        app.expect_screen("[Enter] Open group")
        app.expect_screen("Source root")
        app.send("\r")
        app.expect_screen("[Enter] Overview")
        app.send("\r")
        app.expect_screen("/ Overview")
        app.expect_screen("Not rendered yet")
        app.send("r")
        app.expect_screen("1 resource")
        app.expect_screen("launch-service")
        app.send("\x1b")
        app.expect_screen("/ Overview")
        app.send("\x03")
        app.assert_exit()

    def test_ignored_cache_and_empty_tool_views(self):
        (self.source / "kustomization.yaml").unlink()
        (self.source / "service.yaml").unlink()
        (self.source / ".gitignore").write_text(".cache/\n")
        cache = self.source / ".cache"
        cache.mkdir()
        for n in range(4097):
            (cache / str(n)).touch()
        app = self.start()
        self.open_source(app)
        app.send("\x1b[1;3D/Helm\r\r")
        app.expect_screen("No Helm configuration found")
        screen = "\n".join(app.screen()["rows"])
        self.assertNotIn("(stale)", screen)
        self.assertNotIn("scan failed", screen)
        app.send("\x03")
        app.assert_exit()
