package history

import (
	"encoding/json"
	"slices"
)

type readDefinition struct {
	operation           string
	arguments, required []string
}

var readDefinitions = []readDefinition{
	{"search", []string{"query", "offset", "limit"}, nil},
	{"read", []string{"ref", "offset", "limit", "fields"}, []string{"ref"}},
	{"window", []string{"ref", "offset", "limit"}, []string{"ref"}},
	{"count", []string{"ref", "field", "equals"}, []string{"ref"}},
	{"compare", []string{"ref", "other", "field", "equals"}, []string{"ref", "other"}},
}

// Check supplied fields before decoding can erase explicit zero values.
func (q *HistoryRequest) UnmarshalJSON(data []byte) error {
	type request HistoryRequest
	var decoded request
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var supplied map[string]json.RawMessage
	if err := json.Unmarshal(data, &supplied); err != nil {
		return err
	}
	if err := validateReadFields(HistoryRequest(decoded), supplied); err != nil {
		return err
	}
	*q = HistoryRequest(decoded)
	return nil
}

func (q HistoryRequest) validate() error {
	data, _ := json.Marshal(q)
	var supplied map[string]json.RawMessage
	_ = json.Unmarshal(data, &supplied)
	if q.Fields == nil {
		delete(supplied, "fields")
	}
	return validateReadFields(q, supplied)
}

func validateReadFields(q HistoryRequest, supplied map[string]json.RawMessage) error {
	for _, d := range readDefinitions {
		if d.operation != q.Operation {
			continue
		}
		for name, value := range supplied {
			if name != "operation" && !slices.Contains(d.arguments, name) {
				return invalidQuery(q.Operation + " does not accept " + name)
			}
			if string(value) == "null" {
				return invalidQuery(name + " cannot be null")
			}
		}
		for _, name := range d.required {
			if value, ok := supplied[name]; !ok || string(value) == `""` {
				return invalidQuery(q.Operation + " requires " + name)
			}
		}
		_, field := supplied["field"]
		_, equals := supplied["equals"]
		if field != equals || field && q.Field == "" {
			return invalidQuery("field requires a non-empty name and a typed equals value")
		}
		if equals {
			var value map[string]json.RawMessage
			if json.Unmarshal(supplied["equals"], &value) != nil || len(value) != 1 {
				return invalidQuery("equals must contain exactly one typed value")
			}
			for name, raw := range value {
				if !slices.Contains([]string{"text", "number", "boolean"}, name) || string(raw) == "null" {
					return invalidQuery("equals must contain text, number or boolean")
				}
			}
		}
		return nil
	}
	return invalidQuery("unsupported history operation")
}
