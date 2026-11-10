package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simjay/plous/internal/ai/agent"
)

func TestDiagnosticIntentIsDurableExecutionMetadata(t *testing.T) {
	s, c := newRecordStore(t)
	turn := requireHistoryTurn(t, s, c, "investigate")
	call := agent.ToolCall{ID: "diagnostic-1", Name: "kubernetes_exec_run", Args: json.RawMessage(`{"command":["curl","http://payments/health"]}`)}
	if err := s.SaveToolIntent("", c.ID, turn.Request.ID, call); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(s.root, "conversations", c.ID, "execution", "000001.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "diagnostic-request") {
		t.Fatal("intent was not committed before execution")
	}
	requireHistoryAnswer(t, s, c, turn, "completed")
	s = closeAndReopenHistory(t, s)
	if _, err = s.Turns("", c.ID); err != nil {
		t.Fatal("intent corrupted conversation recovery", err)
	}
	evidence, err := s.TurnEvidence(t.Context(), "", c.ID, turn.Request.ID)
	if err != nil || len(evidence) != 0 {
		t.Fatal("execution intent entered resource evidence", evidence, err)
	}
	if err = s.SaveToolIntent("", c.ID, turn.Request.ID, call); err == nil {
		t.Fatal("closed turn accepted active intent")
	}
}
