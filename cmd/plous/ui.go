package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/kubeconfig"
	"github.com/simjay/plous/internal/workspace"
)

const (
	minWidth  = 80
	minHeight = 16
)

var modes = []string{"Overview", "Resources", "Deployment", "Activity"}
var toolTypes = []string{"kubernetes", "helm", "kustomize", "terraform", "argo", "kargo"}
var toolNames = map[string]string{"kubernetes": "Kubernetes", "helm": "Helm", "kustomize": "Kustomize", "terraform": "Terraform", "argo": "Argo", "kargo": "Kargo"}

type session struct {
	Version    int               `json:"version"`
	Workspace  string            `json:"workspace"`
	Connection string            `json:"connection"`
	Tool       string            `json:"tool"`
	Mode       int               `json:"mode"`
	Focus      int               `json:"focus"`
	TreeKey    string            `json:"tree_key"`
	Expanded   map[string]bool   `json:"expanded"`
	Drafts     map[string]string `json:"drafts"`
	Positions  map[string]int    `json:"positions"`
}

type autosaveMsg struct{}

type model struct {
	dirty                      bool
	lastSession                []byte
	width, height, focus, mode int
	store                      *workspace.Store
	workspaces                 []workspace.Workspace
	active, connection, tool   string
	expanded                   map[string]bool
	treeCursor                 int
	drafts                     map[string]*editor
	positions                  map[string]int
	page                       string
	cursor                     int
	form                       *form
	command                    *editor
	status                     string
	contexts                   []kubeconfig.Context
	chosen                     map[int]bool
	changes                    []workspace.Change
	removeID                   string
	detail                     string
	chatEditing                bool
}

func newModel(store *workspace.Store) (*model, error) {
	m := &model{store: store, focus: 1, expanded: map[string]bool{}, drafts: map[string]*editor{}, positions: map[string]int{}}
	var err error
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
		if s.Version != 1 {
			return nil, fmt.Errorf("unsupported session version %d", s.Version)
		}
		m.active, m.connection, m.tool = s.Workspace, s.Connection, s.Tool
		m.mode = max(0, min(3, s.Mode))
		m.focus = max(0, min(2, s.Focus))
		if s.Expanded != nil {
			m.expanded = s.Expanded
		}
		if s.Positions != nil {
			m.positions = s.Positions
			for key, value := range m.positions {
				m.positions[key] = max(0, min(1000000, value))
			}
		}
		for id, text := range s.Drafts {
			e := newEditor(text)
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
			if row.key == s.TreeKey {
				m.treeCursor = i
				break
			}
		}
	}
	if m.active == "" && len(m.workspaces) > 0 {
		m.active = m.workspaces[0].ID
		m.expanded[m.active] = true
	}
	m.loadHistory()
	return m, nil
}
func (m *model) Init() tea.Cmd { return autosave() }
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
	s := session{Version: 1, Workspace: m.active, Connection: m.connection, Tool: m.tool, Mode: m.mode, Focus: m.focus, Expanded: m.expanded, Drafts: map[string]string{}, Positions: m.positions}
	rows := m.treeRows()
	if m.treeCursor >= 0 && m.treeCursor < len(rows) {
		s.TreeKey = rows[m.treeCursor].key
	}
	for id, e := range m.drafts {
		if id != "" {
			s.Drafts[id] = e.text
		}
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
		// A storage sync can fail after publication. Reload the authoritative
		// revisions so a retry never silently works from an old revision.
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
		}
		m.status = "Save error: " + err.Error()
		return false
	}
	m.workspaces, err = m.store.List()
	if err != nil {
		m.status = err.Error()
		return false
	}
	m.active = saved.ID
	m.expanded[saved.ID] = true
	if m.connection != "" && m.currentConnection() == nil {
		m.connection = ""
		m.tool = ""
	}
	m.loadHistory()
	m.status = "Workspace saved"
	m.persist()
	return true
}
func (m *model) viewKey() string {
	return m.active + "/" + m.tool + "/" + m.connection + "/" + modes[m.mode]
}
func (m *model) selectScope(ws, tool, connection string) {
	m.active, m.tool, m.connection = ws, tool, connection
	m.page = ""
	m.form = nil
	m.command = nil
	m.detail = ""
	m.status = ""
	m.loadHistory()
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case autosaveMsg:
		if m.dirty {
			if err := m.saveSession(); err != nil {
				m.status = "Could not save session: " + err.Error()
			}
		}
		return m, autosave()
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.PasteMsg:
		if m.tooSmall() {
			return m, nil
		}
		// Pasted content is accepted only by a text editor, never interpreted as keys.
		if m.focus == 2 && m.current() != nil {
			m.draft().insert(msg.Content, true)
			m.chatEditing = true
			m.persist()
		} else if m.focus == 1 && m.form != nil {
			m.form.paste(msg.Content)
		}
		return m, nil
	case tea.KeyPressMsg:
		key := msg.String()
		if m.tooSmall() {
			return m, m.updateSmall(msg)
		}
		if key == "tab" || key == "shift+tab" {
			step := 1
			if key == "shift+tab" {
				step = 2
			}
			m.focus = (m.focus + step) % 3
			m.persist()
			return m, nil
		}
		// Ctrl+C cancels the current input; exit is explicit and main-panel scoped.
		if key == "ctrl+c" {
			if m.focus == 1 {
				m.command = nil
				if m.form != nil {
					m.form = nil
					m.page = "settings"
				}
				m.status = "Input canceled. In main navigation, use :quit to exit."
			}
			return m, nil
		}
		if m.focus == 0 {
			m.updateTree(key)
			m.persist()
			return m, nil
		}
		if m.focus == 2 {
			if m.current() == nil {
				return m, nil
			}
			if key == "esc" {
				m.chatEditing = false
			} else if key == "enter" && !m.chatEditing {
				m.chatEditing = true
			} else {
				m.chatEditing = true
				m.draft().key(msg, true)
			}
			m.persist()
			return m, nil
		}
		if m.form != nil {
			m.updateForm(msg)
			m.persist()
			return m, nil
		}
		if m.command != nil {
			switch key {
			case "esc":
				m.command = nil
			case "enter":
				cmd := strings.TrimSpace(m.command.text)
				m.command = nil
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
		if key == ":" {
			e := newEditor("")
			m.command = &e
			m.status = ""
			return m, nil
		}
		m.updateMain(key)
		m.persist()
	}
	return m, nil
}
func (m *model) execute(command string) bool {
	switch command {
	case "1", "2", "3", "4":
		m.mode = int(command[0] - '1')
		m.page = ""
		m.detail = ""
		m.status = ""
	case "new":
		m.beginWorkspace()
	case "settings":
		if m.current() != nil {
			m.page = "settings"
			m.cursor = 0
			m.status = ""
		} else {
			m.status = "Create a workspace first (:new)."
		}
	case "help":
		m.page = "help"
		m.cursor = 0
	case "quit":
		if err := m.saveSession(); err != nil {
			m.status = "Cannot save before exit: " + err.Error()
			return false
		}
		return true
	default:
		if (command == "pod" || command == "pods") && m.tool == "kubernetes" && m.mode == 1 {
			m.status = "Kubernetes Resources commands are not implemented in this round."
		} else {
			m.status = "Unknown command. Use :help for available commands."
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
		step := 1
		if key == "shift+tab" {
			step = 2
		}
		m.focus = (m.focus + step) % 3
		return nil
	}
	if m.focus != 1 {
		return nil
	}
	if key == "esc" || key == "ctrl+c" {
		m.command = nil
		return nil
	}
	if m.command == nil {
		if key == ":" {
			e := newEditor("")
			m.command = &e
		}
		return nil
	}
	if key == "enter" {
		cmd := strings.TrimSpace(m.command.text)
		m.command = nil
		if cmd == "quit" && m.execute(cmd) {
			return tea.Quit
		}
		return nil
	}
	m.command.key(k, false)
	return nil
}
