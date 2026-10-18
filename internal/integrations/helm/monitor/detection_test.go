package monitor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"k8s.io/client-go/rest"
)

func TestDetectHelmReleaseMetadata(t *testing.T) {
	for _, test := range []struct {
		name, driver, found, want       string
		denied, unavailable, wrongOwner bool
	}{
		{name: "absent"},
		{name: "secret", found: "secrets", want: "secret"},
		{name: "configmap", found: "configmaps", want: "configmap"},
		{name: "explicit driver", driver: "configmap", found: "configmaps", want: "configmap"},
		{name: "other driver excluded", driver: "configmap", found: "secrets"},
		{name: "namespace permission", found: "secrets", want: "secret", denied: true},
		{name: "denial is unknown", denied: true},
		{name: "unavailable is unknown", unavailable: true},
		{name: "unrelated object", found: "secrets", wrongOwner: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var requests []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.URL.Path)
				if r.Method != "GET" || r.URL.Query().Get("limit") != "1" || r.URL.Query().Get("labelSelector") != "owner=helm" || !strings.Contains(r.Header.Get("Accept"), "as=PartialObjectMetadataList") {
					t.Errorf("unbounded or non-metadata detection: %s %s %s", r.Method, r.URL, r.Header.Get("Accept"))
				}
				if test.driver == "configmap" && strings.HasSuffix(r.URL.Path, "/secrets") {
					t.Error("ignored explicit driver")
				}
				w.Header().Set("Content-Type", "application/json")
				if test.unavailable || test.denied && !strings.Contains(r.URL.Path, "/namespaces/apps/") {
					code, reason := 403, "Forbidden"
					if test.unavailable {
						code, reason = 503, "ServiceUnavailable"
					}
					w.WriteHeader(code)
					json.NewEncoder(w).Encode(map[string]any{"apiVersion": "v1", "kind": "Status", "status": "Failure", "reason": reason, "code": code})
					return
				}
				items := []any{}
				if test.found != "" && strings.HasSuffix(r.URL.Path, "/"+test.found) {
					owner := "helm"
					if test.wrongOwner {
						owner = "someone-else"
					}
					items = append(items, map[string]any{"metadata": map[string]any{"name": "sh.helm.release.v1.shop.v1", "namespace": "apps", "labels": map[string]string{"owner": owner}}})
				}
				json.NewEncoder(w).Encode(map[string]any{"apiVersion": "meta.k8s.io/v1", "kind": "PartialObjectMetadataList", "items": items})
			}))
			defer server.Close()
			client, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			client.Namespace = "apps"
			got, err := Detect(t.Context(), client, test.driver)
			if got != test.want {
				t.Fatalf("driver = %q, want %q (%v)", got, test.want, err)
			}
			unknown := test.want == "" && (test.denied || test.unavailable)
			if (err != nil) != unknown {
				t.Fatalf("error = %v, unknown = %v", err, unknown)
			}
			if len(requests) == 0 || len(requests) > 4 {
				t.Fatalf("unexpected request count: %v", requests)
			}
		})
	}
}
