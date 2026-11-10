// Package debugging owns standalone agent setup and MCP application startup.
package debugging

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/ai/cli"
	"github.com/simjay/plous/internal/ai/history"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/workspace"
	"github.com/simjay/plous/skills"
)

type configuration struct {
	paths, context, namespace, dataDir, home string
	kind                                     agent.Kind
	policy                                   kubernetes.DebugPolicy
	skipCheck                                bool
}

func Run(ctx context.Context, args []string, input io.ReadCloser, output, diagnostics io.Writer, version, dataDir string, policy kubernetes.DebugPolicy) int {
	setup := len(args) > 0 && args[0] == "setup"
	if len(args) < 2 || !setup && (args[0] != "mcp" || args[1] != "serve") {
		fmt.Fprintln(diagnostics, "Usage: plous setup codex|claude [options]\n       plous mcp serve --context NAME [options]")
		return 2
	}
	c := configuration{dataDir: dataDir, policy: policy, kind: agent.Codex}
	flags := flag.NewFlagSet("plous "+strings.Join(args[:2], " "), flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	flags.StringVar(&c.paths, "kubeconfig", "", "Kubeconfig path or path list (default terminal kubeconfig)")
	flags.StringVar(&c.context, "context", "", "Kubernetes context captured for this server")
	flags.StringVar(&c.namespace, "namespace", "", "Default namespace; active tests are restricted to it")
	flags.StringVar(&c.dataDir, "data-dir", dataDir, "Standalone evidence root (default ~/.plous/mcp)")
	flags.BoolVar(&c.policy.Exec, "allow-exec", policy.Exec, "Allow noninteractive commands in existing containers")
	flags.StringVar(&c.policy.Image, "diagnostic-image", policy.Image, "Enable temporary tests using this diagnostic image")
	var kind string
	if setup {
		switch args[1] {
		case "codex":
			c.kind = agent.Codex
		case "claude", "claude-code":
			c.kind = agent.ClaudeCode
		default:
			fmt.Fprintln(diagnostics, "Choose codex or claude for setup")
			return 2
		}
		flags.StringVar(&c.home, "home", "", "Installation home (default user home)")
		flags.BoolVar(&c.skipCheck, "skip-check", false, "Skip the Kubernetes API connection check")
	} else {
		flags.StringVar(&kind, "agent", "codex", "Client identity: codex or claude")
	}
	if err := flags.Parse(args[2:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(diagnostics, "Unexpected positional arguments")
		return 2
	}
	if !setup {
		switch kind {
		case "codex":
			c.kind = agent.Codex
		case "claude", "claude-code":
			c.kind = agent.ClaudeCode
		default:
			fmt.Fprintln(diagnostics, "agent must be codex or claude")
			return 2
		}
	}
	if setup && c.context == "" {
		if file, ok := input.(*os.File); ok && term.IsTerminal(file.Fd()) {
			reader := bufio.NewReader(input)
			hint := c.paths
			if hint == "" {
				hint = "terminal kubeconfig"
			}
			fmt.Fprintf(diagnostics, "Kubeconfig path [%s]: ", hint)
			value, err := reader.ReadString('\n')
			if err != nil {
				fmt.Fprintln(diagnostics, err)
				return 1
			}
			if strings.TrimSpace(value) != "" {
				c.paths = strings.TrimSpace(value)
			}
			inventory, err := kubecommon.DiscoverCurrent(c.paths)
			if err != nil {
				fmt.Fprintln(diagnostics, err)
				return 1
			}
			for _, entry := range inventory.Contexts {
				fmt.Fprintln(diagnostics, " ", entry.Name)
			}
			fmt.Fprint(diagnostics, "Context name: ")
			value, err = reader.ReadString('\n')
			if err != nil {
				fmt.Fprintln(diagnostics, err)
				return 1
			}
			c.context = strings.TrimSpace(value)
		}
	}
	if c.context == "" {
		fmt.Fprintln(diagnostics, "--context is required; choose an explicit Kubernetes context")
		return 2
	}
	target, err := kubecommon.ResolveTarget(c.paths, c.context, c.namespace)
	if err != nil {
		fmt.Fprintln(diagnostics, "Plous:", err)
		return 1
	}
	c.paths, c.context, c.namespace = target.Paths, target.Context, target.Namespace
	if c.namespace == "*" {
		fmt.Fprintln(diagnostics, "Choose a concrete default namespace")
		return 2
	}
	if c.dataDir == "" {
		home, e := os.UserHomeDir()
		if e != nil {
			fmt.Fprintln(diagnostics, e)
			return 1
		}
		c.dataDir = filepath.Join(home, ".plous", "mcp")
	}
	c.dataDir, err = filepath.Abs(c.dataDir)
	if err != nil {
		fmt.Fprintln(diagnostics, err)
		return 1
	}
	if setup {
		err = install(ctx, c, target, output)
	} else {
		err = serve(ctx, c, target, input, output, diagnostics, version)
	}
	if err != nil {
		fmt.Fprintln(diagnostics, "Plous:", err)
		return 1
	}
	return 0
}

func serve(ctx context.Context, c configuration, target *kubecommon.Target, input io.ReadCloser, output, diagnostics io.Writer, version string) (err error) {
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
	connection := workspace.Connection{ID: "kube-" + target.Fingerprint, Name: target.Context, Tool: "kubernetes", Settings: map[string]string{"context": target.Context, "namespace": target.Namespace}}
	scope := workspace.Scope{ID: workspace.CurrentScopeID(target.Fingerprint), Kind: "current_context", Connection: connection}
	if err = store.EnsureScope(scope); err != nil {
		return err
	}
	archive, err := history.Open(store)
	if err != nil {
		return err
	}
	connections := ai.ClusterConnections(ai.Connection{ConnectionID: connection.ID, Name: connection.Name, Tool: "kubernetes", Context: target.Context, Namespace: target.Namespace, Target: target, TargetCaptured: true})
	scopeJSON, _ := json.Marshal(connections)
	tools, err := ai.OpenExternalTools(ctx, ai.Options{Agent: c.kind, Store: store, History: archive, WorkspaceID: scope.ID, ScopeContext: string(scopeJSON), Connections: connections, Debug: c.policy})
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, tools.Close()) }()
	fmt.Fprintln(diagnostics, "Plous MCP:", target.Context, "/", target.Namespace, "; evidence:", root)
	instructions := skills.DebugInstructions() + "\nCaptured connections (fixed until reconnection):\n" + string(scopeJSON) + "\nHistory contains this standalone session's saved evidence only. TUI presentation tools are unavailable."
	return cli.ServeStdio(ctx, input, output, tools, tools.ID, version, instructions)
}
