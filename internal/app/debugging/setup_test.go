package debugging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/simjay/plous/internal/ai/agent"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/skills"
	"github.com/tailscale/hujson"
	"golang.org/x/sys/unix"
)

func TestSetupConfigPreservesExistingSettingsAndUpdatesOwnServer(t *testing.T) {
	for _, tc := range []struct {
		name, client, old, serversKey string
	}{
		{"codex", "codex", "# Personal settings\nmodel = 'my-model'\n[mcp_servers.other]\ncommand = 'other-server'\n", "mcp_servers"},
		{"grok", "grok", "# Personal settings\n[ui]\ncompact_mode = true\n[mcp_servers.other]\ncommand = 'other-server'\n", "mcp_servers"},
		{"claude", "claude", `{"theme":"dark","mcpServers":{"other":{"command":"other-server"}}}`, "mcpServers"},
		{"cursor", "cursor", `{"theme":"dark","mcpServers":{"other":{"command":"other-server"}}}`, "mcpServers"},
		{"agy", "agy", `{"theme":"dark","mcpServers":{"other":{"command":"other-server"}}}`, "mcpServers"},
		{"gemini", "gemini", `{"theme":"dark","mcpServers":{"other":{"command":"other-server"}}}`, "mcpServers"},
		{"opencode", "opencode", `{// Personal settings
				"mcp": {"other": {"type": "local", "command": ["other-server"]}},
			}`, "mcp"},
		{"opencode-v2", "opencode", `{/* Personal settings */
				"mcp": {"servers": {"other": {"type": "local", "command": ["other-server"]}}},
			}`, "mcp"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := findSetupClient(tc.client)
			args := []string{"mcp", "serve", "--agent", string(client.kind), "--data-dir", t.TempDir(), "--context", "staging"}
			first, err := client.mergeConfig([]byte(tc.old), "/path with spaces/plous", args)
			if err != nil {
				t.Fatal(err)
			}
			args = append(args, "--allow-exec")
			updated, err := client.mergeConfig(first, "/path with spaces/plous", args)
			if err != nil {
				t.Fatal(err)
			}
			again, err := client.mergeConfig(updated, "/path with spaces/plous", args)
			if err != nil || !bytes.Equal(updated, again) {
				t.Fatalf("setup is not idempotent: %s %s %v", updated, again, err)
			}
			var config map[string]any
			if tc.serversKey == "mcp_servers" {
				err = toml.Unmarshal(updated, &config)
			} else {
				standard, parseErr := hujson.Standardize(bytes.Clone(updated))
				if parseErr != nil {
					t.Fatal(parseErr)
				}
				err = json.Unmarshal(standard, &config)
			}
			if err != nil {
				t.Fatal(err)
			}
			servers := config[tc.serversKey].(map[string]any)
			if tc.name == "opencode-v2" {
				servers = servers["servers"].(map[string]any)
			}
			if servers["other"] == nil || servers[serverName] == nil || !bytes.Contains(updated, []byte("other-server")) {
				t.Fatalf("server configuration changed: %s", updated)
			}
			if strings.Contains(tc.old, "Personal settings") && !bytes.Contains(updated, []byte("Personal settings")) {
				t.Fatalf("removed an existing comment: %s", updated)
			}
			if !bytes.Contains(updated, []byte("/path with spaces/plous")) || !bytes.Contains(updated, []byte("--allow-exec")) {
				t.Fatalf("incorrect command: %s", updated)
			}
		})
	}
}

func TestSetupRefusesUnmanagedServersAndMalformedConfig(t *testing.T) {
	for _, tc := range []struct {
		client string
		old    string
	}{
		{"codex", "[mcp_servers.plous_debug]\ncommand = 'other'\n"},
		{"codex", "invalid = ["},
		{"codex", "mcp_servers = false"},
		{"grok", "# BEGIN PLOUS DEBUG MCP\n[mcp_servers.plous_debug]\ncommand = 'plous'\n[ui]\ncompact_mode = true\n# END PLOUS DEBUG MCP"},
		{"claude", `{"mcpServers":{"plous_debug":{"command":"other"}}}`},
		{"claude", `null`},
		{"claude", `{"mcpServers":null}`},
		{"agy", `{"mcpServers":{"plous_debug":{"command":"plous","args":["mcp","serve","--agent","agy"]}}}`},
		{"gemini", `{"mcpServers":{"plous_debug":{"command":"/usr/local/bin/plous","args":["mcp","serve","--agent","gemini"]}}}`},
		{"cursor", `{"mcpServers":{},"mcpServers":{"plous_debug":{"url":"https://example.test"}}}`},
		{"opencode", `{"mcp":{"plous_debug":{"type":"remote","url":"https://example.test"}}}`},
		{"opencode", `{"mcp":{"servers":{"plous_debug":{"type":"remote","url":"https://example.test"}}}}`},
		{"opencode", `{"mcp":{"servers":null}}`},
		{"opencode", `{"mcp":{},"mcp":{"plous_debug":{"type":"remote"}}}`},
	} {
		client, _ := findSetupClient(tc.client)
		if _, err := client.mergeConfig([]byte(tc.old), "/bin/plous", []string{"mcp", "serve"}); err == nil {
			t.Fatalf("accepted unmanaged or invalid config: %s", tc.old)
		}
	}
}

func TestSetupPreservesClientSettingsOnRerun(t *testing.T) {
	const jsonFields = `"disabled":true,"excludeTools":["kubernetes_exec_run"],"env":{"KUBECONFIG":"/custom/kubeconfig","EXTRA":"value"}`
	const tomlFields = "enabled = false\nenv = { KUBECONFIG = '/custom/kubeconfig' }\ntool_timeout_sec = 90\n"
	const openCodeFields = `"disabled":true,"environment":{"KUBECONFIG":"/custom/kubeconfig"},"timeout":{"execution":120000},"codemode":true`
	for _, tc := range []struct {
		client, document, retained string
	}{
		{"agy", `{"mcpServers":{"plous_debug":{"command":"/old/bin/plous","args":%s,` + jsonFields + `}}}`, jsonFields},
		{"gemini", "{\n// Keep this note\n" + `"mcpServers":{"plous_debug":{"command":"/old/bin/plous","args":%s,` + jsonFields + `}},}`, jsonFields},
		{"cursor", "{\n// Keep this note\n" + `"mcpServers":{"plous_debug":{"command":"/old/bin/plous","args":%s,` + jsonFields + `}},}`, jsonFields},
		{"opencode", `{"mcp":{"servers":{"plous_debug":{"type":"local","command":["/old/bin/plous",%s],` + openCodeFields + `}}}}`, openCodeFields},
		{"grok", "# Rewritten by the client\n[mcp_servers.plous_debug]\ncommand = '/old/bin/plous' # keep\nargs = %s\n" + tomlFields, tomlFields},
		{"codex", "[mcp_servers.plous_debug]\ncommand = '/old/bin/plous'\nargs = %s\nenv_vars = ['OTHER']\n" + tomlFields, "env_vars = ['OTHER']\n" + tomlFields},
	} {
		t.Run(tc.client, func(t *testing.T) {
			client, _ := findSetupClient(tc.client)
			args := append(client.launchArgs(configuration{dataDir: "/tmp/evidence"}), "--context", "old-context")
			encoded, err := json.Marshal(args)
			if err != nil {
				t.Fatal(err)
			}
			if tc.client == "opencode" {
				encoded = encoded[1 : len(encoded)-1]
			}
			old := []byte(fmt.Sprintf(tc.document, encoded))
			next := client.launchArgs(configuration{dataDir: "/tmp/evidence", policy: kubernetes.DebugPolicy{Image: "fixture-image"}})
			config, err := client.mergeConfig(old, "/new/bin/plous", next)
			if err != nil || !bytes.Contains(config, []byte(tc.retained)) || !bytes.Contains(config, []byte("fixture-image")) ||
				bytes.Contains(config, []byte("old-context")) || bytes.Contains(config, []byte("/old/bin/plous")) {
				t.Fatalf("unexpected merge result %s, err %v", config, err)
			}
			if bytes.Contains(old, []byte("// Keep this note")) && !bytes.Contains(config, []byte("// Keep this note")) {
				t.Fatalf("removed the JSONC comment: %s", config)
			}
		})
	}
}

func TestTOMLSetupEditsParsedRegistration(t *testing.T) {
	const prompt = "prompt = '''\n# BEGIN PLOUS DEBUG MCP\n[mcp_servers.plous_debug]\ncommand = 'example'\n# END PLOUS DEBUG MCP\n'''\n"
	const neighbor = "other = { command = 'other' }"
	c, _ := findSetupClient("grok")
	args := c.launchArgs(configuration{dataDir: "/tmp/evidence"})
	for _, tc := range []struct {
		name, old string
	}{
		{"separate table", prompt},
		{"inline container", prompt + "mcp_servers = { " + neighbor + " }\n"},
		{"dotted fields", prompt + "mcp_servers.plous_debug.command = '/old/bin/plous'\n" +
			"mcp_servers.plous_debug.args = [\n'mcp', 'serve', '--agent', 'grok', '--data-dir', '/tmp/old'\n]\n"},
		{"inline server", prompt + "[mcp_servers]\nplous_debug = { command = '/old/bin/plous', " +
			"args = ['mcp', 'serve', '--agent', 'grok', '--data-dir', '/tmp/old'], enabled = false }\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registration, err := c.mergeConfig([]byte(tc.old), "/installed/bin/plous", args)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.HasPrefix(registration, []byte(prompt)) {
				t.Fatalf("modified a prompt containing old markers: %s", registration)
			}
			for _, retained := range []string{neighbor, "enabled = false"} {
				if strings.Contains(tc.old, retained) && !bytes.Contains(registration, []byte(retained)) {
					t.Fatalf("removed %s: %s", retained, registration)
				}
			}
			if got := readSetupArgs(t, "grok", registration); !slices.Equal(got, args) {
				t.Fatalf("got args %v, want %v", got, args)
			}
		})
	}
}

func TestSetupInstallsWithoutKubeconfigOrPinnedContext(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KUBECONFIG", filepath.Join(root, "not-configured"))
	wantSkill := strings.Replace(skills.DebugSkill, "\n---\n", "\n---\n\n"+skillMarker, 1)
	for _, tc := range []struct{ name, identity, config, skills string }{
		{"codex", "codex", ".codex/config.toml", ".agents/skills"},
		{"claude", "claude-code", ".claude.json", ".claude/skills"},
		{"opencode", "opencode", ".config/opencode/opencode.json", ".agents/skills"},
		{"grok", "grok", ".grok/config.toml", ".grok/skills"},
		{"agy", "agy", ".gemini/config/mcp_config.json", ".gemini/skills"},
		{"cursor", "cursor", ".cursor/mcp.json", ".agents/skills"},
		{"gemini", "gemini", ".gemini/settings.json", ".gemini/skills"},
	} {
		home := filepath.Join(root, tc.name)
		var out, diagnostics bytes.Buffer
		args := []string{"setup", tc.name, "--home", home, "--data-dir", filepath.Join(root, "evidence")}
		for range 2 {
			if code := Run(t.Context(), args, io.NopCloser(strings.NewReader("")), &out, &diagnostics, "0.0.1", "", kubernetes.DebugPolicy{}); code != 0 {
				t.Fatalf("%s setup: %d %s", tc.name, code, diagnostics.String())
			}
		}
		skillPath := filepath.Join(home, tc.skills, "plous-debug", "SKILL.md")
		configPath := filepath.Join(home, tc.config)
		data, err := os.ReadFile(skillPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != wantSkill {
			t.Fatalf("%s installed a different skill from the shared source", tc.name)
		}
		monitorPath := filepath.Join(home, tc.skills, "plous-monitor", "SKILL.md")
		monitorSkill, err := os.ReadFile(monitorPath)
		wantMonitor := strings.Replace(skills.MonitorSkill, "\n---\n", "\n---\n\n"+skillMarker, 1)
		if err != nil || string(monitorSkill) != wantMonitor {
			t.Fatalf("%s monitor skill installation: %v", tc.name, err)
		}
		for _, path := range []string{skillPath, monitorPath, configPath} {
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatalf("%s is not private: %v", path, err)
			}
		}
		config, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatal(err)
		}
		installed := readSetupArgs(t, tc.name, config)
		if !slices.Equal(installed, []string{"mcp", "serve", "--agent", tc.identity, "--data-dir", filepath.Join(root, "evidence")}) {
			t.Fatalf("setup did not leave the target live: %v", installed)
		}
		if tc.name == "cursor" {
			var cursor struct {
				Servers map[string]struct {
					Env map[string]string `json:"env"`
				} `json:"mcpServers"`
			}
			if err := json.Unmarshal(config, &cursor); err != nil {
				t.Fatal(err)
			}
			if value := cursor.Servers[serverName].Env["KUBECONFIG"]; value != "${KUBECONFIG:-}" {
				t.Fatalf("Cursor must forward KUBECONFIG with an empty default, got %q", value)
			}
		}
	}
}

func readSetupArgs(t *testing.T, client string, config []byte) []string {
	t.Helper()
	type registration struct {
		Args []string `json:"args" toml:"args"`
	}
	switch client {
	case "codex", "grok":
		var doc struct {
			Servers map[string]registration `toml:"mcp_servers"`
		}
		if err := toml.Unmarshal(config, &doc); err != nil {
			t.Fatal(err)
		}
		return doc.Servers[serverName].Args
	case "opencode":
		var doc struct {
			MCP map[string]struct {
				Command []string `json:"command"`
			} `json:"mcp"`
		}
		if err := json.Unmarshal(config, &doc); err != nil {
			t.Fatal(err)
		}
		command := doc.MCP[serverName].Command
		if len(command) == 0 {
			t.Fatal("OpenCode has no MCP command")
		}
		return command[1:]
	default:
		var doc struct {
			Servers map[string]registration `json:"mcpServers"`
		}
		if err := json.Unmarshal(config, &doc); err != nil {
			t.Fatal(err)
		}
		return doc.Servers[serverName].Args
	}
}

func TestSetupLeavesLockedConfigurationAndSkillUntouched(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".cursor")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	lock, err := os.Open(configDir)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	client, _ := findSetupClient("cursor")
	if err := client.install(configuration{home: home}, io.Discard); err == nil || !strings.Contains(err.Error(), "retry setup") {
		t.Fatalf("concurrent setup did not report a retry: %v", err)
	}
	config, skill := client.paths(home, false)
	for _, path := range []string{config, skill} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("concurrent setup wrote %s: %v", path, err)
		}
	}
}

func TestOpenCodeSetupUpdatesTheExistingLayer(t *testing.T) {
	root := t.TempDir()
	client, _ := findSetupClient("opencode")
	primary := filepath.Join(root, "opencode.json")
	overlay := filepath.Join(root, "opencode.jsonc")
	managed := []byte(`{"mcp":{"plous_debug":{"type":"local","command":["/bin/plous","mcp","serve","--agent","opencode","--data-dir","/tmp/evidence"]}}}`)
	if err := os.WriteFile(primary, managed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(overlay, []byte(`{"model":"my-model"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := client.configFile(primary, "/bin/plous"); err != nil || got != primary {
		t.Fatalf("selected %q: %v", got, err)
	}
	if err := os.WriteFile(overlay, managed, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := client.configFile(primary, "/bin/plous"); err == nil {
		t.Fatal("accepted a duplicate OpenCode registration")
	}
}

func TestStandaloneClientsAreNotPanelAgents(t *testing.T) {
	for _, kind := range []agent.Kind{agent.OpenCode, agent.GrokBuild, agent.Antigravity, agent.Cursor, agent.GeminiCLI} {
		if !kind.Valid() || kind.SupportsPanel() {
			t.Errorf("%s is not limited to standalone setup", kind)
		}
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
