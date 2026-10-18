package monitor

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var serviceTestResource = kubecommon.Resource{Name: "services", Kind: "Service", Version: "v1", Namespaced: true, Verbs: []string{"list", "watch"}}
var sliceTestResource = kubecommon.Resource{Name: "endpointslices", Kind: "EndpointSlice", Group: "discovery.k8s.io", Version: "v1", Namespaced: true, Verbs: []string{"list"}}

func serviceFixture(t *testing.T) unstructured.Unstructured {
	return summaryFixture(t, `{"apiVersion":"v1","kind":"Service","metadata":{"namespace":"shop","name":"checkout","uid":"service-current"},"spec":{"type":"ClusterIP","clusterIP":"10.96.0.1"}}`)
}

func TestServiceListQueriesEndpointReadinessWithoutMutatingObjects(t *testing.T) {
	service := serviceFixture(t)
	slice := summaryFixture(t, `{"metadata":{"namespace":"shop","name":"one","uid":"slice-one","labels":{"kubernetes.io/service-name":"checkout"}},"addressType":"IPv4","endpoints":[{"addresses":["10.0.0.1"],"conditions":{"ready":true}},{"addresses":["10.0.0.2"],"conditions":{"ready":false}},{"addresses":["10.0.0.3"]}]}`)
	duplicate := slice.DeepCopy()
	duplicate.SetName("duplicate")
	duplicate.SetUID("slice-two")
	stale := slice.DeepCopy()
	stale.Object["metadata"].(map[string]any)["ownerReferences"] = []any{map[string]any{"apiVersion": "v1", "kind": "Service", "name": "checkout", "uid": "old-service"}}
	stale.Object["endpoints"] = []any{map[string]any{"addresses": []any{"10.0.0.9"}, "conditions": map[string]any{"ready": true}}}
	client := testClient(t, func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/namespaces/shop/services":
			reply(w, objectList("Service", "1", service.Object))
		case "/apis/discovery.k8s.io/v1/namespaces/shop/endpointslices":
			reply(w, objectList("EndpointSlice", "1", slice.Object, duplicate.Object, stale.Object))
		default:
			t.Errorf("unexpected request: %s", request.URL)
		}
	})
	client.catalog = []kubecommon.Resource{serviceTestResource, sliceTestResource}
	collection, err := client.List(context.Background(), serviceTestResource, "shop", "")
	if err != nil {
		t.Fatal(err)
	}
	readiness := collection.ServiceEndpoints["service-current"]
	if readiness.Ready != 1 || readiness.NotReady != 1 || readiness.Unknown != 1 || readiness.Total != 3 || readiness.StaleSlices != 1 || readiness.Slices != 2 {
		t.Fatalf("readiness counted duplicates or stale owners: %#v", readiness)
	}
	columns, rows := Table(serviceTestResource, collection, time.Now())
	if rows[0].Cells[slices.Index(columns, "Ready endpoints")] != "1/3 · 1 unknown" || len(collection.Objects[0].Object) != len(service.Object) {
		t.Fatal(columns, rows)
	}
}

func TestServiceReadinessSeparatesDeniedUnavailableEmptyAndExternalName(t *testing.T) {
	service := serviceFixture(t)
	for _, scenario := range []string{"denied", "unavailable", "empty", "external"} {
		t.Run(scenario, func(t *testing.T) {
			var requests atomic.Int32
			client := testClient(t, func(w http.ResponseWriter, request *http.Request) {
				requests.Add(1)
				if request.URL.Query().Get("labelSelector") != "kubernetes.io/service-name=checkout" {
					t.Errorf("summary not scoped to Service: %s", request.URL)
				}
				if scenario == "denied" {
					apiError(w, 403, "Forbidden")
					return
				}
				reply(w, objectList("EndpointSlice", "1"))
			})
			object := service.DeepCopy()
			resources := []kubecommon.Resource{sliceTestResource}
			if scenario == "unavailable" {
				resources = nil
			}
			if scenario == "external" {
				_ = unstructured.SetNestedField(object.Object, "ExternalName", "spec", "type")
			}
			sections, warning := client.SummarySupplement(context.Background(), serviceTestResource, *object, resources)
			value := findSummaryField(t, sections[0], "Ready / addresses")
			want := map[string]string{"denied": "Access denied", "unavailable": "API unavailable", "empty": "0/0", "external": "Not applicable"}[scenario]
			if value != want || ((scenario == "denied" || scenario == "unavailable") && warning == "") {
				t.Fatalf("state=%q warning=%q", value, warning)
			}
			if (scenario == "unavailable" || scenario == "external") && requests.Load() != 0 {
				t.Fatal("queried an unavailable or inapplicable endpoint API")
			}
		})
	}
}

func TestServiceReadinessBoundMarksPartialEvenWithoutContinue(t *testing.T) {
	service := serviceFixture(t)
	client := testClient(t, func(w http.ResponseWriter, request *http.Request) {
		objects := make([]map[string]any, maxReadinessSlices+1)
		for i := range objects {
			objects[i] = map[string]any{"metadata": map[string]any{"namespace": "shop", "name": fmt.Sprint(i), "labels": map[string]any{"kubernetes.io/service-name": "checkout"}}, "addressType": "IPv4", "endpoints": []any{map[string]any{"addresses": []any{fmt.Sprint(i)}, "conditions": map[string]any{"ready": true}}}}
		}
		reply(w, objectList("EndpointSlice", "1", objects...))
	})
	readiness := client.serviceEndpointReadiness(context.Background(), []unstructured.Unstructured{service}, sliceTestResource, true, "shop", "")["service-current"]
	if !readiness.Partial || readiness.Total != maxReadinessSlices || !strings.HasPrefix(readiness.Cell(), "Partial") {
		t.Fatal(readiness)
	}
}

func TestServicesRefreshEndpointReadinessWithoutServiceWatchEvents(t *testing.T) {
	service := serviceFixture(t)
	var reads atomic.Int32
	client := testClient(t, func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("watch") == "true" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			<-request.Context().Done()
			return
		}
		if strings.HasSuffix(request.URL.Path, "/services") {
			reply(w, objectList("Service", "unchanged", service.Object))
			return
		}
		ready := reads.Add(1) == 1
		reply(w, objectList("EndpointSlice", "1", map[string]any{"metadata": map[string]any{"namespace": "shop", "name": "endpoint", "labels": map[string]any{"kubernetes.io/service-name": "checkout"}}, "addressType": "IPv4", "endpoints": []any{map[string]any{"addresses": []any{"10.0.0.1"}, "conditions": map[string]any{"ready": ready}}}}))
	})
	client.catalog = []kubecommon.Resource{serviceTestResource, sliceTestResource}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var states []int
	client.Observe(ctx, serviceTestResource, "shop", "", func(snapshot Snapshot) {
		if snapshot.Collection != nil {
			states = append(states, snapshot.Collection.ServiceEndpoints["service-current"].Ready)
			if len(states) == 2 {
				cancel()
			}
		}
	})
	if len(states) != 2 || states[0] != 1 || states[1] != 0 {
		t.Fatalf("readiness remained stale while Service watch was idle: %v", states)
	}
}

func TestGatewayParameterAndListenerAttachmentReferences(t *testing.T) {
	gateway := summaryFixture(t, `{"apiVersion":"gateway.networking.k8s.io/v1","kind":"Gateway","metadata":{"name":"edge","namespace":"shop","uid":"gateway"},"spec":{"infrastructure":{"parametersRef":{"group":"example.io","kind":"GatewayConfig","name":"edge-config"}}}}`)
	route := summaryFixture(t, `{"metadata":{"name":"checkout","namespace":"shop","generation":3},"spec":{"parentRefs":[{"name":"edge","sectionName":"https"},{"name":"edge","sectionName":"http"}]},"status":{"parents":[{"parentRef":{"name":"edge","sectionName":"https"},"controllerName":"example.io/gateway","conditions":[{"type":"Accepted","status":"True","reason":"Accepted","observedGeneration":3},{"type":"ResolvedRefs","status":"False","reason":"BackendNotFound","observedGeneration":2}]},{"parentRef":{"name":"another","sectionName":"http"},"conditions":[{"type":"Accepted","status":"True"}]}]}}`)
	resource := kubecommon.Resource{Name: "gateways", Group: "gateway.networking.k8s.io", Kind: "Gateway", Version: "v1", Namespaced: true}
	section := findSummarySection(t, Summary(resource, gateway), "Infrastructure parameters")
	if findSummaryField(t, section, "Name") != "edge-config" || findSummaryField(t, section, "Namespace") != "shop" {
		t.Fatal(section)
	}
	relation := gatewayRouteRelation(route, resource, gateway)
	for _, want := range []string{"https: Accepted=True", "ResolvedRefs=False (BackendNotFound) [stale]", "http: Accepted=Not reported"} {
		if !strings.Contains(relation, want) {
			t.Fatalf("attachment condition lost %q: %s", want, relation)
		}
	}
	parameters := kubecommon.Resource{Name: "gatewayconfigs", Group: "example.io", Kind: "GatewayConfig", Version: "v1", Namespaced: true}
	refs, warning := (*Client)(nil).Related(context.Background(), resource, gateway, []kubecommon.Resource{parameters})
	if len(refs) != 1 || refs[0].Resource.Key() != parameters.Key() || refs[0].Namespace != "shop" || refs[0].Name != "edge-config" || warning != "" {
		t.Fatalf("parameter reference: %#v %s", refs, warning)
	}
}
