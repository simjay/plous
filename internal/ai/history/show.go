package history

import (
	"encoding/json"
	"errors"
	"slices"
)

const ShowHelp = "Open saved evidence by ref and optional offset within the captured history scope."

var showArguments = []string{"ref", "offset"}

func ShowSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"ref":    map[string]any{"type": "string", "minLength": 1, "maxLength": 1024},
		"offset": map[string]any{"type": "integer", "minimum": 0},
	}, "required": []string{"ref"}, "additionalProperties": false}
}

func ValidateShow(args map[string]json.RawMessage) (string, error) {
	for name := range args {
		if !slices.Contains(showArguments, name) {
			return "", errors.New("evidence presentation does not accept " + name)
		}
	}
	var q struct {
		Ref    string
		Offset int
	}
	raw, _ := json.Marshal(args)
	if err := json.Unmarshal(raw, &q); err != nil {
		return "", err
	}
	if q.Ref == "" || q.Offset < 0 {
		return "", errors.New("evidence requires a saved ref and non-negative offset")
	}
	return "evidence", nil
}
