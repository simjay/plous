package activity

import (
	"github.com/charmbracelet/x/ansi"

	"github.com/simjay/plous/internal/workspace"
	"strings"
	"testing"
	"time"
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
			want:   []string{"~ Workspace renamed", "Workspace Name", "− Before  First Workspace", "+ After   Production"},
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
			change := workspace.Change{Before: tc.before, After: tc.after, Revision: 7, Actor: "user", Time: time.Date(2026, 9, 29, 20, 32, 0, 0, time.UTC)}
			text := ansi.Strip(ChangeDetail(change))
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
