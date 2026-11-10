package ai

import (
	"testing"
)

func TestRetiredSourceToolsCannotBeAdvertisedOrDispatched(t *testing.T) {
	op := &operation{o: Options{WorkspaceID: "saved", Connections: []Connection{{ConnectionID: "old", Tool: "source", Path: t.TempDir()}}}}
	retired := map[string]bool{"source_files_read": true, "source_analysis_read": true, "source_show": true}
	for _, definition := range ToolDefinitions(op.o) {
		if retired[definition.Name] {
			t.Fatal("retired tool was advertised", definition.Name)
		}
	}
	for name := range retired {
		if _, err := op.externalRead(t.Context(), name, `{"connection_id":"old","operation":"read","path":"README.md"}`); err == nil {
			t.Fatal("retired tool was dispatched", name)
		}
	}
	if _, err := op.connection("old", ""); err == nil {
		t.Fatal("retired connection was accepted")
	}
}
