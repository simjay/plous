package code

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/simjay/plous/internal/integrations/helm/common"
)

func TestRenderUsesCapturedInputsWithoutClusterAccess(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	dir := t.TempDir()
	config := filepath.Join(dir, "kubeconfig")
	if err := os.WriteFile(config, []byte(fmt.Sprintf("apiVersion: v1\nkind: Config\ncurrent-context: trap\ncontexts:\n- name: trap\n  context: {cluster: trap}\nclusters:\n- name: trap\n  cluster: {server: %s}\n", server.URL)), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", config)
	t.Setenv("PATH", t.TempDir())
	files := map[string]string{
		"Chart.yaml":            "apiVersion: v2\nname: sample\nversion: 1.0.0\n",
		"values.yaml":           "message: original\n",
		"override.yaml":         "message: captured\n",
		"templates/config.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: {{ .Release.Name }}\n  namespace: {{ .Release.Namespace }}\ndata:\n  message: {{ .Values.message }}\n  live: {{ if lookup \"v1\" \"Secret\" \"default\" \"trap\" }}contacted{{ else }}absent{{ end }}\n",
	}
	for name, content := range files {
		file := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	data, err := (Engine{}).Render(context.Background(), common.TemplateRequest{ChartPath: dir, Release: "preview", Namespace: "isolated", ValuesFiles: []string{filepath.Join(dir, "override.yaml")}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"name: preview", "namespace: isolated", "message: captured", "live: absent"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("missing %q in %s", want, data)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("source rendering contacted Kubernetes %d times", requests.Load())
	}
}
