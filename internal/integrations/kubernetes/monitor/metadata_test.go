package monitor

import (
	"context"
	"net/http"
	"strings"
	"testing"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
)

func TestMetadataInventoryPagesAndReportsFailedContinuation(t *testing.T) {
	for _, denied := range []bool{false, true} {
		calls := 0
		client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.Method != "GET" || r.URL.Path != "/api/v1/secrets" || r.URL.Query().Get("labelSelector") != "owner=helm" || !strings.Contains(r.Header.Get("Accept"), "PartialObjectMetadataList") {
				t.Errorf("unexpected request: %s %s %s", r.Method, r.URL, r.Header.Get("Accept"))
			}
			if r.URL.Query().Get("continue") != "" && denied {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			page := map[string]any{"apiVersion": "meta.k8s.io/v1", "kind": "PartialObjectMetadataList", "metadata": map[string]any{}, "items": []any{map[string]any{"metadata": map[string]any{"name": "release", "namespace": "shop", "labels": map[string]any{"owner": "helm"}}}}}
			if r.URL.Query().Get("continue") == "" {
				page["metadata"] = map[string]any{"continue": "next"}
			}
			reply(w, page)
		})
		objects, partial, err := client.ListMetadata(context.Background(), kubecommon.Resource{Version: "v1", Name: "secrets", Namespaced: true}, "", "owner=helm")
		want := 2
		if denied {
			want = 1
		}
		if calls != 2 || len(objects) != want || partial != denied || (err != nil) != denied {
			t.Fatalf("denied=%t: %d rows, partial=%t, calls=%d, error=%v", denied, len(objects), partial, calls, err)
		}
	}
}
