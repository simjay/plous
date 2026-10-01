"""Investigation and recovery flows against an isolated Kubernetes fixture."""

from test_explore import ExploreFixture


class InvestigationTests(ExploreFixture):
    def detail(self, app):
        app.send("\r")
        app.expect("[d Describe]")

    def finish(self, app):
        app.send("\x03")
        app.assert_exit()
        self.assertEqual(self.config.read_bytes(), self.original)

    def test_previous_logs_and_permission_recovery_preserve_events_and_filter(self):
        self.previous_logs_missing = True
        app = self.open_resource()
        app.send("/checkout\r\r")
        app.expect("Pod overview")
        app.send("e")
        app.expect("Back-off restarting failed container")
        app.send("l")
        app.expect("ERROR missing database setting")
        app.send("p")
        app.expect("retained previous instance")
        app.expect("p Return to current instance logs")
        start = len(app.output)
        app.send("p")
        app.expect("ERROR missing database setting", start)
        self.log_denied = True
        app.send("g")
        app.expect("Log access denied")
        app.expect("pods/log")
        self.log_denied = False
        start = len(app.output)
        app.send("g")
        app.expect("ERROR missing database setting", start)
        start = len(app.output)
        app.send("\x1b")
        app.expect("[e Events]", start)
        app.send("y")
        app.expect("[:y YAML]")
        start = len(app.output)
        app.send("\x1b")
        app.expect("[e Events]", start)
        start = len(app.output)
        app.send("\x1b")
        app.expect("Backspace clear", start)
        self.assertFalse(self.mutations)
        self.finish(app)

    def test_statefulset_and_ingress_details_and_related_service(self):
        app = self.open_resource("statefulsets")
        self.detail(app)
        app.expect("Ready replicas")
        app.expect("S Scale")
        app.expect("R Restart")
        app.send("y")
        app.expect("[:y YAML]")
        start = len(app.output)
        app.send("\x1b")
        app.expect("[d Describe]", start)
        start = len(app.output)
        app.send("\x1b")
        app.expect("Filter resources", start)
        app.send(":ingresses\r")
        app.expect("checkout-ingress")
        start = len(app.output)
        self.detail(app)
        app.expect("checkout.example.com", start)
        app.expect("checkout-service:80", start)
        text = app.text(start)
        for absent in ("l Logs", "x Exec", "S Scale", "R Restart"):
            self.assertNotIn(absent, text)
        app.send("r")
        app.expect("Referenced resources")
        app.expect("checkout-service")
        app.send("\x1b[B\r")
        app.expect("Cluster IP")
        start = len(app.output)
        target = "/ingresses/checkout-ingress"
        requests = sum(path.endswith(target) for _, path, _ in self.requests)
        app.send("\x1b")
        app.expect("[r Related]", start)
        app.wait(lambda: sum(path.endswith(target) for _, path, _ in self.requests) > requests,
                 "return to the original Ingress")
        app.wait_json(self.state / "session.json",
                      lambda data: data["explore"][data["workspace"] + "/" + data["connection"]]["resource"] == "ingresses.networking.k8s.io")
        self.assertFalse(self.mutations)
        self.finish(app)
