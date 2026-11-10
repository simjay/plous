// Package tui owns the interactive application, global input, and panel composition.
package tui

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
	"github.com/go-logr/logr"
	"github.com/simjay/plous/internal/ai/history"
	"github.com/simjay/plous/internal/app/terminal"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/workspace"
	"k8s.io/klog/v2"
)

// Run owns the interactive terminal lifetime.
func Run(version, dataDir string, policy kubernetes.DebugPolicy) int {
	if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) {
		fmt.Fprintln(os.Stderr, "Plous needs an interactive terminal. Use --version to print the version.")
		return 1
	}
	root := dataDir
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
	output, err := terminal.ReserveOutput()
	if err != nil {
		store.Close()
		fmt.Fprintln(os.Stderr, "Plous: cannot reserve terminal output:", err)
		return 1
	}
	defer output.Close()
	archive, err := history.Open(store)
	if err != nil {
		store.Close()
		fmt.Fprintln(output.Diagnostics, "Plous:", err)
		return 1
	}
	app, err := newModel(store, archive, version)
	if err != nil {
		store.Close()
		fmt.Fprintln(output.Diagnostics, "Plous:", err)
		return 1
	}
	app.clipboard = systemClipboard()
	app.debugPolicy = policy
	// Dependency diagnostics bypass the renderer and move the terminal cursor.
	// Explore renders request failures and Helm lint findings inside its panel.
	klog.SetLoggerWithOptions(logr.Discard(), klog.ContextualLogger(true))
	logOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(logOutput)
	_, runErr := tea.NewProgram(app, tea.WithInput(&terminal.Input{File: os.Stdin}), tea.WithOutput(output.Renderer)).Run()
	app.stopViews()
	app.stopChat()
	app.drainNativeAgent()
	app.drainChatRestores()
	app.drainChat()
	saveErr := app.saveSession()
	closeErr := store.Close()
	if err := firstError(runErr, saveErr, closeErr); err != nil {
		fmt.Fprintln(output.Diagnostics, "Plous:", err)
		return 1
	}
	return 0
}

func (m *model) stopViews() {
	if m.uiDone != nil && !m.uiStopped {
		close(m.uiDone)
		m.uiStopped = true
	}
	m.stopNativeAgent()
	m.cancelClipboardPaste()
	m.stopActivity()
	m.stopHelm()
	m.stopClusters()
	for _, v := range m.overviews {
		v.stop()
	}
	m.stopGitOps()

	m.exploreClosed = true
	m.stopExplore()
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
