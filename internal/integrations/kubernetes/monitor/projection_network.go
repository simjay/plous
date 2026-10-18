package monitor

import (
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/integrations/observation"
	"strconv"
)

func projectServices(r kubecommon.Resource, o map[string]any, f map[string]observation.Value) error {
	projectStringMap(o, f, "selector.", "spec", "selector")
	for _, field := range []string{"type", "clusterIP", "externalName"} {
		f[field] = projectedValue(o, "spec", field)
	}
	for i, p := range Maps(o, "spec", "ports") {
		for _, field := range []string{"name", "port", "targetPort", "protocol", "nodePort"} {
			f["port."+strconv.Itoa(i)+"."+field] = projectedValue(p, field)
		}
	}
	return nil
}
