package monitor

import (
	"strings"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
)

func ExactResource(resources []kubecommon.Resource, key string) (kubecommon.Resource, bool) {
	for _, r := range resources {
		if r.Key() == key {
			return r, true
		}
	}
	return kubecommon.Resource{}, false
}

func ApiGroup(version string) string {
	group, _, found := strings.Cut(version, "/")
	if found {
		return group
	}
	return ""
}
