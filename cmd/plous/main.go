package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
	"github.com/simjay/plous/internal/workspace"
)

//go:embed VERSION
var versionFile string

func version() string { return strings.TrimSpace(versionFile) }

func main() {
	showVersion := flag.Bool("version", false, "Print the Plous version")
	dataDir := flag.String("data-dir", "", "Persistent data directory (default ~/.plous)")
	flag.Parse()
	if *showVersion {
		fmt.Println(version())
		return
	}
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "Usage: plous [--version] [--data-dir PATH]")
		os.Exit(2)
	}
	if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) {
		fmt.Fprintln(os.Stderr, "Plous needs an interactive terminal. Use --version to print the version.")
		os.Exit(1)
	}
	root := *dataDir
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Plous:", err)
			os.Exit(1)
		}
		root = filepath.Join(home, ".plous")
	}
	store, err := workspace.Open(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Plous:", err)
		os.Exit(1)
	}
	app, err := newModel(store)
	if err != nil {
		store.Close()
		fmt.Fprintln(os.Stderr, "Plous:", err)
		os.Exit(1)
	}
	_, runErr := tea.NewProgram(app).Run()
	saveErr := app.saveSession()
	closeErr := store.Close()
	if err := firstError(runErr, saveErr, closeErr); err != nil {
		fmt.Fprintln(os.Stderr, "Plous:", err)
		os.Exit(1)
	}
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
