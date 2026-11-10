package tui

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	backend "github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/ai/history"
	"github.com/simjay/plous/internal/app/activity"
	appchat "github.com/simjay/plous/internal/app/chat"
	"github.com/simjay/plous/internal/app/monitor"
	appsettings "github.com/simjay/plous/internal/app/settings"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	diffview "github.com/simjay/plous/internal/ui/diff"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
)

const (
	minWidth  = 80
	minHeight = 16
)

var modes = []string{"Overview", "Explore", "Activity"}

type session struct {
	GitOps        map[string]gitOpsPreference          `json:"gitops,omitempty"`
	Version       int                                  `json:"version"`
	Workspace     string                               `json:"workspace"`
	Connection    string                               `json:"connection"`
	Tool          string                               `json:"tool"`
	Mode          int                                  `json:"mode"`
	Focus         int                                  `json:"focus"`
	LeftHidden    bool                                 `json:"left_hidden,omitempty"`
	RightHidden   bool                                 `json:"right_hidden,omitempty"`
	AssistantSize assistantSize                        `json:"assistant_size,omitempty"`
	TreeKey       string                               `json:"tree_key"`
	Expanded      map[string]bool                      `json:"expanded"`
	AutoExpanded  map[string]bool                      `json:"auto_expanded,omitempty"`
	Drafts        map[string]string                    `json:"drafts"`
	Positions     map[string]int                       `json:"positions"`
	ChatIDs       map[string]string                    `json:"chat_ids,omitempty"`
	Explore       map[string]monitor.ExplorePreference `json:"explore,omitempty"`
	Helm          map[string]monitor.HelmPreference    `json:"helm,omitempty"`
}

type autosaveMsg struct{}

type model struct {
	debugPolicy     kubernetes.DebugPolicy
	launchDirectory string

	uiDone                                                                  chan struct{}
	uiStopped                                                               bool
	uiRequests                                                              chan *uiRequest
	uiPending                                                               *uiRequest
	uiInputVersion                                                          uint64
	uiDocument                                                              *uiDocument
	uiLastView                                                              *backend.ViewRequest
	uiLastComparison                                                        *diffview.Model
	currentRoot                                                             bool
	currentPaths, currentError, currentNotice, historyView, historyViewName string
	currentCluster                                                          *workspace.Workspace
	currentInventory                                                        kubecommon.ContextInventory
	currentRequest, currentLoading                                          *currentRequest

	activityUI                   activity.Controller
	mouse                        mouseState
	clipboard                    clipboardAccess
	clipboardPaste               *clipboardPasteRequest
	version                      string
	dirty                        bool
	lastSession                  []byte
	persistenceQueue             []*persistenceJob
	persistenceActive            *persistenceJob
	persistenceErr               error
	quitting                     bool
	width, height, focus, mode   int
	store                        *workspace.Store
	history                      *history.Store
	workspaces                   []workspace.Workspace
	active, connection, tool     string
	expanded                     map[string]bool
	autoExpanded                 map[string]bool
	searches                     [3]*panelSearch
	treeCursor                   int
	drafts                       map[string]*textview.Field
	positions                    map[string]int
	page                         string
	settingsOverlay              *appsettings.SettingsOverlay
	toolSetup                    *appsettings.ToolSetup
	cursor                       int
	form                         *appsettings.Form
	removeForm                   *appsettings.Form
	command                      *textview.Field
	commandFocus                 int
	shortcutPrefix               string
	help                         *helpState
	helpTravel, sourcesTravel    overlayTravel
	leftHidden, rightHidden      bool
	agentReturnFocus             int
	assistantSize                assistantSize
	viewExpansion                *viewExpansion
	status                       string
	loadingFrame                 int
	loadingTickPending           bool
	contexts                     []kubecommon.Context
	chosen                       map[int]bool
	activityReview               *activity.ActivityReview
	activityRecord               *activity.ActivityEntry
	activityListSearch           *panelSearch
	activityObservations         bool
	removeID                     string
	removeProfile                string
	detail                       string
	chatEditing                  bool
	chatSources                  *appchat.ChatSources
	settings                     workspace.Settings
	chats                        map[string]*appchat.ChatState
	chatIDs                      map[string]string
	run                          *appchat.ChatRun
	nativeAgent                  *appchat.Native
	nativeChange                 *nativeAgentChange
	nativeScroll                 nativeScrollState
	nativeRequested, nativeFresh bool
	nativeError                  string
	chatRestore                  appchat.RestoreController

	overviewBack         *overviewReturn
	overviews            map[string]*overviewView
	clusterStates        map[string]*clusterState
	gitOpsViews          map[string]*gitOpsView
	gitOpsPreferences    map[string]gitOpsPreference
	gitOpsReturn         *gitOpsReturn
	helmStates           map[string]*monitor.HelmState
	helmPreferences      map[string]monitor.HelmPreference
	operationDetail      *workspace.Operation
	explore              *monitor.ExploreState
	explorePreferences   map[string]monitor.ExplorePreference
	exploreGeneration    uint64
	exploreClosed        bool
	exploreCatalogs      map[string][]kubecommon.Resource
	exploreTreeSelection string
}

func newModel(store *workspace.Store, archive *history.Store, version string) (*model, error) {
	m := &model{uiRequests: make(chan *uiRequest), uiDone: make(chan struct{}), version: version, store: store, history: archive, mode: 0, focus: 1, agentReturnFocus: 1, expanded: map[string]bool{}, drafts: map[string]*textview.Field{}, positions: map[string]int{}}
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
	treeKey := ""
	if len(data) > 0 {
		var s session
		if err = json.Unmarshal(data, &s); err != nil {
			return nil, fmt.Errorf("read saved session: %w", err)
		}
		if s.Version != 1 && s.Version != 2 {
			return nil, fmt.Errorf("unsupported session version %d", s.Version)
		}
		if s.Version == 1 {
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
		m.helmPreferences = s.Helm
		m.gitOpsPreferences = s.GitOps

		m.active, m.connection, m.tool = s.Workspace, s.Connection, s.Tool
		m.leftHidden, m.rightHidden = s.LeftHidden, s.RightHidden
		if !m.rightHidden && (s.AssistantSize == assistantDouble || s.AssistantSize == "main") {
			// Retired main-area sizes restore with room for both panels.
			m.assistantSize = assistantDouble
		}
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
			e := textview.NewField(text)
			e.Literal = text != ""
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
		treeKey = canonicalTreeKey(s.TreeKey)
	}
	if m.active == "" && len(m.workspaces) > 0 && len(data) == 0 {
		m.active = m.workspaces[0].ID
		m.expanded[m.active] = true
	}

	m.launchDirectory, _ = os.Getwd()
	if m.launchDirectory != "" {
		directory, err := filepath.EvalSymlinks(m.launchDirectory)
		if err != nil {
			m.launchDirectory = ""
		} else {
			m.launchDirectory = directory
		}
	}
	m.expanded[m.currentTreeKey()] = true
	m.restoreCollapsedTree(treeKey)
	// Restore workspace data without reopening the previous connection's page.
	m.active, m.connection, m.tool = "", "", ""
	m.currentRoot = true
	m.currentRequest = &currentRequest{Select: true, Startup: true}
	m.nativeRequested = m.externalAgentSelected()
	m.loadHistory()
	m.loadChat()
	return m, nil
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(waitUI(m.uiRequests, m.uiDone), autosave(), currentTick(), m.syncCurrent(), m.syncExplore(), m.syncOverview(), m.syncClusters(), m.syncGitOps(), m.syncHelm(), m.syncLoading(), m.syncChatRestore(), m.syncActivity())
}

func autosave() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return autosaveMsg{} })
}

func (m *model) current() *workspace.Workspace {
	if m.currentCluster != nil && m.currentCluster.ID == m.active {
		return m.currentCluster
	}
	for i := range m.workspaces {
		if m.workspaces[i].ID == m.active {
			return &m.workspaces[i]
		}
	}
	return nil
}

func (m *model) currentConnection() *workspace.Connection {
	return m.connectionView(m.savedConnection())
}

// Visible context must not capture credentials or initialize connections.
func (m *model) connectionView(c *workspace.Connection) *workspace.Connection {
	if c != nil && monitorCluster(c.Tool) && (m.tool == "kubernetes" || m.tool == "helm" || gitOpsTool(m.tool)) {
		copy := *c
		copy.Tool = m.tool
		if driver := m.helmDriver(m.active, *c); m.tool == "helm" && driver != c.Settings["driver"] {
			copy.Settings = maps.Clone(c.Settings)
			if copy.Settings == nil {
				copy.Settings = map[string]string{}
			}
			copy.Settings["driver"] = driver
		}
		return &copy
	}
	return c
}

func (m *model) selectedConnection() *workspace.Connection {
	if w := m.current(); w != nil {
		for i := range w.Connections {
			if w.Connections[i].ID == m.connection {
				return &w.Connections[i]
			}
		}
	}
	return nil
}

func (m *model) savedConnection() *workspace.Connection {
	c := m.selectedConnection()
	if c != nil && monitorCluster(c.Tool) && !c.TargetCaptured {
		c.TargetCaptured = true
		c.Target, _ = kubecommon.ResolveTarget(clusterSettings(*c)["kubeconfig"], c.Settings["context"], c.Settings["namespace"])
	}
	return c
}

func (m *model) draft() *textview.Field {
	key := m.conversationScope()
	if m.drafts[key] == nil {
		e := textview.NewField("")
		m.drafts[key] = &e
	}
	return m.drafts[key]
}

func (m *model) saveWorkspace(w workspace.Workspace) bool {
	saved, err := m.store.Save(w, "user")
	if err != nil {
		// Sync can fail after a revision is published. Reload it before retrying.
		if current, loadErr := m.store.List(); loadErr == nil {
			m.workspaces = current
			if saved.ID != "" {
				m.active = saved.ID
				m.currentRoot, m.historyView = false, ""
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
		m.settingsOverlay.Page, m.settingsOverlay.Detail, m.settingsOverlay.Cursor = "", "", 0
	}
	m.active = saved.ID
	m.currentRoot, m.historyView = false, ""
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
	if connection != "" {
		for _, w := range m.treeWorkspaces() {
			for _, c := range w.Connections {
				if w.ID == ws && c.ID == connection && !monitorCluster(c.Tool) {
					tool, connection = "", ""
				}
			}
		}
	}
	if (ws != m.active || connection != m.connection) && !m.canSwitchCurrent() {
		return
	}
	if m.explore != nil && m.explore.Action != nil && (ws != m.active || connection != m.connection || tool != m.tool) {
		m.status = "Finish or cancel the open resource action first."
		return
	}
	m.gitOpsReturn = nil
	changedConnection := ws != m.active || connection != m.connection
	if changedConnection {
		m.stopExplore()
		if m.explore != nil {
			m.explore.Key = ""
			m.explore.Requested = ""
			m.explore.PendingCommand = ""
		}
	}
	m.settingsOverlay = nil
	m.toolSetup = nil
	m.active, m.tool, m.connection = ws, tool, connection
	m.currentRoot = workspace.IsCurrentScope(ws)
	m.historyView = ""
	m.page = ""
	m.form = nil
	m.command = nil
	m.detail = ""
	m.status = ""
	if m.activityUI.Workspace != m.active {
		m.loadHistory()
	}
	m.loadChat()
}
