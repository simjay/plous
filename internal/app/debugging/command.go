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

func (c *configuration) bindServeFlags(flags *flag.FlagSet) {
	flags.StringVar(&c.paths, "kubeconfig", c.paths, "Kubeconfig path or path list (default terminal kubeconfig)")
	flags.StringVar(&c.context, "context", c.context, "Pin a Kubernetes context (default follows kubeconfig current-context)")
	flags.StringVar(&c.namespace, "namespace", c.namespace, "Default namespace; active tests are restricted to it")
	flags.StringVar(&c.dataDir, "data-dir", c.dataDir, "Standalone evidence root (default ~/.plous/mcp)")
	flags.BoolVar(&c.policy.Exec, "allow-exec", c.policy.Exec, "Allow noninteractive commands in existing containers")
	flags.StringVar(&c.policy.Image, "diagnostic-image", c.policy.Image, "Enable temporary tests using this diagnostic image")
}

func Usage() string {
	return "       " + setupUsage() + "\n       plous mcp serve [options]"
}

func setupUsage() string { return "plous setup " + setupClientNames() + " [options]" }

func Run(ctx context.Context, args []string, input io.ReadCloser, output, diagnostics io.Writer, version, dataDir string, policy kubernetes.DebugPolicy) int {
	setup := len(args) > 0 && args[0] == "setup"
	if setup && (len(args) == 1 || len(args) == 2 && (args[1] == "-h" || args[1] == "--help")) {
		fmt.Fprintln(output, "Usage: "+setupUsage())
		return 0
	}
	if len(args) < 2 || !setup && (args[0] != "mcp" || args[1] != "serve") {
		fmt.Fprintln(diagnostics, "Usage: plous mcp serve [options]")
		return 2
	}
	c := configuration{dataDir: dataDir, policy: policy}
	flags := flag.NewFlagSet("plous "+strings.Join(args[:2], " "), flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	c.bindServeFlags(flags)
	name := "codex"
	if setup {
		name = args[1]
		flags.StringVar(&c.home, "home", "", "Installation home (default user home)")
		flags.Bool("skip-check", false, "Accepted for compatibility. Setup does not contact Kubernetes.")
	} else {
		flags.StringVar(&name, "agent", name, "Client identity ("+setupClientNames()+")")
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
	client, ok := findSetupClient(name)
	if !ok {
		fmt.Fprintln(diagnostics, "Choose an MCP client from "+setupClientNames())
		return 2
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
		err = client.install(c, output)
	} else {
		c.kind = client.kind
		err = serve(ctx, c, input, output, diagnostics, version)
	}
	if err != nil {
		fmt.Fprintln(diagnostics, "Plous:", err)
		return 1
	}
	return 0
}
