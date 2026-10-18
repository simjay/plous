package monitor

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	apidiscovery "k8s.io/api/apidiscovery/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAggregatedDiscoveryUsesTwoRequestsAndPreservesCapabilities(t *testing.T) {
	var requests atomic.Int32
	groups := apidiscovery.APIGroupDiscoveryList{TypeMeta: metav1.TypeMeta{Kind: "APIGroupDiscoveryList", APIVersion: "apidiscovery.k8s.io/v2"}}
	for i := range 100 {
		group := fmt.Sprintf("fixture-%d.example.io", i)
		resource := apidiscovery.APIResourceDiscovery{
			Resource: "widgets", Scope: apidiscovery.ScopeNamespace,
			ResponseKind: &metav1.GroupVersionKind{Group: group, Version: "v1", Kind: "Widget"},
			Verbs:        []string{"get", "list", "watch"}, ShortNames: []string{fmt.Sprintf("w%d", i)},
			Subresources: []apidiscovery.APISubresourceDiscovery{{Subresource: "status", ResponseKind: &metav1.GroupVersionKind{Group: group, Version: "v1", Kind: "Widget"}, Verbs: []string{"get", "patch"}}},
		}
		fallback := resource
		fallback.Resource = "gadgets"
		fallback.ShortNames = nil
		groups.Items = append(groups.Items, apidiscovery.APIGroupDiscovery{
			ObjectMeta: metav1.ObjectMeta{Name: group},
			Versions: []apidiscovery.APIVersionDiscovery{
				{Version: "v1", Freshness: apidiscovery.DiscoveryFreshnessCurrent, Resources: []apidiscovery.APIResourceDiscovery{resource}},
				{Version: "v2", Freshness: apidiscovery.DiscoveryFreshnessCurrent, Resources: []apidiscovery.APIResourceDiscovery{fallback}},
				{Version: "v3", Freshness: apidiscovery.DiscoveryFreshnessStale, Resources: []apidiscovery.APIResourceDiscovery{resource}},
			},
		})
	}
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch r.URL.Path {
		case "/apis":
			if !strings.Contains(r.Header.Get("Accept"), "as=APIGroupDiscoveryList") {
				t.Error("request did not negotiate bulk discovery")
			}
			reply(w, groups)
		case "/api/v1":
			reply(w, metav1.APIResourceList{APIResources: []metav1.APIResource{{Name: "pods", Kind: "Pod", Verbs: []string{"list"}}}})
		default:
			t.Errorf("bulk discovery made a redundant request: %s", r.URL.Path)
			apiError(w, 404, "NotFound")
		}
	})
	resources, warning, err := client.Discover(t.Context())
	if err != nil || warning == "" || len(resources) != 201 || requests.Load() != 2 {
		t.Fatalf("resources=%d warning=%q error=%v requests=%d", len(resources), warning, err, requests.Load())
	}
	widget, ok := Find(resources, "w0")
	if !ok || widget.Version != "v1" || !widget.Namespaced || !widget.Supports("watch", "") || !widget.Supports("patch", "status") {
		t.Fatalf("lost advertised identity or capabilities: %+v", widget)
	}
	if gadget, ok := Find(resources, "gadgets.fixture-0.example.io"); !ok || gadget.Version != "v2" {
		t.Fatalf("resource in non-preferred version lost: %+v", gadget)
	}
}

func TestLegacyDiscoveryAvoidsResourceRateLimitAndBoundsConcurrency(t *testing.T) {
	var requests, active, peak atomic.Int32
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/apis" {
			var groups metav1.APIGroupList
			for i := range 48 {
				group := fmt.Sprintf("fixture-%d.example.io", i)
				groups.Groups = append(groups.Groups, metav1.APIGroup{Name: group, PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: group + "/v1"}})
			}
			reply(w, groups)
			return
		}
		count := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); count > old; old = peak.Load() {
			if peak.CompareAndSwap(old, count) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		reply(w, metav1.APIResourceList{APIResources: []metav1.APIResource{{Name: "widgets", Kind: "Widget", Verbs: []string{"list"}}}})
	})
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	resources, warning, err := client.Discover(ctx)
	if err != nil || warning != "" || len(resources) != 49 || requests.Load() != 50 {
		t.Fatalf("legacy discovery stalled: resources=%d warning=%q error=%v requests=%d", len(resources), warning, err, requests.Load())
	}
	if peak.Load() > 6 || peak.Load() < 2 {
		t.Fatalf("discovery concurrency=%d", peak.Load())
	}
}
