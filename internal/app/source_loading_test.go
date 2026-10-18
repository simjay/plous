package app

import (
	"github.com/charmbracelet/x/ansi"
	appcode "github.com/simjay/plous/internal/app/code"

	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCodeScansSavedWorkspaceOnlyAfterSelection(t *testing.T) {
	m, s := codeModel(t)
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	restored, err := newModel(m.store, m.history, "test")
	if err != nil {
		t.Fatal(err)
	}
	if restored.syncCode() != nil {
		t.Fatal("startup scanned a saved source")
	}
	if !strings.Contains(ansi.Strip(restored.mainBody(80, 24)), "Current Workspace") {
		t.Fatal("startup did not select Current Workspace")
	}
	restored.selectScope(m.active, "", "")
	restored.mode = 0
	cmd := restored.syncCode()
	if cmd == nil || restored.syncCode() != nil {
		t.Fatal("restoring the workspace must schedule exactly one scan from Overview")
	}
	restored.openSettings()
	restored.help = &helpState{}
	if restored.syncCode() != nil {
		t.Fatal("overlays duplicated the background scan")
	}
	restored.help = nil
	restored.closeSettings()
	restored.mode = 1
	for _, width := range []int{32, 80, 120} {
		view := ansi.Strip(restored.mainBody(width, 24))
		if !strings.Contains(view, "Choose a connection") || strings.Contains(view, "Loading Source") {
			t.Fatal("background scanning replaced the welcome page", view)
		}
	}
	applyCodeResult(t, restored, cmd().(appcode.CodeResult))
	loaded := restored.savedCodeState(m.active, m.connection)
	if loaded.Inventory == nil || loaded.Err != "" || len(loaded.Builds) != 0 || loaded.Preference.Selected != s.Preference.Selected {
		t.Fatal("automatic load lost selection or rendered entries", loaded.Err)
	}
	restored.selectScope(m.active, "manifests", m.connection)
	finishCode(t, restored)
	if restored.coder() != loaded || restored.syncCode() != nil || loaded.Busy || len(loaded.Builds) != 0 {
		t.Fatal("opening the configuration map must retain its snapshot without rendering")
	}
}

func TestCodeInitialFailureAndCancellationRequireExplicitRetry(t *testing.T) {
	m, _ := legacyKustomizeModel(t)
	missing := filepath.Join(t.TempDir(), "missing")
	m.savedConnection().Settings["path"] = missing
	m.selectScope(m.active, "files", m.connection)
	s := m.coder()
	cmd := m.syncCode()
	if cmd == nil {
		t.Fatal("missing source did not begin loading")
	}
	applyCodeResult(t, m, cmd().(appcode.CodeResult))
	if s.Err == "" || m.syncCode() != nil || m.codeScanStatus(m.active, s.Connection) != "scan failed" || !strings.Contains(ansi.Strip(m.codeBody(80, 24)), "Enter Retry") {
		t.Fatal("failed load must stop and offer retry")
	}
	m.executeCode("actions")
	s.Choice = slices.Index(s.MenuItems(), "Diagnostics")
	m.activateCodeMenu()
	if s.Document == nil || !strings.Contains(s.Document.Text(), s.Err) {
		t.Fatal("initial load failure hid full diagnostics")
	}
	m.codeDocumentCommand("a")
	m.activateCodeMenu()
	if s.Document != nil {
		t.Fatal("diagnostics did not close through its action menu")
	}
	if err := os.Mkdir(missing, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(missing, "README.md"), []byte("Ready"), 0600); err != nil {
		t.Fatal(err)
	}
	m.updateCode("enter")
	cmd = m.syncCode()
	if cmd == nil {
		t.Fatal("retry did not start")
	}
	m.updateCode("esc")
	applyCodeResult(t, m, cmd().(appcode.CodeResult))
	if s.Inventory != nil || s.Busy || m.syncCode() != nil {
		t.Fatal("cancelled load restarted or applied a late result")
	}
	m.updateCode("enter")
	finishCode(t, m)
	if len(s.Files()) != 1 || s.Err != "" || s.Notice != "" {
		t.Fatal("retry did not recover the source")
	}
}

func TestCodeGitSourceScansWithoutSelectingFilesInIsolatedSnapshot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git unavailable")
	}
	m, k := legacyKustomizeModel(t)
	root := k.Config.Path
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Git fixture: %s %v", out, err)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "--template=")
	git("add", ".")
	git("commit", "-m", "Fixture")
	head := git("rev-parse", "HEAD")
	c := m.savedConnection()
	c.Tool = "code"
	c.Settings["source_type"], c.Settings["path"] = "Git repository", "file://"+root
	id := c.ID
	m.selectScope(m.active, "", "")
	finishCode(t, m)
	s := m.savedCodeState(m.active, id)
	if s.Inventory.Source.Revision != head || len(s.Inventory.Entries) == 0 || len(s.Builds) != 0 {
		t.Fatal("workspace scan did not capture the Git source without rendering")
	}
	if git("rev-parse", "HEAD") != head || git("status", "--porcelain") != "" {
		t.Fatal("opening changed the original checkout")
	}
}

func TestCodeScansCreatedSourceWithoutSelectingFiles(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Sources")
	m.beginConnection("code", nil)
	m.form.Fields[0].Value = textview.NewField("Mixed")
	m.form.Fields[2].Value = textview.NewField("../../tests/fixtures/code/mixed")
	m.submitForm()
	if m.form != nil || len(m.current().Connections) != 1 {
		t.Fatal("source was not saved", m.status)
	}
	c := m.current().Connections[0]
	if m.connection != "" || !m.expanded[c.ID] {
		t.Fatal("adding a source must expose its children without selecting Files")
	}
	cmd := m.syncCode()
	if cmd == nil || m.syncCode() != nil || m.codeScanStatus(m.active, c.ID) != "scanning" {
		t.Fatal("saving did not schedule a single visible scan")
	}
	rows := m.treeRows()
	selected := m.active + "/code/add"
	for i, row := range rows {
		if row.key == selected {
			m.treeCursor = i
		}
	}
	initial := cmd().(appcode.CodeResult)
	m.Update(initial)
	if initial.Continue != nil {
		m.Update(initial.Continue())
	}
	if m.treeRows()[m.treeCursor].key != selected || m.connection != "" {
		t.Fatal("discovery moved the sidebar selection or active connection")
	}
	if !m.codeHas(m.active, c.ID, "kubernetes") || !m.codeHas(m.active, c.ID, "terraform") || m.codeScanStatus(m.active, c.ID) != "" {
		t.Fatal("manifest capabilities did not appear after saving")
	}
	if len(m.savedCodeState(m.active, c.ID).Builds) != 0 {
		t.Fatal("discovery rendered source entries")
	}
}

func TestCodeBackgroundScanQueueAndStaleResults(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Sources")
	w := *m.current()
	w.Connections = []workspace.Connection{
		{Tool: "code", Name: "First", Settings: map[string]string{"path": "../../tests/fixtures/code/mixed"}},
		{Tool: "code", Name: "Second", Settings: map[string]string{"path": "../../tests/fixtures/code/mixed"}},
	}
	if !m.saveWorkspace(w) {
		t.Fatal(m.status)
	}
	first, second := m.current().Connections[0].ID, m.current().Connections[1].ID
	cmd := m.syncCode()
	if cmd == nil || m.syncCode() != nil {
		t.Fatal("background discovery did not serialize scans")
	}
	old := cmd().(appcode.CodeResult)
	// Reconfiguring a source while its scan runs must discard the old result.
	m.current().Connections[0].Settings["path"] = t.TempDir()
	applyCodeResult(t, m, old)
	if m.codeHas(m.active, first, "kubernetes") || m.codeUI.Preferences[m.active+"/"+first].Fingerprint != "" {
		t.Fatal("obsolete source scan published its inventory or preference")
	}
	finishCode(t, m)
	if old.State.Busy || m.codeHas(m.active, first, "kubernetes") {
		t.Fatal("source edit retained the obsolete scan")
	}
	cmd = m.syncCode()
	if cmd == nil || cmd().(appcode.CodeResult).State.Connection != second {
		t.Fatal("next source was not scanned")
	}
	s := m.savedCodeState(m.active, second)
	r := appcode.CodeResult{State: s, Run: s.Run, Inventory: old.Inventory, Kind: "load"}
	m.current().Connections = m.current().Connections[:1]
	applyCodeResult(t, m, r)
	if m.codeUI.Preferences[m.active+"/"+second].Fingerprint != "" || m.codeHas(m.active, second, "kubernetes") {
		t.Fatal("removed source accepted its late scan")
	}
}

func TestCodeConfigurationChangeHidesPreviousDetection(t *testing.T) {
	m, previous := codeModel(t)
	id := m.connection
	m.savedConnection().Settings["path"] = t.TempDir()
	if m.codeHas(m.active, id, "kubernetes") || m.codeHas(m.active, id, "terraform") {
		t.Fatal("old configuration still controls manifest navigation")
	}
	finishCode(t, m)
	if m.savedCodeState(m.active, id) == previous || m.codeHas(m.active, id, "kubernetes") {
		t.Fatal("new source did not replace previous detection")
	}
}

func TestCodePublishesFileMetadataBeforeInspectingConfigurations(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Discovery")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.yaml"), []byte("apiVersion: v1\nkind: ConfigMap\nmetadata: {name: example}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	w := *m.current()
	w.Connections = []workspace.Connection{{Tool: "code", Name: "source", Settings: map[string]string{"path": root, "source_type": "Local directory"}}}
	if !m.saveWorkspace(w) {
		t.Fatal(m.status)
	}
	m.selectScope(m.active, "files", m.current().Connections[0].ID)
	m.mode = 1
	initial := m.syncCode()().(appcode.CodeResult)
	continuation := m.updateCodeResult(initial)
	state := m.coder()
	if continuation == nil || !state.Busy || state.Inventory == nil || !state.Inventory.Discovering || !state.Inventory.Source.Has("app.yaml") || state.Inventory.Source.Files != nil {
		t.Fatal("file browsing waited for configuration inspection")
	}
	if !strings.Contains(appcode.CodeIdentity(state), "1 file") {
		t.Fatal("file count still depends on captured bodies")
	}
	if strings.Contains(m.codeHint(), "Cancel source operation") {
		t.Fatal("discovery hides file navigation hints")
	}
	m.focus = 1
	state.FileRoot = "nested/path"
	m.codeBrowserControl("parent")
	if state.FileRoot != "nested" {
		t.Fatal("parent navigation was blocked during discovery")
	}
	m.codeBrowserControl("root")
	if state.FileRoot != "" {
		t.Fatal("root navigation was blocked during discovery")
	}
	m.updateCode("right")
	m.mouseItem("code-row", "1", false)
	if state.Cursor != 1 {
		t.Fatal("mouse file selection was blocked during discovery")
	}
	m.updateCode("enter")
	if state.Document == nil || !strings.Contains(state.Document.Text(), "example") {
		t.Fatal("file browsing was blocked during discovery")
	}
	applyCodeResult(t, m, continuation().(appcode.CodeResult))
	if state.Busy || state.Inventory.Discovering || state.Inventory.Entry("plain:app.yaml") == nil {
		t.Fatal("configuration discovery did not complete")
	}
}

func TestCodeDiscoveryUsesGlobalProfileIgnoreOnRefresh(t *testing.T) {
	m, state := codeModel(t)
	profile, err := m.store.DataRoot()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(state.Config.Path, "ignore-test.yaml")
	if err := os.WriteFile(file, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata: {name: example}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, ".plousignore"), []byte("ignore-test.yaml\n"), 0600); err != nil {
		t.Fatal(err)
	}
	state.Pending = "load"
	finishCode(t, m)
	if state.Err != "" || state.Inventory.Source.Has("ignore-test.yaml") {
		t.Fatal("Code discovery did not use global profile policy", state.Err)
	}
	if err := os.WriteFile(filepath.Join(profile, ".plousignore"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	state.Pending = "load"
	finishCode(t, m)
	if state.Err != "" || !state.Inventory.Source.Has("ignore-test.yaml") {
		t.Fatal("refresh did not reload policy", state.Err)
	}
}
