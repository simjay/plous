package code

import (
	"context"
	"strings"
	"testing"
)

func TestBuiltinTransformersRetainExecutionBoundary(t *testing.T) {
	for _, version := range []string{"builtin", "example.invalid/v1"} {
		t.Run(version, func(t *testing.T) {
			root := sourceFiles(t, map[string]string{
				"kustomization.yaml": "resources: []\ntransformers: [transformer.yaml]\n",
				"transformer.yaml":   "apiVersion: " + version + "\nkind: LabelTransformer\nmetadata: {name: labels}\nlabels: {team: platform}\nfieldSpecs: [{path: metadata/labels, create: true}]\n",
			})
			s := fixtureInventory(t, root)
			called := false
			_, err := Build(context.Background(), s, ".", func(context.Context, string) ([]byte, string, error) { called = true; return nil, "fixture", nil })
			if version == "builtin" {
				if err != nil || !called {
					t.Fatal("builtin was rejected", err)
				}
			} else if err == nil || called || !strings.Contains(err.Error(), "external transformer") {
				t.Fatal("external transformer reached execution", err)
			}
		})
	}
}
