"""Overview drilldowns through a real PTY and a temporary Kubernetes API."""

from test_explore import ExploreFixture


class OverviewTests(ExploreFixture):
    def test_metric_clicks_sources_polling_and_resource_return(self):
        self.metrics_enabled = True
        self.node = {
            "apiVersion": "v1", "kind": "Node",
            "metadata": {"name": "checkout-node", "uid": "node-id", "resourceVersion": "10"},
            "status": {"allocatable": {"cpu": "2", "memory": "4Gi"},
                       "capacity": {"cpu": "2", "memory": "4Gi"}},
        }
        self.pod["spec"]["containers"][0]["resources"] = {
            "requests": {"cpu": "250m", "memory": "64Mi"},
        }
        app = self.open_resource()
        app.resize(180, 45)
        app.send(":1\rff")
        app.expect_screen("Cluster Pulse")
        app.expect_screen("25% used")

        def click_text(text):
            app.expect_screen(text)
            for y, row in enumerate(app.screen()["rows"]):
                if text in row:
                    app.mouse_click(row.index(text) + len(text) // 2, y)
                    return
            self.fail("missing click target " + text)

        click_text("CPU")
        app.expect_screen("CPU · All namespaces")
        app.expect_screen("Coverage: 1/1 Nodes")
        app.expect_screen("missing CPU limits")
        self.assertNotIn("Overview evidence", "\n".join(app.screen()["rows"]))
        reads = self.metrics_reads
        app.wait(lambda: self.metrics_reads > reads, "metric detail continues sampling", timeout=12)
        app.capture_screen("overview-cpu-drilldown")
        click_text("Source details")
        app.expect_screen("CPU · Source details")
        app.expect_screen("Oldest source:")
        app.send("/ff:cc")
        app.expect_screen("ff:cc")
        app.send("\x1b")
        app.expect_screen("CPU · Source details")
        app.send("\x1b")
        app.expect_screen("CPU · All namespaces")
        app.send("hh")
        app.expect_screen("Global · Navigation")
        app.send("\x1b")
        app.expect_screen("CPU · All namespaces")
        app.send("\x1b")
        app.expect_screen("Cluster Pulse")

        click_text("Memory")
        app.expect_screen("Memory · All namespaces")
        app.expect_screen("missing memory limits")
        app.capture_screen("overview-memory-drilldown")
        for width, height in ((80, 16), (120, 30), (240, 60)):
            app.resize(width, height)
            app.expect_screen("Memory · All namespaces")
            app.send("\x1b[F")
            app.expect_screen("unbounded")
            app.send("\x1b[H")
            app.capture_screen(f"overview-memory-{width}x{height}")
        app.send("\x1b")
        app.expect_screen("Cluster Pulse")
        click_text("Pods")
        app.expect_screen("checkout-test")
        app.send("\x1b")
        app.expect_screen("Cluster Pulse")
        app.send("\x03")
        app.assert_exit()
        self.assertFalse(self.mutations)
        self.assertEqual(self.config.read_bytes(), self.original)

    def test_unavailable_metric_in_reduced_color(self):
        self.terminal_env = {"TERM": "xterm", "COLORTERM": "", "NO_COLOR": "1"}
        app = self.open_resource()
        app.send(":1\rff\r")
        app.expect_screen("CPU · All namespaces")
        app.expect_screen("Usage unavailable")
        app.expect_screen("Coverage:")
        app.send("\r")
        app.expect_screen("CPU · Source details")
        app.expect_screen("Metrics")
        app.capture_screen("overview-metric-unavailable-no-color")
        app.send("\x1b")
        app.expect_screen("CPU · All namespaces")
        app.send("\x1b")
        app.expect_screen("Cluster Pulse")
        app.send("\x03")
        app.assert_exit()
        self.assertFalse(self.mutations)
