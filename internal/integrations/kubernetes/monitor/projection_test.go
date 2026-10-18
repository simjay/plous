package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func agentTestDiscovery(w http.ResponseWriter) {
	reply(w, map[string]any{"apiVersion": "v1", "kind": "APIResourceList", "resources": []any{
		map[string]any{"name": "pods", "kind": "Pod", "namespaced": true, "verbs": []string{"get", "list"}},
		map[string]any{"name": "configmaps", "kind": "ConfigMap", "namespaced": true, "verbs": []string{"get", "list"}},
	}})
}

func TestAgentReadDenialsNeverContactKubernetes(t *testing.T) {
	var requests atomic.Int32
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) { requests.Add(1) })
	executor, err := NewProjector(client, agentTestScope())
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range commandDefinitions {
		if definition.Access == AccessWrite {
			if _, err := executor.Execute(context.Background(), Command{Operation: definition.Operation, Resource: "pods"}); err == nil {
				t.Errorf("write %s allowed", definition.Operation)
			}
		}
	}
	for _, input := range []string{
		`{"operation":"get","resource":"pods","raw":"/proxy"}`,
		`{"operation":"get","resource":"pods","namespace":"other"}`,
		`{"operation":"get","resource":"pods/exec"}`,
		`{"operation":"get","resource":"secrets"}`,
		`{"operation":"exec","resource":"pods"}`,
		`{"operation":"get","resource":"pods","limit":1000}`,
	} {
		command, err := DecodeCommand(input)
		if err == nil {
			_, err = executor.Execute(context.Background(), command)
		}
		if err == nil {
			t.Errorf("denied input succeeded: %s", input)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("denied calls made %d network requests", requests.Load())
	}
}

func TestProjectionCollectsPagesAndComputesReadiness(t *testing.T) {
	var calls atomic.Int32
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1" {
			agentTestDiscovery(w)
			return
		}
		page := calls.Add(1)
		name := "ready"
		ready := "True"
		if page == 2 {
			name = "unready"
			ready = "False"
			if r.URL.Query().Get("continue") != "next" {
				t.Error("lost cursor")
			}
		}
		pod := testPod(name, name)
		pod["status"].(map[string]any)["conditions"] = []any{map[string]any{"type": "Ready", "status": ready}}
		pod["metadata"].(map[string]any)["managedFields"] = strings.Repeat("discarded blob", 5000)
		result := objectList("Pod", "10", pod)
		if page == 1 {
			result["metadata"].(map[string]any)["continue"] = "next"
		}
		reply(w, result)
	})
	executor, err := NewProjector(client, agentTestScope())
	if err != nil {
		t.Fatal(err)
	}
	result, err := executor.Execute(t.Context(), Command{Operation: "get", Resource: "pods"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(result)
	if !result.Coverage.Complete || result.Coverage.Observed != 2 || calls.Load() != 2 || strings.Contains(string(data), "discarded blob") {
		t.Fatal(string(data), calls.Load())
	}
	if result.Facts["ready"].String() != "1" || result.Facts["unready"].String() != "1" {
		t.Fatal(result.Facts)
	}
}

func TestProjectionRejectsRepeatedContinuation(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1" {
			agentTestDiscovery(w)
			return
		}
		page := objectList("Pod", "10", testPod("web", "one"))
		page["metadata"].(map[string]any)["continue"] = "same"
		reply(w, page)
	})
	executor, _ := NewProjector(client, agentTestScope())
	r, err := executor.Execute(t.Context(), Command{Operation: "get", Resource: "pods"})
	if err == nil && r.Coverage.Complete {
		t.Fatal("repeated continuation claimed completeness", r)
	}
}

func TestAgentReadRejectsOversizedResponse(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1" {
			agentTestDiscovery(w)
			return
		}
		reply(w, map[string]any{"details": strings.Repeat("x", maxAgentResponseBytes+1)})
	})
	executor, _ := NewProjector(client, agentTestScope())
	if _, err := executor.Execute(context.Background(), Command{Operation: "get", Resource: "pods", Name: "web"}); err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("oversized response accepted: %v", err)
	}
}

func TestAgentLogsBoundAndRedact(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/namespaces/shop/pods/web/log" || r.URL.Query().Get("follow") != "false" || r.URL.Query().Get("tailLines") != "2" || r.URL.Query().Get("limitBytes") != "100" || r.URL.Query().Get("container") != "app" {
			t.Errorf("unbounded logs: %s", r.URL)
		}
		fmt.Fprint(w, "token=hidden-value\nAuthorization: Bearer hidden-other\nthird line\n"+strings.Repeat("x", 200))
	})
	executor, _ := NewProjector(client, agentTestScope())
	result, err := executor.Execute(context.Background(), Command{Operation: "logs", Resource: "pods", Name: "web", Container: "app", TailLines: 2, MaxBytes: 100})
	if err != nil {
		t.Fatal(err)
	}
	body := result.Facts["excerpt"].String()
	if strings.Contains(body, "hidden-") || strings.Contains(body, "third line") || result.Coverage.Complete {
		t.Fatalf("unsafe log result: %+v", result)
	}
}

func TestAgentLogsCancellation(t *testing.T) {
	started := make(chan struct{})
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	})
	executor, _ := NewProjector(client, agentTestScope())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := executor.Execute(ctx, Command{Operation: "logs", Resource: "pods", Name: "web", Container: "app"})
		finished <- err
	}()
	<-started
	cancel()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("cancelled read succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not terminate logs")
	}
}

func TestAgentReadSanitizesConfigAndNestedValues(t *testing.T) {
	executor := &Projector{credentials: []string{"known-provider-key"}}
	value := map[string]any{
		"items":  []any{map[string]any{"kind": "Secret", "stringData": map[string]any{"plain": "private"}}},
		"data":   map[string]any{"opaque": "private"},
		"status": map[string]any{"access_token": "private", "password": "private"},
		"logs":   "known-provider-key Bearer private password=private https://user:private@example.org\n-----BEGIN RSA PRIVATE KEY-----\nprivate\n-----END RSA PRIVATE KEY-----",
	}
	result, _ := json.Marshal(executor.sanitize(value))
	if strings.Contains(string(result), `"private"`) || strings.Contains(string(result), "=private") || strings.Contains(string(result), "user:private") || strings.Contains(string(result), "known-provider-key") {
		t.Fatalf("credentials survived redaction: %s", result)
	}
}

func TestAgentLogRedactionCoversJSONKeysAndAuthorization(t *testing.T) {
	executor := &Projector{}
	for _, input := range []string{
		`{"token":"hidden-credential","message":"healthy"}`,
		`{"api_key": "hidden-credential"}`,
		`{"password": "hidden-credential"}`,
		`Authorization: Basic hidden-credential`,
		`{"authorization":"Basic hidden-credential"}`,
		`{"authorization":"Bearer hidden-credential"}`,
		`credential='hidden-credential'`,
	} {
		redacted := executor.sanitizeText(input)
		if strings.Contains(redacted, "hidden-credential") {
			t.Errorf("credential survived log sanitization: %s", redacted)
		}
		if strings.HasPrefix(input, "{") && !json.Valid([]byte(redacted)) {
			t.Errorf("sanitizing JSON logs broke their structure: %s", redacted)
		}
	}
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signature"
	if strings.Contains(executor.sanitizeText("received "+jwt), jwt) {
		t.Fatal("unlabeled JWT survived log sanitization")
	}
}

func TestAgentReadsNeverFollowRedirects(t *testing.T) {
	var escaped atomic.Int32
	outside := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		escaped.Add(1)
		reply(w, map[string]any{"kind": "Pod"})
	}))
	defer outside.Close()
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1" {
			agentTestDiscovery(w)
			return
		}
		http.Redirect(w, r, outside.URL+"/unscoped", http.StatusTemporaryRedirect)
	})
	executor, err := NewProjector(client, agentTestScope())
	if err != nil {
		t.Fatal(err)
	}
	_, err = executor.Execute(context.Background(), Command{Operation: "get", Resource: "pods", Name: "web"})
	if err == nil || escaped.Load() != 0 {
		t.Fatalf("redirect escaped the executor: err=%v requests=%d", err, escaped.Load())
	}
}

func TestAgentReadsValidateGroupDiscoveryBeforeResourceRequest(t *testing.T) {
	for _, groupVersion := range []string{"apps/v1", "other/v1", "apps/../../proxy"} {
		t.Run(groupVersion, func(t *testing.T) {
			var objectRequests atomic.Int32
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/apis/apps":
					reply(w, map[string]any{"kind": "APIGroup", "preferredVersion": map[string]any{"groupVersion": groupVersion}})
				case "/apis/apps/v1":
					reply(w, map[string]any{"kind": "APIResourceList", "resources": []any{map[string]any{"name": "deployments", "kind": "Deployment", "namespaced": true, "verbs": []string{"get"}}}})
				case "/apis/apps/v1/namespaces/shop/deployments/web":
					objectRequests.Add(1)
					reply(w, map[string]any{"kind": "Deployment", "metadata": map[string]string{"name": "web", "namespace": "shop"}})
				default:
					t.Errorf("discovery escaped approved API identity: %s", r.URL)
					http.Error(w, "invalid", http.StatusBadRequest)
				}
			})
			executor, err := NewProjector(client, agentTestScope())
			if err != nil {
				t.Fatal(err)
			}
			result, err := executor.Execute(context.Background(), Command{Operation: "describe", Resource: "deployments.apps", Name: "web"})
			if groupVersion == "apps/v1" {
				if err != nil || objectRequests.Load() != 1 || len(result.Rows) == 0 {
					t.Fatalf("valid grouped read failed: %+v %v", result, err)
				}
			} else if err == nil || objectRequests.Load() != 0 {
				t.Fatalf("untrusted discovery was used: %+v %v calls=%d", result, err, objectRequests.Load())
			}
		})
	}
}

func TestAgentReadCancellationDuringDiscoveryOrObject(t *testing.T) {
	for _, phase := range []string{"discovery", "object"} {
		t.Run(phase, func(t *testing.T) {
			started := make(chan struct{})
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if phase == "object" && r.URL.Path == "/api/v1" {
					agentTestDiscovery(w)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, `{`)
				w.(http.Flusher).Flush()
				close(started)
				<-r.Context().Done()
			})
			executor, err := NewProjector(client, agentTestScope())
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			finished := make(chan error, 1)
			go func() {
				_, err := executor.Execute(ctx, Command{Operation: "get", Resource: "pods", Name: "web"})
				finished <- err
			}()
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("read did not start")
			}
			cancel()
			select {
			case err := <-finished:
				if err == nil {
					t.Fatal("cancelled read returned success")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("read ignored cancellation")
			}
		})
	}
}

func TestAgentDiscoveryAndLogsRejectRedirects(t *testing.T) {
	for _, operation := range []string{"get", "logs"} {
		t.Run(operation, func(t *testing.T) {
			var escaped atomic.Int32
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/unscoped" {
					escaped.Add(1)
					fmt.Fprint(w, "unscoped content")
					return
				}
				http.Redirect(w, r, "/unscoped", http.StatusFound)
			})
			executor, err := NewProjector(client, agentTestScope())
			if err != nil {
				t.Fatal(err)
			}
			command := Command{Operation: operation, Resource: "pods", Name: "web"}
			if operation == "logs" {
				command.Container = "app"
			}
			if _, err := executor.Execute(context.Background(), command); err == nil || escaped.Load() != 0 {
				t.Fatalf("redirect escaped scope: %v requests=%d", err, escaped.Load())
			}
		})
	}
}

func TestAgentReadsDoNotRetryRateLimitedRequests(t *testing.T) {
	for _, phase := range []string{"discovery", "object", "logs"} {
		t.Run(phase, func(t *testing.T) {
			var failedRequests atomic.Int32
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if phase == "object" && r.URL.Path == "/api/v1" {
					agentTestDiscovery(w)
					return
				}
				failedRequests.Add(1)
				w.Header().Set("Retry-After", "0")
				apiError(w, http.StatusTooManyRequests, "TooManyRequests")
			})
			executor, err := NewProjector(client, agentTestScope())
			if err != nil {
				t.Fatal(err)
			}
			command := Command{Operation: "get", Resource: "pods", Name: "web"}
			if phase == "logs" {
				command.Operation, command.Container = "logs", "app"
			}
			result, err := executor.Execute(context.Background(), command)
			if err != nil || result.Outcome != "error" || failedRequests.Load() != 1 {
				t.Fatalf("rate limited request retried: err=%v requests=%d", err, failedRequests.Load())
			}
			body, _ := json.Marshal(result)
			if strings.Contains(string(body), "server-private-value") {
				t.Fatal("API error body escaped sanitization")
			}
		})
	}
}

func TestAgentReadScopeDistinguishesClusterAndNamespacedResources(t *testing.T) {
	var objectRequests atomic.Int32
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1" {
			reply(w, map[string]any{"resources": []any{
				map[string]any{"name": "nodes", "kind": "Node", "namespaced": false, "verbs": []string{"get"}},
				map[string]any{"name": "pods", "kind": "Pod", "namespaced": true, "verbs": []string{"get"}},
			}})
			return
		}
		objectRequests.Add(1)
		switch r.URL.Path {
		case "/api/v1/nodes/worker":
			reply(w, map[string]any{"kind": "Node", "metadata": map[string]string{"name": "worker"}})
		case "/api/v1/namespaces/shop/pods/web":
			reply(w, testPod("web", "one"))
		default:
			t.Errorf("read escaped discovered resource scope: %s", r.URL)
			http.Error(w, "unexpected scope", http.StatusForbidden)
		}
	})
	executor, err := NewProjector(client, agentTestScope())
	if err != nil {
		t.Fatal(err)
	}
	node, err := executor.Execute(context.Background(), Command{Operation: "get", Resource: "nodes", Name: "worker"})
	if err != nil || len(node.Rows) != 1 || node.Rows[0].Identity.Namespace != "" {
		t.Fatalf("cluster-scoped read recorded a namespace request: %+v %v", node, err)
	}
	pod, err := executor.Execute(context.Background(), Command{Operation: "get", Resource: "pods", Name: "web"})
	if err != nil || pod.Namespace != "shop" {
		t.Fatalf("namespaced read lost captured namespace: %+v %v", pod, err)
	}
	if _, err := executor.Execute(context.Background(), Command{Operation: "get", Resource: "pods", Name: "web", Namespace: "other"}); err == nil {
		t.Fatal("reading a cluster resource enabled a later namespace escape")
	}
	if objectRequests.Load() != 2 {
		t.Fatalf("unexpected resource requests: %d", objectRequests.Load())
	}
}

func TestProjectionDoesNotInferMissingReadinessOrWrongKind(t *testing.T) {
	for _, kind := range []string{"Pod", "Deployment"} {
		t.Run(kind, func(t *testing.T) {
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1" {
					agentTestDiscovery(w)
					return
				}
				pod := testPod("unknown", "unknown")
				pod["kind"] = kind
				delete(pod["status"].(map[string]any), "conditions")
				reply(w, objectList("Pod", "10", pod))
			})
			p, err := NewProjector(client, agentTestScope())
			if err != nil {
				t.Fatal(err)
			}
			r, err := p.Execute(t.Context(), Command{Operation: "get", Resource: "pods"})
			if kind != "Pod" {
				if err == nil {
					t.Fatal("accepted object from another kind")
				}
				return
			}
			if err != nil || !r.Coverage.Complete || !r.Facts["unready"].Unknown || r.Facts["readiness_unknown"].String() != "1" {
				t.Fatal(r, err)
			}
		})
	}
}

func TestProjectionRejectsConflictingDuplicateIdentity(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1" {
			agentTestDiscovery(w)
			return
		}
		first, second := testPod("web", "same-uid"), testPod("web", "same-uid")
		second["status"].(map[string]any)["phase"] = "Failed"
		reply(w, objectList("Pod", "10", first, second))
	})
	projector, err := NewProjector(client, agentTestScope())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = projector.Execute(t.Context(), Command{Operation: "get", Resource: "pods"}); err == nil || !strings.Contains(err.Error(), "conflicting") {
		t.Fatal("silently discarded conflicting evidence", err)
	}
}
func TestClusterResourceProjectionDoesNotClaimNamespace(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1" {
			reply(w, map[string]any{"resources": []any{map[string]any{"name": "nodes", "kind": "Node", "namespaced": false, "verbs": []string{"list"}}}})
			return
		}
		if r.URL.Path != "/api/v1/nodes" {
			t.Error(r.URL.Path)
		}
		reply(w, objectList("Node", "10", map[string]any{"metadata": map[string]any{"name": "worker"}}))
	})
	projector, err := NewProjector(client, agentTestScope())
	if err != nil {
		t.Fatal(err)
	}
	got, err := projector.Execute(t.Context(), Command{Operation: "get", Resource: "nodes"})
	if err != nil || got.Namespace != "" || got.Selection["resource"] != "nodes" {
		t.Fatal(got, err)
	}
}
