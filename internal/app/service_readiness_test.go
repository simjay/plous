package app

import (
	"testing"
	"time"

	"github.com/simjay/plous/internal/kubernetes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

func TestServiceReadinessWatchRefreshesOnlyCapturedUIDAndNewerObservation(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	e.resource = kubernetes.Resource{Name: "services", Kind: "Service", Version: "v1", Namespaced: true}
	e.detail = &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Service", "metadata": map[string]any{"name": "checkout", "namespace": "shop", "uid": "captured"}}}
	e.detailValid = true
	observed := time.Now()
	update := func(uid string, ready int, at time.Time) {
		object := e.detail.DeepCopy()
		object.SetUID(types.UID(uid))
		collection := kubernetes.Collection{Objects: []unstructured.Unstructured{*object}, ServiceEndpoints: map[string]kubernetes.EndpointReadiness{uid: {Ready: ready, Total: 2, ObservedAt: at}}}
		m.updateExploreMessage(exploreUpdate{stream: e.stream, snapshot: kubernetes.Snapshot{Collection: &collection}})
	}
	update("captured", 1, observed)
	if e.supplement[0].Fields[0].Value != "1/2" {
		t.Fatal("Service watch did not refresh endpoint readiness")
	}
	update("replacement", 2, observed.Add(time.Second))
	if e.detail.GetUID() != "captured" || e.supplement[0].Fields[0].Value != "1/2" {
		t.Fatal("replacement Service retargeted the open detail supplement")
	}
	update("captured", 0, observed.Add(-time.Second))
	if e.supplement[0].Fields[0].Value != "1/2" {
		t.Fatal("older list snapshot overwrote newer readiness")
	}
	update("captured", 2, observed.Add(2*time.Second))
	if e.supplement[0].Fields[0].Value != "2/2" {
		t.Fatal("newer readiness observation was not displayed")
	}
	run := &exploreDetailRun{}
	e.detailRun = run
	older, _ := kubernetes.EndpointReadinessSummary(kubernetes.EndpointReadiness{Ready: 0, Total: 2, ObservedAt: observed})
	m.updateExploreMessage(exploreDetailResult{run: run, object: e.detail.DeepCopy(), supplement: older, supplementAt: observed})
	if e.supplement[0].Fields[0].Value != "2/2" {
		t.Fatal("an older detail query overwrote newer watch readiness")
	}
}
