package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/simjay/plous/internal/app/monitor"
	helmcommon "github.com/simjay/plous/internal/integrations/helm/common"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func BenchmarkKubernetesTable(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			m := &model{explore: &monitor.ExploreState{Listed: true, Resource: kubecommon.Resource{Name: "pods", Kind: "Pod", Version: "v1", Namespaced: true}, Preference: monitor.ExplorePreference{Namespace: "apps"}}}
			for i := 0; i < count; i++ {
				m.explore.Collection.Objects = append(m.explore.Collection.Objects, unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": fmt.Sprintf("pod-%05d", i), "namespace": "apps", "uid": fmt.Sprint(i)}, "spec": map[string]any{"containers": []any{map[string]any{"name": "app", "image": "fixture:1"}}}, "status": map[string]any{"phase": "Running"}}})
			}
			m.exploreTableBody(100, 30)
			b.ReportAllocs()
			for b.Loop() {
				m.exploreTableBody(100, 30)
			}
		})
	}
}

func BenchmarkHelmInspector(b *testing.B) {
	for _, count := range []int{10, 100} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			var manifest strings.Builder
			for i := 0; i < count; i++ {
				fmt.Fprintf(&manifest, "---\napiVersion: apps/v1\nkind: Deployment\nmetadata: {name: web-%03d, namespace: apps}\nspec: {replicas: 2, template: {spec: {containers: [{name: web, image: fixture:1}]}}}\n", i)
			}
			h := &monitor.HelmState{Tab: 5, Detail: &helmtool.Snapshot{Name: "fixture", Namespace: "apps", Version: 1, Manifest: manifest.String(), Chart: helmcommon.Chart{Metadata: map[string]any{"name": "demo", "version": "1"}}}}
			m := &model{}
			m.helmDetailBody(h, 100, 30)
			b.ReportAllocs()
			for b.Loop() {
				m.helmDetailBody(h, 100, 30)
			}
		})
	}
}

func BenchmarkHelmCards(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			h := &monitor.HelmState{Preference: monitor.HelmPreference{Section: "Charts"}}
			for i := 0; i < count; i++ {
				h.Releases = append(h.Releases, helmtool.Release{Name: fmt.Sprintf("release-%04d", i), Namespace: "apps", Chart: fmt.Sprintf("chart-%02d", i%10), Version: "1.0", Revision: 1, Status: "deployed"})
			}
			monitor.HelmChartCards(h, 100, 30)
			b.ReportAllocs()
			for b.Loop() {
				monitor.HelmChartCards(h, 100, 30)
			}
		})
	}
}
