// Package ai runs the selected agent over projected tools and scoped history.
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/cloudwego/eino/components/model"
	"github.com/simjay/plous/internal/ai/agent"
	journal "github.com/simjay/plous/internal/ai/history"
	aimodel "github.com/simjay/plous/internal/ai/model"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/source"
	"github.com/simjay/plous/internal/workspace"
)

type Options struct {
	Agent                               agent.Kind
	UI                                  agent.UI
	Store                               *workspace.Store
	History                             *journal.Store
	WorkspaceID, ConversationID, TurnID string
	Config                              aimodel.Config
	Key, System, Prompt, ScopeContext   string
	Connections                         []Connection
	Emit                                func(string) error
	Progress                            func(string)
	ContextUsage                        func(int, int)
	Observe                             func(Observation) error
	Model                               model.ToolCallingChatModel
	Runtime                             *RuntimeConfig
	contextWindow                       int
}

type operation struct {
	callSequence           atomic.Uint64
	o                      Options
	model                  model.ToolCallingChatModel
	records                *workspace.Reader
	kube                   map[string]*kubernetes.Projector
	helm                   map[string]*helmtool.Projector
	mu                     sync.Mutex
	usage                  aimodel.Usage
	usageReports           int
	runtime                RuntimeConfig
	capacity               int
	calls, spent, reserved int
	historyAccess          *journal.Access
	sourceReads            map[string]*source.ReadSession
	deliveryBytes          int
	keys                   []string
}

func Run(ctx context.Context, o Options) (aimodel.Result, error) {
	if o.Store == nil || o.History == nil {
		return aimodel.Result{}, errors.New("conversation storage is required")
	}
	o.Connections = append([]Connection(nil), o.Connections...)
	if o.WorkspaceID == "" {
		o.Connections = nil
		o.ScopeContext = ""
	}
	if o.Emit == nil {
		o.Emit = func(string) error { return nil }
	}
	op := &operation{o: o, model: o.Model, keys: []string{o.Key}}
	var err error
	op.runtime, err = loadRuntimeConfig(o.Store, o.Runtime)
	if err != nil {
		return aimodel.Result{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, op.runtime.Timeout())
	defer cancel()
	if o.Agent.Effective() != agent.Plous {
		return aimodel.Result{}, errors.New("external agents run in the native terminal panel")
	}
	catalog, ok := aimodel.LookupModel(o.Config.Provider, o.Config.Model)
	if !ok {
		return aimodel.Result{}, errors.New("selected model unavailable")
	}
	op.capacity = catalog.ContextWindow
	if o.contextWindow > 0 {
		op.capacity = o.contextWindow
	}
	if op.model == nil {
		op.model, err = aimodel.NewChatModel(ctx, o.Config, o.Key)
	}
	if err != nil {
		return aimodel.Result{}, err
	}
	op.records, err = workspace.NewReader(o.Store, o.WorkspaceID)
	if err != nil {
		return aimodel.Result{}, err
	}
	return op.run(ctx)
}

func marshal(v any) json.RawMessage { b, _ := json.Marshal(v); return b }

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

func (op *operation) redact(s string) string {
	for _, key := range op.keys {
		if key != "" {
			s = strings.ReplaceAll(s, key, "[redacted]")
		}
	}
	return s
}

var errReadDenied = errors.New("read denied")

func denyRead(err error) error { return errors.Join(errReadDenied, err) }

// Redact string values without changing numeric facts or corrupting JSON escapes.
func (op *operation) redactJSON(raw json.RawMessage) (json.RawMessage, error) {
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.UseNumber()
	var value any
	if err := d.Decode(&value); err != nil {
		return nil, err
	}
	var visit func(any) any
	visit = func(v any) any {
		switch x := v.(type) {
		case string:
			return op.redact(x)
		case []any:
			for i := range x {
				x[i] = visit(x[i])
			}
		case map[string]any:
			out := make(map[string]any, len(x))
			for key, child := range x {
				out[op.redact(key)] = visit(child)
			}
			return out
		}
		return v
	}
	return json.Marshal(visit(value))
}
