package app

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	backend "github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/ai/history"
	"github.com/simjay/plous/internal/app/activity"
	appchat "github.com/simjay/plous/internal/app/chat"
	appcode "github.com/simjay/plous/internal/app/code"
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
	Code          map[string]appcode.CodePreference    `json:"code,omitempty"`
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
	Kustomize     map[string]appcode.LegacyPreference  `json:"kustomize,omitempty"`
}

type autosaveMsg struct{}

type model struct {
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
	currentDirectory                                                        *workspace.Workspace
	currentDirectoryError                                                   string
	currentInventory                                                        kubecommon.ContextInventory
	currentRequest, currentLoading                                          *currentRequest

	activityUI                   activity.Controller
	codeUI                       appcode.Controller
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

	kustomizePreferences map[string]appcode.LegacyPreference
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
		m.kustomizePreferences = s.Kustomize
		m.codeUI.Preferences = s.Code
		m.active, m.connection, m.tool = s.Workspace, s.Connection, s.Tool
		if m.tool == "kustomize" {
			m.tool = "manifests"
		}
		if m.tool == "tfcode" {
			m.tool = "manifests"
		}
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
	m.captureCurrentDirectory()
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
	return tea.Batch(waitUI(m.uiRequests, m.uiDone), autosave(), currentTick(), m.syncCurrent(), m.syncExplore(), m.syncOverview(), m.syncClusters(), m.syncGitOps(), m.syncHelm(), m.syncCode(), m.syncLoading(), m.syncChatRestore(), m.syncActivity())
}

func autosave() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return autosaveMsg{} })
}

func (m *model) current() *workspace.Workspace {
	if m.currentDirectory != nil && m.currentDirectory.ID == m.active {
		return m.currentDirectory
	}
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
	if (ws != m.active || connection != m.connection) && !m.canSwitchCurrent() {
		return
	}
	if tool == "kustomize" {
		tool = "manifests"
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
	if changedConnection && connection != "" {
		m.queueTreeSources(treeRow{kind: "codeview", workspace: ws, connection: connection})
	}
	if m.activityUI.Workspace != m.active {
		m.loadHistory()
	}
	m.loadChat()
}

func (m *model) Update(msg tea.Msg) (next tea.Model, command tea.Cmd) {
	m.nativeScroll.deferView = false
	if frame, ok := msg.(*nativeScrollFrame); ok {
		if m.nativeScroll.pending == frame && m.nativeAgent == frame.session {
			m.nativeScroll.pending = nil
		} else {
			m.nativeScroll.deferView = true
		}
		return m, nil
	}
	if tick, ok := msg.(nativeAgentTick); ok && m.nativeAgent == tick.session && !tick.session.Finished && tick.session.Reading() {
		m.nativeScroll.deferView = true
		return m, nativeTick(tick.session)
	}
	switch msg.(type) {
	case tea.KeyPressMsg, tea.PasteMsg, tea.MouseMsg, tea.BlurMsg, tea.WindowSizeMsg:
		m.cancelClipboardPaste()
		_, mouse := msg.(tea.MouseMsg)
		if m.focus != 2 || mouse {
			m.uiInputVersion++
		}
	}
	if wheel, ok := msg.(tea.MouseWheelMsg); ok && m.nativeScrollWheel(wheel) {
		return m, m.updateMouse(wheel)
	}
	m.nativeScroll.pending = nil
	m.nativeScroll.onlyNative = false
	// Animation ticks only redraw; they must not start requests or change navigation.
	if _, ok := msg.(loadingTickMsg); ok {
		m.loadingTickPending = false
		if m.loadingVisible() {
			m.loadingFrame = (m.loadingFrame + 1) % len(loadingFrames)
			return m, m.syncLoading()
		}
		return m, nil
	}
	var treePath []string
	switch msg.(type) {
	case clusterDiscoveryResult, helmDetectionResult, monitor.ExploreConnected, monitor.ExploreUpdate, monitor.ExploreDetailResult, monitor.ExploreLogEvent, monitor.ExploreUsageResult, monitor.NamespaceResult, appcode.CodeResult:
		rows := m.treeRows()
		if m.treeCursor >= 0 && m.treeCursor < len(rows) {
			parents := make(map[string]string, len(rows))
			for _, row := range rows {
				parents[row.key] = row.parent
			}
			for key := rows[m.treeCursor].key; key != ""; key = parents[key] {
				treePath = append(treePath, key)
			}
		}
	}
	previousFocus := m.focus
	defer func() {
		if previousFocus != 2 && m.focus == 2 {
			m.agentReturnFocus = previousFocus
		}
		command = tea.Batch(command, m.syncNativeAgent(), m.syncCurrent(), m.syncQuit(), m.syncPersistence(), m.syncDirectoryPicker())
		m.syncViewExpansion()
		m.resizeResourceTerminal()
		m.resizeNativeAgent()
		m.clearStaleSearches()
		command = tea.Batch(command, m.syncExplore(), m.syncResourceAction(), m.syncOverview(), m.syncClusters(), m.syncGitOps(), m.syncHelm(), m.syncCode())
		if m.explore != nil && m.explore.Usage != nil {
			m.explore.Usage.ReconcileCharts(time.Now())
		}
		command = tea.Batch(command, m.syncLoading())
		command = tea.Batch(command, m.syncChatEvidence(), m.syncChatFind(), m.syncChatRestore(), m.syncActivity())
		m.syncExploreTree()
		if len(treePath) > 0 {
			rows := m.treeRows()
			m.treeCursor = max(0, min(m.treeCursor, len(rows)-1))
			for _, key := range treePath {
				found := false
				for i, row := range rows {
					if row.key == key {
						m.treeCursor, found = i, true
						break
					}
				}
				if found {
					break
				}
			}
		}
	}()
	switch msg := msg.(type) {
	case *uiRequest:
		return m, tea.Batch(waitUI(m.uiRequests, m.uiDone), m.receiveUI(msg))
	case uiLoaded:
		m.applyUI(msg)
		return m, nil
	case overviewResult:
		m.applyOverview(msg)
		return m, nil
	case overviewUsageResult:
		m.applyOverviewUsage(msg)
		return m, nil
	case overviewUsagePoll:
		if msg.View.UsageRun == msg.Run && msg.View.UsagePoll {
			msg.View.UsagePoll = false
			if m.overviewVisible() && m.overviewCurrent() == msg.View && msg.View.UsageCancel == nil && msg.View.Cluster.Client != nil {
				return m, m.syncOverviewUsage(msg.View)
			}
		}
		return m, nil
	case overviewInspectResult:
		m.applyOverviewInspect(msg)
		return m, nil
	case overviewPoll:
		if msg.View.Run == msg.Run && msg.View.Poll {
			msg.View.Poll = false
			if m.overviewVisible() && m.overviewCurrent() == msg.View && msg.View.Cancel == nil && msg.View.Cluster.Client != nil {
				return m, m.readOverview(msg.View)
			}
		}
		return m, nil
	case helmDetectionResult:
		m.applyHelmDetection(msg)
		return m, nil
	case clusterDiscoveryResult:
		m.applyClusterDiscovery(msg)
		return m, nil
	case gitOpsResult:
		m.applyGitOpsResult(msg)
		return m, nil
	case gitOpsResourceResult:
		m.applyGitOpsResource(msg)
		return m, nil
	case gitOpsNamespaceResult:
		m.applyGitOpsNamespaces(msg)
		return m, nil
	case gitOpsPoll:
		if msg.View.Poll == msg.Run && msg.View.PollPending {
			msg.View.PollPending = false
			if m.gitOpsVisible() && m.gitOpsCurrent() == msg.View && msg.View.Cancel == nil && msg.View.Namespaces == nil && !msg.View.State.FilterInput() {
				if msg.View.State.Detail != nil {
					msg.View.State.Pending = &monitor.GitOpsAction{Kind: "detail", Ref: msg.View.State.Detail.Ref}
				} else {
					msg.View.State.Loaded = false
				}
			}
		}
		return m, nil
	case appsettings.DirectoryResult:
		m.applyDirectoryResult(msg)
		return m, nil
	case activity.ActivityResult:
		m.applyActivity(msg)
		return m, nil
	case tea.MouseMsg:
		return m, m.updateMouse(msg)
	case appchat.ChatRestoreResult:
		m.updateChatRestore(msg)
		return m, nil
	case chatFindResult:
		m.updateChatFind(msg)
		return m, nil
	case appchat.ChatEvidenceResult:
		msg.Evidence.Records, msg.Evidence.Err, msg.Evidence.Loaded = msg.Records, msg.Err, true
		if s := m.chatSources; s != nil && s.Evidence == msg.Evidence {
			s.List = len(msg.Records) > 1
		}
		return m, nil
	case appchat.ChatSourceDocumentResult:
		return m, m.updateChatSourceDocument(msg)
	case monitor.HelmNamespaceResult:
		m.updateHelmNamespaceResult(msg)
		return m, nil
	case appcode.CodeResult:
		return m, m.updateCodeResult(msg)

	case monitor.HelmResult:
		m.updateHelmResult(msg)
		return m, nil
	case monitor.ResourceProgressResult:
		m.updateResourceProgress(msg)
		return m, nil
	case monitor.ResourceActionResult:
		m.updateResourceActionResult(msg)
		return m, nil
	case monitor.ResourceTerminalTick:
		if m.explore != nil && m.explore.Terminal == msg.Terminal && !msg.Terminal.Done {
			return m, monitor.TerminalTick(msg.Terminal)
		}
		return m, nil
	case monitor.ResourceTerminalResult:
		if m.explore != nil && m.explore.Terminal == msg.Terminal {
			var client *kubernetes.Client
			if msg.Terminal.Launch != nil {
				client = msg.Terminal.Launch.Client
			}
			msg.Terminal.Done, msg.Terminal.Captured, msg.Terminal.Err = true, false, client.ExecErrorText(msg.Err)
			if msg.Terminal.Review == nil {
				msg.Terminal.BeginReview()
			}
		}
		return m, nil
	case monitor.ExploreConnected, monitor.ExploreUpdate, monitor.ExploreDetailResult, monitor.ExploreLogEvent, monitor.ExploreUsageResult, monitor.NamespaceResult:
		return m, m.updateExploreMessage(msg)
	case persistenceResult:
		return m, m.applyPersistence(msg)
	case appchat.ChatEvent:
		return m, m.updateChatEvent(msg)
	case currentResult:
		m.applyCurrent(msg)
		return m, nil
	case currentPoll:
		if m.currentLoading == nil && m.currentRequest == nil {
			m.requestCurrent("", false)
		}
		return m, currentTick()
	case autosaveMsg:
		if m.dirty {
			if err := m.queueSession(); err != nil {
				m.status = "Could not save session: " + err.Error()
			}
		}
		return m, autosave()
	case nativeAgentTick:
		if m.nativeAgent == msg.session && !msg.session.Finished {
			return m, nativeTick(msg.session)
		}
		return m, nil
	case nativeAgentResult:
		msg.session.SaveErr, msg.session.Save = msg.saveErr, msg.save
		if msg.saveErr != nil && m.quitting {
			m.persistenceErr = msg.saveErr
		}
		msg.session.Finished = true
		msg.session.Captured = false
		if msg.err != nil {
			msg.session.Err = msg.err.Error()
		}
		if m.nativeAgent == msg.session {
			m.status = msg.session.Kind.Label() + " session ended. cc Resume · :C New session"
			if msg.err != nil {
				m.status = msg.err.Error()
			}
			if msg.saveErr != nil {
				m.status = "Evidence save failed. :X retries the save."
			}
		}
		m.finishNativeAgentChange(msg.session)
		return m, nil
	case tea.WindowSizeMsg:
		m.mouse.reset()
		m.mouse.spans = nil
		m.width, m.height = msg.Width, msg.Height
		m.ensureVisibleFocus()
		m.clearDocumentPrefix()
		m.resizeResourceTerminal()
		m.shortcutPrefix = ""
		return m, nil
	case tea.BlurMsg:
		m.mouse.reset()
		m.clearDocumentPrefix()
		m.shortcutPrefix = ""
		return m, nil
	case clipboardCopyResult:
		if msg.native {
			m.status = "Copied to clipboard."
			return m, nil
		}
		m.status = "Sent to the terminal clipboard. Clipboard support depends on your terminal."
		return m, tea.SetClipboard(msg.fallback)
	case clipboardPasteResult:
		m.finishClipboardPaste(msg)
		return m, nil
	case tea.PasteMsg:
		m.pasteText(msg.Content)
		return m, nil
	case tea.KeyPressMsg:
		m.mouse.reset()
		key := msg.String()
		// Native input owns Ctrl+C; other input owners use it to exit.
		if key == "ctrl+c" && !m.nativeAgentCaptured() {
			if m.execute("quit") {
				return m, tea.Quit
			}
			return m, nil
		}
		// Mac Option-arrows can arrive as Meta-arrows or Escape+b/f.
		// Escape+b/f is indistinguishable from Alt+B/F.
		switch key {
		case "alt+left", "meta+left", "alt+b":
			m.travelPanel(true)
			return m, nil
		case "alt+right", "meta+right", "alt+f":
			m.travelPanel(false)
			return m, nil
		}
		// Captured Exec retains its existing output-review escape.
		if key == "ctrl+]" && !m.mainTerminalCaptured() {
			if !msg.IsRepeat {
				m.toggleAgentFocus()
			}
			return m, nil
		}
		if m.nativeAgentCaptured() {
			return m, m.nativeAgentKey(msg)
		}
		if handled, cmd := m.clipboardKey(msg); handled {
			return m, cmd
		}
		if key == "esc" && m.clearInputSelection() {
			return m, nil
		}
		if (key == "tab" || key == "shift+tab") && (m.helpVisible() || m.chatSourcesVisible() || (m.settingsOverlay != nil && m.focus == 1)) {
			m.shortcutPrefix = ""
			return m, nil
		}
		if m.chatSourcesVisible() && !m.helpVisible() && !m.commandFocused() && !m.tooSmall() {
			return m, m.chatSourcesKey(msg)
		}
		if key == "esc" && m.restoreExpandedOnEscape() {
			return m, nil
		}
		if handled, cmd := m.featureKey(msg); handled {
			return m, cmd
		}
		if m.tooSmall() {
			m.shortcutPrefix = ""
			return m, m.updateSmall(msg)
		}
		if m.resourceActionVisible() && !m.commandFocused() && !m.documentActive() && !m.valueDocumentActive() {
			a := m.explore.Action
			if !a.Busy && (!slices.Contains([]string{"edit", "scale", "export", "execcommand"}, a.Stage) || a.Stage == "execcommand" && !a.ExecEditing) {
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
		if m.commandFocused() {
			m.shortcutPrefix = ""
			if key == "esc" || key == "enter" {
				m.unparkOverlays()
			}
			switch key {
			case "esc":
				m.command = nil
				m.focus = m.commandFocus
			case "enter":
				cmd := strings.TrimSpace(m.command.Text)
				m.command = nil
				m.focus = m.commandFocus
				quit := m.execute(cmd)
				m.persist()
				if quit {
					return m, tea.Quit
				}
			default:
				m.command.Key(msg, false)
			}
			return m, nil
		}
		if !m.helpVisible() {
			if s := m.activePanelSearch(m.focus); s.ownsKey(key) {
				m.shortcutPrefix = ""
				m.updatePanelSearch(msg)
				m.persist()
				return m, nil
			}
			if m.focus == 1 && m.form != nil && (m.form.Index < len(m.form.Fields) || m.form.Directory != nil && m.form.Directory.Editing) {
				m.shortcutPrefix = ""
				m.updateForm(msg)
				m.persist()
				return m, nil
			}
			if m.focus == 2 && m.chatEditing && !m.chat().History && !m.chat().Models {
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
		if key == "tab" && m.focus != 2 && !(m.focus == 1 && m.form != nil) {
			if !msg.IsRepeat {
				m.toggleTreeFocus()
			}
			return m, nil
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
		if m.helpVisible() {
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

func (m *model) tooSmall() bool {
	if (m.overviewActive() || m.overviewBack != nil) && m.width >= 40 && m.height >= 12 {
		return false
	}
	return m.width > 0 && m.height > 0 && (m.width < minWidth || m.height < minHeight)
}

// Keep invisible form actions disabled while retaining a visible way to exit.
func (m *model) updateSmall(k tea.KeyPressMsg) tea.Cmd {
	key := k.String()
	if m.command != nil && m.focus != 1 {
		return nil
	}
	if key == "esc" {
		if m.command != nil {
			m.unparkOverlays()
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
		cmd := canonicalCommand(strings.TrimSpace(m.command.Text))
		m.unparkOverlays()
		m.command = nil
		m.focus = m.commandFocus
		if cmd == "quit" && m.execute(cmd) {
			return tea.Quit
		}
		return nil
	}
	m.command.Key(k, false)
	return nil
}
