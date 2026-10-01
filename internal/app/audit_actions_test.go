package app

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/vt"
)

func TestAuditTerminalCloseUnblocksPendingTerminalReply(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tm := &resourceTerminal{emulator: vt.NewSafeEmulator(80, 24), cancel: cancel}
	t.Cleanup(tm.close)
	finished := make(chan struct{})
	go func() {
		// A cursor query sends a reply through the input pipe. No reader is
		// attached, which exercises closing while the output writer is blocked.
		_, _ = tm.emulator.Write([]byte("\x1b[6n"))
		close(finished)
	}()
	select {
	case <-finished:
		t.Fatal("cursor reply unexpectedly had an input consumer")
	case <-time.After(10 * time.Millisecond):
	}
	tm.close()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("closing the terminal deadlocked a pending reply")
	}
	if ctx.Err() == nil {
		t.Fatal("closing the terminal left its network context active")
	}
}

func TestAuditActionButtonsAcceptHelpAndRestoreTheirState(t *testing.T) {
	for _, stage := range []string{"menu", "confirm", "review", "done"} {
		t.Run(stage, func(t *testing.T) {
			m := exploreModel(t)
			m.openExploreSelection(0)
			m.explore.detailValid = true
			m.openResourceActions("")
			a := m.explore.action
			a.pending, a.stage = "", stage
			m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
			m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
			if m.help == nil {
				t.Fatal("Help shortcut was swallowed by a resource action's navigation controls")
			}
			press(t, m, "esc")
			if m.help != nil || m.explore.action != a || a.stage != stage {
				t.Fatal("Help did not restore the retained action state")
			}
		})
	}
}

func TestAuditExecDoubleQuotedArgumentPreservesLiteralBackslash(t *testing.T) {
	args, err := splitExecCommand(`/bin/sh -c "printf '\n'"`)
	if err != nil || len(args) != 3 || args[2] != `printf '\n'` {
		t.Fatalf("quoted command was changed: %#v %v", args, err)
	}
}
