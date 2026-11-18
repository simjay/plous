package debugging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/ai/cli"
	"github.com/simjay/plous/internal/ai/history"
	appmonitor "github.com/simjay/plous/internal/app/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/workspace"
	"github.com/simjay/plous/skills"
	"golang.org/x/sys/unix"
)

const contextTool = "kubernetes_context_read"

type standaloneTools struct {
	monitor   appmonitor.ReadTool
	ctx       context.Context
	config    configuration
	store     *workspace.Store
	archive   *history.Store
	gate      chan struct{}
	target    *kubecommon.Target
	session   *ai.ExternalTools
	failure   error
	selection *targetSelection
	pinned    *kubecommon.Target
	control   func() (string, error)
}

func serve(ctx context.Context, c configuration, input io.ReadCloser, output, diagnostics io.Writer, version string) (err error) {
	if file, ok := input.(*os.File); ok {
		pipe, err := duplicateMCPPipe(file)
		if err != nil {
			return fmt.Errorf("prepare MCP input: %w", err)
		}
		if pipe != nil {
			defer pipe.Close()
			input = pipe
		}
	}
	if file, ok := output.(*os.File); ok {
		pipe, err := duplicateMCPPipe(file)
		if err != nil {
			return fmt.Errorf("prepare MCP output: %w", err)
		}
		if pipe != nil {
			defer pipe.Close()
			output = pipe
		}
	}
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
	monitorDataDir := c.dataDir
	if home, homeErr := os.UserHomeDir(); homeErr == nil && monitorDataDir == filepath.Join(home, ".plous", "mcp") {
		monitorDataDir = filepath.Dir(monitorDataDir)
	}
	monitorRoot, err := appmonitor.Root(monitorDataDir)
	if err != nil {
		return err
	}
	tools := &standaloneTools{monitor: appmonitor.ReadTool{Root: monitorRoot}, ctx: ctx, config: c, store: store, archive: archive, gate: make(chan struct{}, 1)}
	defer func() {
		if tools.session != nil {
			if cause := errors.Join(tools.failure, err); cause != nil {
				err = errors.Join(err, tools.session.Finish(cause))
			} else {
				err = errors.Join(err, tools.session.Close())
			}
		}
		if tools.failure == nil || agent.OnlyCancellation(tools.failure, context.Canceled) {
			return
		}
		var configErr *ai.RuntimeConfigError
		switch {
		case errors.As(tools.failure, &configErr):
			err = errors.Join(err, configErr)
		case tools.session == nil:
			err = errors.Join(err, errors.New("could not open the MCP evidence session. Check the evidence directory and its permissions"))
		default:
			err = errors.Join(err, errors.New("MCP tool access stopped. Check the saved session evidence for the failure"))
		}
	}()
	controlContext, stopControl := context.WithCancel(ctx)
	var controls sync.WaitGroup
	defer func() { stopControl(); controls.Wait() }()
	tools.control = sync.OnceValues(func() (string, error) {
		return tools.openTargetControl(controlContext, &controls)
	})
	fmt.Fprintln(diagnostics, "Plous MCP · Evidence:", root)
	instructions := "This is a standalone Plous MCP connection. Read the target with kubernetes_context_read. On explicit user request, use kubernetes_context_select to change it and wait for confirmation before reading the cluster. History is limited to the captured context in this process.\n\n" + skills.DebugInstructions()
	stopCredentialHelpers := kubernetes.StartCredentialHelpers()
	defer stopCredentialHelpers()
	return cli.ServeStdio(ctx, input, output, tools, filepath.Base(root), version, instructions, stopCredentialHelpers)
}

// Inherited stdio descriptors can block Close. Nonblocking duplicates let Go
// interrupt reads and writes and turn a broken stdout pipe into an I/O error.
func duplicateMCPPipe(file *os.File) (*os.File, error) {
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Mode()&(os.ModeNamedPipe|os.ModeSocket) == 0 {
		return nil, nil
	}
	fd, err := unix.FcntlInt(file.Fd(), unix.F_DUPFD_CLOEXEC, 3)
	if err != nil {
		return nil, err
	}
	if err = unix.SetNonblock(fd, true); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), file.Name()), nil
}

func (s *standaloneTools) Definitions() []agent.Tool {
	// Clients can cache schemas. Advertise the configured capabilities without
	// enumerating IDs that could go stale after a kubeconfig context switch.
	return append([]agent.Tool{{
		Name:        contextTool,
		Description: "Read the current kubeconfig context, namespace, and connection IDs. Refresh before starting an investigation or after a context switch.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}, {
		Name:           selectContextTool,
		Description:    "Request a Kubernetes target change for this Plous connection on explicit user request. A file without a context follows its current-context. Empty arguments restore the default kubeconfig and current context. Wait for the user to confirm.",
		Parameters:     json.RawMessage(`{"type":"object","properties":{"kubeconfig":{"type":"string"},"context":{"type":"string"}},"additionalProperties":false}`),
		Mutating:       true,
		NonDestructive: true,
	}}, ai.ToolDefinitions(ai.Options{
		WorkspaceID: "standalone", Monitor: s.monitor,
		Connections: []ai.Connection{{Tool: "kubernetes"}, {Tool: "helm"}},
		Debug:       s.config.policy,
	})...)
}

func (s *standaloneTools) Call(ctx context.Context, call agent.ToolCall, maxBytes int) (body json.RawMessage, err error) {
	if call.Name == selectContextTool {
		return s.requestTarget(ctx, call.Args)
	}
	select {
	case s.gate <- struct{}{}:
		defer func() {
			if err != nil && !errors.Is(err, agent.ErrRequestCancelled) {
				s.failure = err
			}
			<-s.gate
		}()
	case <-ctx.Done():
		return nil, agent.ErrRequestCancelled
	}
	if s.failure != nil {
		return nil, s.failure
	}
	if ctx.Err() != nil {
		return nil, agent.ErrRequestCancelled
	}
	if call.Name == contextTool {
		var args map[string]json.RawMessage
		if json.Unmarshal(call.Args, &args) != nil || args == nil || len(args) != 0 {
			return json.RawMessage(`{"error":"context discovery takes an empty argument object"}`), nil
		}
	}
	target, source, targetErr := s.resolveTarget()
	if call.Name == contextTool || targetErr != nil {
		return json.Marshal(statusForTarget(target, source, targetErr))
	}
	connections := clusterConnections(target)
	connection := connections[0]
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
			Agent: s.config.kind, Monitor: s.monitor, Store: s.store, History: s.archive, WorkspaceID: scope.ID,
			ScopeContext: string(scopeJSON), Connections: connections, Debug: s.config.policy,
		})
		if err != nil {
			return nil, err
		}
		s.target, s.session = target, session
	}
	return s.session.Call(ctx, call, maxBytes)
}
