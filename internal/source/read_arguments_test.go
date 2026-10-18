package source

import (
	"encoding/json"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestSourceOperationRequirementsAgreeWithSchema(t *testing.T) {
	schemas := map[string]*jsonschema.Schema{}
	for _, tool := range []string{"source_files_read", "source_analysis_read"} {
		var doc any
		raw, _ := json.Marshal(ReadSchema(tool))
		_ = json.Unmarshal(raw, &doc)
		compiler := jsonschema.NewCompiler()
		if err := compiler.AddResource("urn:source:test", doc); err != nil {
			t.Fatal(err)
		}
		schema, err := compiler.Compile("urn:source:test")
		if err != nil {
			t.Fatal(err)
		}
		schemas[tool] = schema
	}
	for _, tc := range []struct {
		input string
		valid bool
	}{
		{`{"operation":"list","glob":"*.yaml","limit":100}`, true},
		{`{"operation":"list","query":"secret"}`, false},
		{`{"operation":"list","regex":false}`, false},
		{`{"operation":"list","glob":null}`, false},
		{`{"operation":"read","path":"file.yaml","identity":"revision","max_bytes":1}`, true},
		{`{"operation":"read"}`, false},
		{`{"operation":"read","path":"file.yaml","limit":2}`, false},
		{`{"operation":"read","path":"file.yaml","limit":0}`, false},
		{`{"operation":"read","path":""}`, false},
		{`{"operation":"search","query":"app","regex":true,"case_sensitive":false}`, true},
		{`{"operation":"search","query":"app","offset":1}`, false},
		{`{"operation":"search"}`, false},
		{`{"operation":"catalog","target":"app"}`, true},
		{`{"operation":"catalog","result":"app"}`, false},
		{`{"operation":"resources","result":"app","resource":"Deployment/app"}`, true},
		{`{"operation":"fields","target":"app","resource":"Deployment/app","field":"/spec/replicas"}`, true},
		{`{"operation":"fields","target":"app"}`, false},
		{`{"operation":"findings","target":"app"}`, true},
		{`{"operation":"findings","resource":"app"}`, false},
		{`{"operation":"compare","target":"app","other":"previous"}`, true},
		{`{"operation":"compare","target":"app"}`, false},
	} {
		t.Run(tc.input, func(t *testing.T) {
			var value any
			var request ReadRequest
			_ = json.Unmarshal([]byte(tc.input), &value)
			err := json.Unmarshal([]byte(tc.input), &request)
			valid := err == nil && validateReadArguments(request) == nil
			var fields map[string]any
			_ = json.Unmarshal([]byte(tc.input), &fields)
			var schema *jsonschema.Schema
			for _, d := range readDefinitions {
				if d.operation == fields["operation"] {
					schema = schemas[d.tool]
				}
			}
			if (schema.Validate(value) == nil) != tc.valid || valid != tc.valid {
				t.Fatal("schema/validator disagreement", tc)
			}
		})
	}
}
