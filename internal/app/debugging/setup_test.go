package debugging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/simjay/plous/internal/ai/agent"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

func TestSetupConfigPreservesExistingSettingsAndUpdatesOwnServer(t *testing.T) {
	for _, kind := range []agent.Kind{agent.Codex, agent.ClaudeCode} {
		t.Run(string(kind), func(t *testing.T) {
			old := []byte("# Personal settings\nmodel = 'my-model'\n[mcp_servers.other]\ncommand = 'other-server'\n")
			if kind == agent.ClaudeCode {
				old = []byte(`{"theme":"dark","projects":{"/repo":{"trusted":true}},"mcpServers":{"other":{"command":"other-server"}}}`)
			}
			args := []string{"mcp", "serve", "--context", "staging"}
			first, err := mergeConfig(kind, old, "/path with spaces/plous", args)
			if err != nil {
				t.Fatal(err)
			}
			args = append(args, "--allow-exec")
			updated, err := mergeConfig(kind, first, "/path with spaces/plous", args)
			if err != nil {
				t.Fatal(err)
			}
			again, err := mergeConfig(kind, updated, "/path with spaces/plous", args)
			if err != nil || !bytes.Equal(updated, again) {
				t.Fatalf("setup is not idempotent: %s %s %v", updated, again, err)
			}
			if kind == agent.Codex {
				var config map[string]any
				if err = toml.Unmarshal(updated, &config); err != nil {
					t.Fatal(err)
				}
				if config["model"] != "my-model" || !strings.Contains(string(updated), "# Personal settings") {
					t.Fatal("personal settings or comments lost")
				}
				servers := config["mcp_servers"].(map[string]any)
				if servers["other"].(map[string]any)["command"] != "other-server" {
					t.Fatal("unrelated server changed")
				}
				if servers["plous_debug"].(map[string]any)["command"] != "/path with spaces/plous" {
					t.Fatal("command path changed")
				}
			} else {
				var config map[string]any
				if err = json.Unmarshal(updated, &config); err != nil {
					t.Fatal(err)
				}
				if config["theme"] != "dark" || config["projects"] == nil || config["mcpServers"].(map[string]any)["other"] == nil {
					t.Fatal("unrelated settings lost")
				}
			}
		})
	}
}

func TestSetupRefusesUnmanagedServersAndMalformedConfig(t *testing.T) {
	for _, tc := range []struct {
		kind agent.Kind
		old  string
	}{
		{agent.Codex, "[mcp_servers.plous_debug]\ncommand = 'other'\n"},
		{agent.Codex, "invalid = ["},
		{agent.Codex, configStart + "\nmodel = 'a'\n"},
		{agent.ClaudeCode, `{"mcpServers":{"plous_debug":{"command":"other"}}}`},
		{agent.ClaudeCode, `null`},
		{agent.ClaudeCode, `{"mcpServers":null}`},
	} {
		if _, err := mergeConfig(tc.kind, []byte(tc.old), "/bin/plous", []string{"mcp", "serve"}); err == nil {
			t.Fatalf("accepted unmanaged or invalid config: %s", tc.old)
		}
	}
}

func TestSetupInstallsOneSkillAndCapturedServerWithoutChangingKubeconfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/version" {
			t.Errorf("unexpected setup acquisition %s", r.URL)
		}
		fmt.Fprint(w, `{"gitVersion":"fixture"}`)
	}))
	defer server.Close()
	root := t.TempDir()
	path := filepath.Join(root, "config")
	config := fmt.Sprintf("apiVersion: v1\nkind: Config\nclusters:\n- name: fixture\n  cluster:\n    server: %s\ncontexts:\n- name: staging\n  context:\n    cluster: fixture\n    namespace: shop\ncurrent-context: staging\n", server.URL)
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"codex", "claude"} {
		home := filepath.Join(root, kind)
		var out, diagnostics bytes.Buffer
		args := []string{"setup", kind, "--home", home, "--kubeconfig", path, "--context", "staging", "--data-dir", filepath.Join(root, "evidence"), "--allow-exec", "--diagnostic-image", "diagnostics@sha256:fixture"}
		for range 2 {
			if code := Run(t.Context(), args, io.NopCloser(strings.NewReader("")), &out, &diagnostics, "0.0.1", "", kubernetes.DebugPolicy{}); code != 0 {
				t.Fatalf("setup: %d %s", code, diagnostics.String())
			}
		}
		skillPath := filepath.Join(home, ".agents", "skills", "plous-debug", "SKILL.md")
		if kind == "claude" {
			skillPath = filepath.Join(home, ".claude", "skills", "plous-debug", "SKILL.md")
		}
		data, err := os.ReadFile(skillPath)
		if err != nil || !bytes.HasPrefix(data, []byte("---\nname: plous-debug")) || !bytes.Contains(data, []byte("## Reproduce")) {
			t.Fatalf("missing portable skill: %s %v", data, err)
		}
		info, _ := os.Stat(skillPath)
		if info.Mode().Perm() != 0600 {
			t.Fatal("skill is not private")
		}
	}
	data, _ := os.ReadFile(path)
	if string(data) != config {
		t.Fatal("setup changed kubeconfig")
	}
}

func TestInstallFilesRejectSymlinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	link := filepath.Join(root, "link")
	if err := os.WriteFile(target, []byte("personal config"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readInstallFile(link); err == nil {
		t.Fatal("followed installation symlink")
	}
}
