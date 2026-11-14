package debugging

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/tailscale/hujson"
)

const openCodeMCP = "/mcp"
const openCodeV2Servers = "/mcp/servers"

// OpenCode loads its user files from lowest to highest priority in this order.
// Prefer an existing file and check every layer for a conflicting registration.
func (c setupClient) openCodeConfigFile(defaultPath, executable string) (string, error) {
	dir, selected := filepath.Dir(defaultPath), defaultPath
	var registered string
	for _, name := range []string{"config.json", "opencode.json", "opencode.jsonc"} {
		path := filepath.Join(dir, name)
		raw, err := readInstallFile(path)
		if err != nil {
			return "", err
		}
		if raw == nil {
			continue
		}
		_, serverPath, err := c.parseOpenCodeConfig(raw, executable)
		if err != nil {
			return "", fmt.Errorf("OpenCode configuration %s: %w", path, err)
		}
		selected = path
		if serverPath != "" {
			if registered != "" {
				return "", errors.New("plous_debug is registered in more than one OpenCode configuration file")
			}
			registered = path
		}
	}
	if registered != "" {
		return registered, nil
	}
	return selected, nil
}

func (c setupClient) parseOpenCodeConfig(old []byte, executable string) (hujson.Value, string, error) {
	doc, err := parseJSONConfig(old, true)
	if err != nil {
		return hujson.Value{}, "", err
	}
	for _, path := range []string{openCodeMCP, openCodeV2Servers} {
		if _, err := configJSONObject(doc.Find(path)); err != nil {
			return hujson.Value{}, "", err
		}
	}
	legacy, modern := doc.Find(openCodeMCP+"/"+serverName), doc.Find(openCodeV2Servers+"/"+serverName)
	if legacy != nil && modern != nil {
		return hujson.Value{}, "", errors.New("plous_debug is registered in both OpenCode MCP formats")
	}
	entry, container := legacy, openCodeMCP
	if modern != nil {
		entry, container = modern, openCodeV2Servers
	}
	if entry == nil {
		return doc, "", nil
	}
	if _, err := configJSONObject(entry); err != nil {
		return hujson.Value{}, "", err
	}
	var previous struct {
		Type    string   `json:"type"`
		Command []string `json:"command"`
	}
	if decodeJSONValue(entry, &previous) != nil || previous.Type != "local" || len(previous.Command) == 0 ||
		!c.manages(serverLaunch{Command: previous.Command[0], Args: previous.Command[1:]}, executable) {
		return hujson.Value{}, "", unmanagedServerError()
	}
	return doc, container, nil
}

func (c setupClient) mergeOpenCodeConfig(old []byte, launch serverLaunch) ([]byte, error) {
	doc, registered, err := c.parseOpenCodeConfig(old, launch.Command)
	if err != nil {
		return nil, err
	}
	container := openCodeMCP
	fields := map[string]any{"command": append([]string{launch.Command}, launch.Args...)}
	if registered != "" {
		return applyJSONConfigEdits(&doc, old, patchJSONServer(&doc, registered, fields))
	}
	fields["type"] = "local"
	fields["timeout"] = toolTimeout.Milliseconds()
	// V2 accepts the V1 form. Use the native V2 form in an existing V2 server map.
	if doc.Find(openCodeV2Servers) != nil {
		container = openCodeV2Servers
		fields["codemode"] = false
		fields["timeout"] = map[string]int64{"execution": toolTimeout.Milliseconds()}
	}
	return applyJSONConfigEdits(&doc, old, patchJSONServer(&doc, container, fields))
}
