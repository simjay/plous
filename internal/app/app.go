package app

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
	"github.com/simjay/plous/internal/workspace"
)

// Run starts the Plous executable and returns its process exit code.
func Run(version string) int {
	showVersion := flag.Bool("version", false, "Print the Plous version")
	dataDir := flag.String("data-dir", "", "Persistent data directory (default ~/.plous)")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return 0
	}
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "Usage: plous [--version] [--data-dir PATH]")
		return 2
	}
	if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) {
		fmt.Fprintln(os.Stderr, "Plous needs an interactive terminal. Use --version to print the version.")
		return 1
	}
	root := *dataDir
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Plous:", err)
			return 1
		}
		root = filepath.Join(home, ".plous")
	}
	store, err := workspace.Open(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Plous:", err)
		return 1
	}
	app, err := newModel(store, version)
	if err != nil {
		store.Close()
		fmt.Fprintln(os.Stderr, "Plous:", err)
		return 1
	}
	_, runErr := tea.NewProgram(app).Run()
	saveErr := app.saveSession()
	closeErr := store.Close()
	if err := firstError(runErr, saveErr, closeErr); err != nil {
		fmt.Fprintln(os.Stderr, "Plous:", err)
		return 1
	}
	return 0
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
