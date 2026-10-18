package monitor

import (
	"strings"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

func ValueControls(r kubecommon.Resource) []string {
	labels := []string{"Back", "Copy value"}
	if r.Key() == "certificatesigningrequests.certificates.k8s.io" {
		labels = append(labels, "Export PEM")
	}
	return labels
}

func ActionWarning(a *ResourceActionState) string {
	var notes []string
	if ManagedResource(a.Resource) {
		notes = append(notes, "Advanced operation on a controller or system record. Reconciliation can overwrite changes or recreate the record.")
	}
	if a.Operation == "delete" {
		notes = append(notes, "Normal graceful deletion. Finalizers remain in force.")
		switch a.Resource.Key() {
		case "namespaces":
			notes = append(notes, "This deletes the namespace and its namespaced resources.")
		case "customresourcedefinitions.apiextensions.k8s.io":
			notes = append(notes, "This removes the API definition and its custom resources.")
		case "nodes":
			notes = append(notes, "This deletes the Node API object, not the machine. It does not drain workloads.")
		case "persistentvolumes", "persistentvolumeclaims":
			notes = append(notes, "Storage deletion can affect data. Review the bound volume's reclaim policy and consumers in Related before confirming.")
		case "volumesnapshots.snapshot.storage.k8s.io", "volumesnapshotcontents.snapshot.storage.k8s.io":
			notes = append(notes, "Backing snapshot retention depends on deletionPolicy. Review the snapshot content before confirming.")
		case "volumeattachments.storage.k8s.io":
			notes = append(notes, "This is a controller-managed attachment record. Deletion is not proof of safe volume detachment.")
		}
	}
	if a.Operation == "restart" {
		notes = append(notes, kubernetes.RestartNotice(a.Resource, *a.Object))
		if a.Resource.Key() == "deployments.apps" && kubernetes.Value(a.Object.Object, "spec", "paused") == "true" {
			notes = append(notes, "This Deployment is paused. Rollout will not progress until resumed outside this operation.")
		}
	}
	if a.Resource.Key() == "statefulsets.apps" && (a.Operation == "scale" || a.Operation == "delete") {
		notes = append(notes, "PVC retention: whenScaled="+kubernetes.Value(a.Object.Object, "spec", "persistentVolumeClaimRetentionPolicy", "whenScaled")+", whenDeleted="+kubernetes.Value(a.Object.Object, "spec", "persistentVolumeClaimRetentionPolicy", "whenDeleted")+". Absent policy defaults to Retain.")
	}
	for _, owner := range a.Object.GetOwnerReferences() {
		if owner.Controller != nil && *owner.Controller {
			notes = append(notes, "Managed by "+owner.Kind+" / "+owner.Name+". The controller may reconcile direct changes or recreate this object.")
		}
	}
	if len(a.Object.GetFinalizers()) > 0 {
		notes = append(notes, "Finalizers: "+strings.Join(a.Object.GetFinalizers(), ", "))
	}
	return strings.Join(notes, "\n\n")
}
