package debugging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/ai/cli"
	"github.com/simjay/plous/internal/ai/history"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/workspace"
	"github.com/simjay/plous/skills"
)

const contextTool = "kubernetes_context_read"

type standaloneTools struct {
	ctx     context.Context
	config  configuration
	store   *workspace.Store
	archive *history.Store
	gate    chan struct{}
	target  *kubecommon.Target
	session *ai.ExternalTools
}

func serve(ctx context.Context, c configuration, input io.ReadCloser, output, diagnostics io.Writer, version string) (err error) {
	if err = os.MkdirAll(c.dataDir, 0700); err != nil {
		return err
	}
	root, err := os.MkdirTemp(c.dataDir, "session-")
	if err != nil {
		return err
	}
	store, err := workspace.Open(root)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, store.Close()) }()
	archive, err := history.Open(store)
	if err != nil {
		return err
	}
	tools := &standaloneTools{ctx: ctx, config: c, store: store, archive: archive, gate: make(chan struct{}, 1)}
	defer func() {
		if tools.session != nil {
			err = errors.Join(err, tools.session.Close())
		}
	}()
	mode := c.context
	if mode == "" {
		mode = "follows kubeconfig current context"
	}
	fmt.Fprintln(diagnostics, "Plous MCP:", mode, "· Evidence:", root)
	instructions := skills.DebugInstructions() + "\nCall kubernetes_context_read to discover the current target and connection IDs. Without an explicit context, a user's kubeconfig context switch takes effect on the next call. Confirm the returned context before acting on results from another context. History is limited to the captured context in this process."
	return cli.ServeStdio(ctx, input, output, tools, filepath.Base(root), version, instructions)
}

func (s *standaloneTools) Definitions() []agent.Tool {
	// Clients can cache schemas. Advertise the configured capabilities without
	// enumerating IDs that could go stale after a kubeconfig context switch.
	return append([]agent.Tool{{
		Name:        contextTool,
		Description: "Read the current kubeconfig context, namespace, and connection IDs. Refresh before starting an investigation or after a context switch.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}}, ai.ToolDefinitions(ai.Options{
		WorkspaceID: "standalone",
		Connections: []ai.Connection{{Tool: "kubernetes"}, {Tool: "helm"}},
		Debug:       s.config.policy,
	})...)
}

func (s *standaloneTools) Call(ctx context.Context, call agent.ToolCall, maxBytes int) (json.RawMessage, error) {
	select {
	case s.gate <- struct{}{}:
		defer func() { <-s.gate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	target := s.target
	if target == nil || s.config.context == "" {
		var err error
		target, err = kubecommon.ResolveTarget(s.config.paths, s.config.context, s.config.namespace)
		if err != nil {
			// A missing current context can be corrected from another terminal.
			return json.Marshal(map[string]string{"error": err.Error()})
		}
	}
	connection := ai.Connection{
		ConnectionID: "kube-" + target.Fingerprint, Name: target.Context, Tool: "kubernetes",
		Context: target.Context, Namespace: target.Namespace, Target: target, TargetCaptured: true,
	}
	connections := ai.ClusterConnections(connection)
	if call.Name == contextTool {
		var args map[string]json.RawMessage
		if json.Unmarshal(call.Args, &args) != nil || args == nil || len(args) != 0 {
			return json.RawMessage(`{"error":"context discovery takes an empty argument object"}`), nil
		}
		if s.config.context != "" {
			s.target = target
		}
		return json.Marshal(struct {
			Context     string          `json:"context"`
			Namespace   string          `json:"default_namespace"`
			Connections []ai.Connection `json:"connections"`
		}{target.Context, target.Namespace, connections})
	}
	if s.session == nil || !s.target.SameConfig(target) {
		if s.session != nil {
			if err := s.session.Close(); err != nil {
				return nil, err
			}
			s.session = nil
		}
		scope := workspace.Scope{
			ID: workspace.CurrentScopeID(target.Fingerprint), Kind: "current_context",
			Connection: workspace.Connection{ID: connection.ConnectionID, Name: target.Context, Tool: "kubernetes",
				Settings: map[string]string{"context": target.Context, "namespace": target.Namespace}},
		}
		if err := s.store.EnsureScope(scope); err != nil {
			return nil, err
		}
		scopeJSON, _ := json.Marshal(connections)
		session, err := ai.OpenExternalTools(s.ctx, ai.Options{
			Agent: s.config.kind, Store: s.store, History: s.archive, WorkspaceID: scope.ID,
			ScopeContext: string(scopeJSON), Connections: connections, Debug: s.config.policy,
		})
		if err != nil {
			return nil, err
		}
		s.target, s.session = target, session
	}
	return s.session.Call(ctx, call, maxBytes)
}
