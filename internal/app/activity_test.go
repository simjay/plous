package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/workspace"
)

func TestActivityDetailsDescribeChangedValues(t *testing.T) {
	connection := func(id, name string, settings map[string]string) workspace.Connection {
		return workspace.Connection{ID: id, Name: name, Tool: "kubernetes", Settings: settings}
	}
	stable := connection("unchanged-id", "Unchanged cluster", map[string]string{"context": "unchanged-context"})
	for _, tc := range []struct {
		name   string
		before *workspace.Workspace
		after  workspace.Workspace
		want   []string
		absent []string
	}{
		{
			name:   "creation",
			after:  workspace.Workspace{ID: "internal-workspace-id", Name: "First Workspace"},
			want:   []string{"+ Workspace created", "First Workspace", "No connections yet."},
			absent: []string{"null", "internal-workspace-id", "Before", "After", "{", "}"},
		},
		{
			name:   "rename",
			before: &workspace.Workspace{Name: "First Workspace", Connections: []workspace.Connection{stable}},
			after:  workspace.Workspace{Name: "Production", Connections: []workspace.Connection{stable}},
			want:   []string{"~ Workspace renamed", "Workspace name", "− Before  First Workspace", "+ After   Production"},
			absent: []string{"Unchanged cluster", "unchanged-context", "unchanged-id"},
		},
		{
			name:   "same count replacement",
			before: &workspace.Workspace{Name: "Payments", Connections: []workspace.Connection{stable, connection("old-id", "Old cluster", map[string]string{"context": "old-context"})}},
			after:  workspace.Workspace{Name: "Payments", Connections: []workspace.Connection{connection("new-id", "New cluster", map[string]string{"context": "new-context"}), stable}},
			want:   []string{"+ Added connection", "New cluster", "+ After   new-context", "− Removed connection", "Old cluster", "− Before  old-context"},
			absent: []string{"Unchanged cluster", "unchanged-context", "old-id", "new-id"},
		},
		{
			name:   "settings and empty values",
			before: &workspace.Workspace{Name: "Payments", Connections: []workspace.Connection{connection("same-id", "Development", map[string]string{"context": "stable-context", "namespace": "default", "removed": "old value", "empty-removed": ""})}},
			after:  workspace.Workspace{Name: "Payments", Connections: []workspace.Connection{connection("same-id", "Staging", map[string]string{"context": "stable-context", "namespace": "payments", "empty-added": ""})}},
			want:   []string{"~ Updated connection", "− Before  Development", "+ After   Staging", "Namespace", "− Before  default", "+ After   payments", "removed", "− Before  old value", "empty-removed\n− Before  (empty)", "empty-added\n+ After   (empty)"},
			absent: []string{"stable-context", "same-id", "Added connection", "Removed connection"},
		},
		{
			name:   "reordered connections",
			before: &workspace.Workspace{Name: "Payments", Connections: []workspace.Connection{stable, connection("second", "Second", nil)}},
			after:  workspace.Workspace{Name: "Payments", Connections: []workspace.Connection{connection("second", "Second", map[string]string{}), stable}},
			want:   []string{"No configuration values changed."},
			absent: []string{"Added connection", "Removed connection", "Updated connection"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &model{}
			change := workspace.Change{Before: tc.before, After: tc.after, Revision: 7, Actor: "user", Time: time.Date(2026, 9, 29, 20, 32, 0, 0, time.UTC)}
			text := ansi.Strip(m.changeDetail(change))
			for _, want := range append(tc.want, "Revision 7", "By user") {
				if !strings.Contains(text, want) {
					t.Fatalf("missing %q in detail:\n%s", want, text)
				}
			}
			for _, absent := range tc.absent {
				if strings.Contains(text, absent) {
					t.Fatalf("unexpected %q in detail:\n%s", absent, text)
				}
			}
		})
	}
}

func TestActivityDetailColorsWrappingAndNavigation(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	mainCommand(t, m, "3")
	before := *m.current()
	after := before
	after.Name = "Payments production"
	after.Connections = []workspace.Connection{{ID: "connection-id", Tool: "kustomize", Name: "Production", Settings: map[string]string{"path": strings.Repeat("/長い-path", 20) + "\nsecond line\x1b[2J"}}}
	change := workspace.Change{Before: &before, After: after, Revision: 2, Actor: "user", Time: time.Now()}
	m.changes = []workspace.Change{change}
	press(t, m, "enter")
	if strings.Contains(m.detail, "\x1b[2J") || !strings.Contains(ansi.Strip(m.detail), "second line") {
		t.Fatal("detail must preserve multiline values and strip terminal controls")
	}
	if !strings.Contains(m.viewContent(120, 100), addedStyle.Render("+ After   Payments production")) {
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
