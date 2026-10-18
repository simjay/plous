package chat

import (
	ai "github.com/simjay/plous/internal/ai/model"
)

func ChatModelName(config ai.Config) string {
	if model, ok := ai.LookupModel(config.Provider, config.Model); ok {
		return model.Name
	}
	return config.Model
}
