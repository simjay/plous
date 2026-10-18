package ai

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/ai/history"
	helm "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/source"
)

type showTool struct {
	Name, Owner, Description string
	Schema                   func() map[string]any
	Validate                 func(map[string]json.RawMessage) (string, error)
}

func showTools() []showTool {
	return []showTool{
		{"kubernetes_show", "kubernetes", kubernetes.ShowHelp, kubernetes.ShowSchema, kubernetes.ValidateShow},
		{"helm_show", "helm", helm.ShowHelp, helm.ShowSchema, helm.ValidateShow},
		{"source_show", "source", source.ShowHelp, source.ShowSchema, source.ValidateShow},
		{"history_show", "", history.ShowHelp, history.ShowSchema, history.ValidateShow},
	}
}

func showDefinition(name string) (showTool, bool) {
	for _, definition := range showTools() {
		if definition.Name == name {
			return definition, true
		}
	}
	return showTool{}, false
}

func uiToolOwner(name string) string {
	definition, _ := showDefinition(name)
	return definition.Owner
}

func uiDefinitions() []agent.Tool {
	purpose := map[string]any{"type": "string", "minLength": 1}
	result := []agent.Tool{{Name: "ui_read", Description: "Read the current Plous view, selection, filters, focus and available destinations within this session's captured scope. Returns no resource bodies or editor contents.", Parameters: marshal(map[string]any{"type": "object", "properties": map[string]any{"purpose": purpose}, "required": []string{"purpose"}, "additionalProperties": false})}}
	for _, tool := range showTools() {
		schema := tool.Schema()
		properties := schema["properties"].(map[string]any)
		properties["purpose"] = purpose
		required := append(schema["required"].([]string), "purpose")
		if tool.Owner != "" {
			properties["connection_id"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 1024}
			required = append(required, "connection_id")
		}
		schema["required"] = required
		description := tool.Description + " Opens the main panel without moving keyboard focus. Only opened confirms presentation; blocked or failed did not open it. Use read tools for facts."
		result = append(result, agent.Tool{Name: tool.Name, Description: description, Parameters: marshal(schema)})
	}
	return result
}

func (op *operation) uiTool(ctx context.Context, name string, args map[string]json.RawMessage) (string, error) {
	feedback := func(reason string) (string, error) {
		return string(marshal(agent.UIResult{Status: "blocked", Reason: reason})), nil
	}
	var result agent.UIResult
	var err error
	if name == "ui_read" {
		if len(args) != 0 {
			return feedback("ui_read accepts only purpose")
		}
		result, err = op.o.UI.Read(ctx)
	} else {
		tool, ok := showDefinition(name)
		if !ok {
			return feedback("unsupported presentation tool")
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
		if tool.Owner != "" {
			if q.ConnectionID == "" {
				return feedback("connection_id is required")
			}
			delete(input, "connection_id")
		}
		q.View, err = tool.Validate(input)
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
			if _, err = op.connection(q.ConnectionID, tool.Owner); err != nil {
				return feedback(err.Error())
			}
		}
		if len(marshal(q))+4096 > op.toolBytes() {
			return feedback("UI result needs a larger delivery allowance")
		}
		result, err = op.o.UI.Show(ctx, q)
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
	return string(body), op.o.History.SaveUIResult(op.o.WorkspaceID, op.o.ConversationID, op.o.TurnID, name, marshal(args), body)
}
