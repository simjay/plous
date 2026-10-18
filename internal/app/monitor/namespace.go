package monitor

import (
	tea "charm.land/bubbletea/v2"
	"context"
	textview "github.com/simjay/plous/internal/ui/text"
	"strings"
)

type NamespacePicker struct {
	Run        *NamespaceRequest
	Names      []string
	Loaded     bool
	Err, Query string
	Manual     bool
}

type NamespaceRequest struct{ Cancel context.CancelFunc }

type NamespaceResult struct {
	Run        *NamespaceRequest
	Generation uint64
	Names      []string
	Err        error
}

type NamespaceChoice struct {
	Name, Label string
	Manual      bool
	Reload      bool
}

func NamespaceChoicesFor(p *NamespacePicker, input *textview.Field, current string) []NamespaceChoice {
	query := ""
	if input != nil {
		query = strings.ToLower(strings.TrimSpace(input.Text))
	}
	var choices []NamespaceChoice
	seen := map[string]bool{}
	add := func(name, label string) {
		if seen[name] {
			return
		}
		seen[name] = true
		if query == "" || strings.Contains(strings.ToLower(label), query) || (name == "" && query == "*") {
			choices = append(choices, NamespaceChoice{Name: name, Label: label})
		}
	}
	add("", "All namespaces")
	if current != "" {
		add(current, current)
	}
	if p != nil {
		for _, name := range p.Names {
			if name != "" {
				add(name, name)
			}
		}
	}
	if query == "" {
		choices = append(choices, NamespaceChoice{Label: "Reload namespaces", Reload: true})
	}
	return append(choices, NamespaceChoice{Label: "Enter namespace manually…", Manual: true})
}

func UpdateNamespacePicker(p *NamespacePicker, input **textview.Field, cursor *int, current string, msg tea.KeyPressMsg) (value string, apply, close bool, status string) {
	if p == nil || *input == nil {
		return
	}
	key := msg.String()
	if key == "esc" {
		if p.Manual {
			p.Manual = false
			editor := textview.NewField(p.Query)
			*input = &editor
			*cursor = 0
		} else {
			close = true
		}
		return
	}
	if p.Manual {
		if key == "enter" {
			value = strings.TrimSpace((*input).Text)
			if !ValidExploreNamespace(value) {
				status = "Use a namespace name or * for all namespaces."
				return
			}
			return ExploreNamespace(value), true, false, ""
		}
		(*input).Key(msg, false)
		return
	}
	choices := NamespaceChoicesFor(p, *input, current)
	switch key {
	case "up", "down", "pgup", "pgdown", "home", "end":
		step := 1
		if key == "pgup" || key == "pgdown" {
			step = 10
		}
		if key == "up" || key == "pgup" {
			step = -step
		}
		*cursor = max(0, min(len(choices)-1, *cursor+step))
		if key == "home" {
			*cursor = 0
		} else if key == "end" {
			*cursor = len(choices) - 1
		}
	case "enter":
		choice := choices[max(0, min(len(choices)-1, *cursor))]
		if choice.Reload {
			if p.Run != nil {
				p.Run.Cancel()
				p.Run = nil
			}
			p.Loaded, p.Err = false, ""
		} else if choice.Manual {
			p.Manual, p.Query = true, (*input).Text
			editor := textview.NewField(strings.TrimSpace(p.Query))
			*input = &editor
		} else {
			return choice.Name, true, false, ""
		}
	default:
		(*input).Key(msg, false)
		*cursor = 0
	}
	return
}
