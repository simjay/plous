package source

import (
	"context"
	"slices"
	"strings"
	"testing"

	manifest "github.com/simjay/plous/internal/integrations/manifest"
)

func TestObservedStagesPreserveRealExecutionAndFieldLineage(t *testing.T) {
	snapshot, err := Load(context.Background(), Config{Path: "../../tests/fixtures/code/inspector"})
	if err != nil {
		t.Fatal(err)
	}
	inventory := Discover(snapshot)
	result, err := (Renderer{}).Render(context.Background(), inventory, "kustomize:clusters/prod-us-east", Inputs{})
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Entries) != 10 || result.Observation == nil || result.Observation.Truncated {
		t.Fatalf("missing complete fixture observation: entries=%d observation=%+v", len(inventory.Entries), result.Observation)
	}
	index := slices.IndexFunc(result.Resources, func(r manifest.Resource) bool { return r.Kind == "Deployment" && r.Name == "prod-api" })
	if index < 0 {
		t.Fatal("renamed resource missing")
	}
	fields := Trace(inventory, result, result.Resources[index])
	field := fields[slices.IndexFunc(fields, func(f Field) bool { return f.Pointer == "/spec/replicas" })]
	if !field.ObservedComplete || len(field.Writes) != 2 {
		t.Fatalf("missing replica history: %+v", field)
	}
	if a, b := field.Writes[0], field.Writes[1]; a.Before != "1" || a.After != "4" || b.Before != "4" || b.After != "6" || a.File != "envs/prod/replicas-patch.yaml" || b.File != "envs/prod/kustomization.yaml" || b.Line == 0 {
		t.Fatalf("wrong writer evidence: %+v", field.Writes)
	}
	for _, stage := range result.Observation.Stages {
		if strings.HasPrefix(stage.Scope, "/") || strings.Contains(stage.Scope, "plous-kustomize-build") {
			t.Fatal("temporary source path escaped capture")
		}
	}
	if _, err := (Renderer{}).Render(context.Background(), inventory, "kustomize:clusters/prod-ap-south", Inputs{}); err == nil {
		t.Fatal("unmatched patch unexpectedly succeeded")
	}
}

func TestObservedFieldAlignmentUsesNamesAndRejectsAmbiguity(t *testing.T) {
	final := map[string]any{"containers": []any{map[string]any{"name": "api", "image": "new"}, map[string]any{"name": "helper", "image": "helper"}}}
	prior := map[string]any{"containers": []any{map[string]any{"name": "helper", "image": "helper"}, map[string]any{"name": "api", "image": "old"}}}
	value, ok, safe := alignedField(prior, final, "/containers/0/image")
	if !ok || !safe || value != "old" {
		t.Fatal("field followed position instead of name")
	}
	if pointer, ok := CorrespondingField(manifest.Resource{Object: prior}, manifest.Resource{Object: final}, "/containers/0/image"); !ok || pointer != "/containers/1/image" {
		t.Fatal("field pointer did not follow unique named member", pointer)
	}
	prior["containers"] = []any{map[string]any{"name": "api", "image": "one"}, map[string]any{"name": "api", "image": "two"}}
	if _, _, safe := alignedField(prior, final, "/containers/0/image"); safe {
		t.Fatal("duplicate names treated as certain lineage")
	}
	if _, ok := CorrespondingField(manifest.Resource{Object: prior}, manifest.Resource{Object: final}, "/containers/0/image"); ok {
		t.Fatal("ambiguous field pointer was accepted")
	}
	if _, _, safe := alignedField(map[string]any{"values": []any{1, 2}}, map[string]any{"values": []any{2, 1}}, "/values/0"); safe {
		t.Fatal("unnamed reordered list treated as certain")
	}
}

func TestObservedBuiltinConfigurationDoesNotClaimAnotherLabelsWriter(t *testing.T) {
	snapshot, err := Load(context.Background(), Config{Path: "../../tests/fixtures/code/transformers"})
	if err != nil {
		t.Fatal(err)
	}
	file := "overlays/custom-metadata-labels/kustomization.yaml"
	captureSourceFixture(t, snapshot)
	snapshot.Files[file] = append(snapshot.Files[file], []byte("labels:\n  - pairs: {team: platform}\n")...)
	inventory := Discover(snapshot)
	result, err := (Renderer{}).Render(context.Background(), inventory, "kustomize:overlays/custom-metadata-labels", Inputs{})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range result.Resources {
		if r.Kind != "Deployment" {
			continue
		}
		for _, field := range Trace(inventory, result, r) {
			if field.Pointer == "/metadata/labels/owner" {
				if len(field.Writes) != 1 || field.Writes[0].File != "overlays/custom-metadata-labels/metadataLabelTransformer.yaml" {
					t.Fatalf("custom builtin points at unrelated labels config: %+v", field)
				}
				return
			}
		}
	}
	t.Fatal("owner label missing")
}
