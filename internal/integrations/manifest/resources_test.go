package manifest

import (
	"strings"
	"testing"
)

func TestManifestResourceBoundariesAndRedaction(t *testing.T) {
	text := "# Source: app/templates/secret.yaml\napiVersion: v1\nkind: Secret\nmetadata: {name: credential}\ndata: {password: visible-secret}\n---\n# Source: app/templates/service.yaml\napiVersion: v1\nkind: Service\nmetadata: {name: shop}\nspec: {ports: [{port: 80}]}\n"
	resources, err := Resources(text)
	if err != nil || len(resources) != 2 || resources[0].Source != "app/templates/secret.yaml" {
		t.Fatalf("%+v %v", resources, err)
	}
	output := RedactedManifest(text)
	if strings.Contains(output, "visible-secret") || !strings.Contains(output, "port: 80") {
		t.Fatal(output)
	}
}

func TestMaskedComparisonShowsHiddenChangesWithoutTheirValues(t *testing.T) {
	before := "apiVersion: v1\nkind: Secret\nmetadata: {name: demo}\ndata: {password: before-credential}\n"
	after := strings.ReplaceAll(before, "before-credential", "after-credential")
	left, right := ManifestComparison(before, after, "apps")
	if left == right || !strings.Contains(right, "redacted: changed") || strings.Contains(left+right, "credential") {
		t.Fatalf("hidden change was lost or revealed: %s %s", left, right)
	}
	left, right = ManifestComparison(before, before, "apps")
	if left != right || strings.Contains(right, "redacted: changed") {
		t.Fatal("unchanged credentials appeared changed")
	}
}

func TestResourcesExpandKubernetesLists(t *testing.T) {
	resources, err := Resources("# Source: demo/templates/list.yaml\napiVersion: v1\nkind: List\nitems:\n- apiVersion: v1\n  kind: Service\n  metadata: {name: one}\n- apiVersion: v1\n  kind: ConfigMap\n  metadata: {name: two}\n")
	if err != nil || len(resources) != 2 || resources[1].Name != "two" || resources[0].Source != "demo/templates/list.yaml" {
		t.Fatalf("List objects were lost: %+v %v", resources, err)
	}
}
