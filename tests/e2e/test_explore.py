"""Exercise Explore against a temporary Kubernetes API, never a real cluster."""

from copy import deepcopy
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import queue
from pathlib import Path
import tempfile
import threading
import time
import unittest
from urllib.parse import parse_qs, urlsplit

from support import Terminal


class ExploreFixture(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory(prefix="plous-explore-")
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        self.state = self.root / "state"
        self.requests = []
        self.mutations = []
        self.denied_access = set()
        self.deleted = set()
        self.reject_mutations = False
        self.extra_api_groups = []
        self.api_warning = ""
        self.empty_namespace = ""
        self.namespace_list_denied = False
        self.pod_list_gate = None
        self.log_chunks = None
        self.log_denied = False
        self.extra_pods = []
        self.denied_log_pods = set()
        self.previous_logs_missing = False
        self.exec_error = "fixture: executable not found; choose another shell"
        self.secret = None
        self.metrics_enabled = False
        self.metrics_denied = False
        self.metrics_reads = 0
        self.metrics_times = []
        self.container_metrics = None
        self.node = None
        self.quota = None
        self.pod = {
            "apiVersion": "v1", "kind": "Pod",
            "metadata": {"name": "checkout-test", "namespace": "shop", "uid": "pod-id", "resourceVersion": "10",
                         "ownerReferences": [{"apiVersion": "apps/v1", "kind": "ReplicaSet", "name": "checkout-rs", "uid": "rs-id", "controller": True}]},
            "spec": {"nodeName": "test-worker", "containers": [{"name": "checkout", "image": "example/checkout:v1"}]},
            "status": {"phase": "Running", "containerStatuses": [{"name": "checkout", "ready": False, "restartCount": 12,
                       "state": {"waiting": {"reason": "CrashLoopBackOff"}}}]},
        }
        self.deployment = {
            "apiVersion": "apps/v1", "kind": "Deployment",
            "metadata": {"name": "checkout-deployment", "namespace": "shop", "uid": "deployment-id", "resourceVersion": "20"},
            "spec": {"replicas": 1, "selector": {"matchLabels": {"app": "checkout"}},
                     "template": {"metadata": {"labels": {"app": "checkout"}},
                                  "spec": {"containers": [{"name": "checkout", "image": "example/checkout:v1"}]}}},
            "status": {"readyReplicas": 0},
        }
        self.replica_set = {
            "apiVersion": "apps/v1", "kind": "ReplicaSet",
            "metadata": {"name": "checkout-rs", "namespace": "shop", "uid": "rs-id", "resourceVersion": "15",
                         "ownerReferences": [{"apiVersion": "apps/v1", "kind": "Deployment", "name": "checkout-deployment",
                                              "uid": "deployment-id", "controller": True}]},
            "spec": {"replicas": 1, "selector": {"matchLabels": {"app": "checkout"}}},
        }
        self.service = {
            "apiVersion": "v1", "kind": "Service",
            "metadata": {"name": "checkout-service", "namespace": "shop", "uid": "service-id", "resourceVersion": "30"},
            "spec": {"type": "ClusterIP", "clusterIP": "10.0.0.42", "selector": {"app": "checkout"},
                     "ports": [{"port": 80, "targetPort": 8080, "protocol": "TCP"}]},
        }
        examples = json.loads((Path(__file__).resolve().parents[1] / "fixtures/usability/resources.json").read_text())
        self.stateful_set = next(obj for obj in examples if obj["kind"] == "StatefulSet")
        self.ingress = next(obj for obj in examples if obj["kind"] == "Ingress")
        for obj in [self.stateful_set, self.ingress]:
            obj["metadata"]["namespace"] = "shop"
        self.ingress["metadata"]["name"] = "checkout-ingress"
        self.ingress["spec"]["rules"][0]["http"]["paths"][0]["backend"]["service"]["name"] = "checkout-service"
        test = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass

            def respond(self, value, status=200):
                data = json.dumps(value).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(data)))
                if test.api_warning:
                    self.send_header("Warning", '299 fixture "' + test.api_warning + '"')
                self.end_headers()
                self.wfile.write(data)

            def do_GET(self):
                parsed = urlsplit(self.path)
                path, query = parsed.path, parse_qs(parsed.query)
                test.requests.append(("GET", path, query))
                if path.endswith("/pods") and query.get("watch") != ["true"] and test.pod_list_gate is not None:
                    test.pod_list_gate.wait(8)
                if "/namespaces/platform/" in path or (path == "/api/v1/namespaces" and test.namespace_list_denied):
                    self.respond({"apiVersion": "v1", "kind": "Status", "status": "Failure", "reason": "Forbidden", "code": 403}, 403)
                elif path == "/apis":
                    self.respond({"kind": "APIGroupList", "apiVersion": "v1", "groups": [
                        {"name": "apps", "preferredVersion": {"groupVersion": "apps/v1", "version": "v1"}},
                        {"name": "networking.k8s.io", "preferredVersion": {"groupVersion": "networking.k8s.io/v1", "version": "v1"}}
                    ] + [{"name": group, "preferredVersion": {"groupVersion": group + "/v1", "version": "v1"}}
                         for group in test.extra_api_groups + (["metrics.k8s.io"] if test.metrics_enabled else [])]})
                elif path == "/apis/metrics.k8s.io/v1":
                    self.respond({"kind": "APIResourceList", "apiVersion": "v1", "groupVersion": "metrics.k8s.io/v1",
                                  "resources": [{"name": "pods", "kind": "PodMetrics", "namespaced": True, "verbs": ["get", "list"]}] +
                                  ([{"name": "nodes", "kind": "NodeMetrics", "namespaced": False, "verbs": ["get", "list"]}] if test.node is not None else [])})
                elif path.startswith("/apis/metrics.k8s.io/v1/"):
                    test.metrics_reads += 1
                    test.metrics_times.append((path, time.monotonic()))
                    if test.metrics_denied:
                        self.failure("Forbidden", 403)
                        return
                    metrics = {"apiVersion": "metrics.k8s.io/v1", "kind": "PodMetrics",
                               "metadata": {"name": test.pod["metadata"]["name"], "namespace": "shop"},
                               "timestamp": datetime.now(timezone.utc).isoformat(), "window": "15s",
                               "containers": [{"name": "checkout", "usage": {"cpu": f"{test.metrics_reads * 25}m", "memory": "32Mi"}}]}
                    if test.container_metrics is not None:
                        metrics["containers"] = test.container_metrics
                    if "/nodes" in path and test.node is not None:
                        metrics["kind"] = "NodeMetrics"
                        metrics["metadata"] = {"name": test.node["metadata"]["name"]}
                        metrics["usage"] = {"cpu": "500m", "memory": "512Mi"}
                        metrics.pop("containers")
                    if path.endswith("/nodes"):
                        metrics = {"apiVersion": "metrics.k8s.io/v1", "kind": "NodeMetricsList", "metadata": {}, "items": [metrics]}
                    if path.endswith("/pods"):
                        metrics = {"apiVersion": "metrics.k8s.io/v1", "kind": "PodMetricsList", "metadata": {}, "items": [metrics]}
                    self.respond(metrics)
                elif path.startswith("/apis/fixture-"):
                    self.respond({"kind": "APIResourceList", "apiVersion": "v1", "resources": []})
                elif path in ("/api/v1", "/apis/apps/v1", "/apis/networking.k8s.io/v1"):
                    kinds = [("pods", "Pod", ["po"]), ("services", "Service", ["svc"]),
                             ("events", "Event", ["ev"]), ("namespaces", "Namespace", ["ns"])] if path == "/api/v1" else [
                        ("deployments", "Deployment", ["deploy"]), ("replicasets", "ReplicaSet", ["rs"]),
                        ("statefulsets", "StatefulSet", ["sts"])
                    ]
                    if path == "/apis/networking.k8s.io/v1":
                        kinds = [("ingresses", "Ingress", ["ing"])]
                    if path == "/api/v1" and test.secret is not None:
                        kinds.append(("secrets", "Secret", []))
                    if path == "/api/v1" and test.node is not None:
                        kinds.append(("nodes", "Node", ["no"]))
                    if path == "/api/v1" and test.quota is not None:
                        kinds.append(("resourcequotas", "ResourceQuota", ["quota"]))
                    resources = [{"name": name, "kind": kind, "namespaced": name not in ("namespaces", "nodes"),
                                  "verbs": ["get", "list", "watch", "update", "patch", "delete"], "shortNames": aliases}
                                 for name, kind, aliases in kinds]
                    subresources = [("pods/log", "Pod", ["get"]), ("pods/exec", "PodExecOptions", ["create", "get"])] if path == "/api/v1" else [
                        ("deployments/scale", "Scale", ["get", "update", "patch"]),
                        ("statefulsets/scale", "Scale", ["get", "update", "patch"])
                    ]
                    if path == "/apis/networking.k8s.io/v1":
                        subresources = []
                    resources += [{"name": name, "kind": kind, "namespaced": True, "verbs": verbs} for name, kind, verbs in subresources]
                    self.respond({"kind": "APIResourceList", "apiVersion": "v1", "groupVersion": "v1" if path == "/api/v1" else path.removeprefix("/apis/"),
                                  "resources": resources})
                elif path.endswith("/exec"):
                    self.respond({"apiVersion":"v1", "kind":"Status", "status":"Failure", "reason":"BadRequest", "message":test.exec_error, "code":400},400)
                elif path.endswith("/log"):
                    if test.log_denied or path.split("/")[-2] in test.denied_log_pods:
                        self.failure("Forbidden", 403)
                        return
                    if test.previous_logs_missing and query.get("previous") == ["true"]:
                        self.failure("BadRequest", 400)
                        return
                    if test.log_chunks is not None:
                        self.send_response(200)
                        self.send_header("Content-Type", "text/plain")
                        self.send_header("Connection", "close")
                        self.end_headers()
                        try:
                            while (chunk := test.log_chunks.get(timeout=10)) is not None:
                                self.wfile.write(chunk)
                                self.wfile.flush()
                        except (BrokenPipeError, ConnectionResetError, queue.Empty):
                            pass
                        return
                    data = b"ERROR missing database setting\n"
                    if query.get("timestamps") == ["true"]:
                        data = b"2026-09-29T10:00:00Z " + data
                    self.send_response(200)
                    self.send_header("Content-Type", "text/plain")
                    self.send_header("Content-Length", str(len(data)))
                    self.end_headers()
                    self.wfile.write(data)
                elif query.get("watch") == ["true"]:
                    self.respond({"type": "BOOKMARK", "object": {"apiVersion": "v1", "kind": "Pod", "metadata": {"resourceVersion": "10"}}})
                elif path.endswith("/deployments/checkout-deployment/scale"):
                    self.respond({"apiVersion": "autoscaling/v1", "kind": "Scale", "metadata": deepcopy(test.deployment["metadata"]),
                                  "spec": {"replicas": test.deployment["spec"]["replicas"]}, "status": {"replicas": 1, "selector": "app=checkout"}})
                elif self.object_for(path) is not None:
                    target = self.object_for(path)
                    if target["metadata"]["uid"] in test.deleted:
                        self.failure("NotFound", 404)
                    else:
                        self.respond(target)
                else:
                    name = path.rsplit("/", 1)[-1]
                    items = []
                    kind, version = "Pod", "v1"
                    if name == "pods":
                        scope_matches = "/namespaces/" not in path or "/namespaces/" + test.pod["metadata"]["namespace"] + "/" in path
                        if test.pod["metadata"]["uid"] not in test.deleted and scope_matches and (not test.empty_namespace or "/namespaces/" + test.empty_namespace + "/" not in path):
                            items = [test.pod] + [pod for pod in test.extra_pods if pod["metadata"]["uid"] not in test.deleted]
                    elif name == "namespaces":
                        kind = "Namespace"
                        items = [{"apiVersion": "v1", "kind": kind, "metadata": {"name": ns, "uid": "ns-" + ns}, "status": {"phase": "Active"}}
                                 for ns in ["default", "kube-system", "shop", "platform"]]
                    elif name == "events":
                        kind = "Event"
                        items = [{"apiVersion": "v1", "kind": "Event", "metadata": {"name": "pod-event", "namespace": "shop", "uid": "event-id"},
                                  "type": "Warning", "reason": "BackOff", "message": "Back-off restarting failed container checkout", "count": 12,
                                  "involvedObject": {"kind": "Pod", "name": "checkout-test", "uid": "pod-id"}}]
                    elif name == "deployments":
                        kind, version = "Deployment", "apps/v1"
                        items = [test.deployment] if test.deployment["metadata"]["uid"] not in test.deleted else []
                    elif name == "statefulsets":
                        kind, version, items = "StatefulSet", "apps/v1", [test.stateful_set]
                    elif name == "ingresses":
                        kind, version, items = "Ingress", "networking.k8s.io/v1", [test.ingress]
                    elif name == "replicasets":
                        kind, version = "ReplicaSet", "apps/v1"
                        items = [test.replica_set]
                    elif name == "services":
                        kind = "Service"
                        items = [test.service]
                    elif name == "nodes" and test.node is not None:
                        kind, items = "Node", [test.node]
                    elif name == "resourcequotas" and test.quota is not None:
                        kind, items = "ResourceQuota", [test.quota]
                    elif name == "secrets" and test.secret is not None:
                        kind, items = "Secret", [test.secret]
                    self.respond({"apiVersion": version, "kind": kind + "List", "metadata": {"resourceVersion": "10"}, "items": items})

            def object_for(self, path):
                for resource, obj in [("nodes", test.node), ("resourcequotas", test.quota)]:
                    if obj is not None and path.endswith("/" + resource + "/" + obj["metadata"]["name"]):
                        return obj
                for pod in test.extra_pods:
                    if path.endswith("/pods/" + pod["metadata"]["name"]):
                        return pod
                if test.secret is not None and path.endswith("/secrets/" + test.secret["metadata"]["name"]):
                    return test.secret
                for resource, obj in [("pods", test.pod), ("deployments", test.deployment),
                                      ("replicasets", test.replica_set), ("services", test.service),
                                      ("statefulsets", test.stateful_set), ("ingresses", test.ingress)]:
                    if path.endswith("/" + resource + "/" + obj["metadata"]["name"]):
                        return obj
                return None

            def failure(self, reason, status):
                self.respond({"apiVersion": "v1", "kind": "Status", "status": "Failure", "reason": reason, "code": status}, status)

            def receive(self):
                parsed = urlsplit(self.path)
                path, query = parsed.path, parse_qs(parsed.query)
                body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))) or b"{}")
                test.requests.append((self.command, path, query))
                return path, query, body

            def do_POST(self):
                path, query, body = self.receive()
                if path.endswith("/exec"):
                    self.respond({"apiVersion":"v1", "kind":"Status", "status":"Failure", "reason":"BadRequest", "message":test.exec_error, "code":400},400)
                elif path == "/apis/authorization.k8s.io/v1/selfsubjectaccessreviews":
                    attrs = body["spec"]["resourceAttributes"]
                    key = (attrs["resource"], attrs["verb"], attrs.get("subresource", ""))
                    denied = key in test.denied_access
                    self.respond({"apiVersion": "authorization.k8s.io/v1", "kind": "SelfSubjectAccessReview",
                                  "status": {"allowed": not denied, "denied": denied}})
                else:
                    self.failure("NotFound", 404)

            def do_PUT(self):
                path, query, body = self.receive()
                test.mutations.append(("PUT", path, query, deepcopy(body)))
                if test.reject_mutations:
                    self.failure("Forbidden", 403)
                    return
                scale = path.endswith("/scale")
                target = self.object_for(path.removesuffix("/scale") if scale else path)
                if target is None:
                    self.failure("NotFound", 404)
                    return
                metadata = body.get("metadata", {})
                if any(metadata.get(key) != target["metadata"].get(key) for key in ("uid", "resourceVersion")):
                    self.failure("Conflict", 409)
                    return
                if query.get("dryRun") == ["All"]:
                    self.respond(body)
                    return
                if scale:
                    target["spec"]["replicas"] = body["spec"]["replicas"]
                else:
                    target.clear()
                    target.update(deepcopy(body))
                target["metadata"]["resourceVersion"] = str(int(target["metadata"]["resourceVersion"]) + 1)
                body["metadata"]["resourceVersion"] = target["metadata"]["resourceVersion"]
                self.respond(body)

            def do_PATCH(self):
                path, query, body = self.receive()
                test.mutations.append(("PATCH", path, query, deepcopy(body)))
                target = self.object_for(path)
                if target is None:
                    self.failure("NotFound", 404)
                    return
                if test.reject_mutations:
                    self.failure("Forbidden", 403)
                    return
                for operation in body:
                    if operation["op"] == "test":
                        actual = target
                        for key in operation["path"].strip("/").split("/"):
                            actual = actual.get(key)
                        if actual != operation["value"]:
                            self.failure("Conflict", 409)
                            return
                for operation in body:
                    if operation["op"] == "add":
                        keys = operation["path"].strip("/").split("/")
                        parent = target
                        for key in keys[:-1]:
                            parent = parent.setdefault(key, {})
                        parent[keys[-1]] = deepcopy(operation["value"])
                target["metadata"]["resourceVersion"] = str(int(target["metadata"]["resourceVersion"]) + 1)
                self.respond(target)

            def do_DELETE(self):
                path, query, body = self.receive()
                test.mutations.append(("DELETE", path, query, deepcopy(body)))
                target = self.object_for(path)
                if target is None:
                    self.failure("NotFound", 404)
                    return
                if test.reject_mutations:
                    self.failure("Forbidden", 403)
                    return
                if body.get("preconditions", {}).get("uid") != target["metadata"]["uid"]:
                    self.failure("Conflict", 409)
                    return
                test.deleted.add(target["metadata"]["uid"])
                self.respond({"apiVersion": "v1", "kind": "Status", "status": "Success", "code": 200})

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.server.daemon_threads = True
        threading.Thread(target=self.server.serve_forever, daemon=True).start()
        self.addCleanup(self.server.server_close)
        self.addCleanup(self.server.shutdown)
        self.config = self.root / "kubeconfig"
        self.original = (
            "apiVersion: v1\nkind: Config\ncontexts:\n- name: local-test\n"
            "  context: {cluster: local, user: tester, namespace: shop}\n"
            f"clusters:\n- name: local\n  cluster: {{server: 'http://127.0.0.1:{self.server.server_port}'}}\n"
            "users:\n- name: tester\n  user: {}\n"
        ).encode()
        self.config.write_bytes(self.original)

    def start(self):
        app = Terminal(self.state, self.config, width=160, height=40, env_overrides=getattr(self, "terminal_env", None))
        self.addCleanup(app.close)
        app.expect("AI assistant")
        return app

    def open_resource(self, resource="pods"):
        app = self.start()
        app.send(":new\rShop\r\r")
        app.expect("Add Connection")
        app.send("\r")
        app.expect("Add Context")
        app.send(" \x1b[B\r")
        app.expect("Added 1 Kubernetes")
        app.send(":" + resource + "\r")
        app.expect("Choose Kubernetes connection")
        app.send("\r")
        app.expect("checkout-")
        return app


class ExploreTests(ExploreFixture):
    def test_connection_discovery_keeps_tool_branches_collapsed(self):
        app = self.start()
        app.send(":new\rShop\r\r\r")
        app.expect_screen("Add Context")
        app.send(" \x1b[B\r")
        app.expect_screen("Added 1 Kubernetes")
        app.send(":2\r")
        app.send("\x1b")
        app.send("/local-test\r")
        app.expect_screen("checkout-test")
        app.expect_screen("▸ Kubernetes")
        self.assertNotIn("Helm", "\n".join(row[:30] for row in app.screen()["rows"]))
        app.send("\x1b[1;3C:g\r")
        app.expect_screen("checkout-test")
        app.resize(140, 36)
        app.expect_screen("▸ Kubernetes")
        self.assertNotIn("Helm", "\n".join(row[:30] for row in app.screen()["rows"]))
        app.send("\x03")
        app.assert_exit()
        app.close()

        requests_before_restart = len(self.requests)
        restarted = self.start()
        restarted.expect_screen("Current Workspace")
        self.assertEqual(len(self.requests), requests_before_restart)
        restarted.send("\x1b[1;3D/local-test\r")
        restarted.expect_screen("checkout-test")
        restarted.expect_screen("▸ Kubernetes")
        self.assertNotIn("Helm", "\n".join(row[:30] for row in restarted.screen()["rows"]))
        restarted.send(":pods\r")
        restarted.expect_screen("checkout-test")
        restarted.expect_screen("▸ Kubernetes")
        self.assertNotIn("Helm", "\n".join(row[:30] for row in restarted.screen()["rows"]))
        restarted.send("\x1b")
        restarted.send("/pods\r")
        restarted.expect_screen("▾ Kubernetes")
        restarted.expect_screen("▾ Workloads")
        self.assertNotIn("Helm", "\n".join(row[:30] for row in restarted.screen()["rows"]))
        restarted.send("\x03")
        restarted.assert_exit()
        self.assertEqual(self.config.read_bytes(), self.original)

    def test_workspace_area_and_connection_folding_use_the_same_controls(self):
        app = self.open_resource()
        app.send("\x1b[1;3D/local-test\r")
        saved = app.wait_json(self.state / "session.json", lambda data: data["focus"] == 0 and data["expanded"].get(data["connection"]))
        workspace, connection = saved["workspace"], saved["connection"]
        area = workspace + "/monitor"

        app.send("\x1b[D\x1b[D\r")
        app.wait_json(self.state / "session.json", lambda data: data["expanded"].get(area) is False and data["tool"] == "monitor" and not data["connection"])
        app.send("\x1b[D\r")
        app.wait_json(self.state / "session.json", lambda data: data["expanded"].get(workspace) is False and not data["tool"])

        app.send("\x1b[C\x1b[B\x1b[C\x1b[B\x1b[C")
        saved = app.wait_json(self.state / "session.json", lambda data: all(data["expanded"].get(key) for key in [workspace, area, connection]))
        self.assertEqual(saved["focus"], 0)
        self.assertEqual(saved["mode"], 1)
        self.assertFalse(saved["tool"])
        self.assertFalse(saved["connection"])

        app.send("\r")
        app.wait_json(self.state / "session.json", lambda data: data["connection"] == connection and data["expanded"].get(connection) is True)
        app.send("\r")
        app.wait_json(self.state / "session.json", lambda data: data["expanded"].get(connection) is False)
        app.send("\x03")
        app.assert_exit()
        self.assertEqual(self.config.read_bytes(), self.original)

    def test_connection_collapse_survives_refresh_and_restart(self):
        app = self.open_resource()
        app.send("\x1b[1;3D/local-test\r")
        saved = app.wait_json(self.state / "session.json", lambda data: data["focus"] == 0 and data["expanded"].get(data["connection"]))
        connection = saved["connection"]
        start = len(app.output)
        app.send("\r")
        app.expect("▸", start)
        app.wait_json(self.state / "session.json", lambda data: data["expanded"].get(connection) is False)

        start = len(app.output)
        app.send(":refresh\r")
        app.expect("checkout-test", start)
        app.resize(140, 36)
        app.send("\x03")
        app.assert_exit()
        saved = json.loads((self.state / "session.json").read_text())
        self.assertFalse(saved["expanded"][connection])
        self.assertEqual(saved["connection"], connection)
        self.assertEqual(saved["focus"], 0)

        app = self.start()
        app.expect_screen("Current Workspace")
        app.expect_screen("▸ local-test")
        app.send("\x1b[1;3D\x1b[C")
        app.wait_json(self.state / "session.json", lambda data: data["focus"] == 0 and data["expanded"].get(connection))
        app.send("\x1b[D")
        app.wait_json(self.state / "session.json", lambda data: data["expanded"].get(connection) is False)
        app.send("\x03")
        app.assert_exit()
        self.assertEqual(self.config.read_bytes(), self.original)

    def test_loading_animates_without_input_and_stops_after_result(self):
        self.pod_list_gate = threading.Event()
        self.addCleanup(self.pod_list_gate.set)
        app = self.start()
        app.send(":new\rShop\r\r\r")
        app.expect("Add Context")
        app.send(" \x1b[B\r")
        app.expect("Added 1 Kubernetes")
        app.send(":pods\r")
        app.expect("Choose Kubernetes connection")
        app.send("\r")
        app.expect("Loading Pods")
        start = len(app.output)
        app.wait(lambda: app.text(start).count("●") >= 3, "animated dot sweep")
        lists = [r for r in self.requests if r[1].endswith("/pods") and r[2].get("watch") != ["true"]]
        self.assertEqual(len(lists), 1)
        app.send(":h\r")
        app.expect("Global · Navigation")
        app.send("\x1b")
        self.pod_list_gate.set()
        app.expect("checkout-test")
        app.read(0.2)
        start = len(app.output)
        app.read(0.4)
        self.assertNotIn("●", app.text(start))
        app.send("\x03")
        app.assert_exit()

    def test_log_backlog_keeps_navigation_responsive(self):
        self.log_chunks = queue.Queue()
        self.addCleanup(lambda: self.log_chunks.put(None))
        app = self.open_resource()
        app.send("\r")
        app.expect("Pod overview")
        app.send("l")
        app.wait(lambda: any(path.endswith("/log") for _, path, _ in self.requests), "log request")
        self.log_chunks.put(b"".join(f"backlog {i:05d} received request\n".encode() for i in range(10000)) + b"backlog finished\n")
        app.wait(lambda: "backlog finished" in "\n".join(app.screen()["rows"]), "large log backlog", timeout=5)
        app.expect("older lines removed")
        app.send(" ")
        app.expect("Reading")
        # Keep another backlog arriving while Help opens and Logs closes.
        self.log_chunks.put(b"streaming request\n" * 100000)
        app.send(":h\r")
        app.wait(lambda: "Global" in "\n".join(app.screen()["rows"]), "Help during log burst", timeout=3)
        app.send("\x1b")
        app.send("\x1b")
        app.wait(lambda: "[d Describe]" in "\n".join(app.screen()["rows"]), "leave streaming logs", timeout=3)
        app.send("\x03")
        app.assert_exit()
        self.assertFalse(self.mutations)
        self.assertEqual(self.config.read_bytes(), self.original)

    def test_logs_keep_collecting_while_reading_and_resume_without_refetch(self):
        self.log_chunks = queue.Queue()
        self.addCleanup(lambda: self.log_chunks.put(None))
        app = self.open_resource()
        app.send("\r")
        app.expect("Pod overview")
        app.send("l")
        app.wait(lambda: any(path.endswith("/log") for _, path, _ in self.requests), "log request")
        self.log_chunks.put(b"".join(f"older record {i:03d}\n".encode() for i in range(60)) + b"initial batch finished\n")
        app.expect("initial batch finished")
        app.send("\x1b[5~")
        app.expect("Reading")
        start = len(app.output)
        self.log_chunks.put(b"retained new record\n")
        app.expect("1 new line", start)
        self.assertNotIn("retained new record", app.text(start))
        app.send("/older record")
        app.expect("Search")
        start = len(app.output)
        self.log_chunks.put(b"latest after search\n")
        app.read(0.15)
        app.send("\x1b")
        start = len(app.output)
        app.send(" ")
        app.expect("latest after search", start)
        logs = [request for request in self.requests if request[1].endswith("/log")]
        self.assertEqual(len(logs), 1)
        start = len(app.output)
        app.send("\x1b")
        app.expect("[d Describe]", start)
        app.send("\x03")
        app.assert_exit()
        self.assertEqual(self.mutations, [])
        self.assertEqual(self.config.read_bytes(), self.original)

    def test_namespace_picker_and_failed_container_investigation(self):
        self.pod["spec"]["containers"].insert(0, {"name": "proxy", "image": "example/proxy:v1"})
        self.pod["status"]["containerStatuses"].insert(0, {"name": "proxy", "ready": True, "restartCount": 0, "state": {"running": {}}})
        self.pod["status"]["containerStatuses"][1]["lastState"] = {"terminated": {"reason": "OOMKilled", "exitCode": 137}}
        app = self.start()
        app.send(":new\rShop\r\r")
        app.expect("Add Connection")
        app.send("\r")
        app.expect("Add Context")
        app.send(" \x1b[B\r")
        app.expect("Added 1 Kubernetes")
        app.send(":pods\r")
        app.expect("Choose Kubernetes connection")
        app.send("\r")
        app.expect("CrashLoopBackOff")
        app.expect("Workloads")
        app.send("n")
        app.expect("kube-system")
        app.send("kube-system\r")
        app.expect("No Pods in namespace kube-system")
        app.send(":N shop\r")
        app.expect("checkout-test")
        app.send("\r")
        app.expect("Pod overview")
        app.expect("OOMKilled")
        app.send("p")
        app.expect("ERROR missing database setting")
        app.wait(lambda: any(path.endswith("/log") and query.get("previous") == ["true"] and query.get("container") == ["checkout"]
                             for _, path, query in self.requests), "previous logs for failing container")
        app.send("\x1b")
        self.namespace_list_denied = True
        app.send("n")
        app.expect("Access denied by Kubernetes RBAC")
        app.send("platform\r\r")
        app.wait(lambda: any("/namespaces/platform/pods" in path for _, path, _ in self.requests), "manually selected namespace")
        app.send("\x03")
        app.assert_exit()
        self.assertEqual(self.config.read_bytes(), self.original)

    def test_discovery_diagnostics_do_not_escape_into_terminal(self):
        self.extra_api_groups = [f"fixture-{i}.example.io" for i in range(28)]
        self.api_warning = "fixture API warning"
        self.empty_namespace = "default"
        self.pod["metadata"]["namespace"] = "kube-system"
        self.original = self.original.replace(b"namespace: shop", b"namespace: default")
        self.config.write_bytes(self.original)
        app = self.start()
        app.send(":new\rMinikube test\r\r")
        app.expect("Add Connection")
        app.send("\r")
        app.expect("Add Context")
        app.send(" \x1b[B\r")
        app.expect("Added 1 Kubernetes")
        app.send(":pods\r\r")
        app.expect_screen("No Pods in namespace default.")
        app.expect_screen("[n] Choose namespace")
        app.expect_screen("[g] Refresh")
        screen = app.capture_screen("empty-pods-namespace")
        self.assertIn("+------+ |", "\n".join(screen))
        self.assertNotIn(":k Resource types", "\n".join(screen))
        output = app.text()
        self.assertNotIn("Waited before sending request", output)
        self.assertNotIn("fixture API warning", output)
        self.assertNotIn("request.go:", output)

        row = next(i for i, line in enumerate(screen) if "[n] Choose namespace" in line)
        column = screen[row].index("[n] Choose namespace")
        app.mouse_click(column, row)
        app.expect_screen("All namespaces")
        app.send("\x1b[H\r")
        app.expect("checkout-test")
        app.expect("kube-system")
        app.send(":N platform\r")
        app.expect("Access denied by Kubernetes RBAC")
        app.send("\x03")
        app.assert_exit()
        self.assertEqual(self.config.read_bytes(), self.original)

    def test_live_resources_details_logs_commands_and_restart(self):
        app = self.start()
        app.send(":new\rShop\r\r")
        app.expect("Add Connection")
        app.send("\r")
        app.expect("Add Context")
        app.send(" \x1b[B\r")
        app.expect("Added 1 Kubernetes")
        app.send(":pods\r")
        app.expect("Choose Kubernetes connection")
        app.send("\r")
        app.expect("checkout-test")
        app.expect("CrashLoopBackOff")

        app.send("\r")
        app.expect("[d Describe]")
        app.send("y")
        app.expect("Read only")
        app.expect("apiVersion: v1")
        app.send("e")
        app.expect("Back-off restarting failed container")
        app.send("r")
        app.expect("checkout-rs")
        app.send("l")
        app.expect("ERROR missing database setting")
        app.send("p")
        app.wait(lambda: any(path.endswith("/log") and query.get("previous") == ["true"] for _, path, query in self.requests), "previous container logs")

        start = len(app.output)
        app.send("\x1b")
        app.expect("[r Related]", start)
        start = len(app.output)
        app.send("\x1b")
        app.expect("Filter resources", start)
        app.send("\x1b")
        app.wait_json(self.state / "session.json", lambda data: data["focus"] == 0)
        app.send("/deployments\r")
        app.wait(lambda: any(path.endswith("/deployments") for _, path, _ in self.requests), "deployment list")
        app.send("\r")
        app.expect("checkout-deployment")
        app.send("\x1b")
        app.send(":pods\r")
        app.send("/bb cc :quit\r")
        app.expect("No matching Pods.")
        app.expect("Backspace clear")
        app.send("\x1b")
        app.wait_json(self.state / "session.json", lambda data: data["focus"] == 0)
        start = len(app.output)
        app.send(":2\r\x7f")
        app.expect("checkout-test", start)
        app.wait_json(self.state / "session.json", lambda data: not data["explore"][data["workspace"] + "/" + data["connection"]].get("filter"))
        app.send(":c\rKeep this draft")
        start = len(app.output)
        app.send("\x1b")
        app.expect("cc Write message", start)
        app.send(":namespace platform\r")
        app.expect("Access denied by Kubernetes RBAC")
        app.send("\x03")
        app.assert_exit()

        saved = json.loads((self.state / "session.json").read_text())
        key = saved["workspace"] + "/" + saved["connection"]
        self.assertEqual(saved["explore"][key]["namespace"], "platform")
        self.assertIn("Keep this draft", saved["drafts"].values())
        self.assertEqual(self.config.read_bytes(), self.original)
        for path in self.state.rglob("*.json"):
            self.assertNotIn("missing database setting", path.read_text())
            self.assertNotIn("pod-id", path.read_text())

        restarted = self.start()
        restarted.expect_screen("Current Workspace")
        restarted.send("\x1b[1;3D/local-test\r")
        restarted.send(":2\r")
        restarted.expect("Access denied by Kubernetes RBAC")
        restarted.send(":N shop\r")
        restarted.expect("checkout-test")
        restarted.send("\x03")
        restarted.assert_exit()
