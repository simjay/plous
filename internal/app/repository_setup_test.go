package app

import (
	lipgloss "charm.land/lipgloss/v2"
	"context"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	appcode "github.com/simjay/plous/internal/app/code"

	appsettings "github.com/simjay/plous/internal/app/settings"
	"github.com/simjay/plous/internal/source"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRepositorySetupLocalFolderNeedsNoGit(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Plain files")
	m.beginConnection("code", nil)
	f := m.form
	if len(f.Fields) != 3 || f.Fields[0].Label != "Name" || f.Fields[0].Key != "name" {
		t.Fatal("local setup must begin with Name and omit Git fields")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ordinary folder"), 0600); err != nil {
		t.Fatal(err)
	}
	f.Fields[0].Value = textview.NewField("Notes")
	f.Fields[2].Value = textview.NewField(dir)
	for _, size := range [][2]int{{80, 16}, {100, 24}, {180, 42}} {
		m.width, m.height = size[0], size[1]
		view := m.render()
		if lipgloss.Width(view) > m.width || lipgloss.Height(view) != m.height {
			t.Fatal("setup overflowed", size)
		}
		if !strings.Contains(ansi.Strip(view), "[ Add Source ]") || !strings.Contains(ansi.Strip(view), "[ Cancel ]") {
			t.Fatal("compact setup hid an action", size)
		}
		if strings.Contains(ansi.Strip(view), "Branch") {
			t.Fatal("local setup exposed a Git branch")
		}
	}
	m.submitForm()
	if m.form != nil || len(m.current().Connections) != 1 {
		t.Fatal("plain directory was rejected", m.status)
	}
	c := m.current().Connections[0]
	m.selectScope(m.active, "files", c.ID)
	m.mode = 1
	finishCode(t, m)
	if !m.coder().Inventory.Source.Has("notes.txt") {
		t.Fatal("plain directory did not load")
	}
	m.expanded[c.ID] = true
	var views []string
	for _, row := range m.treeRows() {
		if row.parent == c.ID {
			views = append(views, row.label)
		}
	}
	if !slices.Equal(views, []string{"Files", "Helm", "Kustomize", "Argo", "Flux"}) || len(m.coder().Entries()) != 0 {
		t.Fatal("generic folders need stable views without invented source entries", views)
	}
}

func TestRepositorySetupBranchAndLegacySettings(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Repositories")
	old := workspace.Connection{Tool: "code", Name: "Pinned", Settings: map[string]string{
		"source_type": "Git repository", "path": "https://example.invalid/repo.git",
		"revision": "v1.2.3", "subdirectory": "deploy", "future": "preserve",
	}}
	w := *m.current()
	w.Connections = []workspace.Connection{old}
	if !m.saveWorkspace(w) {
		t.Fatal(m.status)
	}
	c := m.current().Connections[0]
	m.beginConnection("code", &c)
	f := m.form
	if len(f.Fields) != 5 || f.Fields[3].Key != "branch" || f.Fields[3].Value.Text != "" {
		t.Fatal("repository form must expose URL and Branch without mislabeling a legacy tag")
	}
	f.Fields[0].Value = textview.NewField("Renamed")
	m.submitForm()
	saved := m.current().Connections[0]
	if saved.ID != c.ID || saved.Settings["revision"] != "v1.2.3" || saved.Settings["subdirectory"] != "deploy" || saved.Settings["future"] != "preserve" {
		t.Fatal("renaming lost existing reference or settings", saved)
	}
	m.beginConnection("code", &saved)
	m.form.Fields[3].Value = textview.NewField("release/production")
	m.submitForm()
	saved = m.current().Connections[0]
	if saved.Settings["branch"] != "release/production" || saved.Settings["revision"] != "" || appcode.ConfigForConnection(&saved).Revision != "refs/heads/release/production" {
		t.Fatal("explicit branch did not replace the saved reference", saved)
	}
	m.beginConnection("code", &saved)
	m.form.Fields[3].Value = textview.NewField("")
	m.submitForm()
	saved = m.current().Connections[0]
	if appcode.ConfigForConnection(&saved).Revision != "" || saved.Settings["subdirectory"] != "deploy" {
		t.Fatal("blank branch did not return to the default branch")
	}
	for _, branch := range []string{"HEAD", "refs/tags/v1", "-option", "feature..x", "feature/", "x.lock", "x y"} {
		m.beginConnection("code", &saved)
		m.form.Fields[3].Value = textview.NewField(branch)
		m.submitForm()
		if m.form == nil || m.form.Index != 3 || !strings.Contains(m.status, "branch name") {
			t.Fatal("invalid branch was accepted", branch)
		}
	}
}

func TestRepositoryBranchLoadsBranchWhenTagHasSameName(t *testing.T) {
	testModel(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git unavailable")
	}
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Git fixture: %s %v", out, err)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "--template=")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("branch fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "README.md")
	git("commit", "-m", "Tag")
	git("tag", "release")
	git("commit", "--allow-empty", "-m", "Branch")
	head := git("rev-parse", "HEAD")
	git("branch", "release")
	c := workspace.Connection{Settings: map[string]string{"source_type": "Git repository", "path": "file://" + dir, "branch": "release"}}
	snapshot, err := source.Load(context.Background(), appcode.ConfigForConnection(&c))
	if err != nil || snapshot.Revision != head || git("rev-parse", "HEAD") != head {
		t.Fatal("branch selection used a tag or changed the checkout", err)
	}
	delete(c.Settings, "branch")
	snapshot, err = source.Load(context.Background(), appcode.ConfigForConnection(&c))
	if err != nil || snapshot.Revision != head {
		t.Fatal("blank branch did not resolve the remote default", err)
	}
}

func TestRepositoryBranchNavigationKeepsArrowOwnership(t *testing.T) {
	m, s := codeModel(t)
	m.width, m.height = 180, 42
	connection := m.connection
	m.expanded[connection] = false
	clickMouse(t, m, "tree", "tree/"+connection)
	if m.focus != 0 || !m.expanded[connection] || !m.codeVisible() {
		t.Fatal("opening a repository branch stole sidebar arrow keys")
	}
	for _, key := range []string{connection, m.active + "/code", m.active} {
		for i, row := range m.treeRows() {
			if row.key == key {
				m.treeCursor = i
			}
		}
		press(t, m, "left")
		if m.expanded[key] || m.focus != 0 || m.connection != connection || m.coder() != s {
			t.Fatal("Left did not fold without changing the open content", key)
		}
		press(t, m, "right")
		if !m.expanded[key] || m.focus != 0 {
			t.Fatal("Right did not expand", key)
		}
		clickMouse(t, m, "tree", "fold/"+key)
		if m.expanded[key] || m.connection != connection {
			t.Fatal("triangle did not collapse independently", key)
		}
		clickMouse(t, m, "tree", "fold/"+key)
	}
	clickMouse(t, m, "tree", "tree/"+connection+"/files")
	if m.focus != 1 || m.coder() != s {
		t.Fatal("Files did not focus the existing content")
	}
}

func TestSidebarAddRowsShareDisclosureColumn(t *testing.T) {
	m, _ := codeModel(t)
	named := treeRow{kind: "connection", tool: "files", label: "+ Team"}
	if !strings.Contains(ansi.Strip(m.renderTreeRow([]treeRow{named}, 0, 30)[0]), "+ Team") {
		t.Fatal("action alignment removed part of a connection name")
	}
	for _, width := range []int{24, 32, 50} {
		rows := []treeRow{
			{key: "repo", kind: "connection", tool: "files", label: "Repository", depth: 2, parent: "code"},
			{key: "add", kind: "addarea", label: "+ Add Source", depth: 2, parent: "code"},
		}
		branch := ansi.Strip(m.renderTreeRow(rows, 0, width)[0])
		action := ansi.Strip(m.renderTreeRow(rows, 1, width)[0])
		if ansi.StringWidth(strings.Split(branch, "▸")[0]) != ansi.StringWidth(strings.Split(action, "+")[0]) {
			t.Fatalf("icons do not align: %q / %q", branch, action)
		}
	}
	labels := []string{}
	m.expanded[m.connection] = true
	for _, row := range m.treeRows() {
		if row.kind == "codeview" {
			labels = append(labels, row.label)
		}
	}
	if !slices.Equal(labels, []string{"Files", "Helm", "Kustomize", "Argo", "Flux"}) {
		t.Fatal("unexpected repository views", labels)
	}
}

func TestConnectionPickerMouseAndRepositoryTitle(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Setup")
	for i, tool := range appsettings.ToolTypes {
		m.beginToolSetup(false)
		m.cursor = i
		m.View()
		var target string
		for _, span := range m.mouse.spans {
			if strings.HasPrefix(span.Target, fmt.Sprintf("choice/%d/", i)) {
				target = span.Target
				break
			}
		}
		clickMouse(t, m, "main", target)
		if m.form == nil || tool != "kubernetes" && m.form.Tool != tool {
			t.Fatal("group heading displaced the selected connection", tool)
		}
		m.form = nil
		m.closeSettings()
	}
	m.beginToolSetup(false)
	m.beginConnection("code", nil)
	m.form.Fields[1].Value = textview.NewField("Git repository")
	m.form.ChangeSourceType()
	m.width, m.height = 180, 42
	view := ansi.Strip(m.render())
	if strings.Count(ansi.Strip(m.formBody(78, 30)), "Add Source") != 1 || strings.Contains(view, "Add Code Source") || strings.Contains(view, "Git Revision") {
		t.Fatal("repository setup must have one title and one primary action", view)
	}
	if dir := os.Getenv("PLOUS_REVIEW_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		capture := func(name string) {
			t.Helper()
			if err := os.WriteFile(filepath.Join(dir, name+".txt"), []byte(ansi.Strip(m.View().Content)), 0600); err != nil {
				t.Fatal(err)
			}
		}
		m.form.Fields[0].Value = textview.NewField("Platform config")
		m.form.Fields[2].Value = textview.NewField("https://example.invalid/platform-config.git")
		for _, size := range [][2]int{{80, 24}, {120, 32}, {180, 42}} {
			m.width, m.height = size[0], size[1]
			m.form.Index = 0
			capture(fmt.Sprintf("git-%d", m.width))
			m.form.Index = 3
			capture(fmt.Sprintf("branch-%d", m.width))
		}
		m.form = nil
		m.cursor = 0
		capture("connection-picker")
	}
}

func TestRepositorySetupCancelReturnsToItsEntry(t *testing.T) {
	m, _ := testModel(t)
	id := createWorkspace(t, m, "Setup")
	press(t, m, "down")
	press(t, m, "enter")
	press(t, m, "esc")
	if m.page != "tools" || m.cursor != 1 || m.form != nil {
		t.Fatal("cancel lost the selected connection type")
	}
	press(t, m, "esc")
	selectTree(t, m, id+"/code/add")
	if m.form == nil || m.form.Tool != "code" {
		t.Fatal("Add Source did not open its form directly")
	}
	press(t, m, "esc")
	if m.settingsOverlay != nil || m.form != nil || m.focus != 0 || m.treeRows()[m.treeCursor].key != id+"/code/add" {
		t.Fatal("direct setup did not return to its sidebar entry")
	}
}
