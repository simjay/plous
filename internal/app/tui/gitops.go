package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/monitor"
	argomonitor "github.com/simjay/plous/internal/integrations/argo/monitor"
	fluxmonitor "github.com/simjay/plous/internal/integrations/flux/monitor"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	gitopsmonitor "github.com/simjay/plous/internal/integrations/gitops/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/clientcmd"
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

type gitOpsNamespaceResult struct {
	View *gitOpsView
	monitor.NamespaceResult
}

func (m *model) gitOpsBody(width, height int) string {
	v := m.gitOpsCurrent()
	if v == nil {
		return ""
	}
	body := v.State.Body(width, height)
	if !v.Cluster.Loaded {
		body = ui.Heading(v.State.Title()) + "\n\n" + m.loading("Discovering cluster APIs…")
	}
	if v.Namespaces != nil {
		pw, ph := ui.PickerSize(width, height)
		return ui.PickerOverlay(body, m.namespacePickerView(v.Namespaces, v.NamespaceInput, v.State.Namespace, v.NamespaceCursor, pw-4, ph-2), width, height)
	}
	return body
}

func (m *model) gitOpsHint() string {
	v := m.gitOpsCurrent()
	if v.Namespaces != nil {
		return "↑↓ Select · Enter Apply · Esc Back"
	}
	return strings.Join(v.State.Hints(), " · ")
}

func (m *model) gitOpsInputKey(msg tea.KeyPressMsg) bool {
	if !m.gitOpsVisible() || m.focus != 1 || m.helpVisible() || m.settingsOverlay != nil || m.commandFocused() || m.tooSmall() {
		return false
	}
	v := m.gitOpsCurrent()
	if v.Namespaces == nil && !v.State.FilterInput() {
		return false
	}
	m.shortcutPrefix = ""
	if v.Namespaces != nil {
		m.gitOpsNamespaceKey(v, msg)
	} else {
		v.State.Key(msg)
	}
	return true
}

func (m *model) updateGitOps(key string) {
	v := m.gitOpsCurrent()
	if v == nil {
		return
	}
	if v.Namespaces != nil {
		m.gitOpsNamespaceKey(v, localMouseKey(key))
		return
	}
	if key == "esc" && v.State.Loading && v.Cancel != nil {
		v.Cancel()
		v.Cancel = nil
		v.Run++
		v.State.Loading = false
		v.State.Loaded = true
		v.State.Err = "Read cancelled. g refreshes this view."
		return
	}
	hadDetail := v.State.Detail != nil
	handled := v.State.Key(localMouseKey(key))
	if key == "esc" && (hadDetail && v.State.Detail == nil || !handled) {
		if !m.returnToOverview() && !handled {
			m.focusRootSidebar()
		}
	}
	m.rememberGitOps(v)
}

func (m *model) executeGitOps(command string) bool {
	if !m.gitOpsVisible() || m.settingsOverlay != nil {
		return false
	}
	name, argument, _ := strings.Cut(command, " ")
	switch name {
	case "refresh":
		m.updateGitOps("g")
		return true
	case "namespace":
		argument = strings.TrimSpace(argument)
		if argument == "" {
			m.updateGitOps("n")
			return true
		}
		if !monitor.ValidExploreNamespace(argument) {
			m.status = "Use a namespace name or * for all namespaces."
			return true
		}
		v := m.gitOpsCurrent()
		if v.Cancel != nil && v.State.Namespace != monitor.ExploreNamespace(argument) {
			v.Cancel()
			v.Cancel = nil
			v.Run++
		}
		m.closeGitOpsNamespaces(v)
		v.State.SetNamespace(monitor.ExploreNamespace(argument))
		m.rememberGitOps(v)
		return true
	}
	return false
}

func (m *model) gitOpsPaste(text string) bool {
	if !m.gitOpsVisible() || m.focus != 1 || m.helpVisible() || m.settingsOverlay != nil || m.commandFocused() || m.tooSmall() {
		return false
	}
	v := m.gitOpsCurrent()
	if v.Namespaces != nil {
		v.NamespaceInput.Insert(text, false)
		v.NamespaceCursor = 0
		return true
	}
	if v.State.FilterInput() {
		v.State.Paste(text)
		return true
	}
	return false
}

func (m *model) openGitOpsNamespaces(v *gitOpsView) {
	m.closeGitOpsNamespaces(v)
	v.Namespaces = &monitor.NamespacePicker{}
	input := textview.NewField("")
	v.NamespaceInput = &input
	v.NamespaceCursor = 0
}

func (m *model) closeGitOpsNamespaces(v *gitOpsView) {
	v.Namespaces.Cancel()
	v.Namespaces, v.NamespaceInput = nil, nil
}

func (m *model) gitOpsNamespaceKey(v *gitOpsView, msg tea.KeyPressMsg) {
	value, apply, close, status := monitor.UpdateNamespacePicker(v.Namespaces, &v.NamespaceInput, &v.NamespaceCursor, v.State.Namespace, msg)
	m.status = status
	if apply {
		if v.Cancel != nil && value != v.State.Namespace {
			v.Cancel()
			v.Cancel = nil
			v.Run++
		}
		v.State.SetNamespace(value)
		m.closeGitOpsNamespaces(v)
		m.rememberGitOps(v)
	} else if close {
		m.closeGitOpsNamespaces(v)
	}
}

func (m *model) syncGitOpsNamespaces(v *gitOpsView) tea.Cmd {
	p := v.Namespaces
	if p == nil || p.Loaded || p.Run != nil || !v.Cluster.Loaded {
		return nil
	}
	if v.Cluster.Client == nil {
		p.Loaded = true
		p.Err = "Connection unavailable. Enter a namespace manually or refresh."
		return nil
	}
	client := v.Cluster.Client
	load := p.Load(15*time.Second, func(ctx context.Context) (kubernetes.Collection, error) {
		return client.List(ctx, kubecommon.Resource{Name: "namespaces", Kind: "Namespace", Version: "v1"}, "", "")
	})
	return func() tea.Msg {
		return gitOpsNamespaceResult{View: v, NamespaceResult: load()}
	}
}

func (m *model) applyGitOpsNamespaces(r gitOpsNamespaceResult) {
	if r.View != nil {
		r.View.Namespaces.Apply(r.NamespaceResult)
	}
}
