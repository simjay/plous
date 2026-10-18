package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/simjay/plous/internal/app/monitor"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/workspace"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
)

func gitOpsModel(t *testing.T) (*model, *gitOpsView) {
	t.Helper()
	m, _ := testModel(t)
	createWorkspace(t, m, "Platform")
	w := *m.current()
	w.Connections = []workspace.Connection{{Tool: "kubernetes", Name: "production", Settings: map[string]string{"kubeconfig": contextFile(t, "production"), "context": "production"}}}
	if !m.saveWorkspace(w) {
		t.Fatal(m.status)
	}
	m.selectScope(m.active, "argocd", m.current().Connections[0].ID)
	m.mode, m.focus = 1, 1
	v := m.gitOpsCurrent()
	v.Cluster.Loaded = true
	v.Cluster.Catalog = []kubecommon.Resource{
		{Group: "argoproj.io", Version: "v1alpha1", Name: "applications", Kind: "Application", Namespaced: true, Verbs: []string{"get", "list"}},
		{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Name: "kustomizations", Kind: "Kustomization", Namespaced: true, Verbs: []string{"get", "list"}},
		{Group: "apps", Version: "v1", Name: "deployments", Kind: "Deployment", Namespaced: true, Verbs: []string{"get", "list"}},
	}
	v.State.Apply(gitops.Snapshot{State: gitops.Available, ReadAt: time.Now(), Items: []gitops.Item{{Ref: gitops.Reference{Group: "argoproj.io", Version: "v1alpha1", Kind: "Application", Name: "payments", Namespace: "argocd"}, Sync: "Synced", Health: "Degraded", Destination: gitops.Destination{Server: "https://kubernetes.default.svc", Namespace: "shop"}, Reason: "Deployment did not become ready", Resources: []gitops.ManagedResource{{Ref: gitops.Reference{Group: "apps", Kind: "Deployment", Namespace: "shop", Name: "payments"}}}}}})
	return m, v
}

func TestGitOpsSharesSavedConnectionAndHasCategoryBranches(t *testing.T) {
	m, v := gitOpsModel(t)
	c := m.savedConnection()
	if c.Tool != "kubernetes" || m.currentConnection().Tool != "argocd" || m.currentConnection().ID != c.ID {
		t.Fatal("changed saved connection identity")
	}
	m.expanded[m.active], m.expanded[c.ID] = true, true
	leaves := map[string]bool{}
	for _, row := range m.treeRows() {
		if gitOpsTool(row.tool) {
			if row.parent != c.ID || row.depth != 4 || !row.isBranch() {
				t.Fatalf("incorrect sibling: %+v", row)
			}
			leaves[row.tool] = true
		}
	}
	if !leaves["argocd"] || !leaves["flux"] {
		t.Fatal(leaves)
	}
	v.Cluster.Loaded = false
	if len(m.gitOpsCapabilities(m.active, *c)) != 2 {
		t.Fatal("refresh removed existing capabilities")
	}
}

func TestGitOpsCollapsedConnectionDoesNotDiscover(t *testing.T) {
	m, v := gitOpsModel(t)
	m.connection, m.tool = "", ""
	m.expanded[v.Cluster.Connection.ID] = false
	m.clusterStates = nil
	m.gitOpsViews = nil
	if cmd := m.syncClusters(); cmd != nil || len(m.clusterStates) != 0 {
		t.Fatal("collapsed connection initiated a cluster read")
	}
}

func TestGitOpsInputOwnsShortcutsAndPaste(t *testing.T) {
	m, v := gitOpsModel(t)
	m.updateGitOps("/")
	for _, key := range []string{"f", "f", ":", "c", "c"} {
		msg := localMouseKey(key)
		msg.Text = key
		m.Update(msg)
	}
	if v.State.Input == nil || v.State.Input.Text != "ff:cc" || m.command != nil || m.chatEditing || m.viewExpanded() {
		t.Fatal("filter lost literal key ownership")
	}
	m.gitOpsPaste("/model\n")
	if !strings.Contains(v.State.Input.Text, "/model") {
		t.Fatal("filter paste was lost")
	}
	m.Update(localMouseKey("esc"))
	if v.State.Input != nil {
		t.Fatal("Escape did not close filter")
	}
}

func TestGitOpsLateResultsCannotReopenCancelledDetails(t *testing.T) {
	m, v := gitOpsModel(t)
	v.State.Open()
	ctx, cancel := context.WithCancel(context.Background())
	v.Cancel, v.Run, v.State.Loading = cancel, 4, true
	m.updateGitOps("esc")
	if ctx.Err() == nil || v.Cancel != nil {
		t.Fatal("read not cancelled")
	}
	m.applyGitOpsResult(gitOpsResult{View: v, Run: 4, Kind: "detail", Item: gitops.Item{Ref: gitops.Reference{Name: "late"}}})
	if v.State.Detail == nil || v.State.Detail.Ref.Name != "payments" {
		t.Fatal("late result changed detail")
	}
	m.updateGitOps("esc")
	if v.State.Detail != nil {
		t.Fatal("could not leave previous detail")
	}
}

func TestGitOpsResourceBackPreservesOriginAndKubernetesPreference(t *testing.T) {
	m, v := gitOpsModel(t)
	v.State.Query = "payments"
	v.State.Open()
	old := monitor.ExplorePreference{Resource: "pods", Namespace: "original", Filter: "keep"}
	m.explorePreferences = map[string]monitor.ExplorePreference{m.active + "/" + m.connection: old}
	client, err := kubernetes.NewForConfig(&rest.Config{Host: "https://example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	v.Run = 7
	_, v.Cancel = context.WithCancel(context.Background())
	object := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]any{"name": "payments", "namespace": "shop", "uid": "deployment-1"}}}
	m.applyGitOpsResource(gitOpsResourceResult{View: v, Run: 7, Client: client, Connection: v.Cluster.Connection, Catalog: v.Cluster.Catalog, Resource: v.Cluster.Catalog[2], Object: object})
	if !m.exploreVisible() || m.explore.Detail.GetName() != "payments" || m.gitOpsReturn == nil {
		t.Fatal("resource did not open Kubernetes")
	}
	m.rememberExplore()
	if m.explorePreferences[m.active+"/"+m.connection] != old {
		t.Fatal("drill-down replaced saved Kubernetes scope")
	}
	m.backExploreLocation()
	if !m.gitOpsVisible() || m.gitOpsCurrent() != v || v.State.Detail == nil || v.State.Query != "payments" {
		t.Fatal("Back lost GitOps origin")
	}
}

func TestGitOpsDestinationUsesSavedEndpointOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	data := "apiVersion: v1\nkind: Config\nclusters:\n- name: origin\n  cluster:\n    server: https://origin.invalid\n- name: target\n  cluster:\n    server: https://target.invalid\ncontexts:\n- name: origin\n  context:\n    cluster: origin\n- name: target\n  context:\n    cluster: target\ncurrent-context: origin\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	origin := workspace.Connection{ID: "one", Tool: "kubernetes", Settings: map[string]string{"kubeconfig": path, "context": "origin"}}
	target := workspace.Connection{ID: "two", Tool: "helm", Settings: map[string]string{"kubeconfig": path, "context": "target"}}
	got, err := gitOpsDestinationConnection(origin, []workspace.Connection{origin, target}, gitops.ArgoCD, gitops.Destination{Server: "https://target.invalid", Name: "misleading"})
	if err != nil || got.ID != "two" {
		t.Fatalf("saved endpoint not matched: %v", err)
	}
	for _, destination := range []gitops.Destination{{Name: "target"}, {Server: "https://unsaved.invalid"}, {Remote: true}} {
		if _, err := gitOpsDestinationConnection(origin, []workspace.Connection{target}, gitops.ArgoCD, destination); err == nil {
			t.Fatal("unverified destination was accepted")
		}
	}
	got, err = gitOpsDestinationConnection(origin, nil, gitops.ArgoCD, gitops.Destination{Server: "https://kubernetes.default.svc"})
	if err != nil || got.ID != origin.ID {
		t.Fatal("in-cluster destination not supported")
	}
}

func TestGitOpsLegacyHelmKeepsItsDefaultKubeconfig(t *testing.T) {
	_, v := gitOpsModel(t)
	origin := v.Cluster.Connection
	origin.Tool = "helm"
	delete(origin.Settings, "kubeconfig")
	if clusterKubeconfig(origin) != "~/.kube/config" {
		t.Fatal("legacy Helm target inherited KUBECONFIG")
	}
	origin.Tool = "kubernetes"
	if clusterKubeconfig(origin) != "" {
		t.Fatal("Kubernetes environment configuration was lost")
	}
}

func TestGitOpsMouseSelectAndOpen(t *testing.T) {
	m, v := gitOpsModel(t)
	m.mouseChoose(0, false)
	if v.State.Detail != nil {
		t.Fatal("single click opened detail")
	}
	m.mouseChoose(0, true)
	if v.State.Detail == nil || v.State.Detail.Ref.Name != "payments" {
		t.Fatal("double click failed to open selection")
	}
}

func TestGitOpsNamespaceCommandKeepsControllerScope(t *testing.T) {
	m, v := gitOpsModel(t)
	m.execute("N argocd")
	if m.tool != "argocd" || v.State.Namespace != "argocd" || v.State.Loaded {
		t.Fatal("namespace command left the controller view")
	}
	m.execute("N bad/name")
	if v.State.Namespace != "argocd" || !strings.Contains(m.status, "namespace name") {
		t.Fatal("invalid namespace accepted")
	}
}

func TestGitOpsPreferencesSurviveFailureAndRestart(t *testing.T) {
	m, v := gitOpsModel(t)
	v.State.Namespace = "argocd"
	m.rememberGitOps(v)
	selected := v.State.SelectedKey()
	v.State.Snapshot = gitops.Snapshot{State: gitops.ReadDenied}
	m.rememberGitOps(v)
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	restarted, err := newModel(m.store, m.history, "test")
	if err != nil {
		t.Fatal(err)
	}
	p := restarted.gitOpsPreferences[m.active+"/"+m.connection+"/argocd"]
	if p.Namespace != "argocd" || p.Selected != selected {
		t.Fatalf("lost saved scope: %+v", p)
	}
	if restarted.connection != "" || restarted.tool != "" {
		t.Fatal("restart reopened connection")
	}
}

func TestGitOpsAssistantHintUsesCapturedKubernetesIdentity(t *testing.T) {
	m, v := gitOpsModel(t)
	v.State.Open()
	v.State.Detail.Reason = "private-controller-body"
	page := m.assistantPageContext()
	if page.Tool != "kubernetes" || page.ConnectionID != m.connection || page.Resource != "applications.argoproj.io" || page.Name != "payments" || page.Namespace != "argocd" {
		t.Fatalf("wrong read target: %+v", page)
	}
	if strings.Contains(m.contextText(), "private-controller-body") {
		t.Fatal("controller body entered automatic context")
	}
	m.savedConnection().Tool = "helm"
	if page = m.assistantPageContext(); page.ConnectionID != m.connection+"/kubernetes" {
		t.Fatal("legacy Helm projection identity lost")
	}
}

func TestGitOpsCategoriesKeepIndependentScopeAndRejectLateReads(t *testing.T) {
	m, applications := gitOpsModel(t)
	applications.State.Namespace, applications.State.Query = "argocd", "payments"
	ctx, cancel := context.WithCancel(context.Background())
	applications.Cancel, applications.Run, applications.State.Loading = cancel, 7, true
	m.chooseGitOpsSection("Projects")
	projects := m.gitOpsCurrent()
	if projects == applications || projects.Cluster != applications.Cluster || ctx.Err() == nil {
		t.Fatal("category did not isolate its view or cancel the old read")
	}
	m.applyGitOpsResult(gitOpsResult{View: applications, Run: 7, Kind: "list", Snapshot: gitops.Snapshot{State: gitops.Empty}})
	if len(applications.State.Rows()) != 1 {
		t.Fatal("late read replaced inactive category")
	}
	projects.State.Namespace, projects.State.Query = "other", "project"
	m.rememberGitOps(projects)
	m.chooseGitOpsSection("Applications")
	if m.gitOpsCurrent() != applications || applications.State.Namespace != "argocd" || applications.State.Query != "payments" {
		t.Fatal("switch lost application scope")
	}
	m.chooseGitOpsSection("Projects")
	if m.gitOpsCurrent() != projects || projects.State.Namespace != "other" || projects.State.Query != "project" {
		t.Fatal("switch lost project scope")
	}
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	restarted, err := newModel(m.store, m.history, "test")
	if err != nil {
		t.Fatal(err)
	}
	restarted.selectScope(m.active, "argocd", m.connection)
	restored := restarted.gitOpsCurrent()
	if restored.State.Section != gitops.Projects || restored.State.Namespace != "other" {
		t.Fatal("restart lost chosen category or namespace")
	}
}

func TestGitOpsTreeCategoriesExpandSelectAndCollapse(t *testing.T) {
	m, _ := gitOpsModel(t)
	m.expanded[m.active], m.expanded[m.connection] = true, true
	m.expanded[m.connection+"/view/argocd"], m.expanded[m.connection+"/view/flux"] = true, true
	counts := map[string]int{}
	for i, row := range m.treeRows() {
		if row.kind != "gitopssection" {
			continue
		}
		if row.isBranch() || row.depth != 5 || row.parent != m.connection+"/view/"+row.tool {
			t.Fatalf("invalid category: %+v", row)
		}
		counts[row.tool]++
		if row.resource == "Sources" {
			m.treeCursor = i
			m.updateTree("enter")
			if m.tool != "flux" || m.gitOpsCurrent().State.Section != gitops.Sources || m.focus != 1 {
				t.Fatal("category did not open its table")
			}
			m.focusRootSidebar()
			if m.treeCursor != i {
				t.Fatal("Esc did not return to the selected category")
			}
		}
	}
	if counts["argocd"] != 3 || counts["flux"] != 3 {
		t.Fatal(counts)
	}
	m.setTreeExpanded(m.connection, false)
	for _, tool := range []string{"argocd", "flux"} {
		if m.expanded[m.connection+"/view/"+tool] {
			t.Fatal("connection collapse retained open category")
		}
	}
}

func TestGitOpsLegacyFluxSelectionMigratesToHelmReleases(t *testing.T) {
	m, _ := gitOpsModel(t)
	m.tool = "flux"
	key := m.active + "/" + m.connection + "/flux"
	m.gitOpsPreferences = map[string]gitOpsPreference{key: {Namespace: "flux-system", Selected: "helm.toolkit.fluxcd.io/HelmRelease/flux-system/metrics"}}
	view := m.gitOpsCurrent()
	if view.State.Section != gitops.HelmReleases || view.State.Namespace != "flux-system" {
		t.Fatal("legacy selected HelmRelease moved to Kustomizations")
	}
}

func TestGitOpsLegacySelectionSurvivesFirstDeniedRead(t *testing.T) {
	m, _ := gitOpsModel(t)
	key := m.active + "/" + m.connection + "/argocd"
	selected := "argoproj.io/Application/argocd/payments"
	m.gitOpsPreferences = map[string]gitOpsPreference{key: {Namespace: "argocd", Selected: selected}}
	m.gitOpsViews = nil
	view := m.gitOpsCurrent()
	view.State.Apply(gitops.Snapshot{State: gitops.ReadDenied})
	m.rememberGitOps(view)
	if m.gitOpsPreferences[key+"/Applications"].Selected != selected {
		t.Fatal("first denied read discarded migrated selection")
	}
}
