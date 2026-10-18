package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/app/monitor"
	"github.com/simjay/plous/internal/app/navigation"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
	diffview "github.com/simjay/plous/internal/ui/diff"
	docedit "github.com/simjay/plous/internal/ui/editor"
)

type uiPort struct {
	done           chan struct{}
	requests       chan *uiRequest
	scope, profile string
	connections    []ai.Connection
}
type uiRequest struct {
	port     *uiPort
	ctx      context.Context
	show     *agent.ViewRequest
	reply    chan agent.UIResult
	version  uint64
	location string
}
type uiLoaded struct {
	request  *uiRequest
	prepared *navigation.Prepared
	err      error
}
type uiDocument struct {
	model  *docedit.Model
	view   agent.ViewRequest
	scope  string
	origin string
	mode   int
}

func (m *model) uiTools(connections []ai.Connection) agent.UI {
	if m.uiRequests == nil {
		m.uiRequests = make(chan *uiRequest)
	}
	if m.uiDone == nil {
		m.uiDone = make(chan struct{})
	}
	profile, _ := m.store.DataRoot()
	return &uiPort{m.uiDone, m.uiRequests, m.conversationScope(), profile, slices.Clone(connections)}
}
func (p *uiPort) Read(ctx context.Context) (agent.UIResult, error) { return p.call(ctx, nil) }
func (p *uiPort) Show(ctx context.Context, q agent.ViewRequest) (agent.UIResult, error) {
	return p.call(ctx, &q)
}
func (p *uiPort) call(ctx context.Context, show *agent.ViewRequest) (agent.UIResult, error) {
	r := &uiRequest{port: p, ctx: ctx, show: show, reply: make(chan agent.UIResult, 1)}
	select {
	case p.requests <- r:
	case <-p.done:
		return agent.UIResult{}, context.Canceled
	case <-ctx.Done():
		return agent.UIResult{}, ctx.Err()
	}
	select {
	case result := <-r.reply:
		return result, nil
	case <-p.done:
		return agent.UIResult{}, context.Canceled
	case <-ctx.Done():
		return agent.UIResult{}, ctx.Err()
	}
}
func waitUI(requests chan *uiRequest, done <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		select {
		case request := <-requests:
			return request
		case <-done:
			return nil
		}
	}
}

func (m *model) uiLocation() string {
	return fmt.Sprintf("%s/%s/%s/%d/%s/%s", m.active, m.connection, m.tool, m.mode, m.page, m.historyView)
}
func (m *model) uiBlock(scope string) string {
	if m.quitting {
		return "Plous is closing"
	}
	if m.conversationScope() != scope {
		return "The user is viewing another connection; return to the captured connection first"
	}
	if m.width < minWidth || m.height < minHeight {
		return "Resize the terminal to show the main panel"
	}
	if m.form != nil || m.settingsOverlay != nil || m.toolSetup != nil || m.command != nil || m.helpVisible() || m.chatSourcesVisible() {
		return "Close the open form, command or overlay before opening another view"
	}
	if m.explore != nil && (m.explore.Action != nil || m.explore.Terminal != nil || m.explore.Input != nil || m.explore.Picker != "") {
		return "Finish the resource action, terminal or filter input first"
	}
	for _, s := range m.codeUI.States {
		if s.Document != nil && !s.Document.ReadOnly || s.OtherEditorView() != "" || s.Input != nil {
			return "Save or close the source editor or input first"
		}
	}
	for _, h := range m.helmStates {
		if h.Document != nil && !h.Document.ReadOnly || h.Confirm || h.Draft != nil && h.WorkflowVisible() || h.Busy && h.Running == "execute" || h.Input != nil {
			return "Finish the Helm editor, input or operation first"
		}
	}
	return ""
}

func (p *uiPort) matches(m *model) bool {
	if m.conversationScope() != p.scope {
		return false
	}
	current := m.assistantConnections()
	for _, captured := range p.connections {
		index := slices.IndexFunc(current, func(c ai.Connection) bool { return c.ConnectionID == captured.ConnectionID && c.Tool == captured.Tool })
		if index < 0 {
			return false
		}
		c := current[index]
		if c.Path != captured.Path || c.Context != captured.Context || c.Driver != captured.Driver || c.TargetCaptured != captured.TargetCaptured {
			return false
		}
		if (c.Target == nil) != (captured.Target == nil) || c.Target != nil && c.Target.Fingerprint != captured.Target.Fingerprint {
			return false
		}
	}
	return true
}

func (m *model) uiState(p *uiPort) agent.UIResult {
	result := agent.UIResult{Status: "ready", Focus: []string{"sidebar", "main", "assistant"}[m.focus], Visible: m.panelVisible(1) && !m.tooSmall(), Available: []string{"evidence"}}
	if !p.matches(m) {
		return agent.UIResult{Status: "blocked", Reason: "The user is viewing another connection"}
	}
	for _, c := range p.connections {
		switch c.Tool {
		case "kubernetes":
			result.Available = append(result.Available, "resource", "list", "logs")
		case "helm":
			result.Available = append(result.Available, "compare")
		case "source":
			result.Available = append(result.Available, "source")
		}
	}
	page := m.assistantPageContext()
	q := agent.ViewRequest{View: strings.ToLower(modes[m.mode]), ConnectionID: page.ConnectionID, Resource: page.Resource, Namespace: page.Namespace, Name: page.Name, UID: page.UID, Filter: page.Filter, Labels: page.Labels}
	if m.presentedDocumentVisible() {
		q = m.uiDocument.view
		if q.View == "source" {
			d := m.uiDocument.model
			q.Line = 1 + strings.Count(string([]rune(d.Text())[:min(d.Cursor, len([]rune(d.Text())))]), "\n")
		}
		result.Origin = m.uiDocument.origin
	} else if m.exploreVisible() {
		q.View = "list"
		for _, object := range m.explore.Collection.Objects {
			if m.explore.Detail == nil && kubernetes.Identity(object) == m.explore.Selected {
				result.Selected = &agent.ViewRequest{View: "resource", ConnectionID: q.ConnectionID, Resource: q.Resource, Name: object.GetName(), Namespace: object.GetNamespace(), UID: string(object.GetUID())}
				break
			}
		}
		if m.explore.Detail != nil {
			q.View, q.Tab = "resource", strings.ToLower(monitor.ExploreTabs[m.explore.Tab])
		}
		if m.explore.Tab == 4 && m.explore.Detail != nil {
			q.View, q.Container, q.Previous, q.Highlight = "logs", m.explore.Container, m.explore.Previous, m.podLogViewer().Reader.Query
		}
	} else if m.helmVisible() && m.helmer().Comparison != nil && m.helmer().Comparison == m.uiLastComparison && m.uiLastView != nil {
		q = *m.uiLastView
		result.Origin = "recorded"
	} else if m.codeVisible() && m.coder().Document != nil {
		s := m.coder()
		q.View, q.Path = "source", s.DocumentPath
		q.Line = 1 + strings.Count(string([]rune(s.Document.Text())[:min(s.Document.Cursor, len([]rune(s.Document.Text())))]), "\n")
	}
	q.Evidence = nil
	result.View = &q
	result.Reason = m.uiBlock(p.scope)
	return result
}

func (m *model) receiveUI(r *uiRequest) tea.Cmd {
	if r.ctx.Err() != nil {
		return nil
	}
	if r.show == nil {
		r.reply <- m.uiState(r.port)
		return nil
	}
	if !r.port.matches(m) {
		r.reply <- agent.UIResult{Status: "blocked", Reason: "The captured connection is no longer selected or its configuration changed"}
		return nil
	}
	if reason := m.uiBlock(r.port.scope); reason != "" {
		r.reply <- agent.UIResult{Status: "blocked", Reason: reason}
		return nil
	}
	if m.uiPending != nil {
		r.reply <- agent.UIResult{Status: "blocked", Reason: "Another view is loading"}
		return nil
	}
	r.version, r.location = m.uiInputVersion, m.uiLocation()
	m.uiPending = r
	return func() tea.Msg {
		prepared, err := navigation.Load(r.ctx, *r.show, r.port.connections, r.port.profile)
		return uiLoaded{r, prepared, err}
	}
}

func (m *model) applyUI(r uiLoaded) {
	if m.uiPending != r.request {
		return
	}
	m.uiPending = nil
	request := r.request
	if request.ctx.Err() != nil {
		return
	}
	result := agent.UIResult{Status: "blocked"}
	if !request.port.matches(m) {
		result.Reason = "The captured connection is no longer selected or its configuration changed"
	} else if reason := m.uiBlock(request.port.scope); reason != "" {
		result.Reason = reason
	} else if request.version != m.uiInputVersion || request.location != m.uiLocation() {
		result.Reason = "The user changed the main view while the request was loading"
	} else if r.err != nil {
		result.Status, result.Reason = "failed", ui.Plain(r.err.Error())
	} else {
		m.presentUI(r.prepared)
		result = m.uiState(request.port)
		result.Status, result.Origin = "opened", r.prepared.Origin
		q := r.prepared.Request
		q.Evidence = nil
		result.View = &q
	}
	request.reply <- result
}

func (m *model) presentUI(p *navigation.Prepared) {
	previousMode := m.mode
	if m.presentedDocumentVisible() {
		previousMode = m.uiDocument.mode
	}
	q := p.Request
	m.page, m.detail = "", ""
	m.overviewBack, m.gitOpsReturn = nil, nil
	m.restoreView()
	m.shortcutPrefix = ""
	m.mode = 1
	m.uiLastView = &q
	m.uiLastComparison = nil
	switch q.View {
	case "resource", "list", "logs":
		m.tool = "kubernetes"
		m.stopExplore()
		m.exploreGeneration++
		ns := q.Namespace
		if ns == "*" {
			ns = ""
		}
		e := &monitor.ExploreState{Temporary: true, Key: clusterKey(m.active, *m.currentConnection()), Generation: m.exploreGeneration, Client: p.Client, Resources: p.Catalog, Resource: p.Resource, Preference: monitor.ExplorePreference{Resource: p.Resource.Key(), Namespace: ns, Filter: q.Filter, Selector: q.Labels}, Collection: p.Collection, Listed: q.View == "list", Preloaded: q.View == "list", Detail: p.Object, DetailValid: p.Object != nil, DetailLoaded: p.Object != nil, PreserveTree: true}
		m.explore = e
		if p.Object != nil {
			e.Selected, e.Container = kubernetes.Identity(*p.Object), monitor.PreferredPodContainer(*p.Object)
			e.Tab = max(0, slices.Index([]string{"describe", "yaml", "events", "related", "logs"}, q.Tab))
			if q.View == "logs" {
				e.Tab, e.Container, e.Previous = 4, q.Container, q.Previous
			}
			e.DetailText, e.References = p.DetailText, p.References
			e.DetailKey = p.Resource.Key() + "/" + p.Resource.Version + "/" + e.Selected + "/" + fmt.Sprint(e.Tab)
			if e.Tab < 4 {
				e.DetailSections[e.Tab] = monitor.DetailSection{Loaded: true, Text: p.DetailText, References: p.References}
			}
			if q.View == "logs" {
				e.LogKey = e.DetailKey + "/" + e.Container + fmt.Sprint(e.Previous, e.LogSince)
				e.LogConnected = true
				m.appendExploreLogs(p.Text)
				v := m.podLogViewer()
				v.Reader.Query = q.Highlight
				v.Message = "Captured latest 200 lines. g reconnects."
				if q.Highlight != "" {
					v.Following = false
					v.Refresh()
					v.Key("n", "n")
				}
			}
		}
	case "source", "evidence":
		// Both use the shared read-only document, so closing restores the prior feature.
		name, text := q.Path, p.Text
		if q.View == "evidence" {
			name = "Saved evidence"
			var formatted bytes.Buffer
			if json.Indent(&formatted, []byte(text), "", "  ") == nil {
				text = formatted.String()
			}
		}
		d := docedit.New(ui.Plain(text))
		d.ReadOnly, d.Name, d.Target, d.Syntax = true, name, p.Origin, "text"
		if q.View == "source" && (strings.HasSuffix(q.Path, ".yaml") || strings.HasSuffix(q.Path, ".yml")) {
			d.Syntax = "yaml"
		}
		for _, line := range strings.Split(d.Text(), "\n")[:max(0, q.Line-1)] {
			d.Cursor += utf8.RuneCountInString(line) + 1
		}
		q.Evidence = nil
		m.uiDocument = &uiDocument{d, q, m.conversationScope(), p.Origin, previousMode}
		m.page = "assistant-view"
	case "compare":
		m.tool = "helm"
		h := m.helmer()
		m.chooseHelmSection("Releases")
		h.Client, h.Detail, h.Loaded = p.Helm, p.Snapshot, true
		h.Comparison = diffview.New(p.Before, p.After, q.Namespace+"/"+q.Name, uint64(q.Revision), false)
		m.uiLastComparison = h.Comparison
		h.Comparison.Name = fmt.Sprintf("%s: revision %d → %d", q.Tab, q.CompareRevision, q.Revision)
	}
	m.status = "Assistant opened " + q.View + ". Alt+Left/Right moves between panels."
	m.persist()
}

func (m *model) presentedDocumentVisible() bool {
	return m.page == "assistant-view" && m.uiDocument != nil && m.uiDocument.scope == m.conversationScope()
}
func (m *model) presentedDocumentKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if !m.presentedDocumentVisible() || m.focus != 1 {
		return false, nil
	}
	d, key := m.uiDocument.model, msg.String()
	if !d.Searching {
		if key == ":" || key == "tab" || key == "shift+tab" {
			return false, nil
		}
		if m.updateShortcut(msg) {
			return true, nil
		}
	}
	close := key == "esc" && !d.Searching && d.Mode == docedit.Normal && d.Query == "" && d.Prefix() == ""
	event := d.Key(key, msg.Text)
	if close {
		m.closePresentedDocument()
	}
	return true, m.copyWorkspaceText(event.Copy)
}

func (m *model) closePresentedDocument() {
	if m.uiDocument != nil {
		m.mode = m.uiDocument.mode
	}
	m.page, m.uiDocument = "", nil
}
