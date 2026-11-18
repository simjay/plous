package monitor

import (
	"encoding/json"
	"errors"
	"slices"

	"github.com/simjay/plous/internal/integrations/observation"
)

type Command struct {
	Operation       string `json:"operation"`
	View            string `json:"view,omitempty"`
	Name            string `json:"name,omitempty"`
	Namespace       string `json:"namespace,omitempty"`
	Revision        int    `json:"revision,omitempty"`
	CompareRevision *int   `json:"compare_revision,omitempty"`
}
type ReadDefinition struct {
	Operation string
	Arguments []string
	Required  []string
}

var readCommands = []ReadDefinition{
	{"list", []string{"namespace"}, []string{}},
	{"get", []string{"name", "namespace", "revision", "view"}, []string{"name", "view"}},
	{"history", []string{"name", "namespace"}, []string{"name"}},
	{"compare", []string{"name", "namespace", "revision", "compare_revision"}, []string{"name"}},
}

func ReadCommands() []ReadDefinition {
	definitions := slices.Clone(readCommands)
	for i := range definitions {
		definitions[i].Arguments = slices.Clone(definitions[i].Arguments)
		definitions[i].Required = slices.Clone(definitions[i].Required)
	}
	return definitions
}

func readCommand(operation string, supplied map[string]json.RawMessage) (ReadDefinition, error) {
	i := slices.IndexFunc(readCommands, func(d ReadDefinition) bool { return d.Operation == operation })
	if i < 0 {
		if operation == "notes" {
			return ReadDefinition{}, errors.New("release notes can contain credentials; inspect them in Helm Explore")
		}
		return ReadDefinition{}, errors.New("unsupported Helm operation")
	}
	definition := readCommands[i]
	for key := range supplied {
		if key != "operation" && !slices.Contains(definition.Arguments, key) {
			return ReadDefinition{}, errors.New("unsupported arguments for Helm operation")
		}
	}
	return definition, nil
}

func DecodeCommand(supplied map[string]json.RawMessage) (Command, error) {
	var c Command
	input, err := json.Marshal(supplied)
	if err != nil || len(input) > 4096 {
		return c, errors.New("invalid or oversized Helm command")
	}
	if err := json.Unmarshal(input, &c); err != nil {
		return c, err
	}
	// Check the original keys, including explicit zero values and exact casing.
	if _, err := readCommand(c.Operation, supplied); err != nil {
		return Command{}, err
	}
	return c, nil
}

func ValidateRead(command Command, config Config) error {
	encoded, _ := json.Marshal(command)
	var supplied map[string]json.RawMessage
	_ = json.Unmarshal(encoded, &supplied)
	definition, err := readCommand(command.Operation, supplied)
	if err != nil {
		return err
	}
	if slices.Contains(definition.Required, "name") {
		if err := validRelease(command.Name); err != nil {
			return err
		}
	}
	if config.ConnectionID == "" || config.Context == "" {
		return errors.New("a captured Helm connection is required")
	}
	if err = validNamespace(config.Namespace); err != nil && config.Namespace != "*" {
		return err
	}
	if config.Namespace == "*" && command.Operation != "list" {
		return errors.New("all namespaces is available only for release lists; specify a namespace for this read")
	}
	if command.Namespace != "" && command.Namespace != config.Namespace {
		return errors.New("namespace is outside the captured Helm scope")
	}
	if slices.Contains(definition.Required, "view") && !slices.Contains(releaseViews, command.View) {
		return errors.New("get requires a supported Helm release view")
	}
	if command.CompareRevision != nil && (*command.CompareRevision < 1 || command.Revision < 1 || *command.CompareRevision == command.Revision) {
		return errors.New("comparison requires two distinct positive retained revisions")
	}
	if command.Revision < 0 {
		return errors.New("invalid retained revision")
	}
	return nil
}

var releaseViews = []string{"status", "metadata", "chart", "values", "manifest", "hooks"}

const ReadHelp = "Read stored Helm release data. list lists releases; history lists retained revisions. get takes a release and a view: status, metadata, chart, values, manifest or hooks. revision defaults to the latest. compare uses revision as the after revision and compare_revision as the before revision, defaulting to the predecessor. Values and manifests are masked."

func ReadSchema() map[string]any {
	definitions := ReadCommands()
	properties := map[string]any{}
	var variants []observation.OperationVariant
	for _, definition := range definitions {
		variants = append(variants, observation.OperationVariant{
			Operation: definition.Operation, Arguments: definition.Arguments,
			Schema: map[string]any{"required": definition.Required},
		})
		for _, name := range definition.Arguments {
			typ := "string"
			switch name {
			case "revision", "compare_revision":
				typ = "integer"
			}
			field := map[string]any{"type": typ}
			if name == "revision" {
				field["minimum"], field["default"] = 0, 0
			}
			if name == "compare_revision" {
				field["minimum"] = 1
			}
			if name == "view" {
				field["enum"] = slices.Clone(releaseViews)
			}
			if name == "name" {
				field["minLength"], field["maxLength"] = 1, 53
			}
			properties[name] = field
		}
	}
	schema := observation.OperationSchema(properties, variants)
	schema["if"] = map[string]any{"required": []string{"compare_revision"}}
	schema["then"] = map[string]any{"required": []string{"revision"}, "properties": map[string]any{"revision": map[string]any{"minimum": 1}}}
	return schema
}
