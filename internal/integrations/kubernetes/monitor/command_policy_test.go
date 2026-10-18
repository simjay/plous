package monitor

import (
	"strings"
	"testing"
)

func agentTestScope() CommandScope {
	return CommandScope{ConnectionID: "connection-one", Context: "test-context", Namespace: "shop"}
}

func TestAgentCommandPolicy(t *testing.T) {
	for _, definition := range commandDefinitions {
		command := Command{Operation: definition.Operation, Resource: "pods"}
		access, err := Classify(command)
		if err != nil || access != definition.Access {
			t.Fatalf("classify %q: access=%q err=%v", definition.Operation, access, err)
		}
		if access == AccessWrite && ValidateRead(command, agentTestScope()) == nil {
			t.Fatalf("write %q was allowed", definition.Operation)
		}
	}
	for _, command := range []Command{
		{Operation: "get", Resource: "pods"},
		{Operation: "get", Resource: "deployments.apps", Name: "web"},
		{Operation: "get", Resource: "events", FieldSelector: "involvedObject.name=web"},
		{Operation: "describe", Resource: "pods", Name: "web", Namespace: "shop"},
		{Operation: "logs", Resource: "pods", Name: "web", Container: "app", TailLines: 20},
	} {
		if err := ValidateRead(command, agentTestScope()); err != nil {
			t.Errorf("allowed command %#v: %v", command, err)
		}
	}
	for _, command := range []Command{
		{Operation: "exec", Resource: "pods"},
		{Operation: "rollout status", Resource: "deployments.apps"},
		{Operation: "get; apply", Resource: "pods"},
		{Operation: "get", Resource: "pods/exec"},
		{Operation: "get", Resource: "pods", Name: "$(touch /tmp/no)"},
		{Operation: "get", Resource: "secrets"},
		{Operation: "get", Resource: "secrets.example.io"},
		{Operation: "get", Resource: "pods", Namespace: "other"},
		{Operation: "get", Resource: "pods", FieldSelector: "metadata.namespace=other"},
		{Operation: "get", Resource: "pods", FieldSelector: "spec.private=hello"},
		{Operation: "get", Resource: "pods", LabelSelector: "invalid=("},
		{Operation: "get", Resource: "pods", MaxBytes: -1},
		{Operation: "get", Resource: "pods", Name: "web", FieldSelector: "status.phase=Failed"},
		{Operation: "get", Resource: "pods", Container: "app"},
		{Operation: "describe", Resource: "pods"},
		{Operation: "logs", Resource: "pods", Name: "web"},
		{Operation: "logs", Resource: "pods", Name: "web", Container: "app", TailLines: MaxAgentLines + 1},
	} {
		if ValidateRead(command, agentTestScope()) == nil {
			t.Errorf("unsafe command allowed: %#v", command)
		}
	}
	for _, scope := range []CommandScope{{}, {ConnectionID: "x", Context: "y"}, {ConnectionID: "x", Context: "y", Namespace: "bad/namespace"}} {
		if ValidateRead(Command{Operation: "get", Resource: "pods"}, scope) == nil {
			t.Errorf("invalid scope allowed: %#v", scope)
		}
	}
}

func TestAgentCommandDecoderRejectsAmbiguousInput(t *testing.T) {
	for _, input := range []string{
		`get pods`, `null`, `[]`, `{"operation":"get","resource":"pods","limit":10}`, `{"operation":"get","resource":"pods","continue":"cursor"}`, `{"operation":"get","resource":"pods","max_bytes":100}`, `{"operation":"get","raw":"/secret"}`,
		`{"operation":"get","operation":"apply","resource":"pods"}`,
		`{"operation":"get","resource":"pods"} {}`,
		`{"Operation":"get","resource":"pods"}`,
		`{"operation":"get","resource":"pods","previous":false}`,
		`{"operation":"get","resource":"pods","limit":null}`,
		`{"operation":"get","resource":"pods","limit":"2"}`,
		`{"operation":"get","resource":"pods","scope":{"context":"other"}}`,
		strings.Repeat("x", 16385),
	} {
		if _, err := DecodeCommand(input); err == nil {
			t.Errorf("unsafe input accepted: %s", input[:min(len(input), 120)])
		}
	}
	command, err := DecodeCommand(`{"operation":"get","resource":"pods"}`)
	if err != nil || command.Resource != "pods" {
		t.Fatalf("valid command: %#v %v", command, err)
	}
	definitions := ReadCommands()
	definitions[0].Operation = "apply"
	definitions[0].Arguments[0] = "raw"
	if ReadCommands()[0].Operation != "get" || ReadCommands()[0].Arguments[0] != "resource" {
		t.Fatal("caller modified command registry")
	}
}
