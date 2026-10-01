package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/kubeconfig"
	"github.com/simjay/plous/internal/kubernetes"
	"github.com/simjay/plous/internal/workspace"
)

const (
	minWidth  = 80
	minHeight = 16
)

var modes = []string{"Overview", "Explore", "Activity"}
var toolTypes = []string{"kubernetes", "helm", "kustomize", "terraform", "argo", "kargo"}
var toolNames = map[string]string{"kubernetes": "Kubernetes", "helm": "Helm", "kustomize": "Kustomize", "terraform": "Terraform", "argo": "Argo", "kargo": "Kargo"}

type session struct {
	Version      int                          `json:"version"`
	Workspace    string                       `json:"workspace"`
	Connection   string                       `json:"connection"`
	Tool         string                       `json:"tool"`
	Mode         int                          `json:"mode"`
	Focus        int                          `json:"focus"`
	LeftHidden   bool                         `json:"left_hidden,omitempty"`
	RightHidden  bool                         `json:"right_hidden,omitempty"`
	TreeKey      string                       `json:"tree_key"`
	Expanded     map[string]bool              `json:"expanded"`
	AutoExpanded map[string]bool              `json:"auto_expanded,omitempty"`
	Drafts       map[string]string            `json:"drafts"`
	Positions    map[string]int               `json:"positions"`
	ChatIDs      map[string]string            `json:"chat_ids,omitempty"`
	Explore      map[string]explorePreference `json:"explore,omitempty"`
}

type autosaveMsg struct{}

type model struct {
	version                    string
	dirty                      bool
	lastSession                []byte
	width, height, focus, mode int
	store                      *workspace.Store
	workspaces                 []workspace.Workspace
	active, connection, tool   string
	expanded                   map[string]bool
	autoExpanded               map[string]bool
	searches                   [3]*panelSearch
	treeCursor                 int
	drafts                     map[string]*editor
	positions                  map[string]int
	page                       string
	settingsOverlay            *settingsOverlay
	toolSetup                  *toolSetup
	cursor                     int
	form                       *form
	command                    *editor
	commandFocus               int
	shortcutPrefix             string
	help                       *helpState
	leftHidden, rightHidden    bool
	viewExpansion              *viewExpansion
	status                     string
	loadingFrame               int
	loadingTickPending         bool
	contexts                   []kubeconfig.Context
	chosen                     map[int]bool
	activityReview             *activityReview
	changes                    []workspace.Change
	removeID                   string
	removeProfile              string
	detail                     string
	chatEditing                bool
	settings                   workspace.Settings
	chats                      map[string]*chatState
	chatIDs                    map[string]string
	run                        *chatRun
	explore                    *exploreState
	explorePreferences         map[string]explorePreference
	exploreGeneration          uint64
	exploreClosed              bool
	exploreCatalogs            map[string][]kubernetes.Resource
	exploreTreeSelection       string
}

func newModel(store *workspace.Store, version string) (*model, error) {
	m := &model{version: version, store: store, focus: 1, expanded: map[string]bool{}, drafts: map[string]*editor{}, positions: map[string]int{}}
	var err error
	m.chatIDs = map[string]string{}
	m.settings, err = store.ReadSettings()
	if err != nil {
		return nil, err
	}
	m.workspaces, err = store.List()
	if err != nil {
		return nil, err
	}
	data, err := store.ReadSession()
	if err != nil {
		return nil, err
	}
	if len(data) > 0 {
		var s session
		if err = json.Unmarshal(data, &s); err != nil {
			return nil, fmt.Errorf("read saved session: %w", err)
		}
		if s.Version != 1 && s.Version != 2 {
			return nil, fmt.Errorf("unsupported session version %d", s.Version)
		}
		if s.Version == 1 {
			// The old third mode was Deployment; Activity moved from fourth to third.
			switch s.Mode {
			case 2:
				s.Mode = 0
			case 3:
				s.Mode = 2
			}
			for key, value := range s.Positions {
				if strings.HasSuffix(key, "/Resources") {
					s.Positions[strings.TrimSuffix(key, "/Resources")+"/Explore"] = value
					delete(s.Positions, key)
				}
			}
		}
		if s.ChatIDs != nil {
			m.chatIDs = s.ChatIDs
		}
		m.explorePreferences = s.Explore
		m.active, m.connection, m.tool = s.Workspace, s.Connection, s.Tool
		m.mode = max(0, min(len(modes)-1, s.Mode))
		m.focus = max(0, min(2, s.Focus))
		m.leftHidden, m.rightHidden = s.LeftHidden, s.RightHidden
		m.ensureVisibleFocus()
		if s.Expanded != nil {
			m.expanded = s.Expanded
		}
		m.autoExpanded = s.AutoExpanded
		if s.Positions != nil {
			m.positions = s.Positions
			for key, value := range m.positions {
				m.positions[key] = max(0, min(1000000, value))
			}
		}
		for id, text := range s.Drafts {
			e := newEditor(text)
			e.literal = text != ""
			m.drafts[id] = &e
		}
		if m.current() == nil {
			m.active = ""
			m.connection = ""
			m.tool = ""
		}
		if m.connection != "" && m.currentConnection() == nil {
			m.connection = ""
			m.tool = ""
		}
		for i, row := range m.treeRows() {
			if row.key == canonicalTreeKey(s.TreeKey) {
				m.treeCursor = i
				break
			}
		}
	}
	if m.active == "" && len(m.workspaces) > 0 && len(data) == 0 {
		m.active = m.workspaces[0].ID
		m.expanded[m.active] = true
	}
	m.loadHistory()
	m.loadChat()
	return m, nil
}
func (m *model) Init() tea.Cmd { return tea.Batch(autosave(), m.syncExplore(), m.syncLoading()) }
func autosave() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return autosaveMsg{} })
}
func (m *model) current() *workspace.Workspace {
	for i := range m.workspaces {
		if m.workspaces[i].ID == m.active {
			return &m.workspaces[i]
		}
	}
	return nil
}
func (m *model) currentConnection() *workspace.Connection {
	w := m.current()
	if w != nil {
		for i := range w.Connections {
			if w.Connections[i].ID == m.connection {
				return &w.Connections[i]
			}
		}
	}
	return nil
}
func (m *model) draft() *editor {
	if m.drafts[m.active] == nil {
		e := newEditor("")
		m.drafts[m.active] = &e
	}
	return m.drafts[m.active]
}
func (m *model) saveSession() error {
	if m.store == nil {
		return nil
	}
	if err := m.flushChats(); err != nil {
		return err
	}
	s := session{ChatIDs: m.chatIDs, Version: 2, Workspace: m.active, Connection: m.connection, Tool: m.tool, Mode: m.mode, Focus: m.focus, Expanded: m.expanded, Drafts: map[string]string{}, Positions: m.positions}
	s.LeftHidden, s.RightHidden = m.leftHidden, m.rightHidden
	s.Explore = m.explorePreferences
	s.AutoExpanded = m.autoExpanded
	rows := m.treeRows()
	if m.treeCursor >= 0 && m.treeCursor < len(rows) {
		s.TreeKey = rows[m.treeCursor].key
	}
	if search := m.searches[0]; search != nil {
		s.TreeKey = search.treeKey
	}
	if search := m.searches[1]; search != nil && search.kind == "activity" {
		s.Positions = maps.Clone(m.positions)
		s.Positions[m.viewKey()] = search.cursor
	}
	for id, e := range m.drafts {
		s.Drafts[id] = e.text
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if bytes.Equal(data, m.lastSession) {
		m.dirty = false
		return nil
	}
	if err := m.store.WriteSession(data); err != nil {
		return err
	}
	m.lastSession = data
	m.dirty = false
	return nil
}
func (m *model) persist() { m.dirty = true }
func (m *model) loadHistory() {
	m.changes = nil
	if m.current() != nil && m.store != nil {
		changes, err := m.store.History(m.active)
		if err != nil {
			m.status = err.Error()
			return
		}
		for i := len(changes) - 1; i >= 0; i-- {
			m.changes = append(m.changes, changes[i])
		}
	}
}
func (m *model) saveWorkspace(w workspace.Workspace) bool {
	saved, err := m.store.Save(w, "user")
	if err != nil {
		// Sync can fail after a revision is published. Reload it before retrying.
		if current, loadErr := m.store.List(); loadErr == nil {
			m.workspaces = current
			if saved.ID != "" {
				m.active = saved.ID
				m.form = nil
				m.page = "settings"
				m.cursor = 0
				if m.currentConnection() == nil {
					m.connection = ""
					m.tool = ""
				}
			}
			m.loadHistory()
			m.loadChat()
		}
		m.status = "Save error: " + err.Error()
		return false
	}
	m.workspaces, err = m.store.List()
	if err != nil {
		m.status = err.Error()
		return false
	}
	if m.settingsOverlay != nil && m.active != saved.ID {
		m.settingsOverlay.page, m.settingsOverlay.detail, m.settingsOverlay.cursor = "", "", 0
	}
	m.active = saved.ID
	m.expanded[saved.ID] = true
	if m.connection != "" && m.currentConnection() == nil {
		m.connection = ""
		m.tool = ""
	}
	m.loadHistory()
	m.loadChat()
	m.status = "Workspace saved"
	m.persist()
	return true
}
func (m *model) viewKey() string {
	return m.active + "/" + m.tool + "/" + m.connection + "/" + modes[m.mode]
}
func (m *model) selectScope(ws, tool, connection string) {
	if m.explore != nil && m.explore.action != nil && (ws != m.active || connection != m.connection || tool != m.tool) {
		m.status = "Finish or cancel the open resource action first."
		return
	}
	if ws != m.active || connection != m.connection {
		m.stopExplore()
		if m.explore != nil {
			m.explore.key = ""
			m.explore.requested = ""
			m.explore.pendingCommand = ""
		}
	}
	m.settingsOverlay = nil
	m.toolSetup = nil
	m.active, m.tool, m.connection = ws, tool, connection
	m.page = ""
	m.form = nil
	m.command = nil
	m.detail = ""
	m.status = ""
	m.loadHistory()
	m.loadChat()
}

func (m *model) Update(msg tea.Msg) (next tea.Model, command tea.Cmd) {
	// Animation ticks only redraw; they must not start requests or change navigation.
	if _, ok := msg.(loadingTickMsg); ok {
		m.loadingTickPending = false
		if m.loadingVisible() {
			m.loadingFrame = (m.loadingFrame + 1) % len(loadingFrames)
			return m, m.syncLoading()
		}
		return m, nil
	}
	treeKey := ""
	switch msg.(type) {
	case exploreConnected, exploreUpdate, exploreDetailResult, exploreLogEvent, namespaceResult:
		rows := m.treeRows()
		if m.treeCursor >= 0 && m.treeCursor < len(rows) {
			treeKey = rows[m.treeCursor].key
		}
	}
	defer func() {
		m.syncViewExpansion()
		m.resizeResourceTerminal()
		m.clearStaleSearches()
		command = tea.Batch(command, m.syncExplore(), m.syncResourceAction())
		command = tea.Batch(command, m.syncLoading())
		m.syncExploreTree()
		if treeKey != "" {
			for i, row := range m.treeRows() {
				if row.key == treeKey {
					m.treeCursor = i
					break
				}
			}
		}
	}()
	switch msg := msg.(type) {
	case resourceProgressResult:
		m.updateResourceProgress(msg)
		return m, nil
	case resourceActionResult:
		m.updateResourceActionResult(msg)
		return m, nil
	case resourceTerminalTick:
		if m.explore != nil && m.explore.terminal == msg.terminal && !msg.terminal.done {
			return m, terminalTick(msg.terminal)
		}
		return m, nil
	case resourceTerminalResult:
		if m.explore != nil && m.explore.terminal == msg.terminal {
			var client *kubernetes.Client
			if msg.terminal.launch != nil {
				client = msg.terminal.launch.client
			}
			msg.terminal.done, msg.terminal.captured, msg.terminal.err = true, false, client.ExecErrorText(msg.err)
			if msg.terminal.review == nil {
				msg.terminal.beginReview()
			}
		}
		return m, nil
	case exploreConnected, exploreUpdate, exploreDetailResult, exploreLogEvent, namespaceResult:
		return m, m.updateExploreMessage(msg)
	case chatEvent:
		return m, m.updateChatEvent(msg)
	case autosaveMsg:
		if m.dirty {
			if err := m.saveSession(); err != nil {
				m.status = "Could not save session: " + err.Error()
			}
		}
		return m, autosave()
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clearDocumentPrefix()
		m.resizeResourceTerminal()
		m.shortcutPrefix = ""
		return m, nil
	case tea.BlurMsg:
		m.clearDocumentPrefix()
		m.shortcutPrefix = ""
		return m, nil
	case tea.PasteMsg:
		m.shortcutPrefix = ""
		if m.workspacePaste(msg.Content) {
			return m, nil
		}
		if m.resourceTerminalVisible() && m.explore.terminal.captured {
			m.explore.terminal.emulator.Paste(msg.Content)
			return m, nil
		}
		if m.resourceActionVisible() {
			a := m.explore.action
			if !a.busy && (slices.Contains([]string{"scale", "export"}, a.stage) || a.stage == "execcommand" && a.execEditing) {
				a.input.insert(msg.Content, a.stage == "edit")
			}
			return m, nil
		}
		if m.tooSmall() || m.help != nil || (m.command != nil && m.focus == 1) {
			return m, nil
		}
		// Handle paste before key dispatch so pasted commands stay literal.
		if search := m.searches[m.focus]; search != nil {
			if !search.finished {
				search.input.insert(msg.Content, false)
				m.searchChanged()
			}
		} else if m.focus == 2 && m.chatEditing && !m.chat().history && !m.chat().models {
			m.draft().insert(msg.Content, true)
			m.draft().literal = true
			m.persist()
		} else if m.focus == 1 && m.form != nil {
			before := m.form.fields[0].value.text
			m.form.paste(msg.Content)
			if m.form.kind == "kube" && m.form.fields[0].value.text != before {
				m.invalidateContexts()
			}
		} else if m.exploreEditing() && m.explore.input != nil {
			m.explore.input.insert(msg.Content, false)
		}
		return m, nil
	case tea.KeyPressMsg:
		key := msg.String()
		// Handle exit before editors and the small-terminal fallback.
		if key == "ctrl+c" {
			if m.execute("quit") {
				return m, tea.Quit
			}
			return m, nil
		}
		if (key == "tab" || key == "shift+tab") && (m.help != nil || (m.settingsOverlay != nil && m.focus == 1)) {
			m.shortcutPrefix = ""
			return m, nil
		}
		if key == "esc" && m.restoreExpandedOnEscape() {
			return m, nil
		}
		if handled, cmd := m.workspaceKey(msg); handled {
			return m, cmd
		}
		if m.tooSmall() {
			m.shortcutPrefix = ""
			return m, m.updateSmall(msg)
		}
		if m.resourceActionVisible() && m.command == nil && !m.documentActive() && !m.valueDocumentActive() {
			a := m.explore.action
			if !a.busy && (!slices.Contains([]string{"edit", "scale", "export", "execcommand"}, a.stage) || a.stage == "execcommand" && !a.execEditing) {
				if m.updateShortcut(msg) {
					m.persist()
					return m, nil
				}
				if key == ":" {
					m.beginCommand()
					return m, nil
				}
			}
			m.shortcutPrefix = ""
			return m, m.updateResourceActionKey(msg)
		}
		if key == "tab" || key == "shift+tab" {
			if m.exploreEditing() && m.explore.picker != "" {
				m.shortcutPrefix = ""
				return m, nil
			}
			m.cycleFocus(key == "shift+tab")
			m.status = ""
			m.persist()
			return m, nil
		}
		if m.command != nil && m.focus == 1 {
			m.shortcutPrefix = ""
			switch key {
			case "esc":
				m.command = nil
				m.focus = m.commandFocus
			case "enter":
				cmd := strings.TrimSpace(m.command.text)
				m.command = nil
				m.focus = m.commandFocus
				quit := m.execute(cmd)
				m.persist()
				if quit {
					return m, tea.Quit
				}
			default:
				m.command.key(msg, false)
			}
			return m, nil
		}
		if m.help == nil {
			if s := m.searches[m.focus]; s != nil && (!s.finished || key == "esc" || key == "n" || key == "N") && !(m.focus == 1 && m.settingsOverlay != nil) {
				m.shortcutPrefix = ""
				m.updatePanelSearch(msg)
				m.persist()
				return m, nil
			}
			if m.focus == 1 && m.form != nil && m.form.index < len(m.form.fields) {
				m.shortcutPrefix = ""
				m.updateForm(msg)
				m.persist()
				return m, nil
			}
			if m.focus == 2 && m.chatEditing && !m.chat().history && !m.chat().models {
				m.shortcutPrefix = ""
				cmd := m.updateChat(msg)
				m.persist()
				return m, cmd
			}
			if m.exploreEditing() {
				m.shortcutPrefix = ""
				m.updateExploreInput(msg)
				return m, nil
			}
		}
		if m.updateShortcut(msg) {
			m.persist()
			return m, nil
		}
		if key == ":" {
			m.beginCommand()
			return m, nil
		}
		if key == "/" && m.beginPanelSearch() {
			return m, nil
		}
		if m.help != nil {
			m.updateHelp(key)
		} else if m.focus == 0 {
			m.updateTree(key)
		} else if m.focus == 2 {
			cmd := m.updateChat(msg)
			m.persist()
			return m, cmd
		} else if m.form != nil {
			m.updateForm(msg)
		} else {
			m.updateMain(key)
		}
		m.persist()
	}
	return m, nil
}

func (m *model) beginCommand() {
	// Tab resumes a command draft; a fresh colon starts a new command.
	e := newEditor("")
	m.command = &e
	m.commandFocus = m.focus
	m.focus = 1
	m.status = ""
}

func (m *model) execute(command string) bool {
	m.shortcutPrefix = ""
	if m.editorLocalCommand(command) {
		return false
	}
	command = canonicalCommand(command)
	if m.viewCommand(command) {
		return false
	}
	if m.explore != nil && m.explore.action != nil && !slices.Contains([]string{"quit", "help", "chat", "newchat", "openchat", "stopchat", "restoreprompt", "sidebar", "aiinteraction"}, command) {
		m.status = "Finish or cancel the open resource action first."
		return false
	}
	if m.form != nil && (command == "new" || command == "settings" || command == "1" || command == "2" || command == "3") {
		m.status = "Finish or cancel the open form in the main panel first."
		return false
	}
	switch command {
	case "1", "2", "3":
		m.closeHelp()
		m.closeSettings()
		m.focus = 1
		m.mode = int(command[0] - '1')
		m.page = ""
		m.detail = ""
		m.status = ""
	case "review":
		if m.form != nil || m.settingsOverlay != nil || !m.openActivityReview() {
			m.status = "Open an Activity entry to view its changes."
		} else {
			m.closeHelp()
			m.focus = 1
			m.status = ""
		}
	case "new":
		m.closeHelp()
		m.beginWorkspace()
	case "settings":
		m.closeHelp()
		m.openSettings()
	case "help":
		if m.help == nil {
			m.help = &helpState{previousFocus: m.focus, section: m.focus}
		}
		m.status = ""
	case "sidebar", "aiinteraction":
		m.restoreView()
		hidden, name := &m.leftHidden, "Sidebar"
		if command == "aiinteraction" {
			hidden, name = &m.rightHidden, "AI panel"
		}
		*hidden = !*hidden
		m.ensureVisibleFocus()
		m.status = name + " shown."
		if *hidden {
			m.status = name + " hidden."
		}
	case "chat", "newchat", "openchat":
		m.chatCommand(command)
	case "restoreprompt":
		m.restoreChatPrompt()
	case "stopchat":
		if m.run == nil {
			m.status = "No AI response is running."
		} else {
			m.stopChat()
			m.status = "AI response stopped. Received text is saved."
			if err := m.flushChats(); err != nil {
				m.status = "Could not save response: " + err.Error()
			}
		}
	case "quit":
		m.stopChat()
		if err := m.saveSession(); err != nil {
			m.status = "Cannot save before exit: " + err.Error()
			return false
		}
		m.exploreClosed = true
		m.stopExplore()
		return true
	default:
		if !m.executeExplore(command) {
			m.status = "Unknown command. Use :h for available commands."
		}
	}
	return false
}

func (m *model) tooSmall() bool {
	return m.width > 0 && m.height > 0 && (m.width < minWidth || m.height < minHeight)
}

// Keep invisible form actions disabled while retaining a visible way to exit.
func (m *model) updateSmall(k tea.KeyPressMsg) tea.Cmd {
	key := k.String()
	if key == "tab" || key == "shift+tab" {
		m.cycleFocus(key == "shift+tab")
		return nil
	}
	if m.command != nil && m.focus != 1 {
		return nil
	}
	if key == "esc" {
		if m.command != nil {
			m.focus = m.commandFocus
		}
		m.command = nil
		return nil
	}
	if m.command == nil {
		if key == ":" {
			m.beginCommand()
		}
		return nil
	}
	if key == "enter" {
		cmd := canonicalCommand(strings.TrimSpace(m.command.text))
		m.command = nil
		m.focus = m.commandFocus
		if cmd == "quit" && m.execute(cmd) {
			return tea.Quit
		}
		return nil
	}
	m.command.key(k, false)
	return nil
}
