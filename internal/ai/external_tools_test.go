package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/ai/history"
	"github.com/simjay/plous/internal/workspace"
)

func TestNativeToolsPersistProcessFailure(t *testing.T) {
	o, _ := workspaceFixture(t)
	o.Agent = agent.ClaudeCode
	session, err := OpenExternalTools(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	failure := errors.New("native process exited with status 1")
	if err = session.Finish(failure); err != nil {
		t.Fatal("process failure blocked evidence saving", err)
	}
	if err = session.Close(); err != nil {
		t.Fatal("closing the finished session failed", err)
	}
	conversations, err := o.History.Conversations(o.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	for _, conversation := range conversations {
		if conversation.Title != "Claude Code tool evidence" {
			continue
		}
		turns, err := o.History.Turns(o.WorkspaceID, conversation.ID)
		if err != nil || len(turns) != 1 {
			t.Fatal("missing native session outcome", err, turns)
		}
		if turns[0].Response.Status != "error" || turns[0].Response.Error != failure.Error() {
			t.Fatalf("process failure was not retained: %+v", turns[0].Response)
		}
		return
	}
	t.Fatal("native evidence conversation missing")
}

func TestNativeToolsKeepCapturedScopeAndDurableEvidence(t *testing.T) {
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		if r.URL.Path == "/api/v1" {
			deliveryDiscovery(w)
			return
		}
		deliveryPods(w, 3, "")
	}))
	defer server.Close()
	o, _ := workspaceFixture(t)
	path := filepath.Join(t.TempDir(), "kubeconfig")
	config := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: fixture
  cluster:
    server: %s
contexts:
- name: fixture
  context:
    cluster: fixture
    namespace: payments
current-context: fixture
`, server.URL)
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	target, err := kubecommon.ResolveTarget(path, "fixture", "payments")
	if err != nil {
		t.Fatal(err)
	}
	o.Connections = []Connection{{ConnectionID: "east", Tool: "kubernetes", Namespace: "payments", Context: "fixture", Target: target, TargetCaptured: true}}
	o.Agent = agent.ClaudeCode
	runtime := DefaultRuntimeConfig()
	runtime.TimeoutSeconds, runtime.MaxSteps = 1, 1
	o.Runtime = &runtime
	session, err := OpenExternalTools(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if _, bounded := session.Context.Deadline(); bounded {
		t.Fatal("native lifetime inherited a Plous turn deadline")
	}
	// Mutating the caller's selected connection cannot change the captured one.
	o.Connections[0].ConnectionID = "other"
	call := agent.ToolCall{ID: "native-1", Name: "kubernetes_resources_read", Args: json.RawMessage(`{"purpose":"Read captured pods","connection_id":"east","operation":"get","resource":"pods"}`)}
	reply, err := session.Call(t.Context(), call, 65536)
	if err != nil || !strings.Contains(string(reply), "record:") {
		t.Fatalf("missing saved receipt: %s %v", reply, err)
	}
	before := reads.Load()
	call.ID = "native-2"
	call.Args = json.RawMessage(`{"purpose":"Attempt another scope","connection_id":"other","operation":"get","resource":"pods"}`)
	_, err = session.Call(t.Context(), call, 65536)
	if err != nil || reads.Load() != before {
		t.Fatal("native session acquired another connection", reads.Load(), before, err)
	}
	conversations, err := o.History.Conversations(o.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	id := ""
	for _, c := range conversations {
		if c.Title == "Claude Code tool evidence" {
			id = c.ID
		}
	}
	evidence, err := o.History.TurnEvidence(t.Context(), o.WorkspaceID, id, session.ID)
	if err != nil || len(evidence) != 2 || len(evidence[0].Projection.Rows) != 3 {
		t.Fatal("tool delivery preceded durable evidence", err, evidence)
	}
	if err = session.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = session.Call(t.Context(), call, 65536); err == nil || reads.Load() != before {
		t.Fatal("closed session retained authority", err)
	}
	root, _ := o.Store.DataRoot()
	if err = o.Store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	h, err := history.Open(store)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	saved, err := h.TurnEvidence(context.Background(), o.WorkspaceID, id, session.ID)
	if err != nil || len(saved) != 2 || saved[0].Ref != evidence[0].Ref {
		t.Fatal("native evidence lost after restart", err)
	}
}
