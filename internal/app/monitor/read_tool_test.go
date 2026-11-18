package monitor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMonitorReadScheduleUsesCapturedScopeAndProcessStatus(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC()
	states := []State{
		{Name: "retrying", Running: true, LastCheck: now.Add(-2 * time.Minute), Interval: time.Minute, NextCheck: now.Add(2 * time.Minute)},
		{Name: "overdue", Running: true, NextCheck: now.Add(-time.Minute)},
		{Name: "unknown", Running: true},
		{Name: "stopped", NextCheck: now.Add(-time.Minute)},
		{Name: "foreign", Running: true, NextCheck: now.Add(-time.Minute)},
	}
	for _, state := range states {
		dir := filepath.Join(root, state.Name)
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		lock, err := lockMonitor(dir, state.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { lock.Close() })
		state.Fingerprint = "captured"
		if state.Name == "foreign" {
			state.Fingerprint = "another-cluster"
		}
		if err := save(dir, storedState{State: state}); err != nil {
			t.Fatal(err)
		}
	}

	reader := ReadTool{Root: root}
	result, err := reader.Read("captured", "connection", map[string]json.RawMessage{"operation": json.RawMessage(`"status"`)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Connection != "connection" || !result.Coverage.Complete || len(result.Rows) != 4 {
		t.Fatalf("expected only complete status for the captured scope: %+v", result)
	}
	for _, row := range result.Rows {
		name := row.Identity.Name
		overdue := row.Facts["overdue"].Boolean
		if overdue == nil || *overdue != (name == "overdue") {
			t.Fatalf("incorrect overdue state for %s: %+v", name, row.Facts)
		}
		scheduled := name == "retrying" || name == "overdue"
		if _, found := row.Facts["next_check"]; found != scheduled {
			t.Fatalf("incorrect schedule presence for %s: %+v", name, row.Facts)
		}
		if name == "retrying" && row.Facts["next_check"].String() != states[0].NextCheck.Format(time.RFC3339) {
			t.Fatalf("retry schedule was derived from its base interval: %+v", row.Facts)
		}
	}
}

func TestMonitorReadRejectsArgumentsOutsideOperation(t *testing.T) {
	reader := ReadTool{Root: t.TempDir()}
	for _, raw := range []string{
		`{"operation":"status","alert_id":"saved"}`,
		`{"operation":"list","alert_id":""}`,
		`{"operation":"get","alert_id":"saved","namespace":"other"}`,
		`{"operation":"get"}`,
	} {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &fields); err != nil {
			t.Fatal(err)
		}
		if _, err := reader.Read("captured", "connection", fields); err == nil {
			t.Errorf("accepted unsupported arguments: %s", raw)
		}
	}
}
