package manifest

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/simjay/plous/internal/integrations/observation"
)

type Field struct {
	Pointer string
	Value   observation.Value
}

// Fields projects selected configuration into masked scalar fields. Callers
// choose the object or subtree; this is not an unrestricted API-body reader.
func Fields(value any) ([]Field, bool) { return scalarFields(Redact(value)) }

func scalarFields(value any) ([]Field, bool) {
	var fields []Field
	complete := true
	var visit func(any, string, int)
	visit = func(value any, pointer string, depth int) {
		if len(fields) >= 10000 || depth > 32 {
			complete = false
			return
		}
		switch v := value.(type) {
		case map[string]any:
			if len(v) == 0 {
				fields = append(fields, Field{pointer, observation.Text("{}")})
				return
			}
			keys := make([]string, 0, len(v))
			for key := range v {
				keys = append(keys, key)
			}
			slices.Sort(keys)
			for _, key := range keys {
				visit(v[key], pointer+"/"+strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1"), depth+1)
			}
		case []any:
			if len(v) == 0 {
				fields = append(fields, Field{pointer, observation.Text("[]")})
				return
			}
			for n, child := range v {
				visit(child, fmt.Sprintf("%s/%d", pointer, n), depth+1)
			}
		default:
			fact := observation.Unknown()
			switch x := v.(type) {
			case string:
				if len(x) <= 1024 {
					fact = observation.Text(x)
				} else {
					complete = false
				}
			case bool:
				fact = observation.Boolean(x)
			case int:
				fact = observation.Number(int64(x))
			case int64:
				fact = observation.Number(x)
			case float64:
				if x == float64(int64(x)) {
					fact = observation.Number(int64(x))
				} else {
					fact = observation.Text(strconv.FormatFloat(x, 'g', -1, 64))
				}
			case json.Number:
				if n, err := x.Int64(); err == nil {
					fact = observation.Number(n)
				} else {
					fact = observation.Text(string(x))
				}
			}
			fields = append(fields, Field{pointer, fact})
		}
	}
	visit(value, "", 0)
	return fields, complete
}

type FieldChange struct {
	Pointer             string
	Before, After       observation.Value
	HadBefore, HasAfter bool
}

// FieldChanges compares document positions. It makes no list-element lineage
// claim and retains a marker for changes hidden by masking.
func FieldChanges(before, after any) ([]FieldChange, bool) {
	left, right := Redact(before), Redact(after)
	right = markHiddenChanges(before, after, left, right)
	a, ac := scalarFields(left)
	b, bc := scalarFields(right)
	if before == nil {
		a = nil
	}
	if after == nil {
		b = nil
	}
	old, next := map[string]observation.Value{}, map[string]observation.Value{}
	keys := []string{}
	for _, f := range a {
		old[f.Pointer] = f.Value
		keys = append(keys, f.Pointer)
	}
	for _, f := range b {
		next[f.Pointer] = f.Value
		keys = append(keys, f.Pointer)
	}
	slices.Sort(keys)
	var changes []FieldChange
	for _, key := range slices.Compact(keys) {
		a, had := old[key]
		b, has := next[key]
		if had == has && reflect.DeepEqual(a, b) {
			continue
		}
		if !had {
			a = observation.Unknown()
		}
		if !has {
			b = observation.Unknown()
		}
		changes = append(changes, FieldChange{key, a, b, had, has})
	}
	return changes, ac && bc
}
