package text

import (
	tea "charm.land/bubbletea/v2"
	"testing"
)

func TestBackspaceControlByteEditsFieldsAndSearch(t *testing.T) {
	f := NewField("A界")
	f.Key(tea.KeyPressMsg{Code: 'h', Mod: tea.ModCtrl}, false)
	if f.Text != "A" || f.Cursor != 1 {
		t.Fatalf("Backspace control byte did not edit field: %#v", f)
	}
	v := Viewer{Searching: true}
	v.Paste("A界")
	v.Key("ctrl+h", "")
	if v.Query != "A" || !v.Searching {
		t.Fatalf("Backspace control byte did not edit search: %q", v.Query)
	}
}

func TestQueryPointerEditsSelectionAndPreservesLiteralInput(t *testing.T) {
	v := Viewer{Searching: true}
	v.SetLines([]Line{{Text: "alpha 界 beta"}})
	v.Paste("alpha 界 beta")
	v.SearchInput.Pointer(v.Query, Point{Col: 6}, nil, true)
	v.Paste("e\u0301")
	if v.Query != "alpha e\u0301 beta" {
		t.Fatal(v.Query)
	}
	v.Key("left", "")
	v.Paste("/")
	if v.Query != "alpha /e\u0301 beta" {
		t.Fatal(v.Query)
	}
	v.SearchInput.Pointer(v.Query, Point{Col: 0}, nil, true)
	v.Key("esc", "")
	if !v.Searching || v.Query == "" {
		t.Fatal("Escape closed search before clearing selection")
	}
	v.Key("end", "")
	v.Key("backspace", "")
	if v.Query != "alpha /e\u0301 bet" {
		t.Fatal(v.Query)
	}
}
