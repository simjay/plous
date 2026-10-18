package app

import (
	"context"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
)

func (m *model) openNamespacePicker() {
	m.stopNamespacePicker()
	e := m.explorer()
	m.openExploreInput("namespace", "")
	e.Picker = "namespace"
	e.Namespaces = &monitor.NamespacePicker{}
	if e.Preference.Namespace != "" {
		e.PickerCursor = 1
	}
	m.status = ""
}

func (m *model) stopNamespacePicker() { m.explore.StopNamespacePicker() }

func (m *model) syncNamespacePicker() tea.Cmd {
	e := m.explorer()
	if e.Picker != "namespace" {
		m.stopNamespacePicker()
		return nil
	}
	if e.Namespaces == nil {
		e.Namespaces = &monitor.NamespacePicker{}
	}
	p := e.Namespaces
	if p.Loaded || p.Run != nil || e.Connecting {
		return nil
	}
	if e.Client == nil {
		p.Loaded = true
		p.Err = e.Err
		if p.Err == "" {
			p.Err = "Connect to Kubernetes to load available namespaces."
		}
		return nil
	}
	resource := kubecommon.Resource{Name: "namespaces", Kind: "Namespace", Version: "v1"}
	for _, candidate := range e.Resources {
		if candidate.Key() == "namespaces" {
			resource = candidate
			break
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &monitor.NamespaceRequest{Cancel: cancel}
	p.Run = run
	client, generation := e.Client, e.Generation
	return func() tea.Msg {
		defer cancel()
		collection, err := client.List(ctx, resource, "", "")
		result := monitor.NamespaceResult{Run: run, Generation: generation, Err: err}
		for _, object := range collection.Objects {
			result.Names = append(result.Names, object.GetName())
		}
		return result
	}
}

func (m *model) updateNamespaceResult(result monitor.NamespaceResult) {
	e := m.explorer()
	if e.Picker != "namespace" || e.Namespaces == nil || result.Run != e.Namespaces.Run || result.Generation != e.Generation {
		return
	}
	p := e.Namespaces
	p.Run = nil
	p.Loaded = true
	p.Err = kubernetes.ErrorText(result.Err)
	p.Names = result.Names
	sort.Strings(p.Names)
	e.PickerCursor = min(e.PickerCursor, max(0, len(m.namespaceChoices())-1))
}

func (m *model) namespaceChoices() []monitor.NamespaceChoice {
	e := m.explorer()
	return monitor.NamespaceChoicesFor(e.Namespaces, e.Input, e.Preference.Namespace)
}

func (m *model) chooseExploreNamespace(namespace string) {
	e := m.explorer()
	m.clearExploreDetail()
	m.stopNamespacePicker()
	e.Preference.Namespace = namespace
	e.DefaultNamespace = false
	e.Picker, e.InputKind = "", ""
	e.Input = nil
	e.Selected = ""
	m.status = ""
	m.rememberExplore()
}

func (m *model) updateNamespaceInput(msg tea.KeyPressMsg) {
	e := m.explorer()
	value, apply, close, status := monitor.UpdateNamespacePicker(e.Namespaces, &e.Input, &e.PickerCursor, e.Preference.Namespace, msg)
	m.status = status
	if apply {
		m.chooseExploreNamespace(value)
	} else if close {
		m.stopNamespacePicker()
		e.Picker, e.InputKind = "", ""
		e.Input = nil
	}
}

func (m *model) namespacePickerBody(width, height int) string {
	e := m.explorer()
	return m.namespacePickerView(e.Namespaces, e.Input, e.Preference.Namespace, e.PickerCursor, width, height)
}

func (m *model) namespacePickerView(p *monitor.NamespacePicker, input *textview.Field, current string, cursor, width, height int) string {
	if p == nil || input == nil {
		return ""
	}
	if p.Manual {
		lines := []string{ui.Heading("Namespace"), "", "Enter a namespace name", ui.Selected.Render(ui.EditorLine(*input, width, true)), "", ui.Muted.Render("Use * for all namespaces"), "", ui.Muted.Render("Enter apply · Esc back")}
		if m.status != "" {
			lines = append(lines, ui.ChangedStyle.Render(ui.Clipped(m.status, width)))
		}
		return ui.StyledView(strings.Join(lines, "\n"), width, height)
	}
	lines := []string{ui.Heading("Namespace"), ui.Selected.Render("/ " + ui.EditorLine(*input, max(1, width-2), true))}
	if !p.Loaded {
		lines = append(lines, m.loading("Loading namespaces…"))
	} else if p.Err != "" {
		lines = append(lines, ui.ChangedStyle.Render(ui.Clipped(p.Err, width)), ui.Muted.Render("Choose the current scope or enter a name manually."))
	} else if len(p.Names) == 0 {
		lines = append(lines, ui.Muted.Render("No namespaces returned by the cluster."))
	}
	choices := monitor.NamespaceChoicesFor(p, input, current)
	if len(choices) == 1 && input.Text != "" {
		lines = append(lines, ui.Muted.Render("No matching namespaces."))
	}
	items := make([]string, len(choices))
	for i, choice := range choices {
		items[i] = choice.Label
		if !choice.Manual && !choice.Reload && choice.Name == current {
			items[i] += "  ✓"
		}
	}
	lines = append(lines, "")
	available := max(1, height-len(lines)-2)
	lines = append(lines, ui.ListView(items, cursor, width, available), "", ui.Muted.Render("↑↓ Choose · Enter Select · Esc Back"))
	return ui.StyledView(strings.Join(lines, "\n"), width, height)
}
