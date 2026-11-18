package ai

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/ai/history"
	helm "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/integrations/observation"
)

const viewHelp = "Read the current Plous view with get, or open a destination: resource, resource_list, pod_logs, helm_comparison or evidence. Opening preserves focus and edits. Use read tools for evidence."

type viewDestination struct {
	name, owner, view string
	schema            func() map[string]any
	validate          func(map[string]json.RawMessage) (string, error)
}

var viewDestinations = []viewDestination{
	{"resource", "kubernetes", "resource", func() map[string]any { return kubernetes.ShowSchema("resource") }, kubernetes.ValidateShow},
	{"resource_list", "kubernetes", "list", func() map[string]any { return kubernetes.ShowSchema("list") }, kubernetes.ValidateShow},
	{"pod_logs", "kubernetes", "logs", func() map[string]any { return kubernetes.ShowSchema("logs") }, kubernetes.ValidateShow},
	{"helm_comparison", "helm", "", helm.ShowSchema, helm.ValidateShow},
	{"evidence", "", "", history.ShowSchema, history.ValidateShow},
}

func (op *operation) viewSchema() map[string]any {
	variants := []observation.OperationVariant{operationVariant("get", map[string]any{
		"properties": map[string]any{}, "required": []string{},
	})}
	for _, destination := range viewDestinations {
		if destination.owner != "" && !op.hasTool(destination.owner) {
			continue
		}
		schema := destination.schema()
		properties := schema["properties"].(map[string]any)
		required := append(schema["required"].([]string), "destination")
		properties["destination"] = map[string]any{"const": destination.name}
		if destination.owner != "" {
			properties["connection_id"] = op.connectionParameter(destination.owner)
			required = append(required, "connection_id")
		}
		if destination.name == "pod_logs" {
			delete(properties, "resource")
			required = []string{"name", "namespace", "container", "destination", "connection_id"}
		}
		schema["required"] = required
		variants = append(variants, operationVariant("open", schema))
	}
	return observation.OperationSchema(nil, variants)
}

func (op *operation) uiTool(ctx context.Context, args map[string]json.RawMessage) (string, error) {
	feedback := func(reason string) (string, error) {
		return string(marshal(agent.UIResult{Status: "blocked", Reason: reason})), nil
	}
	var result agent.UIResult
	var err error
	original := marshal(args)
	operation := stringArgument(args, "operation")
	delete(args, "operation")
	switch operation {
	case "get":
		if len(args) != 0 {
			return feedback("view get accepts no destination or view arguments")
		}
		result, err = op.o.UI.Read(ctx)
	case "open":
		name := stringArgument(args, "destination")
		delete(args, "destination")
		var destination viewDestination
		for _, candidate := range viewDestinations {
			if candidate.name == name {
				destination = candidate
				break
			}
		}
		if destination.name == "" || destination.owner != "" && !op.hasTool(destination.owner) {
			return feedback("unsupported destination in captured scope")
		}
		if _, exists := args["view"]; exists {
			return feedback("select a destination instead of a view")
		}
		if destination.view != "" {
			args["view"] = marshal(destination.view)
		}
		if name == "pod_logs" {
			if _, exists := args["resource"]; exists {
				return feedback("pod_logs does not accept resource")
			}
			args["resource"] = marshal("pods")
		}
		var q agent.ViewRequest
		if err = decode(string(marshal(args)), &q); err != nil {
			return feedback(err.Error())
		}
		input := make(map[string]json.RawMessage, len(args))
		for key, value := range args {
			var text string
			if json.Unmarshal(value, &text) == nil && len(text) > 1024 {
				return feedback("view argument exceeds 1024 bytes")
			}
			input[key] = value
		}
		if destination.owner != "" {
			if q.ConnectionID == "" {
				return feedback("connection_id is required")
			}
			delete(input, "connection_id")
		}
		q.View, err = destination.validate(input)
		if err != nil {
			return feedback(err.Error())
		}

		if q.View == "evidence" {
			out, readErr := op.historyAccess.Call(ctx, history.HistoryRequest{Operation: "read", Ref: q.Ref, Offset: q.Offset}, 65536)
			if readErr != nil {
				var invalid *history.QueryError
				if errors.As(readErr, &invalid) {
					return feedback(readErr.Error())
				}
				return "", readErr
			}
			q.Evidence, err = op.redactJSON(marshal(out))
			if err != nil {
				return "", err
			}
		} else {
			if _, err = op.connection(q.ConnectionID, destination.owner); err != nil {
				return feedback(err.Error())
			}
		}
		if len(marshal(q))+4096 > op.toolBytes() {
			return feedback("UI result needs a larger delivery allowance")
		}
		result, err = op.o.UI.Show(ctx, q)
	default:
		return feedback("view requires get or open")
	}
	if err != nil {
		return "", err
	}
	body, err := op.redactJSON(marshal(result))
	if err != nil {
		return "", err
	}
	if len(body) > op.toolBytes() {
		return feedback("UI result exceeds the delivery allowance")
	}
	return string(body), op.o.History.SaveUIResult(op.o.WorkspaceID, op.o.ConversationID, op.o.TurnID, "plous_view", original, body)
}
