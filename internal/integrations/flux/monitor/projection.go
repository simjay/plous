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
		return true, errors.New("unsupported Flux API version")
	}
	item := ParseFlux(unstructured.Unstructured{Object: object}, now)
	gitops.ProjectFacts(item, facts)
	facts["suspended"] = observation.Boolean(item.Suspended)
	facts["stale"] = observation.Boolean(item.Stale)
	facts["destination.remote"] = observation.Boolean(item.Destination.Remote)
	for _, condition := range item.Conditions {
		facts["condition."+condition.Type+".stale"] = observation.Boolean(condition.Stale)
	}
	return true, nil
}
