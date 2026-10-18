package monitor

import (
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kjson "k8s.io/apimachinery/pkg/util/json"
	"strings"
	"testing"
	"time"
)

func TestDiagnosticProjectionPreservesConfigurationAndMetrics(t *testing.T) {
	now := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		resource          kubecommon.Resource
		body, field, want string
	}{
		{kubecommon.Resource{Group: "apps", Version: "v1", Name: "deployments", Kind: "Deployment"}, `{"metadata":{"name":"web"},"spec":{"selector":{"matchLabels":{"app":"web"}},"template":{"spec":{"containers":[{"name":"web","image":"web:v2","env":[{"name":"TOKEN","value":"do-not-retain"}]}]}}}}`, "/spec/template/spec/containers/0/image", "web:v2"},
		{kubecommon.Resource{Group: "metrics.k8s.io", Version: "v1beta1", Name: "pods", Kind: "PodMetrics"}, `{"metadata":{"name":"web"},"timestamp":"2026-10-07T00:00:00Z","window":"30s","containers":[{"name":"web","usage":{"cpu":"900m","memory":"900Mi"}}]}`, "cpu_nanocores", "900000000"},
		{kubecommon.Resource{Group: "discovery.k8s.io", Version: "v1", Name: "endpointslices", Kind: "EndpointSlice"}, `{"metadata":{"name":"web"},"endpoints":[{"addresses":["10.0.0.1"],"conditions":{"ready":false},"targetRef":{"kind":"Pod","name":"web"}}]}`, "/endpoints/0/conditions/ready", "false"},
	} {
		var object map[string]any
		if err := kjson.Unmarshal([]byte(test.body), &object); err != nil {
			t.Fatal(err)
		}
		row, err := projectResource(test.resource, object, now)
		if err != nil {
			t.Fatal(err)
		}
		if row.Facts[test.field].String() != test.want {
			t.Fatal(test.field, row)
		}
		for _, value := range row.Facts {
			if strings.Contains(value.String(), "do-not-retain") {
				t.Fatal("credential retained")
			}
		}
		if test.resource.Group == "metrics.k8s.io" {
			if row.Facts["memory_bytes"].String() != "943718400" || row.Facts["fresh"].String() != "true" {
				t.Fatal(row)
			}
			stale, err := projectResource(test.resource, object, now.Add(2*time.Minute))
			if err != nil || stale.Facts["fresh"].String() != "false" {
				t.Fatal("sample freshness lost", stale, err)
			}
		}
	}
}
