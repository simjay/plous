package nativeagents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/ai/cli"
	"github.com/simjay/plous/internal/ui/terminal"
)

var installedToolCalls = []struct{ name, args string }{
	{"kubernetes_resources_read", `{"purpose":"fixture","connection_id":"cluster","operation":"get","resource":"pods","namespace":"*"}`},
	{"kubernetes_logs_read", `{"purpose":"fixture","connection_id":"cluster","resource":"pods","namespace":"apps","name":"shop","container":"app"}`},
	{"kubernetes_usage_read", `{"purpose":"fixture","connection_id":"cluster","resource":"pods","namespace":"apps","name":"shop"}`},
	{"helm_releases_read", `{"purpose":"fixture","connection_id":"cluster/helm","operation":"list","namespace":"*"}`},
	{"helm_configuration_read", `{"purpose":"fixture","connection_id":"cluster/helm","operation":"values","namespace":"apps","name":"shop"}`},
	{"helm_diff_read", `{"purpose":"fixture","connection_id":"cluster/helm","namespace":"apps","name":"shop","revision":2,"compare_revision":1}`},
	{"argo_resources_read", `{"purpose":"fixture","connection_id":"cluster","operation":"get","resource":"applications.argoproj.io","namespace":"*"}`},
	{"flux_resources_read", `{"purpose":"fixture","connection_id":"cluster","operation":"get","resource":"kustomizations.kustomize.toolkit.fluxcd.io","namespace":"*"}`},
	{"source_files_read", `{"purpose":"fixture","connection_id":"source","operation":"list"}`},
	{"source_analysis_read", `{"purpose":"fixture","connection_id":"source","operation":"catalog"}`},
	{"workspace_read", `{"purpose":"fixture","operation":"workspace"}`},
	{"history_read", `{"purpose":"fixture","operation":"search","query":"releases"}`},
	{"ui_read", `{"purpose":"fixture"}`},
	{"kubernetes_show", `{"purpose":"fixture","view":"list","connection_id":"cluster","resource":"pods","namespace":"apps"}`},
	{"helm_show", `{"purpose":"fixture","connection_id":"cluster/helm","namespace":"apps","name":"shop","revision":2,"compare_revision":1}`},
	{"source_show", `{"purpose":"fixture","connection_id":"source","path":"README.md"}`},
	{"history_show", `{"purpose":"fixture","ref":"record:fixture:saved"}`},
}

type installedTools struct {
	generation int32
	calls      atomic.Int32
	listed     atomic.Int32
	received   sync.Map
}

func (f *installedTools) Definitions() []agent.Tool {
	f.listed.Add(1)
	return ai.ToolDefinitions(ai.Options{WorkspaceID: "fixture", UI: f, Connections: []ai.Connection{
		{ConnectionID: "cluster", Tool: "kubernetes"},
		{ConnectionID: "cluster/helm", Tool: "helm"},
		{ConnectionID: "source", Tool: "source"},
	}})
}

func (f *installedTools) Read(context.Context) (agent.UIResult, error) {
	return agent.UIResult{}, errors.New("client compatibility fixture does not render views")
}

func (f *installedTools) Show(context.Context, agent.ViewRequest) (agent.UIResult, error) {
	return agent.UIResult{}, errors.New("client compatibility fixture does not render views")
}

func (f *installedTools) Call(ctx context.Context, call agent.ToolCall, maxBytes int) (json.RawMessage, error) {
	for _, expected := range installedToolCalls {
		if call.Name != expected.name {
			continue
		}
		var got, want any
		if err := json.Unmarshal(call.Args, &got); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(expected.args), &want)
		if !reflect.DeepEqual(got, want) {
			return nil, fmt.Errorf("client changed %s arguments: %s", call.Name, call.Args)
		}
		if _, duplicate := f.received.LoadOrStore(call.Name, true); duplicate {
			return nil, fmt.Errorf("client repeated %s", call.Name)
		}
		f.calls.Add(1)
		return json.Marshal(map[string]string{"ref": fmt.Sprintf("record:fixture:%d:%s", f.generation, call.Name)})
	}
	return nil, fmt.Errorf("unexpected fixture tool: %s", call.Name)
}

func TestInstalledClientFixtureCoversCatalog(t *testing.T) {
	definitions := (&installedTools{}).Definitions()
	if len(definitions) != len(installedToolCalls) {
		t.Fatal("client fixture does not cover the full catalog")
	}
	for _, tool := range definitions {
		t.Run(tool.Name, func(t *testing.T) {
			for _, call := range installedToolCalls {
				if call.name != tool.Name {
					continue
				}
				var parameters, args any
				_ = json.Unmarshal(tool.Parameters, &parameters)
				_ = json.Unmarshal([]byte(call.args), &args)
				compiler := jsonschema.NewCompiler()
				if err := compiler.AddResource("urn:plous:client-test", parameters); err != nil {
					t.Fatal(err)
				}
				schema, err := compiler.Compile("urn:plous:client-test")
				if err != nil {
					t.Fatal(err)
				}
				if err := schema.Validate(args); err != nil {
					t.Fatal(err)
				}
				return
			}
			t.Fatal("missing client call fixture")
		})
	}
}

// Explicitly enabled local protocol checks use synthetic CLI homes and mock model APIs.
func TestInstalledNativeCLIWithMockProvider(t *testing.T) {
	selected := os.Getenv("PLOUS_TEST_INSTALLED_CLI")
	if selected == "" {
		t.Skip("set PLOUS_TEST_INSTALLED_CLI=codex or claude-code for installed binary protocol verification")
	}
	kind := agent.Kind(selected)
	namespace := "mcp__plous"
	wirePrefix := "mcp__plous__"
	if kind == agent.Codex {
		namespace = "mcp__plous_"
	}
	path, err := cli.Executable(kind)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	stateDirs := []string{t.TempDir(), t.TempDir()}
	lastGeneration := map[int]int32{}
	workdir := t.TempDir()
	workdir, _ = filepath.EvalSymlinks(workdir)
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, "codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, "claude"))
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")
	t.Setenv("OPENAI_API_KEY", "fixture-key")
	t.Setenv("ANTHROPIC_API_KEY", "fixture-key")
	expectedTools := map[string]bool{}
	for _, call := range installedToolCalls {
		expectedTools[call.name] = true
	}
	var unrelatedCalls atomic.Int32
	unrelated := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		unrelatedCalls.Add(1)
		http.Error(w, "unrelated server must not be contacted", http.StatusForbidden)
	}))
	defer unrelated.Close()
	var calls atomic.Int32
	var generation atomic.Int32
	var runnerSeen atomic.Bool
	streamed := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"models":[]}`)
			return
		}
		var request map[string]any
		body, _ := io.ReadAll(r.Body)
		if json.Unmarshal(body, &request) != nil {
			t.Errorf("invalid model request: %s", body)
			http.Error(w, "fixture decode", 400)
			return
		}
		if strings.HasSuffix(r.URL.Path, "count_tokens") {
			fmt.Fprint(w, `{"input_tokens":10}`)
			return
		}
		inventory, _ := request["tools"].([]any)
		input, _ := request["input"].([]any)
		for _, v := range input {
			item, _ := v.(map[string]any)
			if item["type"] == "additional_tools" {
				additional, _ := item["tools"].([]any)
				inventory = append(inventory, additional...)
			}
		}
		if len(inventory) == 0 && kind == agent.ClaudeCode && strings.Contains(string(body), "You are naming a coding session") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"fixture-title","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"Pod count"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":2}}`)
			return
		}
		if len(inventory) == 0 {
			t.Errorf("missing model tool inventory on %s: %.1800s", r.URL.Path, body)
			http.Error(w, "fixture tool inventory missing", http.StatusBadRequest)
			return
		}
		seenTools := map[string]bool{}
		hasRunner := false
		for i := 0; i < len(inventory); i++ {
			tool := inventory[i].(map[string]any)
			name, _ := tool["name"].(string)
			if tool["type"] == "namespace" && name == "functions" {
				nested, _ := tool["tools"].([]any)
				inventory = append(inventory, nested...)
				continue
			}
			if name == namespace {
				nested, _ := tool["tools"].([]any)
				for _, item := range nested {
					name, _ := item.(map[string]any)["name"].(string)
					if !expectedTools[name] {
						t.Errorf("unexpected Plous tool: %s", name)
					}
					seenTools[name] = true
				}
				continue
			}
			if kind == agent.Codex {
				if name == "exec" && tool["type"] == "custom" {
					hasRunner = true
					runnerSeen.Store(true)
					continue
				}
				if (name == "wait" || name == "request_user_input_async") && tool["type"] == "function" {
					continue
				}
			}
			if short := strings.TrimPrefix(name, wirePrefix); short != name && expectedTools[short] {
				seenTools[short] = true
				continue
			}
			if name != "request_user_input" && name != "EndConversation" && name != "list_mcp_resources" && name != "list_mcp_resource_templates" && name != "read_mcp_resource" {
				t.Errorf("unexpected model tool: %s %v", name, tool["type"])
			}
		}
		for name := range expectedTools {
			if !seenTools[name] {
				t.Errorf("Plous %s tool was not directly advertised to the model", name)
			}
		}
		n := calls.Add(1)
		partial := fmt.Sprintf("Session %d: Tools ", generation.Load())
		checkResults := func() {
			outputs := map[string]string{}
			if kind == agent.Codex {
				for _, value := range input {
					item, _ := value.(map[string]any)
					if item["type"] == "function_call_output" {
						id, _ := item["call_id"].(string)
						encoded, _ := json.Marshal(item["output"])
						outputs[id] = string(encoded)
					}
				}
			} else {
				messages, _ := request["messages"].([]any)
				for _, value := range messages {
					message, _ := value.(map[string]any)
					content, _ := message["content"].([]any)
					for _, value := range content {
						item, _ := value.(map[string]any)
						if item["type"] == "tool_result" {
							id, _ := item["tool_use_id"].(string)
							encoded, _ := json.Marshal(item["content"])
							outputs[id] = string(encoded)
						}
					}
				}
			}
			for _, call := range installedToolCalls {
				id := fmt.Sprintf("fixture-%d-%s", generation.Load(), call.name)
				ref := fmt.Sprintf("record:fixture:%d:%s", generation.Load(), call.name)
				if !strings.Contains(outputs[id], ref) {
					t.Errorf("client did not deliver current %s result for %s to the model", call.name, id)
				}
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		event := func(name string, data any) {
			raw, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, raw)
		}
		if kind == agent.Codex {
			event("response.created", map[string]any{"type": "response.created", "response": map[string]string{"id": fmt.Sprint("r", n)}})
			toolRound := int32(1)
			if hasRunner {
				toolRound++
				if n == 2 {
					denied := false
					for _, v := range input {
						item, _ := v.(map[string]any)
						denied = denied || item["type"] == "custom_tool_call_output" && item["output"] == "code-mode host is disabled"
					}
					if !denied {
						t.Error("Codex did not reject the advertised Code Mode runner")
					}
				}
			}
			if hasRunner && n == 1 {
				event("response.output_item.done", map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "custom_tool_call", "id": "probe", "call_id": "probe", "name": "exec", "namespace": "functions", "input": `text("runner executed")`}})
			} else if n == toolRound {
				for _, call := range installedToolCalls {
					event("response.output_item.done", map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "function_call", "id": fmt.Sprintf("fixture-%d-%s", generation.Load(), call.name), "call_id": fmt.Sprintf("fixture-%d-%s", generation.Load(), call.name), "name": call.name, "namespace": namespace, "arguments": call.args}})
				}
			} else {
				checkResults()
				event("response.output_item.added", map[string]any{"type": "response.output_item.added", "output_index": 0, "item": map[string]any{"type": "message", "id": "m1", "role": "assistant", "status": "in_progress", "content": []any{}}})
				event("response.content_part.added", map[string]any{"type": "response.content_part.added", "item_id": "m1", "output_index": 0, "content_index": 0, "part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}}})
				event("response.output_text.delta", map[string]any{"type": "response.output_text.delta", "item_id": "m1", "output_index": 0, "content_index": 0, "delta": partial})
				w.(http.Flusher).Flush()
				select {
				case <-streamed:
				case <-time.After(5 * time.Second):
					t.Error("Codex did not deliver partial text before response completion")
				}
				event("response.output_text.delta", map[string]any{"type": "response.output_text.delta", "item_id": "m1", "output_index": 0, "content_index": 0, "delta": "verified."})
				event("response.output_item.done", map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "message", "id": "m1", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": partial + "verified.", "annotations": []any{}}}}})
			}
			event("response.completed", map[string]any{"type": "response.completed", "response": map[string]any{"id": fmt.Sprint("r", n), "status": "completed", "usage": map[string]int{"input_tokens": 10, "output_tokens": 3, "total_tokens": 13}}})
		} else {
			event("message_start", map[string]any{"type": "message_start", "message": map[string]any{"id": fmt.Sprint("m", n), "type": "message", "role": "assistant", "model": "claude-sonnet-4-6", "content": []any{}, "stop_reason": nil, "usage": map[string]int{"input_tokens": 10, "output_tokens": 0}}})
			reason := "end_turn"
			if n == 1 {
				reason = "tool_use"
				for i, call := range installedToolCalls {
					event("content_block_start", map[string]any{"type": "content_block_start", "index": i, "content_block": map[string]any{"type": "tool_use", "id": fmt.Sprintf("fixture-%d-%s", generation.Load(), call.name), "name": wirePrefix + call.name, "input": map[string]any{}}})
					event("content_block_delta", map[string]any{"type": "content_block_delta", "index": i, "delta": map[string]string{"type": "input_json_delta", "partial_json": call.args}})
					event("content_block_stop", map[string]any{"type": "content_block_stop", "index": i})
				}
			} else {
				checkResults()
				event("content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]string{"type": "text", "text": ""}})
				event("content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]string{"type": "text_delta", "text": partial + "verified."}})
				event("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
			}
			event("message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": reason, "stop_sequence": nil}, "usage": map[string]int{"output_tokens": 3}})
			event("message_stop", map[string]string{"type": "message_stop"})
		}
	}))
	defer server.Close()
	if kind == agent.Codex {
		if err = os.MkdirAll(os.Getenv("CODEX_HOME"), 0700); err != nil {
			t.Fatal(err)
		}
		config := fmt.Sprintf("model = \"gpt-6.1-sol\"\nmodel_provider = \"fixture\"\n[model_providers.fixture]\nname = \"fixture\"\nbase_url = %q\nwire_api = \"responses\"\nrequires_openai_auth = false\n[features]\nenable_request_compression = false\n", server.URL+"/v1")
		config += fmt.Sprintf("[projects.%q]\ntrust_level = %q\n", workdir, "trusted")
		config += fmt.Sprintf("[mcp_servers.unrelated]\nurl = %q\n[mcp_servers.plous]\nurl = %q\nhttp_headers = { X-Unrelated = \"must-not-merge\" }\n", unrelated.URL, unrelated.URL)
		if err = os.WriteFile(filepath.Join(os.Getenv("CODEX_HOME"), "config.toml"), []byte(config), 0600); err != nil {
			t.Fatal(err)
		}
	} else {
		t.Setenv("ANTHROPIC_BASE_URL", server.URL)
		t.Setenv("ANTHROPIC_MODEL", "claude-sonnet-4-6")
		if err = os.MkdirAll(os.Getenv("CLAUDE_CONFIG_DIR"), 0700); err != nil {
			t.Fatal(err)
		}
		config, _ := json.Marshal(map[string]any{"hasCompletedOnboarding": true, "theme": "dark", "customApiKeyResponses": map[string]any{"approved": []string{"fixture-key"}}, "projects": map[string]any{workdir: map[string]any{"hasTrustDialogAccepted": true}}, "mcpServers": map[string]any{"unrelated": map[string]string{"type": "http", "url": unrelated.URL}}})
		for _, dir := range []string{home, os.Getenv("CLAUDE_CONFIG_DIR")} {
			if err = os.WriteFile(filepath.Join(dir, ".claude.json"), config, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, phase := range []struct {
		name   string
		resume bool
		empty  bool
		scope  int
	}{{"close-empty", false, true, 0}, {"reopen-empty", true, true, 0}, {"first-message", true, false, 0}, {"other-scope", false, false, 1}, {"resume-conversation", true, false, 0}} {
		generation.Add(1)
		partial := fmt.Sprintf("Session %d: Tools", generation.Load())
		calls.Store(0)
		runnerSeen.Store(false)
		select {
		case <-streamed:
		default:
		}
		if !t.Run(phase.name, func(t *testing.T) {
			tools := &installedTools{generation: generation.Load()}
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			// Match the assistant panel inside a 160-column Plous terminal.
			width := 36
			if value := os.Getenv("PLOUS_TEST_NATIVE_WIDTH"); value != "" {
				width, err = strconv.Atoi(value)
				if err != nil || width < 24 {
					t.Fatal("invalid native test width")
				}
			}
			screen := vt.NewSafeEmulator(width, 30)
			defer func() {
				if pipe, ok := screen.InputPipe().(io.Closer); ok {
					_ = pipe.Close()
				}
			}()
			input := &terminal.Input{Ctx: ctx, Chunks: make(chan []byte, 128)}
			go func() { _, _ = io.Copy(input, screen) }()
			var raw bytes.Buffer
			session, err := cli.StartTerminal(ctx, cli.TerminalOptions{Kind: kind, Path: path, Directory: workdir, StateDirectory: stateDirs[phase.scope], Instructions: "Exercise each exposed Plous tool once, then answer. Code execution is unavailable. Call directly exposed Plous tools.", ID: "native-fixture", Resume: phase.resume, Tools: tools, Width: width, Height: 30, Input: input, Output: io.MultiWriter(&raw, screen)})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { session.Close(); _ = session.Wait() }()
			done := make(chan struct{})
			go func() { _ = session.Wait(); close(done) }()
			sent := false
			loginSent := false
			trusted := false
			completed := false
			restored := phase.name != "resume-conversation"
			for ctx.Err() == nil {
				select {
				case <-done:
					text := ansi.Strip(raw.String())
					t.Fatalf("native exited before ready: %v; tool calls=%d, model calls=%d\n%s", session.Wait(), tools.calls.Load(), calls.Load(), text[max(0, len(text)-4000):])
				default:
				}
				view := screen.Render()
				if loginSent && strings.Contains(view, "Select login method:") {
					completed = true
					break
				}
				if strings.Contains(view, fmt.Sprintf("Session %d: Tools verified.", lastGeneration[phase.scope])) {
					restored = true
				}
				if strings.Contains(view, partial) {
					select {
					case streamed <- struct{}{}:
					default:
					}
				}
				if sent && tools.calls.Load() == int32(len(installedToolCalls)) && calls.Load() >= 2 && strings.Contains(view, partial+" verified.") {
					completed = true
					break
				}
				if !trusted && strings.Contains(view, "trust this folder") {
					screen.SendText("\x1b[B\r")
					trusted = true
					time.Sleep(500 * time.Millisecond)
					continue
				}
				ready := (kind == agent.Codex && tools.listed.Load() >= 1 || kind == agent.ClaudeCode && tools.listed.Load() >= 2) && (strings.Contains(view, "❯") || strings.Contains(view, "›"))
				if !sent && restored && ready && !strings.Contains(view, "trust this folder") {
					if phase.empty {
						if kind == agent.ClaudeCode && phase.name == "close-empty" {
							if !loginSent {
								screen.SendText("/login")
								screen.SendKey(uv.KeyPressEvent{Code: uv.KeyEnter})
								loginSent = true
							}
							continue
						}
						completed = true
						break
					}
					screen.Paste("Check the Plous tools.")
					time.Sleep(200 * time.Millisecond)
					screen.SendKey(uv.KeyPressEvent{Code: uv.KeyEnter})
					sent = true
				}
				time.Sleep(50 * time.Millisecond)
			}
			time.Sleep(100 * time.Millisecond)
			finalView := ansi.Strip(screen.Render())
			session.Close()
			err = session.Wait()
			wantCalls := int32(2)
			wantTools := int32(len(installedToolCalls))
			if phase.empty {
				wantCalls, wantTools = 0, 0
			}
			if runnerSeen.Load() {
				wantCalls++
			}
			if !completed || err != nil && !errors.Is(err, context.Canceled) || tools.calls.Load() != wantTools || calls.Load() != wantCalls {
				t.Fatalf("native result completed=%v err=%v tool calls=%d model calls=%d screen=\n%s", completed, err, tools.calls.Load(), calls.Load(), finalView)
			}
		}) {
			break
		}
		if !phase.empty {
			lastGeneration[phase.scope] = generation.Load()
		}
	}

	if unrelatedCalls.Load() != 0 {
		t.Fatalf("contacted unrelated MCP server %d times", unrelatedCalls.Load())
	}
}
