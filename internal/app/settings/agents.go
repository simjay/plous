package settings

import (
	"github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/ai/cli"
	ai "github.com/simjay/plous/internal/ai/model"
	textview "github.com/simjay/plous/internal/ui/text"
	"strings"
)

func AgentForm(selected agent.Kind) *Form {
	f := &Form{Kind: "mainagent", Title: "Agent", Fields: []Field{{Key: "agent", Label: "Selected agent", Required: true, Value: textview.NewField(selected.Effective().Label()), Options: []string{agent.Plous.Label(), agent.ClaudeCode.Label(), agent.Codex.Label()}}}}
	f.RefreshAgent(nil)
	return f
}

func AgentNote(kind agent.Kind) string {
	if kind.Effective() == agent.Plous {
		return "Provider connections are used only by Plous. Choose models with /model in Plous chat."
	}
	return AgentAvailability(kind) + "\nSign-in and models are managed in " + kind.Label() + "."
}

func (f *Form) RefreshAgent(configs []ai.Config) {
	id := ""
	if i := f.Index - len(f.Fields); i >= 0 && i < len(f.Rows) {
		id = f.Rows[i].ID
	}
	kind := AgentKind(f.Fields[0].Value.Text)
	f.Note = AgentNote(kind)
	f.Rows = nil
	if kind == agent.Plous {
		for _, config := range configs {
			row := Row{Kind: ProviderRow, ID: "provider/" + config.Name, Label: config.Name, Section: "Provider connections"}
			if config.SupportedProvider() != nil {
				row.Label += " · unavailable"
			}
			vendor := ProviderLabel(config.Provider)
			if !strings.EqualFold(strings.TrimSpace(config.Name), vendor) {
				row.Detail = vendor
			}
			f.Rows = append(f.Rows, row)
		}
		f.Rows = append(f.Rows, Row{Kind: AddProviderRow, ID: "add-provider", Label: "Add provider connection", Section: "Provider connections"})
	}
	f.Rows = append(f.Rows, Row{Kind: SaveAgentRow, ID: "save-agent", Label: "Save"}, Row{Kind: CancelAgentRow, ID: "cancel-agent", Label: "Cancel"})
	if f.Index >= len(f.Fields) {
		f.Index = len(f.Fields)
		for i, row := range f.Rows {
			if row.ID == id {
				f.Index = len(f.Fields) + i
				break
			}
		}
	}
}

func AgentKind(label string) agent.Kind {
	for _, kind := range []agent.Kind{agent.Plous, agent.ClaudeCode, agent.Codex} {
		if label == kind.Label() {
			return kind
		}
	}
	return agent.Kind(label)
}
func AgentAvailability(kind agent.Kind) string {
	if !kind.Valid() {
		return kind.Label() + " is not supported."
	}
	if kind.Effective() == agent.Plous {
		return "Built in"
	}
	if _, err := cli.Executable(kind); err != nil {
		return err.Error()
	}
	return "Installed"
}
