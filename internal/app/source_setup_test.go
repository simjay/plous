package app

import (
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	appsettings "github.com/simjay/plous/internal/app/settings"
	textview "github.com/simjay/plous/internal/ui/text"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func directoryForm(t *testing.T) (*model, *appsettings.Form, string) {
	t.Helper()
	m, _ := testModel(t)
	createWorkspace(t, m, "Directory test")
	openTool(t, m, "code")
	f := m.form
	root := t.TempDir()
	f.Source.Base = root
	f.Fields[0].Value = textview.NewField("Local source")
	return m, f, root
}

func finishDirectory(t *testing.T, m *model) {
	t.Helper()
	cmd := m.syncDirectoryPicker()
	if cmd == nil {
		t.Fatal("folder request missing")
	}
	m.applyDirectoryResult(cmd().(appsettings.DirectoryResult))
	if m.form.Directory.Err != "" {
		t.Fatal(m.form.Directory.Err)
	}
}

func TestSourceSetupResolvesValidatesAndKeepsSourceDrafts(t *testing.T) {
	m, f, root := directoryForm(t)
	directory := filepath.Join(root, "my apps")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	f.Fields[2].Value = textview.NewField("./my apps")
	view := ansi.Strip(m.formBody(74, 24))
	if strings.Contains(view, "Git Revision") || !strings.Contains(view, "Relative to:") || !strings.Contains(view, "Resolved:") {
		t.Fatal(view)
	}
	f.Index = 1
	m.updateForm(localMouseKey("right"))
	if len(f.Fields) != 5 || f.Fields[2].Value.Text != "" {
		t.Fatal("local path became a Git URL")
	}
	f.Fields[2].Value = textview.NewField("git@example.invalid:platform/repo.git")
	f.Fields[3].Value = textview.NewField("release")
	m.updateForm(localMouseKey("left"))
	if len(f.Fields) != 3 || f.Fields[2].Value.Text != "./my apps" {
		t.Fatal("source switch lost directory draft")
	}
	f.Fields[2].Value = textview.NewField("missing")
	m.submitForm()
	if m.form != f || f.Index != 2 || !strings.Contains(f.Source.Err, "does not exist") || len(m.current().Connections) != 0 {
		t.Fatal("invalid directory saved", m.status)
	}
	f.Fields[2].Value = textview.NewField("./my apps")
	m.submitForm()
	if m.form != nil || m.current().Connections[0].Settings["path"] != directory {
		t.Fatal("relative path was not saved as absolute", m.status)
	}
}

func TestSourceDirectoryPickerNavigationAndDraftOwnership(t *testing.T) {
	m, f, root := directoryForm(t)
	mode := m.mode
	os.Mkdir(filepath.Join(root, "payments"), 0700)
	os.WriteFile(filepath.Join(root, "file.yaml"), []byte("ignored"), 0600)
	m.openDirectoryPicker()
	finishDirectory(t, m)
	p := f.Directory
	if len(p.Entries) != 1 || p.Entries[0] != "payments" {
		t.Fatal(p.Entries)
	}
	m.updateForm(localMouseKey("down"))
	m.updateForm(localMouseKey("down"))
	m.updateForm(localMouseKey("enter"))
	finishDirectory(t, m)
	if p.Path != filepath.Join(root, "payments") {
		t.Fatal(p.Path)
	}
	m.execute("help")
	press(t, m, "esc")
	if m.form != f || f.Directory != p {
		t.Fatal("Help lost browser")
	}
	typeText(m, "/")
	m.Update(tea.PasteMsg{Content: "ii33:q"})
	if !p.Editing || !strings.HasSuffix(p.Input.Text, "ii33:q") || m.mode != mode {
		t.Fatal("browser text ran commands")
	}
	press(t, m, "esc")
	press(t, m, "tab")
	if !f.LocalDirectory() || f.Directory != p || m.focus != 1 {
		t.Fatal("Tab escaped picker")
	}
	clickMouse(t, m, "main", "directory/0")
	if f.Directory != nil || f.Fields[2].Value.Text != filepath.Join(root, "payments") || len(m.current().Connections) != 0 {
		t.Fatal("folder choice implicitly saved the connection")
	}
	m.updateForm(localMouseKey("enter"))
	if m.form != nil || len(m.current().Connections) != 1 {
		t.Fatal("Save failed", m.status)
	}
}

func TestSourceDirectoryPickerFailureCancellationAndLayout(t *testing.T) {
	m, f, root := directoryForm(t)
	m.openDirectoryPicker()
	cmd := m.syncDirectoryPicker()
	m.updateForm(localMouseKey("esc"))
	m.applyDirectoryResult(cmd().(appsettings.DirectoryResult))
	if f.Directory != nil {
		t.Fatal("late directory response reopened picker")
	}
	m.openDirectoryPicker()
	finishDirectory(t, m)
	p := f.Directory
	p.Pending = filepath.Join(root, "missing")
	m.applyDirectoryResult(m.syncDirectoryPicker()().(appsettings.DirectoryResult))
	if p.Err == "" || p.Path != root {
		t.Fatal("failed path discarded current directory")
	}
	for _, size := range [][2]int{{80, 16}, {80, 20}, {140, 40}} {
		m.width, m.height = size[0], size[1]
		view := m.render()
		if lipgloss.Height(view) != size[1] || lipgloss.Width(view) > size[0] {
			t.Fatal("picker overflows", size)
		}
		if !strings.Contains(ansi.Strip(view), "Use This Directory") {
			t.Fatal("compact picker hides selection action", size)
		}
	}
}
