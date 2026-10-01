package rlm

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"go.starlark.net/starlark"
)

// Walk values with byte, depth, and item limits before formatting. The default
// repr/JSON helpers can expand shared or cyclic collections without a bound.
func encodeValue(value starlark.Value, limit int) (string, error) {
	var output strings.Builder
	items := 0
	write := func(s string) error {
		if len(s) > limit-output.Len() {
			return errors.New("encoded value exceeds size limit")
		}
		output.WriteString(s)
		return nil
	}
	var encode func(starlark.Value, int) error
	encode = func(v starlark.Value, depth int) error {
		items++
		if depth > 32 || items > maxItems {
			return errors.New("value nesting or item count exceeds limit")
		}
		switch v := v.(type) {
		case starlark.NoneType:
			return write("null")
		case starlark.Bool:
			if v {
				return write("true")
			}
			return write("false")
		case starlark.String:
			if len(v) > limit-output.Len() {
				return errors.New("text exceeds size limit")
			}
			encoded, _ := json.Marshal(string(v))
			return write(string(encoded))
		case starlark.Int:
			if _, ok := v.Int64(); !ok {
				return errors.New("integer exceeds range")
			}
			return write(v.String())
		case starlark.Float:
			encoded, err := json.Marshal(float64(v))
			if err != nil {
				return err
			}
			return write(string(encoded))
		case *starlark.Dict:
			if v.Len() > maxItems-items {
				return errors.New("dictionary exceeds item limit")
			}
			if err := write("{"); err != nil {
				return err
			}
			iterator := v.Iterate()
			defer iterator.Done()
			var key starlark.Value
			count := 0
			for iterator.Next(&key) {
				if _, ok := key.(starlark.String); !ok {
					return errors.New("dictionary keys must be strings")
				}
				if count > 0 {
					if err := write(","); err != nil {
						return err
					}
				}
				if err := encode(key, depth+1); err != nil {
					return err
				}
				if err := write(":"); err != nil {
					return err
				}
				item, _, err := v.Get(key)
				if err != nil {
					return err
				}
				if err := encode(item, depth+1); err != nil {
					return err
				}
				count++
			}
			return write("}")
		case *starlark.List, starlark.Tuple:
			iterable := v.(starlark.Iterable)
			iterator := iterable.Iterate()
			defer iterator.Done()
			if err := write("["); err != nil {
				return err
			}
			var item starlark.Value
			count := 0
			for iterator.Next(&item) {
				if count > 0 {
					if err := write(","); err != nil {
						return err
					}
				}
				if err := encode(item, depth+1); err != nil {
					return err
				}
				count++
			}
			return write("]")
		default:
			return fmt.Errorf("cannot encode %s", v.Type())
		}
	}
	if err := encode(value, 0); err != nil {
		return "", err
	}
	return output.String(), nil
}

func decodeValue(input string) (starlark.Value, error) {
	if !json.Valid([]byte(input)) {
		return nil, errors.New("tool result is not valid JSON")
	}
	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.UseNumber()
	var raw any
	if err := decoder.Decode(&raw); err != nil {
		return nil, errors.New("tool result is not valid JSON")
	}
	items := 0
	var convert func(any, int) (starlark.Value, error)
	convert = func(value any, depth int) (starlark.Value, error) {
		items++
		if items > maxItems || depth > 32 {
			return nil, errors.New("JSON nesting or item count exceeds limit")
		}
		switch value := value.(type) {
		case nil:
			return starlark.None, nil
		case bool:
			return starlark.Bool(value), nil
		case string:
			return starlark.String(value), nil
		case json.Number:
			if integer, err := strconv.ParseInt(string(value), 10, 64); err == nil {
				return starlark.MakeInt64(integer), nil
			}
			number, err := value.Float64()
			if err != nil {
				return nil, err
			}
			return starlark.Float(number), nil
		case []any:
			values := make([]starlark.Value, 0, len(value))
			for _, item := range value {
				converted, err := convert(item, depth+1)
				if err != nil {
					return nil, err
				}
				values = append(values, converted)
			}
			return starlark.NewList(values), nil
		case map[string]any:
			values := starlark.NewDict(len(value))
			for key, item := range value {
				converted, err := convert(item, depth+1)
				if err != nil {
					return nil, err
				}
				if err := values.SetKey(starlark.String(key), converted); err != nil {
					return nil, err
				}
			}
			return values, nil
		default:
			return nil, errors.New("unsupported JSON value")
		}
	}
	return convert(raw, 0)
}
