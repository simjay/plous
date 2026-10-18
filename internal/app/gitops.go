package app

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/monitor"
	argomonitor "github.com/simjay/plous/internal/integrations/argo/monitor"
	fluxmonitor "github.com/simjay/plous/internal/integrations/flux/monitor"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	gitopsmonitor "github.com/simjay/plous/internal/integrations/gitops/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
)

type gitOpsPreference struct {
	Section   gitops.Section `json:"section,omitempty"`
	Namespace string         `json:"namespace"`
	Selected  string         `json:"selected,omitempty"`
}

type gitOpsView struct {
	Temporary       bool
	State           *monitor.GitOpsState
	Cluster         *clusterState
	Run             uint64
	Cancel          context.CancelFunc
	Poll            uint64
	PollPending     bool
	Namespaces      *monitor.NamespacePicker
	NamespaceInput  *textview.Field
	NamespaceCursor int
}

type gitOpsResult struct {
	View     *gitOpsView
	Run      uint64
	Kind     string
	Snapshot gitops.Snapshot
	Item     gitops.Item
	Err      error
}

type gitOpsPoll struct {
	View *gitOpsView
	Run  uint64
}

func gitOpsTool(tool string) bool {
	return tool == string(gitops.ArgoCD) || tool == string(gitops.Flux)
}

func (m *model) gitOpsVisible() bool {
	c := m.connectionView(m.selectedConnection())
	return m.mode == 1 && m.page == "" && m.form == nil && c != nil && gitOpsTool(c.Tool)
}

func (m *model) gitOpsCurrent() *gitOpsView {
	c := m.savedConnection()
	if c == nil || !monitorCluster(c.Tool) || !gitOpsTool(m.tool) {
		return nil
	}
	cluster := m.clusterFor(m.active, *c)
	if back := m.overviewBack; back != nil && back.GitOps != nil && back.GitOps.Cluster == cluster && string(back.GitOps.State.Tool) == m.tool {
		return back.GitOps
	}
	rootKey := m.active + "/" + c.ID + "/" + m.tool
	root := m.gitOpsPreferences[rootKey]
	section := gitops.NormalizeSection(gitops.Tool(m.tool), root.Section)
	if root.Section == "" && strings.Contains(root.Selected, "/HelmRelease/") && m.tool == string(gitops.Flux) {
		section = gitops.HelmReleases
	}
	key := cluster.Key + "/" + m.tool + "/" + string(section)
	if m.gitOpsViews == nil {
		m.gitOpsViews = map[string]*gitOpsView{}
	}
	v := m.gitOpsViews[key]
	if v == nil {
		p := m.gitOpsPreferenceFor(rootKey, section)
		v = &gitOpsView{Cluster: cluster, State: monitor.NewGitOpsState(gitops.Tool(m.tool), p.Namespace)}
		v.State.Section = section
		m.gitOpsViews[key] = v
	}
	return v
}

func (m *model) gitOpsCapabilities(ws string, c workspace.Connection) []string {
	cluster := m.clusterStates[clusterKey(ws, c)]
	if cluster == nil || !cluster.Loaded && len(cluster.Catalog) == 0 && cluster.Err == nil && cluster.Warning == "" {
		return nil
	}
	var result []string
	for _, tool := range []gitops.Tool{gitops.ArgoCD, gitops.Flux} {
		if (tool == gitops.ArgoCD && argomonitor.Supports(cluster.Catalog) || tool == gitops.Flux && fluxmonitor.Supports(cluster.Catalog)) || cluster.Err != nil || cluster.Warning != "" {
			result = append(result, string(tool))
		}
	}
	return result
}

func (m *model) syncGitOps() tea.Cmd {
	if m.exploreClosed || m.quitting {
		return nil
	}
	var cmds []tea.Cmd
	var current *gitOpsView
	if m.gitOpsVisible() {
		current = m.gitOpsCurrent()
	}
	for _, v := range m.gitOpsViews {
		if v != current {
			if v.Cancel != nil {
				v.Cancel()
				v.Cancel = nil
				v.Run++
				v.State.Loading = false
			}
			if v.PollPending {
				v.Poll++
				v.PollPending = false
			}
			continue
		}
		s := v.State
		if action := s.TakeAction(); action != nil {
			switch action.Kind {
			case "namespace":
				m.openGitOpsNamespaces(v)
			case "refresh":
				if v.Cancel != nil {
					v.Cancel()
					v.Cancel = nil
					v.Run++
				}
				if v.Cluster.Cancel == nil {
					v.Cluster.refresh()
					cmds = append(cmds, m.discoverCluster(v.Cluster))
				}
				s.Loaded = false
			case "detail":
				cmds = append(cmds, m.readGitOps(v, action))
			case "resource":
				cmds = append(cmds, m.openGitOpsResource(v, *action))
			}
		}
		cmds = append(cmds, m.syncGitOpsNamespaces(v))
		if !s.Loaded && v.Cancel == nil && v.Cluster.Loaded && v.Cluster.Cancel == nil {
			cmds = append(cmds, m.readGitOps(v, nil))
		}
		if s.Loaded && v.Cancel == nil && !v.PollPending {
			v.PollPending = true
			run := v.Poll
			cmds = append(cmds, tea.Tick(15*time.Second, func(time.Time) tea.Msg { return gitOpsPoll{View: v, Run: run} }))
		}
	}
	return tea.Batch(cmds...)
}

func (m *model) readGitOps(v *gitOpsView, action *monitor.GitOpsAction) tea.Cmd {
	if v.Cancel != nil {
		v.Cancel()
	}
	v.Run++
	run := v.Run
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	v.Cancel, v.State.Loading = cancel, true
	client, catalog, warning, err := v.Cluster.Client, v.Cluster.Catalog, v.Cluster.Warning, v.Cluster.Err
	tool, ns, section := v.State.Tool, v.State.Namespace, v.State.Section
	var ref gitops.Reference
	kind := "list"
	if action != nil {
		ref, kind = action.Ref, "detail"
	}
	return func() tea.Msg {
		defer cancel()
		r := gitOpsResult{View: v, Run: run, Kind: kind}
		if err != nil || client == nil {
			failure := gitopsmonitor.ClassifyError(err)
			r.Snapshot = gitops.Snapshot{State: failure.State, Message: failure.Message, ReadAt: time.Now()}
			r.Err = failure
		} else if kind == "detail" {
			r.Item, r.Err = gitOpsClient(tool, client).Get(ctx, ref)
		} else {
			r.Snapshot = gitOpsClient(tool, client, section).LoadWithCatalog(ctx, ns, catalog, warning)
		}
		return r
	}
}

func (m *model) applyGitOpsResult(r gitOpsResult) {
	v := r.View
	if v == nil || v.Run != r.Run || v.Cancel == nil {
		return
	}
	v.Cancel, v.State.Loading = nil, false
	if r.Kind == "detail" {
		if r.Err != nil {
			v.State.Err = r.Err.Error()
		} else {
			v.State.ApplyDetail(r.Item)
		}
	} else {
		first := !v.State.Loaded && v.State.Snapshot.ReadAt.IsZero()
		v.State.Apply(r.Snapshot)
		if first {
			rootKey := v.Cluster.Workspace + "/" + v.Cluster.Connection.ID + "/" + string(v.State.Tool)
			p := m.gitOpsPreferenceFor(rootKey, v.State.Section)
			for i, item := range v.State.Rows() {
				if item.Ref.Key() == p.Selected {
					v.State.Select(i)
					break
				}
			}
		}
	}
	m.resolveGitOpsDestinations(v)
	m.rememberGitOps(v)
}

func (m *model) resolveGitOpsDestinations(v *gitOpsView) {
	var connections []workspace.Connection
	for _, w := range m.treeWorkspaces() {
		if w.ID == v.Cluster.Workspace {
			connections = w.Connections
			break
		}
	}
	if v.State.DestinationErrors == nil {
		v.State.DestinationErrors = map[string]string{}
	}
	items := append([]gitops.Item(nil), v.State.Snapshot.Items...)
	if v.State.Detail != nil {
		items = append(items, *v.State.Detail)
	}
	servers := map[string]string{}
	serverFor := func(c workspace.Connection) string {
		if server, ok := servers[c.ID]; ok {
			return server
		}
		server := savedClusterServer(c)
		servers[c.ID] = server
		return server
	}
	for _, item := range items {
		_, err := gitOpsResolveDestination(v.Cluster.Connection, connections, v.State.Tool, item.Destination, serverFor)
		delete(v.State.DestinationErrors, item.Ref.Key())
		if err != nil {
			v.State.DestinationErrors[item.Ref.Key()] = err.Error()
		}
	}
}

func (m *model) rememberGitOps(v *gitOpsView) {
	if v.Temporary || m.overviewBack != nil {
		return
	}
	if m.gitOpsPreferences == nil {
		m.gitOpsPreferences = map[string]gitOpsPreference{}
	}
	key := v.Cluster.Workspace + "/" + v.Cluster.Connection.ID + "/" + string(v.State.Tool)
	root := m.gitOpsPreferences[key]
	p := m.gitOpsPreferenceFor(key, v.State.Section)
	if p.Namespace != v.State.Namespace {
		p.Selected = ""
	}
	p.Namespace = v.State.Namespace
	if item := v.State.Selected(); item != nil {
		p.Selected = item.Ref.Key()
	} else if v.State.Snapshot.State == gitops.Empty {
		p.Selected = ""
	}
	p.Section = v.State.Section
	m.gitOpsPreferences[key+"/"+string(v.State.Section)] = p
	if root.Section == "" || root.Section == v.State.Section {
		m.gitOpsPreferences[key] = p
	}
	m.persist()
}

func (m *model) stopGitOps() {
	for _, v := range m.gitOpsViews {
		if v.Cancel != nil {
			v.Cancel()
			v.Cancel = nil
			v.Run++
		}
		m.closeGitOpsNamespaces(v)
		v.Poll++
		v.PollPending = false
	}
}

type gitOpsReader interface {
	Get(context.Context, gitops.Reference) (gitops.Item, error)
	LoadWithCatalog(context.Context, string, []kubecommon.Resource, string) gitops.Snapshot
}

func gitOpsClient(tool gitops.Tool, reader gitopsmonitor.Reader, section ...gitops.Section) gitOpsReader {
	switch tool {
	case gitops.ArgoCD:
		return argomonitor.New(reader, section...)
	case gitops.Flux:
		return fluxmonitor.New(reader, section...)
	}
	return gitopsmonitor.New(reader, gitopsmonitor.Definition{})
}

func (m *model) chooseGitOpsSection(section string) {
	if back := m.overviewBack; back != nil && back.GitOps != nil && string(back.GitOps.State.Tool) == m.tool {
		g := back.GitOps
		if g.Cancel != nil {
			g.Cancel()
			g.Cancel = nil
			g.Run++
		}
		g.State = monitor.NewGitOpsState(g.State.Tool, "")
		g.State.Section = gitops.NormalizeSection(g.State.Tool, gitops.Section(section))
		return
	}
	if current := m.gitOpsCurrent(); current != nil {
		m.rememberGitOps(current)
		m.closeGitOpsNamespaces(current)
		if current.Cancel != nil {
			current.Cancel()
			current.Cancel = nil
			current.Run++
			current.State.Loading = false
		}
	}
	if m.gitOpsPreferences == nil {
		m.gitOpsPreferences = map[string]gitOpsPreference{}
	}
	key := m.active + "/" + m.connection + "/" + m.tool
	p := m.gitOpsPreferences[key]
	p.Section = gitops.NormalizeSection(gitops.Tool(m.tool), gitops.Section(section))
	m.gitOpsPreferences[key] = p
	m.persist()
}

func (m *model) gitOpsPreferenceFor(key string, section gitops.Section) gitOpsPreference {
	if p, ok := m.gitOpsPreferences[key+"/"+string(section)]; ok {
		return p
	}
	root := m.gitOpsPreferences[key]
	p := gitOpsPreference{Namespace: root.Namespace, Section: section}
	parts := strings.Split(root.Selected, "/")
	if len(parts) == 4 && gitops.SectionFor(gitops.Reference{Kind: parts[1]}) == section {
		p.Selected = root.Selected
	}
	return p
}
