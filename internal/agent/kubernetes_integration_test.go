package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/simjay/plous/internal/memory"
)

func scopedKubernetesFixture(t *testing.T) (*KubernetesScope, *atomic.Int32) {
	t.Helper()
	requests := &atomic.Int32{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "Bearer fixture-kube-token" {
			t.Error("fixture did not load the selected Kubernetes credential")
		}
		if r.Method != http.MethodGet {
			t.Errorf("assistant used a write method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1":
			fmt.Fprint(w, `{"apiVersion":"v1","kind":"APIResourceList","resources":[{"name":"pods","kind":"Pod","namespaced":true,"verbs":["get","list"]}]}`)
		case "/api/v1/namespaces/payments/pods/checkout":
			fmt.Fprint(w, `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"checkout","namespace":"payments"},"status":{"phase":"Running"},"debug":"fixture-kube-token","nested":[{"kind":"Secret","data":{"token":"private-payload"}}]}`)
		default:
			t.Errorf("request escaped the captured namespace: %s", r.URL)
			http.Error(w, "wrong scope", http.StatusForbidden)
		}
	}))
	t.Cleanup(server.Close)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("assistant used kubeconfig current-context instead of the captured context")
		http.Error(w, "wrong context", http.StatusForbidden)
	}))
	t.Cleanup(other.Close)
	path := filepath.Join(t.TempDir(), "config")
	config := fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: other
contexts:
- name: captured
  context: {cluster: allowed, user: fixture, namespace: payments}
- name: other
  context: {cluster: other, user: fixture, namespace: forbidden}
clusters:
- name: allowed
  cluster: {server: %q, insecure-skip-tls-verify: true}
- name: other
  cluster: {server: %q}
users:
- name: fixture
  user: {token: fixture-kube-token}
`, server.URL, other.URL)
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != config {
			t.Errorf("assistant changed the kubeconfig: %v", err)
		}
	})
	return &KubernetesScope{Path: path, Context: "captured", ConnectionID: "connection-one"}, requests
}

func TestAgentKubernetesCapturedScopeAndSanitizedEvidence(t *testing.T) {
	o, store := fixture(t)
	scope, requests := scopedKubernetesFixture(t)
	o.Kubernetes = scope
	fake := &scriptedModel{steps: []*schema.Message{
		call("kubernetes_read", `{"operation":"get","resource":"pods","name":"checkout"}`),
		answer("The Pod is running."),
	}}
	o.Model = fake
	result, err := Run(context.Background(), o)
	if err != nil || result.Text != "The Pod is running." || requests.Load() != 2 {
		t.Fatalf("result=%+v err=%v requests=%d", result, err, requests.Load())
	}
	toolResult := fake.inputs[1][len(fake.inputs[1])-1].Content
	var evidence struct {
		Evidence string `json:"evidence"`
		Result   struct {
			Source struct {
				Context   string `json:"context"`
				Namespace string `json:"namespace"`
			} `json:"source"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(toolResult), &evidence); err != nil {
		t.Fatal(err)
	}
	if evidence.Evidence == "" || evidence.Result.Source.Context != "captured" || evidence.Result.Source.Namespace != "payments" {
		t.Fatalf("wrong evidence scope: %s", toolResult)
	}
	if strings.Contains(toolResult, "fixture-kube-token") || strings.Contains(toolResult, "private-payload") {
		t.Fatal("credentials reached the model")
	}
	mem, err := memory.New(store, "", o.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"fixture-kube-token", "private-payload"} {
		page, err := mem.Search(context.Background(), memory.Query{Source: "reasoning", Text: secret})
		if err != nil || len(page.Records) != 0 {
			t.Fatalf("credentials reached saved evidence: %s %v", secret, err)
		}
	}
}

func TestAgentKubernetesNoPrefetchWithoutTool(t *testing.T) {
	o, _ := fixture(t)
	o.Kubernetes = &KubernetesScope{Path: "/must-not-open-kubeconfig", Context: "captured", ConnectionID: "connection-one"}
	o.Model = &scriptedModel{steps: []*schema.Message{answer("Hello")}}
	result, err := Run(context.Background(), o)
	if err != nil || result.Text != "Hello" {
		t.Fatalf("ordinary answer tried to initialize Kubernetes: %+v %v", result, err)
	}
}

func TestAgentKubernetesDefaultNamespaceCannotBeOverridden(t *testing.T) {
	o, _ := fixture(t)
	scope, requests := scopedKubernetesFixture(t)
	o.Kubernetes = scope
	fake := &scriptedModel{steps: []*schema.Message{
		call("kubernetes_read", `{"operation":"get","resource":"pods","name":"checkout","namespace":"forbidden"}`),
		answer("That namespace is outside scope."),
	}}
	o.Model = fake
	if _, err := Run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 0 {
		t.Fatal("namespace escape reached the cluster")
	}
	body := fake.inputs[1][len(fake.inputs[1])-1].Content
	if !strings.Contains(body, "namespace is outside") {
		t.Fatalf("expected scoped denial: %s", body)
	}
}

func TestAgentRecursiveKubernetesReadsUseCapturedScope(t *testing.T) {
	o, store := fixture(t)
	scope, requests := scopedKubernetesFixture(t)
	o.Kubernetes = scope
	fake := &scriptedModel{steps: []*schema.Message{
		call("investigate", `{"question":"Check the checkout Pod"}`),
		answer("v = call({\"operation\":\"get\",\"resource\":\"pods\",\"name\":\"checkout\"})\nr = ask(\"Explain the Pod state\", text(v[\"result\"][\"data\"]))\nfinish(r[\"findings\"], evidence=[v[\"evidence\"]])"),
		answer("finish(\"The observed Pod is Running.\")"),
		answer("The investigation found a running Pod."),
	}}
	o.Model = fake
	result, err := Run(context.Background(), o)
	if err != nil || result.Text != "The investigation found a running Pod." || requests.Load() != 2 {
		t.Fatalf("result=%+v err=%v requests=%d", result, err, requests.Load())
	}
	if len(fake.inputs) != 4 || !strings.Contains(fake.inputs[2][1].Content, "Running") {
		t.Fatal("recursive call did not receive selected Kubernetes evidence")
	}
	mem, err := memory.New(store, "", o.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	page, err := mem.Search(context.Background(), memory.Query{Source: "reasoning", Text: "kubernetes_read", Limit: 20})
	if err != nil || len(page.Records) != 2 {
		t.Fatalf("expected one start and one result record: %+v %v", page, err)
	}
}
