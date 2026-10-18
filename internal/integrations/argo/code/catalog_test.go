package code

import (
	"strings"
	"testing"

	"github.com/simjay/plous/internal/integrations/catalog"
)

func TestApplicationSetChartDiscoveryUsesTemplateAndSafeSource(t *testing.T) {
	docs := catalog.Documents(map[string][]byte{"apps.yaml": []byte(`apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata: {name: environments}
spec:
  template:
    spec:
      source:
        repoURL: https://user:private@example.invalid/charts?token=private
        chart: service
        targetRevision: 1.2.3
---
apiVersion: example.invalid/v1
kind: Application
metadata: {name: unrelated}
`)})
	items := Discover(docs)
	if len(items) != 2 {
		t.Fatalf("expected chart and ApplicationSet, got %#v", items)
	}
	chart, app := items[0], items[1]
	if chart.Technology != "helm" || chart.Version != "1.2.3" || chart.Name != "service" || !chart.External || chart.EntryID != "" {
		t.Fatalf("incorrect chart %#v", chart)
	}
	if app.Technology != "argo" || app.Kind != "ApplicationSet" {
		t.Fatalf("incorrect Argo item %#v", app)
	}
	if strings.Contains(chart.Location, "private") || chart.Location != "https://example.invalid/charts" {
		t.Fatalf("unsafe source URL %s", chart.Location)
	}
}
