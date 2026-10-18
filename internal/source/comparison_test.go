package source

import (
	"context"
	"strings"
	"testing"

	manifest "github.com/simjay/plous/internal/integrations/manifest"
)

func TestComparisonMatchesVerifiedRenamesAcrossNamespaces(t *testing.T) {
	snapshot, err := Load(context.Background(), Config{Path: "../../tests/fixtures/code/inspector"})
	if err != nil {
		t.Fatal(err)
	}
	inventory := Discover(snapshot)
	before, err := (Renderer{}).Render(context.Background(), inventory, "kustomize:clusters/dev-local", Inputs{})
	if err != nil {
		t.Fatal(err)
	}
	after, err := (Renderer{}).Render(context.Background(), inventory, "kustomize:clusters/prod-us-east", Inputs{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range Compare(before, after) {
		if c.After.Kind == "Deployment" && c.After.Name == "prod-api" {
			found = true
			if c.Before.Name != "api" || c.Match != "Verified source identity" || c.Status != "Changed" {
				t.Fatalf("rename did not preserve source: %+v; %v %v", c, before.Lineages, after.Lineages)
			}
		}
	}
	if !found {
		t.Fatal("api comparison missing")
	}
}
func TestComparisonRejectsAmbiguousOrConflictingLineage(t *testing.T) {
	r := func(name string) manifest.Resource {
		return manifest.Resource{APIVersion: "v1", Kind: "Service", Name: name, Object: map[string]any{"metadata": map[string]any{"name": name}}}
	}
	a, b, c := r("a"), r("b"), r("c")
	before := &Result{BuildResult: &BuildResult{Resources: []manifest.Resource{a, b}}, Lineages: map[string]string{a.Key(): "shared", b.Key(): "shared"}}
	after := &Result{BuildResult: &BuildResult{Resources: []manifest.Resource{c}}, Lineages: map[string]string{c.Key(): "shared"}}
	changes := Compare(before, after)
	if len(changes) != 3 {
		t.Fatalf("ambiguous resources merged: %+v", changes)
	}
	for _, change := range changes {
		if change.Match != "Ambiguous source identity" {
			t.Fatal(change.Match)
		}
	}
	before.Resources = []manifest.Resource{a}
	before.Lineages = map[string]string{a.Key(): "old-document"}
	after.Resources = []manifest.Resource{a}
	after.Lineages = map[string]string{a.Key(): "different-document"}
	if len(Compare(before, after)) != 2 {
		t.Fatal("same final name overrode conflicting source evidence")
	}
}
func TestPreviewLineFollowsReformattedRedactedSource(t *testing.T) {
	original := "# comment\nkind: Secret\napiVersion: v1\nmetadata: {name: demo}\nstringData:\n  password: private\n---\n# another comment\nreplicas:\n- name: api\n  count: 6\n"
	preview := "apiVersion: v1\nkind: Secret\nmetadata:\n  name: demo\nstringData:\n  password: '[redacted]'\n---\nreplicas:\n- count: 6\n  name: api\n"
	if got := PreviewLine(original, preview, 11); got != 9 {
		t.Fatalf("mapped second document to %d", got)
	}
	if got := PreviewLine(original, preview, 4); got != 4 {
		t.Fatalf("mapped metadata to %d", got)
	}
	if got := PreviewLine(original, "Invalid YAML; preview unavailable.", 11); got != 0 {
		t.Fatal("invented a source location")
	}
}
func TestValidationInventoryMasksGeneratorsAndFindsStructuralCandidates(t *testing.T) {
	snapshot := &Snapshot{Identity: "test", Files: map[string][]byte{
		"base/kustomization.yaml":   []byte("resources: [service.yaml]\nsecretGenerator:\n- name: credentials\n  literals: [password=private-literal]\n  envs: [secrets.env]\n"),
		"base/service.yaml":         []byte("apiVersion: v1\nkind: Service\nmetadata: {name: api}\n"),
		"base/secrets.env":          []byte("TOKEN=private-token\n"),
		"dev/kustomization.yaml":    []byte("resources: [../base]\npatches: [{path: patch.yaml}]\n"),
		"prod/kustomization.yaml":   []byte("resources: [../base]\npatches: [{path: patch.yaml}]\n"),
		"dev/patch.yaml":            []byte("apiVersion: v1\nkind: Service\nmetadata: {name: api, labels: {team: platform}}\n"),
		"prod/patch.yaml":           []byte("apiVersion: v1\nkind: Service\nmetadata: {name: api, labels: {team: platform}}\n"),
		"unused/kustomization.yaml": []byte("apiVersion: kustomize.config.k8s.io/v1alpha1\nkind: Component\n"),
		"standalone.yaml":           []byte("apiVersion: v1\nkind: Service\nmetadata: {name: standalone}\n"),
	}}
	inventory := Discover(snapshot)
	rows := inventory.Generators()
	if len(rows) != 1 || !strings.Contains(rows[0].Message, "TOKEN") || !strings.Contains(rows[0].Message, "password") || strings.Contains(rows[0].Message, "private-") {
		t.Fatalf("unsafe or missing generator inventory: %+v", rows)
	}
	rules := map[string]bool{}
	for _, f := range inventory.Findings {
		rules[f.Rule] = true
	}
	for _, rule := range []string{"duplicate-patch", "unreferenced-manifest", "unused-component"} {
		if !rules[rule] {
			t.Fatal("missing", rule)
		}
	}
}

func TestResourceComparisonPreservesHiddenChangesAndIdentity(t *testing.T) {
	old, _ := manifest.Resources("apiVersion: v1\nkind: Secret\nmetadata: {name: demo, namespace: apps}\ndata: {password: before}\n")
	current, _ := manifest.Resources("apiVersion: v1\nkind: Secret\nmetadata: {name: demo, namespace: apps}\ndata: {password: after}\n")
	changes := Compare(&Result{BuildResult: &BuildResult{Resources: old}}, &Result{BuildResult: &BuildResult{Resources: current}})
	if len(changes) != 1 || changes[0].Status != "Changed" {
		t.Fatal(changes)
	}
	left, right := manifest.MaskedComparison(changes[0].Before.Object, changes[0].After.Object)
	if !strings.Contains(right, "redacted: changed") || strings.Contains(left, "before") || strings.Contains(right, "password: after") {
		t.Fatal("hidden change was lost or revealed", left, right)
	}
	current[0].Namespace = "other"
	changes = Compare(&Result{BuildResult: &BuildResult{Resources: old}}, &Result{BuildResult: &BuildResult{Resources: current}})
	if len(changes) != 2 {
		t.Fatal("different namespaces were silently paired")
	}
}
