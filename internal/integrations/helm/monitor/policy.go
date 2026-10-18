package monitor

import (
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
)

type Access string

const (
	Read  Access = "read"
	Write Access = "write"
)

type Command struct {
	Operation       string `json:"operation"`
	Name            string `json:"name,omitempty"`
	Namespace       string `json:"namespace,omitempty"`
	Revision        int    `json:"revision,omitempty"`
	CompareRevision *int   `json:"compare_revision,omitempty"`
}
type Definition struct {
	Tool      string
	Operation string
	Access    Access
	Arguments []string
}

type ReadDefinition struct {
	Tool      string
	Operation string
	Arguments []string
	Required  []string
}

// Assistant reads exclude release notes even though Explore can display them.
func ReadCommands() []ReadDefinition {
	var result []ReadDefinition
	for _, definition := range Definitions {
		if definition.Access == Read && definition.Arguments != nil {
			required := []string{}
			if slices.Contains(definition.Arguments, "name") {
				required = append(required, "name")
			}
			result = append(result, ReadDefinition{definition.Tool, definition.Operation, append([]string(nil), definition.Arguments...), required})
		}
	}
	return result
}

var Definitions = []Definition{
	{"helm_releases_read", "list", Read, []string{"namespace"}},
	{"helm_releases_read", "status", Read, []string{"name", "namespace", "revision"}},
	{"helm_releases_read", "metadata", Read, []string{"name", "namespace", "revision"}},
	{"helm_releases_read", "history", Read, []string{"name", "namespace"}},
	{"helm_configuration_read", "values", Read, []string{"name", "namespace", "revision"}},
	{"helm_configuration_read", "manifest", Read, []string{"name", "namespace", "revision"}},
	{"helm_configuration_read", "hooks", Read, []string{"name", "namespace", "revision"}},
	{"helm_releases_read", "chart", Read, []string{"name", "namespace", "revision"}},
	{"helm_diff_read", "compare", Read, []string{"name", "namespace", "revision", "compare_revision"}},
	{"", "notes", Read, nil},
	{"", "install", Write, nil},
	{"", "upgrade", Write, nil},
	{"", "rollback", Write, nil},
	{"", "uninstall", Write, nil},
	{"", "test", Write, nil},
	{"", "repo add", Write, nil},
	{"", "repo update", Write, nil},
	{"", "registry login", Write, nil},
}

func Classify(command Command) (Access, error) {
	for _, d := range Definitions {
		if command.Operation == d.Operation {
			return d.Access, nil
		}
	}
	return "", errors.New("unsupported Helm operation")
}
func DecodeCommand(input string) (Command, error) {
	var c Command
	if len(input) > 4096 {
		return c, errors.New("Helm command exceeds limit")
	}
	d := json.NewDecoder(strings.NewReader(input))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return c, errors.New("Helm command must be an object")
	}
	seen := map[string]bool{}
	for d.More() {
		t, e := d.Token()
		if e != nil {
			return c, e
		}
		key, ok := t.(string)
		if !ok || seen[key] || key != "operation" && key != "name" && key != "namespace" && key != "revision" && key != "compare_revision" {
			return c, errors.New("unknown or duplicate Helm argument")
		}
		seen[key] = true
		var raw json.RawMessage
		if e = d.Decode(&raw); e != nil || string(raw) == "null" {
			return c, errors.New("invalid Helm argument")
		}
	}
	if _, err = d.Token(); err != nil {
		return c, err
	}
	if _, err = d.Token(); err != io.EOF {
		return c, errors.New("expected one Helm command")
	}
	err = json.Unmarshal([]byte(input), &c)
	if err != nil {
		return c, err
	}
	for _, definition := range ReadCommands() {
		if definition.Operation != c.Operation {
			continue
		}
		for name := range seen {
			if name != "operation" && !slices.Contains(definition.Arguments, name) {
				return c, errors.New("unsupported arguments for Helm operation")
			}
		}
	}
	return c, nil
}
func ValidateRead(command Command, config Config) error {
	access, err := Classify(command)
	if err != nil {
		return err
	}
	if access != Read {
		return errors.New("the assistant permits only Helm reads")
	}
	if command.Operation == "notes" {
		return errors.New("release notes can contain credentials; inspect them in Helm Explore")
	}
	permitted := false
	for _, definition := range ReadCommands() {
		if definition.Operation == command.Operation {
			permitted = true
			if slices.Contains(definition.Required, "name") {
				if err := validRelease(command.Name); err != nil {
					return err
				}
			}
			if command.Name != "" && !slices.Contains(definition.Arguments, "name") || command.Revision != 0 && !slices.Contains(definition.Arguments, "revision") {
				return errors.New("unsupported arguments for Helm operation")
			}
		}
	}
	if !permitted {
		return errors.New("Helm operation has no permitted assistant read handler")
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
	if command.CompareRevision != nil && (command.Operation != "compare" || *command.CompareRevision < 1 || command.Revision < 1 || *command.CompareRevision == command.Revision) {
		return errors.New("comparison requires two distinct positive retained revisions")
	}
	if command.Revision < 0 {
		return errors.New("invalid retained revision")
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
