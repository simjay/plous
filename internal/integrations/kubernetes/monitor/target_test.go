package monitor

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
)

func TestCapturedClientCannotFollowReplacedContext(t *testing.T) {
	var first, replacement atomic.Int32
	serve := func(count *atomic.Int32) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count.Add(1)
			if r.Method != "GET" {
				t.Errorf("unexpected mutation: %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"selected","namespace":"apps","uid":"one"}}`)
		}))
	}
	a, b := serve(&first), serve(&replacement)
	defer a.Close()
	defer b.Close()
	path := filepath.Join(t.TempDir(), "config")
	write := func(server string) {
		t.Helper()
		data := fmt.Sprintf("apiVersion: v1\nkind: Config\ncurrent-context: temporary\ncontexts:\n- name: temporary\n  context: {cluster: temporary, namespace: apps}\nclusters:\n- name: temporary\n  cluster: {server: '%s'}\n", server)
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(a.URL)
	target, err := kubecommon.ResolveTarget(path, "", "")
	if err != nil {
		t.Fatal(err)
	}
	write(b.URL)
	client, err := NewTarget(target, "apps")
	if err != nil {
		t.Fatal(err)
	}
	resource := kubecommon.Resource{Name: "pods", Kind: "Pod", Version: "v1", Namespaced: true}
	object, err := client.Get(t.Context(), resource, "apps", "selected")
	if err != nil {
		t.Fatal(err)
	}
	if first.Load() != 1 || replacement.Load() != 0 {
		t.Fatal("read followed changed configuration")
	}
	if err = client.Delete(t.Context(), resource, object); err == nil {
		t.Fatal("write accepted a replaced target")
	}
	if first.Load() != 1 || replacement.Load() != 0 {
		t.Fatal("rejected write reached a server")
	}
}
