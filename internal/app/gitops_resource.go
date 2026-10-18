package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/monitor"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	gitopsmonitor "github.com/simjay/plous/internal/integrations/gitops/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/workspace"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/clientcmd"
)

type gitOpsReturn struct{ Workspace, Connection, Tool string }

type gitOpsResourceResult struct {
	View       *gitOpsView
	Run        uint64
	Connection workspace.Connection
	Client     *kubernetes.Client
	Catalog    []kubecommon.Resource
	Resource   kubecommon.Resource
	Object     *unstructured.Unstructured
	Err        error
}

func savedClusterServer(c workspace.Connection) string {
	if c.Target != nil {
		return strings.TrimSuffix(c.Target.Server(), "/")
	}
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	rules.MigrationRules = nil
	if paths := clusterKubeconfig(c); paths != "" {
		rules.Precedence = nil
		for _, path := range filepath.SplitList(paths) {
			if strings.HasPrefix(path, "~/") {
				if home, err := os.UserHomeDir(); err == nil {
					path = filepath.Join(home, path[2:])
				}
			}
			if path != "" {
				rules.Precedence = append(rules.Precedence, path)
			}
		}
	}
	config, err := rules.Load()
	if err != nil {
		return ""
	}
	name := c.Settings["context"]
	if name == "" {
		name = config.CurrentContext
	}
	ctx := config.Contexts[name]
	if ctx == nil || config.Clusters[ctx.Cluster] == nil {
		return ""
	}
	return strings.TrimSuffix(config.Clusters[ctx.Cluster].Server, "/")
}

func gitOpsDestinationConnection(origin workspace.Connection, connections []workspace.Connection, tool gitops.Tool, destination gitops.Destination) (workspace.Connection, error) {
	return gitOpsResolveDestination(origin, connections, tool, destination, savedClusterServer)
}

func gitOpsResolveDestination(origin workspace.Connection, connections []workspace.Connection, tool gitops.Tool, destination gitops.Destination, serverFor func(workspace.Connection) string) (workspace.Connection, error) {
	if destination.Remote {
		return workspace.Connection{}, errors.New("This Flux object targets a remote kubeconfig. Its live destination cannot be verified from this context.")
	}
	if tool == gitops.Flux || destination.Server == "https://kubernetes.default.svc" || destination.Server == "https://kubernetes.default.svc/" {
		return origin, nil
	}
	server := strings.TrimSuffix(destination.Server, "/")
	if server == "" {
		return workspace.Connection{}, errors.New("Destination access unavailable. Argo reports only a cluster name; a matching endpoint is needed to identify its saved context.")
	}
	if serverFor(origin) == server {
		return origin, nil
	}
	var matches []workspace.Connection
	for _, c := range connections {
		if monitorCluster(c.Tool) && serverFor(c) == server {
			matches = append(matches, c)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return workspace.Connection{}, errors.New("Multiple saved contexts match this destination. Open the intended context in Kubernetes to inspect its resources.")
	}
	return workspace.Connection{}, errors.New("Destination access unavailable. Add a context for the reported destination cluster to inspect its live resources.")
}

func (m *model) openGitOpsResource(v *gitOpsView, action monitor.GitOpsAction) tea.Cmd {
	var connections []workspace.Connection
	if w := m.current(); w != nil {
		connections = append(connections, w.Connections...)
	}
	for i := range connections {
		if monitorCluster(connections[i].Tool) && connections[i].Target == nil {
			connections[i].Target, _ = kubecommon.ResolveTarget(clusterKubeconfig(connections[i]), connections[i].Settings["context"], connections[i].Settings["namespace"])
		}
	}
	origin, tool := v.Cluster.Connection, v.State.Tool
	if v.Cancel != nil {
		v.Cancel()
	}
	v.Run++
	run := v.Run
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	v.Cancel, v.State.Loading = cancel, true
	return func() tea.Msg {
		defer cancel()
		r := gitOpsResourceResult{View: v, Run: run}
		r.Connection, r.Err = gitOpsDestinationConnection(origin, connections, tool, action.Destination)
		if r.Err != nil {
			return r
		}
		c := r.Connection
		r.Client, r.Err = kubernetes.NewTarget(c.Target, "")
		if r.Err != nil {
			return r
		}
		r.Catalog, _, r.Err = r.Client.Discover(ctx)
		if r.Err != nil {
			r.Err = gitopsmonitor.ClassifyError(r.Err)
			return r
		}
		var ok bool
		r.Resource, ok = gitops.FindResource(r.Catalog, action.Ref)
		if !ok {
			r.Err = errors.New("The managed resource's API is unavailable in the destination context.")
			return r
		}
		ns := action.Ref.Namespace
		if r.Resource.Namespaced && ns == "" {
			ns = action.Destination.Namespace
		}
		if r.Resource.Namespaced && ns == "" {
			r.Err = errors.New("The controller did not report a namespace for this resource.")
			return r
		}
		r.Object, r.Err = r.Client.Get(ctx, r.Resource, ns, action.Ref.Name)
		if r.Err != nil {
			r.Err = errors.New(kubernetes.ErrorText(r.Err))
		}
		return r
	}
}

func (m *model) applyGitOpsResource(r gitOpsResourceResult) {
	v := r.View
	if v == nil || v.Run != r.Run || v.Cancel == nil {
		return
	}
	v.Cancel, v.State.Loading = nil, false
	if r.Err != nil {
		v.State.Err = r.Err.Error()
		return
	}
	if !m.gitOpsVisible() || m.gitOpsCurrent() != v || r.Object == nil {
		return
	}
	origin := &gitOpsReturn{Workspace: m.active, Connection: m.connection, Tool: m.tool}
	m.selectScope(m.active, "kubernetes", r.Connection.ID)
	m.gitOpsReturn = origin
	m.focus, m.mode = 1, 1
	m.stopExplore()
	m.exploreGeneration++
	preference := monitor.ExplorePreference{Resource: r.Resource.Key(), Namespace: r.Object.GetNamespace()}
	m.explore = &monitor.ExploreState{
		Key: clusterKey(m.active, r.Connection), Generation: m.exploreGeneration,
		Client: r.Client, Resources: r.Catalog, Resource: r.Resource, Preference: preference,
		Selected: kubernetes.Identity(*r.Object), Detail: r.Object, DetailValid: true,
		Container: monitor.PreferredPodContainer(*r.Object), PreserveTree: true,
	}
}

func (m *model) returnToGitOps() bool {
	back := m.gitOpsReturn
	if back == nil {
		return false
	}
	m.stopExplore()
	m.selectScope(back.Workspace, back.Tool, back.Connection)
	m.mode, m.focus = 1, 1
	return true
}
