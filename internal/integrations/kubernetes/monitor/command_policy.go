package monitor

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/validation"
)

type CommandAccess string

const (
	AccessRead  CommandAccess = "read"
	AccessWrite CommandAccess = "write"

	MaxAgentBytes      = 64 * 1024
	MaxAgentLines      = 500
	MaxAgentLogSources = 16
	DefaultAgentLines  = 200
	MaxSinceSeconds    = 7 * 24 * 60 * 60
	MaxSelectorLength  = 2048
)

// Command is a structured operation, never shell input. Resource uses its full
// API identity, for example pods or deployments.apps.
type Command struct {
	Operation     string `json:"operation"`
	Resource      string `json:"resource,omitempty"`
	Name          string `json:"name,omitempty"`
	Namespace     string `json:"namespace,omitempty"`
	Container     string `json:"container,omitempty"`
	LabelSelector string `json:"label_selector,omitempty"`
	FieldSelector string `json:"field_selector,omitempty"`
	MaxBytes      int    `json:"max_bytes,omitempty"`
	TailLines     int    `json:"tail_lines,omitempty"`
	SinceSeconds  int64  `json:"since_seconds,omitempty"`
	Previous      bool   `json:"previous,omitempty"`
}

// CommandScope binds one validated read to a captured connection and namespace.
type CommandScope struct {
	ConnectionID   string `json:"connection_id"`
	ConnectionName string `json:"connection_name,omitempty"`
	Context        string `json:"context"`
	Namespace      string `json:"namespace"`
}

type CommandDefinition struct {
	Tool      string        `json:"tool,omitempty"`
	Operation string        `json:"operation"`
	Access    CommandAccess `json:"access"`
	Arguments []string      `json:"arguments"`
	Required  []string      `json:"required"`
}

var commandDefinitions = []CommandDefinition{
	{"kubernetes_resources_read", "get", AccessRead, []string{"resource", "name", "namespace", "label_selector", "field_selector"}, []string{"resource"}},
	{"kubernetes_resources_read", "describe", AccessRead, []string{"resource", "name", "namespace"}, []string{"resource", "name"}},
	{"kubernetes_usage_read", "usage", AccessRead, []string{"resource", "name", "namespace"}, []string{"resource", "name"}},
	{"kubernetes_logs_read", "logs", AccessRead, []string{"resource", "name", "namespace", "container", "tail_lines", "since_seconds", "previous", "max_bytes"}, []string{"resource", "name", "container"}},
	{"", "apply", AccessWrite, nil, nil},
	{"", "create", AccessWrite, nil, nil},
	{"", "patch", AccessWrite, nil, nil},
	{"", "edit", AccessWrite, nil, nil},
	{"", "delete", AccessWrite, nil, nil},
	{"", "replace", AccessWrite, nil, nil},
	{"", "scale", AccessWrite, nil, nil},
	{"", "annotate", AccessWrite, nil, nil},
	{"", "label", AccessWrite, nil, nil},
	{"", "rollout restart", AccessWrite, nil, nil},
	{"", "rollout undo", AccessWrite, nil, nil},
	{"", "set", AccessWrite, nil, nil},
	{"", "drain", AccessWrite, nil, nil},
	{"", "cordon", AccessWrite, nil, nil},
	{"", "uncordon", AccessWrite, nil, nil},
	{"", "taint", AccessWrite, nil, nil},
}

func ReadCommands() []CommandDefinition {
	var result []CommandDefinition
	for _, definition := range commandDefinitions {
		if definition.Access == AccessRead {
			definition.Arguments = append([]string(nil), definition.Arguments...)
			definition.Required = append([]string(nil), definition.Required...)
			result = append(result, definition)
		}
	}
	return result
}

func Classify(command Command) (CommandAccess, error) {
	for _, definition := range commandDefinitions {
		if command.Operation == definition.Operation {
			if definition.Access == AccessRead {
				encoded, _ := json.Marshal(command)
				var supplied map[string]json.RawMessage
				_ = json.Unmarshal(encoded, &supplied)
				delete(supplied, "operation")
				for _, argument := range definition.Arguments {
					delete(supplied, argument)
				}
				if len(supplied) != 0 {
					return "", errors.New("unsupported arguments for Kubernetes operation")
				}
			}
			return definition.Access, nil
		}
	}
	return "", errors.New("unsupported Kubernetes operation")
}

// DecodeCommand rejects unknown and duplicate arguments, including fields that
// a standard JSON decoder would silently ignore or replace.
func DecodeCommand(input string) (Command, error) {
	var command Command
	if len(input) > 16*1024 {
		return command, errors.New("Kubernetes command exceeds input limit")
	}
	decoder := json.NewDecoder(strings.NewReader(input))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return command, errors.New("Kubernetes command must be a JSON object")
	}
	seen := make(map[string]bool)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return command, errors.New("invalid Kubernetes command")
		}
		key, ok := token.(string)
		if !ok || seen[key] {
			return command, errors.New("duplicate or invalid Kubernetes command argument")
		}
		seen[key] = true
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil || string(value) == "null" {
			return command, errors.New("invalid Kubernetes command argument")
		}
	}
	if _, err := decoder.Token(); err != nil {
		return command, errors.New("invalid Kubernetes command")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return command, errors.New("Kubernetes command must contain one object")
	}
	decoder = json.NewDecoder(strings.NewReader(input))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&command); err != nil {
		return Command{}, errors.New("unknown or malformed Kubernetes command argument")
	}
	// encoding/json accepts case-insensitive struct field names. The command
	// grammar is exact so recorded arguments always identify the executed call.
	allowed := map[string]bool{"operation": true}
	for _, definition := range commandDefinitions {
		for _, argument := range definition.Arguments {
			allowed[argument] = true
		}
	}
	for key := range seen {
		if !allowed[key] {
			return Command{}, errors.New("unknown Kubernetes command argument")
		}
	}
	for _, definition := range commandDefinitions {
		if definition.Operation != command.Operation || definition.Access != AccessRead {
			continue
		}
		permitted := map[string]bool{"operation": true}
		for _, argument := range definition.Arguments {
			permitted[argument] = true
		}
		for argument := range seen {
			if !permitted[argument] {
				return Command{}, errors.New("unsupported arguments for Kubernetes operation")
			}
		}
	}
	return command, nil
}

func ValidateRead(command Command, scope CommandScope) error {
	access, err := Classify(command)
	if err != nil {
		return err
	}
	if access != AccessRead {
		return errors.New("the assistant permits only read operations")
	}
	for _, definition := range ReadCommands() {
		if definition.Operation != command.Operation {
			continue
		}
		values := map[string]string{"resource": command.Resource, "name": command.Name, "container": command.Container}
		for _, field := range definition.Required {
			if values[field] == "" {
				return fmt.Errorf("%s requires %s", command.Operation, field)
			}
		}
	}
	if command.Operation == "usage" && !hasUsageResource(command.Resource) {
		return errors.New("usage requires a Pod, Node or supported workload")
	}
	if scope.ConnectionID == "" || scope.Context == "" || scope.Namespace != "*" && len(validation.IsDNS1123Label(scope.Namespace)) != 0 {
		return errors.New("a captured Kubernetes connection, context, and namespace are required")
	}
	if scope.Namespace == "*" && (command.Operation != "get" || command.Name != "") {
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
	if command.Operation == "get" && command.Name != "" && (command.LabelSelector != "" || command.FieldSelector != "") {
		return errors.New("named reads do not accept list arguments")
	}
	if command.Operation == "logs" {
		if !hasUsageResource(command.Resource) || command.Resource == "nodes" || command.Name == "" || len(validation.IsDNS1123Label(command.Container)) != 0 {
			return errors.New("logs require a supported Pod or workload and an explicit container")
		}
	}
	return nil
}

func ReadOperations(tool string) []string {
	var operations []string
	for _, d := range ReadCommands() {
		if d.Tool == tool {
			operations = append(operations, d.Operation)
		}
	}
	return operations
}
