package source

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

type readDefinition struct {
	tool                string
	operation           string
	arguments, required []string
}

var readDefinitions = []readDefinition{
	{"source_files_read", "list", []string{"path", "glob", "offset", "limit", "identity"}, nil},
	{"source_files_read", "read", []string{"path", "offset", "max_bytes", "identity"}, []string{"path"}},
	{"source_files_read", "search", []string{"path", "query", "glob", "regex", "case_sensitive", "limit", "cursor"}, []string{"query"}},
	{"source_analysis_read", "catalog", []string{"target", "offset", "limit"}, nil},
	{"source_analysis_read", "resources", []string{"target", "result", "resource", "offset", "limit"}, nil},
	{"source_analysis_read", "fields", []string{"target", "result", "resource", "field", "offset", "limit"}, []string{"resource"}},
	{"source_analysis_read", "findings", []string{"target", "offset", "limit"}, nil},
	{"source_analysis_read", "compare", []string{"target", "result", "resource", "other", "offset", "limit"}, []string{"other"}},
}

func validateReadArguments(c ReadRequest) error {
	data, _ := json.Marshal(c)
	var supplied map[string]json.RawMessage
	_ = json.Unmarshal(data, &supplied)
	return validateReadFields(c, supplied)
}

// Validate supplied fields before omitempty can erase explicit zero values.
func (c *ReadRequest) UnmarshalJSON(data []byte) error {
	type request ReadRequest
	var decoded request
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var supplied map[string]json.RawMessage
	if err := json.Unmarshal(data, &supplied); err != nil {
		return err
	}
	if err := validateReadFields(ReadRequest(decoded), supplied); err != nil {
		return err
	}
	*c = ReadRequest(decoded)
	return nil
}

func validateReadFields(c ReadRequest, supplied map[string]json.RawMessage) error {
	delete(supplied, "operation")
	for _, d := range readDefinitions {
		if d.operation != c.Operation {
			continue
		}
		for name, value := range supplied {
			if !slices.Contains(d.arguments, name) {
				return denySourceRead(fmt.Errorf("%s does not accept %s", c.Operation, name))
			}
			if string(value) == "null" {
				return denySourceRead(fmt.Errorf("%s cannot be null", name))
			}
		}
		for _, name := range d.required {
			if value, ok := supplied[name]; !ok || string(value) == `""` {
				return denySourceRead(fmt.Errorf("%s requires %s", c.Operation, name))
			}
		}
		if usesCapturedResult(c.Operation) && c.Target == "" && c.Result == "" {
			return denySourceRead(errors.New("analysis read requires target or result"))
		}
		return nil
	}
	return denySourceRead(errors.New("unsupported source read operation"))
}

func usesCapturedResult(operation string) bool {
	return operation == "resources" || operation == "fields" || operation == "compare"
}

func ReadOperations(tool string) []string {
	var operations []string
	for _, d := range readDefinitions {
		if d.tool == tool {
			operations = append(operations, d.operation)
		}
	}
	return operations
}
