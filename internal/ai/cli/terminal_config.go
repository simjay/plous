package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pelletier/go-toml/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/simjay/plous/internal/ai/agent"
)

type terminalLaunch struct {
	args []string
	env  []string
}

func terminalResumeArgs(o TerminalOptions, args []string) ([]string, error) {
	switch o.Kind {
	case agent.ClaudeCode:
		if o.Resume {
			return append(args, "--resume", o.sessionID), nil
		}
		return append(args, "--session-id", o.sessionID), nil
	case agent.Codex:
		if o.Resume {
			return append(args, "resume", "--last"), nil
		}
		return args, nil
	}
	return nil, errors.New("unknown external agent")
}

func claudeSessionID(directory string, resume bool) (string, error) {
	path := filepath.Join(directory, "session-id")
	if resume {
		data, err := os.ReadFile(path)
		if err == nil {
			id := string(data)
			if !sessionUUID.MatchString(id) {
				return "", errors.New("invalid native Claude Code session ID")
			}
			return id, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	id[6], id[8] = id[6]&0x0f|0x40, id[8]&0x3f|0x80
	value := fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
	return value, os.WriteFile(path, []byte(value), 0600)
}

func prepareTerminal(ctx context.Context, path, configDir string, o TerminalOptions, b *Bridge) (terminalLaunch, error) {
	launch := terminalLaunch{env: append(environment(), "TERM=xterm-256color", "COLORTERM=truecolor", "PLOUS_TOOL_TOKEN="+b.token)}
	switch o.Kind {
	case agent.ClaudeCode:
		config := map[string]any{"mcpServers": map[string]any{"plous": map[string]any{"type": "http", "url": b.URL, "headers": map[string]string{"Authorization": b.Authorization()}}}}
		raw, _ := json.Marshal(config)
		configPath := filepath.Join(configDir, "mcp.json")
		if err := os.WriteFile(configPath, raw, 0600); err != nil {
			return terminalLaunch{}, err
		}
		allowed := []string{}
		for _, tool := range b.tools.Definitions() {
			allowed = append(allowed, "mcp__plous__"+tool.Name)
		}
		args := []string{"--tools", "", "--strict-mcp-config", "--mcp-config", configPath, "--allowedTools", strings.Join(allowed, ","), "--permission-mode", "dontAsk", "--setting-sources", "user", "--settings", `{"disableAllHooks":true,"autoMemoryEnabled":false,"claudeMdExcludes":["**"],"disableSkillShellExecution":true,"disableBundledSkills":true,"permissions":{"deny":["Skill"]}}`, "--no-chrome", "--append-system-prompt", o.Instructions}
		launch.args = args
		return launch, nil
	case agent.Codex:
		// Only query configuration here. No thread, model request or MCP client
		// is started by this short-lived setup process.
		setup, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		child, err := startProcess(setup, path, o.Directory, []string{"app-server", "--listen", "stdio://"}, nil)
		if err != nil {
			return terminalLaunch{}, err
		}
		p := &codexProtocol{process: child}
		defer p.Close()
		if _, err = p.request(1, "initialize", map[string]any{"clientInfo": map[string]string{"name": "plous", "version": "1"}}); err != nil {
			return terminalLaunch{}, err
		}
		if err = p.send(map[string]string{"method": "initialized"}); err != nil {
			return terminalLaunch{}, err
		}
		raw, err := p.request(2, "config/read", map[string]any{"cwd": o.Directory, "includeLayers": false})
		if err != nil {
			return terminalLaunch{}, err
		}
		inherited, err := codexServers(raw)
		if err != nil {
			return terminalLaunch{}, err
		}
		name := "plous"
		for inherited[name] != nil {
			name += "_"
		}
		args := []string{"--no-daemon", "--sandbox", "read-only", "--ask-for-approval", "never", "-c", "sqlite_home=" + strconv.Quote(o.StateDirectory)}
		for _, f := range []string{"shell_tool", "unified_exec", "apply_patch_freeform", "multi_agent", "multi_agent_v2", "apps", "plugins", "remote_plugin", "hooks", "memories", "memory_tool", "js_repl", "code_mode_host", "code_mode_only", "sleep_tool", "view_image", "image_generation", "computer_use", "browser_use", "goals", "shell_snapshot", "shell_snapshot_v2", "workspace_dependencies", "request_permissions_tool"} {
			args = append(args, "-c", "features."+f+"=false")
		}
		args = append(args, "-c", "features.code_mode.enabled=false", "-c", `features.code_mode.direct_only_tool_namespaces=["mcp__`+name+`"]`, "-c", "agents.enabled=false", "-c", "features.skip_host_skill_discovery=true", "-c", `web_search="disabled"`, "-c", "project_doc_max_bytes=0", "-c", "check_for_update_on_startup=false", "-c", "notify=[]", "-c", "developer_instructions="+strconv.Quote(o.Instructions))
		servers := map[string]any{}
		for server, raw := range inherited {
			var entry map[string]any
			if json.Unmarshal(raw, &entry) != nil || entry == nil {
				return terminalLaunch{}, errors.New("invalid inherited Codex MCP server")
			}
			// Disabled entries need a valid transport but no inherited credentials.
			transport := map[string]any{"enabled": false, "required": false}
			if _, ok := entry["command"]; ok {
				transport["command"] = "plous-disabled-mcp"
			} else {
				transport["url"] = "http://127.0.0.1:1"
			}
			servers[server] = transport
		}
		servers[name] = map[string]any{"url": b.URL, "bearer_token_env_var": "PLOUS_TOOL_TOKEN", "enabled": true, "required": true, "startup_timeout_sec": 15, "tool_timeout_sec": 900}
		// The CLI splits dotted override keys literally, without TOML quoting.
		// One inline table preserves arbitrary inherited names and their transport.
		var encoded bytes.Buffer
		if err := toml.NewEncoder(&encoded).SetTablesInline(true).Encode(map[string]any{"mcp_servers": servers}); err != nil {
			return terminalLaunch{}, err
		}
		args = append(args, "-c", strings.TrimSpace(encoded.String()))

		launch.args = args
		return launch, nil
	}
	return terminalLaunch{}, errors.New("unknown external agent")
}

func codexServers(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var config struct {
		Config *struct {
			Servers map[string]json.RawMessage `json:"mcp_servers"`
		}
	}
	// Missing inventory must not leave inherited servers enabled on a changed protocol.
	if json.Unmarshal(raw, &config) != nil || config.Config == nil || config.Config.Servers == nil {
		return nil, errors.New("Codex returned an unsupported MCP configuration")
	}
	return config.Config.Servers, nil
}
