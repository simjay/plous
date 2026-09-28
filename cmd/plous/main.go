package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
)

//go:embed VERSION
var versionFile string

func version() string { return strings.TrimSpace(versionFile) }

func main() {
	showVersion := flag.Bool("version", false, "Print the Plous version")
	flag.Parse()
	if *showVersion {
		fmt.Println(version())
		return
	}
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "Usage: plous [--version]")
		os.Exit(2)
	}
	if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) {
		fmt.Fprintln(os.Stderr, "Plous needs an interactive terminal. Use --version to print the version.")
		os.Exit(1)
	}
	if _, err := tea.NewProgram(model{focus: 1}).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Plous:", err)
		os.Exit(1)
	}
}
