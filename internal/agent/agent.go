// Package agent connects Eino execution to Plous-owned records and read-only tools.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/kubernetes"
	"github.com/simjay/plous/internal/memory"
	"github.com/simjay/plous/internal/rlm"
	"github.com/simjay/plous/internal/workspace"
)

type KubernetesScope struct{ Path, Context, Namespace, ConnectionID string }
type Options struct {
	Store                               *workspace.Store
	WorkspaceID, ConversationID, TurnID string
	Config                              ai.Config
	Key                                 string
	System, Prompt                      string
	History                             []workspace.Turn
	Kubernetes                          *KubernetesScope
	Emit                                func(string) error
	Progress                            func(string)
	// Model is injectable for deterministic local tests; production resolves Config once.
	Model         model.ToolCallingChatModel
	contextWindow int
}

type operation struct {
	o         Options
	memory    *memory.Memory
	model     *boundedModel
	kube      *kubernetes.ReadExecutor
	mu        sync.Mutex
	toolCalls int
}

func Run(ctx context.Context, o Options) (result ai.Result, err error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if o.Kubernetes != nil {
		captured := *o.Kubernetes
		o.Kubernetes = &captured
	}
	if o.Store == nil {
		return result, errors.New("conversation storage is required")
	}
	if o.Emit == nil {
		o.Emit = func(string) error { return nil }
	}
	base := o.Model
	if base == nil {
		base, err = ai.NewChatModel(ctx, o.Config, o.Key)
		if err != nil {
			return result, err
		}
	}
	catalog, ok := ai.LookupModel(o.Config.Provider, o.Config.Model)
	if !ok {
		return result, errors.New("selected model unavailable")
	}
	capacity := catalog.ContextWindow
	if o.contextWindow > 0 {
		capacity = o.contextWindow
	}
	op := &operation{o: o, model: &boundedModel{base: base, state: &modelBudget{capacity: capacity}}}
	defer func() { result.Usage = op.model.usage() }()
	op.memory, err = memory.New(o.Store, o.WorkspaceID, o.ConversationID)
	if err != nil {
		return result, err
	}
	input, err := prepareContext(ctx, o, op.complete)
	if err != nil {
		return result, err
	}
	prompt, err := ai.SchemaMessages(input)
	if err != nil {
		return result, err
	}
	tools := op.tools()
	a, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{Name: "plous", Description: "Read-only workspace investigation", Model: op.model, MaxIterations: 12, ToolsConfig: adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{Tools: tools, ExecuteSequentially: true, UnknownToolsHandler: func(ctx context.Context, name, input string) (string, error) { return op.execute(ctx, name, input) }}}})
	if err != nil {
		return result, errors.New("could not initialize the assistant")
	}
	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: a, EnableStreaming: true})
	iter := runner.Run(ctx, prompt)
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			if errors.Is(event.Err, adk.ErrExceedMaxIterations) || errors.Is(event.Err, compose.ErrExceedMaxSteps) {
				return result, ai.UserError("assistant reached its iteration limit")
			}
			return result, ai.SafeError(ctx, event.Err)
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		output := event.Output.MessageOutput
		if output.Role != schema.Assistant {
			if output.IsStreaming {
				output.MessageStream.Close()
			}
			continue
		}
		var parts []*schema.Message
		text := ""
		receive := func(m *schema.Message) error {
			parts = append(parts, m)
			text += m.Content
			if len(text) > ai.MaxResponseBytes {
				return errors.New("assistant response exceeds the output limit")
			}
			if m.Content != "" {
				result.Text = op.redactStreaming(text)
				return o.Emit(result.Text)
			}
			return nil
		}
		if output.IsStreaming {
			s := output.MessageStream
			for {
				chunk, e := s.Recv()
				if e == io.EOF {
					break
				}
				if e != nil {
					s.Close()
					return result, ai.SafeError(ctx, e)
				}
				if e = receive(chunk); e != nil {
					s.Close()
					return result, e
				}
			}
			s.Close()
		} else if output.Message != nil {
			if err = receive(output.Message); err != nil {
				return result, err
			}
		}
		if len(parts) == 0 {
			continue
		}
		msg, e := schema.ConcatMessages(parts)
		if e != nil {
			return result, errors.New("invalid assistant response stream")
		}
		if len(msg.ToolCalls) > 0 {
			if _, e = op.trace(workspace.ReasoningEntry{Kind: "agent", Status: "completed", Content: msg.Content, Input: marshal(msg.ToolCalls)}); e != nil {
				return result, e
			}
			result.Text = ""
			if e = o.Emit(""); e != nil {
				return result, e
			}
			continue
		}
		if msg.ResponseMeta != nil {
			result.Finish = msg.ResponseMeta.FinishReason
		}
		if final := op.redact(msg.Content); final != result.Text {
			result.Text = final
			if e = o.Emit(final); e != nil {
				return result, e
			}
		}
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if strings.TrimSpace(result.Text) == "" || !ai.CompletedFinish(result.Finish) {
		return result, errors.New("assistant did not complete; partial output was saved")
	}
	return result, nil
}

func (op *operation) complete(ctx context.Context, system, prompt string) (string, error) {
	result, err := op.model.Generate(ctx, []*schema.Message{schema.SystemMessage(system), schema.UserMessage(prompt)})
	if err != nil {
		return "", ai.SafeError(ctx, err)
	}
	if result.ResponseMeta == nil || !ai.CompletedFinish(result.ResponseMeta.FinishReason) {
		return "", errors.New("internal model call did not complete")
	}
	return op.redact(result.Content), nil
}
func (op *operation) trace(entry workspace.ReasoningEntry) (workspace.ReasoningEntry, error) {
	entry.MessageID = op.o.TurnID
	entry.Model = op.o.Config
	entry.Content = op.redact(entry.Content)
	if len(entry.Input) > 0 {
		var input any
		if json.Unmarshal(entry.Input, &input) == nil {
			entry.Input = marshal(op.redactValue(input))
		} else {
			entry.Input = marshal(op.redact(string(entry.Input)))
		}
	}
	return op.o.Store.AppendReasoning(op.o.WorkspaceID, op.o.ConversationID, entry)
}
func (op *operation) redact(s string) string {
	if op.o.Key != "" {
		s = strings.ReplaceAll(s, op.o.Key, "[redacted]")
	}
	return s
}

// Keep a possible credential prefix out of immutable streaming checkpoints until
// the following chunk establishes whether it is part of the credential.
func (op *operation) redactStreaming(s string) string {
	s = op.redact(s)
	key := op.o.Key
	for n := min(len(s), len(key)-1); n > 0; n-- {
		if strings.HasSuffix(s, key[:n]) {
			return s[:len(s)-n]
		}
	}
	return s
}
func (op *operation) redactValue(value any) any {
	switch v := value.(type) {
	case string:
		return op.redact(v)
	case []any:
		for i := range v {
			v[i] = op.redactValue(v[i])
		}
	case map[string]any:
		for k, x := range v {
			v[k] = op.redactValue(x)
		}
	}
	return value
}

func marshal(v any) json.RawMessage { b, _ := json.Marshal(v); return b }

func (op *operation) execute(ctx context.Context, name, input string) (string, error) {
	op.mu.Lock()
	op.toolCalls++
	count := op.toolCalls
	op.mu.Unlock()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if count > 32 {
		return "", ai.UserError("operation reached its tool-call limit")
	}
	if len(input) > 32768 {
		return "", ai.UserError("tool arguments exceed the size limit")
	}
	start, e := op.trace(workspace.ReasoningEntry{Kind: "tool", Tool: name, Status: "started", ParentSource: rlm.CallParent(ctx), Input: json.RawMessage(input)})
	if e != nil {
		return "", e
	}
	if op.o.Progress != nil {
		op.o.Progress("Reading " + name)
	}
	var value any
	var err error
	switch name {
	case "memory":
		value, err = op.readMemory(ctx, input)
	case "kubernetes_read":
		value, err = op.readKubernetes(ctx, input)
	case "investigate":
		var q struct {
			Question string `json:"question"`
		}
		err = decode(input, &q)
		if err == nil && strings.TrimSpace(q.Question) == "" {
			err = errors.New("investigation question is required")
		}
		if err == nil {
			runtime := rlm.Runtime{Complete: op.complete, Read: func(ctx context.Context, s string) (string, error) { return op.execute(ctx, "memory", s) }, Call: func(ctx context.Context, s string) (string, error) { return op.execute(ctx, "kubernetes_read", s) }, Instructions: toolHelp, Trace: func(ctx context.Context, event rlm.Event) error {
				// Tool results are already recorded by the shared executor.
				if event.Kind == "read" || event.Kind == "call" {
					return nil
				}
				parent := start.ID
				status := "completed"
				content := event.Output
				if event.Error != "" {
					status = "error"
					content = event.Error
				}
				_, e := op.trace(workspace.ReasoningEntry{Kind: "investigation", ParentID: parent, Tool: event.Kind, Status: status, Input: marshal(event.Input), Content: content, Source: event.ID, ParentSource: event.ParentID})
				return e
			}}
			value, err = runtime.Investigate(ctx, q.Question)
		}
	default:
		err = errors.New("unsupported tool; only registered read tools are available")
	}
	status := "completed"
	content := ""
	if err != nil {
		status = "denied"
		content = op.redact(err.Error())
		value = map[string]string{"error": content}
	} else {
		content = string(marshal(value))
	}
	if len(content) > 128*1024 {
		status = "error"
		content = "tool result exceeded the output limit"
		value = map[string]string{"error": content}
	}
	traceContent := content
	if name == "memory" && err == nil {
		switch result := value.(type) {
		case memory.Page:
			refs := make([]string, 0, len(result.Records))
			for _, r := range result.Records {
				refs = append(refs, r.Ref)
			}
			traceContent = string(marshal(map[string]any{"refs": refs, "next_cursor": result.NextCursor, "truncated": result.Truncated}))
		case memory.Chunk:
			traceContent = string(marshal(map[string]any{"ref": result.Ref, "next_offset": result.NextOffset, "total_bytes": result.TotalBytes, "truncated": result.Truncated}))
		}
	}
	record, e := op.trace(workspace.ReasoningEntry{Kind: "tool", ParentID: start.ID, Tool: name, Status: status, Content: traceContent})
	if e != nil {
		return "", e
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	return op.redact(string(marshal(map[string]any{"evidence": record.ID, "result": value}))), nil
}

type memoryCommand struct {
	Action         string `json:"action"`
	Source         string `json:"source,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	Text           string `json:"text,omitempty"`
	Cursor         string `json:"cursor,omitempty"`
	Ref            string `json:"ref,omitempty"`
	Offset         int    `json:"offset,omitempty"`
	Limit          int    `json:"limit,omitempty"`
}

func (op *operation) readMemory(ctx context.Context, input string) (any, error) {
	var c memoryCommand
	if err := decode(input, &c); err != nil {
		return nil, err
	}
	switch c.Action {
	case "search":
		if c.Ref != "" || c.Offset != 0 {
			return nil, errors.New("search does not accept ref or offset")
		}
		return op.memory.Search(ctx, memory.Query{Source: c.Source, ConversationID: c.ConversationID, Text: c.Text, Cursor: c.Cursor, Limit: c.Limit})
	case "read":
		if c.Source != "" || c.Text != "" || c.Cursor != "" || c.ConversationID != "" {
			return nil, errors.New("read accepts only ref, offset, and limit")
		}
		return op.memory.Read(ctx, c.Ref, c.Offset, c.Limit)
	default:
		return nil, errors.New("unsupported memory operation; use search or read")
	}
}
func (op *operation) readKubernetes(ctx context.Context, input string) (any, error) {
	command, err := kubernetes.DecodeCommand(input)
	if err != nil {
		return nil, err
	}
	if op.o.Kubernetes == nil {
		return nil, errors.New("select a Kubernetes connection before reading cluster resources")
	}
	captured := op.o.Kubernetes
	scope := kubernetes.CommandScope{ConnectionID: captured.ConnectionID, Context: captured.Context, Namespace: captured.Namespace}
	// Validate operation and arguments before loading a connection. Resolve an
	// unset namespace from kubeconfig, then enforce it again in the executor.
	preliminary := scope
	if preliminary.Namespace == "" {
		preliminary.Namespace = command.Namespace
		if preliminary.Namespace == "" {
			preliminary.Namespace = "default"
		}
	}
	if err = kubernetes.ValidateRead(command, preliminary); err != nil {
		return nil, err
	}
	if op.kube == nil {
		client, e := kubernetes.New(captured.Path, captured.Context, captured.Namespace)
		if e != nil {
			return nil, errors.New("could not open the selected Kubernetes connection")
		}
		if scope.Namespace == "" {
			scope.Namespace = client.Namespace
		}
		op.kube, e = kubernetes.NewReadExecutor(client, scope)
		if e != nil {
			return nil, e
		}
	}

	return op.kube.Execute(ctx, command)
}
func decode(input string, v any) error {
	d := json.NewDecoder(strings.NewReader(input))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return errors.New("invalid or unsupported tool arguments")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("tool input must be one JSON object")
	}
	return nil
}

const toolHelp = `Plous tool response format:
Every read(...) and call(...) returns {"evidence":"execution-record-id","result":{...}}. Unwrap response["result"] before accessing tool data. The records, text, next_cursor, and tool error fields are INSIDE result, never at the top level. An executed tool denial is {"evidence":"...","result":{"error":"reason"}}; check for error before using its data.

Memory search example:
response = read({"action":"search","source":"reasoning","text":"query","limit":10})
page = response["result"]
print(page)
for record in page.get("records", []):
    detail = read({"action":"read","ref":record["ref"],"offset":0,"limit":4096})
    print(detail["result"])

Search sources are conversation, reasoning, activity, or workspace. Each record has ref, text, and truncated. Read returns result.text, result.next_offset, result.total_bytes, and result.truncated. A read's text is the saved JSONL record as a string, not a decoded record object. Pass that string as selected evidence to ask(question, evidence=detail["result"]["text"]). Search pagination uses result.next_cursor as the next request's cursor; read pagination uses result.next_offset as offset. Do not conclude evidence is missing while another page is available.

Search text is an excerpt, not a record title. Inspect matching records before selecting evidence. A kind=request record repeats a question; it is not an observation answering that question. Use print to inspect unfamiliar results before writing the next program.

After selecting and reading a relevant record, delegate and preserve flat evidence references:
child = ask("Check the selected observation", evidence=detail["result"]["text"])
refs = [record["ref"], detail["evidence"]]
for ref in child["evidence"]:
    refs.append(ref)
finish(child["findings"], evidence=refs)

call({"operation":"get|describe|logs","resource":"pods","name":"optional","namespace":"captured namespace","limit":20,"max_bytes":32768}) performs an approved Kubernetes read. For logs supply name and required container and optional tail_lines. Use returned evidence IDs and record refs in finish(). No shell, raw paths, writes, or imports.`

func (op *operation) tools() []tool.BaseTool {
	str := func(desc string, required bool) *schema.ParameterInfo {
		return &schema.ParameterInfo{Type: schema.String, Desc: desc, Required: required}
	}
	integer := func() *schema.ParameterInfo { return &schema.ParameterInfo{Type: schema.Integer} }
	memoryParams := map[string]*schema.ParameterInfo{"action": {Type: schema.String, Enum: []string{"search", "read"}, Required: true}, "source": {Type: schema.String, Enum: []string{"conversation", "reasoning", "activity", "workspace"}}, "text": str("literal search text", false), "conversation_id": str("optional conversation in this workspace", false), "cursor": str("next cursor from a prior search", false), "ref": str("record ref returned by search", false), "offset": integer(), "limit": integer()}
	var operations []string
	for _, definition := range kubernetes.ReadCommands() {
		operations = append(operations, definition.Operation)
	}
	kubeParams := map[string]*schema.ParameterInfo{"operation": {Type: schema.String, Enum: operations, Required: true}, "resource": str("full resource API identity, such as pods, deployments.apps, events", true), "name": str("resource name; omit for a list", false), "namespace": str("captured namespace", false), "container": str("pod container for logs", false), "label_selector": str("Kubernetes label selector", false), "field_selector": str("Kubernetes field selector", false), "continue": str("server pagination cursor", false), "limit": integer(), "max_bytes": integer(), "tail_lines": integer(), "since_seconds": integer(), "previous": {Type: schema.Boolean}}
	defs := []struct {
		name, desc string
		params     map[string]*schema.ParameterInfo
	}{{"memory", "Search and read saved Q&A, investigation results, workspace metadata and Activity. Results have refs; use read to fetch excerpts. Only this workspace is visible.", memoryParams}, {"investigate", "Explore larger histories using a bounded programmatic investigator and recursive model calls. Returns findings and evidence references.", map[string]*schema.ParameterInfo{"question": str("specific investigation question", true)}}}
	if op.o.Kubernetes != nil {
		defs = append(defs, struct {
			name, desc string
			params     map[string]*schema.ParameterInfo
		}{"kubernetes_read", "Read current resources, events, and bounded pod logs in the captured connection. Only get, describe, logs are supported. Writes, raw requests, Secret resources and shell commands are rejected.", kubeParams})
	}
	out := make([]tool.BaseTool, 0, len(defs))
	for _, d := range defs {
		out = append(out, &readTool{info: &schema.ToolInfo{Name: d.name, Desc: d.desc, ParamsOneOf: schema.NewParamsOneOfByParams(d.params)}, run: op.execute})
	}
	return out
}

type readTool struct {
	info *schema.ToolInfo
	run  func(context.Context, string, string) (string, error)
}

func (t *readTool) Info(context.Context) (*schema.ToolInfo, error) { return t.info, nil }
func (t *readTool) InvokableRun(ctx context.Context, input string, _ ...tool.Option) (string, error) {
	return t.run(ctx, t.info.Name, input)
}

// One shared counter covers outer turns, compaction, and all recursive model calls.
type modelBudget struct {
	sync.Mutex
	capacity, calls, tokens int
	usage                   ai.Usage
}
type boundedModel struct {
	base  model.ToolCallingChatModel
	state *modelBudget
}

func (m *boundedModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	b, e := m.base.WithTools(tools)
	if e != nil {
		return nil, e
	}
	return &boundedModel{base: b, state: m.state}, nil
}
func (m *boundedModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	s, e := m.Stream(ctx, input, opts...)
	if e != nil {
		return nil, e
	}
	defer s.Close()
	var chunks []*schema.Message
	for {
		c, e := s.Recv()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, e
		}
		chunks = append(chunks, c)
	}
	if len(chunks) == 0 {
		return nil, errors.New("empty internal model response")
	}
	return schema.ConcatMessages(chunks)
}
func (m *boundedModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	tokens := 0
	for _, msg := range input {
		tokens += estimateText(msg.Content) + 4
		for _, t := range msg.ToolCalls {
			tokens += estimateText(t.Function.Arguments) + 16
		}
	}
	m.state.Lock()
	m.state.calls++
	m.state.tokens += tokens
	allowed := m.state.calls <= 24 && tokens <= InputBudget(m.state.capacity) && m.state.tokens <= 4*m.state.capacity
	m.state.Unlock()
	if !allowed {
		return nil, ai.UserError("assistant reached its model-call or context budget")
	}
	s, e := m.base.Stream(ctx, input, opts...)
	if e != nil {
		return nil, ai.SafeError(ctx, e)
	}
	size := 0
	var observed ai.Usage
	estimatedOutput := 0
	return schema.StreamReaderWithConvert(s, func(msg *schema.Message) (*schema.Message, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		size += len(msg.Content)
		for _, t := range msg.ToolCalls {
			size += len(t.Function.Arguments)
		}
		if size > ai.MaxResponseBytes {
			return nil, ai.UserError("model output exceeds the size limit")
		}
		m.state.Lock()
		outputTokens := (size + 3) / 4
		m.state.tokens += outputTokens - estimatedOutput
		estimatedOutput = outputTokens
		withinBudget := m.state.tokens <= 4*m.state.capacity
		if msg.ResponseMeta != nil && msg.ResponseMeta.Usage != nil {
			u := msg.ResponseMeta.Usage
			next := ai.Usage{Input: max(observed.Input, int64(u.PromptTokens)), Output: max(observed.Output, int64(u.CompletionTokens)), Total: max(observed.Total, int64(u.TotalTokens))}
			m.state.usage.Input += next.Input - observed.Input
			m.state.usage.Output += next.Output - observed.Output
			m.state.usage.Total += next.Total - observed.Total
			observed = next
		}
		m.state.Unlock()
		if !withinBudget {
			return nil, ai.UserError("assistant reached its shared token budget")
		}

		return msg, nil
	}), nil
}
func (m *boundedModel) usage() ai.Usage { m.state.Lock(); defer m.state.Unlock(); return m.state.usage }
