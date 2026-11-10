package monitor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/simjay/plous/internal/integrations/observation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestActiveDiagnosticsDoNotFollowAPIRedirects(t *testing.T) {
	var foreignRequests atomic.Int32
	foreign := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { foreignRequests.Add(1) }))
	defer foreign.Close()
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, foreign.URL+"/unexpected", http.StatusTemporaryRedirect)
	})
	client.Namespace = "shop"
	r, err := client.Debug(t.Context(), "kubernetes_diagnostic_run", DebugCommand{Namespace: "shop", Test: "dns", Target: "payments", TimeoutSeconds: 1}, DebugPolicy{Image: "fixture"}, "captured")
	if err != nil || r.Validate() != nil || foreignRequests.Load() != 0 || r.Facts["exit_code"].String() != "unknown" {
		t.Fatal("active diagnostic escaped the configured API endpoint", err, foreignRequests.Load(), r.Facts)
	}
}

func TestDebugPolicyRejectsDisabledScopeAndArgumentOverrides(t *testing.T) {
	for _, tc := range []struct {
		tool, input string
		policy      DebugPolicy
	}{
		{"kubernetes_exec_run", `{"namespace":"shop","name":"pod","uid":"uid","container":"app","command":["true"]}`, DebugPolicy{}},
		{"kubernetes_exec_run", `{"namespace":"other","name":"pod","uid":"uid","container":"app","command":["true"]}`, DebugPolicy{Exec: true}},
		{"kubernetes_exec_run", `{"namespace":"shop","name":"pod","uid":"uid","container":"app","command":["true"],"test":"http"}`, DebugPolicy{Exec: true}},
		{"kubernetes_diagnostic_run", `{"namespace":"shop","test":"http","target":"http://payments/"}`, DebugPolicy{}},
		{"kubernetes_diagnostic_run", `{"namespace":"shop","test":"http","target":"file:///etc/passwd"}`, DebugPolicy{Image: "test"}},
		{"kubernetes_diagnostic_run", `{"namespace":"shop","test":"http","target":"http://user:password@payments/"}`, DebugPolicy{Image: "test"}},
		{"kubernetes_diagnostic_run", `{"namespace":"shop","test":"dns","target":"-exec"}`, DebugPolicy{Image: "test"}},
		{"kubernetes_diagnostic_run", `{"namespace":"shop","test":"tcp","target":"payments","port":70000}`, DebugPolicy{Image: "test"}},
		{"kubernetes_diagnostic_run", `{"namespace":"shop","test":"dns","target":"payments","image":"override"}`, DebugPolicy{Image: "test"}},
		{"kubernetes_diagnostic_run", `{"namespace":"shop","test":"dns","target":"payments","command":["sh"]}`, DebugPolicy{Image: "test"}},
		{"kubernetes_diagnostic_run", `{"namespace":"shop","test":"dns","target":"payments","timeout_seconds":121}`, DebugPolicy{Image: "test"}},
		{"unknown", `{"namespace":"shop"}`, DebugPolicy{Image: "test"}},
	} {
		if _, err := DecodeDebug(tc.tool, tc.input, tc.policy, "shop"); err == nil {
			t.Fatalf("accepted %s: %s", tc.tool, tc.input)
		}
	}
	command, err := DecodeDebug("kubernetes_diagnostic_run", `{"namespace":"shop","test":"http","target":"http://payments/health?q=a%20b"}`, DebugPolicy{Image: "test"}, "shop")
	if err != nil {
		t.Fatal(err)
	}
	args, _ := command.ProbeCommand()
	if args[len(args)-1] != command.Target || args[len(args)-2] != "--" {
		t.Fatal("HTTP target is not a literal argument", args)
	}
}

func TestDebugExecRecordsNonzeroExitAndBoundsCombinedOutput(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/exec") {
			pod := testPod("pod", "uid")
			pod["status"] = map[string]any{"containerStatuses": []any{map[string]any{"name": "app", "state": map[string]any{"running": map[string]any{}}}}}
			reply(w, pod)
			return
		}
		if !reflect.DeepEqual(r.URL.Query()["command"], []string{"curl", "http://payments/health"}) || r.URL.Query().Get("tty") != "false" || r.URL.Query().Get("stdin") != "false" {
			t.Error("exec changed command or opened interactive input", r.URL)
		}
		upgrader := websocket.Upgrader{Subprotocols: []string{"v5.channel.k8s.io"}}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.BinaryMessage, append([]byte{1}, []byte(strings.Repeat("x", MaxAgentBytes+50))...))
		_ = conn.WriteMessage(websocket.BinaryMessage, append([]byte{2}, []byte("failure")...))
		_ = conn.WriteMessage(websocket.BinaryMessage, append([]byte{3}, []byte(`{"status":"Failure","reason":"NonZeroExitCode","details":{"causes":[{"reason":"ExitCode","message":"22"}]}}`)...))
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	})
	client.Namespace = "shop"
	r, err := client.Debug(t.Context(), "kubernetes_exec_run", DebugCommand{Namespace: "shop", Name: "pod", UID: "uid", Container: "app", Command: []string{"curl", "http://payments/health"}, TimeoutSeconds: 5}, DebugPolicy{Exec: true}, "captured")
	if err != nil || r.Validate() != nil || r.Facts["exit_code"].String() != "22" || r.Facts["output_bytes"].String() != "65536" || r.Coverage.Complete {
		t.Fatalf("wrong exec observation: exit=%s output_bytes=%s complete=%t error=%s; %v", r.Facts["exit_code"].String(), r.Facts["output_bytes"].String(), r.Coverage.Complete, r.Facts["error"].String(), err)
	}
	if _, err = r.Deliver("record:fixture:saved", observation.Selection{}, 65536); err != nil {
		t.Fatal("bounded output cannot be delivered", err)
	}
}

func TestDiagnosticJobOwnsLifecycleAndPreservesTestFailure(t *testing.T) {
	for _, scenario := range []string{"completed", "cancelled", "cleanup-denied"} {
		t.Run(scenario, func(t *testing.T) {
			var job *unstructured.Unstructured
			var deleted bool
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/jobs"):
					job = &unstructured.Unstructured{}
					if err := json.NewDecoder(r.Body).Decode(job); err != nil {
						t.Error(err)
					}
					job.SetUID("job-uid")
					spec, _, _ := unstructured.NestedMap(job.Object, "spec", "template", "spec")
					if spec["automountServiceAccountToken"] != false || spec["restartPolicy"] != "Never" {
						t.Error("runner configuration", spec)
					}
					containers := spec["containers"].([]any)
					container := containers[0].(map[string]any)
					if container["image"] != "configured-image" || container["securityContext"].(map[string]any)["allowPrivilegeEscalation"] != false {
						t.Error("runner changed image or privileges", container)
					}
					if _, present := spec["volumes"]; present {
						t.Error("runner acquired volumes")
					}
					reply(w, job.Object)
				case r.Method == "GET" && strings.Contains(r.URL.Path, "/jobs/"):
					if scenario == "cleanup-denied" {
						http.Error(w, "forbidden", 403)
						return
					}
					reply(w, job.Object)
				case r.Method == "DELETE":
					var options metav1.DeleteOptions
					if err := json.NewDecoder(r.Body).Decode(&options); err != nil {
						t.Error(err)
					}
					if options.Preconditions == nil || options.Preconditions.UID == nil || *options.Preconditions.UID != "job-uid" {
						t.Error("cleanup has no exact identity")
					}
					deleted = true
					reply(w, map[string]any{"kind": "Status", "apiVersion": "v1", "status": "Success"})
				case strings.HasSuffix(r.URL.Path, "/log"):
					fmt.Fprint(w, "HTTP/1.1 503 Service Unavailable\n")
				default:
					pod := testPod("runner", "pod-uid")
					pod["metadata"].(map[string]any)["ownerReferences"] = []any{map[string]any{"apiVersion": "batch/v1", "kind": "Job", "name": job.GetName(), "uid": "job-uid", "controller": true}}
					pod["status"] = map[string]any{"containerStatuses": []any{map[string]any{"name": "diagnostic", "state": map[string]any{"terminated": map[string]any{"exitCode": int64(22)}}}}}
					if scenario == "cancelled" {
						pod["status"] = map[string]any{}
					}
					if strings.HasSuffix(r.URL.Path, "/pods") {
						reply(w, map[string]any{"apiVersion": "v1", "kind": "PodList", "items": []any{pod}})
					} else {
						reply(w, pod)
					}
				}
			})
			client.Namespace = "shop"
			r, err := client.Debug(t.Context(), "kubernetes_diagnostic_run", DebugCommand{Namespace: "shop", Test: "http", Target: "http://payments/health", TimeoutSeconds: 1}, DebugPolicy{Image: "configured-image"}, "captured")
			if err != nil || r.Validate() != nil || job == nil {
				t.Fatalf("diagnostic failed: %+v %v", r, err)
			}
			if scenario != "cleanup-denied" && !deleted {
				t.Fatal("temporary Job was not cleaned up")
			}
			if scenario == "completed" && (r.Facts["exit_code"].String() != "22" || !strings.Contains(r.Rows[len(r.Rows)-1].Facts["output"].String(), "503")) {
				t.Fatal("test failure was lost", r.Facts)
			}
			if scenario == "cancelled" && r.Facts["timed_out"].String() != "true" {
				t.Fatal("timeout was not recorded", r.Facts)
			}
			if scenario == "cleanup-denied" && (r.Coverage.Complete || r.Facts["cleanup"].String() != "unknown") {
				t.Fatal("unconfirmed cleanup reported as complete", r)
			}
		})
	}
}
