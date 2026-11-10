package tui

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	appsettings "github.com/simjay/plous/internal/app/settings"
	helm "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
)

func monitorCluster(tool string) bool { return tool == "kubernetes" || tool == "helm" }

func clusterSettings(c workspace.Connection) map[string]string {
	if !monitorCluster(c.Tool) {
		return nil
	}
	settings := map[string]string{}
	for key, value := range c.Settings {
		if value != "" {
			settings[key] = value
		}
	}
	paths := settings["kubeconfig"]
	if paths == "" && c.Tool == "kubernetes" {
		paths = os.Getenv("KUBECONFIG")
	}
	if paths == "" {
		paths = "~/.kube/config"
	}
	resolved, seen := []string{}, map[string]bool{}
	for _, path := range filepath.SplitList(paths) {
		if path == "" {
			continue
		}
		if strings.HasPrefix(path, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				path = filepath.Join(home, path[2:])
			}
		}
		if absolute, err := filepath.Abs(path); err == nil {
			path = absolute
		}
		if real, err := filepath.EvalSymlinks(path); err == nil {
			path = real
		}
		if !seen[path] {
			resolved, seen[path] = append(resolved, path), true
		}
	}
	settings["kubeconfig"] = strings.Join(resolved, string(os.PathListSeparator))
	if settings["driver"] == "" {
		settings["driver"] = "secret"
	}
	return settings
}

func sameClusterTarget(a, b workspace.Connection) bool {
	x, y := clusterSettings(a), clusterSettings(b)
	return x != nil && y != nil && x["context"] != "" && x["context"] == y["context"] && x["kubeconfig"] == y["kubeconfig"]
}

// Legacy records remain addressable in Settings, Activity, and saved scopes.
// The tree groups only equivalent records with the same display name.
func (m *model) sidebarConnections(w workspace.Workspace) []workspace.Connection {
	connections := []workspace.Connection{}
	identities := []map[string]string{}
	for _, c := range w.Connections {
		if !monitorCluster(c.Tool) {
			continue
		}
		identity, duplicate := clusterSettings(c), -1
		if identity != nil && identity["context"] != "" {
			for n, previous := range connections {
				if previous.Name == c.Name && maps.Equal(identities[n], identity) {
					duplicate = n
					break
				}
			}
		}
		if duplicate >= 0 {
			if w.ID == m.active && c.ID == m.connection {
				connections[duplicate] = c
			}
			continue
		}
		connections, identities = append(connections, c), append(identities, identity)
	}
	return connections
}

type currentRequest struct {
	Name, Command   string
	Select, Startup bool
}
type currentResult struct {
	ObservedFingerprint string
	ObservedError       string
	Request             *currentRequest
	Inventory           kubecommon.ContextInventory
	Target              *kubecommon.Target
	Err                 error
}
type currentPoll struct{}

func currentTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return currentPoll{} })
}

func (m *model) requestCurrent(name string, selectTarget bool) {
	m.currentRequest = &currentRequest{Name: name, Select: selectTarget}
}

func (m *model) syncCurrent() tea.Cmd {
	if m.currentRequest == nil || m.currentLoading != nil || m.quitting {
		return nil
	}
	r := m.currentRequest
	m.currentRequest, m.currentLoading = nil, r
	paths, store := m.currentPaths, m.store
	selected := ""
	if m.currentCluster != nil {
		selected = m.currentCluster.Connections[0].Name
	}
	return func() tea.Msg {
		result := currentResult{Request: r}
		result.Inventory, result.Err = kubecommon.DiscoverCurrent(paths)
		if result.Err == nil && !r.Select && selected != "" {
			observed, err := kubecommon.ResolveTarget(paths, selected, "")
			if err != nil {
				result.ObservedError = err.Error()
			} else {
				result.ObservedFingerprint = observed.Fingerprint
			}
		}
		if result.Err == nil && r.Select {
			result.Target, result.Err = kubecommon.ResolveTarget(paths, r.Name, "")
			if result.Err == nil {
				c := currentConnection(result.Target)
				result.Err = store.EnsureScope(workspace.Scope{ID: workspace.CurrentScopeID(result.Target.Fingerprint), Kind: "current_context", Connection: c})
			}
		}
		return result
	}
}

func currentConnection(t *kubecommon.Target) workspace.Connection {
	return workspace.Connection{ID: "kube-" + t.Fingerprint, Name: t.Context, Tool: "kubernetes", Target: t, TargetCaptured: true, Settings: map[string]string{"context": t.Context, "kubeconfig": t.Paths, "namespace": t.Namespace}}
}

func (m *model) applyCurrent(r currentResult) {
	if r.Request != m.currentLoading {
		return
	}
	m.currentLoading = nil
	if m.currentRequest != nil {
		return
	}
	m.currentInventory = r.Inventory
	if r.Err != nil {
		changed := m.currentError != r.Err.Error()
		m.currentError = r.Err.Error()
		if m.currentRoot && changed {
			m.status = m.currentError
		}
		return
	}
	if r.Request.Select || m.currentCluster != nil {
		m.currentError = ""
	}
	if !r.Request.Select {
		previousNotice := m.currentNotice
		if m.currentCluster != nil && r.Inventory.Current != m.currentCluster.Connections[0].Settings["context"] {
			m.currentNotice = "Kubeconfig default: " + r.Inventory.Current + ". Use :K to switch."
			if r.Inventory.Current == "" {
				m.currentNotice = "Kubeconfig has no default. Use :K to choose a context."
			}
		} else {
			m.currentNotice = ""
		}
		if m.currentCluster != nil && (r.ObservedError != "" || r.ObservedFingerprint != m.currentCluster.Connections[0].Target.Fingerprint) {
			m.currentNotice = "Selected context configuration changed or disappeared. Use :K to select a target."
		}
		if m.currentRoot && m.currentNotice != previousNotice && m.currentNotice != "" {
			m.status = m.currentNotice
		}
		return
	}
	// Startup publishes the available connection even after navigation has moved on.
	// Only an untouched main view may also select its conversation and page scope.
	selectTarget := true
	if r.Request.Startup {
		selectTarget = m.currentRoot && m.active == "" && m.focus == 1 && !m.chatEditing && m.help == nil && m.settingsOverlay == nil && m.historyView == "" && m.command == nil
		if selectTarget {
			selectTarget = m.canSwitchCurrent()
		}
	} else if !m.currentRoot || !m.canSwitchCurrent() {
		return
	}
	c := currentConnection(r.Target)
	w := workspace.Workspace{ID: workspace.CurrentScopeID(r.Target.Fingerprint), Name: "Current Workspace", Connections: []workspace.Connection{c}}
	cursorPath := m.treeCursorPath()
	root := m.currentTreeKey()
	monitorKey := "current/monitor"
	if m.currentCluster != nil {
		monitorKey = m.currentCluster.ID + "/monitor"
	}
	for i, key := range cursorPath {
		switch key {
		case root:
			cursorPath[i] = w.ID
		case monitorKey:
			cursorPath[i] = w.ID + "/monitor"
		case "choose-current":
			cursorPath[i] = c.ID
		}
	}
	defer func() { m.restoreTreeCursor(cursorPath...) }()
	if selectTarget {
		m.resetCurrentClients(w.ID, c.ID)
	}
	m.currentCluster = &w
	m.currentNotice = ""
	if open, set := m.expanded[root]; r.Request.Startup && set {
		m.expanded[w.ID] = open
	} else {
		m.expanded[w.ID] = true
	}
	if open, set := m.expanded[monitorKey]; set {
		m.expanded[w.ID+"/monitor"] = open
	}
	if !selectTarget {
		m.persist()
		return
	}
	if r.Request.Startup {
		if m.explorePreferences == nil {
			m.explorePreferences = map[string]monitor.ExplorePreference{}
		}
		m.explorePreferences[w.ID+"/"+c.ID] = monitor.ExplorePreference{Resource: "pods", Namespace: r.Target.Namespace}
	}
	m.selectScope(w.ID, "kubernetes", c.ID)
	if !r.Request.Startup {
		m.mode = 1
	}
	m.focus = 1
	m.expanded[c.ID], m.expanded[c.ID+"/view/kubernetes"] = true, true
	if command := r.Request.Command; command != "" && canonicalCommand(command) != "connections" {
		m.executeExplore(command)
	}
	m.persist()
}

func (m *model) canSwitchCurrent() bool {
	if m.form != nil {
		m.status = "Finish or cancel the open form first."
		return false
	}
	if m.explore != nil && (m.explore.Action != nil || m.explore.Terminal != nil && !m.explore.Terminal.Done) {
		m.status = "Finish the open resource action or Exec session first."
		return false
	}

	if m.helmActive() {
		h := m.helmer()
		if h.Busy && h.Running == "execute" || h.Confirm || h.Draft != nil && h.WorkflowVisible() || h.Document != nil && h.Document.Dirty() {
			m.status = "Finish the Helm operation or save or close its edited document first."
			return false
		}
	}
	return true
}

// Explicit reselection renews clients without changing the durable history scope.
func (m *model) resetCurrentClients(scope, connection string) {
	if m.active == scope && m.connection == connection && m.explore != nil {
		m.stopExplore()
		m.explore.Key = ""
	}
	for key, h := range m.helmStates {
		if h.Workspace == scope && h.Connection == connection {
			// A suspended action keeps its captured client and exact draft.
			if h.Draft != nil || h.Document != nil && h.Document.Dirty() {
				continue
			}
			h.Stop()
			h.Run++
			delete(m.helmStates, key)
		}
	}
	for key, v := range m.gitOpsViews {
		if v.Cluster.Workspace == scope && v.Cluster.Connection.ID == connection {
			if v.Cancel != nil {
				v.Cancel()
			}
			v.Run++
			v.Poll++
			m.closeGitOpsNamespaces(v)
			delete(m.gitOpsViews, key)
		}
	}
	for key, c := range m.clusterStates {
		if c.Workspace == scope && c.Connection.ID == connection {
			c.stopDiscovery()
			delete(m.clusterStates, key)
		}
	}
}

func (m *model) enterCurrent() {
	if !m.canSwitchCurrent() {
		return
	}
	if w := m.currentCluster; w != nil {
		m.selectScope(w.ID, "kubernetes", w.Connections[0].ID)
	} else {
		m.selectScope("", "", "")
		m.currentRoot = true
		m.requestCurrent("", true)
	}
	m.mode = 1
}

func (m *model) currentBody(width, height int) string {
	message := m.currentError
	action := "Enter  Choose context"
	if message == "" {
		message = "Reading the terminal's current Kubernetes context…"
		if m.currentCluster != nil {
			message = "Current context is available in Monitor. Select it when ready."
		}
	}
	if m.currentCluster != nil {
		action = "Enter  Open current context"
	}

	return ui.StyledTextView(ui.TitleStyle.Render("Current Workspace")+"\n\n"+ansi.Wrap(message, width, "")+"\n\n"+ui.Selected.Render(action+"\n:K     Connections\n:s     Choose kubeconfig or save a workspace\n:g     Refresh"), width, height, 0)
}

func (m *model) beginCurrentPath() {
	m.beginSettingsOverlay()
	m.form = &appsettings.Form{Kind: "currentpath", Title: "Choose Kubeconfig", Fields: []appsettings.Field{{Key: "path", Label: "Kubeconfig path (empty uses terminal environment)", Value: textview.NewField(m.currentPaths)}}}
}

func (m *model) beginSaveCurrent() {
	if !m.canSwitchCurrent() {
		return
	}
	connections := m.currentConnections()
	if len(connections) == 0 {
		m.status = "No current connection is available to save."
		return
	}
	m.beginWorkspace()
	m.form.Kind, m.form.Title = "savecurrent", "Save as Workspace"
	name := connections[0].Name
	if c := m.savedConnection(); m.currentRoot && c != nil {
		name = c.Name
	}
	m.form.Fields[0].Value = textview.NewField(name)
}

func (m *model) conversationScope() string {
	if m.historyView == "@general" {
		return ""
	}
	if m.historyView != "" {
		return m.historyView
	}
	if c := m.selectedConnection(); c != nil {
		if workspace.IsCurrentScope(m.active) {
			return m.active
		}
		return workspace.ConnectionScopeID(m.active, c.ID)
	}
	return ""
}

func (m *model) ensureConversationScope() error {
	if m.historyView != "" || m.savedConnection() == nil || workspace.IsCurrentScope(m.active) {
		return nil
	}
	return m.store.EnsureScope(workspace.Scope{ID: m.conversationScope(), Kind: "connection", WorkspaceID: m.active, Connection: *m.savedConnection()})
}

func (m *model) assistantScopeLabel() string {
	if m.historyView != "" {
		return "History · " + m.historyViewName + " · read only"
	}
	if c := m.savedConnection(); c != nil {
		name := c.Name
		if monitorCluster(c.Tool) {
			if context := c.Settings["context"]; context != "" {
				name = context
			}
		}
		return ui.OneLine(name)
	}
	return "General chat"
}

func (m *model) currentChoices() []monitor.ExploreConnection {
	var choices []monitor.ExploreConnection
	query := ""
	if m.explore != nil && m.explore.Input != nil {
		query = m.explore.Input.Text
	}
	for _, c := range m.currentInventory.Contexts {
		if !ui.SearchContains(c.Name, query) {
			continue
		}
		label := "Current Workspace"
		if c.Name == m.currentInventory.Current {
			label += " · kubeconfig default"
		}
		if m.currentCluster != nil && c.Name == m.currentCluster.Connections[0].Name {
			label += " · selected"
		}
		choices = append(choices, monitor.ExploreConnection{Workspace: "current", Label: c.Name + " · " + label, Connection: workspace.Connection{ID: c.Name, Name: c.Name, Tool: "kubernetes", Settings: map[string]string{"context": c.Name}}})
	}
	return choices
}

func (m *model) currentSummary() string {
	if m.currentCluster == nil {
		return "Current Workspace"
	}
	return fmt.Sprintf("Current Workspace: %s", strings.TrimSpace(m.currentCluster.Connections[0].Name))
}

// The current context uses the same tree structure as saved clusters.
func (m *model) currentTreeKey() string {
	if m.currentCluster != nil {
		return m.currentCluster.ID
	}

	return "current"
}

func (m *model) currentConnections() []workspace.Connection {
	if m.currentCluster == nil {
		return nil
	}
	return append([]workspace.Connection(nil), m.currentCluster.Connections...)
}

func (m *model) saveCurrentWorkspace(name string) {
	connections := m.currentConnections()
	if len(connections) == 0 {
		m.status = "No current connection is available to save."
		return
	}
	origin, tool, selected := m.currentTreeKey(), "", 0
	if m.currentRoot && m.savedConnection() != nil {
		origin, tool = m.active, m.tool
	}
	for i := range connections {
		if m.currentRoot && connections[i].ID == m.connection {
			selected = i
		}
		connections[i].ID, connections[i].Target = "", nil
		connections[i].TargetCaptured = false
	}
	if m.saveWorkspace(workspace.Workspace{Name: name, OriginScope: origin, Connections: connections}) {
		w := m.current()
		m.form = nil
		m.closeSettings()
		if tool == "" || tool == "monitor" {
			tool = w.Connections[selected].Tool
		}
		m.selectScope(w.ID, tool, w.Connections[selected].ID)
		m.mode, m.focus = 1, 1
		m.status = "Workspace saved. Earlier conversations remain in Current Workspace history."
	}
}

type clusterState struct {
	Key, Workspace string
	Connection     workspace.Connection
	Client         *kubernetes.Client
	Catalog        []kubecommon.Resource
	Warning        string
	Err            error
	Loaded         bool
	Run            uint64
	Cancel         context.CancelFunc
	HelmDriver     string
	HelmErr        error
	HelmChecked    bool
	HelmRun        uint64
	HelmCancel     context.CancelFunc
}

type clusterDiscoveryResult struct {
	Cluster *clusterState
	Run     uint64
	Client  *kubernetes.Client
	Target  *kubecommon.Target
	Catalog []kubecommon.Resource
	Warning string
	Err     error
}

func clusterKey(ws string, c workspace.Connection) string {
	return strings.Join([]string{ws, c.ID, c.Settings["kubeconfig"], c.Settings["context"], c.Settings["namespace"]}, "\x00")
}

func clusterKubeconfig(c workspace.Connection) string {
	if c.Tool == "helm" && c.Settings["kubeconfig"] == "" {
		return "~/.kube/config"
	}
	return c.Settings["kubeconfig"]
}

func (m *model) clusterFor(ws string, c workspace.Connection) *clusterState {
	if m.clusterStates == nil {
		m.clusterStates = map[string]*clusterState{}
	}
	key := clusterKey(ws, c)
	cluster := m.clusterStates[key]
	if cluster != nil {
		previous, next := cluster.Connection.Target, c.Target
		changedTarget := c.TargetCaptured && (previous == nil) != (next == nil) || previous != nil && next != nil && previous.Fingerprint != next.Fingerprint
		if changedTarget || cluster.Connection.Settings["driver"] != c.Settings["driver"] {
			cluster.stopDiscovery()
			cluster = nil
		}
	}
	if cluster == nil {
		cluster = &clusterState{Key: key, Workspace: ws, Connection: c}
		m.clusterStates[key] = cluster
	}
	return cluster
}

func (m *model) discoverCluster(c *clusterState) tea.Cmd {
	c.Run++
	run, connection := c.Run, c.Connection
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	c.Cancel = cancel
	return func() tea.Msg {
		defer cancel()
		r := clusterDiscoveryResult{Cluster: c, Run: run}
		if connection.Target == nil && connection.TargetCaptured {
			r.Err = errors.New("The captured cluster configuration is unavailable. Reselect the connection.")
		} else if connection.Target == nil {
			connection.Target, r.Err = kubecommon.ResolveTarget(clusterKubeconfig(connection), connection.Settings["context"], connection.Settings["namespace"])
		}
		if r.Err == nil {
			r.Target = connection.Target
			r.Client, r.Err = kubernetes.NewTarget(connection.Target, connection.Settings["namespace"])
		}
		if r.Err == nil {
			r.Catalog, r.Warning, r.Err = r.Client.Discover(ctx)
		}
		return r
	}
}

func (m *model) applyClusterDiscovery(r clusterDiscoveryResult) {
	c := r.Cluster
	if c == nil || c.Run != r.Run || c.Cancel == nil {
		return
	}
	c.Cancel, c.Loaded = nil, true
	c.Client, c.Catalog, c.Warning, c.Err = r.Client, r.Catalog, r.Warning, r.Err
	c.Connection.Target, c.Connection.TargetCaptured = r.Target, true
	if e := m.explore; e != nil && e.Key == c.Key && !e.Connecting && m.exploreActive() && r.Err == nil {
		e.Client, e.Resources, e.Warning = c.Client, c.Catalog, c.Warning
	}
}

func (m *model) helmDriver(ws string, c workspace.Connection) string {
	if driver := c.Settings["driver"]; driver != "" {
		return driver
	}
	if cluster := m.clusterStates[clusterKey(ws, c)]; cluster != nil && cluster.HelmDriver == "configmap" {
		return "configmap"
	}
	return ""
}

type helmDetectionResult struct {
	Cluster *clusterState
	Run     uint64
	Driver  string
	Err     error
}

func (m *model) syncClusters() tea.Cmd {
	if m.exploreClosed || m.quitting {
		return nil
	}
	var cmds []tea.Cmd
	for _, w := range m.treeWorkspaces() {
		for _, c := range m.sidebarConnections(w) {
			if !monitorCluster(c.Tool) {
				continue
			}
			savedOpen, set := m.expanded["saved-workspaces"]
			areaOpen, areaSet := m.expanded[w.ID+"/monitor"]
			visible := m.expanded[w.ID] && m.expanded[c.ID] && (!areaSet || areaOpen) && (workspace.IsCurrentScope(w.ID) || !set || savedOpen)
			active := m.active == w.ID && m.connection == c.ID && (monitorCluster(m.tool) || gitOpsTool(m.tool))
			if !visible && !active {
				continue
			}
			cluster := m.clusterFor(w.ID, c)
			if !cluster.Loaded && cluster.Cancel == nil {
				cmds = append(cmds, m.discoverCluster(cluster))
			}
			if cluster.Loaded && cluster.Client != nil && !cluster.HelmChecked && cluster.HelmCancel == nil {
				cmds = append(cmds, m.detectHelm(cluster))
			}
		}
	}
	return tea.Batch(cmds...)
}

func (m *model) clusterCapabilities(ws string, c workspace.Connection) []string {
	tools := []string{"kubernetes"}
	if cluster := m.clusterStates[clusterKey(ws, c)]; cluster != nil && cluster.HelmDriver != "" {
		tools = append(tools, "helm")
	}
	return append(tools, m.gitOpsCapabilities(ws, c)...)
}

func (m *model) detectHelm(c *clusterState) tea.Cmd {
	c.HelmRun++
	run, client, driver := c.HelmRun, c.Client, c.Connection.Settings["driver"]
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	c.HelmCancel = cancel
	return func() tea.Msg {
		defer cancel()
		found, err := helm.Detect(ctx, client, driver)
		return helmDetectionResult{Cluster: c, Run: run, Driver: found, Err: err}
	}
}

func (m *model) applyHelmDetection(r helmDetectionResult) {
	c := r.Cluster
	if c == nil || c.HelmRun != r.Run || c.HelmCancel == nil {
		return
	}
	c.HelmCancel, c.HelmChecked, c.HelmErr = nil, true, r.Err
	// Failed refreshes cannot disprove a previously observed installation.
	if r.Err == nil || r.Driver != "" {
		c.HelmDriver = r.Driver
	}
}

func (c *clusterState) stopDiscovery() {
	if c.Cancel != nil {
		c.Cancel()
		c.Cancel = nil
		c.Run++
	}
	if c.HelmCancel != nil {
		c.HelmCancel()
		c.HelmCancel = nil
		c.HelmRun++
	}
}

func (c *clusterState) refresh() {
	c.stopDiscovery()
	c.Loaded, c.HelmChecked = false, false
}

func (m *model) stopClusters() {
	for _, c := range m.clusterStates {
		c.stopDiscovery()
	}
}
