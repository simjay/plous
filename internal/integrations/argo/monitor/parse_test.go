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

func TestApplicationKeepsSyncHealthAndRevisionMeaningSeparate(t *testing.T) {
	object := fixture(t, `{
		"apiVersion":"argoproj.io/v1alpha1","kind":"Application",
		"metadata":{"name":"payments","namespace":"argocd"},
		"spec":{"project":"default","destination":{"server":"https://other.example","namespace":"shop"},"sources":[{"repoURL":"https://user:password@git.example/repo?token=private","path":"apps","targetRevision":"main"},{"repoURL":"https://charts.example","chart":"metrics","targetRevision":"1.*"}]},
		"status":{"sync":{"status":"Synced","revisions":["compared-a","compared-b"]},"health":{"status":"Degraded"},"reconciledAt":"2026-10-04T12:00:00Z","resources":[{"group":"apps","version":"v1","kind":"Deployment","name":"payments","namespace":"shop","status":"Synced","health":{"status":"Degraded","message":"progress deadline exceeded"}},{"version":"v1","kind":"ConfigMap","name":"config","namespace":"shop","status":"Synced"}],"operationState":{"phase":"Succeeded","syncResult":{"revisions":["deployed-a","deployed-b"]}}}
	}`)
	readAt := time.Date(2026, 10, 4, 13, 0, 0, 0, time.UTC)
	item := ParseApplication(object, readAt)
	if item.Sync != "Synced" || item.Health != "Degraded" || item.Reason != "Deployment/payments: progress deadline exceeded" {
		t.Fatalf("status was conflated: %#v", item)
	}
	if item.Resources[1].Health != "Not reported" || !item.InventoryReported || item.Destination.Server != "https://other.example" {
		t.Fatalf("resource coverage or destination lost: %#v", item)
	}
	if field(item, "Compared revision 1") != "compared-a" || field(item, "Successful operation revision 1") != "deployed-a" {
		t.Fatalf("revision meaning lost: %#v", item.Fields)
	}
	if item.ObservedAt.Equal(readAt) || item.ReadAt != readAt || item.ObservedLabel != "Argo reconciled" {
		t.Fatal("controller observation was replaced with read time")
	}
	if item.Sources[0].URL != "https://git.example/repo" || strings.Contains(fmt.Sprint(item), "password") || strings.Contains(fmt.Sprint(item), "private") {
		t.Fatal("credential-bearing URL was not redacted")
	}
}

func TestApplicationFailedOperationDoesNotClaimSuccessfulRevision(t *testing.T) {
	object := fixture(t, `{"apiVersion":"argoproj.io/v1alpha1","kind":"Application","metadata":{"name":"worker","namespace":"argocd"},"status":{"sync":{"status":"OutOfSync","revision":"compared"},"health":{"status":"Healthy"},"operationState":{"phase":"Failed","message":"apply failed","syncResult":{"revision":"attempted"}}}}`)
	item := ParseApplication(object, time.Now())
	if item.Sync != "OutOfSync" || item.Health != "Healthy" || item.Reason != "apply failed" || field(item, "Successful operation revision") != "" || item.InventoryReported {
		t.Fatalf("failed operation or missing coverage misreported: %#v", item)
	}
}

func TestApplicationSetErrorTakesPriorityOverGeneratedParameters(t *testing.T) {
	object := fixture(t, `{"apiVersion":"argoproj.io/v1alpha1","kind":"ApplicationSet","metadata":{"name":"fleet","namespace":"argocd"},"spec":{"generators":[{"git":{}},{"matrix":{}}]},"status":{"conditions":[{"type":"ParametersGenerated","status":"True"},{"type":"ResourcesUpToDate","status":"True"},{"type":"ErrorOccurred","status":"True","reason":"RenderError","message":"template failed","lastTransitionTime":"2026-10-01T12:00:00Z"}]}}`)
	item := Parse(object, time.Now())
	if item.Status != "Failed" || item.Reason != "template failed" || item.ObservedAt.IsZero() || field(item, "Generator") != "git, matrix" || item.Health != "" {
		t.Fatalf("ApplicationSet condition misreported: %#v", item)
	}
}
