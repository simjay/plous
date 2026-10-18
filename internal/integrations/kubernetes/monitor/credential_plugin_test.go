package monitor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// The test executable stands in for an external credential plugin. It never uses
// installed cloud tools or the owner's credentials.
func TestCredentialPluginProcess(t *testing.T) {
	path := os.Getenv("PLOUS_CREDENTIAL_TEST_RESPONSE")
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		os.Exit(1)
	}
	_, _ = os.Stdout.Write(data)
	os.Exit(0)
}

func TestCredentialPluginRefreshAfterAuthenticationFailure(t *testing.T) {
	var requireRenewed atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		token := "fixture-first"
		if requireRenewed.Load() {
			token = "fixture-renewed"
		}
		if request.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			reply(w, map[string]any{"apiVersion": "v1", "kind": "Status", "status": "Failure", "reason": "Unauthorized", "code": 401})
			return
		}
		reply(w, map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": "example", "namespace": "test", "uid": "fixture"}})
	}))
	defer server.Close()
	root := t.TempDir()
	response := filepath.Join(root, "credential.json")
	writeCredential := func(token string) {
		t.Helper()
		contents, _ := json.Marshal(map[string]any{"apiVersion": "client.authentication.k8s.io/v1", "kind": "ExecCredential", "status": map[string]any{"token": token, "expirationTimestamp": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}})
		if err := os.WriteFile(response, contents, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeCredential("fixture-first")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	config := map[string]any{
		"apiVersion": "v1", "kind": "Config", "current-context": "plugin-test",
		"clusters": []any{map[string]any{"name": "test", "cluster": map[string]any{"server": server.URL, "insecure-skip-tls-verify": true}}},
		"contexts": []any{map[string]any{"name": "plugin-test", "context": map[string]any{"cluster": "test", "user": "plugin"}}},
		"users":    []any{map[string]any{"name": "plugin", "user": map[string]any{"exec": map[string]any{"apiVersion": "client.authentication.k8s.io/v1", "command": executable, "args": []string{"-test.run=^TestCredentialPluginProcess$"}, "interactiveMode": "Never", "env": []any{map[string]string{"name": "PLOUS_CREDENTIAL_TEST_RESPONSE", "value": response}}}}}},
	}
	contents, _ := json.Marshal(config)
	path := filepath.Join(root, "kubeconfig")
	if err := os.WriteFile(path, contents, 0600); err != nil {
		t.Fatal(err)
	}
	client, err := New(path, "plugin-test", "test")
	if err != nil {
		t.Fatal(err)
	}
	r := kubecommon.Resource{Name: "pods", Version: "v1", Kind: "Pod", Namespaced: true}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := client.Get(ctx, r, "test", "example"); err != nil {
		t.Fatal(err)
	}
	writeCredential("fixture-renewed")
	requireRenewed.Store(true)
	if _, err := client.Get(ctx, r, "test", "example"); !apierrors.IsUnauthorized(err) {
		t.Fatalf("expected expired authorization: %v", err)
	}
	if _, err := client.Get(ctx, r, "test", "example"); err != nil {
		t.Fatalf("credential refresh: %v", err)
	}
	preserved, err := os.ReadFile(path)
	if err != nil || string(preserved) != string(contents) {
		t.Fatal("authentication rewrote kubeconfig")
	}
}
