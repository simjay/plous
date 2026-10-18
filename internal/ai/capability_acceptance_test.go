package ai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
	journal "github.com/simjay/plous/internal/ai/history"
	"github.com/simjay/plous/internal/source"
)

func TestControllerEvidenceUsesSharedParsersAndSurvivesRestart(t *testing.T) {
	o, h := workspaceFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		group, version, resource, kind := "argoproj.io", "v1alpha1", "applications", "Application"
		if strings.Contains(r.URL.Path, "kustomize.toolkit.fluxcd.io") {
			group, version, resource, kind = "kustomize.toolkit.fluxcd.io", "v1", "kustomizations", "Kustomization"
		}
		if r.URL.Path == "/apis/"+group {
			json.NewEncoder(w).Encode(map[string]any{"kind": "APIGroup", "preferredVersion": map[string]string{"groupVersion": group + "/" + version, "version": version}})
			return
		}
		if r.URL.Path == "/apis/"+group+"/"+version {
			json.NewEncoder(w).Encode(map[string]any{"kind": "APIResourceList", "resources": []any{map[string]any{"name": resource, "kind": kind, "namespaced": true, "verbs": []string{"get", "list"}}}})
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/namespaces/apps/"+resource+"/web") {
			t.Errorf("unexpected API request: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		status := map[string]any{"sync": map[string]string{"status": "OutOfSync", "revision": "commit-one"}, "health": map[string]string{"status": "Degraded"}, "conditions": []any{map[string]any{"type": "Ready", "status": "True", "observedGeneration": 1}}}
		json.NewEncoder(w).Encode(map[string]any{"apiVersion": group + "/" + version, "kind": kind, "metadata": map[string]any{"name": "web", "namespace": "apps", "generation": 2}, "spec": map[string]any{"suspend": true}, "status": status})
	}))
	defer server.Close()
	config := filepath.Join(t.TempDir(), "config")
	data := fmt.Sprintf("apiVersion: v1\nkind: Config\nclusters:\n- name: fixture\n  cluster:\n    server: %s\ncontexts:\n- name: fixture\n  context:\n    cluster: fixture\n    user: fixture\n    namespace: apps\nusers:\n- name: fixture\n  user: {}\ncurrent-context: fixture\n", server.URL)
	if err := os.WriteFile(config, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	o.Connections = []Connection{{ConnectionID: "cluster", Tool: "kubernetes", Path: config, Context: "fixture", Namespace: "apps"}}
	o.Model = &scriptedModel{steps: []*schema.Message{
		call("argo_resources_read", `{"purpose":"inspect Argo health","connection_id":"cluster","operation":"get","resource":"applications.argoproj.io","name":"web"}`),
		call("flux_resources_read", `{"purpose":"inspect Flux readiness","connection_id":"cluster","operation":"get","resource":"kustomizations.kustomize.toolkit.fluxcd.io","name":"web"}`), answer("Argo is degraded. Flux reconciliation is suspended and readiness is stale."),
	}}
	nativeOptions(t, &o)
	result, err := Run(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	if err = h.SaveResponse(o.WorkspaceID, o.ConversationID, o.TurnID, journal.Response{Status: "completed", Text: result.Text}); err != nil {
		t.Fatal(err)
	}
	server.Close()
	if err = h.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := journal.Open(o.Store)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	evidence, err := reopened.TurnEvidence(t.Context(), o.WorkspaceID, o.ConversationID, o.TurnID)
	if err != nil || len(evidence) != 2 {
		t.Fatal(evidence, err)
	}
	facts := evidence[0].Projection.Rows[0].Facts
	if facts["sync"].String() != "OutOfSync" || facts["health"].String() != "Degraded" || facts["detail.compared_revision"].String() != "commit-one" {
		t.Fatal(facts)
	}
	facts = evidence[1].Projection.Rows[0].Facts
	if facts["status"].String() != "Suspended" || facts["stale"].String() != "true" || facts["generation"].String() != "2" {
		t.Fatal(facts)
	}
}

func TestCapturedCodeFieldEvidenceSurvivesWithoutSourceFiles(t *testing.T) {
	o, h := workspaceFixture(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "deployment.yaml")
	if err = os.WriteFile(file, []byte("apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: web}\nspec: {replicas: 3}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	snap, err := source.Load(t.Context(), source.Config{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	inventory := source.Discover(snap)
	target := "plain:deployment.yaml"
	rendered, err := (source.Renderer{}).Render(t.Context(), inventory, target, source.Inputs{})
	if err != nil {
		t.Fatal(err)
	}
	o.Connections = []Connection{{ConnectionID: "src", Tool: "source", Path: root, SourceView: source.CaptureReadView(inventory, map[string]*source.Result{target: rendered}, nil, nil)}}
	args := map[string]string{"purpose": "explain replicas", "connection_id": "src", "operation": "fields", "target": target, "resource": rendered.Resources[0].Key(), "field": "/spec/replicas"}
	model := &scriptedModel{steps: []*schema.Message{call("source_analysis_read", string(marshal(args))), answer("Three replicas, from deployment.yaml.")}}
	o.Model = model
	nativeOptions(t, &o)
	if _, err = Run(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err = h.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := journal.Open(o.Store)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	evidence, err := reopened.TurnEvidence(t.Context(), o.WorkspaceID, o.ConversationID, o.TurnID)
	if err != nil || len(evidence) != 1 || evidence[0].Projection == nil {
		t.Fatal(evidence, err)
	}
	r := evidence[0].Projection
	if r.Coverage.Snapshot != rendered.OperationID || r.Rows[0].Facts["value"].String() != "3" || r.Rows[0].Facts["line"].String() != "4" {
		t.Fatal(r)
	}
	if !strings.Contains(string(marshal(model.inputs)), "Exact source field") {
		t.Fatal("provenance did not reach model")
	}
}

func TestToolCatalogMatchesCapturedScope(t *testing.T) {
	for _, test := range []struct {
		o      Options
		want   []string
		absent []string
	}{
		{Options{}, []string{"history_read"}, []string{"kubernetes_resources_read", "source_files_read", "workspace_read", "argo_resources_read", "flux_resources_read"}},
		{Options{Connections: []Connection{{Tool: "kubernetes"}, {Tool: "source"}}}, []string{"history_read"}, []string{"kubernetes_resources_read", "source_files_read", "workspace_read", "argo_resources_read", "flux_resources_read"}},
		{Options{WorkspaceID: "scope", Connections: []Connection{{Tool: "source"}}}, []string{"source_files_read", "source_analysis_read", "history_read", "workspace_read"}, []string{"recall", "kubernetes_resources_read", "argo_resources_read", "flux_resources_read"}},
		{Options{WorkspaceID: "scope", Connections: []Connection{{Tool: "kubernetes"}}}, []string{"kubernetes_resources_read", "kubernetes_logs_read", "kubernetes_usage_read", "argo_resources_read", "flux_resources_read"}, []string{"source_files_read", "recall"}},
	} {
		names := map[string]bool{}
		for _, tool := range ToolDefinitions(test.o) {
			names[tool.Name] = true
		}
		for _, name := range test.want {
			if !names[name] {
				t.Fatal("missing", name, names)
			}
		}
		for _, name := range test.absent {
			if names[name] {
				t.Fatal("unexpected", name, names)
			}
		}
	}
	op := &operation{o: Options{WorkspaceID: "scope", Connections: []Connection{{ConnectionID: "cluster", Tool: "kubernetes", TargetCaptured: true}}}}
	for _, tool := range []string{"argo_resources_read", "flux_resources_read"} {
		if _, err := op.externalRead(t.Context(), tool, `{"connection_id":"cluster","operation":"get","resource":"secrets"}`); err == nil || strings.Contains(err.Error(), "captured connection is unavailable") {
			t.Fatal("invalid controller request reached acquisition", err)
		}
	}
}
