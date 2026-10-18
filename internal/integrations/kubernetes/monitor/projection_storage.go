package monitor

import (
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/integrations/observation"
)

func projectVolumes(r kubecommon.Resource, o map[string]any, f map[string]observation.Value) error {
	f["phase"] = projectedValue(o, "status", "phase")
	for _, field := range []string{"storageClassName", "volumeName", "volumeMode", "persistentVolumeReclaimPolicy"} {
		f[field] = projectedValue(o, "spec", field)
	}
	section := "status"
	if r.Name == "persistentvolumes" {
		section = "spec"
	}
	f["capacity"] = projectedValue(o, section, "capacity", "storage")
	return nil
}
