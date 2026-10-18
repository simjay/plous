package ai

import (
	"strings"
	"testing"
)

func TestConnectionRoutingRejectsAmbiguityAndForeignTargets(t *testing.T) {
	op := &operation{o: Options{WorkspaceID: "workspace", Connections: []Connection{{ConnectionID: "kube", Tool: "kubernetes", Context: "captured", Path: "/must-not-open", Namespace: "apps"}, {ConnectionID: "helm", Tool: "helm", Context: "captured", Path: "/must-not-open", Namespace: "apps"}}}}
	for _, input := range []string{
		`{"operation":"get","resource":"pods"}`,
		`{"connection_id":"other-workspace","operation":"get","resource":"pods"}`,
		`{"connection_id":"helm","operation":"get","resource":"pods"}`,
		`{"connection_id":"kube","connection_id":"helm","operation":"get","resource":"pods"}`,
		`{"connection_id":null,"operation":"get","resource":"pods"}`,
		`{"connection_id":"kube","operation":"get","resource":"pods","namespace":"*","name":"checkout"}`,
		`{"connection_id":"kube","operation":"get","resource":"pods","namespace":"apps","namespace":"other"}`,
		`{"connection_id":"kube","operation":"get","resource":"pods","Context":"other"}`,
		`{"connection_id":"kube","operation":"get","resource":"pods"} {}`,
	} {
		if _, err := op.readKubernetes(t.Context(), input); err == nil || strings.Contains(err.Error(), "could not open") {
			t.Fatalf("invalid request reached connection: %s %v", input, err)
		}
	}
	for _, input := range []string{
		`{"connection_id":"kube","operation":"list"}`,
		`{"connection_id":"other-workspace","operation":"list"}`,
		`{"connection_id":"helm","operation":"metadata","name":"shop","namespace":"*"}`,
	} {
		if _, err := op.readHelm(t.Context(), input); err == nil || strings.Contains(err.Error(), "kubeconfig") {
			t.Fatalf("invalid Helm target: %s %v", input, err)
		}
	}
}
