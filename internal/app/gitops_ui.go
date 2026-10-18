package app

import (
	"context"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
)

type gitOpsNamespaceResult struct {
	View  *gitOpsView
	Run   *monitor.NamespaceRequest
	Names []string
	Err   error
}

func (m *model) gitOpsBody(width, height int) string {
	v := m.gitOpsCurrent()
	if v == nil {
		return ""
	}
	body := v.State.Body(width, height)
	if !v.Cluster.Loaded {
		body = ui.Heading(v.State.Title()) + "\n\n" + m.loading("Discovering cluster APIs…")
	}
	if v.Namespaces != nil {
		pw, ph := ui.PickerSize(width, height)
		return ui.PickerOverlay(body, m.namespacePickerView(v.Namespaces, v.NamespaceInput, v.State.Namespace, v.NamespaceCursor, pw-4, ph-2), width, height)
	}
	return body
}

func (m *model) gitOpsHint() string {
	v := m.gitOpsCurrent()
	if v.Namespaces != nil {
		return "↑↓ Select · Enter Apply · Esc Back"
	}
	return strings.Join(v.State.Hints(), " · ")
}

func (m *model) gitOpsInputKey(msg tea.KeyPressMsg) bool {
	if !m.gitOpsVisible() || m.focus != 1 || m.helpVisible() || m.settingsOverlay != nil || m.commandFocused() || m.tooSmall() {
		return false
	}
	v := m.gitOpsCurrent()
	if v.Namespaces == nil && !v.State.FilterInput() {
		return false
	}
	m.shortcutPrefix = ""
	if v.Namespaces != nil {
		m.gitOpsNamespaceKey(v, msg)
	} else {
		v.State.Key(msg)
	}
	return true
}

func (m *model) updateGitOps(key string) {
	v := m.gitOpsCurrent()
	if v == nil {
		return
	}
	if v.Namespaces != nil {
		m.gitOpsNamespaceKey(v, localMouseKey(key))
		return
	}
	if key == "esc" && v.State.Loading && v.Cancel != nil {
		v.Cancel()
		v.Cancel = nil
		v.Run++
		v.State.Loading = false
		v.State.Loaded = true
		v.State.Err = "Read cancelled. g refreshes this view."
		return
	}
	hadDetail := v.State.Detail != nil
	handled := v.State.Key(localMouseKey(key))
	if key == "esc" && (hadDetail && v.State.Detail == nil || !handled) {
		if !m.returnToOverview() && !handled {
			m.focusRootSidebar()
		}
	}
	m.rememberGitOps(v)
}

func (m *model) executeGitOps(command string) bool {
	if !m.gitOpsVisible() || m.settingsOverlay != nil {
		return false
	}
	name, argument, _ := strings.Cut(command, " ")
	switch name {
	case "refresh":
		m.updateGitOps("g")
		return true
	case "namespace":
		argument = strings.TrimSpace(argument)
		if argument == "" {
			m.updateGitOps("n")
			return true
		}
		if !monitor.ValidExploreNamespace(argument) {
			m.status = "Use a namespace name or * for all namespaces."
			return true
		}
		v := m.gitOpsCurrent()
		if v.Cancel != nil && v.State.Namespace != monitor.ExploreNamespace(argument) {
			v.Cancel()
			v.Cancel = nil
			v.Run++
		}
		m.closeGitOpsNamespaces(v)
		v.State.SetNamespace(monitor.ExploreNamespace(argument))
		m.rememberGitOps(v)
		return true
	}
	return false
}

func (m *model) gitOpsPaste(text string) bool {
	if !m.gitOpsVisible() || m.focus != 1 || m.helpVisible() || m.settingsOverlay != nil || m.commandFocused() || m.tooSmall() {
		return false
	}
	v := m.gitOpsCurrent()
	if v.Namespaces != nil {
		v.NamespaceInput.Insert(text, false)
		v.NamespaceCursor = 0
		return true
	}
	if v.State.FilterInput() {
		v.State.Paste(text)
		return true
	}
	return false
}

func (m *model) openGitOpsNamespaces(v *gitOpsView) {
	m.closeGitOpsNamespaces(v)
	v.Namespaces = &monitor.NamespacePicker{}
	input := textview.NewField("")
	v.NamespaceInput = &input
	v.NamespaceCursor = 0
}

func (m *model) closeGitOpsNamespaces(v *gitOpsView) {
	if v.Namespaces != nil && v.Namespaces.Run != nil {
		v.Namespaces.Run.Cancel()
	}
	v.Namespaces, v.NamespaceInput = nil, nil
}

func (m *model) gitOpsNamespaceKey(v *gitOpsView, msg tea.KeyPressMsg) {
	value, apply, close, status := monitor.UpdateNamespacePicker(v.Namespaces, &v.NamespaceInput, &v.NamespaceCursor, v.State.Namespace, msg)
	m.status = status
	if apply {
		if v.Cancel != nil && value != v.State.Namespace {
			v.Cancel()
			v.Cancel = nil
			v.Run++
		}
		v.State.SetNamespace(value)
		m.closeGitOpsNamespaces(v)
		m.rememberGitOps(v)
	} else if close {
		m.closeGitOpsNamespaces(v)
	}
}

func (m *model) syncGitOpsNamespaces(v *gitOpsView) tea.Cmd {
	p := v.Namespaces
	if p == nil || p.Loaded || p.Run != nil || !v.Cluster.Loaded {
		return nil
	}
	if v.Cluster.Client == nil {
		p.Loaded = true
		p.Err = "Connection unavailable. Enter a namespace manually or refresh."
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	run := &monitor.NamespaceRequest{Cancel: cancel}
	p.Run = run
	client := v.Cluster.Client
	return func() tea.Msg {
		defer cancel()
		r := gitOpsNamespaceResult{View: v, Run: run}
		collection, err := client.List(ctx, kubecommon.Resource{Name: "namespaces", Kind: "Namespace", Version: "v1"}, "", "")
		r.Err = err
		for _, o := range collection.Objects {
			r.Names = append(r.Names, o.GetName())
		}
		return r
	}
}

func (m *model) applyGitOpsNamespaces(r gitOpsNamespaceResult) {
	if r.View == nil || r.View.Namespaces == nil || r.View.Namespaces.Run != r.Run {
		return
	}
	p := r.View.Namespaces
	p.Run, p.Loaded, p.Err = nil, true, kubernetes.ErrorText(r.Err)
	p.Names = r.Names
	sort.Strings(p.Names)
}
