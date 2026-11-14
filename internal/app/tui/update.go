package tui

import (
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/activity"
	appchat "github.com/simjay/plous/internal/app/chat"
	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

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
	if tick, ok := msg.(logDragTick); ok {
		return m, m.updateLogDrag(tick)
	}
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
	case clusterDiscoveryResult, helmDetectionResult, monitor.ExploreConnected, monitor.ExploreUpdate, monitor.ExploreDetailResult, monitor.ExploreLogEvent, monitor.ExploreUsageResult, monitor.NamespaceResult:
		treePath = m.treeCursorPath()
	}
	previousFocus := m.focus
	defer func() {
		if previousFocus != 2 && m.focus == 2 {
			m.agentReturnFocus = previousFocus
		}
		command = tea.Batch(command, m.syncNativeAgent(), m.syncCurrent(), m.syncQuit(), m.syncPersistence())
		m.syncViewExpansion()
		m.resizeResourceTerminal()
		m.resizeNativeAgent()
		m.clearStaleSearches()
		command = tea.Batch(command, m.syncExplore(), m.syncResourceAction(), m.syncOverview(), m.syncClusters(), m.syncGitOps(), m.syncHelm())
		if m.explore != nil && m.explore.Usage != nil {
			m.explore.Usage.ReconcileCharts(time.Now())
		}
		command = tea.Batch(command, m.syncLoading())
		command = tea.Batch(command, m.syncChatEvidence(), m.syncChatFind(), m.syncChatRestore(), m.syncActivity())
		m.syncExploreTree()
		if len(treePath) > 0 {
			m.restoreTreeCursor(treePath...)
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
		if handled, cmd := m.screenSelectionKey(msg); handled {
			return m, cmd
		}
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
		if (key == "tab" || key == "shift+tab") && (m.helpVisible() || m.chatSourcesVisible() || m.settingsOverlay != nil && m.focus == 1) {
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
			if m.focus == 1 && m.form != nil && m.form.Index < len(m.form.Fields) {
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
