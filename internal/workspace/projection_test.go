package workspace

import (
	"encoding/json"
	"testing"
	"time"
)

func TestActivityProjectionPreservesOperationIdentityAndTime(t *testing.T) {
	when := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	op := Operation{ID: "op-one", WorkspaceID: "ws", ConnectionID: "cluster", Tool: "helm", Namespace: "default", Release: "payments", Kind: "rollback", Status: "completed", Revision: 3, Chart: "payments", Version: "1.0.0", SourceTime: when.Format(time.RFC3339), ObservedAt: when}
	body, _ := json.Marshal(op)
	reader := &Reader{nonce: "test", sources: []source{{kind: "activity", activity: "operation-op-one", data: body}}}
	r, err := reader.Project(t.Context(), Query{Source: "activity"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Rows) != 1 {
		t.Fatal(r)
	}
	for key, want := range map[string]string{"release": "payments", "revision": "3", "observed_at": when.Format(time.RFC3339), "source_time": when.Format(time.RFC3339), "chart": "payments", "version": "1.0.0"} {
		if r.Rows[0].Facts[key].String() != want {
			t.Fatalf("lost %s: %+v", key, r)
		}
	}
}
