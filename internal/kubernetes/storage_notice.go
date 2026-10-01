package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// StorageDeleteNotice reports the current binding and consumers for deletion
// review. Query failures remain visible and never become zero consumers.
func (c *Client) StorageDeleteNotice(ctx context.Context, r Resource, object *unstructured.Unstructured, resources []Resource) string {
	if object == nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if r.Key() == "volumesnapshots.snapshot.storage.k8s.io" || r.Key() == "volumesnapshotcontents.snapshot.storage.k8s.io" {
		return c.snapshotDeleteNotice(ctx, r, object, resources)
	}
	if r.Key() != "persistentvolumeclaims" && r.Key() != "persistentvolumes" {
		return ""
	}
	var notes []string
	var claim *unstructured.Unstructured
	if r.Key() == "persistentvolumes" {
		notes = append(notes, "PV reclaim policy: "+fallback(Value(object.Object, "spec", "persistentVolumeReclaimPolicy"), "Not reported"))
		ref := nestedMap(object.Object, "spec", "claimRef")
		name, ns, uid := Value(ref, "name"), Value(ref, "namespace"), Value(ref, "uid")
		if name == "" {
			notes = append(notes, "Claim binding: no claimRef is reported.")
		} else if target, ok := findResourceIdentity(resources, "persistentvolumeclaims"); !ok {
			notes = append(notes, "Claim binding: PVC API unavailable; consumers are unknown.")
		} else if got, err := c.Get(ctx, target, ns, name); err != nil {
			notes = append(notes, "Claim binding: "+ErrorText(err)+"; consumers are unknown.")
		} else if uid == "" || string(got.GetUID()) != uid || Value(got.Object, "spec", "volumeName") != object.GetName() {
			notes = append(notes, "Claim binding: the referenced PVC does not match this PV binding and claim UID; consumers are unknown.")
		} else {
			claim = got
			notes = append(notes, "Bound claim: "+ns+"/"+name+" (verified UID "+uid+")")
		}
	} else {
		claim = object
		name := Value(object.Object, "spec", "volumeName")
		if name == "" {
			notes = append(notes, "Volume binding: no volumeName is reported.")
		} else if target, ok := findResourceIdentity(resources, "persistentvolumes"); !ok {
			notes = append(notes, "Backing PV reclaim policy: unknown; PV API unavailable.")
		} else if volume, err := c.Get(ctx, target, "", name); err != nil {
			notes = append(notes, "Backing PV reclaim policy: unknown; "+ErrorText(err))
		} else {
			ref := nestedMap(volume.Object, "spec", "claimRef")
			if string(object.GetUID()) == "" || Value(ref, "uid") != string(object.GetUID()) || Value(ref, "name") != object.GetName() || Value(ref, "namespace") != object.GetNamespace() {
				notes = append(notes, "Backing PV reclaim policy: unknown; "+name+" does not report a binding to this claim UID.")
			} else {
				notes = append(notes, "Bound PV: "+name+" (verified claim UID)", "Backing PV reclaim policy: "+fallback(Value(volume.Object, "spec", "persistentVolumeReclaimPolicy"), "Not reported"))
			}
		}
	}
	if claim != nil {
		podResource, ok := findResourceIdentity(resources, "pods")
		if !ok {
			notes = append(notes, "Consumers: unknown; Pods API unavailable.")
		} else if pods, err := c.List(ctx, podResource, claim.GetNamespace(), ""); err != nil {
			notes = append(notes, "Consumers: unknown; "+ErrorText(err))
		} else {
			var consumers []string
			for _, pod := range pods.Objects {
				for _, volume := range Maps(pod.Object, "spec", "volumes") {
					if Value(volume, "persistentVolumeClaim", "claimName") == claim.GetName() {
						consumers = append(consumers, pod.GetName()+" ("+fallback(Value(pod.Object, "status", "phase"), "phase not reported")+")")
						break
					}
				}
			}
			notes = append(notes, fmt.Sprintf("Consumers: %d Pod volume references in namespace %s.", len(consumers), claim.GetNamespace()))
			if len(consumers) > 0 {
				notes = append(notes, strings.Join(consumers[:min(len(consumers), 20)], ", "))
				if len(consumers) > 20 {
					notes = append(notes, fmt.Sprintf("%d more referenced Pods; inspect Related for the list.", len(consumers)-20))
				}
			}
			if pods.Truncated {
				notes = append(notes, fmt.Sprintf("Consumer coverage is partial: only %d Pods were queried; additional references may exist.", len(pods.Objects)))
			}
		}
	}
	return strings.Join(notes, "\n")
}

func (c *Client) snapshotDeleteNotice(ctx context.Context, r Resource, object *unstructured.Unstructured, resources []Resource) string {
	if r.Key() == "volumesnapshotcontents.snapshot.storage.k8s.io" {
		ref := nestedMap(object.Object, "spec", "volumeSnapshotRef")
		return "Snapshot content deletion policy: " + snapshotDeletionPolicy(Value(object.Object, "spec", "deletionPolicy")) + "\n" +
			"Reported snapshot reference: " + fallback(Value(ref, "namespace"), "namespace not reported") + "/" + fallback(Value(ref, "name"), "name not reported") +
			"; UID " + fallback(Value(ref, "uid"), "not reported") + ". The snapshot reference has not been verified."
	}
	name := Value(object.Object, "status", "boundVolumeSnapshotContentName")
	if name == "" {
		return "Backing snapshot deletion policy: unknown; no bound snapshot content is reported."
	}
	contentResource, ok := findResourceIdentity(resources, "volumesnapshotcontents.snapshot.storage.k8s.io")
	if !ok {
		return "Backing snapshot deletion policy: unknown; VolumeSnapshotContent API unavailable."
	}
	content, err := c.Get(ctx, contentResource, "", name)
	if err != nil {
		return "Backing snapshot deletion policy: unknown; " + ErrorText(err)
	}
	ref := nestedMap(content.Object, "spec", "volumeSnapshotRef")
	identityMatches := true
	if kind := Value(ref, "kind"); kind != "" && kind != "VolumeSnapshot" {
		identityMatches = false
	}
	if version := Value(ref, "apiVersion"); version != "" {
		groupVersion, err := schema.ParseGroupVersion(version)
		identityMatches = identityMatches && err == nil && groupVersion.Group == "snapshot.storage.k8s.io"
	}
	if !identityMatches || content.GetName() != name || string(object.GetUID()) == "" || Value(ref, "uid") != string(object.GetUID()) || Value(ref, "name") != object.GetName() || Value(ref, "namespace") != object.GetNamespace() {
		return "Backing snapshot deletion policy: unknown; content " + name + " does not report a reciprocal binding to this snapshot name, namespace, and UID."
	}
	return "Bound snapshot content: " + name + " (verified snapshot UID)\n" +
		"Backing snapshot deletion policy: " + snapshotDeletionPolicy(Value(content.Object, "spec", "deletionPolicy"))
}

func snapshotDeletionPolicy(policy string) string {
	switch policy {
	case "Delete":
		return "Delete. Instructs the snapshot controller to remove the backing snapshot with its content record."
	case "Retain":
		return "Retain. Keeps the backing snapshot after removal of its Kubernetes record."
	default:
		return fallback(policy, "Not reported")
	}
}
