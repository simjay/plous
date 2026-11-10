package ai

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/simjay/plous/internal/ai/agent"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

func TestActiveDiagnosticsRequireDurableIntentBeforeAcquisition(t *testing.T) {
	var requests atomic.Int32
	op := deliveryFixture(t, func(http.ResponseWriter, *http.Request) { requests.Add(1) })
	op.o.Debug = kubernetes.DebugPolicy{Exec: true, Image: "fixture"}
	listed := map[string]bool{}
	for _, tool := range ToolDefinitions(op.o) {
		if tool.Mutating {
			listed[tool.Name] = true
		}
	}
	if !listed["kubernetes_exec_run"] || !listed["kubernetes_diagnostic_run"] {
		t.Fatal("shared capability catalog omits enabled diagnostics", listed)
	}
	session := newToolSession(t.Context(), op)
	defer session.Close()
	if err := op.o.History.Close(); err != nil {
		t.Fatal(err)
	}
	_, err := session.Call(t.Context(), agent.ToolCall{ID: "active", Name: "kubernetes_exec_run", Args: json.RawMessage(`{"purpose":"test durability","connection_id":"east","namespace":"payments","name":"app","uid":"uid","container":"app","command":["true"]}`)}, 65536)
	if err == nil || requests.Load() != 0 {
		t.Fatal("active operation ran without a durable intent", err, requests.Load())
	}
}
