package code

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	manifest "github.com/simjay/plous/internal/integrations/manifest"
)

func TestObservationPreservesEngineOutputAndMasksSecrets(t *testing.T) {
	ctx := context.Background()
	snapshot := fixtureInventory(t, "../../../../tests/fixtures/code/inspector")
	observed, err := Build(ctx, snapshot, "clusters/prod-us-east", nil)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := Build(ctx, snapshot, "clusters/prod-us-east", func(ctx context.Context, dir string) ([]byte, string, error) {
		return observedBuild(ctx, dir, func() error { return nil }, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(observed.Resources) != len(plain.Resources) {
		t.Fatal("observation changed resource count")
	}
	for n, r := range plain.Resources {
		if manifest.JSON(r.Object) != manifest.JSON(observed.Resources[n].Object) {
			t.Fatal("observer changed authoritative engine output")
		}
	}
	snapshot.Files["secret/kustomization.yaml"] = []byte("resources: [secret.yaml]\nnamePrefix: changed-\n")
	snapshot.Files["secret/secret.yaml"] = []byte("apiVersion: v1\nkind: Secret\nmetadata: {name: credentials}\nstringData: {password: private-test-value}\n")
	snapshot = Discover(snapshot.Files)
	result, err := Build(ctx, snapshot, "secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range result.Observation.Stages {
		for _, set := range []map[int]manifest.Resource{stage.Before, stage.After} {
			for _, r := range set {
				if strings.Contains(manifest.JSON(r.Object), "private-test-value") {
					t.Fatal("stage capture leaked an unmasked secret")
				}
			}
		}
		if filepath.IsAbs(stage.Scope) {
			t.Fatal("stage contains host source path")
		}
	}
}
