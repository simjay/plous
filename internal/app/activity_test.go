package app

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ui"
	"github.com/simjay/plous/internal/workspace"
	"strings"
	"testing"
	"time"
)

func TestActivityDetailColorsWrappingAndNavigation(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	mainCommand(t, m, "3")
	before := *m.current()
	after := before
	after.Name = "Payments production"
	after.Connections = []workspace.Connection{{ID: "connection-id", Tool: "kustomize", Name: "Production", Settings: map[string]string{"path": strings.Repeat("/長い-path", 20) + "\nsecond line\x1b[2J"}}}
	change := workspace.Change{Before: &before, After: after, Revision: 2, Actor: "user", Time: time.Now()}
	m.activityUI.Changes = []workspace.Change{change}
	press(t, m, "enter")
	if strings.Contains(m.detail, "\x1b[2J") || !strings.Contains(ansi.Strip(m.detail), "second line") {
		t.Fatal("detail must preserve multiline values and strip terminal controls")
	}
	if !strings.Contains(m.viewContent(120, 100), ui.AddedStyle.Render("+ After   Payments production")) {
		t.Fatal("detail rendering stripped change colors")
	}
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 60}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			for _, offset := range []int{0, 8, 10000} {
				m.cursor = offset
				lines := strings.Split(ansi.Strip(m.render()), "\n")
				if len(lines) != size[1] {
					t.Fatal("detail overflowed the terminal height")
				}
				for _, line := range lines {
					if ansi.StringWidth(line) != size[0] {
						t.Fatalf("detail overflowed a terminal row: %q", line)
					}
				}
			}
		})
	}
	press(t, m, "esc")
	if m.detail != "" || m.mode != 2 || m.positions[m.viewKey()] != 0 {
		t.Fatal("Esc must return to the selected Activity event")
	}
}
