package debugging

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/tailscale/hujson"
)

const jsonMCPServers = "/mcpServers"

func (c setupClient) mergeJSONConfig(old []byte, launch serverLaunch) ([]byte, error) {
	doc, err := parseJSONConfig(old, c.format == jsoncConfig)
	if err != nil {
		return nil, err
	}
	if _, err = configJSONObject(doc.Find(jsonMCPServers)); err != nil {
		return nil, err
	}
	path := jsonMCPServers + "/" + serverName
	entry := doc.Find(path)
	fields, err := configJSONObject(entry)
	if err != nil {
		return nil, err
	}
	if entry != nil {
		var previous struct {
			serverLaunch
			Type string `json:"type"`
		}
		if decodeJSONValue(entry, &previous) != nil || (previous.Type != "" && previous.Type != "stdio") ||
			fields["url"] != nil || !c.manages(previous.serverLaunch, launch.Command) {
			return nil, unmanagedServerError()
		}
	}

	values := map[string]any{"command": launch.Command, "args": launch.Args}
	if entry == nil && c.mcpType != "" {
		values["type"] = c.mcpType
	}
	patch := patchJSONServer(&doc, jsonMCPServers, values)
	if c.kubeconfigEnv != "" && fields["envFile"] == nil {
		env, err := configJSONObject(fields["env"])
		if err != nil {
			return nil, err
		}
		if env["KUBECONFIG"] == nil {
			envPath, value := path+"/env/KUBECONFIG", any(c.kubeconfigEnv)
			if fields["env"] == nil {
				envPath, value = path+"/env", map[string]string{"KUBECONFIG": c.kubeconfigEnv}
			}
			patch = append(patch, jsonConfigEdit{Op: "add", Path: envPath, Value: value})
		}
	}
	return applyJSONConfigEdits(&doc, old, patch)
}

func parseJSONConfig(old []byte, allowComments bool) (hujson.Value, error) {
	if len(old) == 0 {
		old = []byte("{}")
	}
	doc, err := hujson.Parse(old)
	if err != nil || (!allowComments && !doc.IsStandard()) {
		if allowComments {
			return hujson.Value{}, errors.New("existing config must be a JSON or JSONC object")
		}
		return hujson.Value{}, errors.New("existing config must be a JSON object")
	}
	if _, err = configJSONObject(&doc); err != nil {
		return hujson.Value{}, err
	}
	return doc, nil
}

func configJSONObject(node *hujson.Value) (map[string]*hujson.Value, error) {
	if node == nil {
		return nil, nil
	}
	object, ok := node.Value.(*hujson.Object)
	if !ok {
		return nil, errors.New("MCP configuration must be an object")
	}
	fields := make(map[string]*hujson.Value, len(object.Members))
	for i := range object.Members {
		member := &object.Members[i]
		key := member.Name.Value.(hujson.Literal).String()
		if fields[key] != nil {
			return nil, fmt.Errorf("MCP configuration contains duplicate key %q", key)
		}
		fields[key] = &member.Value
	}
	return fields, nil
}

func decodeJSONValue(node *hujson.Value, target any) error {
	value := node.Clone()
	value.Standardize()
	return json.Unmarshal(value.Pack(), target)
}

type jsonConfigEdit struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value"`
}

func patchJSONServer(doc *hujson.Value, container string, fields map[string]any) []jsonConfigEdit {
	path := container + "/" + serverName
	if doc.Find(container) == nil {
		return []jsonConfigEdit{{Op: "add", Path: container, Value: map[string]any{serverName: fields}}}
	}
	if doc.Find(path) == nil {
		return []jsonConfigEdit{{Op: "add", Path: path, Value: fields}}
	}
	var patch []jsonConfigEdit
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		patch = append(patch, jsonConfigEdit{Op: "add", Path: path + "/" + key, Value: fields[key]})
	}
	return patch
}

func applyJSONConfigEdits(doc *hujson.Value, old []byte, edits []jsonConfigEdit) ([]byte, error) {
	patch, err := json.Marshal(edits)
	if err != nil {
		return nil, err
	}
	if err = doc.Patch(patch); err != nil {
		return nil, errors.New("could not update the MCP configuration")
	}
	if len(old) == 0 {
		doc.Format()
	}
	return doc.Pack(), nil
}
