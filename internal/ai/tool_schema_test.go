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
	"github.com/simjay/plous/internal/integrations/observation"
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
	o := Options{WorkspaceID: "scope", UI: &testUI{}, Connections: []Connection{{Tool: "kubernetes"}, {Tool: "helm"}}}
	var names []string
	for _, definition := range ToolDefinitions(o) {
		names = append(names, definition.Name)
	}
	sort.Strings(names)
	want := []string{"argo_read", "flux_read", "helm_read", "history_read", "kubernetes_read", "plous_view", "workspace_read"}
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
		{"kubernetes_resources_read", "get"}, {"kubernetes_logs_read", "logs"}, {"kubernetes_usage_read", "usage"},
		{"helm_releases_read", "list"}, {"helm_configuration_read", "values"}, {"helm_diff_read", "compare"},
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
			input := map[string]any{}
			if d.Operation != "discover" {
				input["resource"] = "pods"
			}
			input["operation"] = d.Operation
			for _, field := range d.Required {
				if field != "resource" {
					input[field] = "app"
				}
			}
			schema := compileToolSchema(t, kube.ReadSchema())
			validate := func() bool {
				commandInput := map[string]any{"operation": d.Operation}
				for name, value := range input {
					commandInput[name] = value
				}
				args, parseErr := observation.ParseArguments(marshal(commandInput))
				if parseErr != nil {
					t.Fatal(parseErr)
				}
				command, err := kube.DecodeCommand(args)
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
			input["operation"] = d.Operation
			for _, f := range d.Required {
				input[f] = "app"
				if f == "view" {
					input[f] = "status"
				}
			}
			schema := compileToolSchema(t, helm.ReadSchema())
			validate := func() bool {
				commandInput := map[string]any{"operation": d.Operation}
				for name, value := range input {
					commandInput[name] = value
				}
				args, parseErr := observation.ParseArguments(marshal(commandInput))
				if parseErr != nil {
					t.Fatal(parseErr)
				}
				command, err := helm.DecodeCommand(args)
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
			want := op == "get"
			if (schema.Validate(input) == nil) != want || (tool.validate(command) == nil) != want {
				t.Fatal(tool.name, op)
			}
		}
	}
}

func TestAdvertisedToolBoundsAndRequiredArguments(t *testing.T) {
	definitions := ToolDefinitions(Options{
		WorkspaceID: "scope", UI: &testUI{}, Connections: []Connection{{Tool: "kubernetes"}, {Tool: "helm"}},
		Debug: kube.DebugPolicy{Exec: true, Image: "fixture"},
	})
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
		{"kubernetes_read", `{"operation":"logs","resource":"pods","name":"app","container":"app","tail_lines":500}`, true},
		{"kubernetes_read", `{"resource":"pods","name":"app","container":"app"}`, false},
		{"kubernetes_read", `{"operation":"usage","resource":"pods","name":"app"}`, true},
		{"kubernetes_read", `{"operation":"logs","resource":"pods","name":"app","container":"app","tail_lines":501}`, false},
		{"kubernetes_read", `{"operation":"usage","resource":"pods","name":"app","tail_lines":0}`, false},
		{"kubernetes_read", `{"operation":"get","resource":"pods"}`, false},
		{"kubernetes_read", `{"operation":"list","resource":"pods","paths":["/spec"]}`, false},
		{"helm_read", `{"operation":"compare","name":"app","revision":3,"compare_revision":1}`, true},
		{"helm_read", `{"operation":"compare","name":"app","compare_revision":1}`, false},
		{"helm_read", `{"operation":"compare","name":"app","revision":0,"compare_revision":1}`, false},
		{"helm_read", `{"operation":"compare","name":"app","revision":3,"compare_revision":0}`, false},
		{"helm_read", `{"operation":"get","view":"values","name":"app","compare_revision":1}`, false},
		{"kubernetes_read", `{"operation":"usage","resource":"nodes","name":"worker"}`, true},
		{"kubernetes_read", `{"operation":"usage","resource":"services","name":"app"}`, false},
		{"kubernetes_run", `{"operation":"diagnostic","namespace":"shop","test":"tcp","target":"app","port":443}`, true},
		{"kubernetes_run", `{"operation":"diagnostic","namespace":"shop","test":"tcp","target":"app"}`, false},
		{"kubernetes_run", `{"operation":"diagnostic","namespace":"shop","test":"dns","target":"app","port":443}`, false},
		{"kubernetes_run", `{"operation":"exec","namespace":"shop","name":"app","uid":"uid","container":"app","command":["true"],"test":""}`, false},
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
