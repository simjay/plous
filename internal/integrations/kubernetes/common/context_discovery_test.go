package common

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDiscoverMetadataWithoutMutationOrAuthExecution(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "auth-ran")
	content := `apiVersion: v1
kind: Config
current-context: production
contexts:
  - name: production
    context:
      namespace: payments
      cluster: production-cluster
      user: auth-user
  - name: development
    context:
      cluster: development-cluster
users:
  - name: auth-user
    user:
      token: DO-NOT-COPY-TOKEN
      exec:
        command: touch
        args: ["` + marker + `"]
clusters:
  - name: production-cluster
    cluster:
      server: https://127.0.0.1:1
      certificate-authority-data: DO-NOT-COPY-CERT
`
	path := writeConfig(t, dir, "config", content)
	got, err := DiscoverContexts(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []Context{
		{Name: "development", Cluster: "development-cluster", Source: path, Kubeconfig: path},
		{Name: "production", Namespace: "payments", Cluster: "production-cluster", Source: path, Kubeconfig: path},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("contexts = %#v; want %#v", got, want)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != content {
		t.Fatalf("kubeconfig changed: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("auth plugin was executed: %v", err)
	}
}

func TestDiscoverKubeconfigListFirstDefinitionWins(t *testing.T) {
	dir := t.TempDir()
	first := writeConfig(t, dir, "first", `contexts:
- name: shared
  context: {cluster: first-cluster, namespace: first-namespace}
- name: zebra
  context: {cluster: first-cluster}
`)
	second := writeConfig(t, dir, "second", `contexts:
- name: shared
  context: {cluster: second-cluster, namespace: second-namespace}
- name: alpha
  context: {cluster: second-cluster}
`)
	t.Setenv("KUBECONFIG", strings.Join([]string{
		filepath.Join(dir, "missing"), first, "", first, second,
	}, string(os.PathListSeparator)))
	got, err := DiscoverContexts("")
	if err != nil {
		t.Fatal(err)
	}
	chain := first + string(os.PathListSeparator) + second
	want := []Context{
		{Name: "alpha", Cluster: "second-cluster", Source: second, Kubeconfig: chain},
		{Name: "shared", Namespace: "first-namespace", Cluster: "first-cluster", Source: first, Kubeconfig: chain},
		{Name: "zebra", Cluster: "first-cluster", Source: first, Kubeconfig: chain},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("contexts = %#v; want %#v", got, want)
	}
}

func TestDiscoverRetainsMergedReferenceProvenance(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	contextPath := writeConfig(t, home, "contexts", `contexts:
- name: production
  context: {cluster: remote-cluster, user: remote-user}
`)
	refsPath := writeConfig(t, home, "references", `clusters:
- name: remote-cluster
  cluster: {server: "https://127.0.0.1:1"}
users:
- name: remote-user
  user: {token: DO-NOT-COPY-TOKEN}
`)
	t.Setenv("KUBECONFIG", strings.Join([]string{
		"~/contexts", filepath.Join(home, "missing"), "~/references", contextPath,
	}, string(os.PathListSeparator)))
	got, err := DiscoverContexts("")
	if err != nil {
		t.Fatal(err)
	}
	chain := contextPath + string(os.PathListSeparator) + refsPath
	want := []Context{{
		Name: "production", Cluster: "remote-cluster", Source: contextPath, Kubeconfig: chain,
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("contexts = %#v; want %#v", got, want)
	}
}

func TestDiscoverExplicitPathOverridesEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, "selected", "contexts:\n- name: selected\n  context: {cluster: cluster}\n")
	t.Setenv("KUBECONFIG", filepath.Join(dir, "missing"))
	contexts, err := DiscoverContexts(path)
	if err != nil || len(contexts) != 1 || contexts[0].Name != "selected" {
		t.Fatalf("contexts = %#v, error = %v", contexts, err)
	}
}

func TestDiscoverDefaultAndHomeExpansion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("KUBECONFIG", "")
	dir := filepath.Join(home, ".kube")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := writeConfig(t, dir, "config", "contexts:\n- name: home\n  context: {cluster: home-cluster}\n")
	for _, source := range []string{"", "~/.kube/config", path} {
		contexts, err := DiscoverContexts(source)
		if err != nil || len(contexts) != 1 || contexts[0].Source != path {
			t.Errorf("DiscoverContexts(%q) = %#v, %v", source, contexts, err)
		}
	}
}

func TestDiscoverMissingFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("KUBECONFIG", "")
	for _, path := range []string{"", filepath.Join(home, "missing")} {
		_, err := DiscoverContexts(path)
		if err == nil || !strings.Contains(err.Error(), "choose an existing kubeconfig path") {
			t.Errorf("DiscoverContexts(%q) error = %v", path, err)
		}
	}
	t.Setenv("KUBECONFIG", filepath.Join(home, "missing"))
	if _, err := DiscoverContexts(""); err == nil || !strings.Contains(err.Error(), "no kubeconfig files found") {
		t.Fatalf("missing KUBECONFIG files error = %v", err)
	}
}

func TestDiscoverInvalidInputs(t *testing.T) {
	tests := []struct {
		name, data, message string
	}{
		{"malformed", "contexts: [invalid", "invalid kubeconfig YAML"},
		{"wrong shape", "contexts: literal", "invalid kubeconfig YAML"},
		{"wrong field shape", "contexts:\n- name: {DO-NOT-EXPOSE: secret}\n", "invalid kubeconfig YAML"},
		{"unnamed", "contexts:\n- context: {cluster: cluster}\n", "unnamed context"},
		{"empty", "", "no contexts found"},
		{"no contexts", "apiVersion: v1\ncontexts: []\n", "no contexts found"},
		{"multiple documents", "contexts: []\n---\ncontexts: []\n", "one YAML document"},
		{"oversized", strings.Repeat(" ", maxFileSize+1), "16 MiB input limit"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeConfig(t, t.TempDir(), "config", test.data)
			_, err := DiscoverContexts(path)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error = %v; want %q", err, test.message)
			}
			if strings.Contains(err.Error(), "DO-NOT-EXPOSE") {
				t.Fatalf("error quotes kubeconfig content: %v", err)
			}
		})
	}
	if _, err := DiscoverContexts(t.TempDir()); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("directory error = %v", err)
	}
}

func TestDiscoverMalformedListMemberFailsInsteadOfPartialResult(t *testing.T) {
	dir := t.TempDir()
	valid := writeConfig(t, dir, "valid", "contexts:\n- name: valid\n  context: {cluster: cluster}\n")
	invalid := writeConfig(t, dir, "invalid", "contexts: [invalid")
	t.Setenv("KUBECONFIG", valid+string(os.PathListSeparator)+invalid)
	contexts, err := DiscoverContexts("")
	if err == nil || contexts != nil {
		t.Fatalf("expected actionable error, got contexts = %#v, error = %v", contexts, err)
	}
}

func TestDiscoverJSONKubeconfig(t *testing.T) {
	path := writeConfig(t, t.TempDir(), "config.json", `{"contexts":[{"name":"json","context":{"cluster":"json-cluster"}}]}`)
	got, err := DiscoverContexts(path)
	if err != nil || len(got) != 1 || got[0].Name != "json" {
		t.Fatalf("contexts = %#v, error = %v", got, err)
	}
}
