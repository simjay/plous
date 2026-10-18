package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/ai/cli"
	journal "github.com/simjay/plous/internal/ai/history"
	"github.com/simjay/plous/internal/integrations/observation"
	"github.com/simjay/plous/internal/workspace"
)

type durableTools struct {
	agent.Tools
	t  *testing.T
	op *operation
}

func (d durableTools) Call(ctx context.Context, c agent.ToolCall, n int) (json.RawMessage, error) {
	body, err := d.Tools.Call(ctx, c, n)
	if err != nil {
		return body, err
	}
	var reply observation.Delivery
	if err = json.Unmarshal(body, &reply); err != nil {
		d.t.Fatal(err)
	}
	saved, err := d.op.o.History.TurnEvidence(ctx, d.op.o.WorkspaceID, d.op.o.ConversationID, d.op.o.TurnID)
	if err != nil || len(saved) == 0 || saved[len(saved)-1].Ref != reply.Ref {
		d.t.Fatal("delivery preceded durable evidence", err)
	}
	return body, nil
}

func TestProductionToolsAndMCPAgreeOnUsageAndSavedEvidence(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture launcher")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 fixture unavailable")
	}
	script, err := filepath.Abs("../../tests/fixtures/ai/tool_parity_process.py")
	if err != nil {
		t.Fatal(err)
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	executable := filepath.Join(t.TempDir(), "fixture-runtime")
	if err = os.WriteFile(executable, []byte("#!/bin/sh\nexec "+quote(python)+" "+quote(script)+" \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, condition := range []string{"complete", "stale", "denied", "closed"} {
		t.Run(condition, func(t *testing.T) {
			var calls atomic.Int32
			now := time.Now().UTC().Add(-time.Second)
			sampleTime := now
			if condition == "stale" {
				sampleTime = now.Add(-time.Hour)
			}
			op := deliveryFixture(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var body any
				switch r.URL.Path {
				case "/api/v1":
					deliveryDiscovery(w)
					return
				case "/apis/metrics.k8s.io":
					body = map[string]any{"preferredVersion": map[string]any{"groupVersion": "metrics.k8s.io/v1beta1"}}
				case "/apis/metrics.k8s.io/v1beta1":
					body = map[string]any{"resources": []any{map[string]any{"name": "pods", "kind": "PodMetrics", "namespaced": true, "verbs": []string{"get", "list"}}}}
				case "/api/v1/namespaces/payments/pods/app":
					body = map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": "app", "namespace": "payments", "uid": "app-uid", "resourceVersion": "1"}, "spec": map[string]any{"containers": []any{map[string]any{"name": "app"}}}, "status": map[string]any{"phase": "Running"}}
				case "/apis/metrics.k8s.io/v1beta1/namespaces/payments/pods/app":
					body = map[string]any{"apiVersion": "metrics.k8s.io/v1beta1", "kind": "PodMetrics", "metadata": map[string]any{"name": "app", "namespace": "payments"}, "timestamp": sampleTime.Format(time.RFC3339Nano), "window": "15s", "containers": []any{map[string]any{"name": "app", "usage": map[string]any{"cpu": "125m", "memory": "10Mi"}}}}
				default:
					t.Errorf("unexpected read %s", r.URL)
					w.WriteHeader(404)
					return
				}
				_ = json.NewEncoder(w).Encode(body)
			})
			session := newToolSession(t.Context(), op)
			defer session.Close()
			args := map[string]any{"purpose": "Measure current Pod", "connection_id": "east", "resource": "pods", "name": "app"}
			if condition == "denied" {
				args["connection_id"] = "other"
			}
			if condition == "closed" {
				session.Close()
			}
			tools := durableTools{session, t, op}
			direct, err := tools.Call(t.Context(), agent.ToolCall{ID: "direct", Name: "kubernetes_usage_read", Args: marshal(args)}, 65536)
			if err != nil && condition != "closed" {
				t.Fatal(err)
			}
			var terminalOutput bytes.Buffer
			native, err := cli.StartTerminal(t.Context(), cli.TerminalOptions{Kind: agent.ClaudeCode, Path: executable, Directory: t.TempDir(), StateDirectory: t.TempDir(), ID: op.o.TurnID, Tools: tools, Width: 80, Height: 24, Input: strings.NewReader(string(marshal(map[string]any{"name": "kubernetes_usage_read", "arguments": args})) + "\n"), Output: &terminalOutput})
			if err == nil {
				err = native.Wait()
			}

			if condition == "closed" {
				if err == nil || calls.Load() != 0 {
					t.Fatal("closed MCP session reached acquisition", err, calls.Load())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var remote struct {
				Definitions []struct {
					Name        string
					InputSchema json.RawMessage
				}
				Delivery observation.Delivery
			}
			foundResult := false
			for _, line := range strings.Split(terminalOutput.String(), "\n") {
				var message struct{ Type, Result string }
				if json.Unmarshal([]byte(strings.TrimSpace(line)), &message) == nil && message.Type == "result" {
					if err = json.Unmarshal([]byte(message.Result), &remote); err != nil {
						t.Fatal(err)
					}
					foundResult = true
				}
			}
			if !foundResult {
				t.Fatalf("no native fixture result: %s", terminalOutput.String())
			}
			for _, d := range tools.Definitions() {
				found := false
				for _, r := range remote.Definitions {
					if d.Name == r.Name {
						found = true
						var a, b any
						_ = json.Unmarshal(d.Parameters, &a)
						_ = json.Unmarshal(r.InputSchema, &b)
						if !reflect.DeepEqual(a, b) {
							t.Fatal("MCP schema drift", d.Name)
						}
					}
				}
				if !found {
					t.Fatal("missing MCP tool", d.Name)
				}
			}
			var local observation.Delivery
			_ = json.Unmarshal(direct, &local)
			normalize := func(d *observation.Delivery) {
				d.Ref = ""
				if d.Observation != nil {
					d.Observation.ObservedAt = time.Time{}
					for i := range d.Observation.Rows {
						delete(d.Observation.Rows[i].Facts, "collected_at")
					}
				}
			}
			normalize(&local)
			normalize(&remote.Delivery)
			if !reflect.DeepEqual(local, remote.Delivery) {
				t.Fatalf("direct/MCP disagreement:\n%s\n%s", marshal(local), marshal(remote.Delivery))
			}
			if condition == "denied" {
				if calls.Load() != 0 {
					t.Fatal("scope denial reached acquisition")
				}
			} else {
				f := local.Observation.Rows[0].Facts
				if condition == "complete" && (f["cpu_nanocores"].Number == nil || *f["cpu_nanocores"].Number != 125000000 || !f["container.app.limits.cpu_nanocores"].Unknown) {
					t.Fatal(f)
				}
				if condition == "stale" && (!f["cpu_nanocores"].Unknown || local.Observation.Coverage.Complete) {
					t.Fatal("partial became zero/complete", f)
				}
			}
			session.Close()
			root, _ := op.o.Store.DataRoot()
			if err = op.o.History.Close(); err != nil {
				t.Fatal(err)
			}
			if err = op.o.Store.Close(); err != nil {
				t.Fatal(err)
			}
			store, err := workspace.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			h, err := journal.Open(store)
			if err != nil {
				t.Fatal(err)
			}
			defer h.Close()
			saved, err := h.TurnEvidence(t.Context(), op.o.WorkspaceID, op.o.ConversationID, op.o.TurnID)
			if err != nil || len(saved) != 2 {
				t.Fatal("restart lost evidence", len(saved), err)
			}
			for _, s := range saved {
				if s.Projection == nil || s.Projection.Validate() != nil {
					t.Fatal("invalid saved observation", s)
				}
			}
		})
	}
}
