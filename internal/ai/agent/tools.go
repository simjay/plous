// Package agent defines the transport-independent boundary to Plous tools.
package agent

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrRequestCancelled identifies an external request that stopped without
// invalidating the session's tool authority.
var ErrRequestCancelled = errors.New("tool request cancelled")

// OnlyCancellation reports whether every cause matches the cancellation.
// Joined evidence or integrity failures must remain visible.
func OnlyCancellation(err, cancellation error) bool {
	switch e := err.(type) {
	case interface{ Unwrap() []error }:
		causes := e.Unwrap()
		for _, cause := range causes {
			if !OnlyCancellation(cause, cancellation) {
				return false
			}
		}
		return len(causes) > 0
	case interface{ Unwrap() error }:
		return OnlyCancellation(e.Unwrap(), cancellation)
	default:
		return err != nil && err == cancellation
	}
}

type Tool struct {
	Mutating       bool            `json:"-"`
	NonDestructive bool            `json:"-"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Parameters     json.RawMessage `json:"parameters"`
}

type ToolCall struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

// Tools is bound to one captured turn. Definitions returns an independent copy.
// Call persists resource evidence or UI execution receipts before returning JSON. Correctable arguments
// produce JSON feedback. A non-nil error stops the turn unless it is an
// external ErrRequestCancelled. maxBytes can narrow,
// never increase, the host's delivery limit. Callers cannot choose another scope.
type Tools interface {
	Definitions() []Tool
	Call(context.Context, ToolCall, int) (json.RawMessage, error)
}
