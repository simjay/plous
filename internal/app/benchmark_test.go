package app

import (
	"fmt"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func BenchmarkYAMLPreparation(b *testing.B) {
	data := map[string]any{}
	for i := range 1000 {
		data[fmt.Sprintf("key-%04d", i)] = strings.Repeat("fixture ", 8)
	}
	m := &model{explore: &exploreState{detail: &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "fixture", "uid": "fixture", "resourceVersion": "1"}, "data": data,
	}}}}
	m.syncYAMLDocument()
	b.ReportAllocs()
	for b.Loop() {
		m.syncYAMLDocument()
	}
}
