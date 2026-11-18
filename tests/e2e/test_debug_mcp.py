"""Standalone MCP subprocess tests using a local Kubernetes API fixture."""

import json
import os
import pty
import select
import shutil
import signal
import subprocess
import tempfile
import threading
import time
import tomllib
import unittest
from datetime import datetime
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlsplit

from support import BINARY, Terminal, debug_skill_body


class StandaloneDebugTests(unittest.TestCase):
    def setUp(self):
        self.storage = tempfile.TemporaryDirectory(prefix="plous-debug-")
        self.root = Path(self.storage.name)
        self.requests = []
        requests = self.requests
        self.read_started = threading.Event()
        self.release_read = threading.Event()
        self.checkout_recovered = threading.Event()
        self.checkout_logs_denied = threading.Event()
        read_started, release_read = self.read_started, self.release_read
        recovered, logs_denied = self.checkout_recovered, self.checkout_logs_denied

        class API(BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass

            def do_GET(self):
                requests.append((self.server.server_port, self.path))
                path = urlsplit(self.path).path
                if path == "/api/v1/namespaces/shop/pods/checkout/log":
                    if logs_denied.is_set():
                        self.send_error(403, "Checkout logs forbidden")
                    elif recovered.is_set():
                        self.reply(b"2026-11-17T18:20:00Z order checkout returned 200 via payments:8080\n", "text/plain")
                    else:
                        self.reply(b"2026-11-17T18:10:00Z dial tcp payments:8081: i/o timeout\n", "text/plain")
                    return
                if self.path == "/api/v1":
                    result = {"kind": "APIResourceList", "groupVersion": "v1", "resources": [
                        {"name": "pods", "kind": "Pod", "namespaced": True, "verbs": ["get", "list"]},
                        {"name": "services", "kind": "Service", "namespaced": True, "verbs": ["get", "list"]},
                    ]}
                elif path in ("/apis", "/apis/example.test"):
                    group = {"name": "example.test", "preferredVersion": {
                        "groupVersion": "example.test/v1", "version": "v1",
                    }, "versions": [
                        {"groupVersion": "example.test/v1", "version": "v1"},
                        {"groupVersion": "example.test/v2", "version": "v2"},
                    ]}
                    result = {"groups": [group]} if path == "/apis" else group
                elif path == "/apis/example.test/v1":
                    result = {"groupVersion": "example.test/v1", "resources": []}
                elif path == "/apis/example.test/v2":
                    result = {"groupVersion": "example.test/v2", "resources": [
                        {"name": "gadgets", "kind": "Gadget", "namespaced": True, "verbs": ["get", "list"]},
                        {"name": "reports", "kind": "Report", "namespaced": False, "verbs": ["get"]},
                    ]}
                elif path.startswith("/apis/example.test/v2/namespaces/shop/gadgets"):
                    gadget = {"apiVersion": "example.test/v2", "kind": "Gadget",
                              "metadata": {"name": "checkout", "namespace": "shop", "uid": "gadget-uid"},
                              "spec": {"minReady": 3, "settings": {"token": "fixture-secret"}}}
                    result = gadget if path.endswith("/checkout") else {"items": [gadget]}
                elif self.path == "/version":
                    result = {"gitVersion": "fixture"}
                elif path == "/apis/apps/v1/namespaces/shop/deployments":
                    result = {"apiVersion": "apps/v1", "kind": "DeploymentList", "items": [{
                        "apiVersion": "apps/v1", "kind": "Deployment",
                        "metadata": {"name": "checkout", "namespace": "shop", "uid": "checkout-deploy", "generation": 2},
                        "spec": {"replicas": 2}, "status": {"observedGeneration": 2, "conditions": [
                            {"type": "Progressing", "status": "False", "reason": "ProgressDeadlineExceeded"},
                            {"type": "Available", "status": "True", "reason": "MinimumReplicasAvailable"},
                        ]},
                    }]}
                elif path == "/apis/apps/v1/namespaces/shop/replicasets":
                    result = {"apiVersion": "apps/v1", "kind": "ReplicaSetList", "items": []}
                elif path == "/api/v1/namespaces/shop/services/payments":
                    result = {"apiVersion": "v1", "kind": "Service",
                              "metadata": {"name": "payments", "namespace": "shop", "uid": "payments-service"},
                              "spec": {"ports": [{"port": 8080, "targetPort": 8080}]}}
                elif self.path.endswith("/pods/blocked"):
                    read_started.set()
                    release_read.wait(10)
                    return
                elif self.path.endswith("/pods/paused"):
                    read_started.set()
                    if not release_read.wait(10):
                        self.send_error(504)
                        return
                    namespace = self.path.split("/namespaces/", 1)[1].split("/", 1)[0]
                    result = {"apiVersion": "v1", "kind": "Pod", "metadata": {
                        "name": "paused", "namespace": namespace, "uid": "paused-uid",
                    }}
                elif "/pods" in self.path:
                    result = {"apiVersion": "v1", "kind": "PodList", "metadata": {"resourceVersion": "1"}, "items": [
                        {"apiVersion": "v1", "kind": "Pod", "metadata": {"name": "checkout", "namespace": self.path.split("/namespaces/", 1)[1].split("/")[0], "uid": "pod-uid"},
                         "spec": {"containers": [{"name": "app", "image": "checkout:fixture"}]}, "status": {"phase": "Running"}}
                    ]}
                else:
                    self.send_error(404)
                    return
                self.reply(json.dumps(result).encode(), "application/json")

            def reply(self, body, content_type):
                self.send_response(200)
                self.send_header("Content-Type", content_type)
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), API)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.other = ThreadingHTTPServer(("127.0.0.1", 0), API)
        self.other_thread = threading.Thread(target=self.other.serve_forever, daemon=True)
        self.other_thread.start()
        self.kubeconfig = self.root / "kubeconfig"
        self.kubeconfig.write_text(f"""apiVersion: v1
kind: Config
clusters:
- name: fixture
  cluster:
    server: http://127.0.0.1:{self.server.server_port}
- name: other
  cluster:
    server: http://127.0.0.1:{self.other.server_port}
contexts:
- name: staging
  context:
    cluster: fixture
    namespace: shop
- name: dev
  context:
    cluster: other
    namespace: apps
current-context: staging
""")
        self.processes = []
        self.next_id = 10

    def tearDown(self):
        self.release_read.set()
        for process in self.processes:
            if process.stdin and not process.stdin.closed:
                process.stdin.close()
            try:
                process.wait(timeout=15)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)
            process.stdout.close()
            process.stderr.close()
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)
        self.other.shutdown()
        self.other.server_close()
        self.other_thread.join(timeout=5)
        self.storage.cleanup()

    def start(self, *options):
        env = dict(os.environ, KUBECONFIG=str(self.kubeconfig))
        env.pop("PLOUS_AGENT_CONFIG", None)
        process = subprocess.Popen([
            str(BINARY), "mcp", "serve",
            "--data-dir", str(self.root / "evidence"), *options,
        ], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
            env=env)
        self.processes.append(process)
        return process

    def send(self, process, identity, method, params=None):
        request = {"jsonrpc": "2.0", "method": method}
        if identity is not None:
            request["id"] = identity
        if params is not None:
            request["params"] = params
        process.stdin.write(json.dumps(request) + "\n")
        process.stdin.flush()

    def response(self, process, method):
        self.assertTrue(select.select([process.stdout], [], [], 10)[0], f"No response to {method}")
        line = process.stdout.readline()
        self.assertTrue(line, f"Server stopped during {method}")
        return json.loads(line)

    def rpc(self, process, identity, method, params=None):
        self.send(process, identity, method, params)
        response = self.response(process, method)
        self.assertEqual(response["id"], identity)
        self.assertNotIn("error", response)
        return response["result"]

    def test_monitor_cli_tui_and_scoped_agent_reads(self):
        def monitor_command(action, name=None, *args):
            names = [name] if name is not None else []
            return [str(BINARY), "monitor", action, *names, "--data-dir", str(self.root / "evidence"), *args]

        def start_monitor(name, *args):
            process = subprocess.Popen(monitor_command("start", name, *args),
                                       stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            workers.append(process)
            return process

        def status(name, expected):
            heartbeat = time.monotonic() + 8
            while time.monotonic() < heartbeat:
                result = subprocess.run(monitor_command("status", name), capture_output=True, text=True, check=True)
                if expected in result.stdout:
                    return result.stdout
                time.sleep(0.05)
            self.fail(f"Monitor {name} did not report {expected}")

        workers = []
        try:
            args = ["--kubeconfig", str(self.kubeconfig), "--namespace", "shop", "--deployment", "checkout"]
            staging = start_monitor("payments", "--context", "staging", *args)
            other_monitor = start_monitor("other-ops", "--context", "dev", *args)
            first_status = status("payments", "stalled rollout")
            self.assertIn("present at start", first_status)
            self.assertIn("Monitor payments (running): staging / shop", first_status)
            self.assertIn("Next check:", first_status)
            self.assertIn("Monitor other-ops (running): dev / shop", status("other-ops", "stalled rollout"))
            self.assertTrue((self.root / "evidence" / "monitor" / "payments" / "watch.json").is_file())
            listed = subprocess.run(monitor_command("list"), capture_output=True, text=True, check=True).stdout
            self.assertIn("payments (running)", listed)
            self.assertIn("other-ops (running)", listed)
            duplicate = subprocess.run(monitor_command("start", "payments"), capture_output=True, text=True)
            self.assertNotEqual(duplicate.returncode, 0)
            self.assertIn('monitor "payments" is already running', duplicate.stderr)

            process = self.start()
            target = self.tool(process, "kubernetes_context_read", {})
            alert_args = {"operation": "list", "purpose": "Read monitor alert", "monitor_name": "payments",
                          "connection_id": target["connections"][0]["connection_id"]}
            before = list(self.requests)
            alerts = self.tool(process, "monitor_read", alert_args)
            [alert] = alerts["observation"]["rows"]
            self.assertEqual(alert["facts"]["detail"]["text"], "ProgressDeadlineExceeded")
            self.assertIn("record:", alerts["ref"])
            alert_id = alert["facts"]["alert_id"]["text"]
            read = self.tool(process, "monitor_read", dict(alert_args, operation="get", alert_id=alert_id))
            self.assertEqual(read["observation"]["rows"], [alert])
            self.assertEqual(self.requests, before)
            available = self.tool(process, "monitor_read",
                                  {key: value for key, value in dict(alert_args, operation="status").items()
                                   if key != "monitor_name"})
            self.assertIn("payments", json.dumps(available))
            self.assertNotIn("other-ops", json.dumps(available))
            [monitor] = available["observation"]["rows"]
            facts = monitor["facts"]
            self.assertFalse(facts["overdue"]["boolean"])
            self.assertGreater(datetime.fromisoformat(facts["next_check"]["text"]),
                               datetime.fromisoformat(facts["last_check"]["text"]))
            self.assertEqual(available["observation"]["facts"]["next_check"], facts["next_check"])
            self.assertEqual(self.requests, before)

            self.kubeconfig.write_text(self.kubeconfig.read_text().replace("current-context: staging", "current-context: dev"))
            other = self.tool(process, "kubernetes_context_read", {})
            alert_args["connection_id"] = other["connections"][0]["connection_id"]
            hidden = self.tool(process, "monitor_read", alert_args)
            self.assertIn("no saved monitor in the captured scope", json.dumps(hidden))
            self.assertNotIn("checkout", json.dumps(hidden))
            self.assertEqual(self.requests, before)

            data_dir = self.root / "evidence"
            app = Terminal(data_dir, self.kubeconfig, width=180, height=40)
            try:
                app.expect_screen("AI assistant · dev")
                app.send(":4\r")
                app.expect_screen("2 open alerts")
                app.expect_screen("staging/shop")
                app.expect_screen("dev/shop")
                app.expect_screen("Last complete check")
                app.expect_screen("Next check")
                app.capture_screen("running-monitors-180x40")
                app.resize(80, 16)
                app.expect_screen("other-ops")
                app.expect_screen("payments")
                app.expect_screen("dev/shop")
                app.expect_screen("Every 5m · 1 open")
                app.expect_screen("Next check")
                app.capture_screen("running-monitors-80x16")
                app.resize(180, 40)
                app.send("\x1b[B\r")
                app.expect_screen("payments · staging/shop")
                app.send("\r")
                app.expect_screen("ProgressDeadlineExceeded")
                app.send(":quit\r")
                app.assert_exit()
            finally:
                app.close()
            self.assertIsNone(staging.poll())
            self.assertIn("payments (running)", status("payments", "running"))

            app = Terminal(data_dir, self.kubeconfig, width=180, height=40)
            try:
                app.expect_screen("AI assistant · dev")
                app.send(":4\r")
                app.expect_screen("staging/shop")
                subprocess.run(monitor_command("stop", "payments"), capture_output=True, text=True, check=True)
                stdout, stderr = staging.communicate(timeout=5)
                self.assertEqual(staging.returncode, 0, stderr)
                self.assertIn("Monitor payments stopped.", stdout)
                self.assertIsNone(other_monitor.poll())
                self.assertIn("Monitor payments (stopped)", status("payments", "stopped"))
                app.wait(lambda: "staging/shop" not in "\n".join(app.screen()["rows"]),
                         "stopped monitor to leave the running list")
                app.expect_screen("dev/shop")
                app.send(":j\r/payments\r")
                app.send("\r")
                app.expect_screen("ProgressDeadlineExceeded")
                app.expect_screen("payments")
                app.send(":quit\r")
                app.assert_exit()
            finally:
                app.close()

            restarted = start_monitor("payments")
            self.assertIn("Monitor payments (running): staging / shop", status("payments", "running"))
            subprocess.run(monitor_command("stop", "payments"), capture_output=True, text=True, check=True)
            stdout, stderr = restarted.communicate(timeout=5)
            self.assertEqual(restarted.returncode, 0, stderr)

            retrying = start_monitor("retrying", "--context", "dev", "--kubeconfig", str(self.kubeconfig),
                                     "--namespace", "apps", "--every", "1m")
            status("retrying", "Coverage / Deployment:")
            snapshot = json.loads((data_dir / "monitor" / "retrying" / "watch.json").read_bytes())
            delay = (datetime.fromisoformat(snapshot["NextCheck"]) - datetime.fromisoformat(snapshot["LastCheck"])).total_seconds()
            self.assertGreater(delay, 60, "the recorded check must include retry backoff")
            self.assertLessEqual(delay, 120)
            subprocess.run(monitor_command("stop", "retrying"), capture_output=True, text=True, check=True, timeout=7)
            _, stderr = retrying.communicate(timeout=5)
            self.assertEqual(retrying.returncode, 0, stderr)
        finally:
            for worker in workers:
                if worker.poll() is None:
                    worker.terminate()
                worker.communicate(timeout=5)

    def test_monitor_confirms_replacement_and_removal(self):
        root = self.root / "monitors"
        record = root / "monitor" / "payments" / "watch.json"
        workers = []
        terminals = []

        def command(action, *args):
            return [str(BINARY), "monitor", action, "payments", "--data-dir", str(root), *args]

        def launch(action, *args, confirm=False):
            terminal = None
            stdin = subprocess.DEVNULL
            if confirm:
                terminal, stdin = pty.openpty()
                terminals.append(terminal)
            try:
                process = subprocess.Popen(command(action, *args), stdin=stdin,
                                           stdout=subprocess.PIPE, stderr=subprocess.PIPE)
                workers.append(process)
            finally:
                if confirm:
                    os.close(stdin)
            if not confirm:
                return process
            prompt = bytearray()
            deadline = time.monotonic() + 5
            while b"[y/N]" not in prompt:
                self.assertTrue(select.select([process.stderr], [], [], max(0, deadline - time.monotonic()))[0],
                                f"Expected a confirmation prompt, got {prompt!r}")
                chunk = os.read(process.stderr.fileno(), 4096)
                self.assertTrue(chunk, f"Process exited without a confirmation prompt: {prompt!r}")
                prompt.extend(chunk)
            return process, terminal, prompt.decode()

        def wait_for_check(context, interval):
            deadline = time.monotonic() + 5
            while time.monotonic() < deadline:
                if record.exists():
                    state = json.loads(record.read_bytes())
                    if (state["Running"] and state["Context"] == context and state["Interval"] == interval
                            and not state["LastCheck"].startswith("0001-")):
                        return state["Incidents"][0]["ID"]
                time.sleep(0.05)
            self.fail(f"No check for {context} with interval {interval}")

        def stop(process):
            subprocess.run(command("stop"), capture_output=True, check=True, timeout=5)
            stdout, stderr = process.communicate(timeout=5)
            self.assertEqual(process.returncode, 0, stderr)
            self.assertIn(b"Monitor payments stopped.", stdout)

        common = ["--kubeconfig", str(self.kubeconfig), "--namespace", "shop", "--deployment", "checkout"]
        staging = [*common, "--context", "staging"]
        dev = [*common, "--context", "dev"]
        try:
            original = launch("start", *staging)
            old_alert = wait_for_check("staging", 300_000_000_000)
            for args in [command("remove"), command("start", *dev, "--replace")]:
                result = subprocess.run(args, capture_output=True, timeout=5)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(b"already running", result.stderr)
            stop(original)

            saved = record.read_bytes()
            for action, args in [("start", dev), ("remove", [])]:
                result = subprocess.run(command(action, *args), input=b"yes\n", capture_output=True, timeout=5)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(b"in a terminal", result.stderr)
                self.assertEqual(record.read_bytes(), saved)

            replacement, terminal, prompt = launch("start", *dev, confirm=True)
            self.assertIn("staging / shop / Deployment/checkout", prompt)
            self.assertIn("dev / shop / Deployment/checkout", prompt)
            self.assertIn("Replace payments and clear its saved alerts?", prompt)
            competing = subprocess.run(command("start", *dev, "--replace"), capture_output=True, timeout=5)
            self.assertNotEqual(competing.returncode, 0)
            self.assertIn(b"being changed", competing.stderr)
            os.write(terminal, b"\n")
            stdout, stderr = replacement.communicate(timeout=5)
            self.assertEqual(replacement.returncode, 0, stderr)
            self.assertIn(b"Canceled.", stdout)
            self.assertEqual(record.read_bytes(), saved)

            resumed = launch("start", "--every", "10m")
            self.assertEqual(wait_for_check("staging", 600_000_000_000), old_alert)
            stop(resumed)

            replacement, terminal, _ = launch("start", *dev, confirm=True)
            os.write(terminal, b"yes\n")
            dev_alert = wait_for_check("dev", 300_000_000_000)
            self.assertNotEqual(dev_alert, old_alert)
            stop(replacement)

            replacement = launch("start", *staging, "--replace")
            self.assertNotIn(wait_for_check("staging", 300_000_000_000), [old_alert, dev_alert])
            stopped = subprocess.run(command("stop"), capture_output=True, check=True, timeout=7)
            self.assertIn(b"Monitor payments stopped.", stopped.stdout)

            saved = record.read_bytes()
            removal, _, prompt = launch("remove", confirm=True)
            self.assertIn("Delete payments and its saved alerts?", prompt)
            removal.send_signal(signal.SIGINT)
            removal.communicate(timeout=5)
            self.assertEqual(removal.returncode, 130)
            self.assertEqual(record.read_bytes(), saved)

            removal, terminal, _ = launch("remove", confirm=True)
            os.write(terminal, b"y\n")
            stdout, stderr = removal.communicate(timeout=5)
            self.assertEqual(removal.returncode, 0, stderr)
            self.assertIn(b"Monitor payments removed.", stdout)
            self.assertFalse(record.exists())
            _, stderr = replacement.communicate(timeout=5)
            self.assertEqual(replacement.returncode, 0, stderr)
            status = subprocess.run(command("status"), capture_output=True, check=True, timeout=5)
            self.assertIn(b"No saved monitor named payments.", status.stdout)
            reused = launch("start", *dev)
            self.assertNotEqual(wait_for_check("dev", 300_000_000_000), dev_alert)
            stop(reused)
        finally:
            for worker in workers:
                if worker.poll() is None:
                    worker.terminate()
                try:
                    worker.communicate(timeout=5)
                except subprocess.TimeoutExpired:
                    worker.kill()
                    worker.communicate(timeout=5)
            for terminal in terminals:
                os.close(terminal)

    def test_cancelled_read_saves_evidence_and_leaves_session_usable(self):
        process = self.start()
        target = self.tool(process, "kubernetes_context_read", {})
        args = {"purpose": "Read fixture Pods", "connection_id": target["connections"][0]["connection_id"],
                "operation": "list", "resource": "pods"}
        self.send(process, 1, "tools/call", {
            "name": "kubernetes_read", "arguments": dict(args, operation="get", name="blocked"),
        })
        self.assertTrue(self.read_started.wait(5), "The fixture read never started")
        self.send(process, None, "notifications/cancelled", {"requestId": 1})
        response = self.response(process, "cancelled tools/call")
        self.assertEqual((response["id"], response["error"]["code"]), (1, -32800))
        self.release_read.set()
        self.assertIn("record:", self.tool(process, "kubernetes_read", args)["ref"])
        evidence = "\n".join(path.read_text() for path in (self.root / "evidence").rglob("reasoning/*.jsonl"))
        self.assertIn("The read was cancelled.", evidence)

    def test_oversized_frame_does_not_close_the_transport(self):
        process = self.start()
        process.stdin.write("x" * (2 << 20) + "\n")
        process.stdin.flush()
        response = self.response(process, "oversized frame")
        self.assertIn("exceeds the size limit", response["error"]["message"])
        self.assertEqual(self.rpc(process, 1, "ping"), {})

    def test_sigterm_exits_with_stdin_open(self):
        process = self.start()
        self.assertEqual(self.rpc(process, 1, "ping"), {})
        process.terminate()
        self.assertEqual(process.wait(timeout=5), 0)

    def test_shutdown_stops_a_running_kubeconfig_credential_helper(self):
        for shutdown in ("signal", "disconnect"):
            with self.subTest(shutdown=shutdown):
                marker = self.root / f"helper-{shutdown}.pid"
                helper = self.root / "credential-helper"
                helper.write_text('#!/bin/sh\nprintf "%s\\n" "$$" > "$1"\nsleep 30\n')
                helper.chmod(0o700)
                config = self.root / f"kubeconfig-{shutdown}"
                config.write_text(f"""apiVersion: v1
kind: Config
clusters:
- name: fixture
  cluster:
    server: https://127.0.0.1:1
    insecure-skip-tls-verify: true
contexts:
- name: fixture
  context:
    cluster: fixture
    user: fixture
current-context: fixture
users:
- name: fixture
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1
      command: {helper}
      args: [{marker}]
      interactiveMode: Never
""")
                process = self.start("--kubeconfig", str(config))
                target = self.tool(process, "kubernetes_context_read", {})
                self.send(process, 1, "tools/call", {"name": "kubernetes_read", "arguments": {
                    "purpose": "Check credential helper shutdown", "connection_id": target["connections"][0]["connection_id"],
                    "operation": "list", "resource": "pods",
                }})
                deadline = time.monotonic() + 5
                while not marker.exists() and time.monotonic() < deadline:
                    time.sleep(0.02)
                self.assertTrue(marker.exists(), "credential helper did not start")
                if shutdown == "signal":
                    process.terminate()
                else:
                    process.stdin.close()
                self.assertEqual(process.wait(timeout=5), 0)
                with self.assertRaises(ProcessLookupError):
                    os.kill(int(marker.read_text().strip()), 0)

    def test_failed_first_read_keeps_transport_and_reports_safe_configuration_error(self):
        process = self.start()
        target = self.tool(process, "kubernetes_context_read", {})
        profile, = (self.root / "evidence").glob("session-*")
        config = profile / "agent" / "config.yaml"
        config.parent.mkdir(exist_ok=True)
        config.write_text("max_calls: fixture-secret-must-not-appear\n")
        params = {"name": "kubernetes_read", "arguments": {
            "purpose": "Inspect fixture Pods", "connection_id": target["connections"][0]["connection_id"],
            "operation": "list", "resource": "pods",
        }}
        for identity in (1, 2):
            self.send(process, identity, "tools/call", params)
            response = self.response(process, "failed tool request")
            self.assertEqual(response["id"], identity)
            self.assertEqual(response["error"]["code"], -32000)
            self.assertNotIn("fixture-secret-must-not-appear", json.dumps(response))
        self.assertEqual(self.rpc(process, 3, "ping"), {})
        process.stdin.close()
        self.assertEqual(process.wait(timeout=5), 1)
        stderr = process.stderr.read()
        self.assertIn(f'agent configuration "{config}"', stderr)
        self.assertIn("supported settings", stderr)
        self.assertNotIn("fixture-secret-must-not-appear", stderr)
        self.assertEqual(self.requests, [])

    def test_closed_stdout_cancels_read_and_saves_evidence(self):
        process = self.start()
        target = self.tool(process, "kubernetes_context_read", {})
        self.send(process, 1, "tools/call", {"name": "kubernetes_read", "arguments": {
            "purpose": "Read blocked fixture Pod", "connection_id": target["connections"][0]["connection_id"],
            "operation": "get", "resource": "pods", "name": "blocked",
        }})
        self.assertTrue(self.read_started.wait(5), "The fixture read never started")
        process.stdout.close()
        self.send(process, 2, "ping")
        self.assertEqual(process.wait(timeout=5), 1)
        self.assertIn("write MCP response:", process.stderr.read())
        self.release_read.set()
        evidence = "\n".join(path.read_text() for path in (self.root / "evidence").rglob("reasoning/*.jsonl"))
        self.assertIn("The read was cancelled.", evidence)

    def test_shared_reads_capabilities_and_independent_profiles(self):
        readonly = self.start()
        active = self.start("--allow-exec", "--diagnostic-image", "fixture-image")
        init = self.rpc(readonly, 1, "initialize")
        self.assertIn("## Reproduce", init["instructions"])
        reads = {tool["name"]: tool for tool in self.rpc(readonly, 2, "tools/list")["tools"]}
        tools = {tool["name"]: tool for tool in self.rpc(active, 2, "tools/list")["tools"]}
        self.assertNotIn("kubernetes_run", reads)
        self.assertNotIn("plous_view", reads)
        self.assertIn("kubernetes_context_read", reads)
        self.assertEqual(reads["kubernetes_context_select"]["annotations"],
                         {"readOnlyHint": False, "destructiveHint": False})
        self.assertIn("kubernetes_run", tools)
        self.assertFalse(tools["kubernetes_run"]["annotations"]["readOnlyHint"])
        target = self.tool(readonly, "kubernetes_context_read", {})
        connection = target["connections"][0]["connection_id"]
        self.assertEqual(target["context"], "staging")
        result = self.rpc(readonly, 3, "tools/call", {"name": "kubernetes_read", "arguments": {
            "purpose": "inspect checkout", "connection_id": connection, "operation": "list", "resource": "pods", "namespace": "shop",
        }})
        evidence = json.loads(result["content"][0]["text"])
        self.assertIn("record:", evidence["ref"])
        self.assertIn("checkout", json.dumps(evidence))
        before = list(self.requests)
        result = self.rpc(active, 3, "tools/call", {"name": "kubernetes_run", "arguments": {"operation": "exec",
            "purpose": "check namespace boundary", "connection_id": connection, "namespace": "other",
            "name": "checkout", "uid": "pod-uid", "container": "app", "command": ["true"],
        }})
        self.assertIn("denied", result["content"][0]["text"])
        self.assertEqual(self.requests, before)
        for process in (readonly, active):
            process.stdin.close()
            self.assertEqual(process.wait(timeout=15), 0, process.stderr.read())
        profiles = list((self.root / "evidence").glob("session-*"))
        self.assertEqual(len(profiles), 2)
        execution = "\n".join(path.read_text() for path in (self.root / "evidence").rglob("execution/*.jsonl"))
        self.assertIn("diagnostic-request", execution)

    def tool(self, process, name, arguments):
        result = self.rpc(process, self.next_id, "tools/call", {"name": name, "arguments": arguments})
        self.next_id += 1
        return json.loads(result["content"][0]["text"])

    def test_discover_list_and_read_fields_of_a_custom_resource(self):
        process = self.start()
        target = self.tool(process, "kubernetes_context_read", {})
        scope = {"connection_id": target["connections"][0]["connection_id"], "purpose": "Inspect the custom API"}

        discovered = self.tool(process, "kubernetes_read", dict(scope, operation="discover", query="Gadget"))
        [kind] = discovered["observation"]["rows"]
        self.assertEqual(kind["identity"]["name"], "gadgets.example.test")
        self.assertEqual(kind["facts"]["api_version"]["text"], "v2")
        self.assertTrue(discovered["observation"]["coverage"]["complete"])
        get_only = self.tool(process, "kubernetes_read", dict(scope, operation="discover", query="Report"))
        self.assertFalse(get_only["observation"]["rows"][0]["facts"]["list"]["boolean"])

        listed = self.tool(process, "kubernetes_read", dict(scope, operation="list", resource=kind["identity"]["name"]))
        [obj] = listed["observation"]["rows"]
        self.assertEqual(obj["identity"]["name"], "checkout")

        selection = ["/spec/minReady", "/spec/settings/token", "/spec/missing"]
        read = self.tool(process, "kubernetes_read", dict(
            scope, operation="get", resource=kind["identity"]["name"],
            api_version=kind["facts"]["api_version"]["text"], name=obj["identity"]["name"], paths=selection))
        [row] = read["observation"]["rows"]
        self.assertEqual(row["facts"]["/spec/minReady"]["number"], 3)
        self.assertEqual(row["facts"]["path_status./spec/settings/token"]["text"], "redacted")
        self.assertEqual(row["facts"]["path_status./spec/missing"]["text"], "missing")
        self.assertNotIn("fixture-secret", json.dumps(read))

        saved = self.tool(process, "history_read", {"operation": "read", "ref": read["ref"], "purpose": "Read saved fields"})
        self.assertIn('"/spec/minReady": {"number": 3}', json.dumps(saved))

    def test_diagnosis_evidence_can_be_reopened_and_compared_after_deployment(self):
        process = self.start()
        target = self.tool(process, "kubernetes_context_read", {})
        self.assertEqual((target["context"], target["default_namespace"]), ("staging", "shop"))
        cluster = {"connection_id": target["connections"][0]["connection_id"], "namespace": "shop"}
        log_args = dict(cluster, operation="logs", purpose="Check the checkout failure", resource="pods",
                        name="checkout", container="app")
        before = self.tool(process, "kubernetes_read", log_args)
        service = self.tool(process, "kubernetes_read", dict(
            cluster, purpose="Check the payments Service port", operation="get", resource="services", name="payments"))
        self.assertIn("payments:8081: i/o timeout", before["observation"]["facts"]["excerpt"]["text"])
        self.assertEqual(service["observation"]["rows"][0]["facts"]["/spec/ports/0/port"]["number"], 8080)

        self.checkout_logs_denied.set()
        denied = self.tool(process, "kubernetes_read", log_args)["observation"]
        self.assertEqual(denied["outcome"], "denied")
        self.assertFalse(denied["coverage"]["complete"])
        self.assertNotIn("excerpt", denied["facts"])
        self.assertIn("RBAC", denied["facts"]["error"]["text"])
        self.checkout_logs_denied.clear()

        self.checkout_recovered.set()
        after = self.tool(process, "kubernetes_read", dict(log_args, purpose="Verify the operator-deployed change"))
        self.assertIn("returned 200 via payments:8080", after["observation"]["facts"]["excerpt"]["text"])
        self.assertNotEqual(after["ref"], before["ref"])
        self.assertGreater(datetime.fromisoformat(after["observation"]["observed_at"]),
                           datetime.fromisoformat(before["observation"]["observed_at"]))

        requests = list(self.requests)
        for delivery in (before, service):
            saved = self.tool(process, "history_read", {
                "purpose": "Show the source from the diagnosis", "operation": "read", "ref": delivery["ref"],
            })["evidence"][0]
            self.assertEqual(saved["ref"], delivery["ref"])
            self.assertEqual(saved["observation"], delivery["observation"])
        self.assertEqual(self.requests, requests)

    def test_setup_registrations_launch_the_shared_stdio_server(self):
        skill_body = debug_skill_body()
        instructions = set()
        for client, config_path, server_keys in (
            ("codex", ".codex/config.toml", ("mcp_servers", "plous_debug")),
            ("claude", ".claude.json", ("mcpServers", "plous_debug")),
            ("opencode", ".config/opencode/opencode.json", ("mcp", "plous_debug")),
            ("grok", ".grok/config.toml", ("mcp_servers", "plous_debug")),
            ("agy", ".gemini/config/mcp_config.json", ("mcpServers", "plous_debug")),
            ("cursor", ".cursor/mcp.json", ("mcpServers", "plous_debug")),
            ("gemini", ".gemini/settings.json", ("mcpServers", "plous_debug")),
        ):
            with self.subTest(client=client):
                home = self.root / client
                setup = subprocess.run([
                    str(BINARY), "setup", client, "--home", str(home),
                    "--data-dir", str(self.root / "evidence"),
                ], check=True, capture_output=True, text=True)
                self.assertIn("MCP server plous_debug:", setup.stdout)
                self.assertIn("current kubecontext", setup.stdout)
                self.assertIn(f"/docs/agent-skills/clients/{client}/", setup.stdout)
                path = home / config_path
                config = path.read_text()
                server = tomllib.loads(config) if path.suffix == ".toml" else json.loads(config)
                for key in server_keys:
                    server = server[key]
                command = server["command"]
                if isinstance(command, str):
                    command = [command, *server["args"]]
                self.assertTrue({"--kubeconfig", "--context", "--namespace"}.isdisjoint(command))
                process = subprocess.Popen(command, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                           stderr=subprocess.PIPE, text=True,
                                           env=dict(os.environ, KUBECONFIG=str(self.kubeconfig)))
                self.processes.append(process)
                initialized = self.rpc(process, 1, "initialize")
                self.assertTrue(initialized["instructions"].strip().endswith(skill_body))
                instructions.add(initialized["instructions"])
                target = self.tool(process, "kubernetes_context_read", {})
                self.assertEqual((target["context"], target["default_namespace"]), ("staging", "shop"))
                self.assertEqual(target["kubeconfig_source"], "KUBECONFIG")
                evidence = self.tool(process, "kubernetes_read", {
                    "purpose": "inspect checkout", "connection_id": target["connections"][0]["connection_id"],
                    "operation": "list", "resource": "pods",
                })
                self.assertIn("record:", evidence["ref"])
                self.assertIn("checkout", json.dumps(evidence))
                selection = self.tool(process, "kubernetes_context_select", {"context": "dev"})
                switched = subprocess.run([str(BINARY), "mcp", "target", selection["local_target_control"], "", "dev"],
                                          check=True, capture_output=True, text=True)
                self.assertIn('Context: "dev"', switched.stdout)
                target = self.tool(process, "kubernetes_context_read", {})
                self.assertEqual((target["context"], target["default_namespace"]), ("dev", "apps"))
                process.stdin.close()
                self.assertEqual(process.wait(timeout=15), 0, process.stderr.read())
        self.assertEqual(len(instructions), 1, "MCP instructions differ by client")
        backends = set()
        for path in (self.root / "evidence").rglob("execution/*.jsonl"):
            for line in path.read_text().splitlines():
                entry = json.loads(line)
                if entry.get("request", {}).get("role") == "external-tools":
                    backends.add(entry["request"]["backend"])
        self.assertEqual(backends, {"codex", "claude-code", "opencode", "grok", "agy", "cursor", "gemini"})

    def test_setup_keeps_the_executable_path_across_upgrades(self):
        stable = self.root / "bin" / "plous"
        first = self.root / "Cellar" / "1" / "bin" / "plous"
        second = self.root / "Cellar" / "2" / "bin" / "plous"
        for binary in (stable, first, second):
            binary.parent.mkdir(parents=True)
        shutil.copy2(BINARY, first)
        stable.symlink_to(first)
        home = self.root / "agent"
        home.mkdir()
        env = dict(os.environ, PATH=str(stable.parent) + os.pathsep + os.environ.get("PATH", ""))
        config = home / ".gemini" / "settings.json"

        for executable in (stable, first):
            with self.subTest(executable=executable):
                subprocess.run([
                    str(executable), "setup", "gemini", "--home", str(home),
                    "--data-dir", str(self.root / "evidence"),
                ], check=True, capture_output=True, text=True, env=env)
                command = json.loads(config.read_text())["mcpServers"]["plous_debug"]["command"]
                self.assertTrue(Path(command).is_symlink(), command)
                self.assertTrue(Path(command).samefile(stable))

        first.rename(second)
        stable.unlink()
        stable.symlink_to(second)
        command = json.loads(config.read_text())["mcpServers"]["plous_debug"]["command"]
        subprocess.run([command, "--version"], check=True, capture_output=True, text=True, env=env)

    def test_live_context_switch_and_pinned_override(self):
        config = self.kubeconfig.read_text()
        live = self.start()
        pinned = self.start("--context", "staging")
        first = self.tool(live, "kubernetes_context_read", {})
        self.assertEqual(first["kubeconfig_source"], "KUBECONFIG")
        pinned_first = self.tool(pinned, "kubernetes_context_read", {})
        self.assertEqual(first, pinned_first)
        args = {
            "purpose": "inspect Pods", "connection_id": first["connections"][0]["connection_id"],
            "operation": "list", "resource": "pods",
        }
        old = self.tool(live, "kubernetes_read", args)
        self.assertIn("record:", old["ref"])
        self.assertTrue(any(port == self.server.server_port and "/namespaces/shop/pods" in path
                            for port, path in self.requests))
        self.kubeconfig.write_text(config.replace("current-context: staging", "current-context: ''"))
        self.assertIn("No current context", self.tool(live, "kubernetes_context_read", {})["error"])
        self.kubeconfig.write_text(config.replace("current-context: staging", "current-context: dev"))
        current = self.tool(live, "kubernetes_context_read", {})
        self.assertEqual((current["context"], current["default_namespace"]), ("dev", "apps"))
        self.assertEqual(self.tool(pinned, "kubernetes_context_read", {}), pinned_first)
        before = list(self.requests)
        stale = self.tool(live, "kubernetes_read", args)
        self.assertIn("denied", json.dumps(stale))
        self.assertEqual(self.requests, before)
        args["connection_id"] = current["connections"][0]["connection_id"]
        new = self.tool(live, "kubernetes_read", args)
        self.assertIn("record:", new["ref"])
        self.assertTrue(any(port == self.other.server_port and "/namespaces/apps/pods" in path
                            for port, path in self.requests))
        self.assertEqual(self.kubeconfig.read_text(), config.replace("current-context: staging", "current-context: dev"))
        results = self.tool(live, "history_read", {
            "purpose": "find saved Pods in dev", "operation": "search", "query": "checkout",
        })
        self.assertNotIn(old["ref"], json.dumps(results))
        history = {"purpose": "review staging evidence", "operation": "read", "ref": old["ref"]}
        self.kubeconfig.write_text(config)
        saved = self.tool(live, "history_read", history)
        self.assertIn("checkout", json.dumps(saved))

    def test_context_errors_report_source_and_recover_on_the_same_connection(self):
        missing = self.root / "missing.yaml"
        process = self.start("--kubeconfig", str(missing), "--context", "develop")
        result = self.tool(process, "kubernetes_context_read", {})
        self.assertEqual(result["kubeconfig_source"], "--kubeconfig")
        self.assertIn("No kubeconfig files found", result["error"])
        self.assertIn(str(missing), result["error"])

        config = self.kubeconfig.read_text()
        missing.write_text(config)
        result = self.tool(process, "kubernetes_context_read", {})
        self.assertIn('Context "develop" was not found', result["error"])
        missing.write_text(config.replace("- name: dev\n", "- name: develop\n"))
        target = self.tool(process, "kubernetes_context_read", {})
        self.assertEqual((target["context"], target["default_namespace"]), ("develop", "apps"))
        self.assertEqual(self.requests, [])

    def test_runtime_target_selection_requires_confirmation_and_stays_local(self):
        process = self.start()
        self.rpc(process, 1, "initialize", {
            "protocolVersion": "2025-11-25", "capabilities": {"elicitation": {"form": {}}},
        })
        self.send(process, None, "notifications/initialized")
        original = self.tool(process, "kubernetes_context_read", {})
        alternate = self.root / "alternate.yaml"
        alternate.write_text(self.kubeconfig.read_text().replace("current-context: staging", "current-context: dev"))

        def choose(identity, arguments, approved):
            self.send(process, identity, "tools/call", {
                "name": "kubernetes_context_select", "arguments": arguments,
            })
            prompt = self.response(process, "elicitation/create")
            self.assertEqual(prompt["method"], "elicitation/create")
            self.assertFalse(select.select([process.stdout], [], [], 0)[0])
            answer = {"action": "accept", "content": {"confirm": True}} if approved else {"action": "decline"}
            self.reply_to_prompt(process, prompt, answer)
            response = self.response(process, "kubernetes_context_select")
            self.assertEqual(response["id"], identity)
            return json.loads(response["result"]["content"][0]["text"])

        self.assertIn("declined", choose(2, {"kubeconfig": str(alternate)}, False)["error"])
        self.assertEqual(self.tool(process, "kubernetes_context_read", {}), original)
        self.assertIn("No kubeconfig", choose(3, {"kubeconfig": str(self.root / "missing")}, True)["error"])
        self.assertEqual(self.tool(process, "kubernetes_context_read", {}), original)
        self.assertEqual(choose(4, {"kubeconfig": str(alternate)}, True)["context"], "dev")
        old_connection = original["connections"][0]["connection_id"]
        denied = self.tool(process, "kubernetes_read", {
            "purpose": "check the old scope", "connection_id": old_connection, "operation": "list", "resource": "pods",
        })
        self.assertIn("denied", json.dumps(denied))
        alternate.write_text(self.kubeconfig.read_text())
        self.assertEqual(self.tool(process, "kubernetes_context_read", {})["context"], "staging")
        self.assertEqual(choose(5, {"kubeconfig": str(alternate), "context": "dev"}, True)["context"], "dev")
        self.assertEqual(self.tool(process, "kubernetes_context_read", {})["context"], "dev")
        other = self.start()
        self.assertEqual(self.tool(other, "kubernetes_context_read", {}), original)
        self.assertEqual(choose(6, {}, True), original)
        self.assertEqual(self.requests, [])

    def test_client_without_confirmation_uses_local_target_command(self):
        process = self.start()
        original = self.tool(process, "kubernetes_context_read", {})
        response = self.tool(process, "kubernetes_context_select", {"context": "dev"})
        control = Path(response["local_target_control"])
        self.assertEqual(self.tool(process, "kubernetes_context_read", {}), original)
        alternate = self.root / "alternate.yaml"
        alternate.write_text(self.kubeconfig.read_text().replace("current-context: staging", "current-context: dev"))
        result = subprocess.run([str(BINARY), "mcp", "target", str(control), str(alternate)],
                                check=True, capture_output=True, text=True)
        self.assertIn('Context: "dev"', result.stdout)
        self.assertEqual(self.tool(process, "kubernetes_context_read", {})["context"], "dev")
        subprocess.run([str(BINARY), "mcp", "target", str(control)], check=True, capture_output=True)
        self.assertEqual(self.tool(process, "kubernetes_context_read", {}), original)
        process.stdin.close()
        self.assertEqual(process.wait(timeout=15), 0, process.stderr.read())
        self.assertFalse(control.exists())

    def test_cancelled_target_selection_ignores_late_approval(self):
        process = self.start()
        self.rpc(process, 1, "initialize", {"protocolVersion": "2025-06-18", "capabilities": {"elicitation": {}}})
        self.send(process, None, "notifications/initialized")
        self.send(process, 2, "tools/call", {"name": "kubernetes_context_select", "arguments": {"context": "dev"}})
        cancelled_prompt = self.response(process, "elicitation/create")
        self.send(process, None, "notifications/cancelled", {"requestId": 2})
        cancelled = self.response(process, "cancelled selection")
        self.assertEqual((cancelled["id"], cancelled["error"]["code"]), (2, -32800))

        self.send(process, 3, "tools/call", {"name": "kubernetes_context_select", "arguments": {"context": "dev"}})
        prompt = self.response(process, "elicitation/create")
        self.assertNotEqual(prompt["id"], cancelled_prompt["id"])
        approved = {"action": "accept", "content": {"confirm": True}}
        self.reply_to_prompt(process, cancelled_prompt, approved)
        self.assertEqual(self.tool(process, "kubernetes_context_read", {})["context"], "staging")
        self.reply_to_prompt(process, prompt, approved)
        result = self.response(process, "confirmed selection")
        self.assertEqual(result["id"], 3)
        self.assertEqual(self.tool(process, "kubernetes_context_read", {})["context"], "dev")
        self.assertEqual(self.requests, [])

    def test_selection_waits_for_in_flight_read_and_keeps_history_scoped(self):
        process = self.start()
        self.rpc(process, 1, "initialize", {"protocolVersion": "2025-11-25", "capabilities": {"elicitation": {"form": {}}}})
        self.send(process, None, "notifications/initialized")
        original = self.tool(process, "kubernetes_context_read", {})
        self.send(process, 2, "tools/call", {"name": "kubernetes_read", "arguments": {
            "purpose": "Read the paused fixture Pod", "connection_id": original["connections"][0]["connection_id"],
            "operation": "get", "resource": "pods", "name": "paused",
        }})
        self.assertTrue(self.read_started.wait(5))
        self.send(process, 3, "tools/call", {"name": "kubernetes_context_select", "arguments": {"context": "dev"}})
        prompt = self.response(process, "elicitation/create")
        self.reply_to_prompt(process, prompt, {"action": "accept", "content": {"confirm": True}})
        self.assertFalse(select.select([process.stdout], [], [], 0.1)[0], "Switch returned before the captured read")
        self.release_read.set()

        # The two independent replies can arrive in one pipe read.
        buffered = b""
        deadline = time.monotonic() + 10
        while buffered.count(b"\n") < 2:
            self.assertTrue(select.select([process.stdout], [], [], max(0, deadline - time.monotonic()))[0],
                            "Missing read or selection response")
            chunk = os.read(process.stdout.fileno(), 16384)
            self.assertTrue(chunk)
            buffered += chunk
        replies = {}
        for line in buffered.splitlines():
            response = json.loads(line)
            replies[response["id"]] = json.loads(response["result"]["content"][0]["text"])
        self.assertIn("paused", json.dumps(replies[2]))
        reference = replies[2]["ref"]
        self.assertEqual(replies[3]["context"], "dev")
        self.assertTrue(any(port == self.server.server_port and path.endswith("/namespaces/shop/pods/paused")
                            for port, path in self.requests))
        self.assertFalse(any(port == self.other.server_port for port, _ in self.requests))

        matches = self.tool(process, "history_read", {
            "purpose": "Search the new scope", "operation": "search", "query": "paused",
        })
        self.assertNotIn(reference, json.dumps(matches))
        selected = self.tool(process, "kubernetes_context_read", {})
        read = self.tool(process, "kubernetes_read", {
            "purpose": "Read Pods after switching", "connection_id": selected["connections"][0]["connection_id"],
            "operation": "list", "resource": "pods",
        })
        self.assertIn("checkout", json.dumps(read))
        self.assertTrue(any(port == self.other.server_port and "/namespaces/apps/pods" in path
                            for port, path in self.requests))
        self.send(process, 4, "tools/call", {"name": "history_read", "arguments": {
            "purpose": "Check old scope visibility", "operation": "read", "ref": reference,
        }})
        denied = self.response(process, "cross-scope history read")
        self.assertEqual((denied["id"], denied["error"]["code"]), (4, -32000))
        self.assertNotIn("paused-uid", json.dumps(denied))

    def test_rejected_client_prompt_can_recover_from_missing_default(self):
        alternate = self.root / "alternate.yaml"
        alternate.write_text(self.kubeconfig.read_text())
        self.kubeconfig.unlink()
        process = self.start()
        self.rpc(process, 1, "initialize", {"protocolVersion": "2025-11-25", "capabilities": {"elicitation": {"form": {}}}})
        self.send(process, None, "notifications/initialized")
        self.assertIn("No kubeconfig", self.tool(process, "kubernetes_context_read", {})["error"])
        self.send(process, 2, "tools/call", {"name": "kubernetes_context_select", "arguments": {"kubeconfig": str(alternate)}})
        prompt = self.response(process, "elicitation/create")
        self.reply_to_prompt(process, prompt, error={"code": -32601, "message": "Unsupported"})
        reply = self.response(process, "selection fallback")
        status = json.loads(reply["result"]["content"][0]["text"])
        control = status["local_target_control"]
        selected = subprocess.run([str(BINARY), "mcp", "target", control, str(alternate)],
                                  check=True, capture_output=True, text=True)
        self.assertIn('Context: "staging"', selected.stdout)
        self.assertEqual(self.tool(process, "kubernetes_context_read", {})["context"], "staging")
        reset = subprocess.run([str(BINARY), "mcp", "target", control], capture_output=True, text=True)
        self.assertNotEqual(reset.returncode, 0)
        self.assertIn("No kubeconfig", reset.stderr)
        self.assertIn("No kubeconfig", self.tool(process, "kubernetes_context_read", {})["error"])
        self.assertEqual(self.requests, [])

    def reply_to_prompt(self, process, prompt, result=None, *, error=None):
        payload = {"error": error} if error is not None else {"result": result}
        process.stdin.write(json.dumps({"jsonrpc": "2.0", "id": prompt["id"], **payload}) + "\n")
        process.stdin.flush()
