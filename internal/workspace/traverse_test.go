package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestTraverseReturnsCompleteSanitizedRecords(t *testing.T) {
	store, w := workspaceReaderFixture(t)
	const tail = "last-namespace-527"
	for i := 0; i < 30; i++ {
		w.Connections = append(w.Connections, Connection{Tool: "kubernetes", Name: fmt.Sprintf("cluster-%02d", i), Settings: map[string]string{
			"context": fmt.Sprintf("context-%02d", i), "namespace": tail, "token": "private-secret",
		}})
	}
	if _, err := store.Save(w, "test"); err != nil {
		t.Fatal(err)
	}
	reader, err := NewReader(store, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"workspace", "activity"} {
		t.Run(source, func(t *testing.T) {
			cursor, found := "", false
			for {
				page, err := reader.Traverse(t.Context(), Query{Source: source, Cursor: cursor, Limit: 1})
				if err != nil || len(page.Records) != 1 {
					t.Fatalf("page=%+v error=%v", page, err)
				}
				record := page.Records[0]
				if record.Truncated || !json.Valid([]byte(record.Text)) || strings.Contains(record.Text, "private-secret") {
					t.Fatalf("record was truncated, invalid, or exposed credentials: %+v", record)
				}
				if strings.Contains(record.Text, tail) {
					found = true
					if len(record.Text) <= 1024 || strings.Count(record.Text, tail) != 30 {
						t.Fatalf("large record lost connection tails: %s", record.Text)
					}
				}
				if page.Complete {
					if page.NextCursor != "" || page.Truncated {
						t.Fatalf("completed page advertised missing records: %+v", page)
					}
					break
				}
				if page.NextCursor == "" || page.NextCursor == cursor || !page.Truncated {
					t.Fatalf("incomplete page did not advance: %+v", page)
				}
				cursor = page.NextCursor
			}
			if !found {
				t.Fatal("last connection was unavailable through traversal")
			}
		})
	}
}

func TestTraverseWorkspaceAndActivity(t *testing.T) {
	store, w := workspaceReaderFixture(t)
	m, err := NewReader(store, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"workspace", "activity"} {
		p, err := m.Traverse(context.Background(), Query{Source: source, Limit: 1})
		if err != nil || len(p.Records) == 0 {
			t.Fatal(p, err)
		}
		for _, r := range p.Records {
			if strings.Contains(r.Text, "private-secret") {
				t.Fatal("credential exposed", r)
			}
		}
	}
}

func TestTraverseRejectsRetiredHistoryRouteAndBindsCursor(t *testing.T) {
	store, w := workspaceReaderFixture(t)
	m, err := NewReader(store, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"conversation", "reasoning", "both", ""} {
		if _, err = m.Traverse(t.Context(), Query{Source: source}); err == nil {
			t.Fatal("retired history route returned", source)
		}
	}
	w.Name = "New name"
	if _, err = store.Save(w, "test"); err != nil {
		t.Fatal(err)
	}
	m, err = NewReader(store, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	page, err := m.Traverse(t.Context(), Query{Source: "activity", Limit: 1})
	if err != nil || page.NextCursor == "" {
		t.Fatal(page, err)
	}
	if _, err = m.Traverse(t.Context(), Query{Source: "workspace", Cursor: page.NextCursor}); err == nil {
		t.Fatal("cursor crossed source scope")
	}
	reopened, err := NewReader(store, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = reopened.Traverse(t.Context(), Query{Source: "activity", Cursor: page.NextCursor}); err == nil {
		t.Fatal("cursor crossed captured reader")
	}
}
