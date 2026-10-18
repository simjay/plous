"""Live usage through the executable and a local Metrics API fixture."""

from test_explore import ExploreFixture


class UsageTests(ExploreFixture):
    def test_usage_remains_readable_without_color(self):
        self.terminal_env = {"TERM": "xterm", "COLORTERM": "", "NO_COLOR": "1"}
        self.metrics_enabled = True
        app = self.open_resource()
        app.resize(180, 48)
        app.expect_screen("32.0Mi")
        app.send("\r")
        app.expect_screen("[I] Inspect charts")
        app.send("I")
        app.expect_screen("Range 15m")
        app.expect_screen("Sampled")
        app.capture_screen("charts-without-color")
        app.send("W")
        app.expect_screen("Range 5m")
        app.send("\x03")
        app.assert_exit()
        self.assertFalse(self.mutations)

    def test_pod_usage_polls_and_charts_without_persisting_samples(self):
        self.metrics_enabled = True
        app = self.open_resource()
        app.resize(180, 48)
        app.expect("32.0Mi")
        app.send("\r")
        app.expect("Live usage")
        app.send("I")
        app.expect("15m ago")
        app.expect("Refresh 15s")
        app.expect("Sampled ")
        reads = self.metrics_reads
        app.wait(lambda: self.metrics_reads > reads, "next live metrics sample", timeout=20)
        app.send("\x03")
        app.assert_exit()
        self.assertFalse(self.mutations)
        self.assertEqual(self.config.read_bytes(), self.original)
        for path in self.state.rglob("*.json*"):
            self.assertNotIn("PodMetrics", path.read_text())
            self.assertNotIn("32.0Mi", path.read_text())

    def test_workload_usage_aggregates_owned_pods(self):
        self.metrics_enabled = True
        app = self.open_resource("deployments")
        app.resize(180, 48)
        app.send("\r")
        app.expect("Live usage")
        app.send("I")
        app.expect("Metrics from 1/1 active Pods")
        app.expect("32.0Mi")
        app.send("\x03")
        app.assert_exit()
        self.assertFalse(self.mutations)

    def test_denied_metrics_preserve_resource_browsing(self):
        self.metrics_enabled = True
        self.metrics_denied = True
        app = self.open_resource()
        app.resize(180, 48)
        app.expect("Usage N/A")
        app.expect("RBAC")
        app.send("\r")
        app.expect("Pod overview")
        app.expect("Live usage")
        app.expect("Access denied")
        app.send("\x03")
        app.assert_exit()
        self.assertFalse(self.mutations)

    def test_inspection_ranges_budgets_and_rankings_keep_sampling_and_navigation(self):
        self.metrics_enabled = True
        self.pod["spec"]["containers"][0]["resources"] = {
            "requests": {"cpu": "250m", "memory": "64Mi"},
            "limits": {"cpu": "500m", "memory": "128Mi"},
        }
        app = self.open_resource()
        app.resize(180, 48)
        app.expect_screen("32.0Mi")
        app.send("I")
        app.expect_screen("Top consumers")
        app.expect_screen("1/1 filtered objects")
        app.capture_screen("charts-top-consumers")
        app.send("\x1b")
        app.expect_screen("checkout-test")
        app.send("\r")
        app.expect_screen("Live usage")
        app.send("I")
        app.expect_screen("Range 15m")
        app.expect_screen("Sampled")
        reads = len(self.metrics_times)
        app.send("W\x1b[D")
        app.expect_screen("Range 5m")
        app.expect_screen("Inspecting")
        for width, height in ((80, 16), (120, 30), (180, 48), (240, 60)):
            app.resize(width, height)
            app.capture_screen(f"charts-inspection-{width}x{height}")
        app.send("ff")
        app.expect_screen("Inspecting")
        app.capture_screen("charts-fullscreen")
        app.send("\x1b[F")
        app.expect_screen("128Mi limits")
        app.capture_screen("charts-budgets")
        app.send("\x1b[H")
        app.send("P")
        app.expect_screen("Live usage")
        # Polling may legitimately fire during a slow terminal journey.
        samples = self.metrics_times[reads - 1:]
        for previous, current in zip(samples, samples[1:]):
            self.assertEqual(previous[0], current[0])
            self.assertGreaterEqual(current[1] - previous[1], 14.5, "display controls restarted sampling")
        app.send("\x1b")
        app.expect_screen("Pod overview")
        app.send("\x1b")
        app.expect_screen("AI assistant")
        app.send("\x03")
        app.assert_exit()
        self.assertFalse(self.mutations)
        self.assertEqual(self.config.read_bytes(), self.original)

    def test_node_charts_and_declared_reservations(self):
        self.metrics_enabled = True
        self.node = {
            "apiVersion": "v1", "kind": "Node",
            "metadata": {"name": "checkout-node", "uid": "node-id", "resourceVersion": "10"},
            "status": {"allocatable": {"cpu": "2", "memory": "4Gi"},
                       "capacity": {"cpu": "2", "memory": "4Gi"}},
        }
        self.pod["spec"]["nodeName"] = "checkout-node"
        self.pod["spec"]["containers"][0]["resources"] = {"requests": {"cpu": "250m", "memory": "64Mi"}}
        app = self.open_resource("nodes")
        app.resize(180, 48)
        app.send("\r")
        app.expect_screen("Live usage")
        app.send("Iff")
        app.expect_screen("Sampled")
        app.send("\x1b[F")
        app.expect_screen("500.0m usage / 2 allocatable")
        app.expect_screen("512.0Mi usage / 4Gi allocatable")
        app.capture_screen("node-chart-budgets")
        app.send("\x1b")
        app.send("/cpu reservations\r")
        app.expect_screen("250m requests / 2 allocatable")
        app.send("\x03")
        app.assert_exit()
        self.assertFalse(self.mutations)

    def test_quota_chart_reports_accounting_and_unknown_values(self):
        self.quota = {
            "apiVersion": "v1", "kind": "ResourceQuota",
            "metadata": {"name": "checkout-quota", "namespace": "shop", "uid": "quota-id", "resourceVersion": "10"},
            "spec": {"hard": {"requests.cpu": "8"}},
            "status": {"hard": {"requests.cpu": "4", "requests.memory": "1Gi", "pods": "0"},
                       "used": {"requests.cpu": "5", "pods": "0"}},
        }
        app = self.open_resource()
        app.send(":resourcequotas\r")
        app.expect_screen("checkout-quota")
        app.resize(180, 48)
        app.send("\rff")
        app.expect_screen("Quota utilization")
        app.expect_screen("5 used / 4 hard")
        app.expect_screen("125%")
        app.expect_screen("not reported used / 1Gi hard")
        app.capture_screen("quota-accounting-chart")
        app.send("\x03")
        app.assert_exit()
        self.assertFalse(self.mutations)
        self.assertEqual(self.metrics_reads, 0)

    def test_multicontainer_shares_and_budgets(self):
        self.metrics_enabled = True
        self.pod["spec"]["containers"][0]["resources"] = {"requests": {"cpu": "100m", "memory": "64Mi"}}
        self.pod["spec"]["containers"].append({"name": "proxy", "image": "example/proxy:v1", "resources": {"limits": {"cpu": "200m", "memory": "128Mi"}}})
        self.container_metrics = [
            {"name": "checkout", "usage": {"cpu": "25m", "memory": "32Mi"}},
            {"name": "proxy", "usage": {"cpu": "75m", "memory": "16Mi"}},
        ]
        app = self.open_resource()
        app.resize(180, 48)
        app.send("\r")
        app.expect_screen("Live usage")
        app.send("Iff")
        app.expect_screen("100.0m")
        app.send("\x1b[F")
        app.expect_screen("75.0m usage / 200m limits")
        app.expect_screen("16.0Mi usage / 128Mi limits")
        app.send("\x1b[A" * 16)
        app.expect_screen("Latest container breakdown")
        app.expect_screen("checkout · CPU")
        app.expect_screen("proxy · CPU")
        app.expect_screen("75%")
        app.capture_screen("container-share-charts")
        app.send("\x03")
        app.assert_exit()
        self.assertFalse(self.mutations)
