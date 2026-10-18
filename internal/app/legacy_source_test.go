package app

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	appcode "github.com/simjay/plous/internal/app/code"
	"github.com/simjay/plous/internal/integrations/manifest"
	"github.com/simjay/plous/internal/workspace"
)

func legacyKustomizeModel(t *testing.T) (*model, *appcode.CodeState) {
	t.Helper()
	m, _ := testModel(t)
	createWorkspace(t, m, "Commerce")
	root := t.TempDir()
	files := map[string]string{
		"payments/base/kustomization.yaml":          "resources: [deployment.yaml]\n",
		"payments/base/deployment.yaml":             "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: payments}\nspec: {replicas: 1}\n",
		"payments/overlays/dev/kustomization.yaml":  "resources: [../../base]\nreplicas: [{name: payments, count: 2}]\n",
		"payments/overlays/prod/kustomization.yaml": "resources: [../../base]\nreplicas: [{name: payments, count: 3}]\n",
		"storefront/base/kustomization.yaml":        "resources: []\n",
		"README.md":                                 "# Source documentation\n",
	}
	for name, text := range files {
		target := filepath.Join(root, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(target), 0700)
		if err := os.WriteFile(target, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	w := *m.current()
	w.Connections = []workspace.Connection{{Tool: "kustomize", Name: "platform-repo", Settings: map[string]string{"path": root}}}
	if !m.saveWorkspace(w) {
		t.Fatal(m.status)
	}
	m.selectScope(m.active, "files", m.current().Connections[0].ID)
	m.mode, m.focus = 1, 1
	k := m.coder()
	return m, k
}

func TestLegacyKustomizeConnectionRoutesThroughCode(t *testing.T) {
	m, s := legacyKustomizeModel(t)
	connection := m.connection
	m.expanded[m.active], m.expanded[m.active+"/code"], m.expanded[connection] = true, true, true
	m.selectScope(m.active, "", "")
	found := false
	for i, row := range m.treeRows() {
		if row.key == connection {
			found = true
			if row.tool != "files" || row.parent != m.active+"/code" {
				t.Fatal("legacy source did not project into Code", row)
			}
			m.treeCursor = i
		}
		if row.parent == connection && row.kind != "codeview" {
			t.Fatal("legacy source exposed an obsolete screen", row)
		}
	}
	if !found {
		t.Fatal("legacy connection missing from sidebar")
	}
	m.focus = 0
	m.updateTree("enter")
	if m.focus != 0 || m.mode != 1 || m.connection != connection || m.tool != "files" || !m.codeVisible() || m.coder().ViewID() != "files" {
		t.Fatal("sidebar did not open Code Files", m.focus, m.mode, m.connection, m.tool)
	}
	if s.Inventory != nil || s.Busy {
		t.Fatal("selecting a connection executed source work")
	}
	finishCode(t, m)
	if s.Inventory == nil || len(s.Builds) != 0 {
		t.Fatal("Code did not discover the saved source without rendering")
	}
	m.selectScope(m.active, "kustomize", connection)
	if m.tool != "manifests" || !m.codeVisible() || m.coder() != s || m.savedConnection().Tool != "kustomize" {
		t.Fatal("direct legacy scope did not normalize without rewriting identity")
	}
	s.Preference.Selected = "kustomize:payments/overlays/prod"
	s.Pending = "render"
	finishCode(t, m)
	if s.Builds[s.Preference.Selected] == nil {
		t.Fatal("legacy connection could not render through Code")
	}
}

func TestLegacyKustomizePreferencesSurviveRestart(t *testing.T) {
	for _, tool := range []string{"kustomize", "tfcode"} {
		for _, placeholder := range []bool{false, true} {
			name := tool
			if placeholder {
				name += "/placeholder"
			}
			t.Run(name, func(t *testing.T) {
				m, _ := legacyKustomizeModel(t)
				key := m.active + "/" + m.connection
				old := appcode.LegacyPreference{
					Fingerprint: "legacy-fingerprint", Selected: "entry:payments/overlays/prod",
					Open: "payments/overlays/prod", Tab: 2,
					Folded: map[string]bool{"payments": true}, Descending: true, Revision: "saved-revision",
				}
				m.kustomizePreferences = map[string]appcode.LegacyPreference{key: old}
				m.codeUI.Preferences = nil
				if placeholder {
					m.codeUI.Preferences = map[string]appcode.CodePreference{key: {}}
				}
				m.tool = tool
				if err := m.saveSession(); err != nil {
					t.Fatal(err)
				}
				if placeholder {
					var serialized struct {
						Code map[string]json.RawMessage `json:"code"`
					}
					if err := json.Unmarshal(m.lastSession, &serialized); err != nil {
						t.Fatal(err)
					}
					if got := string(serialized.Code[key]); got != "{}" {
						t.Fatalf("missing historical zero preference fixture: %q", got)
					}
				}
				restored, err := newModel(m.store, m.history, "test")
				if err != nil {
					t.Fatal(err)
				}
				restored.selectScope(m.active, "manifests", m.connection)
				if restored.tool != "manifests" || restored.connection != m.connection || restored.savedConnection().Tool != "kustomize" {
					t.Fatal("restored scope or connection identity changed")
				}
				state := restored.coder()
				if state.Preference.Selected != "kustomize:"+old.Open || state.Preference.Revision != old.Revision {
					t.Fatal("legacy selection or revision was not carried into Code", state.Preference)
				}
				if !reflect.DeepEqual(restored.kustomizePreferences[key], old) {
					t.Fatal("legacy preference fields changed on restore")
				}
				if err := restored.saveSession(); err != nil {
					t.Fatal(err)
				}
				var saved struct {
					Kustomize map[string]appcode.LegacyPreference `json:"kustomize"`
				}
				if err := json.Unmarshal(restored.lastSession, &saved); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(saved.Kustomize[key], old) {
					t.Fatal("legacy preference fields or JSON ownership changed on save")
				}
			})
		}
	}
}

func TestLegacyKustomizeGitComparisonKeepsInspectedRevision(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git unavailable")
	}
	t.Setenv("HOME", t.TempDir())
	m, local := legacyKustomizeModel(t)
	root := local.Config.Path
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
	git("commit", "-m", "first")
	first := git("rev-parse", "HEAD")
	filename := filepath.Join(root, "payments/base/deployment.yaml")
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(strings.ReplaceAll(string(data), "replicas: 1", "replicas: 4")), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "second")
	second := git("rev-parse", "HEAD")
	c := m.savedConnection()
	c.Settings["source_type"], c.Settings["path"], c.Settings["revision"] = "Git repository", "file://"+root, first
	if !m.saveWorkspace(*m.current()) {
		t.Fatal(m.status)
	}
	c = m.savedConnection()
	m.selectScope(m.active, "manifests", c.ID)
	s := m.coder()
	finishCode(t, m)
	s.Preference.Selected = "kustomize:payments/base"
	s.Pending, s.Other = "compare-revision", second
	finishCode(t, m)
	if s.ComparisonBase.Revision != first || s.Comparison.Revision != second || s.Inventory.Source.Revision != first || s.Preference.Revision != first {
		t.Fatal("comparison retargeted the inspected revision")
	}
	if !strings.Contains(manifest.YAML(s.Comparison.Resources[0].Object, false), "replicas: 4") || !strings.Contains(manifest.YAML(s.Builds[s.Preference.Selected].Resources[0].Object, false), "replicas: 1") {
		t.Fatal("comparison replaced the inspected build or compared the wrong revision")
	}
	s.Prompt("Git Revision", "missing-fixture-ref")
	m.submitCodeInput()
	cmd := m.syncCode()
	if cmd == nil {
		t.Fatal("revision selection did not schedule source loading")
	}
	applyCodeResult(t, m, cmd().(appcode.CodeResult))
	if s.Err == "" || s.Preference.Revision != first || s.Inventory.Source.Revision != first {
		t.Fatal("failed revision load replaced the retained source or selection", s.Err, s.Preference.Revision)
	}
	s.Prompt("Git Revision", second)
	m.submitCodeInput()
	if s.Preference.Revision != first {
		t.Fatal("requested revision replaced the saved selection before loading")
	}
	finishCode(t, m)
	if s.Preference.Revision != second || s.Inventory.Source.Revision != second {
		t.Fatal("successful revision load did not update the source and selection")
	}
	if saved := m.codeUI.Preferences[m.active+"/"+c.ID]; saved.Revision != second {
		t.Fatal("successful revision selection was not retained for restart", saved)
	}
	s.Prompt("Git Revision", "")
	m.submitCodeInput()
	if s.Preference.Revision != second {
		t.Fatal("blank revision replaced the saved selection before loading")
	}
	finishCode(t, m)
	if s.Inventory.Source.Revision != second {
		t.Fatal("blank revision did not resolve HEAD")
	}
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	restored, err := newModel(m.store, m.history, "test")
	if err != nil {
		t.Fatal(err)
	}
	restored.selectScope(m.active, "manifests", m.connection)
	if got := restored.coder().Preference.Revision; got != "HEAD" {
		t.Fatalf("explicit HEAD selection reverted on restart: %q", got)
	}
	finishCode(t, restored)
	if restored.coder().Inventory.Source.Revision != second {
		t.Fatal("restarted source did not load the selected HEAD")
	}
	if git("rev-parse", "HEAD") != second || git("status", "--porcelain") != "" {
		t.Fatal("comparison changed the source checkout")
	}
}
