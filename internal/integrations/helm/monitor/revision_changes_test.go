package monitor

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	helmcommon "github.com/simjay/plous/internal/integrations/helm/common"
)

func TestRevisionChangesRequireReleaseIdentityAndMaskHiddenChanges(t *testing.T) {
	before := &Snapshot{Name: "shop", Namespace: "apps", Version: 3, Manifest: "apiVersion: v1\nkind: Secret\nmetadata: {name: credentials}\ndata: {password: old-secret}\n", Config: map[string]any{"password": "old-value"}}
	after := *before
	after.Version = 4
	after.Manifest = strings.ReplaceAll(before.Manifest, "old-secret", "new-secret")
	after.Config = map[string]any{"password": "new-value"}
	changes, err := CompareRevisions(before, &after)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"old-secret", "new-secret", "old-value", "new-value"} {
		if strings.Contains(changes.Before+changes.After+fmt.Sprint(changes.Changes), secret) {
			t.Fatalf("comparison disclosed %s", secret)
		}
	}
	if !strings.Contains(changes.After, "redacted: changed") || changes.Added == 0 || changes.Removed == 0 {
		t.Fatal("hidden change disappeared", changes)
	}
	if len(changes.Changes) != 2 || changes.Changes[0].Action != "changed" || changes.Changes[1].Label != "Supplied values" {
		t.Fatal("hidden changes disappeared from the object summary", changes.Changes)
	}
	for _, change := range []func(*Snapshot){func(s *Snapshot) { s.Version = 0 }, func(s *Snapshot) { s.Version = after.Version }, func(s *Snapshot) { s.Namespace = "other" }, func(s *Snapshot) { s.Name = "other" }} {
		invalid := *before
		change(&invalid)
		if _, err := CompareRevisions(&invalid, &after); err == nil {
			t.Fatal("accepted invalid revision or wrong identity")
		}
	}
	if _, err := CompareRevisions(nil, &after); err == nil {
		t.Fatal("pruned baseline became initial installation")
	}
}
func TestRevisionChangesCanonicalResourcesAndInitialInstall(t *testing.T) {
	a := "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: first}\ndata: {value: one}\n"
	b := "apiVersion: v1\nkind: Service\nmetadata: {name: second}\nspec: {type: ClusterIP}\n"
	before := &Snapshot{Name: "shop", Namespace: "apps", Version: 1, Manifest: a + "---\n" + b}
	after := *before
	after.Version = 2
	after.Manifest = b + "---\n" + a
	changes, err := CompareRevisions(before, &after)
	if err != nil || changes.Added+changes.Removed != 0 {
		t.Fatal("manifest order produced false changes", err, changes)
	}
	if len(changes.Changes) != 0 {
		t.Fatal("manifest order changed the object summary", changes.Changes)
	}
	initial, err := CompareRevisions(nil, before)
	if err != nil || initial.Before != "" || initial.Added == 0 || initial.Removed != 0 {
		t.Fatal("initial installation baseline", err, initial)
	}
	after.Info.Status = "failed"
	after.Manifest = strings.ReplaceAll(a, "value: one", "value: two")
	changes, err = CompareRevisions(before, &after)
	if err != nil || !strings.Contains(changes.After, "redacted: changed") || strings.Contains(changes.After, "second") {
		t.Fatal("failed revision or removed resource disappeared", err)
	}
}

func TestRevisionSummarySeparatesHooksAndSurvivesLineLimit(t *testing.T) {
	before := &Snapshot{Name: "shop", Namespace: "apps", Version: 1}
	var manifest strings.Builder
	for i := 0; i < 2600; i++ {
		fmt.Fprintf(&manifest, "apiVersion: v1\nkind: Service\nmetadata: {name: service-%04d}\nspec: {type: ClusterIP}\n---\n", i)
	}
	before.Manifest = manifest.String()
	if err := json.Unmarshal([]byte(`{"hooks":[{"name":"preflight","events":["pre-install"],"manifest":"apiVersion: batch/v1\nkind: Job\nmetadata: {name: preflight}\n"}]}`), before); err != nil {
		t.Fatal(err)
	}
	after := *before
	after.Version = 2
	after.Manifest = strings.Replace(before.Manifest, "type: ClusterIP", "type: NodePort", 1)
	after.Hooks = nil
	changes, err := CompareRevisions(before, &after)
	want := []RevisionChange{
		{Group: "resources", Label: "Service / apps/service-0000", Action: "changed"},
		{Group: "hooks", Label: "Hook preflight · Job / apps/preflight", Action: "removed"},
	}
	if err != nil || changes.Unavailable == "" || !reflect.DeepEqual(changes.Changes, want) {
		t.Fatalf("large-chart summary: %v %#v", err, changes)
	}
	if changes.Before != "" || changes.After != "" || changes.Diff != "" {
		t.Fatal("oversized diff was retained")
	}
	after.Manifest, after.Hooks = before.Manifest, before.Hooks
	changes, err = CompareRevisions(before, &after)
	if err != nil || changes.Unavailable != "" || len(changes.Changes) != 0 || changes.Before != "" || changes.After != "" {
		t.Fatal("unchanged large chart produced a size warning or retained a full diff", err)
	}
}
func TestRevisionChangesKeepSummaryWhenFullComparisonIsTooLarge(t *testing.T) {
	after := &Snapshot{Name: "shop", Namespace: "apps", Version: 1, Config: map[string]any{}}
	for _, key := range strings.Fields("a b c d e f g h i j k l") {
		after.Config[key] = "example"
	}
	changes, err := CompareRevisions(nil, after)
	if err != nil || len(changes.Changes) != 1 || changes.Changes[0].Label != "Supplied values" || !strings.Contains(changes.After, "l:") {
		t.Fatal(err, changes)
	}
	after.Config["large"] = strings.Repeat("x", 2<<20)
	changes, err = CompareRevisions(nil, after)
	if err != nil || changes.Unavailable == "" || len(changes.Changes) != 1 || changes.Before != "" || changes.After != "" || changes.Diff != "" {
		t.Fatal("large comparison lost its summary or retained unbounded text", err, changes)
	}
}

func TestRevisionChangesSummarizeObjectsAndInputsWithoutYAML(t *testing.T) {
	before := &Snapshot{Name: "web", Namespace: "apps", Version: 1, Chart: helmcommon.Chart{Metadata: map[string]any{"name": "demo", "version": "1"}}, Manifest: "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: web}\nspec: {replicas: 1}\n---\napiVersion: v1\nkind: Service\nmetadata: {name: old}\n"}
	after := *before
	after.Version = 2
	after.Chart = helmcommon.Chart{Metadata: map[string]any{"name": "demo", "version": "2"}}
	after.Manifest = "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: web}\nspec: {replicas: 2}\n"
	changes, err := CompareRevisions(before, &after)
	if err != nil {
		t.Fatal(err)
	}
	want := []RevisionChange{
		{Group: "resources", Label: "Deployment / apps/web", Action: "changed"},
		{Group: "resources", Label: "Service / apps/old", Action: "removed"},
		{Group: "chart", Label: "Chart metadata", Action: "changed"},
	}
	if !reflect.DeepEqual(changes.Changes, want) {
		t.Fatalf("summary does not describe exact changes: %#v", changes.Changes)
	}
	for _, want := range []string{"replicas: 1", "replicas: 2"} {
		if !strings.Contains(changes.Diff, want) {
			t.Fatal("full comparison lost configuration", want)
		}
	}
	again, err := CompareRevisions(before, &after)
	if err != nil || !reflect.DeepEqual(again.Changes, changes.Changes) {
		t.Fatal("summary ordering not deterministic")
	}
}
