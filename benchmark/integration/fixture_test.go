package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bench "github.com/simjay/plous/benchmark"
	"github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/ai/history"
	"github.com/simjay/plous/internal/integrations/observation"
	"github.com/simjay/plous/internal/source"
	"github.com/simjay/plous/internal/workspace"
)

type evidenceTurn struct{ conversation, turn string }
type fixture struct {
	ui                          *fixtureUI
	nextRoutes                  []bench.Route
	root, sourceRoot, transport string
	options                     ai.Options
	session                     *ai.ExternalTools
	mcp                         *mcpTools
	engine                      *bench.Engine
	tools                       agent.Tools
	variables                   map[string]any
	turns                       []evidenceTurn
	saved                       map[string]string
	cancel                      context.CancelFunc
	closeServer                 func()
}

func prepare(t testing.TB, c bench.Scenario, transport string) *fixture {
	t.Helper()
	data, err := bench.LoadFixture(filepath.Join("..", "fixtures", c.Fixture+".json"))
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{root: t.TempDir(), nextRoutes: data.NextRoutes, transport: transport, variables: map[string]any{}, saved: map[string]string{}}
	t.Cleanup(func() {
		if err := f.close(); err != nil {
			t.Error(err)
		}
	})
	store, err := workspace.Open(filepath.Join(f.root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	f.options.Store = store
	h, err := history.Open(store)
	if err != nil {
		t.Fatal(err)
	}
	f.options.History = h
	f.options.Agent = agent.ClaudeCode
	runtime := ai.DefaultRuntimeConfig()
	f.options.Runtime = &runtime
	if data.Scope != "general" {
		connection := workspace.Connection{ID: "cluster", Tool: "kubernetes", Name: "Fixture cluster", Settings: map[string]string{"context": "fixture", "namespace": "apps"}}
		if data.Scope == "cluster" {
			engine, server, kubeconfig, err := bench.Serve(f.root, data.Routes)
			if err != nil {
				t.Fatal(err)
			}
			f.engine, f.closeServer = engine, server.Close
			connection.Settings["path"] = kubeconfig
			f.options.Connections = ai.ClusterConnections(ai.Connection{ConnectionID: "cluster", Tool: "kubernetes", Context: "fixture", Namespace: "apps", Path: kubeconfig})
		} else {
			f.sourceRoot = filepath.Join(f.root, "source")
			if err = os.MkdirAll(f.sourceRoot, 0700); err != nil {
				t.Fatal(err)
			}
			f.sourceRoot, err = filepath.EvalSymlinks(f.sourceRoot)
			if err != nil {
				t.Fatal(err)
			}
			for path, text := range data.Files {
				full := filepath.Join(f.sourceRoot, path)
				if err = os.MkdirAll(filepath.Dir(full), 0700); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(full, []byte(text), 0600); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := source.Load(t.Context(), source.Config{Path: f.sourceRoot})
			if err != nil {
				t.Fatal(err)
			}
			inventory := source.Discover(snapshot)
			rendered, err := (source.Renderer{}).Render(t.Context(), inventory, "plain:deployment.yaml", source.Inputs{})
			if err != nil {
				t.Fatal(err)
			}
			f.variables["resource"] = rendered.Resources[0].Key()
			connection = workspace.Connection{ID: "source", Tool: "code", Name: "Fixture source", Settings: map[string]string{"path": f.sourceRoot}}
			f.options.Connections = []ai.Connection{{ConnectionID: "source", Tool: "source", Path: f.sourceRoot, SourceView: source.CaptureReadView(inventory, map[string]*source.Result{"plain:deployment.yaml": rendered}, nil, nil)}}
		}
		w, err := store.Save(workspace.Workspace{Name: "PlousBench", Connections: []workspace.Connection{connection}}, "fixture")
		if err != nil {
			t.Fatal(err)
		}
		f.options.WorkspaceID = workspace.ConnectionScopeID(w.ID, connection.ID)
		if err = store.EnsureScope(workspace.Scope{ID: f.options.WorkspaceID, Kind: "connection", WorkspaceID: w.ID, Connection: connection}); err != nil {
			t.Fatal(err)
		}
		other := workspace.Connection{ID: "other", Tool: "kubernetes", Name: "Private fixture", Settings: map[string]string{"context": "other"}}
		otherScope := workspace.ConnectionScopeID(w.ID, other.ID)
		if err = store.EnsureScope(workspace.Scope{ID: otherScope, Kind: "connection", WorkspaceID: w.ID, Connection: other}); err != nil {
			t.Fatal(err)
		}
		f.variables["foreign"] = seed(t, h, otherScope, 0, "Private scope approval")
	}
	f.variables["correction"] = seed(t, h, f.options.WorkspaceID, c.HistorySize, "Correction: release-approval is for staging only.")
	if strings.HasPrefix(c.ID, "ui-") {
		f.ui = &fixtureUI{fixture: f, state: agent.UIResult{Status: "ready", Focus: "assistant", Visible: true, View: &agent.ViewRequest{View: "overview"}}}
		f.options.UI = f.ui
	}
	if err = f.open(t.Context()); err != nil {
		t.Fatal(err)
	}
	return f
}

func seed(t testing.TB, h *history.Store, scope string, size int, correction string) string {
	t.Helper()
	c, err := h.NewConversation(scope, "Fixture decisions")
	if err != nil {
		t.Fatal(err)
	}
	write := func(prompt, answer, status string) {
		turn, err := h.StartTurn(scope, c.ID, history.Request{Backend: agent.ClaudeCode, Prompt: prompt})
		if err != nil {
			t.Fatal(err)
		}
		if err = h.SaveResponse(scope, c.ID, turn.Request.ID, history.Response{Status: status, Text: answer}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < size; i++ {
		write(fmt.Sprintf("Background item %d", i), "Routine fixture entry.", "completed")
	}
	write("release-approval decision?", "release-approval covers production.", "completed")
	write("Correct release-approval.", correction, "completed")
	turns, err := h.Turns(scope, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	ref := turns[len(turns)-1].AnswerRef
	index, err := h.SearchIndex(scope, c.ID, history.SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err = index.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}
	return ref
}

func (f *fixture) open(ctx context.Context) error {
	ctx, f.cancel = context.WithCancel(ctx)
	s, err := ai.OpenExternalTools(ctx, f.options)
	if err != nil {
		return err
	}
	f.session, f.tools = s, s
	conversations, err := f.options.History.Conversations(f.options.WorkspaceID)
	if err != nil {
		return err
	}
	found := false
	for _, c := range conversations {
		turns, err := f.options.History.Turns(f.options.WorkspaceID, c.ID)
		if err != nil {
			return err
		}
		for _, turn := range turns {
			if turn.Request.ID == s.ID {
				f.turns = append(f.turns, evidenceTurn{c.ID, s.ID})
				found = true
			}
		}
	}
	if !found {
		return errors.New("tool session has no evidence journal")
	}
	index, err := f.options.History.SearchIndex(f.options.WorkspaceID, f.turns[len(f.turns)-1].conversation, history.SearchOptions{})
	if err != nil {
		return err
	}
	if err = index.Sync(ctx); err != nil {
		return err
	}
	if f.transport == "mcp" {
		f.mcp, err = openMCP(ctx, s, s.ID)
		if err != nil {
			return err
		}
		f.tools = f.mcp
	}
	return nil
}

func (f *fixture) closeSession() error {
	if f.mcp != nil {
		f.mcp.Close()
		f.mcp = nil
	}
	if f.session != nil {
		err := f.session.Close()
		f.session = nil
		if f.cancel != nil {
			f.cancel()
		}
		return err
	}
	return nil
}

func (f *fixture) close() error {
	err := f.closeSession()
	if f.options.History != nil {
		err = errors.Join(err, f.options.History.Close())
		f.options.History = nil
	}
	if f.options.Store != nil {
		err = errors.Join(err, f.options.Store.Close())
		f.options.Store = nil
	}
	if f.closeServer != nil {
		f.closeServer()
		f.closeServer = nil
	}
	return err
}

func (f *fixture) evidence(ctx context.Context) (map[string]history.ReasoningEntry, error) {
	result := map[string]history.ReasoningEntry{}
	for _, turn := range f.turns {
		rows, err := f.options.History.TurnEvidence(ctx, f.options.WorkspaceID, turn.conversation, turn.turn)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.Projection != nil && row.Projection.Validate() != nil || row.Projection == nil && row.Tool != "history_read" {
				return nil, errors.New("invalid persisted evidence")
			}
			result[row.Ref] = row
		}
	}
	return result, nil
}

func (f *fixture) verify(ctx context.Context, step bench.Step, body json.RawMessage, callErr error) error {
	rows, err := f.evidence(ctx)
	if err != nil {
		return err
	}
	for ref, row := range rows {
		f.saved[ref] = string(bench.JSON(row))
	}
	if f.engine != nil {
		for _, access := range f.engine.Accesses() {
			if !access.Matched || access.Method != "GET" {
				return fmt.Errorf("unexpected fixture access: %+v", access)
			}
		}
	}
	if callErr != nil {
		return nil
	}
	var delivery observation.Delivery
	if err = json.Unmarshal(body, &delivery); err != nil {
		return err
	}
	if delivery.Observation == nil {
		return nil
	}
	saved, ok := rows[delivery.Ref]
	if !ok || delivery.Page == nil {
		return errors.New("tool returned evidence before saving it")
	}
	expected, err := saved.Projection.Deliver(delivery.Ref, observation.Selection{Offset: delivery.Page.Offset, Limit: delivery.Page.Returned, Fields: delivery.Page.Fields}, 65536)
	if err != nil || string(bench.JSON(expected)) != string(bench.JSON(delivery)) {
		return fmt.Errorf("delivered facts differ from saved evidence: %v", err)
	}
	return nil
}

func (f *fixture) restart(ctx context.Context) error {
	if err := f.closeSession(); err != nil {
		return err
	}
	if err := f.options.History.Close(); err != nil {
		return err
	}
	if err := f.options.Store.Close(); err != nil {
		return err
	}
	store, err := workspace.Open(filepath.Join(f.root, "profile"))
	if err != nil {
		return err
	}
	f.options.Store = store
	f.options.History, err = history.Open(store)
	if err != nil {
		return err
	}
	rows, err := f.evidence(ctx)
	if err != nil {
		return err
	}
	if len(rows) != len(f.saved) {
		return errors.New("restart changed the number of saved observations")
	}
	for ref, want := range f.saved {
		if string(bench.JSON(rows[ref])) != want {
			return fmt.Errorf("restart changed saved evidence %s", ref)
		}
	}
	return f.open(ctx)
}

func (f *fixture) before(ctx context.Context, step bench.Step) error {
	switch step.Before {
	case "":
		return nil
	case "ui-block":
		f.ui.blocked = true
		return nil
	case "ui-unblock":
		f.ui.blocked = false
		return nil
	case "restart":
		return f.restart(ctx)
	case "advance":
		if len(f.nextRoutes) == 0 || f.engine == nil {
			return errors.New("fixture has no next frame")
		}
		f.engine.Advance(f.nextRoutes)
		return nil
	case "remove-source":
		if f.sourceRoot == "" {
			return errors.New("fixture has no source root")
		}
		if err := os.RemoveAll(f.sourceRoot); err != nil {
			return err
		}
		return f.restart(ctx)
	case "close":
		return f.session.Close()
	case "cancel":
		f.cancel()
		return nil
	default:
		return fmt.Errorf("unknown fixture transition %q", step.Before)
	}
}

// The indirection follows the new session after a restart without changing the runner.
func (f *fixture) Definitions() []agent.Tool { return f.tools.Definitions() }
func (f *fixture) Call(ctx context.Context, c agent.ToolCall, n int) (json.RawMessage, error) {
	return f.tools.Call(ctx, c, n)
}
func (f *fixture) reads() int {
	if f.engine == nil {
		return 0
	}
	return len(f.engine.Accesses())
}

func execute(t testing.TB, c bench.Scenario, transport string) bench.Result {
	t.Helper()
	f := prepare(t, c, transport)
	r := bench.Run(t.Context(), c, transport, bench.Harness{Tools: f, Variables: f.variables, Reads: f.reads, Before: f.before, Verify: f.verify, Display: f.display})
	if err := f.restart(t.Context()); err != nil {
		r.Failures = append(r.Failures, err.Error())
	}
	if err := f.close(); err != nil {
		r.Failures = append(r.Failures, err.Error())
	}
	return r
}
