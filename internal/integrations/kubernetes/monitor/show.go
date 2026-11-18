package monitor

import (
	"encoding/json"
	"errors"
	"slices"
)

type showDefinition struct {
	view                string
	arguments, required []string
}

var showDefinitions = []showDefinition{
	{"resource", []string{"resource", "namespace", "name", "uid", "tab"}, []string{"resource", "name"}},
	{"list", []string{"resource", "namespace", "filter", "labels"}, []string{"resource"}},
	{"logs", []string{"resource", "namespace", "name", "uid", "container", "previous", "highlight"}, []string{"resource", "namespace", "name", "container"}},
}

var showTabs = []string{"describe", "yaml", "events", "related"}

func ShowSchema(view string) map[string]any {
	for _, d := range showDefinitions {
		if d.view != view {
			continue
		}
		properties := map[string]any{}
		for _, name := range d.arguments {
			properties[name] = map[string]any{"type": "string", "maxLength": 1024}
		}
		if view == "resource" {
			properties["tab"] = map[string]any{"type": "string", "enum": slices.Clone(showTabs)}
		}
		if view == "logs" {
			properties["resource"] = map[string]any{"type": "string", "const": "pods"}
			properties["previous"] = map[string]any{"type": "boolean"}
		}
		return map[string]any{"type": "object", "properties": properties, "required": slices.Clone(d.required), "additionalProperties": false}
	}
	panic("unknown Kubernetes view: " + view)
}

// ValidateShow receives domain arguments after the host removes connection_id.
func ValidateShow(args map[string]json.RawMessage) (string, error) {
	var q struct {
		View, Resource, Namespace, Name, Container, Tab, UID, Filter, Labels, Highlight string
		Previous                                                                        bool
	}
	raw, _ := json.Marshal(args)
	if err := json.Unmarshal(raw, &q); err != nil {
		return "", err
	}
	index := slices.IndexFunc(showDefinitions, func(d showDefinition) bool { return d.view == q.View })
	if index < 0 {
		return "", errors.New("unsupported Kubernetes view")
	}
	d := showDefinitions[index]
	for name := range args {
		if name != "view" && !slices.Contains(d.arguments, name) {
			return "", errors.New(q.View + " does not accept " + name)
		}
	}
	for _, name := range d.required {
		var value string
		if json.Unmarshal(args[name], &value) != nil || value == "" {
			return "", errors.New(q.View + " requires " + name)
		}
	}
	if q.View == "resource" && q.Tab != "" && !slices.Contains(showTabs, q.Tab) {
		return "", errors.New("unsupported resource detail tab")
	}
	if q.View == "logs" && (q.Resource != "pods" || q.Namespace == "*") {
		return "", errors.New("logs requires resource=pods and a concrete namespace")
	}
	return q.View, nil
}
