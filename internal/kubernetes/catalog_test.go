package kubernetes

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestConceptsCatalogMatchesAPIIdentity(t *testing.T) {
	cases := []struct {
		group, name, category, title string
	}{
		{"", "pods", "Workloads", "Pods"},
		{"apps", "deployments", "Workloads", "Deployments"},
		{"batch", "jobs", "Workloads", "Jobs"},
		{"autoscaling", "horizontalpodautoscalers", "Workloads", "HorizontalPodAutoscalers"},
		{"policy", "poddisruptionbudgets", "Workloads", "PodDisruptionBudgets"},
		{"", "configmaps", "Config", "ConfigMaps"},
		{"", "secrets", "Config", "Secrets"},
		{"", "resourcequotas", "Policies", "ResourceQuotas"},
		{"", "limitranges", "Policies", "LimitRanges"},
		{"", "nodes", "Cluster", "Nodes"},
		{"coordination.k8s.io", "leases", "Cluster", "Leases"},
		{"node.k8s.io", "runtimeclasses", "Containers", "RuntimeClasses"},
		{"", "namespaces", "Overview", "Namespaces"},
		{"", "services", "Network", "Services"},
		{"discovery.k8s.io", "endpointslices", "Network", "EndpointSlices"},
		{"storage.k8s.io", "csistoragecapacities", "Storage", "CSIStorageCapacities"},
		{"rbac.authorization.k8s.io", "clusterroles", "Security", "ClusterRoles"},
		{"admissionregistration.k8s.io", "validatingadmissionpolicies", "Security", "ValidatingAdmissionPolicies"},
		{"scheduling.k8s.io", "priorityclasses", "Scheduling", "PriorityClasses"},
		{"resource.k8s.io", "resourceclaims", "Allocation", "ResourceClaims"},
		{"apiextensions.k8s.io", "customresourcedefinitions", "Extensions", "CustomResourceDefinitions"},
		{"example.io", "pods", "Other APIs", "pods (example.io)"},
		{"example.io", "services", "Other APIs", "services (example.io)"},
		{"example.io", "policies", "Other APIs", "policies (example.io)"},
		{"apps", "newresources", "Other APIs", "newresources (apps)"},
		{"", "newresources", "Other APIs", "newresources (core)"},
	}
	for _, tc := range cases {
		r := Resource{Group: tc.group, Name: tc.name, Kind: "DoNotInferFromKind"}
		t.Run(r.Key(), func(t *testing.T) {
			if got := Category(r); got != tc.category {
				t.Errorf("category = %q, want %q", got, tc.category)
			}
			if got := ResourceTitle(r); got != tc.title {
				t.Errorf("title = %q, want %q", got, tc.title)
			}
		})
	}
	for identity, entry := range resourceCatalog {
		if !slices.Contains(Categories, categoryLabel(entry.category)) || entry.title == "" {
			t.Errorf("catalog entry %q cannot appear in navigation: %#v", identity, entry)
		}
	}
}

func TestCatalogUsesStableCategoriesAndCompleteWorkloadOrder(t *testing.T) {
	if len(resourceCatalog) != 73 {
		t.Fatalf("catalog contains %d identities, want 73", len(resourceCatalog))
	}
	for _, category := range navigationCategories {
		for _, value := range []string{category.id, category.label, category.previousLabel} {
			if got := CategoryID(value); got != category.id {
				t.Errorf("saved category %q resolves to %q, want %q", value, got, category.id)
			}
		}
	}
	var resources []Resource
	for identity, entry := range resourceCatalog {
		if entry.category == "workloads" {
			name, group, _ := strings.Cut(identity, ".")
			resources = append(resources, Resource{Name: name, Group: group})
		}
	}
	slices.SortFunc(resources, CompareResources)
	got := make([]string, len(resources))
	for i, resource := range resources {
		got[i] = resource.Key()
	}
	want := []string{
		"pods", "deployments.apps", "statefulsets.apps", "daemonsets.apps",
		"replicasets.apps", "replicationcontrollers", "jobs.batch", "cronjobs.batch",
		"horizontalpodautoscalers.autoscaling", "verticalpodautoscalers.autoscaling.k8s.io",
		"poddisruptionbudgets.policy", "controllerrevisions.apps",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("workload navigation = %v, want %v", got, want)
	}
}

func TestDiscoveryIncludesResourcesMissingFromPreferredVersion(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/apis":
			reply(w, metav1.APIGroupList{Groups: []metav1.APIGroup{{
				Name: "example.io", PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: "example.io/v1"},
				Versions: []metav1.GroupVersionForDiscovery{
					{GroupVersion: "example.io/v2beta1"}, {GroupVersion: "example.io/v1"},
					{GroupVersion: "example.io/v2"}, {GroupVersion: "example.io/v1beta1"},
				},
			}}})
		case "/api/v1":
			reply(w, metav1.APIResourceList{})
		case "/apis/example.io/v1":
			reply(w, metav1.APIResourceList{APIResources: []metav1.APIResource{
				{Name: "widgets", Kind: "Widget", Namespaced: true, Verbs: []string{"get", "list", "watch"}},
			}})
		case "/apis/example.io/v2", "/apis/example.io/v2beta1", "/apis/example.io/v1beta1":
			reply(w, metav1.APIResourceList{APIResources: []metav1.APIResource{
				{Name: "widgets", Kind: "Widget", Verbs: []string{"list"}},
				{Name: "gadgets", Kind: "Gadget", Namespaced: true, Verbs: []string{"list"}},
				{Name: "gadgets/status", Kind: "Gadget", Verbs: []string{"list"}},
				{Name: "reviews", Kind: "Review", Verbs: []string{"create"}},
			}})
		default:
			t.Errorf("unexpected discovery path: %s", r.URL.Path)
			apiError(w, 404, "NotFound")
		}
	})
	resources, warning, err := client.Discover(context.Background())
	if err != nil || warning != "" || len(resources) != 2 {
		t.Fatalf("resources = %#v, warning = %q, err = %v", resources, warning, err)
	}
	widgets, ok := Find(resources, "widgets.example.io")
	if !ok || widgets.Version != "v1" || !widgets.Namespaced || !slices.Contains(widgets.Verbs, "watch") {
		t.Fatalf("preferred version's metadata lost: %#v", widgets)
	}
	gadgets, ok := Find(resources, "gadgets.example.io")
	if !ok || gadgets.Version != "v2" {
		t.Fatalf("non-preferred resource or stable version fallback lost: %#v", gadgets)
	}
}

func TestDiscoveryUsesHealthyVersionWhenPreferredDiscoveryFails(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/apis":
			reply(w, metav1.APIGroupList{Groups: []metav1.APIGroup{{
				Name: "example.io", PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: "example.io/v1"},
				Versions: []metav1.GroupVersionForDiscovery{{GroupVersion: "example.io/v1"}, {GroupVersion: "example.io/v1beta1"}},
			}}})
		case "/api/v1":
			reply(w, metav1.APIResourceList{})
		case "/apis/example.io/v1beta1":
			reply(w, metav1.APIResourceList{APIResources: []metav1.APIResource{{Name: "widgets", Kind: "Widget", Verbs: []string{"list"}}}})
		default:
			apiError(w, 403, "Forbidden")
		}
	})
	resources, warning, err := client.Discover(context.Background())
	if err != nil || warning == "" || len(resources) != 1 || resources[0].Version != "v1beta1" {
		t.Fatalf("partial discovery = %#v, %q, %v", resources, warning, err)
	}
}

func TestDiscoveryCancellationDiscardsPartialCatalog(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		cancel()
		reply(w, metav1.APIGroupList{})
	})
	resources, warning, err := client.Discover(ctx)
	if !errors.Is(err, context.Canceled) || resources != nil || warning != "" {
		t.Fatalf("cancelled discovery = %#v, %q, %v", resources, warning, err)
	}
}

func TestNativeRelationshipsDoNotResolveToSameNamedExtensions(t *testing.T) {
	pod := unstructured.Unstructured{Object: testPod("checkout", "pod")}
	pod.Object["spec"].(map[string]any)["nodeName"] = "worker"
	resources := []Resource{{Group: "example.io", Name: "nodes", Kind: "Node"}}
	var client *Client
	references, warning := client.Related(context.Background(), Resource{Name: "pods", Kind: "Pod"}, pod, resources)
	if len(references) != 0 || !strings.Contains(warning, "nodes API unavailable") {
		t.Fatalf("core Pod reference resolved to an extension: %#v, %q", references, warning)
	}
	deployment := Resource{Group: "apps", Name: "deployments", Kind: "Deployment"}
	resources = []Resource{{Group: "example.io", Name: "replicasets", Kind: "ReplicaSet", ShortNames: []string{"replicasets.apps"}}}
	references, warning = client.Related(context.Background(), deployment, unstructured.Unstructured{}, resources)
	if len(references) != 0 || !strings.Contains(warning, "replicasets.apps API unavailable") {
		t.Fatalf("workload relationship followed an extension alias: %#v, %q", references, warning)
	}
}
