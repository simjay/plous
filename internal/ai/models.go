package ai

import "errors"

type Model struct {
	ID, Name      string
	ContextWindow int
	Default       bool
}

// This is the sole allowlist for new selections and requests. Additions require
// a Plous code change; provider discovery must never expand it at runtime.
var modelCatalog = map[string][]Model{
	"openai": {
		{ID: "gpt-5.4-mini", Name: "GPT-5.4 mini", ContextWindow: 400_000, Default: true},
		{ID: "gpt-5.4", Name: "GPT-5.4", ContextWindow: 1_050_000},
	},
	"anthropic": {
		{ID: "claude-haiku-4-5-20251001", Name: "Claude Haiku 4.5", ContextWindow: 200_000},
		{ID: "claude-sonnet-5-5", Name: "Claude Sonnet 5.5", ContextWindow: 1_000_000, Default: true},
		{ID: "claude-opus-5-5", Name: "Claude Opus 5.5", ContextWindow: 1_000_000},
	},
}

func Providers() []string { return []string{"openai", "anthropic"} }

// Models returns a copy so UI state cannot modify the request allowlist.
func Models(provider string) []Model {
	return append([]Model(nil), modelCatalog[provider]...)
}

func LookupModel(provider, id string) (Model, bool) {
	for _, model := range modelCatalog[provider] {
		if model.ID == id {
			return model, true
		}
	}
	return Model{}, false
}

// Supported is separate from Validate so old configuration and request history
// remain readable after a vendor or model is removed from the catalog.
func (c Config) Supported() error {
	if err := c.SupportedProvider(); err != nil {
		return err
	}
	if c.Model == "" {
		return errors.New("connect a vendor in Settings (:s), then choose a chat model with /model")
	}
	if _, ok := LookupModel(c.Provider, c.Model); !ok {
		return errors.New("model unavailable; choose a supported chat model with /model")
	}
	return nil
}

func (c Config) SupportedProvider() error {
	if len(modelCatalog[c.Provider]) == 0 {
		return errors.New("vendor unavailable; choose OpenAI or Anthropic in Settings (:s)")
	}
	return nil
}

// WithDefaultModel is for initial setup or replacing a removed connection.
// Requests use the user's saved selection, captured when the operation starts.
func (c Config) WithDefaultModel() (Config, error) {
	if err := c.SupportedProvider(); err != nil {
		return Config{}, err
	}
	for _, model := range modelCatalog[c.Provider] {
		if model.Default {
			c.Model = model.ID
			return c, nil
		}
	}
	return Config{}, errors.New("no default model configured for this vendor")
}
