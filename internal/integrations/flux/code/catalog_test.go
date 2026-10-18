package code

import (
	"strings"
	"testing"

	"github.com/simjay/plous/internal/integrations/catalog"
)

func TestHelmSourceResolutionPreservesFrameworkAndNamespace(t *testing.T) {
	docs := catalog.Documents(map[string][]byte{"apps.yaml": []byte(`apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata: {name: api, namespace: apps}
spec:
  chart:
    spec:
      chart: service
      version: 1.2.3
      sourceRef: {kind: HelmRepository, name: charts, namespace: platform}
---
apiVersion: source.toolkit.fluxcd.io/v1
kind: HelmRepository
metadata: {name: charts, namespace: platform}
spec: {url: https://expected.invalid}
---
apiVersion: source.toolkit.fluxcd.io/v1
kind: HelmRepository
metadata: {name: charts, namespace: apps}
spec: {url: https://wrong-namespace.invalid}
---
apiVersion: example.invalid/v1
kind: HelmRepository
metadata: {name: charts, namespace: platform}
spec: {url: https://wrong-framework.invalid}
`)})
	items := Discover(docs)
	var chart *catalog.Item
	for n := range items {
		if items[n].Technology == "helm" {
			chart = &items[n]
		}
	}
	if len(items) != 4 || chart == nil {
		t.Fatalf("incorrect catalog %#v", items)
	}
	if !strings.Contains(chart.Location, "https://expected.invalid") || strings.Contains(chart.Location, "wrong-") || chart.EntryID != "" || !chart.External {
		t.Fatalf("incorrect source resolution %#v", chart)
	}
}
