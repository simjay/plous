package kubernetes

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
	executor, err := NewReadExecutor(client, agentTestScope())
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
		if _, err := executor.ExecuteJSON(context.Background(), input); err == nil {
			t.Errorf("denied input succeeded: %s", input)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("denied calls made %d network requests", requests.Load())
	}
}

func TestAgentReadsCaptureScopePaginateAndRedact(t *testing.T) {
	var requests atomic.Int32
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet {
			t.Errorf("non-read HTTP method %s", r.Method)
		}
		if r.URL.Path == "/api/v1" {
			agentTestDiscovery(w)
			return
		}
		if r.URL.Path != "/api/v1/namespaces/shop/pods" || r.URL.Query().Get("limit") != "1" || r.URL.Query().Get("labelSelector") != "app=web" || r.URL.Query().Get("continue") != "next-page" {
			t.Errorf("scope or pagination mismatch: %s", r.URL)
		}
		pod := testPod("web", "one")
		pod["metadata"].(map[string]any)["annotations"] = map[string]any{"last-applied": "private annotation"}
		pod["nested"] = []any{map[string]any{"kind": "Secret", "data": map[string]any{"value": "private secret"}}, map[string]any{"credentials": "private credential"}}
		pod["spec"].(map[string]any)["containers"].([]any)[0].(map[string]any)["env"] = []any{map[string]any{"name": "PASSWORD", "value": "private env"}}
		page := objectList("Pod", "10", pod)
		page["metadata"].(map[string]any)["continue"] = "last-page"
		reply(w, page)
	})
	scope := agentTestScope()
	executor, err := NewReadExecutor(client, scope)
	if err != nil {
		t.Fatal(err)
	}
	scope.Namespace = "other"
	client.Namespace = "other"
	result, err := executor.Execute(context.Background(), Command{Operation: "get", Resource: "pods", Limit: 1, LabelSelector: "app=web", Continue: "next-page"})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(result)
	if strings.Contains(string(body), "private ") {
		t.Fatalf("sensitive nested data survived: %s", body)
	}
	if !result.Truncated || result.Continue != "last-page" || result.Source.Namespace != "shop" || result.Command.Namespace != "shop" || result.FetchedAt.IsZero() || requests.Load() != 2 {
		t.Fatalf("incorrect result metadata: %+v requests=%d", result, requests.Load())
	}
}

func TestAgentReadResultLimits(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1" {
			agentTestDiscovery(w)
			return
		}
		pod := testPod("web", "one")
		pod["details"] = strings.Repeat("x", 600)
		page := objectList("Pod", "10", pod, pod, pod)
		page["metadata"].(map[string]any)["continue"] = "must-not-skip-truncated-items"
		reply(w, page)
	})
	executor, _ := NewReadExecutor(client, agentTestScope())
	result, err := executor.Execute(context.Background(), Command{Operation: "get", Resource: "pods", Limit: 1, MaxBytes: 300})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result.Data)
	if encodedSize(result.Data) > 300 || !result.Truncated || result.Notice == "" || result.Continue != "" || strings.Contains(string(encoded), "must-not-skip") {
		t.Fatalf("unbounded or incorrectly paged output: %+v", result)
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
	executor, _ := NewReadExecutor(client, agentTestScope())
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
	executor, _ := NewReadExecutor(client, agentTestScope())
	result, err := executor.Execute(context.Background(), Command{Operation: "logs", Resource: "pods", Name: "web", Container: "app", TailLines: 2, MaxBytes: 100})
	if err != nil {
		t.Fatal(err)
	}
	body := result.Data.(string)
	if strings.Contains(body, "hidden-") || strings.Contains(body, "third line") || !result.Truncated {
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
	executor, _ := NewReadExecutor(client, agentTestScope())
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
	executor := &ReadExecutor{credentials: []string{"known-provider-key"}}
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
	executor := &ReadExecutor{}
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
	executor, err := NewReadExecutor(client, agentTestScope())
	if err != nil {
		t.Fatal(err)
	}
	_, err = executor.Execute(context.Background(), Command{Operation: "get", Resource: "pods", Name: "web"})
	if err == nil || escaped.Load() != 0 {
		t.Fatalf("redirect escaped the executor: err=%v requests=%d", err, escaped.Load())
	}
}

func TestAgentReadsPageWithoutSkippingOrPrefetching(t *testing.T) {
	var listRequests atomic.Int32
	cursor := "next+/page=token"
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1" {
			agentTestDiscovery(w)
			return
		}
		if r.URL.Path != "/api/v1/namespaces/shop/pods" || r.URL.Query().Get("limit") != "1" {
			t.Errorf("unexpected bounded list request: %s", r.URL)
		}
		page := listRequests.Add(1)
		name := "first"
		if page == 2 {
			name = "second"
			if r.URL.Query().Get("continue") != cursor {
				t.Errorf("continuation token changed: %s", r.URL)
			}
		} else if page != 1 || r.URL.Query().Get("continue") != "" {
			t.Errorf("unexpected page: %d %s", page, r.URL)
		}
		result := objectList("Pod", "10", testPod(name, name))
		if page == 1 {
			result["metadata"].(map[string]any)["continue"] = cursor
		}
		reply(w, result)
	})
	executor, err := NewReadExecutor(client, agentTestScope())
	if err != nil {
		t.Fatal(err)
	}
	first, err := executor.Execute(context.Background(), Command{Operation: "get", Resource: "pods", Limit: 1})
	if err != nil || !first.Truncated || first.Continue != cursor || listRequests.Load() != 1 {
		t.Fatalf("first page prefetched or lost continuation: %+v %v calls=%d", first, err, listRequests.Load())
	}
	second, err := executor.Execute(context.Background(), Command{Operation: "get", Resource: "pods", Limit: 1, Continue: first.Continue})
	if err != nil || second.Truncated || second.Continue != "" || listRequests.Load() != 2 {
		t.Fatalf("second page failed: %+v %v calls=%d", second, err, listRequests.Load())
	}
	for i, result := range []ReadResult{first, second} {
		items := result.Data.(map[string]any)["items"].([]any)
		if len(items) != 1 || items[0].(map[string]any)["metadata"].(map[string]any)["name"] != []string{"first", "second"}[i] {
			t.Fatalf("page contents changed: %+v", result)
		}
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
			executor, err := NewReadExecutor(client, agentTestScope())
			if err != nil {
				t.Fatal(err)
			}
			result, err := executor.Execute(context.Background(), Command{Operation: "describe", Resource: "deployments.apps", Name: "web"})
			if groupVersion == "apps/v1" {
				if err != nil || objectRequests.Load() != 1 || result.Data == nil {
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
			executor, err := NewReadExecutor(client, agentTestScope())
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
			executor, err := NewReadExecutor(client, agentTestScope())
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
			executor, err := NewReadExecutor(client, agentTestScope())
			if err != nil {
				t.Fatal(err)
			}
			command := Command{Operation: "get", Resource: "pods", Name: "web"}
			if phase == "logs" {
				command.Operation, command.Container = "logs", "app"
			}
			_, err = executor.Execute(context.Background(), command)
			if err == nil || failedRequests.Load() != 1 {
				t.Fatalf("rate limited request retried: err=%v requests=%d", err, failedRequests.Load())
			}
			if strings.Contains(err.Error(), "server-private-value") {
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
	executor, err := NewReadExecutor(client, agentTestScope())
	if err != nil {
		t.Fatal(err)
	}
	node, err := executor.Execute(context.Background(), Command{Operation: "get", Resource: "nodes", Name: "worker"})
	if err != nil || node.Command.Namespace != "" || node.Source.Namespace != "shop" {
		t.Fatalf("cluster-scoped read recorded a namespace request: %+v %v", node, err)
	}
	pod, err := executor.Execute(context.Background(), Command{Operation: "get", Resource: "pods", Name: "web"})
	if err != nil || pod.Command.Namespace != "shop" {
		t.Fatalf("namespaced read lost captured namespace: %+v %v", pod, err)
	}
	if _, err := executor.Execute(context.Background(), Command{Operation: "get", Resource: "pods", Name: "web", Namespace: "other"}); err == nil {
		t.Fatal("reading a cluster resource enabled a later namespace escape")
	}
	if objectRequests.Load() != 2 {
		t.Fatalf("unexpected resource requests: %d", objectRequests.Load())
	}
}
