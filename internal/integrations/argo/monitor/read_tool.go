package monitor

import (
	"errors"
	"slices"

	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

const ReadHelp = `Read Argo controller facts from the captured Kubernetes connection. Use list for objects of one type and get for a named object. Returns controller status, reported revisions, sources, dependencies and managed inventory. Controller observations do not prove current downstream resource health. No cross-cluster or source acquisition occurs.`

var readResources = []string{"applications.argoproj.io", "applicationsets.argoproj.io", "appprojects.argoproj.io"}

func ReadOperations() []string { return []string{"list", "get"} }

func ReadSchema() map[string]any {
	return kubernetes.ControllerReadSchema(readResources, ReadOperations())
}

func ValidateRead(c kubernetes.Command) error {
	if !slices.Contains(ReadOperations(), c.Operation) || !slices.Contains(readResources, c.Resource) {
		return errors.New("Argo reads require list or get and a supported controller resource")
	}
	return nil
}
