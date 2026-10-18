package monitor

import (
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

const AllApplicationContainers = "All application containers"

func (a *ResourceActionState) AllPodsChoice() bool {
	return a.Operation == "logs" && a.Resource.Key() != "pods" && kubernetes.HasPodTargets(a.Resource)
}
