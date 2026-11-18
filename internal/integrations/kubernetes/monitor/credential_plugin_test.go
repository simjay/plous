package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == CredentialHelperCommand {
		os.Exit(RunCredentialHelper(os.Args[2:], os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

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
	previous := credentialHelpers.Load()
	stop := StartCredentialHelpers()
	t.Cleanup(func() {
		stop()
		credentialHelpers.Store(previous)
	})
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

func TestCredentialHelperTimeoutReapsProcessGroupAndAllowsNextInvocation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	scope := &credentialHelperScope{ctx: ctx, cancel: cancel}
	defer scope.close()
	address, err := scope.socket()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	started, survived := filepath.Join(root, "started"), filepath.Join(root, "survived")
	args := []string{"sh", "-c", `(sleep 1; touch "$2") & touch "$1"; wait`, "fixture", started, survived}
	err = runCredentialHelper(t.Context(), "/bin/sh", args, io.Discard, 500*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("hung helper did not time out", err)
	}
	if _, err := os.Stat(started); err != nil {
		t.Fatal("fixture helper did not run", err)
	}
	var output bytes.Buffer
	if err := runCredentialHelper(t.Context(), "/bin/sh", []string{"sh", "-c", "printf recovered"}, &output, 3*time.Second); err != nil || output.String() != "recovered" {
		t.Fatal("timed-out helper blocked the next invocation", err, output.String())
	}
	time.Sleep(600 * time.Millisecond)
	if _, err := os.Stat(survived); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("helper descendant survived timeout", err)
	}
	scope.close()
	if _, err := os.Stat(filepath.Dir(address)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("credential supervisor left its socket directory", err)
	}
}

func TestCredentialHelperAllowlistAppliesToTheOriginalCommand(t *testing.T) {
	t.Setenv("PATH", "/bin:/usr/bin")
	for _, test := range []struct {
		policy    clientcmdapi.PolicyType
		command   string
		allowlist []string
		allowed   bool
	}{
		{clientcmdapi.PluginPolicyDenyAll, "sh", nil, false},
		{clientcmdapi.PluginPolicyAllowlist, "sh", []string{"/bin/sh"}, true},
		{clientcmdapi.PluginPolicyAllowlist, "/bin/sh", []string{"sh"}, true},
		{clientcmdapi.PluginPolicyAllowlist, "sh", []string{"/bin/echo"}, false},
		{clientcmdapi.PluginPolicyAllowAll, "missing-plous-fixture-helper", nil, false},
	} {
		t.Run(fmt.Sprint(test.policy, "/", test.command), func(t *testing.T) {
			_, err := authorizedCredentialHelper(credentialInvocation{Policy: test.policy, Command: test.command, Allowlist: test.allowlist})
			if (err == nil) != test.allowed {
				t.Fatal("unexpected credential helper policy result", err)
			}
		})
	}
}

func TestCredentialHelperLauncherRequiresARegisteredHelperFromItsParent(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	scope := &credentialHelperScope{ctx: ctx, cancel: cancel, helpers: make(map[string][]byte)}
	defer scope.close()
	address, id, err := scope.register(credentialInvocation{Command: "/bin/echo", Arguments: []string{"unexpected"}})
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []*exec.Cmd{
		exec.CommandContext(ctx, executable, CredentialHelperCommand, address, "unregistered"),
		exec.CommandContext(ctx, "/bin/sh", "-c", `"$@"; status=$?; exit "$status"`, "fixture", executable, CredentialHelperCommand, address, id),
	} {
		output, err := cmd.Output()
		if err == nil || len(output) != 0 {
			t.Fatal("untrusted launcher executed a credential helper", err)
		}
	}
}
