package settings

import (
	"github.com/simjay/plous/internal/ai/agent"
	ai "github.com/simjay/plous/internal/ai/model"
	textview "github.com/simjay/plous/internal/ui/text"
)

type Field struct {
	Key, Label string
	Required   bool
	Value      textview.Field
	Options    []string
	Secret     bool
}

type Form struct {
	Kind, Title, Tool, Connection string
	Note                          string
	Profile                       string
	Fields                        []Field
	Index                         int
	Remove                        bool
	Advanced                      bool
	Agent                         ai.Config
	ApiKey                        string
	Parent                        *Form
	Rows                          []Row
	EntryCommands                 agent.Commands
	commandAgent                  agent.Kind
}

func (f *Form) Paste(s string) {
	if f.Index < len(f.Fields) && len(f.Fields[f.Index].Options) == 0 {
		f.Fields[f.Index].Value.Insert(s, false)
		if f.Kind == "mainagent" {
			f.RefreshAgentNote()
		}
	}
}

func (f *Form) Actions() []string {
	if f.Kind == "mainagent" {
		actions := make([]string, len(f.Rows))
		for i, row := range f.Rows {
			actions[i] = row.Label
		}
		return actions
	}
	if f.Kind == "contextname" {
		return []string{"Use name", "Cancel"}
	}
	if f.Kind == "helmchart" {
		return []string{"Preview", "Edit Values", "Cancel"}
	}
	actions := []string{"Save", "Cancel"}

	if f.Kind == "appsettings" {
		label := "Advanced…"
		if f.Advanced {
			label = "Hide advanced"
		}
		actions = append(actions, label)
	}
	if f.Remove {
		label := "Remove connection"
		if f.Kind == "appsettings" {
			label = "Remove provider connection"
		}
		actions = append(actions, label)
	}
	return actions
}
