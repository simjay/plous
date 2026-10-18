package source

import (
	"encoding/json"
	"errors"
	"slices"
)

const ShowHelp = "Open a captured source's relative file path at an optional 1-based line. identity optionally checks the file content identity."

var showArguments = []string{"path", "line", "identity"}

func ShowSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"path":     map[string]any{"type": "string", "minLength": 1, "maxLength": 1024},
		"line":     map[string]any{"type": "integer", "minimum": 0},
		"identity": map[string]any{"type": "string", "maxLength": 1024},
	}, "required": []string{"path"}, "additionalProperties": false}
}

func ValidateShow(args map[string]json.RawMessage) (string, error) {
	for name := range args {
		if !slices.Contains(showArguments, name) {
			return "", errors.New("source presentation does not accept " + name)
		}
	}
	var q struct {
		Path, Identity string
		Line           int
	}
	raw, _ := json.Marshal(args)
	if err := json.Unmarshal(raw, &q); err != nil {
		return "", err
	}
	if q.Path == "" || q.Line < 0 {
		return "", errors.New("source requires a relative path and optional 1-based line")
	}
	return "source", nil
}
