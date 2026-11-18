// Package app selects the interactive or standalone application entry point.
package app

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/simjay/plous/internal/app/debugging"
	"github.com/simjay/plous/internal/app/monitor"
	"github.com/simjay/plous/internal/app/tui"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

// Run starts the Plous executable and returns its process exit code.
func Run(version string) int {
	if len(os.Args) > 1 && os.Args[1] == kubernetes.CredentialHelperCommand {
		return kubernetes.RunCredentialHelper(os.Args[2:], os.Stdout, os.Stderr)
	}
	showVersion := flag.Bool("version", false, "Print the Plous version")
	dataDir := flag.String("data-dir", "", "Persistent data directory (default ~/.plous)")
	allowExec := flag.Bool("allow-exec", false, "Allow agent commands in containers in the captured default namespace")
	diagnosticImage := flag.String("diagnostic-image", "", "Enable temporary agent tests using this diagnostic image")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: plous [options]\n"+debugging.Usage()+"\n       "+monitor.Usage)
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return 0
	}
	policy := kubernetes.DebugPolicy{Exec: *allowExec, Image: *diagnosticImage}
	if flag.NArg() != 0 {
		if flag.Arg(0) == "monitor" {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			return monitor.Command(ctx, flag.Args()[1:], *dataDir, os.Stdin, os.Stdout, os.Stderr)
		}
		if flag.Arg(0) == "mcp" || flag.Arg(0) == "setup" {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			return debugging.Run(ctx, flag.Args(), os.Stdin, os.Stdout, os.Stderr, version, *dataDir, policy)
		}
		flag.Usage()
		return 2
	}
	return tui.Run(version, *dataDir, policy)
}
