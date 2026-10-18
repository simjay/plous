package chat

import "github.com/simjay/plous/internal/source"

type AssistantPage struct {
	Source       *source.ReadSelection `json:"source,omitempty"`
	ConnectionID string                `json:"connection_id,omitempty"`
	Connection   string                `json:"connection,omitempty"`
	Tool         string                `json:"tool,omitempty"`
	Context      string                `json:"context,omitempty"`
	View         string                `json:"view"`
	Resource     string                `json:"resource,omitempty"`
	Name         string                `json:"name,omitempty"`
	UID          string                `json:"uid,omitempty"`
	Namespace    string                `json:"namespace,omitempty"`
	Filter       string                `json:"filter,omitempty"`
	Labels       string                `json:"labels,omitempty"`
	Revision     int                   `json:"revision,omitempty"`
}
