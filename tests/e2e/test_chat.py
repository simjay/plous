"""Exercise settings, streaming, cancellation, and history through the executable."""
import http.server
import json
import os
import sys
from pathlib import Path
import tempfile
import threading
import unittest

from support import FIXTURE, ROOT, Terminal


class ChatTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory(prefix="plous-chat-")
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        self.state = self.root / "state"
        self.requests = []
        self.catalog_calls = 0
        self.tool_steps = []
        self.stream_steps = []
        self.reply_start = "Hello from "
        self.reply_end = "local AI"
        self.release = threading.Event()
        owner = self

        class Provider(http.server.BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass

            def do_GET(self):
                owner.catalog_calls += 1
                self.send_error(500, "Model discovery must not run")

            def do_POST(self):
                body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                owner.requests.append(body)
                self.send_response(200)
                self.send_header("Content-Type", "text/event-stream")
                self.end_headers()

                def chunk(content="", finish=None, delta=None):
                    data = {"id": "mock", "object": "chat.completion.chunk", "created": 1,
                            "model": "gpt-5.4-mini", "choices": [{"index": 0, "delta": delta or {"content": content},
                                                               "finish_reason": finish}]}
                    self.wfile.write(("data: " + json.dumps(data) + "\n\n").encode())
                    self.wfile.flush()

                try:
                    if owner.stream_steps:
                        step = owner.stream_steps.pop(0)
                        if "tool" in step:
                            chunk(delta={"tool_calls": [{"index": 0, "id": f"call-{len(owner.requests)}",
                                                         "type": "function", "function": {
                                                             "name": step["tool"], "arguments": json.dumps(step["arguments"](body) if callable(step["arguments"]) else step["arguments"])}}]})
                            chunk(finish="tool_calls")
                        else:
                            chunk(step["text"](body) if callable(step["text"]) else step["text"])
                            if step.get("wait"):
                                owner.release.wait(15)
                            chunk(finish="stop")
                        self.wfile.write(b"data: [DONE]\n\n")
                        self.wfile.flush()
                        return
                    if owner.tool_steps:
                        name, arguments = owner.tool_steps.pop(0)
                        chunk(delta={"tool_calls": [{"index": 0, "id": f"call-{len(owner.requests)}",
                                                     "type": "function", "function": {
                                                         "name": name, "arguments": json.dumps(arguments)}}]})
                        chunk(finish="tool_calls")
                        self.wfile.write(b"data: [DONE]\n\n")
                        self.wfile.flush()
                        return
                    chunk(owner.reply_start)
                    if next((m.get("content") for m in reversed(body["messages"]) if m["role"] == "user"), "") == "wait":
                        owner.release.wait(10)
                    chunk(owner.reply_end)
                    chunk("", "stop")
                    self.wfile.write(b"data: [DONE]\n\n")
                    self.wfile.flush()
                except (BrokenPipeError, ConnectionResetError):
                    pass

        self.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Provider)
        threading.Thread(target=self.server.serve_forever, daemon=True).start()
        self.addCleanup(self.stop_server)

    def stop_server(self):
        self.release.set()
        self.server.shutdown()
        self.server.server_close()

    def start(self, kubeconfig=FIXTURE, env_overrides=None):
        config = self.state / "agent" / "config.yaml"
        config.parent.mkdir(parents=True, exist_ok=True)
        config.write_text("max_calls: 128\n")
        app = Terminal(self.state, kubeconfig, width=160, height=36, env_overrides=env_overrides)
        self.addCleanup(app.close)
        app.expect("AI assistant")
        return app

    def configure_connection(self, app, name, provider, instructions=""):
        steps = {"openai": 0, "anthropic": 1}[provider]
        endpoint = f"http://127.0.0.1:{self.server.server_port}/v1"
        app.send("\x1b[C" * steps + "\rtest-only-secret\r")
        app.send("\x1b[B" * 2 + "\r")  # Open optional connection details from Save.
        app.send(name + "\r")
        app.send(endpoint + "\r")
        app.send("\x15\r" + instructions + "\r\r")
        self.assertNotIn("test-only-secret", app.text())
        app.expect_screen("Provider connection saved")
        app.send("\x1b")
        app.expect_screen("Shared across workspaces")

    def records(self, path):
        try:
            data = path.read_bytes()
        except FileNotFoundError:
            return []
        # An append can be observed before its final newline reaches the reader.
        return [json.loads(line) for line in data.split(b"\n")[:-1] if line]

    def responses(self, workspace=True):
        pattern = "**/conversations/*/conversation/*.jsonl" if workspace else "conversations/*/conversation/*.jsonl"
        return sorted((path, record["turn_id"])
                      for path in self.state.glob(pattern)
                      for record in self.records(path) if record.get("role") == "user")

    def response(self, reference):
        path, turn = reference
        latest = {"status": "streaming", "text": ""}
        for record in self.records(path):
            if record.get("role") == "assistant" and record["turn_id"] == turn:
                latest = {**record, "status": "streaming"}
            if record.get("kind") == "turn_end" and record["turn_id"] == turn:
                self.assertEqual(record["answer_id"], latest["id"])
                latest["status"] = record["status"]
        return latest

    def request(self, reference):
        path, turn = reference
        return next(record["request"] for record in self.records(path.parent.parent / "execution" / path.name)
                    if record.get("kind") == "request" and record["message_id"] == turn)

    def wait_response(self, app, reference, predicate):
        app.wait(lambda: predicate(self.response(reference)), "saved conversation response", timeout=60)
        return self.response(reference)

    def saved_answer_id(self, reference, text):
        path, _ = reference
        answer = self.response(reference)
        self.assertEqual(answer["status"], "completed")
        self.assertEqual(answer["text"], text)
        return "record:" + path.parent.parent.name + ":" + answer["id"]

    def assert_tool_pairs(self, messages):
        pending = set()
        for message in messages:
            if message["role"] == "tool":
                self.assertIn(message["tool_call_id"], pending)
                pending.remove(message["tool_call_id"])
            else:
                self.assertFalse(pending, "unfinished tool exchange")
                pending.update(call["id"] for call in message.get("tool_calls", []))
        self.assertFalse(pending)

    def test_read_older_messages_then_resume_stream_and_reopen(self):
        self.reply_start = "\n".join(f"Earlier reply line {i:02d}" for i in range(80)) + "\nLatest reply"
        self.reply_end = "\nFinal streamed reply"
        app = self.start()
        app.send(":s\r\r\r\r")
        self.configure_connection(app, "Local", "openai")
        app.send(":c\rwait\r")
        responses = lambda: self.responses(workspace=False)
        app.wait(lambda: len(responses()) == 1, "saved long reply")
        response = responses()[0]
        self.wait_response(app, response, lambda r: r["text"] == self.reply_start)
        app.expect("Latest reply")
        app.send("\x1b")
        start = len(app.output)
        app.send("\x1b[H")
        app.expect("│ wait", start=start)
        self.release.set()
        self.wait_response(app, response, lambda r: r["status"] == "completed")
        start = len(app.output)
        app.send("\x1b[F")
        app.expect("Final streamed reply", start=start)
        start = len(app.output)
        app.send("/Earlier reply line 00")
        app.expect("│ wait", start=start)
        app.send("\r\x1b")
        app.send("\x03")
        app.assert_exit()
        app.close()

        restarted = self.start()
        restarted.expect("Final streamed reply")
        restarted.send(":c\r")
        restarted.send("\x1b")
        start = len(restarted.output)
        restarted.send("\x1b[H")
        restarted.expect("│ wait", start=start)
        restarted.send("\x03")
        restarted.assert_exit()
        self.assertEqual(len(self.requests), 1)

    def test_wider_reply_reader_preserves_draft_and_saved_sources(self):
        self.reply_start = "## Answer\n\n| Resource | Status |\n| --- | --- |\n| frontend | Ready |\n\n"
        self.reply_end = "Recorded answer only."
        app = self.start()
        app.send(":s\r\r\r\r")
        self.configure_connection(app, "Local", "openai")
        app.send(":c\rinspect frontend\r")
        responses = lambda: self.responses(workspace=False)
        app.wait(lambda: len(responses()) == 1, "saved reply")
        self.wait_response(app, responses()[0], lambda r: r["status"] == "completed")
        # Persistence finishes before its event reaches the interface.
        app.expect_screen("Read reply")
        app.send("keep this draft")
        app.send("\x1b")
        app.send(":A\r")
        app.expect_screen("Plous Markdown Viewer")
        app.send("/Recorded\r")
        app.expect_screen("Recorded answer only.")
        app.send("\r")
        app.expect_screen("No tool sources recorded")
        app.send("\x1b")
        app.expect_screen("Plous Markdown Viewer")
        app.send("\x1b")
        app.send("\x1b")
        app.expect_screen("keep this draft")
        app.send("\x03")
        app.assert_exit()
        self.assertEqual(len(self.requests), 1)

    def test_model_picker_switches_requests_and_survives_restart(self):
        app = self.start()
        app.send(":s\r\r\r\r")
        self.configure_connection(app, "Primary", "openai")
        app.wait(lambda: len(list(self.state.glob("settings/*/change.json"))) == 1,
                 "saved vendor connection")

        responses = lambda: self.responses(workspace=False)
        app.send(":c\r")
        app.expect("400k tokens")
        app.send("wait\r")
        app.wait(lambda: len(responses()) == 1, "first request")
        self.wait_response(app, responses()[0], lambda r: r["text"] == "Hello from ")
        self.assertEqual(self.requests[0]["model"], "gpt-5.4-mini")

        app.send("/model\r")
        app.expect("Choose a model")
        app.send("\x1b[B\r")
        app.wait(lambda: len(list(self.state.glob("settings/*/change.json"))) == 2, "model selection")
        app.expect("400k tokens")  # The running operation retains its captured model.
        self.assertEqual(len(self.requests), 1)
        self.assertEqual(self.response(responses()[0])["status"], "streaming")
        self.release.set()
        self.wait_response(app, responses()[0], lambda r: r["status"] == "completed")
        app.expect("Reply details")

        app.send("Follow up\r")
        app.wait(lambda: len(responses()) == 2, "second request")
        self.wait_response(app, responses()[1], lambda r: r["status"] == "completed")
        self.assertEqual(self.requests[1]["model"], "gpt-5.4")
        self.assertEqual(self.requests[1]["messages"][-1]["content"], "Follow up")
        requests = [self.request(reference) for reference in responses()]
        self.assertEqual([request["agent"]["name"] for request in requests],
                         ["Primary", "Primary"])
        app.send("\x03")
        app.assert_exit()
        app.close()

        restarted = self.start()
        restarted.send(":c\r/model\r")
        restarted.expect("GPT-5.4 ✓")
        restarted.send("\x1b")
        restarted.send("After restart\r")
        restarted.wait(lambda: len(responses()) == 3, "request after restart")
        self.wait_response(restarted, responses()[2], lambda r: r["status"] == "completed")
        self.assertEqual(self.requests[2]["model"], "gpt-5.4")
        settings = sorted(self.state.glob("settings/*/change.json"))
        current = json.loads(settings[-1].read_text())["after"]
        self.assertEqual(len(current["providers"]), 1)
        self.assertNotIn("model", current["providers"][0])
        self.assertEqual(current["agent"]["model"], "gpt-5.4")
        self.assertEqual(self.catalog_calls, 0)
        self.assertTrue(all(request["role"] == "chat" for request in requests))
        restarted.send("\x03")
        restarted.assert_exit()

    def test_chat_setup_send_cancel_and_reopen(self):
        app = self.start()
        app.send(":settings\r")
        app.expect("Shared across workspaces")
        app.send("\r\r\r")
        app.expect("Vendor")
        self.configure_connection(app, "Test assistant", "openai", "Be concise")
        app.wait(lambda: len(list(self.state.glob("settings/*/change.json"))) == 1, "settings save")
        app.send(":new\rChat workspace\r\r")
        app.wait(lambda: len(list(self.state.glob("workspaces/*/revisions/*/change.json"))) == 1, "workspace save")
        app.send(":c\rHello\x1b[13;2u:q\x0afallback")  # Shift+Enter, then literal colon text.
        app.wait_json(self.state / "session.json", lambda s: "Hello\n:q\nfallback" in s["drafts"].values())
        self.assertEqual(len(self.requests), 0)
        app.send("\r")
        app.wait(lambda: len(self.responses()) == 1, "saved request")
        self.wait_response(app, self.responses()[0], lambda r: r["status"] == "completed")
        self.assertEqual(self.response(self.responses()[0])["text"], "Hello from local AI")
        self.assertEqual(self.requests[0]["messages"][-1]["content"], "Hello\n:q\nfallback")
        self.assertFalse(any('"connections":' in m["content"] for m in self.requests[0]["messages"]))
        saved_request = self.request(self.responses()[0])
        self.assertEqual(saved_request.get("context", ""), "")
        app.expect("local AI")

        app.send("wait\r")
        app.wait(lambda: len(self.responses()) == 2, "second saved request")
        second = self.responses()[1]
        self.wait_response(app, second, lambda r: r["text"] == "Hello from ")
        app.send("\x1b")
        self.assertEqual(self.response(second)["status"], "streaming")
        app.send(":X\r")
        self.wait_response(app, second, lambda r: r["status"] == "cancelled")
        app.send("\x03")
        app.assert_exit()
        app.close()
        self.release.set()

        restarted = self.start()
        restarted.expect("cancelled")
        restarted.send(":C\r")
        restarted.expect("No conversation yet.")
        restarted.send("\x1b")
        restarted.send(":H\r")
        restarted.expect("Conversations")
        restarted.send("\x1b[B\r")
        restarted.expect("cancelled")
        restarted.send("\x03")
        restarted.assert_exit()
        self.assertEqual(len(self.requests), 2)
        self.assertEqual(self.catalog_calls, 0)
        self.assertEqual((self.state / "credentials.json").stat().st_mode & 0o777, 0o600)
        for path in self.state.rglob("*"):
            if path.is_file() and path.suffix in (".json", ".jsonl") and path.name != "credentials.json":
                self.assertNotIn("test-only-secret", path.read_text())
        saved = json.loads((self.state / "session.json").read_text())
        self.assertTrue(saved["chat_ids"][""])

    def test_general_chat_before_workspace_and_after_restart(self):
        app = self.start()
        app.send("ccHello before setup")
        app.wait_json(self.state / "session.json", lambda s: s["drafts"].get("") == "Hello before setup")
        app.send("\r")
        self.assertEqual(len(self.requests), 0)
        app.send("\x1b")
        app.send(":s\r\r\r\r")
        self.configure_connection(app, "Test assistant", "openai", "Be concise")
        app.wait(lambda: len(list(self.state.glob("settings/*/change.json"))) == 1, "settings save")
        app.send(":c\r\r")
        responses = lambda: self.responses(workspace=False)
        app.wait(lambda: len(responses()) == 1, "general chat response")
        self.wait_response(app, responses()[0], lambda r: r["status"] == "completed")
        self.assertEqual(self.requests[0]["messages"][-1]["content"], "Hello before setup")
        self.assertNotIn("Selected workspace metadata:", self.requests[0]["messages"][0]["content"])
        self.assertEqual(list(self.state.glob("workspaces/*/revisions/*/change.json")), [])
        app.send("Draft for later\x03")
        app.assert_exit()
        app.close()

        restarted = self.start()
        restarted.expect("local AI")
        restarted.expect("Draft for later")
        restarted.send(":H\r")
        restarted.expect("Conversations")
        restarted.send("\r:c\r continued")
        restarted.wait_json(self.state / "session.json", lambda s: s["drafts"].get("") == "Draft for later continued")
        restarted.send("\x03")
        restarted.assert_exit()
        self.assertEqual(len(self.requests), 1)

    def test_agent_reads_workspace_rejects_write_and_restores_bounded_evidence(self):
        app = self.start()
        app.send(":s\r\r\r\r")
        self.configure_connection(app, "Local", "openai")
        app.send(":new\rInvestigation workspace\r\r")
        app.wait(lambda: len(list(self.state.glob("workspaces/*/revisions/*/change.json"))) == 1,
                 "workspace save")
        app.send("\x1b[B\rInvestigation source\r\r" + str(self.root) + "\r\r")
        app.expect("Source added")
        app.send("/Investigation source\r")
        self.stream_steps = [
            {"tool": "workspace_read", "arguments": {"purpose": "Inspect saved workspace configuration", "operation": "workspace", "limit": 1}},
            {"tool": "workspace_read", "arguments": {"purpose": "Verify writes are denied", "operation": "write"}},
        ]
        app.send(":c\rInspect the workspace\r")
        app.wait(lambda: len(self.responses()) == 1, "investigation request")
        reference = self.responses()[0]
        self.wait_response(app, reference, lambda response: response["status"] == "completed")
        self.assertEqual(len(self.requests), 3)
        self.assertIn("tools", self.requests[0])
        self.assertNotIn("memory", [tool["function"]["name"] for tool in self.requests[0]["tools"]])
        self.assertNotIn("response_format", self.requests[0])
        findings = self.requests[2]["messages"]
        self.assertIn("Investigation workspace", json.dumps(findings))
        self.assertIn("workspace_read supports workspace or activity", json.dumps(findings))
        self.assertIn("purpose", json.dumps(self.requests[0]["tools"]))

        conversation, _ = reference
        trace = self.records(conversation.parent.parent / "reasoning" / conversation.name)
        self.assertTrue(any(record.get("kind") == "tool" and record["status"] == "completed" for record in trace))
        self.assertTrue(any(record.get("kind") == "tool" and record["status"] == "denied" for record in trace))
        expected_files = {"metadata.json", "manifest.json", "conversation", "reasoning", "execution"}
        app.wait(lambda: {path.name for path in conversation.parent.parent.iterdir()} == expected_files,
                 "completed native history publication")
        manifest = json.loads((conversation.parent.parent / "manifest.json").read_text())
        self.assertEqual(set(manifest["streams"]), {"conversation", "reasoning", "execution"})
        self.assertFalse(list(self.state.rglob("active.md")))
        self.assertTrue(all(record.get("projection") for record in trace))
        self.assertTrue(all(record.get("role") in ("user", "assistant") or record.get("kind") == "turn_end"
                            for record in self.records(conversation)))
        self.assertEqual(self.response(reference)["text"], "Hello from local AI")

        app.send("Draft retained")
        app.send("\x1b")
        app.expect_screen("[:A] Read reply")
        app.send("\r")
        app.expect("workspace_read")
        app.send("\r")
        app.expect("View captured result")
        app.send("\r")
        app.expect_screen("Plous Editor · Captured source")
        captured_name = next(record["projection"]["rows"][0]["facts"]["name"]["text"]
                             for record in trace if record.get("status") == "completed")
        app.send("/" + captured_name + "\r")
        app.expect_screen("Search · 1/")
        self.assertEqual(len(self.requests), 3, "opening a saved source contacted the provider")
        for _ in range(4):
            app.send("\x1b")
        app.send("cc")
        app.expect("Draft retained")
        app.send("\x15Follow up without evidence\r")
        app.wait(lambda: len(self.responses()) == 2, "followup request")
        self.wait_response(app, self.responses()[1], lambda response: response["status"] == "completed")
        messages = self.requests[3]["messages"]
        self.assertTrue(any(message["role"] == "tool" for message in messages))
        self.assert_tool_pairs(messages)
        self.assertEqual(messages[-1]["content"], "Follow up without evidence")
        self.assertIn("Investigation workspace", json.dumps(messages))
        self.assertEqual(self.catalog_calls, 0)
        app.send("\x03")
        app.assert_exit()
        app.close()

        restarted = self.start()
        restarted.send("\x1b[1;3D/Investigation source\r:c\r")
        restarted.send("\x1b")
        restarted.expect_screen("[:A] Read reply")
        restarted.send("[\r")
        restarted.expect("workspace_read")
        restarted.send("\x1b[B\r")
        restarted.expect("Read denied")
        self.assertEqual(len(self.requests), 4, "reopening sources reran the investigation")
        restarted.send("\x03")
        restarted.assert_exit()

    def test_history_read_restores_sources_after_restart(self):
        app = self.start()
        app.send(":s\r\r\r\r")
        self.configure_connection(app, "Local", "openai")
        self.reply_start, self.reply_end = "Saved decision: ", "release-blue-marker"
        app.send(":c\rWhich release did we choose?\r")
        responses = lambda: self.responses(workspace=False)
        app.wait(lambda: len(responses()) == 1, "original conversation record")
        self.wait_response(app, responses()[0], lambda r: r["status"] == "completed")
        record_id = self.saved_answer_id(responses()[0], self.reply_start + self.reply_end)

        self.stream_steps = [
            {"tool": "history_read", "arguments": {"purpose": "Recover saved evidence", "operation": "read", "ref": record_id}},
            {"text": "The earlier decision was blue."},
        ]
        app.send("Recall our original decision\r")
        app.wait(lambda: len(responses()) == 2, "history read")
        reference = responses()[1]
        self.wait_response(app, reference, lambda r: r["status"] == "completed")
        self.assertEqual(len(self.requests), 3)
        self.assertEqual({request["model"] for request in self.requests}, {"gpt-5.4-mini"})
        self.assertIn("Saved decision: release-blue-marker", json.dumps(self.requests[-1]["messages"]))
        conversation, turn_id = reference
        trace = [record for record in self.records(conversation.parent.parent / "reasoning" / conversation.name)
                 if record.get("message_id") == turn_id]
        results = [record for record in trace if record.get("tool") == "history_read"]
        self.assertEqual(len(results), 1)
        self.assertEqual(results[0]["projection"]["rows"][0]["identity"]["name"], record_id)
        self.assertIn("Saved decision:", json.dumps(results[0]["projection"]))
        self.assertEqual(self.response(reference)["text"], "The earlier decision was blue.")
        app.send("\x03")
        app.assert_exit()
        app.close()

        restarted = self.start()
        restarted.expect("The earlier decision was blue.")
        self.assertEqual(len(self.requests), 3)
        restarted.send(":c\r")
        restarted.send("\x1b")
        restarted.send("\r")
        restarted.expect("Saved history")
        restarted.send("\x1b[B\r")
        restarted.expect("View captured result")
        restarted.send("\r")
        restarted.expect("Captured source")
        restarted.send("/Saved decision: release-blue-marker\r")
        restarted.resize(161, 36)
        restarted.expect_screen("1/1 matches")
        restarted.expect_screen('"ref": "record:')
        self.assertEqual(len(self.requests), 3, "opening a saved recall contacted the provider")
        for _ in range(3):
            restarted.send("\x1b")
        restarted.send(":c\rWhat did we conclude?\r")
        restarted.wait(lambda: len(responses()) == 3, "followup after recall restart")
        self.wait_response(restarted, responses()[-1], lambda r: r["status"] == "completed")
        messages = self.requests[-1]["messages"]
        self.assertEqual(messages[-1]["content"], "What did we conclude?")
        self.assertTrue(any(message["role"] == "tool" for message in messages))
        self.assert_tool_pairs(messages)
        self.assertEqual([record for record in self.records(conversation.parent.parent / "reasoning" / conversation.name)
                          if record.get("message_id") == turn_id], trace)
        restarted.send("\x03")
        restarted.assert_exit()

    def test_stop_after_history_read_survives_restart(self):
        app = self.start()
        app.send(":s\r\r\r\r")
        self.configure_connection(app, "Local", "openai")
        app.send(":c\rSave an observation to recall\r")
        responses = lambda: self.responses(workspace=False)
        app.wait(lambda: len(responses()) == 1, "original observation")
        self.wait_response(app, responses()[0], lambda r: r["status"] == "completed")
        record_id = self.saved_answer_id(responses()[0], "Hello from local AI")
        self.stream_steps = [
            {"tool": "history_read", "arguments": {"purpose": "Recover saved evidence", "operation": "read", "ref": record_id}},
            {"text": "Checking saved evidence", "wait": True},
        ]
        app.send("Start a recall to cancel\r")
        app.wait(lambda: len(self.requests) == 3, "running main model request", timeout=60)
        reference = responses()[1]
        app.send("\x1b")
        app.send(":X\r")
        self.wait_response(app, reference, lambda r: r["status"] == "cancelled")
        self.release.set()
        app.send("\x03")
        app.assert_exit()
        app.close()

        restarted = self.start()
        restarted.expect("cancelled")
        self.assertEqual(len(self.requests), 3)
        restarted.send(":c\rHello after cancellation\r")
        restarted.wait(lambda: len(responses()) == 3, "new request after cancelled recall")
        self.wait_response(restarted, responses()[-1], lambda r: r["status"] == "completed")
        messages = self.requests[-1]["messages"]
        self.assertEqual(messages[-1]["content"], "Hello after cancellation")
        self.assertNotIn("Checking saved evidence", json.dumps(messages))
        self.assertTrue(any(m["role"] == "assistant" for m in messages))
        self.assert_tool_pairs(messages)
        self.assertEqual(self.response(reference)["status"], "cancelled")
        restarted.send("\x03")
        restarted.assert_exit()

    def test_removed_context_editor_preserves_originals_and_reopens(self):
        app = self.start()
        app.send(":s\r\r\r\r")
        self.configure_connection(app, "Local", "openai")
        self.reply_start, self.reply_end = "Decision: ", "release-cobalt"
        app.send(":c\rVerbose original marker stays only in the archive\r")
        responses = lambda: self.responses(workspace=False)
        app.wait(lambda: len(responses()) == 1, "original exchange")
        self.wait_response(app, responses()[0], lambda r: r["status"] == "completed")
        original_path = responses()[0][0]
        original = original_path.read_bytes()

        self.stream_steps = [{"tool": "bash", "arguments": {"command": "printf '\\nDecision: release-cobalt\\n' >> active.md"}},
                             {"text": "Decision retained."}]
        app.send("Keep the decision for later\r")
        app.wait(lambda: len(responses()) == 2, "context edit turn")
        self.wait_response(app, responses()[1], lambda r: r["status"] == "completed")
        self.assertTrue(original_path.read_bytes().startswith(original))
        self.assertFalse(list(self.state.rglob("active.md")))
        self.assertIn("tool is not available", json.dumps(self.requests[-1]["messages"]))
        self.assertIn("Verbose original marker", json.dumps(self.requests[-1]["messages"]))
        self.assertNotIn("bash", [tool["function"]["name"] for tool in self.requests[-1]["tools"]])
        app.send("\x03")
        app.assert_exit()
        app.close()

        restarted = self.start()
        record_id = self.saved_answer_id(responses()[0], "Decision: release-cobalt")
        self.stream_steps = [
            {"tool": "history_read", "arguments": {"purpose": "Recover saved evidence", "operation": "read", "ref": record_id}},
            {"text": "Decision: release-cobalt"},
        ]
        restarted.send(":c\rConfirm our decision\r")
        restarted.wait(lambda: len(responses()) == 3, "reopened context")
        self.wait_response(restarted, responses()[-1], lambda r: r["status"] == "completed")
        messages = self.requests[-1]["messages"]
        self.assertIn("release-cobalt", json.dumps(messages))
        self.assertIn("Verbose original marker", json.dumps(messages))
        self.assertIn("Verbose original marker", original_path.read_text())
        self.assert_tool_pairs(messages)
        restarted.send("\x03")
        restarted.assert_exit()

    def test_workspace_chat_reads_kubernetes_and_helm_while_viewing_helm(self):
        sys.path.insert(0, str(ROOT / "tests/fixtures/helm"))
        from helm_api_fixture import HelmAPI
        api = HelmAPI(self.root)
        self.addCleanup(api.close)
        paths = api.requests
        config = self.root / "kubeconfig"
        config.write_text("apiVersion: v1\nkind: Config\ncontexts:\n- name: staging\n  context: {cluster: fixture, namespace: apps}\nclusters:\n- name: fixture\n  cluster: {server: '" + api.url + "'}\n")
        env = {}
        app = self.start(config, env)
        app.send(":s\r\r\r\r")
        self.configure_connection(app, "Local", "openai")
        app.send(":new\rCommerce\r\r\r")
        app.expect("Add Context")
        app.send(" \x1b[B\r")
        app.expect("Added 1")
        self.assertEqual(paths, [])
        self.assertFalse((self.root / "calls.jsonl").exists())
        app.send("/staging\r")
        app.expect_screen("Helm")
        app.send("/Releases\r")
        app.expect("shop")

        targets = {}

        def target(body):
            system = body["messages"][0]["content"]
            scope = system.split("Captured connection and current page:\n", 1)[1]
            captured = json.JSONDecoder().raw_decode(scope)[0]
            self.assertEqual(captured["workspace"], "Commerce")
            self.assertEqual(captured["viewing"]["tool"], "helm")
            self.assertEqual({c["tool"] for c in captured["connections"]}, {"kubernetes", "helm"})
            self.assertNotIn(str(config), scope)
            targets.update({c["tool"]: c["connection_id"] for c in captured["connections"]})
            return {"purpose": "Read the shop Pod phase", "connection_id": targets["kubernetes"], "operation": "get", "resource": "pods", "name": "shop", "namespace": "apps"}

        self.stream_steps = [
            {"tool": "kubernetes_resources_read", "arguments": target},
            {"tool": "helm_releases_read", "arguments": lambda _: {"purpose": "Read the deployed shop chart version", "connection_id": targets["helm"], "operation": "metadata", "name": "shop", "namespace": "apps", "revision": 2}},
            {"text": "Cross-tool investigation complete."},
        ]
        app.send(":c\rCompare the shop Pod and its Helm release\r")
        app.wait(lambda: len(self.responses()) == 1, "workspace request")
        self.wait_response(app, self.responses()[0], lambda r: r["status"] == "completed")
        self.assertIn("/api/v1/namespaces/apps/pods/shop", [path.split("?")[0] for path in paths])
        self.assertTrue(any("/secrets/sh.helm.release.v1.shop.v2" in path for path in paths))
        self.assertIn("Running", json.dumps(self.requests[2]["messages"]))
        self.assertIn("1.0.0", json.dumps(self.requests[2]["messages"]))
        self.assertFalse((self.root / "writes.jsonl").exists())
        app.send("Keep this workspace draft\x03")
        app.assert_exit()
        app.close()
        restarted = self.start(config, env)
        restarted.send("\x1b[1;3D/staging\r")
        restarted.expect_screen("Helm")
        restarted.send("/Releases\r")
        restarted.expect("Cross-tool investigation complete.")
        restarted.expect("Keep this workspace draft")
        restarted.send("\x03")
        restarted.assert_exit()
