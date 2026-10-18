package monitor

import (
	"encoding/json"
	"errors"
	"slices"
)

const ShowHelp = "Open Kubernetes resource details, a filtered resource list or Pod logs. Use an API-qualified resource. Objects need a concrete namespace; logs need resource=pods and an explicit container. list filter is a case-insensitive text filter; labels is a label selector."

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

func ShowSchema() map[string]any {
	properties := map[string]any{}
	var views []string
	var variants []any
	for _, d := range showDefinitions {
		views = append(views, d.view)
		for _, name := range d.arguments {
			properties[name] = map[string]any{"type": "string", "maxLength": 1024}
		}
	}
	properties["previous"] = map[string]any{"type": "boolean"}
	properties["tab"].(map[string]any)["enum"] = slices.Clone(showTabs)
	for _, d := range showDefinitions {
		branch := map[string]any{"view": map[string]any{"enum": []string{d.view}}}
		for name := range properties {
			if !slices.Contains(d.arguments, name) {
				branch[name] = false
			}
		}
		if d.view == "logs" {
			branch["resource"] = map[string]any{"enum": []string{"pods"}}
		}
		variants = append(variants, map[string]any{"properties": branch, "required": d.required})
	}
	properties["view"] = map[string]any{"type": "string", "enum": views}
	return map[string]any{"type": "object", "properties": properties, "required": []string{"view"}, "additionalProperties": false, "dependentSchemas": map[string]any{"view": map[string]any{"anyOf": variants}}}
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
