package source

import (
	"context"
	"slices"
	"testing"
)

func TestNativeBuiltinTransformerAndSourceOwnership(t *testing.T) {
	snapshot, err := Load(context.Background(), Config{Path: "../../tests/fixtures/code/transformers"})
	if err != nil {
		t.Fatal(err)
	}
	inv := Discover(snapshot)
	if len(inv.Entries) != 2 {
		t.Fatalf("expected two kustomization nodes, not their input files: %+v", inv.Entries)
	}
	entry := inv.Entry("kustomize:overlays/custom-metadata-labels")
	if entry == nil || len(entry.References) != 3 {
		t.Fatal("transformer input was not retained in node references")
	}
	if !slices.Contains(inv.Affected("plain:base/deployment.yaml"), entry.ID) {
		t.Fatal("an owned file lost its affected source links")
	}
	result, err := (Renderer{}).Render(context.Background(), inv, entry.ID, Inputs{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Resources) != 2 {
		t.Fatalf("transformer was treated as an output resource: %+v", result.Resources)
	}
	for _, resource := range result.Resources {
		metadata := resource.Object["metadata"].(map[string]any)
		labels := metadata["labels"].(map[string]any)
		if labels["owner"] != "platform" {
			t.Fatal("native builtin transformer did not run")
		}
		if resource.Kind == "Deployment" {
			spec := resource.Object["spec"].(map[string]any)
			selector := spec["selector"].(map[string]any)["matchLabels"].(map[string]any)
			if _, ok := selector["owner"]; ok {
				t.Fatal("metadata-only transformer changed the workload selector")
			}
		}
	}
}
