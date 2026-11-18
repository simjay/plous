package observation

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// ParseArguments accepts one JSON argument object with unique keys and non-null values.
func ParseArguments(raw json.RawMessage) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("arguments must be an object")
	}
	args := map[string]json.RawMessage{}
	for d.More() {
		token, err = d.Token()
		name, ok := token.(string)
		if err != nil || !ok || args[name] != nil {
			return nil, errors.New("duplicate or invalid argument")
		}
		var value json.RawMessage
		if err = d.Decode(&value); err != nil || string(value) == "null" {
			return nil, errors.New("invalid argument")
		}
		args[name] = value
	}
	if _, err = d.Token(); err != nil {
		return nil, err
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, errors.New("expected one argument object")
	}
	return args, nil
}
