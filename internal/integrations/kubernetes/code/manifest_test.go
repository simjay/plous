package code

import (
	"testing"

	"github.com/simjay/plous/internal/integrations/manifest"
)

func TestManifestDetectionDistinguishesConfigAndInvalidResource(t *testing.T) {
	for _, test := range []struct {
		data              string
		detected, invalid bool
	}{
		{"apiVersion: v1\nkind: ConfigMap\nmetadata: {name: app}\n", true, false},
		{"apiVersion: v1\nkind: ConfigMap\nmetadata: [broken", true, true},
		{"apiVersion: v1\nkind: ConfigMap\n", false, false},
		{"name: values\nreplicas: 3\n", false, false},
	} {
		detected, err := Detect([]byte(test.data))
		if detected != test.detected || (err != nil) != test.invalid {
			t.Fatalf("detected=%v error=%v for %q", detected, err, test.data)
		}
	}
}
func TestValidationRejectsMissingAndDuplicateIdentity(t *testing.T) {
	resource := manifest.Resource{APIVersion: "v1", Kind: "ConfigMap", Name: "app", Namespace: "apps"}
	if err := Validate([]manifest.Resource{resource, resource}); err == nil {
		t.Fatal("accepted duplicate identity")
	}
	resource.Name = ""
	if err := Validate([]manifest.Resource{resource}); err == nil {
		t.Fatal("accepted missing name")
	}
}
