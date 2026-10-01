package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/kubernetes"
	"github.com/simjay/plous/internal/workspace"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/validation"
)

var resourceCommands = map[string]string{
	"po": "pods", "pod": "pods", "pods": "pods",
	"deploy": "deployments.apps", "deployment": "deployments.apps", "deployments": "deployments.apps",
	"sts": "statefulsets.apps", "statefulsets": "statefulsets.apps",
	"ds": "daemonsets.apps", "daemonsets": "daemonsets.apps",
	"rs": "replicasets.apps", "replicasets": "replicasets.apps",
	"job": "jobs.batch", "jobs": "jobs.batch", "cj": "cronjobs.batch", "cronjobs": "cronjobs.batch",
	"svc": "services", "services": "services", "ep": "endpoints", "endpoints": "endpoints",
	"eps": "endpointslices.discovery.k8s.io", "endpointslices": "endpointslices.discovery.k8s.io",
	"ing": "ingresses.networking.k8s.io", "ingresses": "ingresses.networking.k8s.io",
	"netpol": "networkpolicies.networking.k8s.io", "networkpolicies": "networkpolicies.networking.k8s.io",
	"cm": "configmaps", "configmaps": "configmaps", "sec": "secrets", "secrets": "secrets",
	"pvc": "persistentvolumeclaims", "persistentvolumeclaims": "persistentvolumeclaims",
	"pv": "persistentvolumes", "persistentvolumes": "persistentvolumes",
	"sc": "storageclasses.storage.k8s.io", "storageclasses": "storageclasses.storage.k8s.io",
	"no": "nodes", "nodes": "nodes", "ns": "namespaces", "namespaces": "namespaces",
	"ev": "events", "events": "events", "sa": "serviceaccounts", "serviceaccounts": "serviceaccounts",
	"crd": "customresourcedefinitions.apiextensions.k8s.io", "customresourcedefinitions": "customresourcedefinitions.apiextensions.k8s.io",
}

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
		if !m.exploreVisible() || m.explore == nil || m.explore.detail == nil {
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
	case "kinds", "namespace", "connections", "refresh", "yaml", "describe":
		special = name
	}
	resource, known := "", false
	if special == "" {
		resource, known = resourceCommands[name]
		if !known && m.explore != nil {
			if found, ok := kubernetes.Find(m.explore.resources, name); ok {
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
	if arg != "" && (known || special == "namespace") && !validExploreNamespace(arg) {
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
		e.picker = "connections"
		e.pickerCursor = 0
		e.inputKind = "connections"
		input := newEditor("")
		e.input = &input
		e.requested = resource
		e.pendingCommand = command
		return true
	}
	if e.key == "" || e.connecting {
		e.pendingCommand = command
		if known {
			e.requested = resource
		}
		return true
	}
	e.pendingCommand = ""
	if known {
		m.chooseExploreResource(resource)
		if arg != "" {
			e.preference.Namespace = exploreNamespace(arg)
			e.defaultNamespace = false
		}
		m.rememberExplore()
		return true
	}
	switch special {
	case "kinds":
		m.openExploreInput("kinds", "")
		e.picker = "kinds"
		e.pickerCursor = 0
	case "namespace":
		if arg != "" {
			m.chooseExploreNamespace(exploreNamespace(arg))
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
func validExploreNamespace(text string) bool {
	return text == "*" || len(validation.IsDNS1123Label(text)) == 0
}
func exploreNamespace(text string) string {
	if text == "*" {
		return ""
	}
	return text
}
func (m *model) openExploreInput(kind, text string) {
	e := m.explorer()
	input := newEditor(text)
	e.input = &input
	e.inputKind = kind
	e.pickerCursor = 0
	m.shortcutPrefix = ""
}
func (m *model) clearExploreDetail() {
	e := m.explorer()
	m.stopResourceAction()
	m.stopResourceTerminal()
	if e.detailRun != nil {
		e.detailRun.cancel()
		e.detailRun = nil
	}
	if e.log != nil {
		e.log.cancel()
		e.log = nil
	}
	e.detail = nil
	e.detailSections = [4]detailSection{}
	e.detailIdentity, e.detailLoaded = "", false
	e.yamlDocument = nil
	e.yamlOrigin = nil
	e.detailKey = ""
	e.logKey = ""
	m.resetExploreLogs()
	e.trail = nil
	e.logOrigin = nil
	e.scroll = 0
	e.reveal = false
	e.detailValid = false
	e.container = ""
	e.previous = false
	e.mergedLogs = false
}
func (m *model) chooseExploreResource(resource string) {
	e := m.explorer()
	if e.action != nil {
		m.status = "Finish or cancel the open resource action first."
		return
	}
	if e.key == "" {
		preference, ok := m.explorePreferences[m.active+"/"+m.connection]
		if !ok {
			preference.Namespace = "default"
			if c := m.currentConnection(); c != nil && c.Settings["namespace"] != "" {
				preference.Namespace = c.Settings["namespace"]
			}
		}
		e.preference = preference
	}
	m.clearExploreDetail()
	e.preference.Resource = resource
	e.preference.Filter = ""
	e.preference.Selector = ""
	e.preference.Sort = 0
	e.preference.Descending = false
	e.picker = ""
	e.input = nil
	e.inputKind = ""
	e.selected = ""
	if e.key == "" {
		e.requested = resource
	} else {
		e.requested = ""
	}
	m.rememberExplore()
}
func (m *model) refreshExplore() {
	e := m.explorer()
	if e.detail != nil {
		if e.detailRun != nil {
			e.detailRun.cancel()
			e.detailRun = nil
		}
		if e.log != nil {
			e.log.cancel()
			e.log = nil
		}
		e.detailKey, e.logKey = "", ""
		return
	}
	m.rememberExplore()
	m.stopExplore()
	e.key = ""
	e.requested = e.preference.Resource
}
func (m *model) openExploreSelection(tab int) {
	e := m.explorer()
	if tab != 4 && e.tab == 4 {
		m.returnFromExploreLogs()
	}
	if tab == 4 && e.resource.Key() != "pods" {
		if e.detail != nil && kubernetes.HasPodTargets(e.resource) {
			m.openResourceActions("logs")
		} else {
			m.status = "Open a Pod or workload's details to choose its logs."
		}
		return
	}
	if tab == 1 && e.detail != nil && e.tab != 1 {
		e.yamlOrigin = &detailSectionReturn{tab: e.tab, scroll: e.scroll, relatedCursor: e.relatedCursor}
	} else if tab != 1 && tab != 4 {
		e.yamlOrigin = nil
	}
	if e.detail == nil {
		_, rows := m.exploreRows()
		if len(rows) == 0 {
			return
		}
		object := rows[0].Object
		for _, row := range rows {
			if kubernetes.Identity(row.Object) == e.selected {
				object = row.Object
				break
			}
		}
		if len(e.trail) > 0 {
			e.trail = append(e.trail, exploreLocation{preference: e.preference, selected: kubernetes.Identity(object)})
		}
		e.detail = object.DeepCopy()
		e.selected = kubernetes.Identity(object)
		e.detailKey = ""
		e.detailValid = false
		e.tab = 0
		e.container = preferredPodContainer(object)
		e.previous = false
	}
	if tab == 4 && e.tab != 4 {
		origin := e.location()
		e.logOrigin = &origin
	}
	e.tab = tab
	e.scroll = 0
	e.reveal = false
	if tab == 4 {
		m.podLogViewer().Follow()
	}
	e.picker = ""
	e.input = nil
	e.inputKind = ""
}

func (m *model) updateExplore(key string) {
	e := m.explorer()
	if key == "n" {
		m.openNamespacePicker()
		return
	}
	if key == "g" && e.detail == nil {
		m.refreshExplore()
		return
	}
	if e.detail != nil {
		if m.runDetailActionKey(key) {
			return
		}
		switch key {
		case "a":
			m.openResourceActions("")
		case "esc":
			m.backExploreLocation()
		case "[", "]":
			if e.tab == 4 {
				m.returnFromExploreLogs()
			}
			step := 1
			if key == "[" {
				step = -1
			}
			m.openExploreSelection((e.tab + 4 + step) % 4)
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
			if e.tab == 0 && e.resource.Key() == "pods" {
				m.openExploreSelection(4)
				e.previous = true
				e.logKey = ""
			} else if e.tab == 4 {
				e.previous = !e.previous
				m.podLogViewer().Follow()
				e.logKey = ""
			}
		case "w":
			if e.tab == 4 {
				v := m.podLogViewer()
				v.Reader.Wrap = !v.Reader.Wrap
			}
		case "t":
			if e.tab == 4 {
				e.logTimestamps = !e.logTimestamps
				m.podLogViewer().SetFormatter(podLogFormatter(e.logTimestamps, e.mergedLogs))
				return
			}
		case "g":
			if e.tab == 4 {
				m.refreshExplore()
			}
		case "v":
			if e.tab == 1 && e.resource.Key() == "secrets" {
				e.reveal = !e.reveal
			}
		case "o":
			if e.resource.Key() == "pods" && (e.tab == 0 || e.tab == 4) || e.mergedLogs && e.tab == 4 {
				e.picker = "containers"
				e.pickerCursor = 0
				for i, name := range m.logContainers() {
					if name == e.container {
						e.pickerCursor = i
					}
				}
			}
		case "u":
			if e.tab == 4 {
				e.picker = "logrange"
				e.pickerCursor = 0
			}
		case "up", "k", "down", "j", "pgup", "pgdown", "home", "end":
			step := 1
			if key == "pgup" || key == "pgdown" {
				step = max(1, m.height-15)
			}
			if key == "up" || key == "k" || key == "pgup" {
				step = -step
			}
			if e.tab == 3 {
				e.scroll = 0
				e.relatedCursor = max(0, min(len(e.references)-1, e.relatedCursor+step))
			} else {
				limit := m.exploreDetailScrollLimit()
				e.scroll = max(0, min(limit, min(e.scroll, limit)+step))
			}
			if key == "home" {
				e.scroll = 0
				e.relatedCursor = 0
			}
			if key == "end" {
				e.scroll = m.exploreDetailScrollLimit()
				e.relatedCursor = max(0, len(e.references)-1)
			}
		case "enter":
			if e.tab == 3 && len(e.references) > 0 {
				ref := e.references[min(e.relatedCursor, len(e.references)-1)]
				trail := append(e.trail, e.location())
				m.chooseExploreResource(ref.Resource.Key())
				e.trail = trail
				e.resource = ref.Resource
				if ref.Resource.Namespaced {
					e.preference.Namespace = ref.Namespace
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
				e.detail = object
				e.container = preferredPodContainer(*object)
				e.tab = 0
				e.detailValid = false
				m.rememberExplore()
			}
		}
		return
	}
	if key == "backspace" {
		if e.preference.Filter != "" {
			e.preference.Filter = ""
			m.rememberExplore()
		}
		return
	}
	_, rows := m.exploreRows()
	cursor := 0
	for i, row := range rows {
		if kubernetes.Identity(row.Object) == e.selected {
			cursor = i
			break
		}
	}
	switch key {
	case "/":
		m.openExploreInput("filter", e.preference.Filter)
	case "f":
		m.openExploreInput("selector", e.preference.Selector)
	case "S":
		e.picker = "sort"
		e.pickerCursor = e.preference.Sort
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
		if len(e.trail) > 0 {
			m.backExploreLocation()
			return
		}
		m.focusRootSidebar()
		return
	}
	if len(rows) > 0 {
		e.selected = kubernetes.Identity(rows[max(0, min(len(rows)-1, cursor))].Object)
	}
}

func (m *model) backExploreLocation() {
	e := m.explorer()
	if m.returnFromExploreLogs() || m.returnFromExploreYAML() {
		return
	}
	trail := e.trail
	m.clearExploreDetail()
	if len(trail) > 0 {
		previous := trail[len(trail)-1]
		e.trail = trail[:len(trail)-1]
		e.restoreLocation(previous)
		m.rememberExplore()
	}
}

func (m *model) exploreEditing() bool {
	return m.focus == 1 && m.help == nil && m.form == nil && m.page == "" && m.mode == 1 && m.explore != nil && (m.explore.input != nil || m.explore.picker != "")
}
func (m *model) updateExploreInput(msg tea.KeyPressMsg) {
	e := m.explorer()
	key := msg.String()
	if e.picker == "namespace" {
		m.updateNamespaceInput(msg)
		return
	}
	if key == "esc" {
		e.input = nil
		e.inputKind = ""
		e.picker = ""
		m.status = ""
		return
	}
	count := 0
	switch e.picker {
	case "kinds":
		count = len(m.exploreKinds())
	case "connections":
		count = len(m.exploreConnections())
	case "containers":
		if e.detail != nil {
			count = len(m.logContainers())
		}
	case "sort":
		columns, _ := m.exploreRows()
		count = len(columns)
	case "logrange":
		count = len(logRanges)
	}
	if e.picker != "" && (key == "up" || key == "down" || key == "pgup" || key == "pgdown") {
		step := 1
		if key == "pgup" || key == "pgdown" {
			step = 10
		}
		if key == "up" || key == "pgup" {
			step = -step
		}
		e.pickerCursor = max(0, min(count-1, e.pickerCursor+step))
		return
	}
	if e.picker == "kinds" && (key == "left" || key == "right") {
		step := 1
		if key == "left" {
			step = -1
		}
		categories := m.exploreCategories()
		if len(categories) == 0 {
			return
		}
		e.group = (e.group + len(categories) + step) % len(categories)
		e.pickerCursor = 0
		e.input.text = ""
		e.input.cursor = 0
		return
	}
	if key != "enter" {
		if e.input != nil {
			e.input.key(msg, false)
			e.pickerCursor = 0
		}
		return
	}
	if e.picker != "" {
		if count == 0 {
			return
		}
		index := max(0, min(count-1, e.pickerCursor))
		switch e.picker {
		case "kinds":
			m.chooseExploreResource(m.exploreKinds()[index].Key())
		case "connections":
			choice := m.exploreConnections()[index]
			command := e.pendingCommand
			m.selectScope(choice.workspace, "kubernetes", choice.connection.ID)
			m.focus = 1
			m.mode = 1
			e.picker, e.inputKind = "", ""
			e.input = nil
			if command != "" && canonicalCommand(command) != "connections" {
				m.executeExplore(command)
			}
			return
		case "containers":
			e.container = m.logContainers()[index]
			if e.tab == 4 && index == 0 {
				e.container = ""
			}
			if e.tab == 4 && e.resource.Key() == "pods" {
				e.mergedLogs = index == 0
			}
			e.logKey = ""
			m.podLogViewer().Follow()
		case "logrange":
			e.logSince = logRanges[index].seconds
			e.logKey = ""
			m.podLogViewer().Follow()
		case "sort":
			if e.preference.Sort == index {
				e.preference.Descending = !e.preference.Descending
			} else {
				e.preference.Sort = index
				e.preference.Descending = false
			}
			m.rememberExplore()
		}
		e.picker = ""
		e.input = nil
		e.inputKind = ""
		return
	}
	value := strings.TrimSpace(e.input.text)
	switch e.inputKind {
	case "namespace":
		if !validExploreNamespace(value) {
			m.status = "Use a namespace name or * for all namespaces."
			return
		}
		m.clearExploreDetail()
		e.preference.Namespace = exploreNamespace(value)
		e.defaultNamespace = false
	case "filter":
		e.preference.Filter = value
	case "selector":
		if _, err := labels.Parse(value); err != nil {
			m.status = "Invalid label selector. Example: app=checkout"
			return
		}
		e.preference.Selector = value
	}
	e.input = nil
	e.inputKind = ""
	m.status = ""
	m.rememberExplore()
}

func (m *model) exploreKinds() []kubernetes.Resource {
	e := m.explorer()
	query := ""
	if e.input != nil {
		query = strings.ToLower(e.input.text)
	}
	var resources []kubernetes.Resource
	for _, resource := range e.resources {
		if query != "" {
			if strings.Contains(strings.ToLower(resource.Kind+" "+resource.Key()+" "+strings.Join(resource.ShortNames, " ")), query) {
				resources = append(resources, resource)
			}
		} else if kubernetes.Category(resource) == m.exploreCategory() {
			resources = append(resources, resource)
		}
	}
	return resources
}

type exploreConnection struct {
	workspace, label string
	connection       workspace.Connection
}

func (m *model) exploreConnections() []exploreConnection {
	e := m.explorer()
	query := ""
	if e.input != nil {
		query = strings.ToLower(e.input.text)
	}
	var choices []exploreConnection
	for _, w := range m.workspaces {
		for _, c := range w.Connections {
			label := w.Name + " / " + c.Name
			if c.Tool == "kubernetes" && strings.Contains(strings.ToLower(label), query) {
				choices = append(choices, exploreConnection{workspace: w.ID, label: label, connection: c})
			}
		}
	}
	return choices
}
