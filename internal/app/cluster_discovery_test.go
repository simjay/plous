package app

import (
	"context"
	"errors"
	"image/color"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/vt"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/ui"
	"github.com/simjay/plous/internal/workspace"
)

func TestExploreReusesClusterDiscoveryAndExplicitRefreshInvalidatesIt(t *testing.T) {
	m := exploreModel(t)
	c := *m.savedConnection()
	cluster := m.clusterFor(m.active, c)
	cluster.Client, cluster.Catalog, cluster.Loaded = m.explore.Client, m.explore.Resources, true
	m.explore.Key = ""
	m.syncExplore()
	if m.explore.Connecting || m.explore.Client != cluster.Client || m.explore.Stream == nil || cluster.Cancel != nil {
		t.Fatal("Explore rediscovered a loaded cluster or did not start browsing")
	}
	m.explore.Preference.Namespace = ""
	m.rememberExplore()
	m.stopExplore()
	m.explore.Key = ""
	m.syncExplore()
	if m.explore.Preference.Namespace != "" || m.explore.Client != cluster.Client {
		t.Fatal("shared discovery lost all-namespace selection")
	}
	m.refreshExplore()
	if cluster.Loaded || m.explore.Key != "" {
		t.Fatal("explicit refresh reused the previous catalog")
	}
	if m.syncExplore() == nil || cluster.Cancel == nil || !m.explore.Connecting {
		t.Fatal("refresh did not start shared discovery")
	}
	if m.syncExplore() != nil || m.syncClusters() != nil {
		t.Fatal("pending discovery was started twice")
	}
	m.stopClusters()
}

func TestSharedDiscoveryCompletionCannotRestartInactiveExplore(t *testing.T) {
	m := exploreModel(t)
	previous := m.clusterFor(m.active, *m.savedConnection())
	m.explore.Key = ""
	m.syncExplore()
	run := previous.Run
	m.mode = 0
	m.syncExplore()
	m.applyClusterDiscovery(clusterDiscoveryResult{Cluster: previous, Run: run, Client: m.explore.Client, Catalog: []kubecommon.Resource{{Name: "pods", Version: "v1"}}})
	if m.syncExplore() != nil || m.explore.Key != "" || m.explore.Stream != nil {
		t.Fatal("background discovery restarted inactive Explore")
	}
	m.stopClusters()
}

func TestClusterCatalogDoesNotCrossCapturedTargets(t *testing.T) {
	m, _ := testModel(t)
	c := workspace.Connection{ID: "cluster", Tool: "kubernetes", TargetCaptured: true, Target: &kubecommon.Target{Fingerprint: "first"}}
	previous := m.clusterFor("workspace", c)
	previous.Loaded = true
	ctx, cancel := context.WithCancel(t.Context())
	previous.Cancel = cancel
	c.Target = &kubecommon.Target{Fingerprint: "replacement"}
	replacement := m.clusterFor("workspace", c)
	if replacement == previous || replacement.Loaded || ctx.Err() == nil {
		t.Fatal("changed target reused a catalog or left its discovery running")
	}
	c.Target = nil
	if missing := m.clusterFor("workspace", c); missing == replacement || missing.Loaded {
		t.Fatal("unavailable captured target reused the previous client")
	}
}

func TestClusterCapabilitiesShareCurrentAndSavedTree(t *testing.T) {
	for _, current := range []bool{false, true} {
		t.Run(map[bool]string{false: "saved", true: "current"}[current], func(t *testing.T) {
			m, _ := testModel(t)
			if current {
				m.currentPaths = currentFixture(t)
				resolveCurrentTest(t, m, "")
			} else {
				if !m.saveWorkspace(workspace.Workspace{Name: "Saved", Connections: []workspace.Connection{{Name: "alpha", Tool: "kubernetes", Settings: map[string]string{"kubeconfig": currentFixture(t), "context": "alpha", "namespace": "apps"}}}}) {
					t.Fatal(m.status)
				}
				m.selectScope(m.active, "kubernetes", m.current().Connections[0].ID)
			}
			t.Cleanup(m.stopClusters)
			c := *m.savedConnection()
			m.expanded[m.active], m.expanded[c.ID] = true, true
			if got := m.clusterCapabilities(m.active, c); !slices.Equal(got, []string{"kubernetes"}) {
				t.Fatal("pre-populated optional tools", got)
			}
			if m.syncClusters() == nil {
				t.Fatal("cluster omitted from discovery")
			}
			cluster := m.clusterStates[clusterKey(m.active, c)]
			if cluster == nil || cluster.Cancel == nil {
				t.Fatal("discovery not started")
			}
			catalog := []kubecommon.Resource{
				{Group: "argoproj.io", Version: "v1alpha1", Name: "applications", Kind: "Application", Namespaced: true},
				{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Name: "kustomizations", Kind: "Kustomization", Namespaced: true},
			}
			m.applyClusterDiscovery(clusterDiscoveryResult{Cluster: cluster, Run: cluster.Run, Target: c.Target, Catalog: catalog})
			if got := m.clusterCapabilities(m.active, c); !slices.Equal(got, []string{"kubernetes", "argocd", "flux"}) {
				t.Fatal(got)
			}
			_, cluster.HelmCancel = context.WithCancel(t.Context())
			cluster.HelmRun = 1
			m.applyHelmDetection(helmDetectionResult{Cluster: cluster, Run: 1, Driver: "configmap"})
			if got := m.clusterCapabilities(m.active, c); !slices.Equal(got, []string{"kubernetes", "helm", "argocd", "flux"}) {
				t.Fatal(got)
			}
			for _, tool := range []string{"kubernetes", "helm", "argocd", "flux"} {
				row := m.treeRows()[treeIndexFor(t, m, c.ID+"/view/"+tool)]
				if row.kind != "capability" || row.parent != c.ID || !row.isBranch() {
					t.Fatalf("divergent branch: %+v", row)
				}
				m.mode, m.focus, m.treeCursor = 1, 0, treeIndexFor(t, m, row.key)
				m.updateTree("enter")
				if m.tool != tool || m.connection != c.ID {
					t.Fatal("branch selected wrong scope")
				}
				active, _ := m.treeRowState(row)
				if !active {
					t.Fatal("selected tool is not styled active")
				}
			}
			m.tool = "helm"
			if m.currentConnection().Settings["driver"] != "configmap" || m.assistantConnections()[1].Driver != "configmap" || m.savedConnection().Settings["driver"] != "" {
				t.Fatal("detected driver not shared or persisted unexpectedly")
			}
		})
	}
}

func TestClusterRefreshRejectsLateDetectionAndPreservesCursor(t *testing.T) {
	m, v := gitOpsModel(t)
	c := v.Cluster
	m.expanded[m.active], m.expanded[c.Connection.ID] = true, true
	_, c.HelmCancel = context.WithCancel(t.Context())
	c.HelmRun = 1
	key := c.Connection.ID + "/view/argocd"
	m.treeCursor = treeIndexFor(t, m, key)
	m.Update(helmDetectionResult{Cluster: c, Run: 1, Driver: "secret"})
	if m.treeRows()[m.treeCursor].key != key {
		t.Fatal("tool insertion moved cursor")
	}
	c.refresh()
	m.applyHelmDetection(helmDetectionResult{Cluster: c, Run: 1, Driver: "configmap"})
	if c.HelmDriver != "secret" {
		t.Fatal("late detection changed driver")
	}
	_, c.HelmCancel = context.WithCancel(t.Context())
	c.HelmRun++
	m.applyHelmDetection(helmDetectionResult{Cluster: c, Run: c.HelmRun, Err: errors.New("denied")})
	if c.HelmDriver != "secret" {
		t.Fatal("unknown refresh removed observed Helm")
	}
	m.expanded[c.Connection.ID+"/view/helm"] = true
	m.treeCursor = treeIndexFor(t, m, c.Connection.ID+"/helm/Releases")
	_, c.HelmCancel = context.WithCancel(t.Context())
	c.HelmRun++
	m.Update(helmDetectionResult{Cluster: c, Run: c.HelmRun})
	if c.HelmDriver != "" || m.treeRows()[m.treeCursor].key != c.Connection.ID {
		t.Fatal("empty discovery retained Helm or moved cursor")
	}
	m.stopGitOps()
	m.stopClusters()
}

func TestTreeSelectionColorsCoverEveryCell(t *testing.T) {
	for _, kind := range []string{"current", "workspace", "connection", "capability", "category", "resource", "helmsection", "gitopssection"} {
		t.Run(kind, func(t *testing.T) {
			m, _ := testModel(t)
			m.focus, m.treeCursor = 0, 0
			row := treeRow{key: "selected", kind: kind, label: "Selected cluster", depth: 3, badge: "k8s"}
			lines := m.renderTreeRow([]treeRow{row}, 0, 48)
			terminal := vt.NewSafeEmulator(48, 3)
			defer terminal.Close()
			if _, err := terminal.Write([]byte(ui.CanvasStyle.Render(strings.Join(lines, "\n")))); err != nil {
				t.Fatal(err)
			}
			want := color.NRGBAModel.Convert(ui.Highlight)
			for x := 0; x < 48; x++ {
				cell := terminal.CellAt(x, 0)
				if cell == nil || cell.Style.Bg == nil || color.NRGBAModel.Convert(cell.Style.Bg) != want {
					t.Fatalf("selection background missing at column %d: %+v", x, cell)
				}
			}
			label := terminal.CellAt(9, 0)
			if label.Style.Fg == nil || color.NRGBAModel.Convert(label.Style.Fg) != color.NRGBAModel.Convert(ui.ShortcutStyle.GetForeground()) {
				t.Fatal("focused label has no selection color")
			}
		})
	}
}
