package app

import (
	"context"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/kubernetes"
)

type namespacePicker struct {
	run        *namespaceRequest
	names      []string
	loaded     bool
	err, query string
	manual     bool
}

type namespaceRequest struct{ cancel context.CancelFunc }

type namespaceResult struct {
	run        *namespaceRequest
	generation uint64
	names      []string
	err        error
}

type namespaceChoice struct {
	name, label string
	manual      bool
	reload      bool
}

func (m *model) openNamespacePicker() {
	m.stopNamespacePicker()
	e := m.explorer()
	m.openExploreInput("namespace", "")
	e.picker = "namespace"
	e.namespaces = &namespacePicker{}
	if e.preference.Namespace != "" {
		e.pickerCursor = 1
	}
	m.status = ""
}

func (m *model) stopNamespacePicker() {
	if e := m.explore; e != nil && e.namespaces != nil {
		if e.namespaces.run != nil {
			e.namespaces.run.cancel()
		}
		e.namespaces = nil
	}
}

func (m *model) syncNamespacePicker() tea.Cmd {
	e := m.explorer()
	if e.picker != "namespace" {
		m.stopNamespacePicker()
		return nil
	}
	if e.namespaces == nil {
		e.namespaces = &namespacePicker{}
	}
	p := e.namespaces
	if p.loaded || p.run != nil || e.connecting {
		return nil
	}
	if e.client == nil {
		p.loaded = true
		p.err = e.err
		if p.err == "" {
			p.err = "Connect to Kubernetes to load available namespaces."
		}
		return nil
	}
	resource := kubernetes.Resource{Name: "namespaces", Kind: "Namespace", Version: "v1"}
	for _, candidate := range e.resources {
		if candidate.Key() == "namespaces" {
			resource = candidate
			break
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &namespaceRequest{cancel: cancel}
	p.run = run
	client, generation := e.client, e.generation
	return func() tea.Msg {
		defer cancel()
		collection, err := client.List(ctx, resource, "", "")
		result := namespaceResult{run: run, generation: generation, err: err}
		for _, object := range collection.Objects {
			result.names = append(result.names, object.GetName())
		}
		return result
	}
}

func (m *model) updateNamespaceResult(result namespaceResult) {
	e := m.explorer()
	if e.picker != "namespace" || e.namespaces == nil || result.run != e.namespaces.run || result.generation != e.generation {
		return
	}
	p := e.namespaces
	p.run = nil
	p.loaded = true
	p.err = kubernetes.ErrorText(result.err)
	p.names = result.names
	sort.Strings(p.names)
	e.pickerCursor = min(e.pickerCursor, max(0, len(m.namespaceChoices())-1))
}

func (m *model) namespaceChoices() []namespaceChoice {
	e := m.explorer()
	query := ""
	if e.input != nil {
		query = strings.ToLower(strings.TrimSpace(e.input.text))
	}
	var choices []namespaceChoice
	seen := map[string]bool{}
	add := func(name, label string) {
		if seen[name] {
			return
		}
		seen[name] = true
		if query == "" || strings.Contains(strings.ToLower(label), query) || (name == "" && query == "*") {
			choices = append(choices, namespaceChoice{name: name, label: label})
		}
	}
	add("", "All namespaces")
	if e.preference.Namespace != "" {
		add(e.preference.Namespace, e.preference.Namespace)
	}
	if e.namespaces != nil {
		for _, name := range e.namespaces.names {
			if name != "" {
				add(name, name)
			}
		}
	}
	if query == "" {
		choices = append(choices, namespaceChoice{label: "Reload namespaces", reload: true})
	}
	return append(choices, namespaceChoice{label: "Enter namespace manually…", manual: true})
}

func (m *model) chooseExploreNamespace(namespace string) {
	e := m.explorer()
	m.clearExploreDetail()
	m.stopNamespacePicker()
	e.preference.Namespace = namespace
	e.defaultNamespace = false
	e.picker, e.inputKind = "", ""
	e.input = nil
	e.selected = ""
	m.status = ""
	m.rememberExplore()
}

func (m *model) updateNamespaceInput(msg tea.KeyPressMsg) {
	e := m.explorer()
	p := e.namespaces
	if p == nil || e.input == nil {
		return
	}
	key := msg.String()
	if key == "esc" {
		if p.manual {
			p.manual = false
			input := newEditor(p.query)
			e.input = &input
			e.pickerCursor = 0
		} else {
			m.stopNamespacePicker()
			e.picker, e.inputKind = "", ""
			e.input = nil
		}
		m.status = ""
		return
	}
	if p.manual {
		if key == "enter" {
			value := strings.TrimSpace(e.input.text)
			if !validExploreNamespace(value) {
				m.status = "Use a namespace name or * for all namespaces."
				return
			}
			m.chooseExploreNamespace(exploreNamespace(value))
		} else {
			e.input.key(msg, false)
		}
		return
	}
	choices := m.namespaceChoices()
	switch key {
	case "up", "down", "pgup", "pgdown", "home", "end":
		step := 1
		if key == "pgup" || key == "pgdown" {
			step = 10
		}
		if key == "up" || key == "pgup" {
			step = -step
		}
		e.pickerCursor = max(0, min(len(choices)-1, e.pickerCursor+step))
		if key == "home" {
			e.pickerCursor = 0
		} else if key == "end" {
			e.pickerCursor = len(choices) - 1
		}
	case "enter":
		choice := choices[max(0, min(len(choices)-1, e.pickerCursor))]
		if choice.reload {
			if p.run != nil {
				p.run.cancel()
				p.run = nil
			}
			p.loaded, p.err = false, ""
		} else if choice.manual {
			p.manual, p.query = true, e.input.text
			e.inputKind = "namespace"
			input := newEditor(strings.TrimSpace(p.query))
			e.input = &input
		} else {
			m.chooseExploreNamespace(choice.name)
		}
	default:
		e.input.key(msg, false)
		e.pickerCursor = 0
	}
}

func (m *model) namespacePickerBody(width, height int) string {
	e := m.explorer()
	p := e.namespaces
	if p == nil || e.input == nil {
		return ""
	}
	if p.manual {
		lines := []string{gradientHeading("Namespace"), "", "Enter a namespace name", selected.Render(editorLine(*e.input, width, true)), "", muted.Render("Use * for all namespaces"), "", muted.Render("Enter apply · Esc back")}
		if m.status != "" {
			lines = append(lines, changedStyle.Render(clipped(m.status, width)))
		}
		return styledView(strings.Join(lines, "\n"), width, height)
	}
	lines := []string{gradientHeading("Namespace"), selected.Render("/ " + editorLine(*e.input, max(1, width-2), true))}
	if !p.loaded {
		lines = append(lines, m.loading("Loading namespaces…"))
	} else if p.err != "" {
		lines = append(lines, changedStyle.Render(clipped(p.err, width)), muted.Render("Choose the current scope or enter a name manually."))
	} else if len(p.names) == 0 {
		lines = append(lines, muted.Render("No namespaces returned by the cluster."))
	}
	choices := m.namespaceChoices()
	if len(choices) == 1 && e.input.text != "" {
		lines = append(lines, muted.Render("No matching namespaces."))
	}
	items := make([]string, len(choices))
	for i, choice := range choices {
		items[i] = choice.label
		if !choice.manual && !choice.reload && choice.name == e.preference.Namespace {
			items[i] += "  ✓"
		}
	}
	lines = append(lines, "")
	available := max(1, height-len(lines)-2)
	lines = append(lines, listView(items, e.pickerCursor, width, available), "", muted.Render("↑↓ Choose · Enter Select · Esc Back"))
	return styledView(strings.Join(lines, "\n"), width, height)
}

func (m *model) exploreCategories() []string {
	available := map[string]bool{}
	for _, resource := range m.explorer().resources {
		available[kubernetes.Category(resource)] = true
	}
	var categories []string
	for _, category := range kubernetes.Categories {
		if available[category] {
			categories = append(categories, category)
		}
	}
	return categories
}

func (m *model) exploreCategory() string {
	categories := m.exploreCategories()
	if len(categories) == 0 {
		return "No resource categories"
	}
	e := m.explorer()
	e.group = max(0, min(len(categories)-1, e.group))
	return categories[e.group]
}
