package app

import (
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConnectionLabelsKeepMutedBadgesAndTruncateNames(t *testing.T) {
	m, _ := testModel(t)
	for _, badge := range []string{"k8s", "local", "remote", "tf"} {
		for _, width := range []int{20, 24, 32, 46} {
			for _, focused := range []bool{false, true} {
				m.focus = 1
				if focused {
					m.focus = 0
				}
				row := treeRow{kind: "connection", key: "cluster", connection: "cluster", tool: "files", label: "production-長い-connection-name-that-does-not-fit", badge: badge, depth: 2}
				lines := m.renderTreeRow([]treeRow{row}, 0, width)
				if len(lines) != 1 {
					t.Fatalf("%s wrapped at width %d", badge, width)
				}
				line := ansi.Strip(lines[0])
				if ansi.StringWidth(line) != width || !strings.Contains(line, "... ("+badge+")") {
					t.Fatalf("name displaced badge at %d: %q", width, line)
				}
				badgeStyle := ui.Muted
				if focused {
					badgeStyle = badgeStyle.Background(ui.Highlight)
				}
				if !strings.Contains(lines[0], badgeStyle.Render(" ("+badge+")")) {
					t.Fatalf("%s badge lost its muted style", badge)
				}
			}
		}
	}
}

func TestClusterIdentityNormalizesPathsButPreservesAccessOptions(t *testing.T) {
	testModel(t)
	path := installDefaultKubeconfig(t, contextFile(t, "minikube"))
	base := workspace.Connection{Tool: "kubernetes", Settings: map[string]string{"context": "minikube", "kubeconfig": path}}
	link := filepath.Join(t.TempDir(), "config-link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"~/.kube/config", filepath.Dir(path) + "/../.kube/config", link} {
		other := base
		other.Tool, other.Settings = "helm", maps.Clone(base.Settings)
		other.Settings["kubeconfig"], other.Settings["driver"] = alias, "secret"
		if !maps.Equal(clusterSettings(base), clusterSettings(other)) {
			t.Fatal("equivalent Kubernetes/Helm connection was not recognized", alias)
		}
	}
	for key, value := range map[string]string{"context": "other", "namespace": "team", "driver": "configmap", "repository_config": "private-repositories", "future": "preserved"} {
		other := base
		other.Settings = maps.Clone(base.Settings)
		other.Settings[key] = value
		if maps.Equal(clusterSettings(base), clusterSettings(other)) {
			t.Fatal("distinct access options were merged", key)
		}
	}
}

func TestLegacyIdenticalMonitorRowsPreserveActiveIdentityAndHistory(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Legacy")
	path := contextFile(t, "minikube")
	w := *m.current()
	w.Connections = []workspace.Connection{
		{Tool: "kubernetes", Name: "minikube", Settings: map[string]string{"context": "minikube", "kubeconfig": path}},
		{Tool: "helm", Name: "minikube", Settings: map[string]string{"context": "minikube", "kubeconfig": path, "driver": "secret"}},
		{Tool: "helm", Name: "minikube", Settings: map[string]string{"context": "minikube", "kubeconfig": path, "driver": "configmap"}},
	}
	if !m.saveWorkspace(w) {
		t.Fatal(m.status)
	}
	before, _ := m.store.History(m.active)
	beforeJSON, _ := json.Marshal(before)
	active := m.current().Connections[1].ID
	m.selectScope(m.active, "helm", active)
	m.expanded[active] = true
	connections := []string{}
	for _, row := range m.treeRows() {
		if row.kind == "connection" && row.workspace == m.active {
			connections = append(connections, row.connection)
		}
	}
	if len(connections) != 2 || connections[0] != active || len(m.current().Connections) != 3 {
		t.Fatal("duplicate row was not grouped without losing the active identity", connections)
	}
	after, _ := m.store.History(m.active)
	afterJSON, _ := json.Marshal(after)
	if string(beforeJSON) != string(afterJSON) {
		t.Fatal("sidebar grouping rewrote history")
	}
}

func TestContextCustomNameIsPendingAndDoesNotChangeKubeconfig(t *testing.T) {
	m, _ := testModel(t)
	context := "arn:aws:eks:us-east-2:123456789012:cluster/production-platform"
	path := installDefaultKubeconfig(t, contextFile(t, context, "staging"))
	before, _ := os.ReadFile(path)
	createWorkspace(t, m, "Platform")
	press(t, m, "enter")
	press(t, m, "space")
	picker := m.form
	press(t, m, "right")
	if m.form.Kind != "contextname" {
		t.Fatal("name control did not open")
	}
	if m.form.Fields[0].Value.Text != "" {
		t.Fatal("optional name requires clearing the context before typing")
	}
	press(t, m, "ctrl+u")
	typeText(m, "Production")
	press(t, m, "enter")
	m.execute("help")
	m.closeHelp()
	press(t, m, "enter")
	if m.form != picker || !m.chosen[0] || m.pendingContextName(0) != "Production" || len(m.current().Connections) != 0 {
		t.Fatal("naming submitted the connection or lost the selection")
	}
	press(t, m, "right")
	press(t, m, "ctrl+u")
	typeText(m, "cancelled name")
	press(t, m, "esc")
	if m.pendingContextName(0) != "Production" {
		t.Fatal("cancelled edit replaced the pending name")
	}
	addSelectedContexts(t, m)
	c := m.current().Connections[0]
	if c.Name != "Production" || c.Settings["context"] != context {
		t.Fatal("custom label changed the actual context", c)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("naming changed kubeconfig")
	}
}

func TestContextPickerBlocksRenamedClusterAcrossToolsAndOptions(t *testing.T) {
	m, _ := testModel(t)
	path := installDefaultKubeconfig(t, contextFile(t, "minikube"))
	createWorkspace(t, m, "Platform")
	w := *m.current()
	w.Connections = []workspace.Connection{{Tool: "helm", Name: "Local cluster", Settings: map[string]string{"context": "minikube", "kubeconfig": path, "namespace": "another-team", "driver": "configmap"}}}
	if !m.saveWorkspace(w) {
		t.Fatal(m.status)
	}
	press(t, m, "enter")
	if !m.contextAlreadyAdded(0) {
		t.Fatal("renamed or customized cluster could be added twice")
	}
	press(t, m, "space")
	press(t, m, "right")
	if m.chosen[0] || m.form.Kind != "kube" {
		t.Fatal("already-added context accepted selection or a new name")
	}
	m.chosen[0] = true
	m.addContexts()
	if len(m.current().Connections) != 1 || !strings.Contains(m.status, "already") {
		t.Fatal("save bypassed duplicate protection")
	}
}

func TestConnectionFormRejectsDuplicateButAllowsLegacyRename(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Platform")
	w := *m.current()
	w.Connections = []workspace.Connection{{Tool: "kubernetes", Name: "Cluster", Settings: map[string]string{"context": "minikube", "kubeconfig": contextFile(t, "minikube")}}}
	if !m.saveWorkspace(w) {
		t.Fatal(m.status)
	}
	c := m.current().Connections[0]
	m.toolSetup = nil
	m.beginConnection("helm", nil)
	for i := range m.form.Fields {
		f := &m.form.Fields[i]
		if value, ok := c.Settings[f.Key]; ok {
			f.Value = textview.NewField(value)
		} else if f.Key == "name" {
			f.Value = textview.NewField("Different label")
		} else if f.Key == "namespace" {
			f.Value = textview.NewField("another-team")
		}
	}
	m.submitForm()
	if m.form == nil || len(m.current().Connections) != 1 || !strings.Contains(m.status, "already exists") {
		t.Fatal("manual form accepted duplicate cluster", m.status)
	}
	legacy := c
	legacy.ID, legacy.Tool = "", "helm"
	w = *m.current()
	w.Connections = append(w.Connections, legacy)
	if !m.saveWorkspace(w) {
		t.Fatal(m.status)
	}
	m.beginConnection(c.Tool, &c)
	m.form.Fields[0].Value = textview.NewField("Renamed cluster")
	m.submitForm()
	if m.current().Connections[0].Name != "Renamed cluster" || m.current().Connections[0].ID != c.ID {
		t.Fatal("renaming changed identity", fmt.Sprint(m.current().Connections))
	}
}

func TestContextNamingMouseAndCompactLayouts(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 32}, {180, 42}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			m, _ := testModel(t)
			m.width, m.height = size[0], size[1]
			context := "arn:aws:eks:us-east-2:123456789012:cluster/production-platform"
			installDefaultKubeconfig(t, contextFile(t, context, "staging"))
			createWorkspace(t, m, "Platform")
			press(t, m, "enter")
			capture := func(scene string, labels ...string) {
				t.Helper()
				view := m.View().Content
				plain := ansi.Strip(view)
				for _, label := range labels {
					if !strings.Contains(plain, label) {
						t.Fatalf("%s hides %q\n%s", scene, label, plain)
					}
				}
				lines := strings.Split(view, "\n")
				if len(lines) != m.height {
					t.Fatalf("%s: %d rows, want %d", scene, len(lines), m.height)
				}
				for _, line := range lines {
					if ansi.StringWidth(line) != m.width {
						t.Fatalf("%s exceeds terminal width: %q", scene, ansi.Strip(line))
					}
				}
				if dir := os.Getenv("PLOUS_REVIEW_DIR"); dir != "" {
					if err := os.MkdirAll(dir, 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("connections-%s-%d.txt", scene, m.width)), []byte(plain), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			capture("picker", "[→ Name]", "Add Selected Contexts")
			clickMouse(t, m, "main", "contextname/0")
			capture("name", "Name This Connection", "Name", "Use name")
			press(t, m, "ctrl+u")
			typeText(m, "Production")
			clickMouse(t, m, "main", "formaction/0")
			capture("named", "[ ] Production")
			press(t, m, "space")
			addSelectedContexts(t, m)
			m.beginToolSetup(true)
			capture("duplicate", "Production · Already added")
			m.current().Connections[0].Name = strings.Repeat("production-platform-", 6)
			capture("duplicate-long", "... · Already added")
			press(t, m, "down")
			press(t, m, "right")
			press(t, m, "ctrl+u")
			typeText(m, "Pending staging")
			clickMouse(t, m, "main", "formaction/0")
			clickMouse(t, m, "main", "context/1")
			if m.pendingContextName(1) != "staging" {
				t.Fatal("refresh retained a stale pending name")
			}
		})
	}
}
