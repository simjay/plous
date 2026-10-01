"""Opt-in acceptance on Minikube; writes stay in a newly owned namespace."""

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
import uuid

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "e2e"))
from support import Terminal


@unittest.skipUnless(os.environ.get("PLOUS_MINIKUBE_TEST_KUBECONFIG"), "explicit Minikube configuration required")
class MinikubeAcceptance(unittest.TestCase):
    def cli(self, *args, value=None):
        result = subprocess.run(
            ["kubectl", "--kubeconfig", str(self.source), "--context=minikube", "--request-timeout=20s", *args],
            input=json.dumps(value) if value is not None else None,
            capture_output=True, text=True, timeout=40,
        )
        if result.returncode:
            # Commands can return credentials. Never include stdout in errors.
            raise AssertionError("Minikube acceptance command failed: " + " ".join(args[:3]))
        return json.loads(result.stdout) if result.stdout.strip().startswith("{") else result.stdout

    def setUp(self):
        self.source = Path(os.environ["PLOUS_MINIKUBE_TEST_KUBECONFIG"])
        self.original = self.source.read_bytes()
        config = json.loads(self.original)
        self.assertEqual(config["current-context"], "minikube")
        selected = next(context for context in config["contexts"] if context["name"] == "minikube")
        config["contexts"] = [selected]
        config["clusters"] = [cluster for cluster in config["clusters"] if cluster["name"] == selected["context"]["cluster"]]
        config["users"] = [user for user in config["users"] if user["name"] == selected["context"]["user"]]
        temporary = tempfile.TemporaryDirectory(prefix="plous-terminal-minikube-")
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.namespace = "plous-ui-acceptance-" + uuid.uuid4().hex[:10]
        created = self.cli("create", "-f", "-", "-o", "json", value={
            "apiVersion": "v1", "kind": "Namespace", "metadata": {
                "name": self.namespace, "labels": {"app.kubernetes.io/managed-by": "plous-acceptance"}}})
        self.namespace_uid = created["metadata"]["uid"]
        self.addCleanup(self.remove_namespace)
        for context in config["contexts"]:
            context["context"]["namespace"] = self.namespace
        self.config = self.root / "kubeconfig"
        self.config.write_text(json.dumps(config))
        self.config.chmod(0o600)
        self.deployment = self.cli("create", "-f", "-", "-o", "json", value={
            "apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "ui-echo", "namespace": self.namespace},
            "spec": {"replicas": 1, "selector": {"matchLabels": {"app": "ui-echo"}}, "template": {
                "metadata": {"labels": {"app": "ui-echo"}}, "spec": {"terminationGracePeriodSeconds": 1, "containers": [{
                    "name": "echo", "image": "docker.io/library/busybox:1.37.0", "imagePullPolicy": "IfNotPresent",
                    "command": ["sh", "-c", "i=0; while true; do echo plous-ui-log-$i; i=$((i+1)); sleep 1; done"],
                    "resources": {"requests": {"cpu": "10m", "memory": "16Mi"}, "limits": {"cpu": "100m", "memory": "32Mi"}},
                }]}}}})
        self.cli("-n", self.namespace, "rollout", "status", "deployment/ui-echo", "--timeout=30s")

    def remove_namespace(self):
        current = self.cli("get", "namespace", self.namespace, "-o", "json")
        self.assertEqual(current["metadata"]["uid"], self.namespace_uid)
        self.assertEqual(current["metadata"]["labels"]["app.kubernetes.io/managed-by"], "plous-acceptance")
        # No shared playground resources are removed.
        self.cli("delete", "namespace", self.namespace, "--wait=true", "--timeout=30s")
        self.assertEqual(self.source.read_bytes(), self.original)

    def start(self, resource="deployments"):
        app = Terminal(self.root / "state", self.config, width=180, height=48)
        self.addCleanup(app.close)
        app.expect("AI assistant")
        app.send(":new\rAcceptance\r\r")
        app.expect("Add tool")
        app.send("\r")
        app.expect("Add Kubernetes contexts")
        app.send(" \x1b[B\r")
        app.expect("Added 1 Kubernetes")
        app.send(":" + resource + "\r")
        app.expect("Choose Kubernetes connection")
        app.send("\r")
        app.expect("ui-echo")
        return app

    def expect_later(self, app, text, start=0):
        app.wait(lambda: text in app.text(start), text, timeout=60)

    def test_scale_restart_logs_yaml_and_delete_in_real_terminal(self):
        app = self.start()
        app.send("\r")
        app.expect("Desired replicas:")
        app.read(0.5)
        app.send("S")
        app.expect("Current desired: 1")
        app.send("\x152\r")
        app.expect("Confirm scale")
        # Cancel leaves the server unchanged and returns one layer.
        app.send("\x1b")
        app.expect("Proposed:")
        self.assertEqual(self.cli("-n", self.namespace, "get", "deployment/ui-echo", "-o", "json")["spec"]["replicas"], 1)
        app.send("\r\x1b[C\r")
        app.expect("Scale request accepted")
        self.expect_later(app, "Replica target reached")
        app.resize(100, 30)
        app.expect("Enter/Esc details")
        app.resize(180, 48)
        app.send("\x1b")
        start = len(app.output)
        app.send("R")
        app.expect("Confirm restart", start)
        app.send("\x1b[C\r")
        app.expect("Pod template updated", start)
        self.expect_later(app, "Rollout complete", start)
        app.send("\r")
        start = len(app.output)
        app.send("y")
        app.expect("apiVersion", start)
        app.send("\x1b")
        app.expect("[d Describe]", start)
        app.send("l")
        app.expect("Choose Pod")
        app.send("\r")
        self.expect_later(app, "plous-ui-log-")
        app.send("\x1b[5~")
        app.expect("Reading")
        app.send(" ")
        start = len(app.output)
        app.send("\x1b")
        app.expect("[d Describe]", start)
        app.read(0.5)
        start = len(app.output)
        app.send("x")
        app.expect("Choose Pod", start)
        app.send("\r")
        app.expect("Custom command")
        app.send("\r\r")
        app.expect("Terminal input")
        app.send("for i in $(seq 1 100); do echo review-line-$i; done\r")
        self.expect_later(app, "review-line-100")
        app.send("\x1d")
        app.expect("Review output")
        app.send("\x1b[5~")
        app.send("/review-line-25")
        app.expect("review-line-25")
        app.send("\x1b")
        start = len(app.output)
        app.send("\x1b")
        app.expect("[d Describe]", start)
        app.send("D")
        app.expect("Confirm delete")
        app.send("\x1b[C\r")
        app.expect("Deletion requested")
        self.expect_later(app, "Object deleted")
        app.send("\x03")
        app.assert_exit()

    def test_editor_dry_run_apply_and_readonly_resources(self):
        self.cli("create", "-f", "-", "-o", "json", value={"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": "ui-settings", "namespace": self.namespace}, "data": {"setting": "before"}})
        app = self.start()
        app.send(":configmaps\r")
        app.expect("ui-settings")
        app.send("/ui-settings\r")
        app.expect("Backspace clear")
        app.send("\r")
        app.expect("[d Describe]")
        app.send("E")
        app.expect("Plous Editor")
        obj = self.cli("-n", self.namespace, "get", "configmap/ui-settings", "-o", "json")
        obj["data"]["setting"] = "after"
        app.send("ggVG\x1b[200~" + json.dumps(obj, indent=2) + "\x1b[201~:v\r")
        app.expect("Review only. Nothing saved.")
        self.assertEqual(self.cli("-n", self.namespace, "get", "configmap/ui-settings", "-o", "json")["data"]["setting"], "before")
        app.send(":w\r")
        app.expect("Saved to Kubernetes")
        self.assertEqual(self.cli("-n", self.namespace, "get", "configmap/ui-settings", "-o", "json")["data"]["setting"], "after")
        app.send(":q\r")
        app.send("\x1b")
        for resource, text in [("statefulsets", "store"), ("daemonsets", "node-reporter"), ("jobs", "completed-example"), ("cronjobs", "periodic-report"), ("services", "backend-podinfo"), ("ingresses", "frontend"), ("persistentvolumeclaims", "data-store-0"), ("certificates.cert-manager.io", "playground")]:
            start = len(app.output)
            app.send(":" + resource + " plous-demo\r")
            app.expect(text, start)
            app.send("\r")
            app.expect("[d Describe]", start)
            app.send("\x1b")
        app.send("\x03")
        app.assert_exit()

    def test_limited_connection_reads_pods_and_explains_denied_operations(self):
        self.cli("create", "-f", "-", "-o", "json", value={"apiVersion": "v1", "kind": "ServiceAccount", "metadata": {"name": "observer", "namespace": self.namespace}})
        self.cli("create", "-f", "-", "-o", "json", value={
            "apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role", "metadata": {"name": "observer", "namespace": self.namespace},
            "rules": [{"apiGroups": [""], "resources": ["pods"], "verbs": ["get", "list", "watch"]}]})
        self.cli("create", "-f", "-", "-o", "json", value={
            "apiVersion": "rbac.authorization.k8s.io/v1", "kind": "RoleBinding", "metadata": {"name": "observer", "namespace": self.namespace},
            "subjects": [{"kind": "ServiceAccount", "name": "observer", "namespace": self.namespace}],
            "roleRef": {"apiGroup": "rbac.authorization.k8s.io", "kind": "Role", "name": "observer"}})
        token = self.cli("-n", self.namespace, "create", "token", "observer", "--duration=10m").strip()
        config = json.loads(self.config.read_text())
        config["users"] = [{"name": "observer", "user": {"token": token}}]
        for context in config["contexts"]:
            context["context"]["user"] = "observer"
        self.config.write_text(json.dumps(config))
        app = self.start("pods")
        app.send("\r")
        app.expect("[d Describe]")
        app.send("l")
        app.expect("Log access denied by Kubernetes RBAC")
        app.send("\x1b")
        app.read(0.5)
        start = len(app.output)
        app.send("D")
        app.expect("Permission denied", start)
        app.send("\x1b")
        app.send("\x1b")
        start = len(app.output)
        app.send(":pods plous-demo\r")
        app.expect("Access denied by Kubernetes RBAC", start)
        app.send("\x03")
        app.assert_exit()


if __name__ == "__main__":
    unittest.main()
