package monitor

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type Reference struct {
	Resource                       kubecommon.Resource
	Namespace, Name, UID, Relation string
	List                           bool
}

func Find(resources []kubecommon.Resource, name string) (kubecommon.Resource, bool) {
	if resource, ok := findResourceIdentity(resources, name); ok {
		return resource, true
	}
	var matches []kubecommon.Resource
	for _, r := range resources {
		if r.Matches(name) {
			matches = append(matches, r)
		}
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	return kubecommon.Resource{}, false
}

// Relationships name an API identity, not a user-entered alias. Falling back
// to aliases could connect a core reference to an unrelated extension API.
func findResourceIdentity(resources []kubecommon.Resource, name string) (kubecommon.Resource, bool) {
	for _, r := range resources {
		if r.Key() == name {
			return r, true
		}
	}
	return kubecommon.Resource{}, false
}

func (c *Client) Events(ctx context.Context, object unstructured.Unstructured) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	list, err := c.resource(kubecommon.Resource{Name: "events", Version: "v1", Namespaced: true}, object.GetNamespace()).List(ctx, metav1.ListOptions{FieldSelector: "involvedObject.uid=" + string(object.GetUID()), Limit: 500})
	if err != nil {
		return "", err
	}
	sort.Slice(list.Items, func(i, j int) bool { return eventTime(list.Items[i]) > eventTime(list.Items[j]) })
	var entries []string
	for _, event := range list.Items {
		o := event.Object
		entries = append(entries, Value(o, "type")+" · "+Value(o, "reason"), Value(o, "message"), eventTime(event)+" · count "+fallback(firstValue(o, "series.count", "count", "deprecatedCount"), "Not reported"), "")
	}
	if len(entries) == 0 {
		return "No events retained for this object.", nil
	}
	return strings.Join(entries, "\n"), nil
}
func eventTime(object unstructured.Unstructured) string {
	for _, fields := range [][]string{{"series", "lastObservedTime"}, {"lastTimestamp"}, {"eventTime"}, {"deprecatedLastTimestamp"}, {"metadata", "creationTimestamp"}} {
		if value := Value(object.Object, fields...); value != "" {
			return value
		}
	}
	return ""
}

func (c *Client) Related(ctx context.Context, r kubecommon.Resource, object unstructured.Unstructured, resources []kubecommon.Resource) ([]Reference, string) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var result []Reference
	var warnings []string
	seen := map[string]bool{}
	add := func(resource kubecommon.Resource, ns, name, uid, relation string) {
		if name == "" {
			return
		}
		key := resource.Key() + "/" + ns + "/" + name
		if !seen[key] {
			seen[key] = true
			result = append(result, Reference{Resource: resource, Namespace: ns, Name: name, UID: uid, Relation: relation})
		}
	}
	byName := func(name, ns, objectName, relation string) {
		if objectName == "" {
			return
		}
		resource, ok := findResourceIdentity(resources, name)
		if ok {
			add(resource, ns, objectName, "", relation)
		} else {
			warnings = append(warnings, name+" API unavailable")
		}
	}
	for _, owner := range object.GetOwnerReferences() {
		gv, _ := schema.ParseGroupVersion(owner.APIVersion)
		found := false
		for _, resource := range resources {
			if resource.Group == gv.Group && resource.Kind == owner.Kind {
				ns := object.GetNamespace()
				if !resource.Namespaced {
					ns = ""
				}
				add(resource, ns, owner.Name, string(owner.UID), "Owner")
				found = true
				break
			}
		}
		if !found {
			warnings = append(warnings, owner.Kind+" owner API unavailable")
		}
	}
	if !HasPresenter(r) {
		result = append(result, c.customDefinitionReference(ctx, r, object, resources)...)
		return result, strings.Join(warnings, "\n")
	}
	o, ns := object.Object, object.GetNamespace()
	switch r.Name {
	case "pods":
		byName("nodes", "", Value(o, "spec", "nodeName"), "Assigned node")
		byName("serviceaccounts", ns, Value(o, "spec", "serviceAccountName"), "Service account")
		for _, volume := range Maps(o, "spec", "volumes") {
			byName("persistentvolumeclaims", ns, Value(volume, "persistentVolumeClaim", "claimName"), "Volume claim")
			byName("configmaps", ns, Value(volume, "configMap", "name"), "Configuration")
			byName("secrets", ns, Value(volume, "secret", "secretName"), "Secret reference")
			for _, source := range Maps(volume, "projected", "sources") {
				byName("configmaps", ns, Value(source, "configMap", "name"), "Projected configuration")
				byName("secrets", ns, Value(source, "secret", "name"), "Projected secret")
			}
		}
		for _, kind := range []string{"containers", "initContainers", "ephemeralContainers"} {
			for _, container := range Maps(o, "spec", kind) {
				for _, env := range Maps(container, "envFrom") {
					byName("configmaps", ns, Value(env, "configMapRef", "name"), "Environment configuration")
					byName("secrets", ns, Value(env, "secretRef", "name"), "Environment secret")
				}
				for _, env := range Maps(container, "env") {
					byName("configmaps", ns, Value(env, "valueFrom", "configMapKeyRef", "name"), "Environment configuration")
					byName("secrets", ns, Value(env, "valueFrom", "secretKeyRef", "name"), "Environment secret")
				}
			}
		}
	case "persistentvolumeclaims":
		byName("storageclasses.storage.k8s.io", "", Value(o, "spec", "storageClassName"), "Storage class")
	case "persistentvolumes":
		if claim, ok := findResourceIdentity(resources, "persistentvolumeclaims"); ok {
			add(claim, Value(o, "spec", "claimRef", "namespace"), Value(o, "spec", "claimRef", "name"), Value(o, "spec", "claimRef", "uid"), "Bound claim")
		}
		byName("storageclasses.storage.k8s.io", "", Value(o, "spec", "storageClassName"), "Storage class")
	case "endpointslices":
		byName("services", ns, object.GetLabels()["kubernetes.io/service-name"], "Service")
		for _, endpoint := range Maps(o, "endpoints") {
			targetVersion := Value(endpoint, "targetRef", "apiVersion")
			if Value(endpoint, "targetRef", "kind") == "Pod" && (targetVersion == "" || targetVersion == "v1") {
				targetNS := Value(endpoint, "targetRef", "namespace")
				if targetNS == "" {
					targetNS = ns
				}
				if pod, ok := findResourceIdentity(resources, "pods"); ok {
					add(pod, targetNS, Value(endpoint, "targetRef", "name"), Value(endpoint, "targetRef", "uid"), "Endpoint · ready="+Value(endpoint, "conditions", "ready"))
				} else {
					warnings = append(warnings, "pods API unavailable")
				}
			}
		}
	}
	childKind := ""
	switch r.Name {
	case "deployments":
		childKind = "replicasets.apps"
	case "replicasets", "statefulsets", "daemonsets", "replicationcontrollers", "jobs":
		childKind = "pods"
	case "cronjobs":
		childKind = "jobs.batch"
	case "services":
		childKind = "endpointslices.discovery.k8s.io"
	case "nodes":
		childKind = "pods"
	}
	if childKind != "" {
		child, ok := findResourceIdentity(resources, childKind)
		if !ok {
			warnings = append(warnings, childKind+" API unavailable")
		} else {
			selector := ""
			if r.Name == "services" {
				selector = labels.Set{"kubernetes.io/service-name": object.GetName()}.String()
			}
			collection, err := c.List(ctx, child, ns, selector)
			if err != nil {
				warnings = append(warnings, child.Kind+": "+ErrorText(err))
			} else {
				if collection.Truncated {
					warnings = append(warnings, "Related list limited to "+fmt.Sprint(MaxObjects)+" objects")
				}
				for _, candidate := range collection.Objects {
					linked := false
					relation := "Owned " + child.Kind
					for _, owner := range candidate.GetOwnerReferences() {
						if object.GetUID() != "" && owner.UID == object.GetUID() {
							linked = true
						}
					}
					if r.Name == "services" && candidate.GetLabels()["kubernetes.io/service-name"] == object.GetName() {
						linked = true
						relation = "Service endpoints"
					}
					if r.Name == "nodes" && Value(candidate.Object, "spec", "nodeName") == object.GetName() {
						linked = true
						relation = "Assigned Pod"
					}
					if linked {
						add(child, candidate.GetNamespace(), candidate.GetName(), string(candidate.GetUID()), relation)
					}
				}
			}
		}
	}
	result = append(result, resourceListReferences(r, object, resources)...)
	warnings = append(warnings, c.additionalRelated(ctx, r, object, resources, add)...)
	return result, strings.Join(warnings, "\n")
}
