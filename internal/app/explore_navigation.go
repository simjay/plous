package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	textview "github.com/simjay/plous/internal/ui/text"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
)

func (m *model) executeExplore(command string) bool {
	return m.runExploreCommand(command, true)
}

func (m *model) runExploreCommand(command string, route bool) bool {
	command = canonicalCommand(command)
	name, arg, _ := strings.Cut(command, " ")
	arg = strings.TrimSpace(arg)
	if action := actionCommand(name); action != "" {
		if arg != "" {
			m.status = "This action takes no command arguments."
			return true
		}
		if m.form != nil {
			m.status = "Finish or cancel the open form first."
			return true
		}
		if !m.exploreVisible() || m.explore == nil || m.explore.Detail == nil {
			m.status = "Open the resource details first to use its actions."
			return true
		}
		m.closeHelp()
		m.focus = 1
		m.openResourceActions(action)
		return true
	}
	special := ""
	switch name {
	case "namespace", "connections", "refresh", "yaml", "describe":
		special = name
	}
	resource, known := "", false
	if special == "" {
		resource, known = monitor.ResourceCommands[name]
		if !known && m.explore != nil {
			if found, ok := kubernetes.Find(m.explore.Resources, name); ok {
				resource, known = found.Key(), true
			}
		}
	}
	if !known && special == "" {
		return false
	}
	if m.form != nil {
		m.status = "Finish or cancel the open form in the main panel first."
		return true
	}
	if arg != "" && special != "namespace" && !known {
		m.status = "This command does not take arguments."
		return true
	}
	if arg != "" && (known || special == "namespace") && !monitor.ValidExploreNamespace(arg) {
		m.status = "Use a valid namespace name or * for all namespaces."
		return true
	}
	if route {
		m.closeHelp()
		m.closeSettings()
		m.mode, m.focus = 1, 1
		m.page, m.detail, m.status = "", "", ""
	}
	e := m.explorer()
	if special == "connections" || !m.exploreActive() {
		if m.currentRoot {
			m.requestCurrent("", false)
		}
		e.Picker = "connections"
		e.PickerCursor = 0
		e.InputKind = "connections"
		input := textview.NewField("")
		e.Input = &input
		e.Requested = resource
		e.PendingCommand = command
		return true
	}
	if e.Key == "" || e.Connecting {
		e.PendingCommand = command
		if known {
			e.Requested = resource
		}
		return true
	}
	e.PendingCommand = ""
	if known {
		m.chooseExploreResource(resource)
		if arg != "" {
			e.Preference.Namespace = monitor.ExploreNamespace(arg)
			e.DefaultNamespace = false
		}
		m.rememberExplore()
		return true
	}
	switch special {
	case "namespace":
		if arg != "" {
			m.chooseExploreNamespace(monitor.ExploreNamespace(arg))
		} else {
			m.openNamespacePicker()
		}
	case "refresh":
		m.refreshExplore()
	case "yaml":
		m.openExploreSelection(1)
	case "describe":
		m.openExploreSelection(0)
	}
	return true
}

func (m *model) openExploreInput(kind, text string) {
	e := m.explorer()
	input := textview.NewField(text)
	e.Input = &input
	e.InputKind = kind
	e.PickerCursor = 0
	m.shortcutPrefix = ""
}

func (m *model) clearExploreDetail() {
	e := m.explorer()
	m.stopResourceAction()
	m.stopResourceTerminal()
	if e.DetailRun != nil {
		e.DetailRun.Cancel()
		e.DetailRun = nil
	}
	if e.Log != nil {
		e.Log.Cancel()
		e.Log = nil
	}
	e.Detail = nil
	e.DetailSections = [4]monitor.DetailSection{}
	e.DetailIdentity, e.DetailLoaded = "", false
	e.YamlDocument = nil
	e.YamlOrigin = nil
	e.DetailKey = ""
	e.LogKey = ""
	m.resetExploreLogs()
	e.Trail = nil
	e.LogOrigin = nil
	e.Scroll = 0
	e.Reveal = false
	e.DetailValid = false
	e.Container = ""
	e.Previous = false
	e.MergedLogs = false
}

func (m *model) chooseExploreResource(resource string) {
	e := m.explorer()
	if e.Action != nil {
		m.status = "Finish or cancel the open resource action first."
		return
	}
	if e.Key == "" {
		preference, ok := m.explorePreferences[m.active+"/"+m.connection]
		if !ok {
			preference.Namespace = "default"
			if c := m.currentConnection(); c != nil && c.Settings["namespace"] != "" {
				preference.Namespace = c.Settings["namespace"]
			}
		}
		e.Preference = preference
	}
	m.clearExploreDetail()
	e.Preference.Resource = resource
	e.PreserveTree = false
	m.exploreTreeSelection = ""
	e.Preference.Filter = ""
	e.Preference.Selector = ""
	e.Preference.Sort = 0
	e.Preference.Descending = false
	e.Picker = ""
	e.Input = nil
	e.InputKind = ""
	e.Selected = ""
	if e.Key == "" {
		e.Requested = resource
	} else {
		e.Requested = ""
	}
	m.rememberExplore()
}

func (m *model) refreshExplore() {
	e := m.explorer()
	if e.Detail != nil {
		if e.DetailRun != nil {
			e.DetailRun.Cancel()
			e.DetailRun = nil
		}
		if e.Log != nil {
			e.Log.Cancel()
			e.Log = nil
		}
		e.DetailKey, e.LogKey = "", ""
		return
	}
	m.rememberExplore()
	m.stopExplore()
	if c := m.savedConnection(); c != nil {
		m.clusterFor(m.active, *c).refresh()
	}
	e.Key = ""
	e.Requested = e.Preference.Resource
}

func (m *model) openExploreSelection(tab int) {
	e := m.explorer()
	if tab != 4 && e.Tab == 4 {
		m.returnFromExploreLogs()
	}
	if tab == 4 && e.Resource.Key() != "pods" {
		if e.Detail != nil && kubernetes.HasPodTargets(e.Resource) {
			m.openResourceActions("logs")
		} else {
			m.status = "Open a Pod or workload's details to choose its logs."
		}
		return
	}
	if tab == 1 && e.Detail != nil && e.Tab != 1 {
		e.YamlOrigin = &monitor.DetailSectionReturn{Tab: e.Tab, Scroll: e.Scroll, RelatedCursor: e.RelatedCursor}
	} else if tab != 1 && tab != 4 {
		e.YamlOrigin = nil
	}
	if e.Detail == nil {
		_, rows := m.exploreRows()
		if len(rows) == 0 {
			return
		}
		object := rows[0].Object
		for _, row := range rows {
			if kubernetes.Identity(row.Object) == e.Selected {
				object = row.Object
				break
			}
		}
		if len(e.Trail) > 0 {
			e.Trail = append(e.Trail, monitor.ExploreLocation{Preference: e.Preference, Selected: kubernetes.Identity(object)})
		}
		e.Detail = object.DeepCopy()
		e.Selected = kubernetes.Identity(object)
		e.DetailKey = ""
		e.DetailValid = false
		e.Tab = 0
		e.Container = monitor.PreferredPodContainer(object)
		e.Previous = false
	}
	if tab == 4 && e.Tab != 4 {
		origin := e.Location()
		e.LogOrigin = &origin
	}
	if e.Usage != nil && tab != 0 {
		e.Usage.Charts.Inspect = false
	}
	e.Tab = tab
	e.Scroll = 0
	e.Reveal = false
	if tab == 4 {
		m.podLogViewer().Follow()
	}
	e.Picker = ""
	e.Input = nil
	e.InputKind = ""
}

func (m *model) updateExplore(key string) {
	e := m.explorer()
	if m.updateChartKey(key) {
		return
	}
	if key == "I" || key == "W" || key == "P" {
		if m.chartsAvailable() {
			m.chartCommand(canonicalCommand(key))
			return
		}
	}
	if key == "n" {
		m.openNamespacePicker()
		return
	}
	if key == "g" && e.Detail == nil {
		m.refreshExplore()
		return
	}
	if e.Detail != nil {
		if m.runDetailActionKey(key) {
			return
		}
		switch key {
		case "a":
			m.openResourceActions("")
		case "esc":
			m.backExploreLocation()
		case "[", "]":
			if e.Tab == 4 {
				m.returnFromExploreLogs()
			}
			step := 1
			if key == "[" {
				step = -1
			}
			m.openExploreSelection((e.Tab + 4 + step) % 4)
		case "d":
			m.openExploreSelection(0)
		case "y":
			m.openExploreSelection(1)
		case "e":
			m.openExploreSelection(2)
		case "r":
			m.openExploreSelection(3)
		case "l":
			m.openExploreSelection(4)
		case "p":
			if e.Tab == 0 && e.Resource.Key() == "pods" {
				m.openExploreSelection(4)
				e.Previous = true
				e.LogKey = ""
			} else if e.Tab == 4 {
				e.Previous = !e.Previous
				m.podLogViewer().Follow()
				e.LogKey = ""
			}
		case "w":
			if e.Tab == 4 {
				v := m.podLogViewer()
				v.Reader.Wrap = !v.Reader.Wrap
			}
		case "t":
			if e.Tab == 4 {
				e.LogTimestamps = !e.LogTimestamps
				m.podLogViewer().SetFormatter(monitor.PodLogFormatter(e.LogTimestamps, e.MergedLogs))
				return
			}
		case "g":
			if e.Tab == 4 {
				m.refreshExplore()
			}
		case "v":
			if e.Tab == 1 && e.Resource.Key() == "secrets" {
				e.Reveal = !e.Reveal
			}
		case "o":
			if e.Resource.Key() == "pods" && (e.Tab == 0 || e.Tab == 4) || e.MergedLogs && e.Tab == 4 {
				e.Picker = "containers"
				e.PickerCursor = 0
				for i, name := range m.logContainers() {
					if name == e.Container {
						e.PickerCursor = i
					}
				}
			}
		case "u":
			if e.Tab == 4 {
				e.Picker = "logrange"
				e.PickerCursor = 0
			}
		case "up", "k", "down", "j", "pgup", "pgdown", "home", "end":
			step := 1
			if key == "pgup" || key == "pgdown" {
				step = max(1, m.height-15)
			}
			if key == "up" || key == "k" || key == "pgup" {
				step = -step
			}
			if e.Tab == 3 {
				e.Scroll = 0
				e.RelatedCursor = max(0, min(len(e.References)-1, e.RelatedCursor+step))
			} else {
				limit := m.exploreDetailScrollLimit()
				e.Scroll = max(0, min(limit, min(e.Scroll, limit)+step))
			}
			if key == "home" {
				e.Scroll = 0
				e.RelatedCursor = 0
			}
			if key == "end" {
				e.Scroll = m.exploreDetailScrollLimit()
				e.RelatedCursor = max(0, len(e.References)-1)
			}
		case "enter":
			if e.Tab == 3 && len(e.References) > 0 {
				ref := e.References[min(e.RelatedCursor, len(e.References)-1)]
				trail := append(e.Trail, e.Location())
				m.chooseExploreResource(ref.Resource.Key())
				e.Trail = trail
				e.Resource = ref.Resource
				if ref.Resource.Namespaced {
					e.Preference.Namespace = ref.Namespace
				}
				if ref.List {
					m.rememberExplore()
					return
				}
				object := &unstructured.Unstructured{Object: map[string]any{}}
				object.SetName(ref.Name)
				object.SetNamespace(ref.Namespace)
				object.SetKind(ref.Resource.Kind)
				object.SetAPIVersion(ref.Resource.GVR().GroupVersion().String())
				object.Object["metadata"].(map[string]any)["uid"] = ref.UID
				e.Detail = object
				e.Container = monitor.PreferredPodContainer(*object)
				e.Tab = 0
				e.DetailValid = false
				m.rememberExplore()
			}
		}
		return
	}
	if key == "backspace" {
		if e.Preference.Filter != "" {
			e.Preference.Filter = ""
			m.rememberExplore()
		}
		return
	}
	_, rows := m.exploreRows()
	cursor := 0
	for i, row := range rows {
		if kubernetes.Identity(row.Object) == e.Selected {
			cursor = i
			break
		}
	}
	switch key {
	case "/":
		m.openExploreInput("filter", e.Preference.Filter)
	case "L":
		m.openExploreInput("selector", e.Preference.Selector)
	case "S":
		e.Picker = "sort"
		e.PickerCursor = e.Preference.Sort
	case "enter", "d":
		m.openExploreSelection(0)
	case "y":
		m.openExploreSelection(1)
	case "l":
		m.status = "Open the resource details first to use Logs."
	case "up", "k":
		cursor--
	case "down", "j":
		cursor++
	case "pgup":
		cursor -= max(1, m.height-15)
	case "pgdown":
		cursor += max(1, m.height-15)
	case "home":
		cursor = 0
	case "end":
		cursor = len(rows) - 1
	case "esc":
		if len(e.Trail) > 0 {
			m.backExploreLocation()
			return
		}
		if m.returnToGitOps() || m.returnToOverview() {
			return
		}
		m.browseExploreResources()
		return
	}
	if len(rows) > 0 {
		e.Selected = kubernetes.Identity(rows[max(0, min(len(rows)-1, cursor))].Object)
	}
}

func (m *model) browseExploreResources() {
	if !m.exploreVisible() || m.explore.Input != nil || m.explore.Picker != "" || m.explore.Detail != nil || m.form != nil || m.settingsOverlay != nil {
		return
	}
	m.restoreView()
	m.leftHidden, m.focus = false, 1
	m.focusRootSidebar()
}

func (m *model) backExploreLocation() {
	e := m.explorer()
	if m.returnFromExploreLogs() || m.returnFromExploreYAML() {
		return
	}
	if len(e.Trail) == 0 && (m.returnToGitOps() || m.returnToOverview()) {
		return
	}
	trail := e.Trail
	m.clearExploreDetail()
	if len(trail) > 0 {
		previous := trail[len(trail)-1]
		e.Trail = trail[:len(trail)-1]
		e.RestoreLocation(previous)
		m.rememberExplore()
	}
}

func (m *model) exploreEditing() bool {
	return m.focus == 1 && !m.helpVisible() && m.form == nil && m.page == "" && m.mode == 1 && m.explore != nil && (m.explore.Input != nil || m.explore.Picker != "")
}

func (m *model) updateExploreInput(msg tea.KeyPressMsg) {
	e := m.explorer()
	key := msg.String()
	if e.Picker == "namespace" {
		m.updateNamespaceInput(msg)
		return
	}
	if key == "esc" {
		e.Input = nil
		e.InputKind = ""
		e.Picker = ""
		m.status = ""
		return
	}
	count := 0
	switch e.Picker {
	case "connections":
		count = len(m.exploreConnections())
	case "containers":
		if e.Detail != nil {
			count = len(m.logContainers())
		}
	case "sort":
		columns, _ := m.exploreRows()
		count = len(columns)
	case "logrange":
		count = len(monitor.LogRanges)
	}
	if e.Picker != "" && (key == "up" || key == "down" || key == "pgup" || key == "pgdown") {
		step := 1
		if key == "pgup" || key == "pgdown" {
			step = 10
		}
		if key == "up" || key == "pgup" {
			step = -step
		}
		e.PickerCursor = max(0, min(count-1, e.PickerCursor+step))
		return
	}
	if key != "enter" {
		if e.Input != nil {
			e.Input.Key(msg, false)
			e.PickerCursor = 0
		}
		return
	}
	if e.Picker != "" {
		if count == 0 {
			return
		}
		index := max(0, min(count-1, e.PickerCursor))
		switch e.Picker {
		case "connections":
			choice := m.exploreConnections()[index]
			command := e.PendingCommand
			if choice.Workspace == "current" {
				if !m.canSwitchCurrent() {
					return
				}
				m.requestCurrent(choice.Connection.Settings["context"], true)
				m.currentRequest.Command = command
				e.Picker, e.InputKind, e.Input = "", "", nil
				return
			}
			m.selectScope(choice.Workspace, "kubernetes", choice.Connection.ID)
			m.focus = 1
			m.mode = 1
			e.Picker, e.InputKind = "", ""
			e.Input = nil
			if command != "" && canonicalCommand(command) != "connections" {
				m.executeExplore(command)
			}
			return
		case "containers":
			e.Container = m.logContainers()[index]
			if e.Tab == 4 && index == 0 {
				e.Container = ""
			}
			if e.Tab == 4 && e.Resource.Key() == "pods" {
				e.MergedLogs = index == 0
			}
			e.LogKey = ""
			m.podLogViewer().Follow()
		case "logrange":
			e.LogSince = monitor.LogRanges[index].Seconds
			e.LogKey = ""
			m.podLogViewer().Follow()
		case "sort":
			if e.Preference.Sort == index {
				e.Preference.Descending = !e.Preference.Descending
			} else {
				e.Preference.Sort = index
				e.Preference.Descending = false
			}
			m.rememberExplore()
		}
		e.Picker = ""
		e.Input = nil
		e.InputKind = ""
		return
	}
	value := strings.TrimSpace(e.Input.Text)
	switch e.InputKind {
	case "namespace":
		if !monitor.ValidExploreNamespace(value) {
			m.status = "Use a namespace name or * for all namespaces."
			return
		}
		m.clearExploreDetail()
		e.Preference.Namespace = monitor.ExploreNamespace(value)
		e.DefaultNamespace = false
	case "filter":
		e.Preference.Filter = value
	case "selector":
		if _, err := labels.Parse(value); err != nil {
			m.status = "Invalid label selector. Example: app=checkout"
			return
		}
		e.Preference.Selector = value
	}
	e.Input = nil
	e.InputKind = ""
	m.status = ""
	m.rememberExplore()
}

func (m *model) exploreConnections() []monitor.ExploreConnection {
	if m.currentRoot {
		return m.currentChoices()
	}
	e := m.explorer()
	query := ""
	if e.Input != nil {
		query = strings.ToLower(e.Input.Text)
	}
	var choices []monitor.ExploreConnection
	for _, w := range m.workspaces {
		for _, c := range w.Connections {
			label := w.Name + " / " + c.Name
			if monitorCluster(c.Tool) && strings.Contains(strings.ToLower(label), query) {
				c.Tool = "kubernetes"
				choices = append(choices, monitor.ExploreConnection{Workspace: w.ID, Label: label, Connection: c})
			}
		}
	}
	return choices
}
