package monitor

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/simjay/plous/internal/integrations/observation"
)

var pointerEscape = strings.NewReplacer("~1", "/", "~0", "~")

func validPointer(pointer string) bool {
	if !strings.HasPrefix(pointer, "/") {
		return false
	}
	for i := 0; i < len(pointer); i++ {
		if pointer[i] == '~' {
			i++
			if i == len(pointer) || pointer[i] != '0' && pointer[i] != '1' {
				return false
			}
		}
	}
	return true
}

func (e *Projector) projectPaths(object map[string]any, facts map[string]observation.Value, paths []string) {
	for _, pointer := range paths {
		value, status := e.pathFact(object, pointer)
		facts["path_status."+pointer] = observation.Text(status)
		if status == "found" {
			facts[pointer] = value
		}
	}
}

func (e *Projector) pathFact(object map[string]any, pointer string) (observation.Value, string) {
	var value any = object
	for _, part := range strings.Split(pointer[1:], "/") {
		key := pointerEscape.Replace(part)
		switch node := value.(type) {
		case map[string]any:
			if isSecretObject(node) || manifestMask.hidesKey(key) {
				return observation.Unknown(), "redacted"
			}
			child, ok := node[key]
			if !ok {
				return observation.Unknown(), "missing"
			}
			value = child
		case []any:
			index, err := strconv.Atoi(key)
			if err != nil || index < 0 || index >= len(node) || strconv.Itoa(index) != key {
				return observation.Unknown(), "missing"
			}
			value = node[index]
		default:
			return observation.Unknown(), "missing"
		}
	}
	switch v := value.(type) {
	case string:
		if len(v) > 1024 {
			return observation.Unknown(), "too_large"
		}
		if e.maskedFieldText(v, manifestMask) != v {
			return observation.Unknown(), "redacted"
		}
		return observation.Text(v), "found"
	case bool:
		return observation.Boolean(v), "found"
	case int64:
		return observation.Number(v), "found"
	case float64:
		if v == float64(int64(v)) {
			return observation.Number(int64(v)), "found"
		}
		return observation.Text(strconv.FormatFloat(v, 'g', -1, 64)), "found"
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return observation.Number(n), "found"
		}
		return observation.Text(string(v)), "found"
	case nil:
		return observation.Unknown(), "null"
	default:
		return observation.Unknown(), "non_scalar"
	}
}
