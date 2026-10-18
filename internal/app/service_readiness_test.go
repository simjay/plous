package app

import (
	"testing"
	"time"

	"github.com/simjay/plous/internal/app/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

func TestServiceReadinessWatchRefreshesOnlyCapturedUIDAndNewerObservation(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	e.Resource = kubecommon.Resource{Name: "services", Kind: "Service", Version: "v1", Namespaced: true}
	e.Detail = &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Service", "metadata": map[string]any{"name": "checkout", "namespace": "shop", "uid": "captured"}}}
	e.DetailValid = true
	observed := time.Now()
	update := func(uid string, ready int, at time.Time) {
		object := e.Detail.DeepCopy()
		object.SetUID(types.UID(uid))
		collection := kubernetes.Collection{Objects: []unstructured.Unstructured{*object}, ServiceEndpoints: map[string]kubernetes.EndpointReadiness{uid: {Ready: ready, Total: 2, ObservedAt: at}}}
		m.updateExploreMessage(monitor.ExploreUpdate{Stream: e.Stream, Snapshot: kubernetes.Snapshot{Collection: &collection}})
	}
	update("captured", 1, observed)
	if e.Supplement[0].Fields[0].Value != "1/2" {
		t.Fatal("Service watch did not refresh endpoint readiness")
	}
	update("replacement", 2, observed.Add(time.Second))
	if e.Detail.GetUID() != "captured" || e.Supplement[0].Fields[0].Value != "1/2" {
		t.Fatal("replacement Service retargeted the open detail supplement")
	}
	update("captured", 0, observed.Add(-time.Second))
	if e.Supplement[0].Fields[0].Value != "1/2" {
		t.Fatal("older list snapshot overwrote newer readiness")
	}
	update("captured", 2, observed.Add(2*time.Second))
	if e.Supplement[0].Fields[0].Value != "2/2" {
		t.Fatal("newer readiness observation was not displayed")
	}
	run := &monitor.ExploreDetailRun{}
	e.DetailRun = run
	older, _ := kubernetes.EndpointReadinessSummary(kubernetes.EndpointReadiness{Ready: 0, Total: 2, ObservedAt: observed})
	m.updateExploreMessage(monitor.ExploreDetailResult{Run: run, Object: e.Detail.DeepCopy(), Supplement: older, SupplementAt: observed})
	if e.Supplement[0].Fields[0].Value != "2/2" {
		t.Fatal("an older detail query overwrote newer watch readiness")
	}
}
