package ai

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"

	"github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/ai/history"
	argo "github.com/simjay/plous/internal/integrations/argo/monitor"
	flux "github.com/simjay/plous/internal/integrations/flux/monitor"
	helm "github.com/simjay/plous/internal/integrations/helm/monitor"
	kube "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/integrations/observation"
	"github.com/simjay/plous/internal/workspace"
)

type toolDefinition struct {
	name, connection, description string
	schema                        func() map[string]any
	read                          func(context.Context, Connection, map[string]json.RawMessage) (observation.Result, error)
	kind                          toolKind
}

type toolKind int

const (
	readTool toolKind = iota
	historyTool
	runTool
	viewTool
)

func (op *operation) tools() []toolDefinition {
	result := []toolDefinition{{name: "history_read", description: history.ReadHelp, schema: history.ReadSchema, kind: historyTool}}
	if op.hasTool("kubernetes") {
		result = append(result,
			toolDefinition{"kubernetes_read", "kubernetes", kube.ReadHelp, kube.ReadSchema, op.readKubernetes, readTool},
			toolDefinition{"argo_read", "kubernetes", argo.ReadHelp, argo.ReadSchema, op.kubernetesReader(argo.ValidateRead), readTool},
			toolDefinition{"flux_read", "kubernetes", flux.ReadHelp, flux.ReadSchema, op.kubernetesReader(flux.ValidateRead), readTool},
		)
		if op.o.Monitor != nil {
			result = append(result, toolDefinition{"monitor_read", "kubernetes", op.o.Monitor.Description(), op.o.Monitor.Schema, op.readMonitor, readTool})
		}
		if len(op.o.Debug.Operations()) > 0 {
			result = append(result, toolDefinition{
				name: "kubernetes_run", connection: "kubernetes", kind: runTool, read: op.debugTool,
				description: kube.RunHelp, schema: op.runSchema,
			})
		}
	}
	if op.hasTool("helm") {
		result = append(result, toolDefinition{"helm_read", "helm", helm.ReadHelp, helm.ReadSchema, op.readHelm, readTool})
	}
	if op.o.WorkspaceID != "" {
		result = append(result, toolDefinition{name: "workspace_read", description: workspace.ReadHelp, schema: workspace.ReadSchema, read: op.readWorkspace})
	}
	if op.o.UI != nil {
		result = append(result, toolDefinition{name: "plous_view", description: viewHelp, schema: op.viewSchema, kind: viewTool})
	}
	return result
}

func (op *operation) tool(name string) (toolDefinition, bool) {
	for _, tool := range op.tools() {
		if tool.name == name {
			return tool, true
		}
	}
	return toolDefinition{}, false
}

func (op *operation) toolDefinitions() []agent.Tool {
	var result []agent.Tool
	for _, tool := range op.tools() {
		parameters := tool.schema()
		properties := parameters["properties"].(map[string]any)
		properties["purpose"] = map[string]any{"type": "string", "minLength": 1}
		required := append(parameters["required"].([]string), "purpose")
		if tool.connection != "" {
			properties["connection_id"] = op.connectionParameter(tool.connection)
			required = append(required, "connection_id")
		}
		if tool.kind == readTool || tool.kind == runTool {
			properties["fields"] = observation.FieldsSchema()
		}
		parameters["required"] = required
		result = append(result, agent.Tool{
			Name: tool.name, Description: tool.description, Parameters: marshal(parameters),
			Mutating: tool.kind == runTool || tool.kind == viewTool, NonDestructive: tool.kind == viewTool,
		})
	}
	return result
}

func (op *operation) connectionParameter(owner string) map[string]any {
	var ids, descriptions []string
	for _, c := range op.o.Connections {
		if c.Tool == owner && c.ConnectionID != "" {
			ids = append(ids, c.ConnectionID)
			descriptions = append(descriptions, c.Name+"; context="+c.Context+"; default_namespace="+c.Namespace)
		}
	}
	if len(ids) == 0 {
		return map[string]any{"type": "string", "minLength": 1, "description": "Exact ID for this tool in the current connection inventory."}
	}
	return map[string]any{"type": "string", "enum": ids, "description": strings.Join(descriptions, " | ")}
}

func (op *operation) readWorkspace(ctx context.Context, _ Connection, args map[string]json.RawMessage) (observation.Result, error) {
	var q struct {
		Operation string `json:"operation"`
		Cursor    string `json:"cursor"`
		Limit     int    `json:"limit"`
	}
	if err := decode(string(marshal(args)), &q); err != nil {
		return observation.Result{}, denyRead(err)
	}
	if !slices.Contains(workspace.ReadOperations(), q.Operation) {
		return observation.Result{}, denyRead(errors.New("workspace_read requires workspace or activity"))
	}
	return op.records.Project(ctx, workspace.Query{Source: q.Operation, Cursor: q.Cursor, Limit: q.Limit})
}

func stringArgument(args map[string]json.RawMessage, key string) string {
	var value string
	_ = json.Unmarshal(args[key], &value)
	return value
}

func operationVariant(operation string, schema map[string]any) observation.OperationVariant {
	properties := schema["properties"].(map[string]any)
	arguments := slices.Collect(maps.Keys(properties))
	properties["operation"] = map[string]any{"const": operation}
	delete(schema, "type")
	delete(schema, "additionalProperties")
	return observation.OperationVariant{Operation: operation, Arguments: arguments, Schema: schema}
}
