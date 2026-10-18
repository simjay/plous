package history

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSearchIndexStartsLazilyAndEmbedsOnlyNewRecordsAfterRestart(t *testing.T) {
	s, c := newRecordStore(t)
	turn := requireHistoryTurn(t, s, c, "Remember initial-marker")
	requireHistoryAnswer(t, s, c, turn, "completed")
	root, err := s.HistoryRoot("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "search")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("saving conversation alone initialized search", err)
	}
	var mu sync.Mutex
	seen := map[string]int{}
	options := SearchOptions{Monitor: true, EmbeddingID: "lifecycle-test", Embed: func(_ context.Context, texts []string) ([][]float64, int64, error) {
		mu.Lock()
		defer mu.Unlock()
		vectors := make([][]float64, len(texts))
		for i, text := range texts {
			seen[text]++
			vectors[i] = []float64{1, 0}
		}
		return vectors, int64(len(texts)), nil
	}}
	awaitIndexed := func(index *SearchIndex, store *Store, query string, expected int) {
		t.Helper()
		access, err := NewAccess(store, "", c.ID, index)
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			out, err := access.Call(t.Context(), HistoryRequest{Operation: "search", Query: query}, 65536)
			if err != nil {
				t.Fatal(err)
			}
			if out.Search.LexicalComplete && out.Search.Semantic == "complete" && len(out.Search.Hits) == expected {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("background indexing did not catch up: %+v", out.Search)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	index, err := s.SearchIndex("", c.ID, options)
	if err != nil {
		t.Fatal(err)
	}
	awaitIndexed(index, s, "initial-marker", 2)
	s = closeAndReopenHistory(t, s)
	index, err = s.SearchIndex("", c.ID, options)
	if err != nil {
		t.Fatal(err)
	}
	awaitIndexed(index, s, "initial-marker", 2)
	turn = requireHistoryTurn(t, s, c, "Remember appended-marker")
	requireHistoryAnswer(t, s, c, turn, "completed")
	awaitIndexed(index, s, "appended-marker", 4)
	mu.Lock()
	defer mu.Unlock()
	for _, text := range []string{"Remember initial-marker", "Remember initial-marker answer", "Remember appended-marker", "Remember appended-marker answer"} {
		if seen[text] != 1 {
			t.Fatalf("saved content embedded %d times: %q", seen[text], text)
		}
	}
}

func TestEmbeddingFailureLeavesSavedEvidenceAndLexicalSearchAvailable(t *testing.T) {
	s, c := newRecordStore(t)
	_, evidence := projectedFixture(t, s, c)
	index, err := s.SearchIndex("", c.ID, SearchOptions{Monitor: true, EmbeddingID: "unavailable-test", Embed: func(context.Context, []string) ([][]float64, int64, error) {
		return nil, 0, errors.New("fixture embedding outage")
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err = index.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}
	access, err := NewAccess(s, "", c.ID, index)
	if err != nil {
		t.Fatal(err)
	}
	out, err := access.Call(t.Context(), HistoryRequest{Operation: "search", Query: "OOMKilled"}, 65536)
	if err != nil || out.Search.Semantic != "unavailable" || !out.Search.LexicalComplete || len(out.Search.Hits) != 1 || out.Search.Hits[0].Ref != evidence.Ref {
		t.Fatal("embedding outage lost lexical evidence", out, err)
	}
	out, err = access.Call(t.Context(), HistoryRequest{Operation: "read", Ref: evidence.Ref}, 65536)
	if err != nil || len(out.Evidence) != 1 || out.Evidence[0].Observation == nil {
		t.Fatal("embedding outage prevented a saved read", out, err)
	}
}

func TestLexicalReadersShareIndexAndEveryIndexReceivesCommits(t *testing.T) {
	store, conversation := newRecordStore(t)
	lexical, err := store.SearchIndex("", conversation.ID, SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	main, err := store.SearchIndex("", conversation.ID, SearchOptions{Monitor: true})
	if err != nil || main != lexical {
		t.Fatal("main did not reuse the opened lexical index", err)
	}
	semantic, err := store.SearchIndex("", conversation.ID, SearchOptions{Monitor: true, EmbeddingID: "test", Embed: func(_ context.Context, texts []string) ([][]float64, int64, error) {
		vectors := make([][]float64, len(texts))
		for n := range vectors {
			vectors[n] = []float64{1, 0}
		}
		return vectors, 0, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	// Initial catch-up completes before appending, so notification fan-out is required.
	for _, index := range []*SearchIndex{lexical, semantic} {
		if err := index.Sync(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	turn := requireHistoryTurn(t, store, conversation, "fanout-marker")
	requireHistoryAnswer(t, store, conversation, turn, "completed")
	deadline := time.Now().Add(5 * time.Second)
	for _, index := range []*SearchIndex{lexical, semantic} {
		access, err := NewAccess(store, "", conversation.ID, index)
		if err != nil {
			t.Fatal(err)
		}
		for {
			result, err := access.Call(t.Context(), HistoryRequest{Operation: "search", Query: "fanout-marker"}, 65536)
			if err != nil {
				t.Fatal(err)
			}
			if result.Search.LexicalComplete && len(result.Search.Hits) >= 2 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("an active index missed the durable commit", result.Search)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}
