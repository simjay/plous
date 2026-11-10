package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/activity"
	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	diffview "github.com/simjay/plous/internal/ui/diff"
	textview "github.com/simjay/plous/internal/ui/text"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func mouseResourceID(object unstructured.Unstructured) string {
	return kubernetes.Identity(object) + "/" + string(object.GetUID())
}

func (m *model) mouseSettingLabel() string {
	if m.settings.MouseDisabled {
		return "Mouse support: Off"
	}
	return "Mouse support: On"
}

func (m *model) mouseForm(kind, value string) tea.Cmd {
	f := m.form
	helm := false
	if f == nil && m.helmVisible() && m.helmer().Draft != nil {
		f, helm = m.helmer().Draft.Form, true
	}
	if f == nil {
		return nil
	}
	if kind == "field" || kind == "option" {
		for i := range f.Fields {
			if f.Fields[i].Key == value {
				f.Index = i
				if kind == "option" && len(f.Fields[i].Options) > 0 {
					if helm {
						m.helmKey(localMouseKey("right"))
					} else {
						m.updateForm(localMouseKey("right"))
					}
				}
				return nil
			}
		}
	} else if i, err := strconv.Atoi(value); err == nil && i >= 0 && i < len(f.Actions()) {
		f.Index = len(f.Fields) + i
		if helm {
			m.updateHelm("enter")
		} else {
			m.updateForm(localMouseKey("enter"))
		}
	}
	return nil
}

func (m *model) mouseChoose(i int, double bool) tea.Cmd {
	if i < 0 {
		return nil
	}
	if s := m.chatSources; m.chatSourcesVisible() {
		if s.List && i < len(s.Evidence.Records) {
			s.Index = i
			if double {
				return m.chatSourcesKey(localMouseKey("enter"))
			}
		}
		return nil
	}
	if m.focus == 2 {
		c := m.chat()
		if c.Models && i <= len(m.chatModelChoices()) {
			c.ModelCursor = i
			m.updateModelPicker("enter")
		} else if c.History && i < len(m.filteredConversations()) {
			c.Cursor = i
			if double {
				return m.updateChat(localMouseKey("enter"))
			}
		}
		return nil
	}
	if m.settingsOverlay != nil || m.page != "" {
		m.cursor = i
		m.updateMain("enter")
		return nil
	}
	if m.resourceActionVisible() {
		a := m.explore.Action
		if a.Busy || a.Pending != "" {
			return nil
		}
		a.Cursor = i
		return m.updateResourceActionKey(localMouseKey("enter"))
	}

	if m.gitOpsVisible() {
		v := m.gitOpsCurrent()
		if v.Namespaces != nil {
			v.NamespaceCursor = i
			m.gitOpsNamespaceKey(v, localMouseKey("enter"))
		} else if !v.State.Loading && !v.State.FilterInput() {
			v.State.Select(i)
			if double {
				m.updateGitOps("enter")
			}
			m.rememberGitOps(v)
		}
		return nil
	}
	if m.helmVisible() {
		h := m.helmer()
		if h.Busy {
			return nil
		}
		if h.Discard {
			h.Choice = i
			_, cmd := m.helmKey(localMouseKey("enter"))
			return cmd
		}
		if h.Namespaces != nil {
			h.PickerCursor = i
			m.updateHelmNamespaceInput(h, localMouseKey("enter"))
			return nil
		}
		if h.Menu != nil {
			h.Choice = i
			m.activateHelmMenu(h)
			return nil
		}
		h.Cursor = i
		h.Preference.Selected = h.CollectionSelectionKey()
		m.rememberHelm(h)
		if double {
			m.updateHelm("enter")
		}
		return nil
	}
	if m.exploreVisible() && m.explore.Picker != "" {
		m.explore.PickerCursor = i
		m.updateExploreInput(localMouseKey("enter"))
	}
	return nil
}

func (m *model) mouseItem(kind, value string, double bool) tea.Cmd {
	switch kind {
	case "resource-types":
		m.browseExploreResources()

	case "activity-type":
		if m.mode == 2 && m.page == "" && m.detail == "" && m.form == nil && m.settingsOverlay == nil {
			if filter, err := strconv.Atoi(value); err == nil && filter >= 0 && filter <= int(activity.HelmHistory) {
				m.cycleActivityType(filter - int(m.activityUI.Filter))
			}
		}

	case "diffaction":
		source, _, _, _ := m.expandableView()
		if comparison, ok := source.(*diffview.Model); ok {
			i, err := strconv.Atoi(value)
			if err == nil && i >= 0 && i <= 1 {
				comparison.Reader.Searching, comparison.Reader.Selecting = false, false
				comparison.ErrorActive = false
				comparison.Actions, comparison.Action = true, i
				return m.mouseControl("enter")
			}
		}
	case "readersearch":
		source, reader, document, _ := m.expandableView()
		if comparison, ok := source.(*diffview.Model); ok && comparison.ErrorActive {
			reader = &comparison.ErrorReader
		}
		if m.documentActive() && m.explore.Action.Diagnostic != nil {
			reader, document = m.explore.Action.Diagnostic, nil
		}
		if s := m.chatSources; m.chatSourcesVisible() {
			document = s.Document
			if s.ShowReply {
				reader = &s.Reply.Reader
			}
		}
		if document != nil {
			document.ResumeSearch()
		} else if reader != nil {
			reader.Searching = true
		}
	case "execaction":
		if m.resourceActionVisible() && m.explore.Action.Stage == "execcommand" {
			i, err := strconv.Atoi(value)
			if err == nil && i >= 0 && i <= 4 {
				m.explore.Action.ExecEditing = false
				m.explore.Action.ExecChoice = i
				return m.updateExecLaunch(localMouseKey("enter"))
			}
		}
	case "execfield":
		if m.resourceActionVisible() && m.explore.Action.Stage == "execcommand" {
			m.explore.Action.ExecEditing = true
		}
	case "contextname":
		if i, err := strconv.Atoi(value); err == nil {
			m.beginContextName(i)
		}
	case "context":
		if m.form == nil || m.form.Kind != "kube" {
			return nil
		}
		i, err := strconv.Atoi(value)
		if err == nil && i >= 1 && i <= len(m.contexts)+3 {
			m.form.Index = i
			m.updateContextPicker(localMouseKey("enter"))
		}
	case "resource":
		if !m.exploreVisible() || m.explore.Detail != nil {
			return nil
		}
		_, rows := m.exploreRows()
		for _, row := range rows {
			if mouseResourceID(row.Object) == value {
				m.explore.Selected = kubernetes.Identity(row.Object)
				m.rememberExplore()
				if double {
					m.openExploreSelection(0)
				}
				break
			}
		}
	case "section":
		if m.exploreVisible() {
			m.updateExplore(value)
		}
	case "sort":
		if m.helmVisible() {
			h := m.helmer()
			columns, _, _ := h.CollectionData()
			for i, column := range columns {
				if column == value {
					h.Menu, h.MenuKind, h.Choice = monitor.HelmMenuItems(columns), "sort", i
					m.activateHelmMenu(h)
					break
				}
			}
			return nil
		}
		if m.exploreVisible() {
			columns, _ := m.exploreRows()
			for i, col := range columns {
				if col == value {
					m.explore.Preference.Descending = m.explore.Preference.Sort == i && !m.explore.Preference.Descending
					m.explore.Preference.Sort = i
					m.rememberExplore()
					break
				}
			}
		}
	case "related":
		if m.exploreVisible() {
			for i, ref := range m.explore.References {
				if fmt.Sprintf("%s/%s/%s/%s", ref.Resource.Key(), ref.Namespace, ref.Name, ref.Relation) == value {
					m.explore.RelatedCursor = i
					if double {
						m.updateExplore("enter")
					}
					break
				}
			}
		}
	case "activity":
		for i, entry := range m.activityEntries() {
			if mouseActivityID(entry) == value {
				m.setActivityPosition(i)
				if double {
					m.openActivityEntry(entry)
				}
				break
			}
		}
	case "reply", "sources", "readreply":
		c := m.chat()
		for i, turn := range c.Turns {
			if turn.Request.ID == value {
				m.chatEditing = false
				c.Reply, c.ReplySelected = i, true
				if kind == "sources" {
					return m.openChatSources()
				}
				if kind == "readreply" {
					m.openChatReply()
				}
				break
			}
		}
	case "helmrow", "helmrevision", "helmresource", "helmtab":
		return m.mouseHelmItem(kind, value, double)

	case "confirm":
		i, err := strconv.Atoi(value)
		if err != nil || i < 0 || i > 1 {
			return nil
		}
		if m.documentActive() && m.explore.Action.Discard {
			m.explore.Action.DocumentAction = i
			return m.updateDocumentKey(localMouseKey("enter"))
		}
		if m.resourceActionVisible() {
			m.explore.Action.Cursor = i
			return m.updateResourceActionKey(localMouseKey("enter"))
		}
		if m.helmVisible() {
			m.helmer().Choice = i
			m.updateHelm("enter")
		}
	case "documentaction":
		if m.documentActive() {
			a := m.explore.Action
			i, err := strconv.Atoi(value)
			if err == nil && i >= 0 && i < 6 && !a.Busy {
				a.DocumentAction = i
				a.Document.Controls = true
				return m.updateDocumentKey(localMouseKey("enter"))
			}
		}
	case "valueaction":
		if m.valueDocumentActive() {
			a := m.explore.Action
			i, err := strconv.Atoi(value)
			if err == nil && i >= 0 && i < len(monitor.ValueControls(a.Resource)) {
				a.Cursor = i
				a.ValueDocument.Controls = true
				return m.valueDocumentKey(localMouseKey("enter"))
			}
		}
	case "document":
		_, _, d, _ := m.expandableView()
		if d != nil {
			d.Controls = false
		}
	}
	return nil
}

func mouseActivityID(entry activity.ActivityEntry) string {
	return entry.Identity()
}

func (m *model) mouseWheel(span textview.MouseSpan, button tea.MouseButton) tea.Cmd {
	if span.Target == "command" {
		return nil
	}
	delta, key := 3, "down"
	if button == tea.MouseWheelUp {
		delta, key = -3, "up"
	} else if button != tea.MouseWheelDown {
		return nil
	}
	if m.helpVisible() {
		if strings.HasPrefix(span.Target, "help/") {
			i, _ := strconv.Atoi(strings.TrimPrefix(span.Target, "help/"))
			old := m.help.section
			m.help.section = i
			for range 3 {
				m.updateHelp(key)
			}
			m.help.section = old
		}
		return nil
	}
	if s := m.chatSources; m.chatSourcesVisible() {
		if s.ShowReply {
			s.Reply.Reader.Scroll(delta)
		} else if s.Document != nil {
			s.Document.Scroll(delta)
		} else {
			for range 3 {
				m.chatSourcesKey(localMouseKey(key))
			}
		}
		return nil
	}
	if span.Scope == "tree" {
		for range 3 {
			m.updateTree(key)
		}
		return nil
	}
	if span.Scope == "chat" {
		c := m.chat()
		for range 3 {
			if c.Models {
				m.updateModelPicker(key)
			} else if c.History {
				if delta < 0 {
					c.Cursor = max(0, c.Cursor-1)
				} else {
					c.Cursor = min(max(0, len(m.filteredConversations())-1), c.Cursor+1)
				}
			} else {
				m.scrollChat(key)
			}
		}
		return nil
	}
	if span.Scope != "main" {
		return nil
	}
	if m.form != nil {
		return nil
	}
	if m.settingsOverlay != nil {
		for range 3 {
			m.updateMain(key)
		}
		return nil
	}

	if m.exploreVisible() && m.explore.Picker != "" {
		for range 3 {
			m.updateExploreInput(localMouseKey(key))
		}
		return nil
	}
	if m.gitOpsVisible() {
		for range 3 {
			m.updateGitOps(key)
		}
		return nil
	}
	if m.helmVisible() {
		h := m.helmer()
		if h.Namespaces != nil {
			for range 3 {
				m.updateHelmNamespaceInput(h, localMouseKey(key))
			}
			return nil
		}
		if h.Menu != nil || h.Confirm {
			for range 3 {
				m.updateHelm(key)
			}
			return nil
		}
		if h.Discard || h.Input != nil {
			return nil
		}
	}
	if m.resourceActionVisible() && !m.documentActive() && !m.valueDocumentActive() {
		if m.explore.Action.Stage == "scale" || m.explore.Action.Stage == "export" || m.explore.Action.Stage == "execcommand" {
			return nil
		}
		for range 3 {
			m.updateResourceActionKey(localMouseKey(key))
		}
		return nil
	}
	source, reader, document, _ := m.expandableView()
	if comparison, ok := source.(*diffview.Model); ok && comparison.ErrorActive {
		reader = &comparison.ErrorReader
	}
	if m.documentActive() && m.explore.Action.Diagnostic != nil {
		reader, document = m.explore.Action.Diagnostic, nil
	}
	if m.documentActive() && m.explore.Action.Discard {
		return nil
	}
	if document != nil {
		document.Scroll(delta)
		return nil
	}
	if reader != nil {
		if m.logWorkspaceActive() {
			m.podLogViewer().Pause()
		}
		reader.Scroll(delta)
		return nil
	}
	if m.helmVisible() && m.helmer().Draft != nil && m.helmer().Plan == nil && m.helmer().WorkflowVisible() {
		return nil
	}
	for range 3 {
		m.updateMain(key)
	}
	return nil
}

func (m *model) mouseHelmItem(kind, value string, double bool) tea.Cmd {
	if !m.helmVisible() {
		return nil
	}
	h := m.helmer()
	if h.Busy || h.Confirm || h.Discard || h.Menu != nil || h.Document != nil || h.Comparison != nil {
		return nil
	}
	switch kind {
	case "helmrow":
		index := 0
		for _, group := range h.ChartGroups() {
			for _, release := range group.Releases {
				if release.Key() == value {
					h.Cursor = index
					h.Preference.Selected = release.Key()
					m.rememberHelm(h)
					if double {
						m.updateHelm("enter")
					}
					return nil
				}
				index++
			}
		}
	case "helmrevision":
		if h.Timeline == nil {
			return nil
		}
		for i, r := range h.Timeline.Revisions {
			if strconv.Itoa(r.Revision) == value {
				h.Timeline.Cursor, h.Timeline.Scroll = i, 0
				if double {
					m.helmTimelineKey(h, "enter")
				}
				break
			}
		}
	case "helmresource":
		rs, _ := h.InspectorResources()
		for i, r := range rs {
			if r.Kind+"/"+r.Namespace+"/"+r.Name == value {
				h.Subcursor = i
				if double {
					m.updateHelm("enter")
				}
				break
			}
		}
	case "helmtab":
		i, err := strconv.Atoi(value)
		if err == nil {
			for _, tab := range monitor.HelmInspectorTabs {
				if tab == i {
					h.SaveInspectorPosition()
					h.Tab = i
					h.Subcursor = h.InspectorPositions[h.InspectorPositionKey()]
					h.Find = nil
					break
				}
			}
		}
	}
	return nil
}
