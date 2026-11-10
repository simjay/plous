package monitor

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

const ExecHelp = "Run a bounded noninteractive command in an existing container. Requires exact Pod UID and an argument array. Can mutate application state. Remote outcome may be unknown on timeout; do not automatically retry. Restricted to the captured default namespace. Output is paged in saved rows."
const DiagnosticHelp = "Run one HTTP GET, DNS lookup, or TCP connection test in a temporary Job using the host-configured image. Creates cluster resources and cleans them up. The runner has its own environment and policy. Inspect exit code and cleanup. Restricted to the captured default namespace. Output is paged in saved rows."

// DebugPolicy is host configuration, never a tool argument.
type DebugPolicy struct {
	Exec  bool
	Image string
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

func DebugSchema(tool string) map[string]any {
	p := map[string]any{
		"namespace":       map[string]any{"type": "string", "minLength": 1},
		"timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 120, "default": 30},
	}
	required := []string{"namespace"}
	if tool == "kubernetes_exec_run" {
		for _, k := range []string{"name", "uid", "container"} {
			p[k] = map[string]any{"type": "string", "minLength": 1}
			required = append(required, k)
		}
		p["command"] = map[string]any{"type": "array", "minItems": 1, "maxItems": 64, "items": map[string]any{"type": "string", "maxLength": 4096}}
		required = append(required, "command")
	} else {
		p["test"] = map[string]any{"type": "string", "enum": []string{"http", "dns", "tcp"}}
		p["target"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 2048}
		p["port"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 65535}
		required = append(required, "test", "target")
	}
	return map[string]any{"type": "object", "properties": p, "required": required, "additionalProperties": false}
}

func DecodeDebug(tool, input string, policy DebugPolicy, namespace string) (DebugCommand, error) {
	var c DebugCommand
	if tool != "kubernetes_exec_run" && tool != "kubernetes_diagnostic_run" {
		return c, errors.New("unknown debugging tool")
	}
	d := json.NewDecoder(strings.NewReader(input))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, errors.New("invalid debugging arguments")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return c, errors.New("expected one debugging argument object")
	}
	if len(validation.IsDNS1123Label(c.Namespace)) != 0 || c.Namespace != namespace {
		return c, errors.New("active debugging requires the captured connection's default namespace")
	}
	if c.TimeoutSeconds == 0 {
		c.TimeoutSeconds = 30
	}
	if c.TimeoutSeconds < 1 || c.TimeoutSeconds > 120 {
		return c, errors.New("timeout_seconds must be 1-120")
	}
	if tool == "kubernetes_exec_run" {
		if !policy.Exec {
			return c, errors.New("container exec is not enabled by the host")
		}
		if len(validation.IsDNS1123Subdomain(c.Name)) != 0 || c.UID == "" || len(c.UID) > 128 || len(validation.IsDNS1123Label(c.Container)) != 0 || len(c.Command) == 0 || len(c.Command) > 64 || c.Command[0] == "" {
			return c, errors.New("exec requires a Pod name, exact UID, container, and command argument array")
		}
		if c.Test != "" || c.Target != "" || c.Port != 0 {
			return c, errors.New("exec does not accept diagnostic test arguments")
		}
		for _, arg := range c.Command {
			if len(arg) > 4096 || strings.ContainsRune(arg, 0) {
				return c, errors.New("command argument exceeds its limit or contains NUL")
			}
		}
	} else {
		if policy.Image == "" {
			return c, errors.New("temporary diagnostics require a host-configured image")
		}
		if c.Name != "" || c.UID != "" || c.Container != "" || c.Command != nil {
			return c, errors.New("temporary diagnostics do not accept Pod or command overrides")
		}
		if _, err := c.ProbeCommand(); err != nil {
			return c, err
		}
	}
	return c, nil
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
