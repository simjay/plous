package settings

import (
	ai "github.com/simjay/plous/internal/ai/model"
	"github.com/simjay/plous/internal/ui"
)

type SettingsOverlay struct {
	Page, Detail  string
	Cursor, Focus int
}

func ProviderLabel(provider string) string {
	switch provider {
	case "openai":
		return "OpenAI"
	case "anthropic":
		return "Anthropic"
	case "gemini":
		return "Google Gemini"
	case "xai":
		return "xAI (Grok)"
	default:
		return "OpenAI-compatible"
	}
}

func ApplyAIFields(a ai.Config, values map[string]string) ai.Config {
	for key, value := range values {
		switch key {
		case "name":
			a.Name = value
		case "provider":
			a.Provider = value
		case "endpoint":
			a.Endpoint = value
		case "key_env":
			a.KeyEnv = value
		case "instructions":
			a.Instructions = value
		}
	}
	return a
}

func WorkspaceLocation(width int) []string {
	label := ui.Muted.Background(ui.Surface)
	return []string{
		ui.Clipped(label.Render("Location  ")+ui.ShortcutStyle.Background(ui.Surface).Render("● Local")+label.Render("   ○ Remote (coming soon)"), width),
		label.Render(ui.Clipped("Remote workspaces and sharing need plous-server.", width)),
	}
}
