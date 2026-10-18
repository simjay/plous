package monitor

import (
	"fmt"
	"strings"
	"testing"
	"time"

	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utiljson "k8s.io/apimachinery/pkg/util/json"
)

func fixture(t *testing.T, data string) unstructured.Unstructured {
	t.Helper()
	var object unstructured.Unstructured
	if err := utiljson.Unmarshal([]byte(data), &object.Object); err != nil {
		t.Fatal(err)
	}
	return object
}

func field(item gitops.Item, label string) string {
	for _, f := range item.Fields {
		if f.Label == label {
			return f.Value
		}
	}
	return ""
}

func TestFluxConditionGenerationIsIndependentOfTopLevelObservation(t *testing.T) {
	tests := []struct {
		name, conditions, status, contains string
		suspend, stale                     bool
	}{
		{"current failure during retry", `{"type":"Ready","status":"False","observedGeneration":5,"reason":"HealthCheckFailed","message":"Deployment timed out"},{"type":"Reconciling","status":"True","observedGeneration":5,"reason":"ProgressingWithRetry"}`, "Not ready", "Deployment timed out", false, false},
		{"stale ready", `{"type":"Ready","status":"True","observedGeneration":4,"message":"old success"}`, "Awaiting status", "generation 5", false, true},
		{"unversioned ready", `{"type":"Ready","status":"True","message":"old success"}`, "Awaiting status", "generation 5", false, true},
		{"current ready", `{"type":"Ready","status":"True","observedGeneration":5,"message":"applied"}`, "Ready", "applied", false, false},
		{"current reconciliation replaces stale failure", `{"type":"Ready","status":"False","observedGeneration":4,"message":"old failure"},{"type":"Reconciling","status":"True","observedGeneration":5,"message":"progressing"}`, "Reconciling", "progressing", false, false},
		{"current stalled", `{"type":"Stalled","status":"True","observedGeneration":5,"message":"invalid configuration"}`, "Stalled", "invalid configuration", false, false},
		{"suspended retains condition", `{"type":"Ready","status":"False","observedGeneration":5,"message":"prior failure"}`, "Suspended", "prior failure", true, false},
		{"missing status", ``, "Unknown", "not reported", false, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			object := fixture(t, fmt.Sprintf(`{"apiVersion":"kustomize.toolkit.fluxcd.io/v1","kind":"Kustomization","metadata":{"name":"platform","namespace":"flux-system","generation":5},"spec":{"suspend":%t},"status":{"observedGeneration":3,"conditions":[%s]}}`, test.suspend, test.conditions))
			item := ParseFlux(object, time.Now())
			if item.Status != test.status || item.Stale != test.stale || !strings.Contains(item.Reason, test.contains) {
				t.Fatalf("status=%q stale=%t reason=%q", item.Status, item.Stale, item.Reason)
			}
			if test.suspend && len(item.Conditions) != 1 {
				t.Fatal("suspension discarded previous conditions")
			}
		})
	}
}

func TestFluxReferencesInventoryAndRemoteDestination(t *testing.T) {
	object := fixture(t, `{"apiVersion":"kustomize.toolkit.fluxcd.io/v1","kind":"Kustomization","metadata":{"name":"platform","namespace":"flux-system","generation":2},"spec":{"targetNamespace":"shop","kubeConfig":{"secretRef":{"name":"private-kubeconfig"}},"sourceRef":{"kind":"GitRepository","name":"config","namespace":"shared"},"dependsOn":[{"name":"base"},{"name":"network","namespace":"infra"}]},"status":{"lastAppliedRevision":"main@sha1:older","lastAttemptedRevision":"main@sha1:newer","inventory":{"entries":[{"id":"shop_web_apps_Deployment","v":"v1"},{"id":"_shop__Namespace","v":"v1"},{"id":"invalid"}]}}}`)
	item := ParseFlux(object, time.Now())
	if !item.Destination.Remote || item.Destination.Namespace != "shop" || len(item.Sources) != 1 || item.Sources[0].Ref.Namespace != "shared" {
		t.Fatalf("destination/source references lost: %#v", item)
	}
	if len(item.Dependencies) != 2 || item.Dependencies[0].Namespace != "flux-system" || item.Dependencies[1].Namespace != "infra" {
		t.Fatalf("dependency namespace defaults lost: %#v", item.Dependencies)
	}
	if !item.InventoryReported || len(item.Resources) != 2 || item.Resources[1].Ref.Namespace != "" || item.Resources[1].Ref.Group != "" || item.Resources[0].Health != "Not reported" {
		t.Fatalf("inventory parsed incorrectly: %#v", item.Resources)
	}
	if field(item, "Last applied revision") != "main@sha1:older" || field(item, "Last attempted revision") != "main@sha1:newer" || field(item, "Inventory note") == "" {
		t.Fatalf("revision/inventory coverage lost: %#v", item.Fields)
	}
	if strings.Contains(fmt.Sprint(item), "private-kubeconfig") {
		t.Fatal("kubeconfig credential reference leaked into read model")
	}
}

func TestHelmReleaseChartAndSourceMetadataDoNotImplyGitRevision(t *testing.T) {
	object := fixture(t, `{"apiVersion":"helm.toolkit.fluxcd.io/v2","kind":"HelmRelease","metadata":{"name":"metrics","namespace":"monitoring"},"spec":{"chart":{"spec":{"chart":"metrics","version":"1.*","sourceRef":{"kind":"HelmRepository","name":"charts"}}}},"status":{"lastAttemptedRevision":"1.4.2","helmChart":"monitoring/monitoring-metrics","history":[{"name":"metrics","status":"deployed","chartVersion":"1.4.2"}]}}`)
	item := ParseFlux(object, time.Now())
	if field(item, "Last attempted revision") != "" || field(item, "Last attempted chart revision") != "1.4.2" || len(item.Sources) != 2 || item.Sources[1].Ref.Kind != "HelmChart" || item.InventoryReported {
		t.Fatalf("Helm metadata incorrect: %#v", item)
	}
	source := fixture(t, `{"apiVersion":"source.toolkit.fluxcd.io/v1","kind":"GitRepository","metadata":{"name":"config","namespace":"flux-system","generation":1},"spec":{"url":"https://git.example/config","secretRef":{"name":"token"},"ref":{"branch":"main"}},"status":{"conditions":[{"type":"Ready","status":"False","observedGeneration":1,"message":"fetch failed"}],"artifact":{"url":"https://artifact.example/private","revision":"main@sha1:old","digest":"sha256:example"}}}`)
	metadata := ParseFlux(source, time.Now())
	if metadata.Status != "Not ready" || field(metadata, "Fetched revision") != "main@sha1:old" || strings.Contains(fmt.Sprint(metadata), "artifact.example") || strings.Contains(fmt.Sprint(metadata), "token") {
		t.Fatalf("source exposes content/credentials or loses independent status: %#v", metadata)
	}
}
