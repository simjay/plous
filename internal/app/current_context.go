package app

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/monitor"
	appsettings "github.com/simjay/plous/internal/app/settings"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
)

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
	// A result from startup or a picker must not interrupt newer navigation.
	if !m.currentRoot || !m.canSwitchCurrent() {
		return
	}
	if r.Request.Startup && (m.active != "" || m.focus != 1 || m.chatEditing || m.help != nil || m.settingsOverlay != nil || m.historyView != "") {
		m.currentError = "Current context is available. Use :K to select it when ready."
		return
	}
	c := currentConnection(r.Target)
	w := workspace.Workspace{ID: workspace.CurrentScopeID(r.Target.Fingerprint), Name: "Current Workspace", Connections: []workspace.Connection{c}}
	m.resetCurrentClients(w.ID, c.ID)
	m.currentCluster = &w
	m.currentNotice = ""
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
	m.expanded[w.ID], m.expanded[c.ID], m.expanded[c.ID+"/view/kubernetes"] = true, true, true
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
	if m.codeActive() && m.coder().Document != nil && m.coder().Document.Dirty() {
		m.status = "Save or close the edited source before switching connections."
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
	if message == "" {
		message = "Reading the terminal's current Kubernetes context…"
	}
	if m.currentDirectory != nil {
		message += "\n\nOpen Code in the sidebar to browse the launch directory:\n" + ui.OneLine(m.currentDirectory.Connections[0].Settings["path"])
	} else if m.currentDirectoryError != "" {
		message += "\n\n" + m.currentDirectoryError
	}
	return ui.StyledTextView(ui.TitleStyle.Render("Current Workspace")+"\n\n"+ansi.Wrap(message, width, "")+"\n\n"+ui.Selected.Render("Enter  Choose context\n:K     Connections\n:s     Choose kubeconfig or save a workspace\n:g     Refresh"), width, height, 0)
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
	if m.historyView == "" && m.currentDirectory != nil && m.active == m.currentDirectory.ID && m.savedConnection() != nil {
		return m.store.EnsureScope(workspace.CurrentDirectoryScope(m.savedConnection().Settings["path"]))
	}
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
