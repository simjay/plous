package history

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/simjay/plous/internal/integrations/observation"
)

func TestHistoryOperationArguments(t *testing.T) {
	raw, _ := json.Marshal(ReadSchema())
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("urn:history:read", doc); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("urn:history:read")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		input string
		valid bool
	}{
		{`{"operation":"search"}`, true},
		{`{"operation":"search","query":"pods","offset":0,"limit":100}`, true},
		{`{"operation":"read","ref":"record:c:r","fields":[],"offset":0,"limit":0}`, true},
		{`{"operation":"window","ref":"record:c:r","offset":0,"limit":2}`, true},
		{`{"operation":"count","ref":"record:c:r"}`, true},
		{`{"operation":"count","ref":"record:c:r","field":"ready","equals":{"boolean":false}}`, true},
		{`{"operation":"compare","ref":"record:c:r","other":"record:c:s","field":"phase","equals":{"text":"Running"}}`, true},
		{`{"operation":"search","ref":"record:c:r"}`, false},
		{`{"operation":"search","fields":[]}`, false},
		{`{"operation":"read","ref":"record:c:r","field":"phase","equals":{"text":"NotARealPhase"}}`, false},
		{`{"operation":"read","ref":"record:c:r","query":"Running"}`, false},
		{`{"operation":"read","ref":"record:c:r","other":"record:c:s"}`, false},
		{`{"operation":"window","ref":"record:c:r","fields":[]}`, false},
		{`{"operation":"window","ref":"record:c:r","query":"pods"}`, false},
		{`{"operation":"count","ref":"record:c:r","query":"pods"}`, false},
		{`{"operation":"count","ref":"record:c:r","offset":0}`, false},
		{`{"operation":"count","ref":"record:c:r","limit":0}`, false},
		{`{"operation":"count","ref":"record:c:r","fields":[]}`, false},
		{`{"operation":"count","ref":"record:c:r","other":"record:c:s"}`, false},
		{`{"operation":"count","ref":"record:c:r","field":"phase"}`, false},
		{`{"operation":"count","ref":"record:c:r","equals":{"text":"Running"}}`, false},
		{`{"operation":"count","ref":"record:c:r","field":"","equals":{"text":"Running"}}`, false},
		{`{"operation":"count","ref":"record:c:r","field":"phase","equals":{"text":"Running","extra":true}}`, false},
		{`{"operation":"count","ref":"record:c:r","field":"phase","equals":{"text":null}}`, false},
		{`{"operation":"compare","ref":"record:c:r"}`, false},
		{`{"operation":"compare","ref":"record:c:r","other":"record:c:s","limit":0}`, false},
		{`{"operation":"read"}`, false},
		{`{"operation":"read","ref":""}`, false},
		{`{"operation":"read","ref":"record:c:r","fields":null}`, false},
		{`{"operation":"read","ref":"record:c:r","Limit":0}`, false},
	} {
		t.Run(tc.input, func(t *testing.T) {
			var input any
			if err := json.Unmarshal([]byte(tc.input), &input); err != nil {
				t.Fatal(err)
			}
			if got := schema.Validate(input) == nil; got != tc.valid {
				t.Fatalf("schema accepted=%v, want %v", got, tc.valid)
			}
			var q HistoryRequest
			err := json.Unmarshal([]byte(tc.input), &q)
			if (err == nil) != tc.valid {
				t.Fatalf("decode: %v", err)
			}
			if err == nil && q.validate() != nil {
				t.Fatal("typed request rejected", q)
			}
		})
	}
}

func TestHistoryAccessRejectsInappropriateArgumentsBeforeStorage(t *testing.T) {
	value := observation.Text("Running")
	for _, q := range []HistoryRequest{
		{Operation: "read", Ref: "record:c:r", Field: "phase", Equals: &value},
		{Operation: "search", Ref: "record:c:r"},
		{Operation: "count", Ref: "record:c:r", Limit: 1},
		{Operation: "compare", Ref: "record:c:r", Other: "record:c:s", Fields: []string{}},
	} {
		// No storage is installed; invalid requests must never reach it.
		_, err := (&Access{}).Call(t.Context(), q, 65536)
		var invalid *QueryError
		if !errors.As(err, &invalid) {
			t.Fatalf("%+v: %v", q, err)
		}
	}
}
