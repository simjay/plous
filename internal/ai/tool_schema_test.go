package ai

import (
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/simjay/plous/internal/ai/agent"
	aimodel "github.com/simjay/plous/internal/ai/model"
	argo "github.com/simjay/plous/internal/integrations/argo/monitor"
	flux "github.com/simjay/plous/internal/integrations/flux/monitor"
	helm "github.com/simjay/plous/internal/integrations/helm/monitor"
	kube "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

func compileToolSchema(t *testing.T, parameters any) *jsonschema.Schema {
	t.Helper()
	raw, _ := json.Marshal(parameters)
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("urn:plous:test", doc); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("urn:plous:test")
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func TestCapabilityCatalogHasOneOwnerPerOperation(t *testing.T) {
	var names []string
	owners := map[string]string{}
	for _, d := range externalDefinitions() {
		names = append(names, d.Name)
		properties := d.parameters()["properties"].(map[string]any)
		for _, operation := range d.Operations {
			// Controller integrations share cluster access, but own their policies.
			domain := d.Owner
			if d.Name == "argo_resources_read" || d.Name == "flux_resources_read" {
				domain = d.Name
			}
			key := domain + "/" + operation
			if previous := owners[key]; previous != "" {
				t.Fatalf("%s belongs to both %s and %s", key, previous, d.Name)
			}
			owners[key] = d.Name
		}
		for _, field := range map[string][]string{
			"kubernetes_resources_read": {"container", "tail_lines", "max_bytes"},
			"kubernetes_logs_read":      {"label_selector", "field_selector", "continue"},
			"kubernetes_usage_read":     {"container", "tail_lines", "continue"},
			"helm_releases_read":        {"compare_revision"}, "helm_configuration_read": {"compare_revision"},
			"source_files_read":    {"target", "result", "field", "other"},
			"source_analysis_read": {"path", "query", "regex", "max_bytes"},
		}[d.Name] {
			if _, exists := properties[field]; exists {
				t.Fatalf("%s exposes unrelated argument %s", d.Name, field)
			}
		}
	}
	for _, d := range historyDefinitions() {
		names = append(names, d.Name)
	}
	for _, d := range uiDefinitions() {
		names = append(names, d.Name)
	}
	sort.Strings(names)
	want := []string{"argo_resources_read", "flux_resources_read", "helm_diff_read", "helm_configuration_read", "helm_releases_read", "history_read", "kubernetes_logs_read", "kubernetes_resources_read", "kubernetes_usage_read", "source_analysis_read", "source_files_read", "ui_read", "kubernetes_show", "helm_show", "source_show", "history_show", "workspace_read"}
	sort.Strings(want)
	if !reflect.DeepEqual(names, want) {
		t.Fatal(names)
	}
}

func TestCapabilityBoundaryRejectsBeforeAcquisition(t *testing.T) {
	var reads atomic.Int32
	op := deliveryFixture(t, func(http.ResponseWriter, *http.Request) { reads.Add(1) })
	op.o.Connections = append(op.o.Connections, Connection{Tool: "helm", ConnectionID: "helm"}, Connection{Tool: "source", ConnectionID: "source"})
	session := newToolSession(t.Context(), op)
	defer session.Close()
	for _, tc := range []struct{ tool, operation string }{
		{"kubernetes_resources_read", "logs"}, {"kubernetes_resources_read", "usage"},
		{"kubernetes_logs_read", "get"}, {"kubernetes_usage_read", "logs"},
		{"helm_releases_read", "values"}, {"helm_configuration_read", "compare"}, {"helm_diff_read", "status"},
		{"source_files_read", "fields"}, {"source_analysis_read", "read"},
		{"kubernetes_read", "get"}, {"helm_read", "status"}, {"source_read", "read"}, {"argo_read", "get"}, {"flux_read", "get"},
		{"kubernetes_resources", "get"}, {"kubernetes_logs", "logs"}, {"kubernetes_usage", "usage"},
		{"helm_releases", "list"}, {"helm_configuration", "values"}, {"helm_compare", "compare"},
		{"argo_resources", "get"}, {"flux_resources", "get"}, {"source_files", "list"}, {"source_analysis", "catalog"},
		{"history", "search"}, {"ui_show", "list"},
	} {
		body, err := session.Call(t.Context(), agent.ToolCall{ID: tc.tool + tc.operation, Name: tc.tool, Args: marshal(map[string]any{"purpose": "Verify boundary", "connection_id": "east", "operation": tc.operation})}, 65536)
		if err != nil || !strings.Contains(string(body), `"outcome":"denied"`) && !strings.Contains(string(body), `"error":"tool is not available in captured scope"`) {
			t.Fatalf("%s/%s: %s %v", tc.tool, tc.operation, body, err)
		}
	}
	if reads.Load() != 0 {
		t.Fatal("invalid tool operation contacted API")
	}
}

func TestIntegrationSchemasAndValidatorsAgreeOnOperationRequirements(t *testing.T) {
	scope := kube.CommandScope{ConnectionID: "cluster", Context: "fixture", Namespace: "shop"}
	for _, d := range kube.ReadCommands() {
		t.Run("kubernetes/"+d.Operation, func(t *testing.T) {
			input := map[string]any{"resource": "pods"}
			if len(kube.ReadOperations(d.Tool)) > 1 {
				input["operation"] = d.Operation
			}
			for _, field := range d.Required {
				if field != "resource" {
					input[field] = "app"
				}
			}
			schema := compileToolSchema(t, kube.ReadSchema(d.Tool))
			validate := func() bool {
				commandInput := map[string]any{"operation": d.Operation}
				for name, value := range input {
					commandInput[name] = value
				}
				data, _ := json.Marshal(commandInput)
				command, err := kube.DecodeCommand(string(data))
				return err == nil && kube.ValidateRead(command, scope) == nil
			}
			if schema.Validate(input) != nil || !validate() {
				t.Fatal("valid command rejected", input)
			}
			for _, field := range d.Required {
				value := input[field]
				delete(input, field)
				if schema.Validate(input) == nil || validate() {
					t.Fatal("missing required field accepted", input, field)
				}
				input[field] = value
			}
			input["revision"] = 1
			if schema.Validate(input) == nil || validate() {
				t.Fatal("unknown argument accepted")
			}
		})
	}
	for _, d := range helm.ReadCommands() {
		t.Run("helm/"+d.Operation, func(t *testing.T) {
			input := map[string]any{}
			if len(helm.ReadOperations(d.Tool)) > 1 {
				input["operation"] = d.Operation
			}
			for _, f := range d.Required {
				input[f] = "app"
			}
			schema := compileToolSchema(t, helm.ReadSchema(d.Tool))
			validate := func() bool {
				commandInput := map[string]any{"operation": d.Operation}
				for name, value := range input {
					commandInput[name] = value
				}
				data, _ := json.Marshal(commandInput)
				command, err := helm.DecodeCommand(string(data))
				return err == nil && helm.ValidateRead(command, helm.Config{ConnectionID: "cluster", Context: "fixture", Namespace: "shop"}) == nil
			}
			if schema.Validate(input) != nil || !validate() {
				t.Fatal("valid command rejected", input)
			}
			for _, field := range d.Required {
				delete(input, field)
				if schema.Validate(input) == nil || validate() {
					t.Fatal("missing required field accepted", field)
				}
			}
		})
	}
	for _, tool := range []struct {
		name, resource string
		schema         map[string]any
		validate       func(kube.Command) error
	}{
		{"argo", "applications.argoproj.io", argo.ReadSchema(), argo.ValidateRead},
		{"flux", "kustomizations.kustomize.toolkit.fluxcd.io", flux.ReadSchema(), flux.ValidateRead},
	} {
		schema := compileToolSchema(t, tool.schema)
		for _, op := range []string{"get", "describe", "usage"} {
			command := kube.Command{Operation: op, Resource: tool.resource, Name: "app"}
			data, _ := json.Marshal(command)
			var input any
			_ = json.Unmarshal(data, &input)
			want := op != "usage"
			if (schema.Validate(input) == nil) != want || (tool.validate(command) == nil) != want {
				t.Fatal(tool.name, op)
			}
		}
	}
}

func TestAdvertisedToolBoundsAndRequiredArguments(t *testing.T) {
	var definitions []agent.Tool
	for _, d := range externalDefinitions() {
		definitions = append(definitions, agent.Tool{Name: d.Name, Description: d.Description, Parameters: marshal(d.parameters())})
	}
	definitions = append(definitions, historyDefinitions()...)
	definitions = append(definitions, uiDefinitions()...)
	prepared, err := aimodel.Prepare(nil, definitions)
	if err != nil {
		t.Fatal(err)
	}
	schemas := map[string][]*jsonschema.Schema{}
	for i, d := range definitions {
		eino, err := prepared.Tools[i].ParamsOneOf.ToJSONSchema()
		if err != nil {
			t.Fatal(err)
		}
		for _, parameters := range []any{d.Parameters, eino} {
			raw, _ := json.Marshal(parameters)
			var root map[string]any
			if err := json.Unmarshal(raw, &root); err != nil || root["type"] != "object" {
				t.Fatal(d.Name, "must have an object schema", err)
			}
			// Claude Code drops MCP tools with these root keywords on supported
			// installations that do not enable its optional schema rewrite.
			for _, keyword := range []string{"anyOf", "oneOf", "allOf"} {
				if _, exists := root[keyword]; exists {
					t.Fatalf("%s has unsupported root %s", d.Name, keyword)
				}
			}
			schemas[d.Name] = append(schemas[d.Name], compileToolSchema(t, root))
		}
	}
	for _, tc := range []struct {
		tool, args string
		valid      bool
	}{
		{"kubernetes_logs_read", `{"resource":"pods","name":"app","container":"app","tail_lines":500}`, true},
		{"kubernetes_logs_read", `{"operation":"logs","resource":"pods","name":"app","container":"app"}`, false},
		{"kubernetes_usage_read", `{"operation":"usage","resource":"pods","name":"app"}`, false},
		{"kubernetes_logs_read", `{"resource":"pods","name":"app","container":"app","tail_lines":501}`, false},
		{"kubernetes_usage_read", `{"resource":"pods","name":"app","tail_lines":0}`, false},
		{"kubernetes_resources_read", `{"operation":"describe","resource":"pods"}`, false},
		{"source_files_read", `{"operation":"read"}`, false},
		{"source_files_read", `{"operation":"search","query":""}`, false},
		{"source_files_read", `{"operation":"search","query":"app","limit":101}`, false},
		{"source_files_read", `{"operation":"read","path":"README.md","max_bytes":0}`, true},
		{"source_files_read", `{"operation":"read","path":"README.md","max_bytes":16385}`, false},
		{"source_analysis_read", `{"operation":"fields","target":"one"}`, false},
		{"source_analysis_read", `{"operation":"compare","target":"one"}`, false},
		{"helm_diff_read", `{"name":"app","revision":3,"compare_revision":1}`, true},
		{"helm_diff_read", `{"operation":"compare","name":"app","revision":3,"compare_revision":1}`, false},
		{"helm_diff_read", `{"name":"app","compare_revision":1}`, false},
		{"helm_diff_read", `{"name":"app","revision":0,"compare_revision":1}`, false},
		{"helm_diff_read", `{"name":"app","revision":3,"compare_revision":0}`, false},
		{"helm_configuration_read", `{"operation":"values","name":"app","compare_revision":1}`, false},
		{"workspace_read", `{"operation":"activity","limit":0}`, true},
		{"workspace_read", `{"operation":"activity","limit":21}`, false},
		{"history_read", `{"operation":"compare","ref":"record:one:1"}`, false},
		{"history_read", `{"operation":"window","ref":"record:one:1","limit":101}`, false},
		{"history_read", `{"operation":"read","ref":"record:one:1","limit":0}`, true},
	} {
		t.Run(tc.tool+tc.args, func(t *testing.T) {
			var input map[string]any
			_ = json.Unmarshal([]byte(tc.args), &input)
			input["purpose"] = "Inspect captured data"
			if tc.tool != "workspace_read" && tc.tool != "history_read" {
				input["connection_id"] = "fixture"
			}
			for _, schema := range schemas[tc.tool] {
				if err := schema.Validate(input); (err == nil) != tc.valid {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestToolInputPolicyDoesNotDependOnSchema(t *testing.T) {
	for _, d := range externalDefinitions() {
		t.Run(d.Name, func(t *testing.T) {
			d.Schema = func() map[string]any { t.Fatal("execution generated a schema"); return nil }
			input := map[string]any{"connection_id": "fixture"}
			if len(d.Operations) > 1 {
				input["operation"] = d.Operations[0]
			}
			prepared, err := d.prepareInput(string(marshal(input)))
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal([]byte(prepared), &got); err != nil || got["operation"] != d.Operations[0] {
				t.Fatal(prepared, err)
			}
			input["operation"] = "not-allowed"
			if _, err := d.prepareInput(string(marshal(input))); err == nil {
				t.Fatal("accepted an unsupported operation")
			}
			if len(d.Operations) == 1 {
				input["operation"] = d.Operations[0]
				if _, err := d.prepareInput(string(marshal(input))); err == nil {
					t.Fatal("accepted a redundant operation")
				}
			}
		})
	}
}
