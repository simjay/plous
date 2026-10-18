package code

import (
	"testing"
)

func TestCodeComparisonAlignsInsertedLines(t *testing.T) {
	rows := CodeComparisonRows("kind: Service\nmetadata:\n  name: api\n", "kind: Service\nmetadata:\n  labels: {}\n  name: api\n")
	if len(rows) != 4 || rows[2].HasBefore || !rows[2].Changed || rows[3].Changed || rows[3].Before != rows[3].After || rows[3].Line != 4 {
		t.Fatal("an insertion shifted equal lines out of alignment", rows)
	}
}
