package common

import (
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
)

func FindResource(catalog []kubecommon.Resource, ref Reference) (kubecommon.Resource, bool) {
	for _, resource := range catalog {
		if resource.Group == ref.Group && resource.Kind == ref.Kind {
			return resource, true
		}
	}
	return kubecommon.Resource{}, false
}
