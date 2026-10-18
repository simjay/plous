"""Exercise Argo and Flux through an isolated Kubernetes API and terminal."""

from copy import deepcopy
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import tempfile
import threading
import unittest
from urllib.parse import parse_qs, urlsplit

from support import Terminal


class GitOpsTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory(prefix="plous-gitops-")
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        self.state = self.root / "state"
        self.requests = []
        self.mutations = []
        self.denied = set()
        self.failed = set()
        self.blocked = {}
        self.resources = {
            "v1": [("pods", "Pod"), ("services", "Service"), ("configmaps", "ConfigMap"),
                   ("namespaces", "Namespace"), ("events", "Event")],
            "apps/v1": [("deployments", "Deployment"), ("replicasets", "ReplicaSet")],
            "argoproj.io/v1alpha1": [("applications", "Application"), ("applicationsets", "ApplicationSet"), ("appprojects", "AppProject")],
            "kustomize.toolkit.fluxcd.io/v1": [("kustomizations", "Kustomization")],
            "helm.toolkit.fluxcd.io/v2": [("helmreleases", "HelmRelease")],
            "source.toolkit.fluxcd.io/v1": [("gitrepositories", "GitRepository")],
        }
        self.objects = {}
        test = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass

            def respond(self, value, status=200):
                data = json.dumps(value).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def failure(self, reason, code):
                self.respond({"apiVersion": "v1", "kind": "Status", "status": "Failure", "reason": reason,
                              "message": "fixture " + reason, "code": code}, code)

            def do_GET(self):
                parsed = urlsplit(self.path)
                path, query = parsed.path, parse_qs(parsed.query)
                test.requests.append(("GET", path, query))
                if gate := test.blocked.get(path):
                    gate.wait(timeout=10)
                if path in test.denied:
                    self.failure("Forbidden", 403)
                    return
                if path in test.failed:
                    self.failure("ServiceUnavailable", 503)
                    return
                if path == "/api":
                    self.respond({"apiVersion": "v1", "kind": "APIVersions", "versions": ["v1"]})
                    return
                if path == "/version":
                    self.respond({"major": "1", "minor": "34", "gitVersion": "v1.34.0"})
                    return
                if path == "/apis":
                    groups = []
                    for gv in test.resources:
                        if gv == "v1":
                            continue
                        group, version = gv.rsplit("/", 1)
                        value = {"groupVersion": gv, "version": version}
                        groups.append({"name": group, "versions": [value], "preferredVersion": value})
                    self.respond({"apiVersion": "v1", "kind": "APIGroupList", "groups": groups})
                    return
                for gv, resources in test.resources.items():
                    base = "/api/v1" if gv == "v1" else "/apis/" + gv
                    if path == base:
                        self.respond({"apiVersion": "v1", "kind": "APIResourceList", "groupVersion": gv,
                                      "resources": [{"name": name, "kind": kind,
                                                     "namespaced": name != "namespaces", "verbs": ["get", "list", "watch"]}
                                                    for name, kind in resources]})
                        return
                    if not path.startswith(base + "/"):
                        continue
                    rest = path[len(base) + 1:].split("/")
                    namespace = ""
                    if len(rest) >= 3 and rest[0] == "namespaces":
                        namespace, rest = rest[1], rest[2:]
                    resource = rest[0]
                    kind = next((kind for name, kind in resources if name == resource), None)
                    if kind is None:
                        continue
                    if query.get("watch") == ["true"]:
                        self.respond({"type": "BOOKMARK", "object": {"apiVersion": gv, "kind": kind,
                                      "metadata": {"resourceVersion": "1"}}})
                        return
                    objects = test.objects.get((gv, resource), [])
                    if namespace:
                        objects = [o for o in objects if o["metadata"].get("namespace") == namespace]
                    if len(rest) == 2:
                        obj = next((o for o in objects if o["metadata"]["name"] == rest[1]), None)
                        if obj is None:
                            self.failure("NotFound", 404)
                        else:
                            self.respond(obj)
                        return
                    if len(rest) == 1:
                        self.respond({"apiVersion": gv, "kind": kind + "List", "metadata": {"resourceVersion": "1"}, "items": objects})
                        return
                self.failure("NotFound", 404)

            def mutation(self):
                size = int(self.headers.get("Content-Length", "0"))
                test.mutations.append((self.command, self.path, self.rfile.read(size)))
                self.failure("Forbidden", 403)

            do_POST = mutation
            do_PUT = mutation
            do_PATCH = mutation
            do_DELETE = mutation

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.server.daemon_threads = True
        threading.Thread(target=self.server.serve_forever, daemon=True).start()
        self.addCleanup(self.server.server_close)
        self.addCleanup(self.server.shutdown)
        self.url = f"http://127.0.0.1:{self.server.server_port}"
        self.config = self.root / "kubeconfig"
        self.original = (
            "apiVersion: v1\nkind: Config\ncontexts:\n- name: local-test\n"
            "  context: {cluster: fixture, user: tester}\nclusters:\n- name: fixture\n"
            f"  cluster: {{server: '{self.url}'}}\nusers:\n- name: tester\n  user: {{}}\n"
        ).encode()
        self.config.write_bytes(self.original)
        self.seed_objects()

    def seed_objects(self):
        def obj(gv, kind, name, namespace, spec=None, status=None):
            return {"apiVersion": gv, "kind": kind,
                    "metadata": {"name": name, "namespace": namespace, "generation": 2,
                                 "uid": namespace + "/" + name, "resourceVersion": "1"},
                    "spec": spec or {}, "status": status or {}}

        self.application = obj("argoproj.io/v1alpha1", "Application", "payments", "argocd", {
            "source": {"repoURL": self.url + "/forbidden-source", "path": "clusters/shop", "targetRevision": "main"},
            "destination": {"server": "https://kubernetes.default.svc", "namespace": "shop"},
            "project": "default",
        }, {
            "sync": {"status": "Synced", "revision": "revision-compared"},
            "health": {"status": "Degraded", "message": "Deployment payments exceeded its progress deadline"},
            "reconciledAt": "2026-10-01T12:00:00Z",
            "resources": [{"group": "apps", "version": "v1", "kind": "Deployment", "name": "payments", "namespace": "shop",
                           "status": "Synced", "health": {"status": "Degraded"}},
                          {"group": "", "version": "v1", "kind": "ConfigMap", "name": "payments-settings", "namespace": "shop", "status": "Synced"}],
        })
        worker = deepcopy(self.application)
        worker["metadata"].update(name="worker", uid="argocd/worker")
        worker["status"].update(sync={"status": "OutOfSync", "revision": "revision-new"}, health={"status": "Healthy"}, resources=[])
        remote = deepcopy(self.application)
        remote["metadata"].update(name="remote-reports", uid="argocd/remote-reports")
        remote["spec"]["destination"] = {"name": "unconfigured-remote", "namespace": "reports"}
        remote["status"]["health"]["message"] = "Deployment reports exceeded its progress deadline"
        remote["status"]["resources"] = [{"group": "apps", "version": "v1", "kind": "Deployment", "name": "reports", "namespace": "reports", "status": "Synced", "health": {"status": "Degraded"}}]
        self.objects[("argoproj.io/v1alpha1", "applications")] = [self.application, worker, remote]

        self.objects[("argoproj.io/v1alpha1", "applicationsets")] = []
        self.objects[("argoproj.io/v1alpha1", "appprojects")] = [obj("argoproj.io/v1alpha1", "AppProject", "default", "argocd", {
            "description": "Commerce applications", "sourceRepos": [self.url + "/forbidden-source"],
            "destinations": [{"server": "https://kubernetes.default.svc", "namespace": "shop"}],
        })]

        def condition(status, reason, message, generation=2):
            return {"type": "Ready", "status": status, "reason": reason, "message": message,
                    "observedGeneration": generation, "lastTransitionTime": "2026-10-01T11:00:00Z"}

        base = {"sourceRef": {"kind": "GitRepository", "name": "platform-config"}, "path": "./clusters/shop", "interval": "1m"}
        self.platform = obj("kustomize.toolkit.fluxcd.io/v1", "Kustomization", "platform", "flux-system", base, {
            "conditions": [condition("False", "HealthCheckFailed", "health check failed: Deployment/shop/payments is not ready")],
            "lastAppliedRevision": "main@sha1:previous", "lastAttemptedRevision": "main@sha1:latest", "observedGeneration": 1,
            "inventory": {"entries": [{"id": "shop_payments_apps_Deployment", "v": "v1"}]},
        })
        storefront = obj("kustomize.toolkit.fluxcd.io/v1", "Kustomization", "storefront", "flux-system", {
            **base, "dependsOn": [{"name": "platform"}],
        }, {"conditions": [condition("False", "DependencyNotReady", "dependency platform is not ready")]})
        suspended = obj("kustomize.toolkit.fluxcd.io/v1", "Kustomization", "preview", "flux-system", {
            **base, "suspend": True,
        }, {"conditions": [condition("True", "ReconciliationSucceeded", "Applied revision: main@sha1:previous")]})
        awaiting = obj("kustomize.toolkit.fluxcd.io/v1", "Kustomization", "pending", "flux-system", base, {
            "conditions": [condition("True", "ReconciliationSucceeded", "Applied previous generation", generation=1)],
        })
        self.objects[("kustomize.toolkit.fluxcd.io/v1", "kustomizations")] = [self.platform, storefront, suspended, awaiting]
        self.objects[("helm.toolkit.fluxcd.io/v2", "helmreleases")] = [obj("helm.toolkit.fluxcd.io/v2", "HelmRelease", "metrics", "flux-system", {
            "interval": "5m", "targetNamespace": "monitoring", "chart": {"spec": {"chart": "metrics", "version": "1.4.2",
                "sourceRef": {"kind": "HelmRepository", "name": "charts"}}},
        }, {"conditions": [condition("True", "ReconciliationSucceeded", "Helm install succeeded for release monitoring/metrics with chart metrics@1.4.2")],
            "lastAttemptedRevision": "1.4.2"})]
        self.objects[("source.toolkit.fluxcd.io/v1", "gitrepositories")] = [obj("source.toolkit.fluxcd.io/v1", "GitRepository", "platform-config", "flux-system", {
            "url": self.url + "/forbidden-source", "ref": {"branch": "main"}, "interval": "1m",
        }, {"conditions": [condition("True", "Succeeded", "stored artifact for revision main@sha1:latest")],
            "artifact": {"revision": "main@sha1:latest", "url": self.url + "/forbidden-artifact.tar.gz"}})]
        self.objects[("apps/v1", "deployments")] = [obj("apps/v1", "Deployment", "payments", "shop", {
            "replicas": 2, "selector": {"matchLabels": {"app": "payments"}},
            "template": {"metadata": {"labels": {"app": "payments"}}, "spec": {"containers": [{"name": "payments", "image": "example/payments:v2"}]}},
        }, {"replicas": 2, "readyReplicas": 0, "conditions": [{"type": "Progressing", "status": "False", "reason": "ProgressDeadlineExceeded"}]})]
        self.objects[("v1", "configmaps")] = [obj("v1", "ConfigMap", "payments-settings", "shop")]
        self.objects[("v1", "namespaces")] = [obj("v1", "Namespace", name, "") for name in ["argocd", "flux-system", "shop", "monitoring"]]

    def start(self, width=160, height=40, env_overrides=None):
        empty_path = self.root / "bin"
        empty_path.mkdir(exist_ok=True)
        app = Terminal(self.state, self.config, width=width, height=height, env_overrides={"PATH": str(empty_path), **(env_overrides or {})})
        self.addCleanup(app.close)
        app.expect("AI assistant")
        return app

    def assert_read_only(self):
        self.assertEqual(self.mutations, [])
        self.assertFalse(any("forbidden-source" in path or "forbidden-artifact" in path for _, path, _ in self.requests))
        self.assertEqual(self.config.read_bytes(), self.original)

    def open_tool(self, tool="Argo", width=160, height=40):
        app = self.start(width=width, height=height)
        app.send(":new\rCommerce\r\r\r")
        app.expect_screen("Add Context")
        app.send(" \x1b[B\r")
        app.expect_screen("Added 1 Kubernetes")
        self.assertEqual(self.requests, [])
        app.send(":2\r")
        app.send("\x1b")
        app.send("/local-test\r")
        app.expect_screen("Argo")
        app.expect_screen("Flux")
        app.expect_screen("Kubernetes")
        self.assertFalse(any("Helm" in row for row in app.screen()["rows"]))
        app.send("\x1b[B" * (3 if tool == "Flux" else 2) + "\r")
        return app

    def seed_overview(self):
        self.resources["v1"].append(("nodes", "Node"))
        self.resources["apps/v1"].extend([("statefulsets", "StatefulSet"), ("daemonsets", "DaemonSet")])
        self.resources["autoscaling/v2"] = [("horizontalpodautoscalers", "HorizontalPodAutoscaler")]
        self.objects[("v1", "nodes")] = [{
            "apiVersion": "v1", "kind": "Node", "metadata": {"name": "node-one", "uid": "node-one"},
            "status": {"allocatable": {"cpu": "8", "memory": "16Gi"},
                       "conditions": [{"type": "Ready", "status": "True"}]},
        }]
        self.objects[("v1", "pods")] = [{
            "apiVersion": "v1", "kind": "Pod",
            "metadata": {"name": "broken-checkout", "namespace": "shop", "uid": "broken-checkout", "resourceVersion": "1"},
            "spec": {"containers": [{"name": "web", "image": "fixture:v1", "resources": {
                "requests": {"cpu": "500m", "memory": "1Gi"}, "limits": {"cpu": "2", "memory": "2Gi"}}}]},
            "status": {"phase": "Running", "containerStatuses": [{"name": "web", "ready": False,
                       "restartCount": 4, "state": {"waiting": {"reason": "CrashLoopBackOff"}}}]},
        }]

    def test_overview_cluster_dashboard_inspection_and_resize(self):
        self.seed_overview()
        app = self.open_tool(width=220, height=46)
        app.send(":1\r")
        app.expect_screen("Observed")
        app.expect_screen("Cluster Pulse")
        app.expect_screen("Recent Deployments")
        app.expect_screen("Needs Attention")
        app.expect_screen("Infrastructure Snapshot")
        app.expect_screen("Unavailable")
        app.expect_screen("0.50c")
        app.capture_screen("overview-large")
        app.send("\x1b[B\x1b[C/broken-checkout\r")
        app.expect_screen("Pod overview")
        app.send("\x1b")
        app.expect_screen("Overview")
        app.expect_screen("broken-checkout")
        app.send("\x1b")
        app.resize(80, 24)
        app.expect_screen("Needs Attention")
        app.capture_screen("overview-80x24")
        app.send("hh")
        app.expect_screen("Overview · Selected")
        app.send("\x1b")
        app.send("e")
        app.expect_screen("Needs Attention")
        app.send("\x1b")
        app.send("\x1b[F\x1b[B/Argo\r")
        app.expect_screen("Applications")
        app.send("\x1b")
        app.expect_screen("Overview")
        app.send("\x1b")
        app.resize(60, 18)
        app.expect_screen("Overview")
        app.capture_screen("overview-60x18")
        app.send("\x03")
        app.assert_exit()
        self.assert_read_only()

    def test_overview_distinguishes_node_usage_from_pod_requests(self):
        from datetime import datetime, timezone
        self.seed_overview()
        self.resources["metrics.k8s.io/v1beta1"] = [("nodes", "NodeMetrics")]
        sample = {
            "apiVersion": "metrics.k8s.io/v1beta1", "kind": "NodeMetrics",
            "metadata": {"name": "node-one", "uid": "node-one"},
            "timestamp": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
            "window": "15s", "usage": {"cpu": "2", "memory": "4Gi"},
        }
        self.objects[("metrics.k8s.io/v1beta1", "nodes")] = [sample]
        app = self.open_tool(width=180, height=42)
        app.send(":1\r")
        app.expect_screen("25% used")
        app.expect_screen("2.00c / 8.00c")
        app.expect_screen("0.50c")
        app.capture_screen("overview-measured")
        sample["timestamp"] = "2000-01-01T00:00:00Z"
        app.send("g")
        app.expect_screen("Usage unavailable")
        app.expect_screen("0.50c")
        app.capture_screen("overview-expired-metrics")
        app.send("\x03")
        app.assert_exit()
        self.assert_read_only()

    def test_overview_usage_updates_before_slow_inventory_finishes(self):
        from datetime import datetime, timezone
        self.seed_overview()
        self.resources["metrics.k8s.io/v1beta1"] = [("nodes", "NodeMetrics")]
        sample = {"apiVersion": "metrics.k8s.io/v1beta1", "kind": "NodeMetrics",
                  "metadata": {"name": "node-one"},
                  "timestamp": datetime.now(timezone.utc).isoformat(), "window": "15s",
                  "usage": {"cpu": "2", "memory": "4Gi"}}
        self.objects[("metrics.k8s.io/v1beta1", "nodes")] = [sample]
        app = self.open_tool(width=180, height=42)
        gate = threading.Event()
        self.blocked["/api/v1/pods"] = gate
        self.addCleanup(gate.set)
        app.send(":1\r")
        app.wait(lambda: "25% used" in "\n".join(app.screen()["rows"]), "usage before inventory", timeout=4)
        app.expect_screen("Loading")
        app.expect_screen("1 sample")
        app.capture_screen("overview-usage-before-inventory")
        sample["usage"]["cpu"] = "3"
        sample["timestamp"] = datetime.now(timezone.utc).isoformat()
        app.expect_screen("38% used")
        app.expect_screen("Loading")
        self.assertFalse(gate.is_set())
        gate.set()
        app.expect_screen("0.50c")
        app.expect_screen("38% used")
        app.capture_screen("overview-independent-usage")
        app.send("\x03")
        app.assert_exit()
        self.assert_read_only()

    def test_overview_partial_controller_counts_are_unknown(self):
        self.seed_overview()
        self.denied.add("/apis/helm.toolkit.fluxcd.io/v2/helmreleases")
        app = self.open_tool(width=180, height=42)
        app.send(":1\r")
        app.expect_screen("Observed")
        app.send("\x1b[B\x1b[C\x1b[F\x1b[Be/Flux")
        app.expect_screen("? Helm releases")
        app.expect_screen("Partial")
        app.capture_screen("overview-partial")
        app.send("\x03")
        app.assert_exit()
        self.assert_read_only()

    def test_overview_realistic_cluster_and_group_inspection(self):
        from datetime import datetime, timedelta, timezone
        self.seed_overview()
        now = datetime.now(timezone.utc)
        stamp = now.isoformat().replace("+00:00", "Z")
        for key in list(self.resources):
            if "argoproj" in key or "toolkit.fluxcd" in key:
                del self.resources[key]
        nodes = []
        metrics = []
        for i in range(13):
            node = deepcopy(self.objects[("v1", "nodes")][0])
            node["metadata"] = {"name": f"node-{i}", "uid": f"node-{i}"}
            node["status"]["allocatable"] = {"cpu": "9137692308n", "memory": "43.8076923077Gi"}
            nodes.append(node)
            metrics.append({"apiVersion": "metrics.k8s.io/v1beta1", "kind": "NodeMetrics",
                            "metadata": node["metadata"], "timestamp": stamp, "window": "15s",
                            "usage": {"cpu": "334m", "memory": "13.7153846154Gi"}})
        self.objects[("v1", "nodes")] = nodes
        self.resources["metrics.k8s.io/v1beta1"] = [("nodes", "NodeMetrics")]
        self.objects[("metrics.k8s.io/v1beta1", "nodes")] = metrics
        pods = []
        for i in range(347):
            phase = "Running" if i < 323 else "Failed" if i < 326 else "Succeeded"
            name = f"scheduled-weekly-jobs-298612-abc{i}" if phase == "Failed" else f"service-pod-{i}"
            pod = {"apiVersion": "v1", "kind": "Pod", "metadata": {"name": name, "namespace": "services", "uid": name},
                   "spec": {"containers": [{"name": "worker", "image": "fixture:v1"}]},
                   "status": {"phase": phase, "conditions": [{"type": "Ready", "status": "True"}],
                              "containerStatuses": [{"name": "worker", "ready": True, "state": {"running": {}}}]}}
            if phase == "Failed":
                pod["metadata"]["ownerReferences"] = [{"apiVersion": "batch/v1", "kind": "Job", "name": "scheduled-weekly-jobs-298612", "uid": "weekly-job", "controller": True}]
                pod["status"]["containerStatuses"] = [{"name": "worker", "ready": False, "state": {"terminated": {"reason": "Error", "exitCode": 1, "finishedAt": stamp}}}]
            pods.append(pod)
        self.objects[("v1", "pods")] = pods
        deployments = []
        for i in range(75):
            deployments.append({"apiVersion": "apps/v1", "kind": "Deployment",
                                "metadata": {"name": f"payment-processing-service-{i:03}", "namespace": "services", "uid": f"deploy-{i}", "generation": 1,
                                             "annotations": {"deployment.kubernetes.io/revision": str(1992-i)}},
                                "spec": {"replicas": 1}, "status": {"observedGeneration": 1, "replicas": 1, "readyReplicas": 1, "availableReplicas": 1,
                                "conditions": [{"type": "Progressing", "status": "True", "reason": "NewReplicaSetAvailable",
                                                "lastUpdateTime": (now-timedelta(minutes=i+3)).isoformat().replace("+00:00", "Z")}]}})
        self.objects[("apps/v1", "deployments")] = deployments
        self.original = self.original.replace(b"kind: Config\n", b"kind: Config\ncurrent-context: local-test\n")
        self.config.write_bytes(self.original)
        app = self.start(width=220, height=50)
        app.expect_screen("Cluster Pulse")
        for text in ("347 total", "21 done", "3 failed", "118.79c", "569.5Gi", "Available", "r1992", "3 affected", "1 integration"):
            app.expect_screen(text)
        app.capture_screen("overview-realistic-220x50")
        app.send("\x1b[B\x1b[C\r")
        app.expect_screen("Affected resources")
        app.expect_screen("abc323")
        app.send("\r")
        app.expect_screen("Pod overview")
        app.expect_screen("abc323")
        app.send("\x1b")
        app.expect_screen("Affected resources")
        app.send("\x1b")
        app.expect_screen("3 affected")
        for width, height in ((150, 40), (110, 30), (80, 24), (60, 18)):
            app.resize(width, height)
            app.expect_screen("Attention")
            app.capture_screen(f"overview-realistic-{width}x{height}")
        app.send("cc")
        app.expect_screen("AI assistant")
        app.capture_screen("overview-compact-chat")
        app.send("\x03")
        app.assert_exit()
        self.assert_read_only()

    def test_overview_restart_changes_and_reduced_color(self):
        self.seed_overview()
        self.original = self.original.replace(b"kind: Config\n", b"kind: Config\ncurrent-context: local-test\n")
        self.config.write_bytes(self.original)
        app = self.start(width=150, height=40, env_overrides={"TERM": "xterm", "COLORTERM": "", "NO_COLOR": "1"})
        app.expect_screen("Cluster Pulse")
        app.send("\x1b[C" * 3)
        app.expect_screen("4 restarts (total)")
        self.objects[("v1", "pods")][0]["status"]["containerStatuses"][0]["restartCount"] = 7
        app.send("g")
        app.expect_screen("+3 restarts /")
        app.capture_screen("overview-restart-change-150x40")
        self.objects[("v1", "pods")][0]["metadata"]["uid"] = "replacement-pod"
        self.objects[("v1", "pods")][0]["status"]["containerStatuses"][0]["restartCount"] = 15
        app.send("g")
        app.expect_screen("15 restarts (total)")
        self.assertNotIn("+8 restarts", "\n".join(app.screen()["rows"]))
        app.resize(80, 24)
        app.send("\x1b[B\x1b[F\x1b[B/broken-checkout")
        app.expect_screen("× CrashLoopBackOff")
        app.capture_screen("overview-reduced-color-80x24")
        app.send("\x03")
        app.assert_exit()
        self.assert_read_only()

    def test_overview_full_application_visual_matrix(self):
        from datetime import datetime, timedelta, timezone
        self.seed_overview()
        now = datetime.now(timezone.utc)
        stamp = now.isoformat().replace("+00:00", "Z")
        self.resources["v1"].append(("secrets", "Secret"))
        self.objects[("v1", "secrets")] = [
            {"apiVersion": "v1", "kind": "Secret", "type": "helm.sh/release.v1",
             "metadata": {"name": "sh.helm.release.v1.metrics.v2", "namespace": "monitoring",
                          "labels": {"owner": "helm", "name": "metrics", "version": "2",
                                     "status": "deployed", "modifiedAt": str(int(now.timestamp()))}}}
        ]
        self.resources["metrics.k8s.io/v1beta1"] = [("nodes", "NodeMetrics")]
        self.objects[("metrics.k8s.io/v1beta1", "nodes")] = [
            {"apiVersion": "metrics.k8s.io/v1beta1", "kind": "NodeMetrics",
             "metadata": {"name": "node-one", "uid": "node-one"}, "timestamp": stamp, "window": "15s",
             "usage": {"cpu": "336m", "memory": "5Gi"}}
        ]
        self.objects[("apps/v1", "deployments")][0]["status"]["conditions"][0]["lastUpdateTime"] = stamp
        self.original = self.original.replace(b"kind: Config\n", b"kind: Config\ncurrent-context: local-test\n")
        self.config.write_bytes(self.original)
        app = self.start(width=220, height=50)
        app.expect_screen("Cluster Pulse")
        app.expect_screen("4 integrations")
        app.expect_screen("1 sample")
        app.capture_screen("overview-all-tools-220x50")
        self.objects[("metrics.k8s.io/v1beta1", "nodes")][0]["timestamp"] = (now+timedelta(seconds=1)).isoformat().replace("+00:00", "Z")
        self.objects[("metrics.k8s.io/v1beta1", "nodes")][0]["usage"]["cpu"] = "640m"
        app.send("g")
        app.expect_screen("8% used")
        app.expect_screen("sampled")
        app.capture_screen("overview-all-tools-history-220x50")
        for width, height in ((80,24),(109,40),(110,40),(149,40),(150,40),(180,45),(220,50)):
            app.resize(width,height)
            app.expect_screen("Cluster Pulse")
            app.capture_screen(f"overview-normal-{width}x{height}")
            app.send("\x1b[H\x1b[B\x1b[H")
            columns = any("Deployments" in row and "Attention" in row for row in app.screen()["rows"])
            app.send("\x1b[C" if columns else "\x1b[F\x1b[B")
            app.expect_screen("Attention")
            if width == 80:
                app.expect_screen("CPU 8%")
            app.capture_screen(f"overview-attention-{width}x{height}")
            app.send("\x1b[H\x1b[A" if columns else "\x1b[H\x1b[A\x1b[H\x1b[A")
        app.send("\x1b[H\x1b[B\x1b[C\x1b[C")
        app.expect_screen("Infrastructure Snapshot")
        app.send("\x1b[H" + "\x1b[B" * 6 + "\r")
        app.expect_screen("Helm · Releases")
        app.expect_screen("metrics")
        app.send("\x1b")
        app.expect_screen("↑↓ Counters")
        app.send("\x1b[B\x1b[B\x1b[B\r")
        app.expect_screen("Helm · Failed")
        app.expect_screen("No matching objects")
        app.send("\x1b")
        app.expect_screen("↑↓ Counters")
        app.send("k")
        app.expect_screen("Infrastructure Snapshot")
        app.send("e")
        app.capture_screen("overview-inventory-expanded")
        app.send("\x1b")
        app.send("ff")
        app.expect_screen("Recent Deployments")
        app.capture_screen("overview-fullscreen-220x50")
        app.send("ffii")
        app.expect_screen("AI panel: double")
        app.capture_screen("overview-assistant-double")
        app.send("ii")
        app.expect_screen("AI panel hidden")
        app.send(":1\r")
        app.expect_screen("Overview")
        app.send("\x03")
        app.assert_exit()
        self.assert_read_only()

    def test_current_context_shares_discovered_tool_navigation(self):
        self.original = self.original.replace(b"kind: Config\n", b"kind: Config\ncurrent-context: local-test\n")
        self.config.write_bytes(self.original)
        app = self.start(width=180, height=50)
        app.expect_screen("Current Workspace")
        app.send("\x1b[1;3D/local-test\r")
        app.send("\x1b[D\x1b[C")
        app.expect_screen("Argo")
        app.expect_screen("Flux")
        self.assertFalse(any("Helm" in row for row in app.screen()["rows"]))
        app.send("\x1b[B\x1b[B\r")
        app.expect_screen("Synced")
        app.expect_screen("Degraded")
        self.select_category(app, "Applications")
        app.expect_screen("payments")
        app.send("\x1b[1;3D/local-test\r")
        app.send("\x1b[D\x1b[C")
        app.send("\x1b[B" * 3 + "\r")
        app.expect_screen("health check failed: Deployment/shop/payments is not ready")
        app.expect_screen("platform")
        self.assertFalse(any(self.state.joinpath("workspaces").iterdir()))
        self.assert_read_only()
        app.send("\x03")
        app.assert_exit()

    def select_category(self, app, name):
        app.send("\x1b[1;3D/" + name)
        # Kubernetes also exposes these CRDs before the Monitor category match.
        if name != "Sources":
            app.send("\x1b[B\x1b[B")
        app.send("\r")
        app.expect_screen(name)

    def test_argo_status_resource_and_back_preserve_list_scope(self):
        app = self.open_tool()
        app.expect_screen("Synced")
        app.expect_screen("Degraded")
        app.expect_screen("OutOfSync")
        app.expect_screen("Healthy")
        app.send("/payments\r")
        app.expect_screen("Deployment payments exceeded its progress deadline")
        app.expect_screen("Deployment/payments")
        app.expect_screen("Not reported")
        app.send("\r")
        app.expect_screen("Describe")
        app.expect_screen("payments")
        app.expect_screen("ProgressDeadlineExceeded")
        self.assertTrue(any(path == "/apis/apps/v1/namespaces/shop/deployments/payments" for _, path, _ in self.requests))
        app.send("\x1b")
        app.expect_screen("Configuration & revisions")
        app.expect_screen("Degraded")
        app.send("\x1b")
        app.expect_screen("payments")
        screen = "\n".join(app.screen()["rows"])
        self.assertNotIn("remote-reports", screen)
        self.assertNotIn("worker", screen)
        app.resize(80, 16)
        app.send("ff")
        app.expect_screen("Synced")
        app.expect_screen("Degraded")
        app.capture_screen("gitops-argo-compact")
        app.send("ff")
        app.resize(160, 40)
        app.send("\x7f/remote-reports\r")
        before = len(self.requests)
        app.send("\r")
        app.expect_screen("Destination access unavailable")
        self.assertFalse(any("/deployments/reports" in path for _, path, _ in self.requests[before:]))
        app.send("\x03")
        app.assert_exit()
        self.assert_read_only()

    def test_flux_dependency_source_and_status_are_cluster_observations(self):
        app = self.open_tool("Flux")
        for label in ["Kustomizations", "Not ready", "Suspended", "Awaiting status"]:
            app.expect_screen(label)
        app.send("/storefront\r")
        app.expect_screen("dependency platform is not ready")
        app.expect_screen("Dependency: Kustomization/platform")
        app.send("\r")
        app.expect_screen("health check failed: Deployment/shop/payments is not ready")
        app.expect_screen("Source: GitRepository/platform-config")
        app.send("\x1b")
        app.expect_screen("dependency platform is not ready")
        app.send("\x1b[B\r")
        app.expect_screen("GitRepository/platform-config")
        app.expect_screen("stored artifact for revision main@sha1:latest")
        app.send("\x1b")
        app.send("\x1b")
        app.send("\x7f/preview\r")
        app.expect_screen("Suspended")
        app.expect_screen("Applied revision:")
        app.expect_screen("main@sha1:previous")
        app.send("\x1b")
        self.select_category(app, "HelmReleases")
        app.expect_screen("metrics")
        app.send("/metrics\r")
        app.expect_screen("Helm install succeeded")
        app.send("\x1b[F\r")
        app.expect_screen("1.4.2")
        app.send("\x03")
        app.assert_exit()
        self.assert_read_only()

    def test_denied_clusterwide_list_can_use_readable_namespace(self):
        self.denied.add("/apis/argoproj.io/v1alpha1/applications")
        app = self.open_tool()
        app.expect_screen("Read denied")
        screen = "\n".join(app.screen()["rows"])
        self.assertNotIn("No objects", screen)
        app.send("n")
        app.expect_screen("All namespaces")
        app.send("argocd\r")
        app.expect_screen("payments")
        app.expect_screen("Degraded")
        self.assertTrue(any(path == "/apis/argoproj.io/v1alpha1/namespaces/argocd/applications" for _, path, _ in self.requests))
        app.send("\x03")
        app.assert_exit()
        self.assert_read_only()

    def test_failed_refresh_retains_rows_then_empty_list_clears_them(self):
        app = self.open_tool()
        app.expect_screen("payments")
        self.failed.add("/apis/argoproj.io/v1alpha1/applications")
        app.send("g")
        app.expect_screen("Connection unavailable")
        app.expect_screen("Previous observations")
        app.expect_screen("payments")
        self.failed.clear()
        self.objects[("argoproj.io/v1alpha1", "applications")] = []
        app.send("g")
        app.expect_screen("No Applications")
        self.assertNotIn("payments", "\n".join(app.screen()["rows"]))
        app.send("\x03")
        app.assert_exit()
        self.assert_read_only()

    def test_flux_categories_isolate_denial_and_keep_parent_when_source_read_is_denied(self):
        self.denied.add("/apis/helm.toolkit.fluxcd.io/v2/helmreleases")
        self.denied.add("/apis/source.toolkit.fluxcd.io/v1/namespaces/flux-system/gitrepositories/platform-config")
        app = self.open_tool("Flux")
        app.expect_screen("platform")
        self.assertNotIn("Partial results", "\n".join(app.screen()["rows"]))
        app.send("/platform\r\r")
        app.expect_screen("Read denied")
        app.expect_screen("health check failed")
        self.select_category(app, "HelmReleases")
        app.expect_screen("Read denied")
        app.send("\x03")
        app.assert_exit()
        self.assert_read_only()

    def test_category_tables_preview_empty_sources_and_projects(self):
        app = self.open_tool()
        app.expect_screen("payments")
        app.expect_screen("Enter Details")
        app.capture_screen("gitops-argo-table-preview")
        self.select_category(app, "ApplicationSets")
        app.expect_screen("No ApplicationSets")
        self.select_category(app, "Projects")
        app.expect_screen("default")
        app.send("/default\r")
        app.expect_screen("Commerce applications")
        app.expect_screen("Applications in namespace argocd")
        app.expect_screen("payments")
        app.send("\r")
        app.expect_screen("Application/payments")
        app.send("\x1b")
        app.expect_screen("AppProject/default")
        self.select_category(app, "Sources")
        app.expect_screen("platform-config")
        app.expect_screen("Fetched revision")
        app.expect_screen("main@sha1:latest")
        app.capture_screen("gitops-flux-sources-table-preview")
        app.send("\r")
        app.expect_screen("GitRepository/platform-config")
        app.send("\x03")
        app.assert_exit()
        self.assert_read_only()
