package monitor

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/simjay/plous/internal/integrations/observation"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	MaxAgentBytes      = 64 * 1024
	MaxAgentLines      = 500
	MaxAgentLogSources = 16
	DefaultAgentLines  = 200
	MaxSinceSeconds    = 7 * 24 * 60 * 60
	MaxSelectorLength  = 2048
	MaxSelectedPaths   = 16
	MaxPointerLength   = 256
)

// Command is a structured operation, never shell input. Resource uses its full
// API identity, for example pods or deployments.apps.
type Command struct {
	Operation     string   `json:"operation"`
	APIVersion    string   `json:"api_version,omitempty"`
	Query         string   `json:"query,omitempty"`
	Paths         []string `json:"paths,omitempty"`
	Resource      string   `json:"resource,omitempty"`
	Name          string   `json:"name,omitempty"`
	Namespace     string   `json:"namespace,omitempty"`
	Container     string   `json:"container,omitempty"`
	LabelSelector string   `json:"label_selector,omitempty"`
	FieldSelector string   `json:"field_selector,omitempty"`
	MaxBytes      int      `json:"max_bytes,omitempty"`
	TailLines     int      `json:"tail_lines,omitempty"`
	SinceSeconds  int64    `json:"since_seconds,omitempty"`
	Previous      bool     `json:"previous,omitempty"`
}

// CommandScope binds one validated read to a captured connection and namespace.
type CommandScope struct {
	ConnectionID   string `json:"connection_id"`
	ConnectionName string `json:"connection_name,omitempty"`
	Context        string `json:"context"`
	Namespace      string `json:"namespace"`
}

type CommandDefinition struct {
	Operation string
	Arguments []string
	Required  []string
}

var readCommands = []CommandDefinition{
	{"list", []string{"resource", "api_version", "namespace", "label_selector", "field_selector"}, []string{"resource"}},
	{"get", []string{"resource", "api_version", "name", "namespace", "paths"}, []string{"resource", "name"}},
	{"usage", []string{"resource", "name", "namespace"}, []string{"resource", "name"}},
	{"logs", []string{"resource", "name", "namespace", "container", "tail_lines", "since_seconds", "previous", "max_bytes"}, []string{"resource", "name", "container"}},
	{"discover", []string{"query"}, nil},
}

func ReadCommands() []CommandDefinition {
	definitions := slices.Clone(readCommands)
	for i := range definitions {
		definitions[i].Arguments = slices.Clone(definitions[i].Arguments)
		definitions[i].Required = slices.Clone(definitions[i].Required)
	}
	return definitions
}

func readCommand(operation string, supplied map[string]json.RawMessage) (CommandDefinition, error) {
	i := slices.IndexFunc(readCommands, func(d CommandDefinition) bool { return d.Operation == operation })
	if i < 0 {
		return CommandDefinition{}, errors.New("unsupported Kubernetes operation")
	}
	definition := readCommands[i]
	for key := range supplied {
		if key != "operation" && !slices.Contains(definition.Arguments, key) {
			return CommandDefinition{}, errors.New("unsupported arguments for Kubernetes operation")
		}
	}
	return definition, nil
}

// DecodeCommand validates the admitted fields of a parsed request and decodes its values.
func DecodeCommand(supplied map[string]json.RawMessage) (Command, error) {
	var command Command
	input, err := json.Marshal(supplied)
	if err != nil || len(input) > 16*1024 {
		return command, errors.New("invalid or oversized Kubernetes command")
	}
	if err := json.Unmarshal(input, &command); err != nil {
		return Command{}, errors.New("unknown or malformed Kubernetes command argument")
	}
	// encoding/json accepts case-insensitive struct field names. The command
	// grammar is exact so recorded arguments always identify the executed call.
	if _, err := readCommand(command.Operation, supplied); err != nil {
		return Command{}, err
	}
	return command, nil
}

func ValidateRead(command Command, scope CommandScope) error {
	// The encoded shape also validates callers that construct commands directly.
	encoded, _ := json.Marshal(command)
	var supplied map[string]json.RawMessage
	_ = json.Unmarshal(encoded, &supplied)
	definition, err := readCommand(command.Operation, supplied)
	if err != nil {
		return err
	}
	for _, field := range definition.Required {
		if _, ok := supplied[field]; !ok {
			return fmt.Errorf("%s requires %s", command.Operation, field)
		}
	}
	if command.Operation == "usage" && !hasUsageResource(command.Resource) {
		return errors.New("usage requires a Pod, Node or supported workload")
	}
	if scope.ConnectionID == "" || scope.Context == "" || scope.Namespace != "*" && len(validation.IsDNS1123Label(scope.Namespace)) != 0 {
		return errors.New("a captured Kubernetes connection, context, and namespace are required")
	}
	if command.Operation == "discover" {
		if len(command.Query) > MaxPointerLength {
			return errors.New("discovery query exceeds its size limit")
		}
		return nil
	}
	if len(command.Paths) > MaxSelectedPaths {
		return errors.New("too many selected field paths")
	}
	seen := map[string]bool{}
	for _, pointer := range command.Paths {
		if len(pointer) > MaxPointerLength || !validPointer(pointer) || seen[pointer] {
			return errors.New("paths must contain distinct, bounded JSON Pointers")
		}
		seen[pointer] = true
	}
	if command.APIVersion != "" && len(validation.IsDNS1123Label(command.APIVersion)) != 0 {
		return errors.New("api_version must be a Kubernetes version such as v1")
	}
	if scope.Namespace == "*" && command.Operation != "list" {
		return errors.New("all namespaces is available only for resource lists; specify a namespace for this read")
	}
	if command.Namespace != "" && command.Namespace != scope.Namespace {
		return errors.New("namespace is outside the captured Kubernetes scope")
	}
	if command.Resource == "" || len(command.Resource) > 253 || len(validation.IsDNS1123Subdomain(command.Resource)) != 0 {
		return errors.New("resource must be an explicit Kubernetes API resource identity")
	}
	resource, _, _ := strings.Cut(command.Resource, ".")
	if resource == "secrets" || resource == "secret" {
		return errors.New("Secret payloads are unavailable to the assistant")
	}
	if command.Name != "" && len(validation.IsDNS1123Subdomain(command.Name)) != 0 {
		return errors.New("invalid Kubernetes resource name")
	}
	if command.MaxBytes < 0 || command.MaxBytes > MaxAgentBytes || command.TailLines < 0 || command.TailLines > MaxAgentLines || command.SinceSeconds < 0 || command.SinceSeconds > MaxSinceSeconds {
		return errors.New("Kubernetes read limit is outside the permitted range: max_bytes 0-65536, tail_lines 0-500, since_seconds 0-604800; 0 uses the default")
	}
	if len(command.LabelSelector) > MaxSelectorLength || len(command.FieldSelector) > MaxSelectorLength {
		return errors.New("Kubernetes read arguments exceed the permitted size")
	}
	if _, err := labels.Parse(command.LabelSelector); err != nil {
		return errors.New("invalid Kubernetes label selector")
	}
	selector, err := fields.ParseSelector(command.FieldSelector)
	if err != nil {
		return errors.New("invalid Kubernetes field selector")
	}
	for _, requirement := range selector.Requirements() {
		switch requirement.Field {
		case "metadata.name", "metadata.namespace", "involvedObject.name", "involvedObject.uid", "involvedObject.kind", "reason", "type", "status.phase":
		default:
			return errors.New("unsupported Kubernetes field selector")
		}
		if requirement.Field == "metadata.namespace" && scope.Namespace != "*" && requirement.Value != scope.Namespace {
			return errors.New("field selector is outside the captured Kubernetes namespace")
		}
	}
	if command.Operation == "logs" {
		if !slices.Contains(podTargetResources, command.Resource) || len(validation.IsDNS1123Label(command.Container)) != 0 {
			return errors.New("logs require a supported Pod or workload and an explicit container")
		}
	}
	return nil
}

const ReadHelp = "Read Kubernetes objects, logs and current usage on the captured cluster. discover finds API resource types; use history_read for later pages. list and get accept an optional api_version such as v1. Named get accepts exact JSON Pointer paths to bounded, masked scalar fields. list takes a resource type and optional selectors; get requires a name. Use the exact API resource name, such as deployments.apps. Namespace * is for lists. Continue saved rows through history_read using page.next_offset. logs requires one explicit container and returns bounded excerpts. usage reports identity-checked CPU and memory with coverage. Secrets are denied."

func ReadSchema() map[string]any { return readSchema(ReadCommands()) }

func readSchema(definitions []CommandDefinition) map[string]any {
	properties := map[string]any{}
	var variants []observation.OperationVariant
	for _, definition := range definitions {
		for _, name := range definition.Arguments {
			field := map[string]any{"type": "string"}
			switch name {
			case "resource", "name":
				field["minLength"], field["maxLength"] = 1, 253
			case "api_version":
				field["minLength"], field["maxLength"] = 1, 63
			case "query":
				field["maxLength"] = MaxPointerLength
			case "paths":
				field = map[string]any{"type": "array", "maxItems": MaxSelectedPaths, "uniqueItems": true,
					"items": map[string]any{"type": "string", "pattern": "^/", "maxLength": MaxPointerLength}}
			case "container":
				field["minLength"], field["maxLength"] = 1, 63
			case "label_selector", "field_selector":
				field["maxLength"] = MaxSelectorLength
			case "max_bytes":
				field = map[string]any{"type": "integer", "minimum": 0, "maximum": MaxAgentBytes, "default": MaxAgentBytes, "description": "0 uses the default"}
			case "tail_lines":
				field = map[string]any{"type": "integer", "minimum": 0, "maximum": MaxAgentLines, "default": DefaultAgentLines, "description": "0 uses the default"}
			case "since_seconds":
				field = map[string]any{"type": "integer", "minimum": 0, "maximum": MaxSinceSeconds}
			case "previous":
				field = map[string]any{"type": "boolean", "default": false}
			}
			properties[name] = field
		}
		branch := map[string]any{}
		if len(definition.Required) > 0 {
			branch["required"] = definition.Required
		}
		switch definition.Operation {
		case "logs":
			branch["properties"] = map[string]any{"resource": map[string]any{"enum": slices.Clone(podTargetResources)}}
		case "usage":
			branch["properties"] = map[string]any{"resource": map[string]any{"enum": append(slices.Clone(podTargetResources), "nodes")}}
		}
		variants = append(variants, observation.OperationVariant{Operation: definition.Operation, Arguments: definition.Arguments, Schema: branch})
	}
	return observation.OperationSchema(properties, variants)
}

// Controller reads retain the Kubernetes grammar with a narrower operation set.
func ControllerReadSchema(resources, operations []string) map[string]any {
	var definitions []CommandDefinition
	for _, definition := range ReadCommands() {
		if slices.Contains(operations, definition.Operation) {
			definitions = append(definitions, definition)
		}
	}
	schema := readSchema(definitions)
	schema["properties"].(map[string]any)["resource"] = map[string]any{"type": "string", "enum": resources}
	return schema
}
