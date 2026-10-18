package plousbench

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
)

func decodeValue(raw []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var value any
	if err := d.Decode(&value); err != nil {
		return nil, err
	}
	if d.Decode(new(any)) != io.EOF {
		return nil, errors.New("expected one JSON value")
	}
	return value, nil
}

func Pointer(value any, path string) (any, bool) {
	if path == "" {
		return value, true
	}
	if !strings.HasPrefix(path, "/") {
		return nil, false
	}
	for _, part := range strings.Split(path[1:], "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch v := value.(type) {
		case map[string]any:
			var ok bool
			value, ok = v[part]
			if !ok {
				return nil, false
			}
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(v) {
				return nil, false
			}
			value = v[i]
		default:
			return nil, false
		}
	}
	return value, true
}

func Assess(step Step, body json.RawMessage, callErr error, reads int) []string {
	var failures []string
	if (callErr != nil) != step.Error {
		failures = append(failures, fmt.Sprintf("expected error=%t, got %v", step.Error, callErr))
	}
	if step.MinReads != nil && reads < *step.MinReads || step.MaxReads != nil && reads > *step.MaxReads {
		failures = append(failures, fmt.Sprintf("unexpected API read count: %d", reads))
	}
	if len(step.Checks) == 0 {
		return failures
	}
	value, err := decodeValue(body)
	if err != nil {
		return append(failures, "tool did not return valid JSON")
	}
	for _, check := range step.Checks {
		got, exists := Pointer(value, check.Path)
		passed := false
		switch {
		case check.Absent:
			passed = !exists
		case check.Equals != nil:
			want, err := decodeValue(check.Equals)
			passed = err == nil && exists && reflect.DeepEqual(got, want)
		case check.Contains != nil:
			text, ok := got.(string)
			passed = exists && ok && strings.Contains(text, *check.Contains)
		case check.Length != nil:
			list, ok := got.([]any)
			passed = exists && ok && len(list) == *check.Length
		}
		if !passed {
			failures = append(failures, fmt.Sprintf("%s: expected %s, got %s (present=%t)", check.Path, JSON(check), JSON(got), exists))
		}
	}
	return failures
}

func resolve(raw json.RawMessage, vars map[string]any) (json.RawMessage, error) {
	value, err := decodeValue(raw)
	if err != nil {
		return nil, err
	}
	var visit func(any) (any, error)
	visit = func(v any) (any, error) {
		switch x := v.(type) {
		case string:
			if strings.HasPrefix(x, "$") {
				r, ok := vars[x[1:]]
				if !ok {
					return nil, fmt.Errorf("unknown captured variable %s", x)
				}
				return r, nil
			}
		case map[string]any:
			for key, old := range x {
				next, err := visit(old)
				if err != nil {
					return nil, err
				}
				x[key] = next
			}
		case []any:
			for i, old := range x {
				next, err := visit(old)
				if err != nil {
					return nil, err
				}
				x[i] = next
			}
		}
		return v, nil
	}
	v, err := visit(value)
	if err != nil {
		return nil, err
	}
	return JSON(v), nil
}
