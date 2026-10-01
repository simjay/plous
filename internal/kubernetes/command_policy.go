package kubernetes

import (
	"encoding/json"
	"errors"
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

	MaxAgentItems = 100
	MaxAgentBytes = 64 * 1024
	MaxAgentLines = 500
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
	Continue      string `json:"continue,omitempty"`
	Limit         int    `json:"limit,omitempty"`
	MaxBytes      int    `json:"max_bytes,omitempty"`
	TailLines     int    `json:"tail_lines,omitempty"`
	SinceSeconds  int64  `json:"since_seconds,omitempty"`
	Previous      bool   `json:"previous,omitempty"`
}

// CommandScope is captured by the application before model execution. The model
// cannot select a different connection, context, or namespace.
type CommandScope struct {
	ConnectionID string `json:"connection_id"`
	Context      string `json:"context"`
	Namespace    string `json:"namespace"`
}

type CommandDefinition struct {
	Operation string        `json:"operation"`
	Access    CommandAccess `json:"access"`
	Arguments []string      `json:"arguments"`
}

var commandDefinitions = []CommandDefinition{
	{"get", AccessRead, []string{"resource", "name", "namespace", "label_selector", "field_selector", "continue", "limit", "max_bytes"}},
	{"describe", AccessRead, []string{"resource", "name", "namespace", "max_bytes"}},
	{"logs", AccessRead, []string{"resource", "name", "namespace", "container", "tail_lines", "since_seconds", "previous", "max_bytes"}},
	{"apply", AccessWrite, nil},
	{"create", AccessWrite, nil},
	{"patch", AccessWrite, nil},
	{"edit", AccessWrite, nil},
	{"delete", AccessWrite, nil},
	{"replace", AccessWrite, nil},
	{"scale", AccessWrite, nil},
	{"annotate", AccessWrite, nil},
	{"label", AccessWrite, nil},
	{"rollout restart", AccessWrite, nil},
	{"rollout undo", AccessWrite, nil},
	{"set", AccessWrite, nil},
	{"drain", AccessWrite, nil},
	{"cordon", AccessWrite, nil},
	{"uncordon", AccessWrite, nil},
	{"taint", AccessWrite, nil},
}

func ReadCommands() []CommandDefinition {
	var result []CommandDefinition
	for _, definition := range commandDefinitions {
		if definition.Access == AccessRead {
			definition.Arguments = append([]string(nil), definition.Arguments...)
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
	if scope.ConnectionID == "" || scope.Context == "" || len(validation.IsDNS1123Label(scope.Namespace)) != 0 {
		return errors.New("a captured Kubernetes connection, context, and namespace are required")
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
	if command.MaxBytes < 0 || command.MaxBytes > MaxAgentBytes || command.Limit < 0 || command.Limit > MaxAgentItems || command.TailLines < 0 || command.TailLines > MaxAgentLines || command.SinceSeconds < 0 || command.SinceSeconds > 7*24*60*60 {
		return errors.New("Kubernetes read limit is outside the permitted range")
	}
	if len(command.LabelSelector) > 2048 || len(command.FieldSelector) > 2048 || len(command.Continue) > 4096 {
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
		if requirement.Field == "metadata.namespace" && requirement.Value != scope.Namespace {
			return errors.New("field selector is outside the captured Kubernetes namespace")
		}
	}
	if command.Operation == "get" && command.Name != "" && (command.LabelSelector != "" || command.FieldSelector != "" || command.Continue != "" || command.Limit != 0) {
		return errors.New("named reads do not accept list arguments")
	}
	if command.Operation == "describe" && command.Name == "" {
		return errors.New("describe requires a resource name")
	}
	if command.Operation == "logs" {
		if command.Resource != "pods" || command.Name == "" || len(validation.IsDNS1123Label(command.Container)) != 0 {
			return errors.New("logs require an explicit Pod and container")
		}
	}
	return nil
}
