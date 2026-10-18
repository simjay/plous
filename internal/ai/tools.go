package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	aimodel "github.com/simjay/plous/internal/ai/model"

	argo "github.com/simjay/plous/internal/integrations/argo/monitor"
	flux "github.com/simjay/plous/internal/integrations/flux/monitor"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/integrations/observation"
	"github.com/simjay/plous/internal/source"
	"github.com/simjay/plous/internal/workspace"
)

type toolDefinition struct {
	Owner       string
	Name        string
	Description string
	Operations  []string
	Schema      func() map[string]any
}

func externalDefinitions() []toolDefinition {
	definitions := []toolDefinition{
		{"kubernetes", "kubernetes_resources_read", kubernetes.ResourcesHelp, kubernetes.ReadOperations("kubernetes_resources_read"), func() map[string]any { return kubernetes.ReadSchema("kubernetes_resources_read") }},
		{"kubernetes", "kubernetes_logs_read", kubernetes.LogsHelp, kubernetes.ReadOperations("kubernetes_logs_read"), func() map[string]any { return kubernetes.ReadSchema("kubernetes_logs_read") }},
		{"kubernetes", "kubernetes_usage_read", kubernetes.UsageHelp, kubernetes.ReadOperations("kubernetes_usage_read"), func() map[string]any { return kubernetes.ReadSchema("kubernetes_usage_read") }},
		{"helm", "helm_releases_read", helmtool.ReleasesHelp, helmtool.ReadOperations("helm_releases_read"), func() map[string]any { return helmtool.ReadSchema("helm_releases_read") }},
		{"helm", "helm_configuration_read", helmtool.ConfigurationHelp, helmtool.ReadOperations("helm_configuration_read"), func() map[string]any { return helmtool.ReadSchema("helm_configuration_read") }},
		{"helm", "helm_diff_read", helmtool.CompareHelp, helmtool.ReadOperations("helm_diff_read"), func() map[string]any { return helmtool.ReadSchema("helm_diff_read") }},
		{"kubernetes", "argo_resources_read", argo.ReadHelp, argo.ReadOperations(), argo.ReadSchema},
		{"kubernetes", "flux_resources_read", flux.ReadHelp, flux.ReadOperations(), flux.ReadSchema},
		{"source", "source_files_read", source.FilesHelp, source.ReadOperations("source_files_read"), func() map[string]any { return source.ReadSchema("source_files_read") }},
		{"source", "source_analysis_read", source.AnalysisHelp, source.ReadOperations("source_analysis_read"), func() map[string]any { return source.ReadSchema("source_analysis_read") }},
		{"workspace", "workspace_read", workspace.ReadHelp, workspace.ReadOperations(), workspace.ReadSchema},
	}

	return definitions
}

func (d toolDefinition) parameters() map[string]any {
	parameters := d.Schema()
	properties := parameters["properties"].(map[string]any)
	required := parameters["required"].([]string)
	properties["purpose"] = map[string]any{"type": "string", "minLength": 1, "description": "What you are looking for in this result"}
	properties["fields"] = observation.FieldsSchema()
	required = append(required, "purpose")
	if d.Owner != "workspace" {
		properties["connection_id"] = map[string]any{"type": "string", "minLength": 1, "description": "Exact ID in the captured connection inventory"}
		required = append(required, "connection_id")
	}
	parameters["required"] = required
	return parameters
}

func (op *operation) definitions() []toolDefinition {
	var result []toolDefinition
	for _, d := range externalDefinitions() {
		if op.hasTool(d.Owner) || d.Name == "workspace_read" && op.o.WorkspaceID != "" {
			result = append(result, d)
		}
	}
	return result
}

func (op *operation) externalRead(ctx context.Context, name, input string) (any, error) {
	var definition *toolDefinition
	for _, d := range externalDefinitions() {
		if d.Name == name {
			definition = &d
			break
		}
	}
	if definition == nil {
		return nil, denyRead(errors.New("tool is unavailable in the captured workspace"))
	}
	input, err := definition.prepareInput(input)
	if err != nil {
		return nil, denyRead(err)
	}

	switch name {
	case "kubernetes_resources_read", "kubernetes_logs_read", "kubernetes_usage_read":
		return op.readKubernetes(ctx, input)
	case "argo_resources_read", "flux_resources_read":
		_, args, err := connectionCommand(input)
		if err != nil {
			return nil, denyRead(err)
		}
		command, err := kubernetes.DecodeCommand(args)
		if err == nil {
			if name == "argo_resources_read" {
				err = argo.ValidateRead(command)
			} else {
				err = flux.ValidateRead(command)
			}
		}
		if err != nil {
			return nil, denyRead(err)
		}
		return op.readKubernetes(ctx, input)
	case "helm_releases_read", "helm_configuration_read", "helm_diff_read":
		return op.readHelm(ctx, input)
	case "source_files_read", "source_analysis_read":
		return op.readSource(ctx, input)
	case "workspace_read":
		if op.o.WorkspaceID == "" {
			return nil, denyRead(errors.New("General chat has no connection tools"))
		}
		var q struct {
			Operation string `json:"operation"`
			Cursor    string `json:"cursor"`
			Limit     int    `json:"limit"`
		}
		if err := decode(input, &q); err != nil {
			return nil, denyRead(err)
		}
		return op.records.Project(ctx, workspace.Query{Source: q.Operation, Cursor: q.Cursor, Limit: q.Limit})
	}
	return nil, denyRead(errors.New("tool is unavailable in the captured workspace"))
}

func (op *operation) toolDefinitions() []aimodel.Tool {
	var result []aimodel.Tool
	for _, d := range op.definitions() {
		result = append(result, aimodel.Tool{Name: d.Name, Description: d.Description, Parameters: marshal(d.parameters())})
	}
	result = append(result, historyDefinitions()...)
	if op.o.UI != nil {
		for _, definition := range uiDefinitions() {
			if owner := uiToolOwner(definition.Name); owner == "" || op.hasTool(owner) {
				result = append(result, definition)
			}
		}
	}
	return result
}

func (d toolDefinition) prepareInput(input string) (string, error) {
	fields, err := toolArguments([]byte(input))
	if err != nil {
		return "", err
	}
	operations := d.Operations
	if len(operations) == 1 {
		if _, supplied := fields["operation"]; supplied {
			return "", fmt.Errorf("%s does not accept operation", d.Name)
		}
		fields["operation"] = marshal(operations[0])
		input = string(marshal(fields))
	} else {
		var operation string
		if json.Unmarshal(fields["operation"], &operation) != nil || !slices.Contains(operations, operation) {
			return "", fmt.Errorf("%s supports %s", d.Name, strings.Join(operations, " or "))
		}
	}

	return input, nil
}
