// Package debugging owns standalone agent setup and MCP application startup.
package debugging

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/simjay/plous/internal/ai/agent"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

type configuration struct {
	paths, context, namespace, dataDir, home string
	kind                                     agent.Kind
	policy                                   kubernetes.DebugPolicy
}

func Run(ctx context.Context, args []string, input io.ReadCloser, output, diagnostics io.Writer, version, dataDir string, policy kubernetes.DebugPolicy) int {
	setup := len(args) > 0 && args[0] == "setup"
	if len(args) < 2 || !setup && (args[0] != "mcp" || args[1] != "serve") {
		fmt.Fprintln(diagnostics, "Usage: plous setup codex|claude [options]\n       plous mcp serve [options]")
		return 2
	}
	c := configuration{dataDir: dataDir, policy: policy, kind: agent.Codex}
	flags := flag.NewFlagSet("plous "+strings.Join(args[:2], " "), flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	flags.StringVar(&c.paths, "kubeconfig", "", "Kubeconfig path or path list (default terminal kubeconfig)")
	flags.StringVar(&c.context, "context", "", "Pin a Kubernetes context (default follows kubeconfig current-context)")
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
		flags.Bool("skip-check", false, "Accepted for compatibility. Setup does not contact Kubernetes.")
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
	if c.namespace == "*" {
		fmt.Fprintln(diagnostics, "Choose a concrete default namespace")
		return 2
	}
	var err error
	if c.paths != "" {
		c.paths, err = kubecommon.NormalizeConfigPaths(c.paths)
		if err != nil {
			fmt.Fprintln(diagnostics, "Plous:", err)
			return 1
		}
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
		err = install(c, output)
	} else {
		err = serve(ctx, c, input, output, diagnostics, version)
	}
	if err != nil {
		fmt.Fprintln(diagnostics, "Plous:", err)
		return 1
	}
	return 0
}
