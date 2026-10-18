package app

import (
	appactivity "github.com/simjay/plous/internal/app/activity"
	appcode "github.com/simjay/plous/internal/app/code"
	"github.com/simjay/plous/internal/app/monitor"
	appsettings "github.com/simjay/plous/internal/app/settings"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	helpview "github.com/simjay/plous/internal/ui/help"
	"maps"
	"strings"
)

func (m *model) helpSections() [3]helpview.Section {
	sections := [3]helpview.Section{m.workspaceHelp(), m.mainHelp(), m.chatHelp()}
	for i := range sections {
		sections[i].Note("Copy Plous selections with the footer Copy button; viewers also use y. Native terminal copy uses terminal selection. Paste into active text input with the terminal Paste command. Ctrl+C exits outside active native agent input.")
		m.resolveHelpCommands(&sections[i])
	}
	return sections
}

func (m *model) resolveHelpCommands(s *helpview.Section) {
	commands := map[string]commandDefinition{}
	for _, c := range applicationCommands {
		commands[c.name] = c
	}
	for _, c := range monitor.ResourceActionDefinitions {
		commands[c.Command] = commandDefinition{key: c.Alias, name: c.Command}
	}
	for gi := range s.Groups {
		for ei := range s.Groups[gi].Entries {
			e := &s.Groups[gi].Entries[ei]
			if c, ok := commands[e.Command]; ok {
				for _, binding := range []string{":" + c.key, ":" + c.name} {
					if binding != e.Key {
						e.Alternatives = append(e.Alternatives, binding)
					}
				}
			}
		}
	}
}

func (m *model) workspaceHelp() helpview.Section {
	if search := m.searches[0]; search != nil && !search.finished {
		s := helpview.Search("Workspaces · Search", "Open selected result")
		s.Add("Results", helpview.Key("select", "↑↓", "Select result"), helpview.Key("page", "PgUp / PgDn", "Jump ten rows"))
		return s
	}
	s := helpview.New("Workspaces", helpview.Key("select", "↑↓", "Select row"), helpview.Key("open", "Enter", "Select / open"), helpview.Key("filter", "/", "Search tree"))
	rows := m.treeRowsRead()
	if m.treeCursor >= 0 && m.treeCursor < len(rows) {
		r := rows[m.treeCursor]
		s.Title = "Workspaces · " + r.label
		if r.isBranch() {
			s.Add("Branch", helpview.Key("fold", "←→", "Fold / expand"))
		}
		if r.sourceRefreshable() {
			s.Add("Source", helpview.Command("refresh", "g", "Refresh source files"))
		} else if r.kind == "current" || r.kind == "capability" || r.kind == "gitopssection" || r.kind == "helmsection" || r.kind == "discovery" || r.kind == "connection" && monitorCluster(r.tool) {
			s.Add("Connection", helpview.Command("refresh", "g", "Refresh tool discovery"))
		}
		if r.isAddAction() || r.kind == "savecurrent" || r.kind == "choosecurrent" || r.kind == "settings" {
			s.Groups[0].Entries[1].Label = r.label
		}
	}
	s.Add("Manage", helpview.Command("new", ":n", "New workspace"), helpview.Command("connections", ":K", "Choose connection"))
	s.Add("Navigation", helpview.Key("ends", "Home / End", "First / last"), helpview.Key("page", "PgUp / PgDn", "Jump ten rows"))
	if m.searches[0] != nil {
		s.Add("Search", helpview.Key("back", "Esc", "Clear tree search"))
	}
	// Global alternatives remain reachable without filling the fixed global box.
	for _, e := range globalHelpSection().Groups[0].Entries {
		s.Add("Global commands", e)
	}
	return s
}

func (m *model) mainHelp() helpview.Section {
	if m.presentedDocumentVisible() && m.form == nil && m.settingsOverlay == nil {
		return m.uiDocument.model.Help("Assistant document", m.uiDocument.model.Target, "Back")
	}
	if m.form != nil {
		return m.form.Help()
	}
	if m.settingsOverlay != nil {
		switch m.page {
		case "tools":
			return helpview.Picker("Add connection", "Open tool setup")
		case "remove", "removeprovider":
			return helpview.Picker("Remove connection", "Activate selected choice")
		}
		return (*appsettings.Form)(nil).Help()
	}
	if s := m.activePanelSearch(1); s != nil && !s.finished {
		return helpview.Search("Find / filter", "Finish search / open selection")
	}
	if m.mode == 2 && m.page == "" {
		if m.activityReview != nil {
			return m.activityReview.Comparison.Help("Activity comparison (read only)", "", "Return to saved summary")
		}
		state := appactivity.HelpContext{Detail: m.detail != "", LoadError: m.activityUI.Error != ""}
		if m.operationDetail != nil {
			state.OpenRelease = m.activityReleaseUnavailable(m.operationDetail) == ""
			state.Observations = m.activityHasObservations()
		} else {
			state.Compare = m.activityDetailActive()
		}
		return state.Help()
	}
	if m.overviewVisible() {
		if v := m.overviewRead(); v != nil {
			return v.State.Help()
		}
		return (&monitor.OverviewState{}).Help()
	}
	if m.mode != 1 || m.page != "" {
		s := helpview.New("Overview", helpview.Key("back", "Esc", "Back / sidebar"))
		if m.current() == nil || len(m.current().Connections) == 0 {
			s.Add("Workspace", helpview.Key("setup", "Enter", "Add connection"))
		}
		s.Add("Read", helpview.Find()...)
		s.Add("Navigation", helpview.Scroll()...)
		return s
	}
	if m.explore != nil && m.explore.Picker == "connections" {
		return m.explore.Help()
	}
	c := m.selectedConnection()
	if c != nil && sourceConnection(c.Tool) {
		var s *appcode.CodeState
		if c := m.selectedConnection(); c != nil {
			s = m.savedCodeState(m.active, c.ID)
		}
		return s.Help()
	}
	if c != nil && monitorCluster(c.Tool) && m.tool == "helm" {
		connection := *c
		connection.Settings = maps.Clone(c.Settings)
		if connection.Settings == nil {
			connection.Settings = map[string]string{}
		}
		connection.Settings["driver"] = m.helmDriver(m.active, *c)
		c = &connection
		key := m.active + "/" + c.ID + "/" + monitor.HelmConfig(c).Fingerprint()
		return m.helmStates[key].Help()
	}
	if c != nil && monitorCluster(c.Tool) && gitOpsTool(m.tool) {
		if c := m.selectedConnection(); c != nil {
			root := m.active + "/" + c.ID + "/" + m.tool
			preference := m.gitOpsPreferences[root]
			section := gitops.NormalizeSection(gitops.Tool(m.tool), preference.Section)
			if preference.Section == "" && strings.Contains(preference.Selected, "/HelmRelease/") && m.tool == string(gitops.Flux) {
				section = gitops.HelmReleases
			}
			for _, v := range m.gitOpsViews {
				if v.Cluster.Key == clusterKey(m.active, *c) && v.State.Tool == gitops.Tool(m.tool) && v.State.Section == section {
					if v.Namespaces != nil {
						return v.Namespaces.Help(v.State.Title() + " · Namespace")
					}
					return v.State.Help()
				}
			}
		}
		return helpview.New("Monitor · "+m.tool, helpview.Command("refresh", "g", "Refresh connection"))
	}
	s := m.explore.Help()
	if m.explore != nil && m.explore.Detail != nil && m.explore.Tab != 1 && m.explore.Tab != 4 && m.explore.Action == nil && m.explore.Terminal == nil {
		s.Note(strings.TrimSpace(m.usageHelp()))
	}
	return s
}

func (m *model) chatHelp() helpview.Section {
	if m.externalAgentSelected() {
		s := helpview.New(m.settings.MainAgent.Label() + " terminal")
		s.Note("The installed CLI owns input, menus and conversation history. The session keeps its captured connection.")
		s.Add("", helpview.Command("chat", "cc / :c", "Enter terminal"))
		s.Add("", helpview.Command("stopchat", ":X", "Close native session"))
		s.Note("Alt+Left/Right move panels directly. Ctrl+] switches between the agent and previous panel. Ctrl+C goes to the active native terminal for its interrupt behavior. To save and exit Plous, press Ctrl+] then Ctrl+C. Use the CLI's /resume for history.")
		s.Note("Wheel or PgUp/PgDn reads terminal output. End or Esc returns to live output while reading; typing or paste also returns live. Up/Down stays with the native prompt while captured. Fullscreen CLI views handle their own scrolling.")
		return s
	}
	if m.chatSourcesVisible() {
		return m.chatSources.Help()
	}
	if s := m.searches[2]; s != nil && !s.finished {
		return helpview.Search("AI chat · Find", "Finish search / open conversation")
	}
	return m.chat().Help(m.chatEditing, m.run != nil && !m.run.Finished, m.settings.MainAgent)
}

func globalHelpSection() helpview.Section {
	s := helpview.New("Global")
	for _, command := range []struct{ name, label string }{{"settings", "Settings"}, {"help", "Help"}, {"sidebar", "Sidebar"}, {"aiinteraction", "AI panel"}, {"fullscreen", "Fullscreen"}} {
		s.Add("", helpview.Command(command.name, navigationBinding(command.name), command.label))
	}
	s.Add("", helpview.Command("quit", ":quit", "Save and exit"))
	return s
}
