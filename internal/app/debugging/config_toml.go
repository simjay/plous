package debugging

import (
	"bytes"
	"errors"
	"slices"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

const tomlMCPServers = "mcp_servers"

type tomlConfig struct{ forwardKubeconfig bool }

func (c tomlConfig) merge(old []byte, launch serverLaunch, owns func(serverLaunch) bool) ([]byte, error) {
	var settings map[string]any
	if toml.Unmarshal(old, &settings) != nil {
		return nil, errors.New("existing config is not valid TOML")
	}
	servers, ok := settings[tomlMCPServers].(map[string]any)
	if settings[tomlMCPServers] != nil && !ok {
		return nil, errors.New("mcp_servers must be a TOML table")
	}
	entry, ok := servers[serverName].(map[string]any)
	previous := tomlServerLaunch(entry)
	if servers[serverName] != nil && (!ok || !owns(previous)) {
		return nil, unmanagedServerError()
	}

	fields := map[string]any{}
	if previous.Command != launch.Command {
		fields["command"] = launch.Command
	}
	if !slices.Equal(previous.Args, launch.Args) {
		fields["args"] = launch.Args
	}
	if c.forwardKubeconfig && entry == nil {
		fields["env_vars"] = []string{"KUBECONFIG"}
	}
	if len(fields) == 0 {
		return old, nil
	}
	if entry == nil {
		fields["tool_timeout_sec"] = int(toolTimeout.Seconds())
	}
	result, err := editTOMLServer(old, entry != nil, fields)
	if err != nil {
		return nil, err
	}
	// Verify that the edited values belong to the intended semantic table.
	var parsed map[string]any
	if toml.Unmarshal(result, &parsed) != nil {
		return nil, errors.New("updated config would be invalid TOML")
	}
	installedServers, _ := parsed[tomlMCPServers].(map[string]any)
	installedEntry, _ := installedServers[serverName].(map[string]any)
	installed := tomlServerLaunch(installedEntry)
	if installed.Command != launch.Command || !slices.Equal(installed.Args, launch.Args) {
		return nil, errors.New("updated config does not contain the requested Plous MCP launch")
	}
	return result, nil
}

func tomlServerLaunch(entry map[string]any) serverLaunch {
	command, _ := entry["command"].(string)
	raw, _ := entry["args"].([]any)
	args := make([]string, 0, len(raw))
	for _, arg := range raw {
		value, ok := arg.(string)
		if !ok {
			return serverLaunch{}
		}
		args = append(args, value)
	}
	return serverLaunch{Command: command, Args: args}
}

type tomlEdit struct {
	start, end int
	text       []byte
}

func editTOMLServer(old []byte, exists bool, fields map[string]any) ([]byte, error) {
	var edits []tomlEdit
	var parser unstable.Parser
	parser.Reset(old)
	var table []string
	for parser.NextExpression() {
		expr := parser.Expression()
		switch expr.Kind {
		case unstable.Table, unstable.ArrayTable:
			table, _ = tomlKey(nil, expr)
		case unstable.KeyValue:
			if err := collectTOMLEdits(old, table, expr, exists, fields, &edits); err != nil {
				return nil, err
			}
		}
	}
	if parser.Error() != nil {
		return nil, errors.New("existing config is not valid TOML")
	}
	if len(edits) == 0 {
		if exists {
			return nil, errors.New("could not locate the Plous MCP launch in the TOML configuration")
		}
		body, err := toml.Marshal(fields)
		if err != nil {
			return nil, err
		}
		prefix := ""
		if len(old) != 0 {
			prefix = "\n\n"
			if bytes.HasSuffix(old, []byte("\n")) {
				prefix = "\n"
			}
		}
		section := append([]byte(prefix+"["+tomlMCPServers+"."+serverName+"]\n"), body...)
		return append(bytes.Clone(old), section...), nil
	}
	// Edits follow parser order. Apply them from the end to keep offsets valid.
	result := bytes.Clone(old)
	for i := len(edits) - 1; i >= 0; i-- {
		edit := edits[i]
		result = slices.Replace(result, edit.start, edit.end, edit.text...)
	}
	return result, nil
}

func collectTOMLEdits(old []byte, prefix []string, node *unstable.Node, exists bool, fields map[string]any, edits *[]tomlEdit) error {
	path, key := tomlKey(prefix, node)
	server := []string{tomlMCPServers, serverName}
	switch {
	case len(path) == 3 && slices.Equal(path[:2], server):
		name := path[2]
		if value := fields[name]; value != nil {
			assignment, err := tomlField(name, value)
			if err != nil {
				return err
			}
			_, encoded, _ := bytes.Cut(assignment, []byte(" = "))
			end := int(node.Raw.Offset + node.Raw.Length)
			tail := old[key.Raw.Offset+key.Raw.Length : end]
			_, original, _ := bytes.Cut(tail, []byte("="))
			start := end - len(bytes.TrimLeft(original, " \t"))
			*edits = append(*edits, tomlEdit{start, end, encoded})
		}
	case len(path) <= 2 && slices.Equal(path, server[:len(path)]) &&
		node.Value().Kind == unstable.InlineTable:
		value := node.Value()
		if !exists && len(path) == 1 {
			assignment, err := tomlField(serverName, fields)
			if err != nil {
				return err
			}
			text := append([]byte(" "), assignment...)
			if value.Child() != nil {
				text = append(text, ',')
			}
			text = append(text, ' ')
			at := int(value.Raw.Offset) + 1
			*edits = append(*edits, tomlEdit{at, at, text})
		}
		for children := value.Children(); children.Next(); {
			if err := collectTOMLEdits(old, path, children.Node(), exists, fields, edits); err != nil {
				return err
			}
		}
	}
	return nil
}

func tomlKey(prefix []string, node *unstable.Node) ([]string, *unstable.Node) {
	path := slices.Clone(prefix)
	var key *unstable.Node
	for parts := node.Key(); parts.Next(); {
		key = parts.Node()
		path = append(path, string(key.Data))
	}
	return path, key
}

func tomlField(name string, value any) ([]byte, error) {
	var data bytes.Buffer
	err := toml.NewEncoder(&data).SetTablesInline(true).Encode(map[string]any{name: value})
	return bytes.TrimSuffix(data.Bytes(), []byte("\n")), err
}
