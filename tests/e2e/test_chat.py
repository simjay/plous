"""Exercise settings, streaming, cancellation, and history through the executable."""
import http.server
import json
from pathlib import Path
import tempfile
import threading
import unittest

from support import FIXTURE, Terminal


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
                                                             "name": step["tool"], "arguments": json.dumps(step["arguments"])}}]})
                            chunk(finish="tool_calls")
                        else:
                            chunk(step["text"])
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
                    if body["messages"][-1]["content"] == "wait":
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

    def start(self):
        app = Terminal(self.state, FIXTURE, width=160, height=36)
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

    def records(self, path):
        try:
            data = path.read_bytes()
        except FileNotFoundError:
            return []
        # An append can be observed before its final newline reaches the reader.
        return [json.loads(line) for line in data.split(b"\n")[:-1] if line]

    def responses(self, workspace=True):
        pattern = "workspaces/*/conversations/*/conversation.jsonl" if workspace else "conversations/*/conversation.jsonl"
        return sorted((path, record["turn_id"])
                      for path in self.state.glob(pattern)
                      for record in self.records(path) if record["role"] == "user")

    def response(self, reference):
        path, turn = reference
        latest = {"status": "streaming", "text": ""}
        for record in self.records(path):
            if record["role"] == "assistant" and record["turn_id"] == turn:
                latest = record
        return latest

    def request(self, reference):
        path, turn = reference
        return next(record["request"] for record in self.records(path.with_name("reasoning.jsonl"))
                    if record["kind"] == "request" and record["message_id"] == turn)

    def wait_response(self, app, reference, predicate):
        app.wait(lambda: predicate(self.response(reference)), "saved conversation response")
        return self.response(reference)

    def test_read_older_messages_then_resume_stream_and_reopen(self):
        self.reply_start = "\n".join(f"Earlier reply line {i:02d}" for i in range(80)) + "\nLatest reply"
        self.reply_end = "\nFinal streamed reply"
        app = self.start()
        app.send(":s\r\r")
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

    def test_model_picker_switches_requests_and_survives_restart(self):
        app = self.start()
        app.send(":s\r\r")
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
        app.expect("1.05M tokens")
        self.assertEqual(len(self.requests), 1)
        self.assertEqual(self.response(responses()[0])["status"], "streaming")
        self.release.set()
        self.wait_response(app, responses()[0], lambda r: r["status"] == "completed")

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
        app.expect("AI connections")
        app.send("\r")
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
        self.assertIn('"workspace": "Chat workspace"', self.requests[0]["messages"][0]["content"])
        saved_request = self.request(self.responses()[0])
        self.assertEqual(json.loads(saved_request["context"])["workspace"], "Chat workspace")
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
        restarted.send("\r")
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
        self.assertTrue(saved["chat_ids"][saved["workspace"]])

    def test_general_chat_before_workspace_and_after_restart(self):
        app = self.start()
        app.send("ccHello before setup")
        app.wait_json(self.state / "session.json", lambda s: s["drafts"].get("") == "Hello before setup")
        app.send("\r")
        self.assertEqual(len(self.requests), 0)
        app.send("\x1b")
        app.send(":s\r\r")
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

    def test_agent_reads_workspace_rejects_write_and_keeps_evidence_out_of_followup(self):
        app = self.start()
        app.send(":s\r\r")
        self.configure_connection(app, "Local", "openai")
        app.send(":new\rInvestigation workspace\r\r")
        app.wait(lambda: len(list(self.state.glob("workspaces/*/revisions/*/change.json"))) == 1,
                 "workspace save")
        self.tool_steps = [("memory", {"action": "search", "source": "workspace", "limit": 1}),
                           ("memory", {"action": "write", "source": "workspace"})]
        app.send(":c\rInspect the workspace\r")
        app.wait(lambda: len(self.responses()) == 1, "investigation request")
        reference = self.responses()[0]
        self.wait_response(app, reference, lambda response: response["status"] == "completed")
        self.assertEqual(len(self.requests), 3)
        self.assertIn("tools", self.requests[0])
        self.assertNotIn("response_format", self.requests[0])
        first_result = json.loads(self.requests[1]["messages"][-1]["content"])
        self.assertIn("Investigation workspace", json.dumps(first_result))
        self.assertIn("evidence", first_result)
        denied = json.loads(self.requests[2]["messages"][-1]["content"])
        self.assertIn("unsupported memory operation", json.dumps(denied))

        conversation, _ = reference
        trace = self.records(conversation.with_name("reasoning.jsonl"))
        self.assertTrue(any(record["kind"] == "tool" and record["status"] == "completed" for record in trace))
        self.assertTrue(any(record["kind"] == "tool" and record["status"] == "denied" for record in trace))
        self.assertEqual({path.name for path in conversation.parent.iterdir()},
                         {"metadata.json", "conversation.jsonl", "reasoning.jsonl"})
        self.assertTrue(all(record["role"] in ("user", "assistant") for record in self.records(conversation)))
        self.assertEqual(self.response(reference)["text"], "Hello from local AI")

        app.send("Follow up without evidence\r")
        app.wait(lambda: len(self.responses()) == 2, "followup request")
        self.wait_response(app, self.responses()[1], lambda response: response["status"] == "completed")
        messages = self.requests[3]["messages"]
        self.assertFalse(any(message["role"] == "tool" or message.get("tool_calls") for message in messages))
        self.assertEqual([message["content"] for message in messages if message["role"] == "assistant"],
                         ["Hello from local AI"])
        self.assertEqual(self.catalog_calls, 0)
        app.send("\x03")
        app.assert_exit()

    def test_recursive_investigation_reads_originals_and_reopens_without_trace_in_prompt(self):
        app = self.start()
        app.send(":s\r\r")
        self.configure_connection(app, "Local", "openai")
        self.reply_start, self.reply_end = "Saved decision: ", "release-blue-marker"
        app.send(":c\rWhich release did we choose?\r")
        responses = lambda: self.responses(workspace=False)
        app.wait(lambda: len(responses()) == 1, "original conversation record")
        self.wait_response(app, responses()[0], lambda r: r["status"] == "completed")

        self.stream_steps = [
            {"tool": "investigate", "arguments": {"question": "Retrieve our original release decision"}},
            {"text": 'child = ask("Find the original release decision in conversation records")\nfinish(child["findings"], evidence=child["evidence"])'},
            {"text": 'record = read({"action":"search","source":"conversation","text":"release-blue-marker"})\nfinish("Original release is blue.", evidence=[record["evidence"]])'},
            {"text": "The earlier decision was blue."},
        ]
        app.send("Investigate our original decision\r")
        app.wait(lambda: len(responses()) == 2, "recursive investigation")
        reference = responses()[1]
        self.wait_response(app, reference, lambda r: r["status"] == "completed")
        self.assertEqual(len(self.requests), 5)
        self.assertEqual({request["model"] for request in self.requests}, {"gpt-5.4-mini"})
        self.assertIn("Original release is blue", self.requests[-1]["messages"][-1]["content"])
        conversation, turn_id = reference
        trace = [record for record in self.records(conversation.with_name("reasoning.jsonl"))
                 if record.get("message_id") == turn_id]
        recursive_calls = [record for record in trace if record["kind"] == "investigation"
                           and record.get("parent_source")]
        reads = [record for record in trace if record.get("tool") == "memory" and record["status"] == "started"]
        self.assertTrue(recursive_calls)
        self.assertEqual(len(reads), 1)
        self.assertTrue(reads[0].get("parent_source"))
        self.assertNotIn('record = read(', conversation.read_text())
        self.assertEqual(self.response(reference)["text"], "The earlier decision was blue.")
        app.send("\x03")
        app.assert_exit()
        app.close()

        restarted = self.start()
        restarted.expect("The earlier decision was blue.")
        self.assertEqual(len(self.requests), 5)
        restarted.send(":c\rWhat did we conclude?\r")
        restarted.wait(lambda: len(responses()) == 3, "followup after investigation restart")
        self.wait_response(restarted, responses()[-1], lambda r: r["status"] == "completed")
        messages = self.requests[-1]["messages"]
        self.assertFalse(any(message["role"] == "tool" or message.get("tool_calls") for message in messages))
        self.assertNotIn('record = read(', json.dumps(messages))
        self.assertEqual([m["content"] for m in messages if m["role"] == "assistant"],
                         ["Saved decision: release-blue-marker", "The earlier decision was blue."])
        restarted.send("\x03")
        restarted.assert_exit()

    def test_stop_during_recursive_model_call_survives_restart(self):
        app = self.start()
        app.send(":s\r\r")
        self.configure_connection(app, "Local", "openai")
        self.stream_steps = [
            {"tool": "investigate", "arguments": {"question": "Check archived observations"}},
            {"text": 'child = ask("Read an older observation")\nfinish(child["findings"])'},
            {"text": 'finish("Unfinished observation', "wait": True},
        ]
        app.send(":c\rStart an investigation to cancel\r")
        responses = lambda: self.responses(workspace=False)
        app.wait(lambda: len(self.requests) == 3, "running recursive model request")
        reference = responses()[0]
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
        restarted.wait(lambda: len(responses()) == 2, "new request after cancelled investigation")
        self.wait_response(restarted, responses()[-1], lambda r: r["status"] == "completed")
        messages = self.requests[-1]["messages"]
        self.assertEqual([m["content"] for m in messages if m["role"] == "user"], ["Hello after cancellation"])
        self.assertFalse(any(m["role"] in ("assistant", "tool") for m in messages))
        self.assertEqual(self.response(reference)["status"], "cancelled")
        restarted.send("\x03")
        restarted.assert_exit()
