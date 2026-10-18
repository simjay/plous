package manifest

import (
	"strings"
	"testing"
)

func TestFieldChangesKeepTypesPointersAndHiddenChanges(t *testing.T) {
	before := map[string]any{"a/b": map[string]any{"count": int64(1), "enabled": false, "password": "old-private"}, "empty": map[string]any{}}
	after := map[string]any{"a/b": map[string]any{"count": int64(3), "enabled": true, "password": "new-private"}, "empty": []any{}}
	changes, complete := FieldChanges(before, after)
	if !complete {
		t.Fatal("small comparison incomplete")
	}
	found := map[string]bool{}
	for _, change := range changes {
		found[change.Pointer] = true
		if change.Pointer == "/a~1b/count" && (change.Before.Number == nil || change.After.Number == nil || change.After.String() != "3") {
			t.Fatal(change)
		}
		if change.Pointer == "/a~1b/password" && change.After.String() != "[redacted: changed]" {
			t.Fatal(change)
		}
		if strings.Contains(change.Before.String()+change.After.String(), "private") {
			t.Fatal("unmasked value")
		}
	}
	for _, field := range []string{"/a~1b/count", "/a~1b/enabled", "/a~1b/password", "/empty"} {
		if !found[field] {
			t.Fatal("lost change", field, changes)
		}
	}
	fields, complete := Fields(map[string]any{"description": strings.Repeat("x", 3000), "httpHeaders": []any{map[string]any{"name": "Authorization", "value": "private"}}})
	if complete {
		t.Fatal("oversized field silently complete")
	}
	for _, field := range fields {
		if strings.Contains(field.Value.String(), "private") {
			t.Fatal("unmasked header")
		}
	}
}
