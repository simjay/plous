"""Activity layout and navigation against immutable local synthetic history."""
import json
from pathlib import Path
import tempfile
import unittest

from support import FIXTURE, Terminal


class ActivityTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory(prefix="plous-activity-")
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        self.state = self.root / "state"
        self.config = self.root / "kubeconfig"
        self.config.write_bytes(FIXTURE.read_bytes())
        app = self.start()
        app.send(":new\rCommerce\r\r")
        app.expect("Add Connection")
        app.send("\x03")
        app.assert_exit()
        self.workspace = next((self.state / "workspaces").iterdir()).name
        operations = self.state / "workspaces" / self.workspace / "operations"
        operations.mkdir()
        base = dict(workspace_id=self.workspace, connection_id="fixture-cluster", connection_fingerprint="fixture-config",
                    tool="helm", context="fixture", namespace="apps", release="checkout", kind="revision", revision=1,
                    source_time="2026-09-30T12:00:00Z", observed_at="2026-10-03T12:00:00Z", status="deployed", chart="checkout", version="1.0.0")
        records = [dict(base, id="first"),
                   dict(base, id="second", status="superseded", observed_at="2026-10-04T12:00:00Z"),
                   dict(base, id="new-revision", revision=2, source_time="2026-10-01T12:00:00Z"),
                   dict(base, id="fallback", release="missing-time", source_time="invalid"),
                   dict(base, id="attempt", kind="install", release="payment-api", status="submitted")]
        for record in records:
            (operations / (record["id"] + ".json")).write_text(json.dumps(record))
        self.records = {p: p.read_bytes() for p in (self.state / "workspaces").rglob("*.json")}

    def start(self):
        app = Terminal(self.state, self.config, width=240, height=48)
        self.addCleanup(app.close)
        app.expect("AI assistant")
        if hasattr(self, "workspace"):
            app.send("\x1b[1;3D/Commerce\r\x1b[1;3C")
        return app

    def test_timeline_preview_types_search_observations_and_resize(self):
        app = self.start()
        app.send(":3\r")
        app.expect_screen("All activity")
        app.expect_screen("Preview")
        app.expect_screen("5 entries")
        app.capture_screen("activity-wide")
        app.send("\x1b[C")
        app.expect_screen("[Plous actions]")
        app.expect_screen("Install payment-api")
        app.expect_screen("Outcome unknown")
        app.send("\x1b[C\x1b[C")
        app.expect_screen("[Observed Helm history]")
        app.expect_screen("3 entries")
        app.expect_screen("observation time")
        app.send("/checkout")
        app.expect_screen("2 entries")
        app.send("\x1b[B\r")
        app.expect_screen("O Observations")
        app.send("O")
        app.expect_screen("Record: first")
        app.expect_screen("Record: second")
        app.capture_screen("activity-observations")
        app.send("\x1b")
        app.expect_screen("Search activity")
        app.expect_screen("2 entries")
        app.expect_screen("superseded")
        app.send("\x1b")
        app.expect_screen("3 entries")
        for width, height in ((80, 16), (120, 30), (200, 50), (240, 60)):
            app.resize(width, height)
            app.expect_screen("Observed Helm")
            rows = app.capture_screen(f"activity-{width}x{height}")
            self.assertEqual(len(rows), height)
            if width <= 120:
                self.assertNotIn("Preview", "\n".join(rows))
            self.assertTrue(any("›" in row for row in rows))
        app.send("\r")
        app.expect_screen("Release unavailable")
        app.send("\x03")
        app.assert_exit()
        self.assertEqual({p: p.read_bytes() for p in self.records}, self.records)
        self.assertEqual(self.config.read_bytes(), FIXTURE.read_bytes())

    def test_no_matches_recovers_without_losing_type(self):
        app = self.start()
        app.send(":3\r")
        app.expect_screen("5 entries")
        app.send("\x1b[C/no-such-release")
        app.expect_screen("No matching activity")
        app.capture_screen("activity-no-matches")
        app.send("\x1b")
        app.expect_screen("Install payment-api")
        app.expect_screen("[Plous actions]")
        app.send("g")
        app.expect_screen("1 entry")
        app.send("\x03")
        app.assert_exit()

    def test_compact_observations_and_search_return(self):
        app = self.start()
        app.send(":3\r")
        app.expect_screen("5 entries")
        app.send("\x1b[C\x1b[C\x1b[C/checkout")
        app.expect_screen("2 entries")
        app.send("\x1b[B\r")
        app.resize(80, 16)
        app.expect_screen("O Observations")
        app.expect_screen("Release unavailable")
        app.send("O")
        app.expect_screen("superseded")
        app.capture_screen("activity-compact-observations")
        app.send("\x1b[F")
        app.expect_screen("no longer saved")
        app.send("\x1b[H")
        app.expect_screen("superseded")
        app.send("\x1b")
        app.expect_screen("Search activity")
        app.expect_screen("2 entries")
        app.send("\x1b")
        app.expect_screen("3 entries")
        app.send("\x03")
        app.assert_exit()
        self.assertEqual({p: p.read_bytes() for p in self.records}, self.records)

    def test_compact_load_error_details_and_retry(self):
        app = self.start()
        app.send(":3\r")
        app.expect_screen("5 entries")
        record = self.state / "workspaces" / self.workspace / "operations" / "first.json"
        original = record.read_bytes()
        record.write_text("invalid fixture")
        app.resize(80, 16)
        app.send("g")
        app.expect_screen("g Retry")
        app.expect_screen("d Error details")
        app.capture_screen("activity-compact-load-error")
        app.send("d")
        app.expect_screen("invalid operation record")
        app.send("\x1b")
        app.expect_screen("g Retry")
        record.write_bytes(original)
        app.send("g")
        app.wait(lambda: "Retry" not in "\n".join(app.screen()["rows"]), "successful history retry")
        app.expect_screen("5 entries")
        app.send("\x03")
        app.assert_exit()
        self.assertEqual({p: p.read_bytes() for p in self.records}, self.records)
