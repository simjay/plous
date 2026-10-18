package agent

import (
	"context"
	"encoding/json"
)

// UI is a session-bound host port. Only the application loop changes view state.
type UI interface {
	Read(context.Context) (UIResult, error)
	Show(context.Context, ViewRequest) (UIResult, error)
}

type ViewRequest struct {
	View            string `json:"view"`
	ConnectionID    string `json:"connection_id,omitempty"`
	Resource        string `json:"resource,omitempty"`
	Namespace       string `json:"namespace,omitempty"`
	Name            string `json:"name,omitempty"`
	UID             string `json:"uid,omitempty"`
	Tab             string `json:"tab,omitempty"`
	Filter          string `json:"filter,omitempty"`
	Labels          string `json:"labels,omitempty"`
	Container       string `json:"container,omitempty"`
	Previous        bool   `json:"previous,omitempty"`
	Highlight       string `json:"highlight,omitempty"`
	Path            string `json:"path,omitempty"`
	Line            int    `json:"line,omitempty"`
	Identity        string `json:"identity,omitempty"`
	Revision        int    `json:"revision,omitempty"`
	CompareRevision int    `json:"compare_revision,omitempty"`
	Ref             string `json:"ref,omitempty"`
	Offset          int    `json:"offset,omitempty"`
	// Evidence is resolved by the guarded history reader, never supplied by a model.
	Evidence json.RawMessage `json:"-"`
}

type UIResult struct {
	Selected  *ViewRequest `json:"selected,omitempty"`
	Status    string       `json:"status"`
	Reason    string       `json:"reason,omitempty"`
	View      *ViewRequest `json:"view,omitempty"`
	Focus     string       `json:"focus,omitempty"`
	Visible   bool         `json:"visible"`
	Available []string     `json:"available,omitempty"`
	Origin    string       `json:"origin,omitempty"`
}
