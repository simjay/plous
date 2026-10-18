package source

import (
	"strings"
	"testing"
)

func TestDeprecatedKustomizationFieldsRemainInspectable(t *testing.T) {
	input := []byte("bases: [../base]\ncommonLabels: {app: demo}\npatchesStrategicMerge: []\n")
	snapshot := &Snapshot{Files: map[string][]byte{
		"prod/kustomization.yaml": input,
		"base/kustomization.yaml": []byte("resources: []\n"),
	}}
	inventory := Discover(snapshot)
	var warnings []string
	for _, finding := range inventory.Findings {
		if finding.Rule != "deprecated-kustomization-field" {
			continue
		}
		if finding.Severity != "warning" || finding.File != "prod/kustomization.yaml" || finding.Target != "kustomize:prod" {
			t.Fatalf("warning lost its source: %+v", finding)
		}
		warnings = append(warnings, finding.Message)
	}
	text := strings.Join(warnings, "\n")
	for _, field := range []string{"bases", "commonLabels", "patchesStrategicMerge"} {
		if !strings.Contains(text, "'"+field+"' is deprecated") {
			t.Fatalf("missing %s warning: %s", field, text)
		}
	}
	if len(warnings) != 3 || strings.Contains(text, "kustomize edit") || sourceTextFixture(t, snapshot, "prod/kustomization.yaml") != string(input) {
		t.Fatal("discovery changed source bytes or replaced native warnings with CLI instructions")
	}
}
