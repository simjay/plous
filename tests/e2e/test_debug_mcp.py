"""Standalone MCP subprocess tests using a local Kubernetes API fixture."""

import json
import os
import select
import shutil
import subprocess
import tempfile
import threading
import tomllib
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
                requests.append((self.server.server_port, self.path))
                if self.path == "/api/v1":
                    result = {"kind": "APIResourceList", "groupVersion": "v1", "resources": [
                        {"name": "pods", "kind": "Pod", "namespaced": True, "verbs": ["get", "list"]}
                    ]}
                elif self.path == "/version":
                    result = {"gitVersion": "fixture"}
                elif "/pods" in self.path:
                    result = {"apiVersion": "v1", "kind": "PodList", "metadata": {"resourceVersion": "1"}, "items": [
                        {"apiVersion": "v1", "kind": "Pod", "metadata": {"name": "checkout", "namespace": self.path.split("/namespaces/", 1)[1].split("/")[0], "uid": "pod-uid"},
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
        process = subprocess.Popen([
            str(BINARY), "mcp", "serve",
            "--data-dir", str(self.root / "evidence"), *options,
        ], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
            env=dict(os.environ, KUBECONFIG=str(self.kubeconfig)))
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
        self.assertIn("## Reproduce", init["instructions"])
        reads = {tool["name"]: tool for tool in self.rpc(readonly, 2, "tools/list")["tools"]}
        tools = {tool["name"]: tool for tool in self.rpc(active, 2, "tools/list")["tools"]}
        self.assertNotIn("kubernetes_exec_run", reads)
        self.assertNotIn("kubernetes_show", reads)
        self.assertIn("kubernetes_context_read", reads)
        self.assertIn("kubernetes_diagnostic_run", tools)
        self.assertFalse(tools["kubernetes_exec_run"]["annotations"]["readOnlyHint"])
        target = self.tool(readonly, "kubernetes_context_read", {})
        connection = target["connections"][0]["connection_id"]
        self.assertEqual(target["context"], "staging")
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

    def tool(self, process, name, arguments):
        result = self.rpc(process, self.next_id, "tools/call", {"name": name, "arguments": arguments})
        self.next_id += 1
        return json.loads(result["content"][0]["text"])

    def test_setup_registrations_launch_the_shared_stdio_server(self):
        for client, config_path, server_keys in (
            ("opencode", ".config/opencode/opencode.json", ("mcp", "plous_debug")),
            ("grok", ".grok/config.toml", ("mcp_servers", "plous_debug")),
            ("agy", ".gemini/config/mcp_config.json", ("mcpServers", "plous_debug")),
            ("cursor", ".cursor/mcp.json", ("mcpServers", "plous_debug")),
            ("gemini", ".gemini/settings.json", ("mcpServers", "plous_debug")),
        ):
            with self.subTest(client=client):
                home = self.root / client
                subprocess.run([
                    str(BINARY), "setup", client, "--home", str(home),
                    "--kubeconfig", str(self.kubeconfig), "--context", "dev",
                    "--data-dir", str(self.root / "evidence"),
                ], check=True, capture_output=True, text=True)
                path = home / config_path
                config = path.read_text()
                server = tomllib.loads(config) if path.suffix == ".toml" else json.loads(config)
                for key in server_keys:
                    server = server[key]
                command = server["command"]
                if isinstance(command, str):
                    command = [command, *server["args"]]
                process = subprocess.Popen(command, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                           stderr=subprocess.PIPE, text=True)
                self.processes.append(process)
                initialized = self.rpc(process, 1, "initialize")
                self.assertIn("## Verify", initialized["instructions"])
                target = self.tool(process, "kubernetes_context_read", {})
                self.assertEqual((target["context"], target["default_namespace"]), ("dev", "apps"))
                evidence = self.tool(process, "kubernetes_resources_read", {
                    "purpose": "inspect checkout", "connection_id": target["connections"][0]["connection_id"],
                    "operation": "get", "resource": "pods",
                })
                self.assertIn("record:", evidence["ref"])
                self.assertIn("checkout", json.dumps(evidence))
                process.stdin.close()
                self.assertEqual(process.wait(timeout=15), 0, process.stderr.read())
        backends = set()
        for path in (self.root / "evidence").rglob("execution/*.jsonl"):
            for line in path.read_text().splitlines():
                entry = json.loads(line)
                if entry.get("request", {}).get("role") == "external-tools":
                    backends.add(entry["request"]["backend"])
        self.assertEqual(backends, {"opencode", "grok", "agy", "cursor", "gemini"})

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
        pinned_first = self.tool(pinned, "kubernetes_context_read", {})
        self.assertEqual(first, pinned_first)
        args = {
            "purpose": "inspect Pods", "connection_id": first["connections"][0]["connection_id"],
            "operation": "get", "resource": "pods",
        }
        old = self.tool(live, "kubernetes_resources_read", args)
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
        stale = self.tool(live, "kubernetes_resources_read", args)
        self.assertIn("denied", json.dumps(stale))
        self.assertEqual(self.requests, before)
        args["connection_id"] = current["connections"][0]["connection_id"]
        new = self.tool(live, "kubernetes_resources_read", args)
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
