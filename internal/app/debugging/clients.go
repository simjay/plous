package debugging

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/simjay/plous/internal/ai/agent"
)

type setupClient struct {
	kind       agent.Kind
	command    string
	configPath string
	skillDir   string
	config     configAdapter
}

var setupClients = [...]setupClient{
	{kind: agent.Codex, command: "codex", configPath: ".codex/config.toml", skillDir: ".agents/skills", config: tomlConfig{forwardKubeconfig: true}},
	{kind: agent.ClaudeCode, command: "claude", configPath: ".claude.json", skillDir: ".claude/skills", config: jsonConfig{mcpType: "stdio"}},
	{kind: agent.OpenCode, command: "opencode", configPath: ".config/opencode/opencode.json", skillDir: ".agents/skills", config: openCodeConfig{}},
	{kind: agent.GrokBuild, command: "grok", configPath: ".grok/config.toml", skillDir: ".grok/skills", config: tomlConfig{}},
	{kind: agent.Antigravity, command: "agy", configPath: ".gemini/config/mcp_config.json", skillDir: ".gemini/skills", config: jsonConfig{}},
	{kind: agent.Cursor, command: "cursor", configPath: ".cursor/mcp.json", skillDir: ".agents/skills", config: jsonConfig{allowComments: true, mcpType: "stdio", kubeconfigEnv: "${KUBECONFIG:-}"}},
	{kind: agent.GeminiCLI, command: "gemini", configPath: ".gemini/settings.json", skillDir: ".gemini/skills", config: jsonConfig{allowComments: true}},
}

func findSetupClient(name string) (setupClient, bool) {
	for _, client := range setupClients {
		if name == client.command || name == string(client.kind) {
			return client, true
		}
	}
	return setupClient{}, false
}

func setupClientNames() string {
	names := make([]string, 0, len(setupClients))
	for _, client := range setupClients {
		names = append(names, client.command)
	}
	return strings.Join(names, "|")
}

func (c setupClient) paths(home string, useEnvironment bool) (config, skill string) {
	config = filepath.Join(home, c.configPath)
	skill = filepath.Join(home, c.skillDir, "plous-debug", "SKILL.md")
	if !useEnvironment {
		return config, skill
	}
	switch c.kind {
	case agent.Codex:
		if dir := os.Getenv("CODEX_HOME"); dir != "" {
			config = filepath.Join(dir, "config.toml")
		}
	case agent.OpenCode:
		if dir := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(dir) {
			config = filepath.Join(dir, "opencode", "opencode.json")
		}
	case agent.GrokBuild:
		if dir := os.Getenv("GROK_HOME"); dir != "" {
			config = filepath.Join(dir, "config.toml")
			skill = filepath.Join(dir, "skills", "plous-debug", "SKILL.md")
		}
	case agent.GeminiCLI:
		if dir := os.Getenv("GEMINI_CLI_HOME"); dir != "" {
			config = filepath.Join(dir, c.configPath)
			skill = filepath.Join(dir, c.skillDir, "plous-debug", "SKILL.md")
		}
	}
	return config, skill
}
