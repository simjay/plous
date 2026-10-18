package code

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"strings"
	"sync"
	"testing"
)

func TestNativeHelmNamespacesValuesAndComponents(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root := sourceFiles(t, map[string]string{
		"kustomization.yaml":                     "resources: [dev, prod]\n",
		"dev/kustomization.yaml":                 "resources: [../base]\nnamespace: development\nnamePrefix: dev-\n",
		"prod/kustomization.yaml":                "resources: [../base]\nnamespace: production\nnamePrefix: prod-\n",
		"base/kustomization.yaml":                "helmCharts:\n- name: demo\n  releaseName: demo\n  valuesInline: {value: inline}\n  additionalValuesFiles: [extra.yaml]\ncomponents: [../component]\n",
		"base/extra.yaml":                        "extra: captured\n",
		"base/charts/demo/Chart.yaml":            "apiVersion: v2\nname: demo\nversion: 1.0.0\n",
		"base/charts/demo/values.yaml":           "value: default\n",
		"base/charts/demo/templates/config.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: settings\n  namespace: {{ .Release.Namespace }}\ndata:\n  namespace: {{ .Release.Namespace }}\n  value: {{ .Values.value }}\n  extra: {{ .Values.extra }}\n",
		"component/kustomization.yaml":           "apiVersion: kustomize.config.k8s.io/v1alpha1\nkind: Component\ncommonAnnotations: {component: applied}\n",
	})
	snapshot := fixtureInventory(t, root)
	files := maps.Clone(snapshot.Files)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			built, err := BuildHelm(context.Background(), snapshot, ".", nil)
			if err != nil {
				t.Error(err)
				return
			}
			if len(built.Resources) != 2 {
				t.Errorf("resources: %+v", built.Resources)
				return
			}
			for _, resource := range built.Resources {
				want := "development"
				if strings.HasPrefix(resource.Name, "prod-") {
					want = "production"
				}
				data := resource.Object["data"].(map[string]any)
				annotations := resource.Object["metadata"].(map[string]any)["annotations"].(map[string]any)
				if resource.Namespace != want || data["namespace"] != want || data["value"] != "inline" || data["extra"] != "captured" || annotations["component"] != "applied" {
					t.Errorf("incorrect composed render: %+v", resource.Object)
				}
			}
		})
	}
	wg.Wait()
	after := fixtureInventory(t, root)
	if !maps.EqualFunc(files, after.Files, bytes.Equal) {
		t.Fatal("build mutated original files")
	}
}

func TestNativeHelmDoesNotMoveBeforeBuiltinGenerators(t *testing.T) {
	root := sourceFiles(t, map[string]string{
		"kustomization.yaml":                "generatorOptions: {disableNameSuffixHash: true}\nconfigMapGenerator:\n- name: settings\n  behavior: merge\n  literals: [builtin=value]\nhelmCharts:\n- name: demo\n  releaseName: demo\n",
		"charts/demo/Chart.yaml":            "apiVersion: v2\nname: demo\nversion: 1.0.0\n",
		"charts/demo/values.yaml":           "{}\n",
		"charts/demo/templates/config.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: settings}\ndata: {chart: value}\n",
	})
	snapshot := fixtureInventory(t, root)
	if _, err := BuildHelm(context.Background(), snapshot, ".", nil); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatal("Helm ran before the builtin generator", err)
	}
}

func TestNativeBuildResetsCustomSchema(t *testing.T) {
	root := sourceFiles(t, map[string]string{
		"custom/kustomization.yaml":  "openapi: {path: schema.json}\nresources: [../base]\n",
		"custom/schema.json":         `{"swagger":"2.0","definitions":{}}`,
		"base/kustomization.yaml":    "resources: [deployment.yaml]\n",
		"base/deployment.yaml":       "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: demo}\nspec:\n  template:\n    spec:\n      containers:\n      - {name: one, image: one}\n      - {name: two, image: two}\n",
		"patched/kustomization.yaml": "resources: [../base]\npatches:\n- patch: |-\n    apiVersion: apps/v1\n    kind: Deployment\n    metadata: {name: demo}\n    spec:\n      template:\n        spec:\n          containers:\n          - {name: one, image: updated}\n",
	})
	snapshot := fixtureInventory(t, root)
	if _, err := Build(context.Background(), snapshot, "custom", nil); err != nil {
		t.Fatal(err)
	}
	built, err := Build(context.Background(), snapshot, "patched", nil)
	if err != nil {
		t.Fatal(err)
	}
	text := fmt.Sprint(built.Resources[0].Object)
	if !strings.Contains(text, "updated") || !strings.Contains(text, "name:two") {
		t.Fatal("custom schema leaked into another build", text)
	}
}
