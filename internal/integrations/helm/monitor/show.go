package monitor

import (
	"encoding/json"
	"errors"
	"slices"
)

const ShowHelp = "Open a comparison of two exact retained Helm revisions of one release. revision selects the after revision; compare_revision selects the before revision, matching helm_diff_read. Requires a release name, concrete namespace and two different positive revisions. tab selects values or manifests. Release lists are not supported."

var showArguments = []string{"namespace", "name", "revision", "compare_revision", "tab"}
var showRequired = []string{"namespace", "name", "revision", "compare_revision"}
var showTabs = []string{"values", "manifests"}

func ShowSchema() map[string]any {
	properties := map[string]any{}
	for _, name := range showArguments {
		properties[name] = map[string]any{"type": "string", "maxLength": 1024}
	}
	for _, name := range []string{"revision", "compare_revision"} {
		properties[name] = map[string]any{"type": "integer", "minimum": 1}
	}
	properties["tab"].(map[string]any)["enum"] = slices.Clone(showTabs)
	return map[string]any{"type": "object", "properties": properties, "required": slices.Clone(showRequired), "additionalProperties": false}
}

func ValidateShow(args map[string]json.RawMessage) (string, error) {
	for name := range args {
		if !slices.Contains(showArguments, name) {
			return "", errors.New("Helm comparison does not accept " + name)
		}
	}
	var q struct {
		Namespace, Name, Tab string
		Revision             int
		CompareRevision      int `json:"compare_revision"`
	}
	raw, _ := json.Marshal(args)
	if err := json.Unmarshal(raw, &q); err != nil {
		return "", err
	}
	if q.Name == "" || q.Namespace == "" || q.Namespace == "*" || q.Revision < 1 || q.CompareRevision < 1 || q.Revision == q.CompareRevision || q.Tab != "" && !slices.Contains(showTabs, q.Tab) {
		return "", errors.New("compare requires a release name, concrete namespace, two different positive revisions, and values or manifests")
	}
	return "compare", nil
}
