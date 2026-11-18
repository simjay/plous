package ai

import (
	"errors"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
)

// Connection is a send-time snapshot. Local configuration stays out of prompts.
type Connection struct {
	TargetCaptured bool               `json:"-"`
	Target         *kubecommon.Target `json:"-"`
	ConnectionID   string             `json:"connection_id"`
	Name           string             `json:"name"`
	Tool           string             `json:"tool"`
	Context        string             `json:"context,omitempty"`
	Namespace      string             `json:"default_namespace,omitempty"`
	Path           string             `json:"-"`
	Driver         string             `json:"-"`
}

// ClusterConnections exposes both read capabilities of one captured cluster.
// The base identity is retained for existing saved connections and histories.
func ClusterConnections(c Connection) []Connection {
	other := c
	if c.Tool == "kubernetes" {
		other.Tool = "helm"
	} else if c.Tool == "helm" {
		other.Tool = "kubernetes"
	} else {
		return []Connection{c}
	}
	other.ConnectionID = c.ConnectionID + "/" + other.Tool
	return []Connection{c, other}
}

func (op *operation) hasTool(name string) bool {
	if op.o.WorkspaceID == "" {
		return false
	}
	for _, c := range op.o.Connections {
		if c.Tool == name {
			return true
		}
	}
	return false
}

func (op *operation) connection(id, tool string) (Connection, error) {
	if op.o.WorkspaceID != "" {
		for _, c := range op.o.Connections {
			if c.ConnectionID == id && (tool == "" || c.Tool == tool) && (c.Tool == "kubernetes" || c.Tool == "helm") {
				return c, nil
			}
		}
	}
	return Connection{}, errors.New("connection is unavailable for this read in the captured workspace")
}
