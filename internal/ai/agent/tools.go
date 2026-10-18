// Package agent defines the transport-independent boundary to Plous tools.
package agent

import (
	"context"
	"encoding/json"
)

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ToolCall struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

// Tools is bound to one captured turn. Definitions returns an independent copy.
// Call persists resource evidence or UI execution receipts before returning JSON. Correctable arguments
// produce JSON feedback; a non-nil error stops the turn. maxBytes can narrow,
// never increase, the host's delivery limit. Callers cannot choose another scope.
type Tools interface {
	Definitions() []Tool
	Call(context.Context, ToolCall, int) (json.RawMessage, error)
}
