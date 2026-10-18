package monitor

import (
	"errors"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/integrations/observation"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"slices"
	"time"
)

const ReadHelp = `Read Argo controller facts from the captured Kubernetes connection. Use operation get to list or read a named resource, or describe with a name. Returns controller status, reported revisions, sources, dependencies and managed inventory. Controller observations do not prove current downstream resource health. No cross-cluster or source acquisition occurs.`

var readResources = []string{"applications.argoproj.io", "applicationsets.argoproj.io", "appprojects.argoproj.io"}

func ReadOperations() []string { return []string{"get", "describe"} }

func ReadSchema() map[string]any {
	return kubernetes.ControllerReadSchema(readResources, ReadOperations())
}

func ValidateRead(c kubernetes.Command) error {
	if !slices.Contains(ReadOperations(), c.Operation) || !slices.Contains(readResources, c.Resource) {
		return errors.New("Argo reads require get or describe and a supported controller resource")
	}
	return nil
}

func ProjectFacts(r kubecommon.Resource, object map[string]any, facts map[string]observation.Value, now time.Time) (bool, error) {
	if !slices.Contains(readResources, r.Key()) {
		return false, nil
	}
	if !SupportsVersion(r) {
		return true, errors.New("unsupported Argo API version")
	}
	item := Parse(unstructured.Unstructured{Object: object}, now)
	gitops.ProjectFacts(item, facts)
	facts["controller_error"] = observation.Boolean(item.ControllerError)
	return true, nil
}
