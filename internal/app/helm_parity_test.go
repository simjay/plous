package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/ui"
)

func TestHelmNamespacePickerReadsCapturedContextAndAppliesScope(t *testing.T) {
	m, h := helmModel(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != "GET" || r.URL.Path != "/api/v1/namespaces" {
			t.Errorf("unexpected namespace request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"apiVersion":"v1","kind":"NamespaceList","items":[{"metadata":{"name":"apps"}},{"metadata":{"name":"production"}}]}`)
	}))
	defer server.Close()
	config := fmt.Sprintf("apiVersion: v1\nkind: Config\ncurrent-context: wrong\ncontexts:\n- name: wrong\n  context: {cluster: wrong}\n- name: staging\n  context: {cluster: fixture, namespace: apps}\nclusters:\n- name: wrong\n  cluster: {server: 'https://wrong.invalid'}\n- name: fixture\n  cluster: {server: '%s'}\n", server.URL)
	if err := os.WriteFile(h.Config.Kubeconfig, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	var err error
	h.Config.Target, err = kubecommon.ResolveTarget(h.Config.Kubeconfig, h.Config.Context, h.Config.Namespace)
	if err != nil {
		t.Fatal(err)
	}
	m.updateHelm("n")
	if h.Namespaces == nil || !m.loadingVisible() {
		t.Fatal("n did not open the loading picker")
	}
	cmd := m.syncHelmNamespaces(h)
	m.updateHelmNamespaceResult(cmd().(monitor.HelmNamespaceResult))
	if requests.Load() != 1 || h.Namespaces.Err != "" || !h.Namespaces.Loaded {
		t.Fatalf("namespace request failed: %#v", h.Namespaces)
	}
	view := ansi.Strip(m.helmBody(80, 24))
	for _, text := range []string{"All namespaces", "apps  ✓", "production", "Enter namespace manually"} {
		if !strings.Contains(view, text) {
			t.Fatalf("picker missing %q: %s", text, view)
		}
	}
	typeText(m, "prod")
	press(t, m, "enter")
	if h.Preference.Namespace != "production" || h.Namespaces != nil || h.Input != nil || h.Loaded || len(h.Releases) != 0 {
		t.Fatal("selection failed to reset inventory for the new scope")
	}
	if m.currentConnection().Settings["namespace"] != "apps" || m.helmPreferences[h.Workspace+"/"+h.Connection].Namespace != "production" {
		t.Fatal("browsing scope changed connection settings or was not persisted")
	}
	if data, _ := os.ReadFile(h.Config.Kubeconfig); string(data) != config {
		t.Fatal("namespace selection changed the external context")
	}
}

func TestHelmNamespaceCancellationManualFallbackAndStaleResults(t *testing.T) {
	m, h := helmModel(t)
	h.Detail = &helmtool.Snapshot{Name: "shop", Namespace: "apps"}
	detail := h.Detail
	m.executeHelm("namespace")
	ctx, cancel := context.WithCancel(context.Background())
	old := &monitor.NamespaceRequest{Cancel: cancel}
	h.Namespaces.Run = old
	press(t, m, "esc")
	if ctx.Err() == nil || h.Detail != detail || h.Preference.Namespace != "apps" {
		t.Fatal("cancelling namespace picker lost scope or detail")
	}
	m.executeHelm("namespace")
	m.updateHelmNamespaceResult(monitor.HelmNamespaceResult{State: h, Run: old, Names: []string{"wrong"}})
	if h.Namespaces.Loaded || len(h.Namespaces.Names) != 0 {
		t.Fatal("stale namespace result entered a new picker")
	}
	h.Namespaces.Loaded, h.Namespaces.Err = true, "Access denied"
	typeText(m, "allowed-team")
	press(t, m, "enter")
	if !h.Namespaces.Manual || h.Input.Text != "allowed-team" {
		t.Fatal("manual fallback lost the search query")
	}
	m.Update(tea.PasteMsg{Content: " bb :quit"})
	press(t, m, "enter")
	if m.status == "" || h.Preference.Namespace != "apps" || m.command != nil || m.leftHidden {
		t.Fatal("invalid or pasted namespace input executed navigation")
	}
	press(t, m, "esc")
	if h.Namespaces.Manual || h.Input.Text != "allowed-team" || h.Detail != detail {
		t.Fatal("manual entry did not return to the preceding picker")
	}
	press(t, m, "enter")
	press(t, m, "enter")
	if h.Preference.Namespace != "allowed-team" || h.Namespaces != nil || h.Detail != nil {
		t.Fatal("manual namespace was not applied")
	}
}

func TestHelmAllNamespacesAndCancelledListCannotRestoreOldInventory(t *testing.T) {
	m, h := helmModel(t)
	h.Releases = []helmtool.Release{{Name: "old", Namespace: "apps"}}
	h.Busy, h.Running, h.Run = true, "list", 3
	ctx, cancel := context.WithCancel(context.Background())
	h.Cancel = cancel
	m.executeHelm("namespace")
	if ctx.Err() == nil || h.Busy {
		t.Fatal("namespace picker was blocked by an old read")
	}
	h.Namespaces.Loaded = true
	press(t, m, "home")
	press(t, m, "enter")
	m.updateHelmResult(monitor.HelmResult{State: h, Run: 3, Kind: "list", Releases: []helmtool.Release{{Name: "stale"}}})
	if h.Preference.Namespace != "*" || len(h.Releases) != 0 || strings.Contains(ansi.Strip(m.helmBody(80, 24)), "stale") {
		t.Fatal("all-namespace scope accepted a cancelled inventory")
	}
}

func TestHelmSortAndRefreshKeepSelectedIdentity(t *testing.T) {
	m, h := helmModel(t)
	h.Releases = []helmtool.Release{
		{Name: "alpha", Namespace: "apps", Chart: "demo", Version: "1.0", Revision: 12, Status: "failed"},
		{Name: "beta", Namespace: "apps", Chart: "demo", Version: "2.0", Revision: 2, Status: "deployed"},
		{Name: "gamma", Namespace: "other", Chart: "demo", Version: "2.0", Revision: 1, Status: "deployed"},
	}
	m.updateHelm("down")
	key := h.CollectionSelectionKey()
	m.updateHelm("S")
	for i, label := range h.Menu {
		if label == "Revision" {
			h.Choice = i
		}
	}
	m.updateHelm("enter")
	if h.CollectionSelectionKey() != key || h.ReleaseRows()[0].Revision != 1 || h.ReleaseRows()[2].Revision != 12 {
		t.Fatal("sorting moved the selection or sorted revisions as text")
	}
	if !strings.Contains(ansi.Strip(m.helmBody(80, 24)), "↑") {
		t.Fatal("table header hid the active sort direction")
	}
	h.Preference.Filter = "12"
	if rows := h.ReleaseRows(); len(rows) != 1 || rows[0].Name != "alpha" {
		t.Fatal("filter did not search the visible revision column")
	}
	h.Preference.Filter = ""
	m.updateHelmResult(monitor.HelmResult{State: h, Run: h.Run, Kind: "list", Releases: append(h.Releases, helmtool.Release{Name: "new", Namespace: "apps"})})
	if h.CollectionSelectionKey() != key {
		t.Fatal("refresh changed the selected release")
	}
	h.Preference.Filter = "beta"
	h.Cursor = 0
	m.updateHelm("backspace")
	if h.CollectionSelectionKey() != key || h.Preference.Sort != "Revision" || h.Preference.Namespace != "apps" {
		t.Fatal("clearing a filter lost selection, sort, or namespace")
	}
	m.chooseHelmSection("Charts")
	h.Cursor = 1
	key = h.CollectionSelectionKey()
	m.updateHelmResult(monitor.HelmResult{State: h, Run: h.Run, Kind: "list", Releases: h.Releases})
	if h.CollectionSelectionKey() != key {
		t.Fatal("refresh changed the selected chart release")
	}
	m.updateHelm("enter")
	if h.Preference.Selected != key || h.Pending != "detail" {
		t.Fatal("chart card opened another release")
	}
}

func TestHelmTablesShareScopeStylesAndFitPopulatedViews(t *testing.T) {
	m, h := helmModel(t)
	h.Preference.Namespace = "*"
	h.Releases = []helmtool.Release{
		{Name: "backend", Namespace: "commerce", Chart: "podinfo", Version: "6.11.2", Revision: 4, Status: "deployed"},
		{Name: "frontend", Namespace: "production", Chart: "podinfo", Version: "6.11.2", Revision: 12, Status: "pending-upgrade"},
		{Name: "payments", Namespace: "production", Chart: "checkout", Version: "1.0.0", Revision: 3, Status: "failed"},
	}
	view := m.helmBody(140, 24)
	text := ansi.Strip(view)
	for _, want := range []string{"ctx staging", "[ all namespaces ▾ ] n", "Helm / Releases", "Name ↑", "Namespace", "Status", "Chart", "Version", "Revision", "3 releases · all namespaces"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, "Commerce / Helm") || strings.Contains(text, "Releases · Charts") || strings.Contains(text, "[ g Refresh ]") {
		t.Fatal("redundant old collection chrome remains")
	}
	for state, style := range map[string]lipgloss.Style{"deployed": ui.AddedStyle, "failed": ui.RemovedStyle, "pending-upgrade": ui.ChangedStyle} {
		r, g, b, _ := style.GetForeground().RGBA()
		color := fmt.Sprintf("38;2;%d;%d;%d", r>>8, g>>8, b>>8)
		found := false
		for _, line := range strings.Split(view, "\n") {
			if strings.Contains(line, state) && strings.Contains(line, color) {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s has no semantic status styling", state)
		}
	}

	for _, section := range []string{"Releases", "Charts"} {
		h.Preference.Section = section
		for _, size := range [][2]int{{28, 10}, {50, 16}, {80, 24}, {160, 40}} {
			view := m.helmBody(size[0], size[1])
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("%s overflow at %v", section, size)
			}
		}
	}
}

func TestHelmNamespaceCommandsAndPreparedPlanKeepTheirTargets(t *testing.T) {
	m, h := helmModel(t)
	for _, command := range []string{"N production", "namespace *", "N apps"} {
		mainCommand(t, m, command)
		if h.Namespaces != nil || h.Input != nil || h.Detail != nil {
			t.Fatal("explicit namespace command opened an editor or retained a detail")
		}
	}
	if h.Preference.Namespace != "apps" {
		t.Fatal("namespace command did not apply its argument")
	}
	mainCommand(t, m, "N invalid namespace")
	if h.Preference.Namespace != "apps" || m.status == "" {
		t.Fatal("invalid namespace changed the scope")
	}
	h.Busy, h.Pending = false, ""
	h.BeginWorkflow()
	h.Plan = &helmtool.Plan{Operation: "preview", Context: "staging", Namespace: "reviewed", Name: "demo"}
	m.executeHelm("namespace other")
	if h.Plan.Namespace != "reviewed" || h.Preference.Namespace != "apps" || h.Namespaces != nil {
		t.Fatal("namespace command silently retargeted a prepared plan")
	}
	m.executeHelm("refresh")
	if h.Pending != "" || h.Plan == nil {
		t.Fatal("Plan refresh started an unrelated inventory read or discarded the preview")
	}
}

func TestHelmSummaryIncludesChartAppVersion(t *testing.T) {
	m, h := helmModel(t)
	h.Detail = &helmtool.Snapshot{Name: "demo", Namespace: "apps"}
	h.Detail.Chart.Metadata = map[string]any{"name": "example", "version": "1.0.0", "appVersion": "7.2.1"}
	if view := ansi.Strip(m.helmBody(110, 30)); !strings.Contains(view, "App version:") || !strings.Contains(view, "7.2.1") {
		t.Fatal("recorded chart app version missing from release summary", view)
	}
}
