"""Standalone MCP subprocess tests using a local Kubernetes API fixture."""

import json
import select
import subprocess
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

from support import BINARY


class StandaloneDebugTests(unittest.TestCase):
    def setUp(self):
        self.storage = tempfile.TemporaryDirectory(prefix="plous-debug-")
        self.root = Path(self.storage.name)
        self.requests = []
        requests = self.requests

        class API(BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass

            def do_GET(self):
                requests.append(self.path)
                if self.path == "/api/v1":
                    result = {"kind": "APIResourceList", "groupVersion": "v1", "resources": [
                        {"name": "pods", "kind": "Pod", "namespaced": True, "verbs": ["get", "list"]}
                    ]}
                elif self.path == "/version":
                    result = {"gitVersion": "fixture"}
                elif "/pods" in self.path:
                    result = {"apiVersion": "v1", "kind": "PodList", "metadata": {"resourceVersion": "1"}, "items": [
                        {"apiVersion": "v1", "kind": "Pod", "metadata": {"name": "checkout", "namespace": "shop", "uid": "pod-uid"},
                         "spec": {"containers": [{"name": "app", "image": "checkout:fixture"}]}, "status": {"phase": "Running"}}
                    ]}
                else:
                    self.send_error(404)
                    return
                body = json.dumps(result).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), API)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.kubeconfig = self.root / "kubeconfig"
        self.kubeconfig.write_text(f"""apiVersion: v1
kind: Config
clusters:
- name: fixture
  cluster:
    server: http://127.0.0.1:{self.server.server_port}
contexts:
- name: staging
  context:
    cluster: fixture
    namespace: shop
current-context: staging
""")
        self.processes = []

    def tearDown(self):
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
        self.storage.cleanup()

    def start(self, *options):
        process = subprocess.Popen([
            str(BINARY), "mcp", "serve", "--context", "staging", "--kubeconfig", str(self.kubeconfig),
            "--data-dir", str(self.root / "evidence"), *options,
        ], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        self.processes.append(process)
        return process

    def rpc(self, process, identity, method, params=None):
        request = {"jsonrpc": "2.0", "id": identity, "method": method}
        if params is not None:
            request["params"] = params
        process.stdin.write(json.dumps(request) + "\n")
        process.stdin.flush()
        self.assertTrue(select.select([process.stdout], [], [], 10)[0], f"No response to {method}")
        line = process.stdout.readline()
        self.assertTrue(line, f"Server stopped during {method}")
        response = json.loads(line)
        self.assertEqual(response["id"], identity)
        self.assertNotIn("error", response)
        return response["result"]

    def test_shared_reads_capabilities_and_independent_profiles(self):
        readonly = self.start()
        active = self.start("--allow-exec", "--diagnostic-image", "fixture-image")
        init = self.rpc(readonly, 1, "initialize")
        self.assertIn("staging", init["instructions"])
        self.assertIn("## Reproduce", init["instructions"])
        reads = {tool["name"]: tool for tool in self.rpc(readonly, 2, "tools/list")["tools"]}
        tools = {tool["name"]: tool for tool in self.rpc(active, 2, "tools/list")["tools"]}
        self.assertNotIn("kubernetes_exec_run", reads)
        self.assertNotIn("kubernetes_show", reads)
        self.assertIn("kubernetes_diagnostic_run", tools)
        self.assertFalse(tools["kubernetes_exec_run"]["annotations"]["readOnlyHint"])
        connection = reads["kubernetes_resources_read"]["inputSchema"]["properties"]["connection_id"]["enum"][0]
        result = self.rpc(readonly, 3, "tools/call", {"name": "kubernetes_resources_read", "arguments": {
            "purpose": "inspect checkout", "connection_id": connection, "operation": "get", "resource": "pods", "namespace": "shop",
        }})
        evidence = json.loads(result["content"][0]["text"])
        self.assertIn("record:", evidence["ref"])
        self.assertIn("checkout", json.dumps(evidence))
        before = list(self.requests)
        result = self.rpc(active, 3, "tools/call", {"name": "kubernetes_exec_run", "arguments": {
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

    def test_server_requires_explicit_context(self):
        result = subprocess.run([str(BINARY), "mcp", "serve", "--data-dir", str(self.root / "unused")], capture_output=True, text=True, timeout=10)
        self.assertEqual(result.returncode, 2)
        self.assertIn("--context is required", result.stderr)
        self.assertEqual(result.stdout, "")
        self.assertFalse((self.root / "unused").exists())
