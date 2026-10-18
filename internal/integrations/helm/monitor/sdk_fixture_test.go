package monitor

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	helmcommon "github.com/simjay/plous/internal/integrations/helm/common"
)

func TestHelmSDKReadsSyntheticReleaseStorage(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	for _, driver := range []string{"secret", "configmap"} {
		t.Run(driver, func(t *testing.T) {
			testHelmSDKStorage(t, driver)
		})
	}
}

func testHelmSDKStorage(t *testing.T, driver string) {
	snapshot := Snapshot{Name: "shop", Namespace: "apps", Version: 2, Chart: helmcommon.Chart{Metadata: map[string]any{"apiVersion": "v2", "name": "demo", "version": "1.0.0"}, Values: map[string]any{"replicas": 1}}, Config: map[string]any{"replicas": 2}, Manifest: "apiVersion: v1\nkind: Service\nmetadata: {name: shop}\n"}
	snapshot.Info.Status = "deployed"
	snapshot.Info.LastDeployed = "2026-10-01T10:00:00Z"
	data, _ := json.Marshal(snapshot)
	var encoded bytes.Buffer
	gz := gzip.NewWriter(&encoded)
	_, _ = gz.Write(data)
	_ = gz.Close()
	payload := base64.StdEncoding.EncodeToString(encoded.Bytes())
	kind, collection := "ConfigMap", "configmaps"
	if driver == "secret" {
		payload = base64.StdEncoding.EncodeToString([]byte(payload))
		kind, collection = "Secret", "secrets"
	}
	record := map[string]any{"apiVersion": "v1", "kind": kind, "metadata": map[string]any{"name": "sh.helm.release.v1.shop.v2", "namespace": "apps", "labels": map[string]string{"name": "shop", "owner": "helm", "status": "deployed", "version": "2"}}, "type": "helm.sh/release.v1", "data": map[string]string{"release": payload}}
	var mutations atomic.Int32
	var corrupt atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != "GET" {
			mutations.Add(1)
			http.Error(w, "writes forbidden", http.StatusForbidden)
			return
		}
		switch r.URL.Path {
		case "/version":
			_ = json.NewEncoder(w).Encode(map[string]string{"major": "1", "minor": "34", "gitVersion": "v1.34.0"})
		case "/api/v1/namespaces/apps/" + collection:
			_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": "v1", "kind": kind + "List", "items": []any{record}})
		case "/api/v1/namespaces/apps/" + collection + "/sh.helm.release.v1.shop.v2":
			if corrupt.Load() {
				_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": "v1", "kind": kind, "metadata": record["metadata"]})
				return
			}
			_ = json.NewEncoder(w).Encode(record)
		default:
			http.Error(w, "fixture endpoint unavailable: "+r.URL.Path, 404)
		}
	}))
	defer server.Close()
	configPath := filepath.Join(t.TempDir(), "config")
	body := fmt.Sprintf("apiVersion: v1\nkind: Config\nclusters:\n- name: synthetic\n  cluster: {server: %q}\ncontexts:\n- name: staging\n  context: {cluster: synthetic, namespace: apps}\ncurrent-context: unrelated\n", server.URL)
	if err := os.WriteFile(configPath, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	client, err := New(Config{ConnectionID: "fixture", Kubeconfig: configPath, Context: "staging", Driver: driver})
	if err != nil {
		t.Fatal(err)
	}
	releases, err := client.List(context.Background(), "apps")
	if err != nil || len(releases) != 1 || releases[0].Chart != "demo" || releases[0].Error != "" {
		t.Fatalf("list %+v %v", releases, err)
	}
	got, err := client.Snapshot(context.Background(), "shop", "apps", 2)
	if err != nil {
		t.Fatal(err)
	}
	if got.Effective["replicas"] != json.Number("2") || got.Release().Version != "1.0.0" || len(got.History) != 1 {
		t.Fatalf("snapshot %+v", got)
	}
	corrupt.Store(true)
	if _, err := client.Snapshot(context.Background(), "shop", "apps", 2); err == nil || driver == "secret" && !strings.Contains(err.Error(), "could not be decoded") {
		t.Fatalf("malformed storage did not produce an inspectable read error: %v", err)
	}
	after, _ := os.ReadFile(configPath)
	if !bytes.Equal(after, []byte(body)) || mutations.Load() != 0 {
		t.Fatal("read mutated kubeconfig or provider")
	}
	if !strings.Contains(got.Manifest, "kind: Service") {
		t.Fatal("manifest lost")
	}
}
