package monitor

import (
	"errors"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/integrations/observation"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"slices"
	"time"
)

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
