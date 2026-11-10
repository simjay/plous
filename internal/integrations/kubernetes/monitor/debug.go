package monitor

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/integrations/observation"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	utilexec "k8s.io/client-go/util/exec"
)

type debugOutput struct {
	mu        sync.Mutex
	data      []byte
	truncated bool
}

func (b *debugOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	keep := min(n, MaxAgentBytes-len(b.data))
	b.data = append(b.data, p[:keep]...)
	b.truncated = b.truncated || keep < n
	return n, nil
}
func (b *debugOutput) snapshot() (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.ToValidUTF8(string(b.data), "�"), b.truncated
}

func (c *Client) Debug(ctx context.Context, tool string, command DebugCommand, policy DebugPolicy, connection string) (r observation.Result, err error) {
	r = observation.New("kubernetes.debug.v1", tool, time.Now().UTC())
	r.Connection, r.Namespace = connection, command.Namespace
	r.Selection = map[string]string{"test": command.Test, "target": command.Target, "port": strconv.Itoa(command.Port), "pod": command.Name, "uid": command.UID, "container": command.Container}
	raw, _ := json.Marshal(command)
	command, err = DecodeDebug(tool, string(raw), policy, c.Namespace)
	if err != nil {
		r.Outcome = "denied"
		r.Facts["error"] = observation.Text(err.Error())
		return r, nil
	}
	r.Facts["timeout_seconds"] = observation.Number(int64(command.TimeoutSeconds))
	if tool == "kubernetes_exec_run" {
		args, _ := json.Marshal(command.Command)
		hash := sha256.Sum256(args)
		r.Selection["command_sha256"] = hex.EncodeToString(hash[:])
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(command.TimeoutSeconds)*time.Second)
	defer cancel()
	if err = c.checkTarget(); err != nil {
		r.Outcome = "denied"
		r.Facts["error"] = observation.Text(c.ExecErrorText(err))
		return r, nil
	}
	config := rest.CopyConfig(c.config)
	config.WarningHandlerWithContext = rest.NoWarnings{}
	rawClient, err := guardedREST(config)
	if err != nil {
		return r, err
	}
	c = &Client{target: c.target, config: config, Namespace: c.Namespace, rest: rawClient, dynamic: dynamic.New(rawClient)}
	if tool == "kubernetes_diagnostic_run" {
		return c.diagnostic(ctx, command, policy, r)
	}
	var output debugOutput
	err = c.Exec(ctx, ExecOptions{Namespace: command.Namespace, Pod: command.Name, Container: command.Container, UID: types.UID(command.UID), Command: command.Command, Stdout: &output, Stderr: &output})
	r.Facts["location"] = observation.Text("existing container")
	r.Rows = []observation.Row{{Identity: observation.Identity{API: "v1", Kind: "Pod", Namespace: command.Namespace, Name: command.Name, UID: command.UID}, Facts: map[string]observation.Value{"container": observation.Text(command.Container)}}}
	r.Coverage.Observed = 1
	text, truncated := output.snapshot()
	debugOutputRows(&r, r.Rows[0].Identity, text)
	r.Facts["exit_code"] = observation.Unknown()
	var exit utilexec.ExitError
	if err == nil {
		r.Facts["exit_code"] = observation.Number(0)
		r.Complete()
	} else if errors.As(err, &exit) {
		r.Facts["exit_code"] = observation.Number(int64(exit.ExitStatus()))
		r.Complete()
	} else {
		r.Outcome = "error"
		r.Facts["error"] = observation.Text(c.ExecErrorText(err))
		r.Facts["remote_outcome"] = observation.Text("unknown; do not automatically retry a mutating command")
	}
	if err == nil || errors.As(err, &exit) {
		pod, checkErr := c.Get(ctx, kubecommon.Resource{Name: "pods", Version: "v1", Kind: "Pod", Namespaced: true}, command.Namespace, command.Name)
		if checkErr != nil || string(pod.GetUID()) != command.UID {
			r.Partial("Pod identity could not be confirmed after exec; output cannot establish which Pod instance ran the command")
		}
	}
	if truncated {
		r.Partial("combined stdout and stderr exceeded 64 KiB")
	}
	r.Facts["timed_out"] = observation.Boolean(errors.Is(ctx.Err(), context.DeadlineExceeded))
	return r, nil
}

var debugJobs = schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}
var debugPods = schema.GroupVersionResource{Version: "v1", Resource: "pods"}

func (c *Client) diagnostic(ctx context.Context, command DebugCommand, policy DebugPolicy, r observation.Result) (result observation.Result, resultErr error) {
	args, err := command.ProbeCommand()
	if err != nil {
		return r, err
	}
	key := make([]byte, 12)
	if _, err = rand.Read(key); err != nil {
		return r, err
	}
	token := hex.EncodeToString(key)
	name := "plous-debug-" + token
	r.Selection["job"] = name
	r.Facts["location"] = observation.Text("temporary diagnostic Pod; environment and policy may differ from application")
	r.Facts["image"] = observation.Text(policy.Image)
	r.Facts["exit_code"] = observation.Unknown()
	r.Facts["cleanup"] = observation.Text("unknown")
	jobs := c.dynamic.Resource(debugJobs).Namespace(command.Namespace)
	// This also handles an unconfirmed create: delete only our random name and
	// label, with a UID precondition, using an independent cleanup deadline.
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		job, e := jobs.Get(cleanup, name, metav1.GetOptions{})
		if apierrors.IsNotFound(e) {
			r.Facts["cleanup"] = observation.Text("absent")
		} else if e == nil && job.GetLabels()["plous.dev/diagnostic"] == token && job.GetUID() != "" {
			uid := job.GetUID()
			propagation := metav1.DeletePropagationBackground
			e = jobs.Delete(cleanup, name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}, PropagationPolicy: &propagation})
			if e == nil || apierrors.IsNotFound(e) {
				r.Facts["cleanup"] = observation.Text("deletion requested; garbage collection may still be pending")
			}
		} else if e == nil {
			r.Partial("diagnostic Job identity changed; cleanup was not attempted")
		}
		if e != nil && !apierrors.IsNotFound(e) {
			r.Partial("diagnostic cleanup could not be confirmed; finished Job TTL is the fallback")
			r.Facts["cleanup_error"] = observation.Text(c.ExecErrorText(e))
		}
		r.Facts["timed_out"] = observation.Boolean(errors.Is(ctx.Err(), context.DeadlineExceeded))
		result = r
	}()
	labels := map[string]any{"plous.dev/diagnostic": token}
	manifest := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "batch/v1", "kind": "Job", "metadata": map[string]any{"name": name, "namespace": command.Namespace, "labels": labels},
		"spec": map[string]any{"backoffLimit": int64(0), "activeDeadlineSeconds": int64(command.TimeoutSeconds), "ttlSecondsAfterFinished": int64(300),
			"template": map[string]any{"metadata": map[string]any{"labels": labels}, "spec": map[string]any{
				"restartPolicy": "Never", "automountServiceAccountToken": false,
				"securityContext": map[string]any{"runAsNonRoot": true, "runAsUser": int64(65532), "seccompProfile": map[string]any{"type": "RuntimeDefault"}},
				"containers": []any{map[string]any{"name": "diagnostic", "image": policy.Image, "command": stringSliceAny(args),
					"securityContext": map[string]any{"allowPrivilegeEscalation": false, "readOnlyRootFilesystem": true, "capabilities": map[string]any{"drop": []any{"ALL"}}},
					"resources":       map[string]any{"requests": map[string]any{"cpu": "10m", "memory": "16Mi"}, "limits": map[string]any{"cpu": "100m", "memory": "64Mi"}},
				}},
			}},
		},
	}}
	job, err := jobs.Create(ctx, manifest, metav1.CreateOptions{FieldManager: "plous-debug"})
	if err != nil {
		r.Outcome = "error"
		r.Facts["error"] = observation.Text(c.ExecErrorText(err))
		r.Facts["creation"] = observation.Text("not confirmed; cleanup attempted by unique name")
		return r, nil
	}
	if job.GetUID() == "" {
		r.Outcome = "error"
		r.Facts["error"] = observation.Text("created Job has no UID")
		return r, nil
	}
	r.Rows = []observation.Row{{Identity: observation.Identity{API: "batch/v1", Kind: "Job", Namespace: command.Namespace, Name: name, UID: string(job.GetUID())}, Facts: map[string]observation.Value{}}}
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		pods, e := c.dynamic.Resource(debugPods).Namespace(command.Namespace).List(ctx, metav1.ListOptions{LabelSelector: "plous.dev/diagnostic=" + token, Limit: 2})
		if e != nil {
			r.Outcome = "error"
			r.Facts["error"] = observation.Text(c.ExecErrorText(e))
			return r, nil
		}
		for _, pod := range pods.Items {
			owned := false
			for _, owner := range pod.GetOwnerReferences() {
				owned = owned || owner.UID == job.GetUID() && owner.Kind == "Job" && owner.Controller != nil && *owner.Controller
			}
			if !owned {
				continue
			}
			phase, _, _ := unstructured.NestedString(pod.Object, "status", "phase")
			r.Facts["runner_phase"] = observation.Text(phase)
			statuses, _, _ := unstructured.NestedSlice(pod.Object, "status", "containerStatuses")
			for _, raw := range statuses {
				status, ok := raw.(map[string]any)
				if !ok || status["name"] != "diagnostic" {
					continue
				}
				terminated, found, _ := unstructured.NestedMap(status, "state", "terminated")
				if !found {
					waiting, _, _ := unstructured.NestedString(status, "state", "waiting", "reason")
					if waiting != "" {
						r.Facts["runner_waiting_reason"] = observation.Text(waiting)
					}
					continue
				}
				code, found, _ := unstructured.NestedInt64(terminated, "exitCode")
				if !found {
					continue
				}
				r.Facts["exit_code"] = observation.Number(code)
				r.Rows = append(r.Rows, observation.Row{Identity: observation.Identity{API: "v1", Kind: "Pod", Namespace: command.Namespace, Name: pod.GetName(), UID: string(pod.GetUID())}, Facts: map[string]observation.Value{"container": observation.Text("diagnostic")}})
				stream, e := c.rest.Get().AbsPath("/api/v1/namespaces", command.Namespace, "pods", pod.GetName(), "log").Param("container", "diagnostic").Param("limitBytes", "65537").Stream(ctx)
				if e != nil {
					r.Outcome = "error"
					r.Facts["error"] = observation.Text(c.ExecErrorText(e))
					return r, nil
				}
				data, e := io.ReadAll(io.LimitReader(stream, MaxAgentBytes+1))
				stream.Close()
				debugOutputRows(&r, r.Rows[len(r.Rows)-1].Identity, strings.ToValidUTF8(string(data[:min(len(data), MaxAgentBytes)]), "�"))
				r.Coverage.Observed = 1
				r.Complete()
				if len(data) > MaxAgentBytes {
					r.Partial("diagnostic output exceeded 64 KiB")
				}
				if e != nil {
					r.Partial("diagnostic output could not be fully collected")
				}
				fresh, e := c.dynamic.Resource(debugPods).Namespace(command.Namespace).Get(ctx, pod.GetName(), metav1.GetOptions{})
				if e != nil || fresh.GetUID() != pod.GetUID() {
					r.Partial("diagnostic Pod identity could not be confirmed after log collection")
				}
				return r, nil
			}
		}
		select {
		case <-ctx.Done():
			r.Outcome = "error"
			r.Facts["error"] = observation.Text("diagnostic did not finish before cancellation or deadline")
			return r, nil
		case <-ticker.C:
		}
	}
}

func stringSliceAny(values []string) []any {
	r := make([]any, len(values))
	for i, v := range values {
		r[i] = v
	}
	return r
}

// Output lives in bounded rows so JSON escaping cannot make the aggregate
// envelope undeliverable. History can page all saved chunks without a rerun.
func debugOutputRows(r *observation.Result, identity observation.Identity, text string) {
	r.Facts["output_bytes"] = observation.Number(int64(len(text)))
	for offset := 0; len(text) > 0; {
		n := min(len(text), 8192)
		for n < len(text) && !utf8.RuneStart(text[n]) {
			n--
		}
		r.Rows = append(r.Rows, observation.Row{Identity: identity, Facts: map[string]observation.Value{"output": observation.Text(text[:n]), "output_offset": observation.Number(int64(offset))}})
		offset += n
		text = text[n:]
	}
}
