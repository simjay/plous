package monitor

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
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	return client
}
func reply(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
func apiError(w http.ResponseWriter, code int, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(metav1.Status{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"}, Status: "Failure", Code: int32(code), Reason: metav1.StatusReason(reason), Message: "server-private-value"})
}
func testPod(name, uid string) map[string]any {
	return map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": name, "namespace": "shop", "uid": uid, "resourceVersion": "10"}, "spec": map[string]any{"containers": []any{map[string]any{"name": "app"}}}, "status": map[string]any{"phase": "Running"}}
}
func objectList(kind, rv string, objects ...map[string]any) map[string]any {
	return map[string]any{"apiVersion": "v1", "kind": kind + "List", "metadata": map[string]any{"resourceVersion": rv}, "items": objects}
}

func TestDiscoveryKeepsHealthyAPIsAndCustomResources(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/apis":
			reply(w, map[string]any{"kind": "APIGroupList", "groups": []any{map[string]any{"preferredVersion": map[string]string{"groupVersion": "example.io/v1"}}, map[string]any{"preferredVersion": map[string]string{"groupVersion": "broken.io/v1"}}}})
		case "/api/v1":
			reply(w, map[string]any{"resources": []any{map[string]any{"name": "pods", "kind": "Pod", "namespaced": true, "verbs": []string{"get", "list", "watch"}, "shortNames": []string{"po"}}, map[string]any{"name": "pods/log", "kind": "Pod", "verbs": []string{"get"}}}})
		case "/apis/example.io/v1":
			reply(w, map[string]any{"resources": []any{map[string]any{"name": "widgets", "kind": "Widget", "namespaced": true, "verbs": []string{"get", "list"}}}})
		default:
			apiError(w, 503, "ServiceUnavailable")
		}
	})
	resources, warning, err := client.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 2 || warning == "" {
		t.Fatalf("resources=%#v warning=%q", resources, warning)
	}
	if _, ok := Find(resources, "widgets.example.io"); !ok {
		t.Fatal("custom resource was hidden by a failed API group")
	}
	if pods, ok := Find(resources, "po"); !ok {
		t.Fatal("discovered short name did not resolve")
	} else if !pods.Supports("get", "log") || pods.Supports("create", "exec") {
		t.Fatalf("discovery lost subresource capabilities: %#v", pods.Subresources)
	}
}

func TestListPaginatesAndUsesServerPrinterColumns(t *testing.T) {
	var requests atomic.Int32
	resource := kubecommon.Resource{Group: "example.io", Version: "v1", Name: "widgets", Kind: "Widget", Namespaced: true}
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/apis/example.io/v1/namespaces/shop/widgets" || r.URL.Query().Get("labelSelector") != "app=shop" {
			t.Errorf("unexpected request %s", r.URL.String())
		}
		if strings.Contains(r.Header.Get("Accept"), "as=Table") {
			reply(w, map[string]any{"apiVersion": "meta.k8s.io/v1", "kind": "Table", "columnDefinitions": []any{map[string]string{"name": "Name"}, map[string]string{"name": "Phase"}}, "rows": []any{map[string]any{"cells": []string{"one", "Healthy"}, "object": map[string]any{"metadata": map[string]string{"name": "one", "namespace": "shop", "uid": "a"}}}, map[string]any{"cells": []string{"two", "Pending"}, "object": map[string]any{"metadata": map[string]string{"name": "two", "namespace": "shop", "uid": "b"}}}}})
			return
		}
		page := objectList("Widget", "10", map[string]any{"metadata": map[string]string{"name": "one", "namespace": "shop", "uid": "a"}})
		if r.URL.Query().Get("continue") == "" {
			page["metadata"].(map[string]any)["continue"] = "next"
		} else {
			page = objectList("Widget", "10", map[string]any{"metadata": map[string]string{"name": "two", "namespace": "shop", "uid": "b"}})
		}
		reply(w, page)
	})
	collection, err := client.List(context.Background(), resource, "shop", "app=shop")
	if err != nil {
		t.Fatal(err)
	}
	if len(collection.Objects) != 2 || requests.Load() != 3 {
		t.Fatalf("objects=%d requests=%d", len(collection.Objects), requests.Load())
	}
	columns, rows := Table(resource, collection, time.Now())
	if len(columns) != 3 || columns[2] != "Phase" || rows[1].Cells[2] != "Pending" {
		t.Fatalf("server columns lost: %#v %#v", columns, rows)
	}
	if collection.Objects[0].GetKind() != "Widget" {
		t.Fatal("missing object kind was not restored")
	}
}

func TestObserveRelistsAfterExpiredWatch(t *testing.T) {
	var lists atomic.Int32
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("watch") == "true" {
			reply(w, map[string]any{"type": "ERROR", "object": map[string]any{"kind": "Status", "apiVersion": "v1", "code": 410, "reason": "Expired", "message": "expired"}})
			return
		}
		index := lists.Add(1)
		reply(w, objectList("Pod", fmt.Sprint(index), testPod(fmt.Sprintf("pod-%d", index), fmt.Sprint(index))))
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var names []string
	client.Observe(ctx, kubecommon.Resource{Name: "pods", Kind: "Pod", Version: "v1", Namespaced: true, Verbs: []string{"list", "watch"}}, "shop", "", func(snapshot Snapshot) {
		if snapshot.Collection != nil {
			names = append(names, snapshot.Collection.Objects[0].GetName())
			if len(names) == 2 {
				cancel()
			}
		}
	})
	if len(names) != 2 || names[0] == names[1] {
		t.Fatalf("expired watch did not re-list: %v", names)
	}
}

func TestWatchDeniedFallsBackAndStopsOnCancellation(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("watch") == "true" {
			apiError(w, 403, "Forbidden")
			return
		}
		reply(w, objectList("Pod", "1", testPod("pod", "id")))
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fallback := false
	client.Observe(ctx, kubecommon.Resource{Name: "pods", Kind: "Pod", Version: "v1", Namespaced: true, Verbs: []string{"list", "watch"}}, "shop", "", func(snapshot Snapshot) {
		if strings.Contains(snapshot.Status, "Watch denied") {
			fallback = true
			cancel()
		}
	})
	if !fallback {
		t.Fatal("watch denial did not report periodic refresh")
	}
}

func TestForbiddenListIsNotAnEmptySuccessOrLeakedError(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) { apiError(w, 403, "Forbidden") })
	collection, err := client.List(context.Background(), kubecommon.Resource{Name: "pods", Kind: "Pod", Version: "v1", Namespaced: true}, "shop", "")
	if err == nil || len(collection.Objects) != 0 {
		t.Fatal("forbidden resource appeared successful")
	}
	message := ErrorText(err)
	if !strings.Contains(message, "Access denied") || strings.Contains(message, "server-private-value") {
		t.Fatalf("unsafe error: %q", message)
	}
}

func TestLogsUseExplicitContainerAndPreviousRequest(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/namespaces/shop/pods/pod/log" || r.URL.Query().Get("container") != "init" || r.URL.Query().Get("previous") != "true" || r.URL.Query().Get("follow") != "false" {
			t.Errorf("wrong log request: %s", r.URL.String())
		}
		fmt.Fprintln(w, "time ERROR configuration missing")
	})
	var text string
	err := client.Logs(context.Background(), LogOptions{Namespace: "shop", Pod: "pod", Container: "init", Previous: true, Follow: true}, func(line string) { text += line })
	if err != nil || !strings.Contains(text, "configuration missing") {
		t.Fatalf("log stream: %q %v", text, err)
	}
}

func TestSavedKubeconfigPathListAndContextAreReadWithoutWrites(t *testing.T) {
	root := t.TempDir()
	first, second := filepath.Join(root, "contexts"), filepath.Join(root, "credentials")
	a := []byte("apiVersion: v1\nkind: Config\ncontexts:\n- name: selected\n  context: {cluster: local, user: developer, namespace: shop}\ncurrent-context: unused\n")
	b := []byte("apiVersion: v1\nkind: Config\nclusters:\n- name: local\n  cluster: {server: 'https://127.0.0.1:1'}\nusers:\n- name: developer\n  user: {token: private-token}\n")
	if err := os.WriteFile(first, a, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, b, 0600); err != nil {
		t.Fatal(err)
	}
	client, err := New(first+string(os.PathListSeparator)+second, "selected", "")
	if err != nil {
		t.Fatal(err)
	}
	if client.Namespace != "shop" {
		t.Fatalf("namespace=%q", client.Namespace)
	}
	for path, want := range map[string][]byte{first: a, second: b} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != string(want) {
			t.Fatal("kubeconfig was changed")
		}
	}
}

func TestRelationsFollowOwnerUIDRatherThanSharedLabels(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apis/apps/v1/namespaces/shop/replicasets" {
			t.Errorf("wrong related endpoint: %s", r.URL.Path)
		}
		reply(w, objectList("ReplicaSet", "1", map[string]any{"metadata": map[string]any{"name": "owned", "namespace": "shop", "uid": "child", "ownerReferences": []any{map[string]any{"uid": "deployment", "name": "checkout", "kind": "Deployment", "apiVersion": "apps/v1"}}}}, map[string]any{"metadata": map[string]any{"name": "unrelated", "namespace": "shop", "labels": map[string]string{"app": "checkout"}}}))
	})
	resource := kubecommon.Resource{Group: "apps", Version: "v1", Name: "deployments", Kind: "Deployment", Namespaced: true}
	children := kubecommon.Resource{Group: "apps", Version: "v1", Name: "replicasets", Kind: "ReplicaSet", Namespaced: true}
	object := unstructured.Unstructured{Object: map[string]any{"metadata": map[string]any{"name": "checkout", "namespace": "shop", "uid": "deployment"}}}
	refs, warning := client.Related(context.Background(), resource, object, []kubecommon.Resource{resource, children})
	if warning != "" || len(refs) != 1 || refs[0].Name != "owned" {
		t.Fatalf("incorrect ownership: %#v %s", refs, warning)
	}
}

func TestSecretYAMLIsRedactedWithoutChangingTheObject(t *testing.T) {
	object := unstructured.Unstructured{Object: map[string]any{"kind": "Secret", "metadata": map[string]any{"name": "key", "annotations": map[string]any{"kubectl.kubernetes.io/last-applied-configuration": "secret-in-annotation"}}, "data": map[string]any{"token": "secret-value"}}}
	masked := YAML(object, false)
	if strings.Contains(masked, "secret-value") || strings.Contains(masked, "secret-in-annotation") {
		t.Fatal("secret displayed without explicit reveal")
	}
	if !strings.Contains(YAML(object, true), "secret-value") {
		t.Fatal("redaction mutated the underlying object")
	}
}
