package debugging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pelletier/go-toml/v2"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/simjay/plous/internal/ai/agent"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/skills"
)

const skillMarker = "<!-- Managed by plous setup. -->\n"
const configStart = "# BEGIN PLOUS DEBUG MCP"
const configEnd = "# END PLOUS DEBUG MCP"

func install(ctx context.Context, c configuration, target *kubecommon.Target, output io.Writer) error {
	if !c.skipCheck {
		client, err := kubernetes.NewTarget(target, c.namespace)
		if err != nil {
			return errors.New("cannot initialize Kubernetes connection")
		}
		check, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		if err = client.CheckConnection(check); err != nil {
			return errors.New("Kubernetes API connection check failed; check authentication and network access, or use --skip-check")
		}
	}
	home := c.home
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return err
		}
	}
	home, err := filepath.Abs(home)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return err
	}
	args := []string{"mcp", "serve", "--agent", string(c.kind), "--kubeconfig", c.paths, "--context", c.context, "--namespace", c.namespace, "--data-dir", c.dataDir}
	if c.policy.Exec {
		args = append(args, "--allow-exec")
	}
	if c.policy.Image != "" {
		args = append(args, "--diagnostic-image", c.policy.Image)
	}
	var configPath, skillPath string
	if c.kind == agent.Codex {
		configRoot := filepath.Join(home, ".codex")
		if c.home == "" && os.Getenv("CODEX_HOME") != "" {
			configRoot = os.Getenv("CODEX_HOME")
		}
		configPath = filepath.Join(configRoot, "config.toml")
		skillPath = filepath.Join(home, ".agents", "skills", "plous-debug", "SKILL.md")
	} else {
		configPath = filepath.Join(home, ".claude.json")
		skillPath = filepath.Join(home, ".claude", "skills", "plous-debug", "SKILL.md")
	}
	oldConfig, err := readInstallFile(configPath)
	if err != nil {
		return err
	}
	oldSkill, err := readInstallFile(skillPath)
	if err != nil {
		return err
	}
	if len(oldSkill) > 0 && !bytes.Equal(oldSkill, []byte(skills.DebugSkill)) && !bytes.Contains(oldSkill, []byte(skillMarker)) {
		return errors.New("plous-debug skill already exists and is not managed by Plous")
	}
	newConfig, err := mergeConfig(c.kind, oldConfig, executable, args)
	if err != nil {
		return err
	}
	// Keep YAML frontmatter at the start, including for clients that do not
	// accept comments before the frontmatter delimiter.
	skill := strings.Replace(skills.DebugSkill, "\n---\n", "\n---\n\n"+skillMarker, 1)
	if err = writeInstallFile(skillPath, []byte(skill)); err != nil {
		return err
	}
	if err = writeInstallFile(configPath, newConfig); err != nil {
		return fmt.Errorf("skill installed, but MCP configuration could not be saved: %w", err)
	}
	fmt.Fprintf(output, "Installed plous-debug for %s.\nSkill: %s\nMCP configuration: %s\nTarget: %s / %s\nExec enabled: %t\nTemporary diagnostics enabled: %t\nRestart or reconnect the agent.\n", c.kind.Label(), skillPath, configPath, c.context, c.namespace, c.policy.Exec, c.policy.Image != "")
	return nil
}

func mergeConfig(kind agent.Kind, old []byte, executable string, args []string) ([]byte, error) {
	const name = "plous_debug"
	if kind == agent.Codex {
		var existing map[string]any
		if len(old) > 0 {
			if err := toml.Unmarshal(old, &existing); err != nil {
				return nil, errors.New("existing Codex config is not valid TOML")
			}
		}
		text := string(old)
		start, end := strings.Index(text, configStart), strings.Index(text, configEnd)
		if start >= 0 || end >= 0 {
			if start < 0 || end < start || strings.Count(text, configStart) != 1 || strings.Count(text, configEnd) != 1 {
				return nil, errors.New("Plous MCP config markers are malformed")
			}
			text = text[:start] + text[end+len(configEnd):]
		} else if servers, ok := existing["mcp_servers"].(map[string]any); ok && servers[name] != nil {
			return nil, errors.New("Codex already has an unmanaged plous_debug MCP server")
		}
		section, err := toml.Marshal(map[string]any{"command": executable, "args": args, "tool_timeout_sec": 150})
		if err != nil {
			return nil, err
		}
		result := []byte(strings.TrimRight(text, "\n") + "\n\n" + configStart + "\n[mcp_servers.plous_debug]\n" + string(section) + configEnd + "\n")
		var check map[string]any
		if toml.Unmarshal(result, &check) != nil {
			return nil, errors.New("updated Codex config would be invalid")
		}
		return result, nil
	}
	config := map[string]json.RawMessage{}
	if len(old) > 0 && json.Unmarshal(old, &config) != nil {
		return nil, errors.New("existing Claude configuration is not valid JSON")
	}
	if config == nil {
		return nil, errors.New("existing Claude configuration must be an object")
	}
	servers := map[string]json.RawMessage{}
	if raw := config["mcpServers"]; raw != nil {
		if json.Unmarshal(raw, &servers) != nil || servers == nil {
			return nil, errors.New("existing Claude MCP configuration must be an object")
		}
	}
	if raw := servers[name]; raw != nil {
		var previous struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		}
		if json.Unmarshal(raw, &previous) != nil || previous.Command != executable && filepath.Base(previous.Command) != "plous" || len(previous.Args) < 2 || previous.Args[0] != "mcp" || previous.Args[1] != "serve" {
			return nil, errors.New("Claude already has an unmanaged plous_debug MCP server")
		}
	}
	servers[name], _ = json.Marshal(map[string]any{"type": "stdio", "command": executable, "args": args})
	config["mcpServers"], _ = json.Marshal(servers)
	result, err := json.MarshalIndent(config, "", "  ")
	return append(result, '\n'), err
}

func readInstallFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("installation target must be a regular file: %s", path)
	}
	return os.ReadFile(path)
}

func writeInstallFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), "plous-install-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(data)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
