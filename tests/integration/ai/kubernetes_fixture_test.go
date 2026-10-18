package integration

import (
	"fmt"
	agent "github.com/simjay/plous/internal/ai"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func scopedKubernetesFixture(t *testing.T) (*agent.Connection, *atomic.Int32) {
	t.Helper()
	requests := &atomic.Int32{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "Bearer fixture-kube-token" {
			t.Error("fixture did not load the selected Kubernetes credential")
		}
		if r.Method != http.MethodGet {
			t.Errorf("assistant used a write method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1":
			fmt.Fprint(w, `{"apiVersion":"v1","kind":"APIResourceList","resources":[{"name":"pods","kind":"Pod","namespaced":true,"verbs":["get","list"]}]}`)
		case "/api/v1/pods":
			if r.URL.Query().Get("limit") != "50" {
				t.Error("all-namespace list lost its bound", r.URL)
			}
			fmt.Fprint(w, `{"apiVersion":"v1","kind":"PodList","metadata":{"continue":"next-page"},"items":[{"metadata":{"name":"checkout","namespace":"payments"}},{"metadata":{"name":"worker","namespace":"jobs"}}]}`)
		case "/api/v1/namespaces/payments/pods/checkout":
			fmt.Fprint(w, `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"checkout","namespace":"payments"},"status":{"phase":"Running"},"debug":"fixture-kube-token","nested":[{"kind":"Secret","data":{"token":"private-payload"}}]}`)
		default:
			t.Errorf("request escaped the captured namespace: %s", r.URL)
			http.Error(w, "wrong scope", http.StatusForbidden)
		}
	}))
	t.Cleanup(server.Close)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("assistant used kubeconfig current-context instead of the captured context")
		http.Error(w, "wrong context", http.StatusForbidden)
	}))
	t.Cleanup(other.Close)
	path := filepath.Join(t.TempDir(), "config")
	config := fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: other
contexts:
- name: captured
  context: {cluster: allowed, user: fixture, namespace: payments}
- name: other
  context: {cluster: other, user: fixture, namespace: forbidden}
clusters:
- name: allowed
  cluster: {server: %q, insecure-skip-tls-verify: true}
- name: other
  cluster: {server: %q}
users:
- name: fixture
  user: {token: fixture-kube-token}
`, server.URL, other.URL)
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != config {
			t.Errorf("assistant changed the kubeconfig: %v", err)
		}
	})
	return &agent.Connection{Tool: "kubernetes", Path: path, Context: "captured", ConnectionID: "captured"}, requests
}
