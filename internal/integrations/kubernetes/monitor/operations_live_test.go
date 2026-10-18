package monitor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

type liveSizeQueue struct {
	ctx   context.Context
	sizes chan remotecommand.TerminalSize
}

func (q liveSizeQueue) Next() *remotecommand.TerminalSize {
	select {
	case size := <-q.sizes:
		return &size
	case <-q.ctx.Done():
		return nil
	}
}

// This test only accepts an explicitly supplied disposable kind configuration.
func TestDisposableClusterOperations(t *testing.T) {
	path := os.Getenv("PLOUS_TEST_KUBECONFIG")
	if path == "" {
		t.Skip("PLOUS_TEST_KUBECONFIG is required for disposable-cluster verification")
	}
	absolute, err := filepath.Abs(path)
	if err != nil || !strings.HasPrefix(absolute, "/tmp/plous-kind-verification-") {
		t.Fatal("live verification requires an explicit /tmp/plous-kind-verification-*/ kubeconfig")
	}
	config, err := clientcmd.LoadFromFile(absolute)
	if err != nil || !strings.HasPrefix(config.CurrentContext, "kind-plous-test-") {
		t.Fatal("live verification requires a disposable kind-plous-test-* context")
	}
	verifyClusterOperations(t, absolute, config.CurrentContext)
}

// Minikube verification is opt-in and confines writes to a newly created namespace.
func TestPreparedMinikubeOperations(t *testing.T) {
	path := os.Getenv("PLOUS_MINIKUBE_TEST_KUBECONFIG")
	if path == "" {
		t.Skip("PLOUS_MINIKUBE_TEST_KUBECONFIG is required for explicit Minikube acceptance")
	}
	config, err := clientcmd.LoadFromFile(path)
	if err != nil || config.CurrentContext != "minikube" {
		t.Fatal("acceptance requires an explicit minikube kubeconfig")
	}
	verifyClusterOperations(t, path, "minikube")
}

func verifyClusterOperations(t *testing.T, path, contextName string) {
	t.Helper()
	client, err := New(path, contextName, "default")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	resources, warning, err := client.Discover(ctx)
	if err != nil || warning != "" {
		t.Fatalf("discovery: %s %v", warning, err)
	}
	namespace := fmt.Sprintf("plous-operations-%d", time.Now().UnixNano())
	nsAPI := client.dynamic.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"})
	createdNamespace, err := nsAPI.Create(ctx, &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": namespace, "labels": map[string]any{"app.kubernetes.io/managed-by": "plous-acceptance"}}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if err := nsAPI.Delete(cleanup, namespace, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: ptrUID(createdNamespace.GetUID())}}); err != nil {
			t.Errorf("remove disposable namespace: %v", err)
		}
	})
	cm, found := Find(resources, "configmaps")
	if !found {
		t.Fatal("ConfigMaps missing")
	}
	original, err := client.resource(cm, namespace).Create(ctx, &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "settings", "namespace": namespace}, "data": map[string]any{"message": "before"},
	}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	access, err := client.CheckAccess(ctx, cm, namespace, original.GetName(), "update", "")
	if err != nil || !access.Known || !access.Allowed {
		t.Fatalf("permission: %#v %v", access, err)
	}
	proposed := original.DeepCopy()
	_ = unstructured.SetNestedField(proposed.Object, "after", "data", "message")
	parsed, err := ParseManifest(cm, original, YAML(*proposed, true))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.ValidateEdit(ctx, cm, original, parsed); err != nil {
		t.Fatalf("validate: %v", err)
	}
	unchanged, err := client.Get(ctx, cm, namespace, original.GetName())
	if err != nil || Value(unchanged.Object, "data", "message") != "before" {
		t.Fatal("dry-run persisted the edit")
	}
	applied, err := client.ApplyEdit(ctx, cm, original, parsed)
	if err != nil || Value(applied.Object, "data", "message") != "after" {
		t.Fatalf("apply: %v", err)
	}
	if _, err = client.ApplyEdit(ctx, cm, original, parsed); !apierrors.IsConflict(err) {
		t.Fatalf("stale edit must conflict: %v", err)
	}
	t.Log("Edit dry-run, apply, conflict protection, and authorization verified")
	deployment, found := Find(resources, "deployments.apps")
	if !found {
		t.Fatal("Deployments missing")
	}
	workload, err := client.resource(deployment, namespace).Create(ctx, &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]any{"name": "echo", "namespace": namespace},
		"spec": map[string]any{"replicas": int64(1), "selector": map[string]any{"matchLabels": map[string]any{"app": "echo"}}, "template": map[string]any{
			"metadata": map[string]any{"labels": map[string]any{"app": "echo"}},
			"spec":     map[string]any{"terminationGracePeriodSeconds": int64(1), "containers": []any{map[string]any{"name": "echo", "image": "docker.io/library/busybox:1.37.0", "imagePullPolicy": "IfNotPresent", "resources": map[string]any{"requests": map[string]any{"cpu": "10m", "memory": "16Mi"}, "limits": map[string]any{"cpu": "100m", "memory": "32Mi"}}, "command": []any{"/bin/sh", "-c", "echo plous-live-log; while true; do sleep 1; done"}}}},
		}},
	}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pods, _ := Find(resources, "pods")
	var running *unstructured.Unstructured
	for ctx.Err() == nil {
		collection, listErr := client.List(ctx, pods, namespace, "app=echo")
		if listErr != nil {
			t.Fatal(listErr)
		}
		for i := range collection.Objects {
			if runningContainer(collection.Objects[i], "echo") {
				running = collection.Objects[i].DeepCopy()
				break
			}
		}
		if running != nil {
			break
		}
		if !pause(ctx, time.Second) {
			break
		}
	}
	if running == nil {
		t.Fatal("disposable Pod did not start")
	}
	var logs strings.Builder
	since, tail, timestamps := int64(300), int64(20), false
	if err := client.Logs(ctx, LogOptions{Namespace: namespace, Pod: running.GetName(), Container: "echo", SinceSeconds: &since, TailLines: &tail, Timestamps: &timestamps}, func(line string) { logs.WriteString(line) }); err != nil || !strings.Contains(logs.String(), "plous-live-log") {
		t.Fatalf("logs: %q %v", logs.String(), err)
	}
	execCtx, stopExec := context.WithTimeout(ctx, 15*time.Second)
	defer stopExec()
	queue := liveSizeQueue{ctx: execCtx, sizes: make(chan remotecommand.TerminalSize, 2)}
	queue.sizes <- remotecommand.TerminalSize{Width: 80, Height: 24}
	go func() {
		if pause(execCtx, 1500*time.Millisecond) {
			queue.sizes <- remotecommand.TerminalSize{Width: 120, Height: 36}
		}
	}()
	var output bytes.Buffer
	input, writeInput := io.Pipe()
	defer input.Close()
	defer writeInput.Close()
	go func() {
		if pause(execCtx, 250*time.Millisecond) {
			_, _ = io.WriteString(writeInput, "sleep 1; stty size; sleep 1; stty size; printf 'plous-exec-ok\\n'; exit\n")
		}
	}()
	err = client.Exec(execCtx, ExecOptions{
		Namespace: namespace, Pod: running.GetName(), UID: running.GetUID(), Container: "echo",
		Command: []string{"/bin/sh"}, Stdin: input, Stdout: &output,
		TTY: true, SizeQueue: queue,
	})
	stopExec()
	_ = writeInput.Close()
	if err != nil || !strings.Contains(output.String(), "24 80") || !strings.Contains(output.String(), "36 120") || !strings.Contains(output.String(), "plous-exec-ok") {
		t.Fatalf("interactive exec and resize: %q %v", output.String(), err)
	}
	t.Log("Pod logs and interactive Exec input/output with two terminal sizes verified")
	interruptCtx, stopInterrupt := context.WithTimeout(ctx, 10*time.Second)
	defer stopInterrupt()
	interruptInput, sendInterrupt := io.Pipe()
	defer interruptInput.Close()
	defer sendInterrupt.Close()
	go func() {
		if !pause(interruptCtx, 250*time.Millisecond) {
			return
		}
		_, _ = io.WriteString(sendInterrupt, "sleep 30\n")
		if pause(interruptCtx, 500*time.Millisecond) {
			_, _ = io.WriteString(sendInterrupt, "\x03printf 'plous-interrupt-ok\\n'; exit\n")
		}
	}()
	var interrupted bytes.Buffer
	err = client.Exec(interruptCtx, ExecOptions{Namespace: namespace, Pod: running.GetName(), UID: running.GetUID(), Container: "echo", Command: []string{"/bin/sh"}, Stdin: interruptInput, Stdout: &interrupted, TTY: true})
	stopInterrupt()
	_ = sendInterrupt.Close()
	if err != nil || !strings.Contains(interrupted.String(), "\r\nplous-interrupt-ok\r\n") {
		t.Fatalf("remote interrupt: %q %v", interrupted.String(), err)
	}
	t.Log("Explicit terminal interrupt returns control to the remote shell")
	workload, err = client.Get(ctx, deployment, namespace, workload.GetName())
	if err != nil {
		t.Fatal(err)
	}
	scale, err := client.GetScale(ctx, deployment, workload)
	if err != nil {
		t.Fatal(err)
	}
	scaled, err := client.Scale(ctx, deployment, workload, scale, 2)
	if err != nil {
		t.Fatalf("scale: %v", err)
	}
	waitLiveProgress(t, ctx, client, NewOperationTarget(deployment, "scale", workload, scaled, 2), "complete")
	workload, err = client.Get(ctx, deployment, namespace, workload.GetName())
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := client.Restart(ctx, deployment, workload)
	if err != nil {
		t.Fatalf("restart rollout: %v", err)
	}
	waitLiveProgress(t, ctx, client, NewOperationTarget(deployment, "restart", workload, restarted, 0), "complete")

	if err := client.Delete(ctx, cm, applied); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := client.Get(ctx, cm, namespace, applied.GetName()); !apierrors.IsNotFound(err) {
		t.Fatalf("delete did not remove exact resource: %v", err)
	}
	waitLiveProgress(t, ctx, client, NewOperationTarget(cm, "delete", applied, nil, 0), "complete")
	t.Log("Scale convergence, restart completion, and graceful UID-precondition deletion verified")
	verifyHeldDeletion(t, ctx, client, cm, namespace)
	verifyFailedRollout(t, ctx, client, deployment, namespace)
	if contextName == "minikube" {
		verifyPreparedResources(t, ctx, client, resources)
	}
}

func ptrUID(uid types.UID) *types.UID { return &uid }

func waitLiveProgress(t *testing.T, ctx context.Context, client *Client, target OperationTarget, state string) OperationProgress {
	t.Helper()
	deadline, stop := context.WithTimeout(ctx, 90*time.Second)
	defer stop()
	var p OperationProgress
	for deadline.Err() == nil {
		o, err := client.Get(deadline, target.Resource, target.Object.GetNamespace(), target.Object.GetName())
		p = target.Progress(o, err)
		if p.State == state {
			return p
		}
		if !p.Watching() {
			t.Fatalf("unexpected operation outcome: %+v", p)
		}
		if !pause(deadline, time.Second) {
			break
		}
	}
	t.Fatalf("operation did not reach %s: %+v", state, p)
	return p
}

func verifyHeldDeletion(t *testing.T, ctx context.Context, client *Client, r kubecommon.Resource, ns string) {
	t.Helper()
	api := client.resource(r, ns)
	o, err := api.Create(ctx, &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "held-delete", "namespace": ns, "finalizers": []any{"acceptance.plous.dev/hold"}}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// Remove only the finalizer installed by this test, even if an assertion fails.
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		current, getErr := client.Get(cleanup, r, ns, o.GetName())
		if apierrors.IsNotFound(getErr) {
			return
		}
		if getErr != nil || current.GetUID() != o.GetUID() {
			t.Errorf("test finalizer cleanup target unavailable: %v", getErr)
			return
		}
		current.SetFinalizers(slices.DeleteFunc(current.GetFinalizers(), func(value string) bool { return value == "acceptance.plous.dev/hold" }))
		if _, err := api.Update(cleanup, current, metav1.UpdateOptions{}); err != nil {
			t.Errorf("remove test finalizer: %v", err)
		}
	}()
	if err := client.Delete(ctx, r, o); err != nil {
		t.Fatal(err)
	}
	current, err := client.Get(ctx, r, ns, o.GetName())
	p := NewOperationTarget(r, "delete", o, nil, 0).Progress(current, err)
	if !p.Watching() || !strings.Contains(p.Details, "acceptance.plous.dev/hold") {
		t.Fatalf("finalizer progress: %+v", p)
	}
	t.Log("Finalizer-delayed deletion reported without removing the finalizer")
}

func verifyFailedRollout(t *testing.T, ctx context.Context, client *Client, r kubecommon.Resource, ns string) {
	t.Helper()
	api := client.resource(r, ns)
	o, err := client.Get(ctx, r, ns, "echo")
	if err != nil {
		t.Fatal(err)
	}
	proposed := o.DeepCopy()
	_ = unstructured.SetNestedField(proposed.Object, int64(15), "spec", "progressDeadlineSeconds")
	containers, _, _ := unstructured.NestedSlice(proposed.Object, "spec", "template", "spec", "containers")
	container := containers[0].(map[string]any)
	container["image"] = "plous-acceptance-missing:never"
	container["imagePullPolicy"] = "Never"
	_ = unstructured.SetNestedSlice(proposed.Object, containers, "spec", "template", "spec", "containers")
	broken, err := client.ApplyEdit(ctx, r, o, proposed)
	if err != nil {
		t.Fatal(err)
	}
	p := waitLiveProgress(t, ctx, client, NewOperationTarget(r, "restart", broken, broken, 0), "attention")
	if !strings.Contains(p.Details, "ProgressDeadlineExceeded") {
		t.Fatalf("failed rollout: %+v", p)
	}
	current, err := client.Get(ctx, r, ns, "echo")
	if err != nil {
		t.Fatal(err)
	}
	current.Object["spec"] = o.Object["spec"]
	restored, err := api.Update(ctx, current, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	waitLiveProgress(t, ctx, client, NewOperationTarget(r, "restart", restored, restored, 0), "complete")
	t.Log("Failed rollout reason and recovery verified in the isolated namespace")
}

func verifyPreparedResources(t *testing.T, ctx context.Context, client *Client, resources []kubecommon.Resource) {
	t.Helper()
	for _, key := range []string{"pods", "deployments.apps", "statefulsets.apps", "daemonsets.apps", "jobs.batch", "cronjobs.batch", "services", "ingresses.networking.k8s.io", "persistentvolumeclaims", "configmaps", "secrets", "serviceaccounts", "roles.rbac.authorization.k8s.io", "rolebindings.rbac.authorization.k8s.io", "certificates.cert-manager.io"} {
		r, found := Find(resources, key)
		if !found {
			t.Errorf("prepared API missing: %s", key)
			continue
		}
		list, err := client.List(ctx, r, "plous-demo", "")
		if err != nil {
			t.Errorf("prepared list %s: %v", key, err)
			continue
		}
		if len(list.Objects) == 0 {
			t.Errorf("prepared resources missing: %s", key)
			continue
		}
		for _, o := range list.Objects {
			if len(Summary(r, o)) == 0 || summaryText(r, o) == "" {
				t.Errorf("empty summary: %s", key)
			}
			if YAML(o, false) == "" {
				t.Errorf("empty YAML: %s", key)
			}
			if _, err := client.Get(ctx, r, o.GetNamespace(), o.GetName()); err != nil {
				t.Errorf("prepared detail %s: %v", key, err)
			}
		}
		t.Logf("Prepared %s: %d objects inspected", key, len(list.Objects))
	}
}
