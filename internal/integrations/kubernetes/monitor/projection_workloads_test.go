package monitor

import (
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/integrations/observation"
	"testing"
)

func TestWorkloadCountsAndReadinessHaveOneDefinition(t *testing.T) {
	for _, kind := range []string{"deployments.apps", "statefulsets.apps", "replicasets.apps", "replicationcontrollers", "daemonsets.apps"} {
		t.Run(kind, func(t *testing.T) {
			status := map[string]any{"readyReplicas": int64(2), "availableReplicas": int64(1), "updatedReplicas": int64(3), "observedGeneration": int64(6)}
			if kind == "daemonsets.apps" {
				status = map[string]any{"numberReady": int64(2), "numberAvailable": int64(1), "updatedNumberScheduled": int64(3), "desiredNumberScheduled": int64(4), "observedGeneration": int64(6)}
			}
			object := map[string]any{"metadata": map[string]any{"generation": int64(7)}, "spec": map[string]any{"replicas": int64(4)}, "status": status}
			facts := workloadFacts(kind, object)
			for key, want := range map[string]string{"desired_replicas": "4", "unready_replicas": "2", "unavailable_replicas": "3", "generation_observed": "false", "ready_ratio": "2/4"} {
				if facts[key].String() != want {
					t.Fatal(key, facts[key])
				}
			}
			if workloadReadyColumn(kind).value(object) != facts["ready_ratio"].String() {
				t.Fatal("UI diverged from projected readiness")
			}
		})
	}
}
func TestWorkloadOmittedZeroAndMissingStatus(t *testing.T) {
	r := kubecommon.Resource{Group: "apps", Version: "v1", Name: "deployments"}
	object := map[string]any{"spec": map[string]any{"replicas": int64(3)}, "status": map[string]any{}}
	f := map[string]observation.Value{}
	if err := projectWorkloads(r, object, f); err != nil {
		t.Fatal(err)
	}
	if f["readyReplicas"].String() != "0" || f["unready_replicas"].String() != "3" {
		t.Fatal(f)
	}
	delete(object, "status")
	if err := projectWorkloads(r, object, f); err != nil {
		t.Fatal(err)
	}
	if !f["readyReplicas"].Unknown || !f["unready_replicas"].Unknown {
		t.Fatal("missing status invented a count", f)
	}
}
