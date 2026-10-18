package monitor

import (
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/integrations/observation"
)

func projectNodes(r kubecommon.Resource, o map[string]any, f map[string]observation.Value) error {
	f["unschedulable"] = projectedValue(o, "spec", "unschedulable")
	for _, field := range []string{"kubeletVersion", "osImage", "containerRuntimeVersion", "architecture"} {
		f[field] = projectedValue(o, "status", "nodeInfo", field)
	}
	return nil
}
