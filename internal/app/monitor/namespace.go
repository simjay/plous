package monitor

import (
	"context"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	textview "github.com/simjay/plous/internal/ui/text"
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

func (p *NamespacePicker) Cancel() {
	if p != nil && p.Run != nil {
		p.Run.Cancel()
		p.Run = nil
	}
}

func (p *NamespacePicker) Load(timeout time.Duration, list func(context.Context) (kubernetes.Collection, error)) func() NamespaceResult {
	if p == nil || p.Loaded || p.Run != nil {
		return nil
	}
	var ctx context.Context
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	run := &NamespaceRequest{Cancel: cancel}
	p.Run = run
	return func() NamespaceResult {
		defer cancel()
		collection, err := list(ctx)
		result := NamespaceResult{Run: run, Err: err}
		for _, object := range collection.Objects {
			result.Names = append(result.Names, object.GetName())
		}
		return result
	}
}

func (p *NamespacePicker) Apply(result NamespaceResult) bool {
	if p == nil || result.Run == nil || p.Run != result.Run {
		return false
	}
	p.Cancel()
	p.Loaded, p.Err = true, kubernetes.ErrorText(result.Err)
	p.Names = result.Names
	sort.Strings(p.Names)
	return true
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
			p.Cancel()
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
