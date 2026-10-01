package kubernetes

import (
	"context"
	"fmt"
	"reflect"
	"slices"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// PodTargets follows controller UIDs, never a label match that could select an
// unrelated workload. Discovery supplies every served group/version.
func (c *Client) PodTargets(ctx context.Context, r Resource, object *unstructured.Unstructured, resources []Resource) ([]unstructured.Unstructured, error) {
	current, err := c.Get(ctx, r, object.GetNamespace(), object.GetName())
	if err != nil {
		return nil, err
	}
	if current.GetUID() != object.GetUID() || object.GetUID() == "" {
		return nil, TargetChangedError()
	}
	if r.Key() == "pods" {
		return []unstructured.Unstructured{*current}, nil
	}
	if !HasPodTargets(r) {
		return nil, operationError("This resource has no Pod execution target.")
	}
	parents := []unstructured.Unstructured{*current}
	intermediate := ""
	switch r.Key() {
	case "deployments.apps":
		intermediate = "replicasets.apps"
	case "cronjobs.batch":
		intermediate = "jobs.batch"
	}
	for _, key := range []string{intermediate, "pods"} {
		if key == "" {
			continue
		}
		child, ok := findResourceIdentity(resources, key)
		if !ok {
			return nil, operationError(fmt.Sprintf("The %s API is unavailable.", key))
		}
		collection, err := c.List(ctx, child, object.GetNamespace(), "")
		if err != nil {
			return nil, err
		}
		if collection.Truncated {
			return nil, operationError(fmt.Sprintf("Too many %s to resolve ownership completely. Open the Pod directly.", key))
		}
		var children []unstructured.Unstructured
		for _, candidate := range collection.Objects {
			for _, owner := range candidate.GetOwnerReferences() {
				if owner.Controller == nil || !*owner.Controller {
					continue
				}
				for _, parent := range parents {
					if owner.UID != "" && owner.UID == parent.GetUID() && owner.Name == parent.GetName() && owner.Kind == parent.GetKind() && groupOf(owner.APIVersion) == groupOf(parent.GetAPIVersion()) {
						children = append(children, candidate)
						break
					}
				}
			}
		}
		parents = children
	}
	slices.SortFunc(parents, func(a, b unstructured.Unstructured) int {
		if a.GetName() < b.GetName() {
			return -1
		}
		if a.GetName() > b.GetName() {
			return 1
		}
		return 0
	})
	return parents, nil
}

func groupOf(version string) string {
	for i, r := range version {
		if r == '/' {
			return version[:i]
		}
	}
	return ""
}

func HasPodTargets(r Resource) bool {
	switch r.Key() {
	case "pods", "deployments.apps", "statefulsets.apps", "daemonsets.apps", "replicasets.apps", "replicationcontrollers", "jobs.batch", "cronjobs.batch":
		return true
	}
	return false
}

func RunningContainers(object unstructured.Unstructured) []string {
	var names []string
	for _, path := range []string{"containerStatuses", "initContainerStatuses", "ephemeralContainerStatuses"} {
		states, _, _ := unstructured.NestedSlice(object.Object, "status", path)
		for _, item := range states {
			state, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if _, found, _ := unstructured.NestedMap(state, "state", "running"); found {
				if name, _, _ := unstructured.NestedString(state, "name"); name != "" {
					names = append(names, name)
				}
			}
		}
	}
	return names
}

func MetadataEditOnly(r Resource, object *unstructured.Unstructured) bool {
	switch r.Key() {
	case "controllerrevisions.apps", "ipaddresses.networking.k8s.io", "volumeattributesclasses.storage.k8s.io", "certificatesigningrequests.certificates.k8s.io":
		return true
	case "configmaps", "secrets":
		immutable, _, _ := unstructured.NestedBool(object.Object, "immutable")
		return immutable
	}
	return false
}

func ValidateResourceEdit(r Resource, original, proposed *unstructured.Unstructured) error {
	if r.Key() == "events" || r.Key() == "events.events.k8s.io" {
		return operationError("Events are inspection-only; editing is unavailable.")
	}
	if !MetadataEditOnly(r, original) {
		return nil
	}
	a, b := original.DeepCopy(), proposed.DeepCopy()
	delete(a.Object, "metadata")
	delete(b.Object, "metadata")
	if !reflect.DeepEqual(a.Object, b.Object) {
		return operationError("Only metadata can be edited on this object.")
	}
	return nil
}
