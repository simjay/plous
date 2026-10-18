package monitor

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSDKChartRepositoryWithoutExecutables(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Chdir(t.TempDir())
	if err := os.Mkdir("demo", 0700); err != nil {
		t.Fatal(err)
	}
	artifact := chartFixture(t)
	var requests atomic.Int32
	var address string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/index.yaml" {
			fmt.Fprintf(w, "apiVersion: v1\nentries:\n  demo:\n  - apiVersion: v2\n    name: demo\n    version: 1.0.0\n    urls: [demo.tgz]\n")
		} else if r.URL.Path == "/demo.tgz" {
			w.Write(artifact.Archive)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	address = server.URL
	client := &Client{Runner: SDKRunner{}}
	got, err := client.Acquire(context.Background(), "demo", address, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got.Digest != artifact.Digest || requests.Load() != 2 {
		t.Fatal("explicit repository was shadowed by the local directory", requests.Load())
	}
	if _, err := client.Acquire(t.Context(), "./missing", "", "1.0.0"); err == nil || requests.Load() != 2 {
		t.Fatal("missing local path reached remote acquisition", err)
	}
	dir := t.TempDir()
	config := filepath.Join(dir, "repositories.yaml")
	index := filepath.Join(dir, "enterprise-index.yaml")
	os.WriteFile(config, []byte("apiVersion: v1\nrepositories:\n- name: enterprise\n  url: "+address+"\n"), 0600)
	os.WriteFile(index, []byte("apiVersion: v1\nentries:\n  demo:\n  - apiVersion: v2\n    name: demo\n    version: 1.0.0\n    description: fixture\n    urls: [demo.tgz]\n"), 0600)
	client.Config = Config{RepositoryConfig: config, RepositoryCache: dir}
	got, err = client.Acquire(context.Background(), "enterprise/demo", "", "1.0.0")
	if err != nil || got.Digest != artifact.Digest {
		t.Fatal("configured chart acquisition failed", err)
	}
}

func TestSDKOCIUsesNativeInlineAuthentication(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	artifact := chartFixture(t)
	config := []byte(`{"name":"demo","version":"1.0.0"}`)
	digest := func(data []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(data)) }
	chartDigest, configDigest := digest(artifact.Archive), digest(config)
	manifest, _ := json.Marshal(map[string]any{"schemaVersion": 2, "mediaType": "application/vnd.oci.image.manifest.v1+json",
		"config": map[string]any{"mediaType": "application/vnd.cncf.helm.config.v1+json", "size": len(config), "digest": configDigest},
		"layers": []any{map[string]any{"mediaType": "application/vnd.cncf.helm.chart.content.v1.tar+gzip", "size": len(artifact.Archive), "digest": chartDigest}}})
	var authorized atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "fixture" || password != "private-token" {
			w.Header().Set("WWW-Authenticate", `Basic realm="fixture"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		authorized.Store(true)
		var data []byte
		switch {
		case strings.Contains(r.URL.Path, "/manifests/"):
			data = manifest
			w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
		case strings.HasSuffix(r.URL.Path, chartDigest):
			data = artifact.Archive
		case strings.HasSuffix(r.URL.Path, configDigest):
			data = config
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Docker-Content-Digest", digest(data))
		w.Header().Set("Content-Length", fmt.Sprint(len(data)))
		if r.Method != "HEAD" {
			w.Write(data)
		}
	}))
	defer server.Close()
	original := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = original })
	dir := t.TempDir()
	registryConfig := filepath.Join(dir, "registry.json")
	host := strings.TrimPrefix(server.URL, "https://")
	data, _ := json.Marshal(map[string]any{"auths": map[string]any{host: map[string]string{"username": "fixture", "password": "private-token"}}, "credsStore": "must-not-run"})
	os.WriteFile(registryConfig, data, 0600)
	client := &Client{Config: Config{RegistryConfig: registryConfig}, Runner: SDKRunner{}}

	for _, suffix := range []string{"", ":1.0.0", "@" + digest(manifest)} {
		got, err := client.Acquire(context.Background(), "oci://"+host+"/demo"+suffix, "", "1.0.0")
		if err != nil {
			t.Fatal(suffix, err)
		}
		if !authorized.Load() || got.Digest != artifact.Digest {
			t.Fatal("OCI chart or authentication mismatch")
		}
	}

	after, _ := os.ReadFile(registryConfig)
	if string(after) != string(data) {
		t.Fatal("registry configuration changed")
	}
}
