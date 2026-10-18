package app

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	appsettings "github.com/simjay/plous/internal/app/settings"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func contextFile(t *testing.T, names ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	text := "apiVersion: v1\nkind: Config\nclusters:\n- name: example\n  cluster: {server: 'https://example.invalid'}\ncontexts:\n"
	for _, name := range names {
		text += fmt.Sprintf("- name: %s\n  context: {cluster: example, namespace: sandbox}\n", name)
	}
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func installDefaultKubeconfig(t *testing.T, source string) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".kube", "config")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func editKubeconfigPath(t *testing.T, m *model, path string) {
	t.Helper()
	press(t, m, "home")
	press(t, m, "end")
	press(t, m, "ctrl+u")
	typeText(m, path)
}

func addSelectedContexts(t *testing.T, m *model) {
	t.Helper()
	// Leave the path editor before using End as a navigation key.
	if m.form.Index == 0 {
		press(t, m, "down")
	}
	press(t, m, "end")
	press(t, m, "up")
	press(t, m, "enter")
}

func TestWorkspaceToolEntryPointsAndAutomaticDiscovery(t *testing.T) {
	for _, entry := range []string{"creation", "overview", "tree", "settings"} {
		t.Run(entry, func(t *testing.T) {
			m, _ := testModel(t)
			path := installDefaultKubeconfig(t, contextFile(t, "dev", "prod"))
			t.Setenv("KUBECONFIG", contextFile(t, "not-the-selected-file"))
			id := createWorkspace(t, m, "Payments")
			if entry != "creation" {
				press(t, m, "esc")
				switch entry {
				case "overview":
					mainCommand(t, m, "1")
					press(t, m, "enter")
				case "tree":
					selectTree(t, m, id+"/add")
				case "settings":
					openSettingsItem(t, m, "Add Connection")
				}
			}
			if m.page != "tools" || m.toolSetup == nil {
				t.Fatal("entry did not open the shared tool picker")
			}
			press(t, m, "enter")
			if m.form == nil || m.form.Fields[0].Value.Text != "~/.kube/config" || m.page != "contexts" || len(m.contexts) != 2 || !m.contextsReady() {
				t.Fatal("Kubernetes must show the explicit default file and its contexts together")
			}
			press(t, m, "space")
			press(t, m, "down")
			press(t, m, "enter")
			if len(m.current().Connections) != 0 {
				t.Fatal("checking a context must not save before Add Selected Contexts")
			}
			addSelectedContexts(t, m)
			if m.settingsOverlay != nil || m.toolSetup != nil || m.form != nil || m.page != "" || len(m.current().Connections) != 2 || !m.expanded[id+"/monitor"] {
				t.Fatal("adding must return to the workspace with Kubernetes expanded")
			}
			for _, c := range m.current().Connections {
				if c.Settings["kubeconfig"] != path {
					t.Fatal("connection lost its discovered file reference")
				}
			}
		})
	}
}

func TestAddContextTargetsItsWorkspaceAndSupportsAnotherFile(t *testing.T) {
	m, _ := testModel(t)
	first := createWorkspace(t, m, "First")
	addKubernetes(t, m)
	second := createWorkspace(t, m, "Second")
	press(t, m, "esc")
	selectTree(t, m, first+"/monitor/add")
	if m.active != first || m.page != "contexts" || m.toolSetup == nil || !m.toolSetup.ContextsOnly {
		t.Fatal("tree action opened contexts for the wrong workspace")
	}
	press(t, m, "space")
	if m.chosen[0] || !strings.Contains(m.contextPickerBody(70, 20), "Already added") {
		t.Fatal("saved contexts must be marked and cannot be checked again")
	}
	addSelectedContexts(t, m)
	if len(m.current().Connections) != 2 || m.page != "contexts" {
		t.Fatal("empty selection added duplicates")
	}
	path := contextFile(t, "another-context")
	editKubeconfigPath(t, m, path)
	press(t, m, "enter")
	press(t, m, "enter")
	press(t, m, "space")
	addSelectedContexts(t, m)
	if len(m.current().Connections) != 3 || m.current().Connections[2].Settings["kubeconfig"] != path {
		t.Fatal("another kubeconfig must add to, not replace, existing connections")
	}
	selectTree(t, m, second)
	if len(m.current().Connections) != 0 {
		t.Fatal("adding to another workspace changed the active workspace's data")
	}
	press(t, m, "right")
	selectTree(t, m, second+"/add")
	if m.page != "tools" || m.active != second {
		t.Fatal("workspace Add Connection action did not target its workspace")
	}
}

func TestContextRefreshInvalidatesSelectionsAndRecoversFromErrors(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Empty")
	press(t, m, "enter")
	if len(m.contexts) != 0 || m.toolSetup.Err == "" || m.form.Index != 0 {
		t.Fatal("missing default config must show the file field and error")
	}
	valid := contextFile(t, "dev")
	editKubeconfigPath(t, m, valid)
	press(t, m, "enter")
	press(t, m, "enter")
	press(t, m, "space")
	if !m.chosen[0] {
		t.Fatal("valid file did not load")
	}
	bad := filepath.Join(t.TempDir(), "broken")
	if err := os.WriteFile(bad, []byte("contexts: ["), 0600); err != nil {
		t.Fatal(err)
	}
	editKubeconfigPath(t, m, bad)
	if len(m.contexts) != 0 || len(m.chosen) != 0 || m.contextsReady() || !strings.Contains(m.contextPickerBody(70, 20), "Refresh required") {
		t.Fatal("editing the file path must invalidate the previous checklist")
	}
	addSelectedContexts(t, m)
	if len(m.current().Connections) != 0 || !strings.Contains(m.status, "Refresh Contexts") {
		t.Fatal("stale contexts were added before refresh")
	}
	press(t, m, "home")
	press(t, m, "enter")
	press(t, m, "enter")
	if m.form == nil || !strings.Contains(m.toolSetup.Err, "invalid kubeconfig YAML") || len(m.contexts) != 0 || m.contextsReady() {
		t.Fatal("bad file should keep the integrated view open with no stale contexts")
	}
	for _, path := range []string{"", contextFile(t), filepath.Join(t.TempDir(), "missing")} {
		editKubeconfigPath(t, m, path)
		press(t, m, "enter")
		press(t, m, "enter")
		if m.toolSetup.Err == "" || m.contextsReady() {
			t.Fatalf("invalid input %q enabled adding", path)
		}
	}
	editKubeconfigPath(t, m, valid)
	press(t, m, "enter")
	press(t, m, "enter")
	if !m.contextsReady() || len(m.contexts) != 1 || len(m.chosen) != 0 {
		t.Fatal("successful refresh must reset the selection and clear errors")
	}
	press(t, m, "space")
	// Refresh the same path after its file changes externally.
	data, err := os.ReadFile(contextFile(t, "replacement"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(valid, data, 0600); err != nil {
		t.Fatal(err)
	}
	press(t, m, "up")
	press(t, m, "enter")
	if len(m.contexts) != 1 || m.contexts[0].Name != "replacement" || len(m.chosen) != 0 {
		t.Fatal("refresh did not reread the file or reset selected contexts")
	}
	press(t, m, "esc")
	press(t, m, "esc")
	if m.settingsOverlay != nil || m.form != nil || len(m.current().Connections) != 0 {
		t.Fatal("Esc must close setup without adding connections")
	}
}

func TestContextPathInputGuardsAndExit(t *testing.T) {
	m, _ := testModel(t)
	mode := m.mode
	installDefaultKubeconfig(t, contextFile(t, "dev"))
	createWorkspace(t, m, "Local")
	press(t, m, "enter")
	press(t, m, "space")
	press(t, m, "home")
	typeText(m, ":q ss cc 11")
	m.Update(tea.PasteMsg{Content: " :quit hh\n"})
	if m.command != nil || m.chatEditing || m.help != nil || m.mode != mode || len(m.contexts) != 0 || len(m.chosen) != 0 || !strings.Contains(m.form.Fields[0].Value.Text, ":quit hh") {
		t.Fatal("path text and paste must stay literal and invalidate the checklist")
	}
	press(t, m, "down")
	form := m.form
	mainCommand(t, m, "3")
	if m.form != form || m.mode != mode {
		t.Fatal("mode command discarded unfinished setup")
	}
	mainCommand(t, m, "help")
	press(t, m, "esc")
	if m.form != form {
		t.Fatal("Help lost the path editor")
	}
	mainCommand(t, m, "chat")
	typeText(m, "preserved draft")
	press(t, m, "esc")
	press(t, m, "alt+left")
	if m.form != form || m.focus != 1 {
		t.Fatal("chat did not preserve and resume setup")
	}
	assertCtrlCQuit(t, m)
	if len(m.current().Connections) != 0 {
		t.Fatal("exit applied unsaved selections")
	}
}

func TestContextPickerFitsSmallTerminalAndTrapsFocus(t *testing.T) {
	m, _ := testModel(t)
	installDefaultKubeconfig(t, contextFile(t, "development", "production", "staging", "test-4", "test-5", "test-6"))
	createWorkspace(t, m, "Local")
	press(t, m, "enter")
	for _, size := range [][2]int{{80, 16}, {120, 30}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		press(t, m, "home")
		for _, want := range []string{"~/.kube/config", "Refresh Contexts", "development", "production", "staging", "test-4", "test-5", "test-6", "Add Selected Contexts", "Cancel"} {
			press(t, m, "tab")
			press(t, m, "shift+tab")
			screen := ansi.Strip(m.render())
			if m.focus != 1 || !strings.Contains(screen, want) || len(strings.Split(screen, "\n")) != size[1] {
				t.Fatalf("%v hides selected action %q or moves focus\n%s", size, want, screen)
			}
			for _, fixed := range []string{"Kubeconfig File", "~/.kube/config", "Contexts", "Add Selected Contexts", "Cancel"} {
				if !strings.Contains(screen, fixed) {
					t.Fatalf("%v loses fixed section %q while scrolling\n%s", size, fixed, screen)
				}
			}
			press(t, m, "down")
		}
		typeText(m, ":")
		press(t, m, "tab")
		press(t, m, "shift+tab")
		if m.focus != 1 || !strings.Contains(ansi.Strip(m.render()), ":▏") {
			t.Fatal("command prompt must remain visible and modal")
		}
		press(t, m, "esc")
	}
}

func TestAddConnectionOnlyOffersImplementedTools(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Tools")
	m.beginToolSetup(false)
	screen := ansi.Strip(m.connectionPickerBody(100, 30))
	for _, name := range []string{"Kubernetes Cluster", "Source"} {
		if !strings.Contains(screen, name) {
			t.Fatalf("missing supported option %s", name)
		}
	}
	for _, name := range []string{"Terraform", "Argo", "Kargo", "Other Connections"} {
		if strings.Contains(screen, name) {
			t.Fatalf("unimplemented option remains: %s", name)
		}
	}
	if len(appsettings.ToolTypes) != 2 {
		t.Fatal("unsupported connection can still be selected")
	}
}
