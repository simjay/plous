package monitor

import (
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

const RunHelp = "Run enabled Kubernetes diagnostics in the captured default namespace. exec requires a Pod UID and a command argument array. diagnostic runs one HTTP GET, DNS lookup or TCP check in a temporary Job. Both are bounded and can change state. A timeout can leave the remote outcome unknown."

type DebugPolicy struct {
	Exec  bool
	Image string
}

type debugOperation struct {
	name     string
	required []string
	optional []string
	granted  func(DebugPolicy) bool
}

var debugOperations = []debugOperation{
	{"exec", []string{"namespace", "name", "uid", "container", "command"}, []string{"timeout_seconds"}, func(p DebugPolicy) bool { return p.Exec }},
	{"diagnostic", []string{"namespace", "test", "target"}, []string{"port", "timeout_seconds"}, func(p DebugPolicy) bool { return p.Image != "" }},
}

func findDebugOperation(operation string) (debugOperation, bool) {
	for _, definition := range debugOperations {
		if definition.name == operation {
			return definition, true
		}
	}
	return debugOperation{}, false
}

func (p DebugPolicy) Allows(operation string) bool {
	definition, ok := findDebugOperation(operation)
	return ok && definition.granted(p)
}

func (p DebugPolicy) Operations() []string {
	var operations []string
	for _, d := range debugOperations {
		if d.granted(p) {
			operations = append(operations, d.name)
		}
	}
	return operations
}

type DebugCommand struct {
	Namespace      string   `json:"namespace"`
	Name           string   `json:"name,omitempty"`
	UID            string   `json:"uid,omitempty"`
	Container      string   `json:"container,omitempty"`
	Command        []string `json:"command,omitempty"`
	Test           string   `json:"test,omitempty"`
	Target         string   `json:"target,omitempty"`
	Port           int      `json:"port,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
}

func DebugSchema(operation string) map[string]any {
	properties := map[string]any{
		"namespace":       map[string]any{"type": "string", "minLength": 1},
		"timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 120, "default": 30},
		"name":            map[string]any{"type": "string", "minLength": 1},
		"uid":             map[string]any{"type": "string", "minLength": 1, "maxLength": 128},
		"container":       map[string]any{"type": "string", "minLength": 1, "maxLength": 63},
		"command":         map[string]any{"type": "array", "minItems": 1, "maxItems": 64, "items": map[string]any{"type": "string", "maxLength": 4096}},
		"test":            map[string]any{"type": "string", "enum": []string{"http", "dns", "tcp"}},
		"target":          map[string]any{"type": "string", "minLength": 1, "maxLength": 2048},
		"port":            map[string]any{"type": "integer", "minimum": 1, "maximum": 65535},
	}
	definition, ok := findDebugOperation(operation)
	if !ok {
		panic("unknown Kubernetes diagnostic operation: " + operation)
	}
	for field := range properties {
		if !slices.Contains(definition.required, field) && !slices.Contains(definition.optional, field) {
			delete(properties, field)
		}
	}
	schema := map[string]any{"type": "object", "properties": properties, "required": slices.Clone(definition.required), "additionalProperties": false}
	if operation == "diagnostic" {
		schema["if"] = map[string]any{"properties": map[string]any{"test": map[string]any{"const": "tcp"}}}
		schema["then"] = map[string]any{"required": []string{"port"}}
		schema["else"] = map[string]any{"properties": map[string]any{"port": false}}
	}
	return schema
}

func DecodeDebug(operation string, supplied map[string]json.RawMessage, policy DebugPolicy, namespace string) (DebugCommand, error) {
	var c DebugCommand
	definition, ok := findDebugOperation(operation)
	if !ok || !definition.granted(policy) {
		return c, errors.New("diagnostic operation is unavailable under the host grants")
	}
	for field := range supplied {
		if !slices.Contains(definition.required, field) && !slices.Contains(definition.optional, field) {
			return c, errors.New("unsupported argument for Kubernetes diagnostic operation")
		}
	}
	for _, field := range definition.required {
		if supplied[field] == nil {
			return c, errors.New("missing required argument: " + field)
		}
	}
	input, err := json.Marshal(supplied)
	if err != nil || json.Unmarshal(input, &c) != nil {
		return c, errors.New("invalid debugging arguments")
	}
	if supplied["timeout_seconds"] != nil && c.TimeoutSeconds == 0 {
		return c, errors.New("timeout_seconds must be 1-120")
	}
	if supplied["port"] != nil && c.Test != "tcp" {
		return c, errors.New("only TCP diagnostics accept a port")
	}
	err = c.validate(operation, policy, namespace)
	return c, err
}

func (c *DebugCommand) validate(operation string, policy DebugPolicy, namespace string) error {
	if !policy.Allows(operation) {
		return errors.New("diagnostic operation is unavailable under the host grants")
	}
	if len(validation.IsDNS1123Label(c.Namespace)) != 0 || c.Namespace != namespace {
		return errors.New("active debugging requires the captured connection's default namespace")
	}
	if c.TimeoutSeconds == 0 {
		c.TimeoutSeconds = 30
	}
	if c.TimeoutSeconds < 1 || c.TimeoutSeconds > 120 {
		return errors.New("timeout_seconds must be 1-120")
	}
	if operation == "diagnostic" {
		if c.Name != "" || c.UID != "" || c.Container != "" || len(c.Command) != 0 {
			return errors.New("temporary diagnostics do not accept Pod or command overrides")
		}
		_, err := c.ProbeCommand()
		return err
	}
	if c.Test != "" || c.Target != "" || c.Port != 0 {
		return errors.New("exec does not accept diagnostic test arguments")
	}
	if len(validation.IsDNS1123Subdomain(c.Name)) != 0 || c.UID == "" || len(c.UID) > 128 || len(validation.IsDNS1123Label(c.Container)) != 0 || len(c.Command) == 0 || len(c.Command) > 64 || c.Command[0] == "" {
		return errors.New("exec requires a Pod name, exact UID, container, and command argument array")
	}
	for _, arg := range c.Command {
		if len(arg) > 4096 || strings.ContainsRune(arg, 0) {
			return errors.New("command argument exceeds its limit or contains NUL")
		}
	}
	return nil
}

func (c DebugCommand) ProbeCommand() ([]string, error) {
	if len(c.Target) == 0 || len(c.Target) > 2048 || strings.ContainsAny(c.Target, "\x00\r\n") {
		return nil, errors.New("a bounded diagnostic target is required")
	}
	switch c.Test {
	case "http":
		u, err := url.Parse(c.Target)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || c.Port != 0 {
			return nil, errors.New("HTTP requires an http(s) URL without credentials; port belongs in the URL")
		}
		return []string{"curl", "--silent", "--show-error", "--include", "--fail-with-body", "--connect-timeout", "5", "--max-time", strconv.Itoa(c.TimeoutSeconds), "--", c.Target}, nil
	case "dns", "tcp":
		if len(validation.IsDNS1123Subdomain(c.Target)) != 0 && net.ParseIP(c.Target) == nil {
			return nil, errors.New("DNS and TCP targets must be a hostname or IP address")
		}
		if c.Test == "dns" {
			if c.Port != 0 {
				return nil, errors.New("DNS does not accept port")
			}
			return []string{"nslookup", c.Target}, nil
		}
		if c.Port < 1 || c.Port > 65535 {
			return nil, errors.New("TCP requires port 1-65535")
		}
		return []string{"nc", "-z", "-v", "-w", "5", c.Target, strconv.Itoa(c.Port)}, nil
	}
	return nil, errors.New("test must be http, dns, or tcp")
}
