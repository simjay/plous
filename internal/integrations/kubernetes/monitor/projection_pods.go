package monitor

import (
	"fmt"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/integrations/observation"
	"strings"
)

func projectPods(r kubecommon.Resource, o map[string]any, f map[string]observation.Value) error {
	if r.Group != "" || r.Version != "v1" {
		return fmt.Errorf("unsupported Pod API version")
	}
	projectPodReservations(o, f)
	facts := podFacts(o)
	f["phase"] = projectedValue(o, "status", "phase")
	f["status_label"] = observation.Text(facts.Status)
	f["ready"] = facts.Ready
	f["containers_ready"] = observation.Number(int64(facts.ContainersReady))
	f["containers_total"] = observation.Number(int64(facts.ContainersTotal))
	f["restarts"] = observation.Number(int64(facts.Restarts))
	f["node"] = projectedValue(o, "spec", "nodeName")
	for _, field := range []string{"qosClass", "podIP", "reason", "message"} {
		f[field] = projectedValue(o, "status", field)
	}
	for _, group := range []string{"containerStatuses", "initContainerStatuses", "ephemeralContainerStatuses"} {
		for _, c := range Maps(o, "status", group) {
			prefix := group + "." + Value(c, "name") + "."
			for _, field := range []string{"ready", "restartCount"} {
				f[prefix+field] = projectedValue(c, field)
			}
			for _, state := range []string{"state", "lastState"} {
				for _, kind := range []string{"waiting", "terminated"} {
					for _, field := range []string{"reason", "message", "exitCode", "finishedAt"} {
						if v := projectedValue(c, state, kind, field); !v.Unknown {
							f[prefix+state+"."+kind+"."+field] = v
						}
					}
				}
			}
		}
	}
	return nil
}

type podObservation struct {
	Ready                                      observation.Value
	ContainersReady, ContainersTotal, Restarts int
	Status                                     string
}

func podFacts(o map[string]any) podObservation {
	ready, total, restarts, status := podDisplayState(o)
	facts := podObservation{Ready: observation.Unknown(), ContainersReady: ready, ContainersTotal: total, Restarts: restarts, Status: status}
	for _, c := range Maps(o, "status", "conditions") {
		if Value(c, "type") == "Ready" {
			switch strings.ToLower(Value(c, "status")) {
			case "true":
				facts.Ready = observation.Boolean(true)
			case "false":
				facts.Ready = observation.Boolean(false)
			}
		}
	}
	return facts
}
