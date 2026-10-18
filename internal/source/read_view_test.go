package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/simjay/plous/internal/integrations/observation"
)

func readObservation(t *testing.T, scope ReadScope, request ReadRequest) observation.Result {
	t.Helper()
	value, err := GuardedRead(t.Context(), scope, request)
	if err != nil {
		t.Fatal(err)
	}
	result, ok := value.(observation.Result)
	if !ok || result.Validate() != nil {
		t.Fatalf("invalid projection: %+v", value)
	}
	return result
}

func TestSourceTemplatesAndSyntaxErrorsRemainDiagnosable(t *testing.T) {
	scope, root := readScope(t)
	cases := map[string]string{
		"template.yaml": "{{- if .Values.enabled }}\napiVersion: v1\nkind: ConfigMap\nmetadata: {name: demo}\n{{- end }}\n",
		"broken.yaml":   "apiVersion: v1\nkind: ConfigMap\nmetadata: [broken",
	}
	for file, body := range cases {
		if err := os.WriteFile(filepath.Join(root, file), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		r := readObservation(t, scope, ReadRequest{Operation: "read", Path: file})
		if r.Facts["excerpt"].String() != body {
			t.Fatal("diagnostic source changed", r)
		}
	}
	for _, body := range []string{"metadata: [broken\npassword: private", "{{ if .Values.enabled }}\nkind: Secret\ndata: {value: private}\n{{ end }}", "metadata: [broken\nvalue: \\u0074oken"} {
		if err := os.WriteFile(filepath.Join(root, "unsafe.yaml"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := GuardedRead(t.Context(), scope, ReadRequest{Operation: "read", Path: "unsafe.yaml"}); err == nil {
			t.Fatal("credential diagnostic was readable")
		}
	}
}

func TestSourceSearchUsesCurrentFilesWithOrWithoutDiscovery(t *testing.T) {
	scope, root := readScope(t)
	file := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(file, []byte("earlier-marker"), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := Load(t.Context(), Config{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	captured := scope
	captured.View = CaptureReadView(Discover(snapshot), nil, nil, nil)
	if err := os.WriteFile(file, []byte("current-marker"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		scope   ReadScope
		query   string
		matches int
	}{
		{"discovery does not retain old content", captured, "earlier-marker", 0},
		{"discovery permits current reads", captured, "current-marker", 1},
		{"local read sees current file", scope, "current-marker", 1},
		{"local read does not retain earlier file", scope, "earlier-marker", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := readObservation(t, test.scope, ReadRequest{Operation: "search", Query: test.query})
			if len(result.Rows) != test.matches {
				t.Fatal("search used the wrong source contents", result)
			}
		})
	}
	next, err := Load(t.Context(), Config{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	captured.View = CaptureReadView(Discover(next), nil, nil, nil)
	result := readObservation(t, captured, ReadRequest{Operation: "search", Query: "current-marker"})
	if len(result.Rows) != 1 || result.Coverage.Snapshot == next.Identity || next.Identity == snapshot.Identity {
		t.Fatal("new capture did not expose the changed file", result)
	}
}

func TestCodeReadsPreserveSnapshotFieldsAndComparison(t *testing.T) {
	scope, root := readScope(t)
	file := "deployment.yaml"
	before := "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: web}\nspec:\n  replicas: 1\n  template:\n    spec:\n      containers: [{name: web, image: web:v1}]\n"
	if err := os.WriteFile(filepath.Join(root, file), []byte(before), 0600); err != nil {
		t.Fatal(err)
	}
	snap, err := Load(t.Context(), Config{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	inventory := Discover(snap)
	target := "plain:" + file
	old, err := (Renderer{}).Render(t.Context(), inventory, target, Inputs{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, file), []byte(strings.ReplaceAll(before, "replicas: 1", "replicas: 3")), 0600); err != nil {
		t.Fatal(err)
	}
	next, err := snap.RefreshFile(file)
	if err != nil {
		t.Fatal(err)
	}
	inventory = Discover(next)
	current, err := (Renderer{}).Render(t.Context(), inventory, target, Inputs{})
	if err != nil {
		t.Fatal(err)
	}
	builds := map[string]*Result{target: current}
	scope.View = CaptureReadView(inventory, builds, nil, nil, old)
	delete(builds, target)
	if err := os.WriteFile(filepath.Join(root, file), []byte("unrelated later bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	r := readObservation(t, scope, ReadRequest{Operation: "read", Path: file})
	if r.Facts["excerpt"].String() != "unrelated later bytes" || r.ObservedAt.Before(next.Observed) || r.Coverage.Snapshot == next.Identity {
		t.Fatal("snapshot drift", r)
	}
	r = readObservation(t, scope, ReadRequest{Operation: "fields", Target: target, Resource: current.Resources[0].Key(), Field: "/spec/replicas"})
	if len(r.Rows) != 1 || r.Rows[0].Facts["value"].String() != "3" || r.Rows[0].Facts["evidence"].String() != "Exact source field" || r.Facts["stale"].String() != "false" {
		t.Fatal(r)
	}
	r = readObservation(t, scope, ReadRequest{Operation: "compare", Result: current.OperationID, Other: old.OperationID})
	found := false
	for _, row := range r.Rows {
		if row.Facts["field"].String() == "/spec/replicas" {
			found = row.Facts["before"].String() == "1" && row.Facts["after"].String() == "3"
		}
	}
	if !found {
		t.Fatal("missing field change", r)
	}
	r = readObservation(t, scope, ReadRequest{Operation: "catalog", Limit: 1})
	if r.Coverage.Complete || r.Facts["next_offset"].String() != "1" {
		t.Fatal("false catalog completeness", r)
	}
	r = readObservation(t, scope, ReadRequest{Operation: "resources", Result: "forged"})
	if r.Outcome != "unsupported" {
		t.Fatal("unknown result was treated as current", r)
	}
	for _, request := range []ReadRequest{
		{Operation: "catalog", Target: "missing"},
		{Operation: "findings", Target: "missing"},
		{Operation: "fields", Target: target, Resource: current.Resources[0].Key(), Field: "/missing"},
	} {
		if r := readObservation(t, scope, request); r.Outcome != "unsupported" || r.Coverage.Complete || r.Facts["source_identity"].String() != next.Identity {
			t.Fatal("missing selection claimed complete coverage", r)
		}
	}
}

func TestGitSnapshotReadsStayPinnedAndDenyExcludedFiles(t *testing.T) {
	scope, _ := readScope(t)
	scope.Root = ""
	snap := &Snapshot{Config: Config{Repository: "https://example.invalid/repo.git"}, Identity: "snapshot-one", Revision: "commit-one", Observed: time.Now().UTC(), Files: map[string][]byte{"main.txt": []byte("captured text"), ".env": []byte("private"), "config.yaml": []byte("password: private"), "binary.txt": {0xff, 0}}}
	scope.View = CaptureReadView(Discover(snap), nil, nil, nil)
	for _, op := range []ReadRequest{{Operation: "read", Path: "main.txt"}, {Operation: "list"}, {Operation: "search", Query: "captured"}} {
		r := readObservation(t, scope, op)
		if r.Facts["revision"].String() != "commit-one" || strings.Contains(string(sourceJSON(r)), "private") {
			t.Fatal(r)
		}
	}
	for _, path := range []string{".env", "config.yaml", "../main.txt", "binary.txt"} {
		if _, err := GuardedRead(t.Context(), scope, ReadRequest{Operation: "read", Path: path}); err == nil {
			t.Fatal("read excluded file", path)
		}
	}
	scope.Root = scope.ProfileRoot
	if _, err := GuardedRead(t.Context(), scope, ReadRequest{Operation: "catalog"}); err == nil {
		t.Fatal("snapshot bypassed profile boundary")
	}
}

func TestFindingsDoNotClaimUnrenderedTargetsAreHealthy(t *testing.T) {
	scope, root := readScope(t)
	snap := &Snapshot{Config: Config{Path: root}, Identity: "source", Observed: time.Now().UTC(), Files: map[string][]byte{"deployment.yaml": []byte("apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: web}\n")}}
	scope.View = CaptureReadView(Discover(snap), nil, nil, nil)
	r := readObservation(t, scope, ReadRequest{Operation: "findings"})
	if r.Coverage.Complete || !strings.Contains(r.SearchText(), "not rendered") {
		t.Fatal(r)
	}
}

func TestCapturedCatalogAndFieldHistoryUseSharedAnalysis(t *testing.T) {
	snapshot, err := Load(t.Context(), Config{Path: "../../tests/fixtures/code/inspector"})
	if err != nil {
		t.Fatal(err)
	}
	inventory := Discover(snapshot)
	target := "kustomize:clusters/prod-us-east"
	result, err := (Renderer{}).Render(t.Context(), inventory, target, Inputs{})
	if err != nil {
		t.Fatal(err)
	}
	scope := ReadScope{Root: snapshot.Config.Path, ProfileRoot: t.TempDir(), ConnectionID: "src", View: CaptureReadView(inventory, map[string]*Result{target: result}, nil, nil)}
	catalog := readObservation(t, scope, ReadRequest{Operation: "catalog", Target: target})
	dependency := false
	for _, row := range catalog.Rows {
		if row.Identity.Kind == "Entry" {
			dependency = row.Facts["dependency.0"].String() == "kustomize:envs/prod"
		}
	}
	if !dependency {
		t.Fatal("captured dependency absent", catalog)
	}
	resource := ""
	for _, r := range result.Resources {
		if r.Kind == "Deployment" && r.Name == "prod-api" {
			resource = r.Key()
		}
	}
	if resource == "" {
		t.Fatal("fixture deployment absent")
	}
	query := ReadRequest{Operation: "fields", Target: target, Resource: resource, Field: "/spec/replicas", Limit: 1}
	first := readObservation(t, scope, query)
	if first.Coverage.Complete || len(first.Rows) != 1 || first.Rows[0].Facts["value"].Number == nil || first.Rows[0].Facts["value"].String() != "6" {
		t.Fatal(first)
	}
	query.Offset = 1
	second := readObservation(t, scope, query)
	if second.Rows[0].Identity.Kind != "FieldWrite" || second.Rows[0].Facts["before"].String() != "1" || second.Rows[0].Facts["after"].String() != "4" || second.Rows[0].Facts["file"].String() != "envs/prod/replicas-patch.yaml" {
		t.Fatal(second)
	}
	query.Offset = 2
	third := readObservation(t, scope, query)
	if third.Rows[0].Facts["before"].String() != "4" || third.Rows[0].Facts["after"].String() != "6" || third.Coverage.Snapshot != first.Coverage.Snapshot {
		t.Fatal(third)
	}
}
