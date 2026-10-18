package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	bolt "go.etcd.io/bbolt"
)

func TestUnsupportedHistoryIsNotImported(t *testing.T) {
	for _, invalid := range []string{"old-version", "old-stream", "missing-manifest"} {
		t.Run(invalid, func(t *testing.T) {
			s, c := newRecordStore(t)
			projectedFixture(t, s, c)
			h := s.histories[":"+c.ID]
			path := filepath.Join(h.dir, "manifest.json")
			if invalid == "missing-manifest" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			} else {
				manifest := h.manifest
				if invalid == "old-version" {
					manifest.Version = 1
				} else {
					manifest.Streams["context"] = manifest.Streams["execution"]
					delete(manifest.Streams, "execution")
				}
				if err := atomicHistoryJSON(path, manifest); err != nil {
					t.Fatal(err)
				}
			}
			s = closeAndReopenHistory(t, s)
			if _, err := s.Turns("", c.ID); err == nil {
				t.Fatal("unsupported history was accepted")
			}
		})
	}
}

func TestNativeHistoryRebuildRestoresMessagesAndSources(t *testing.T) {
	s, c := newRecordStore(t)
	turn, saved := projectedFixture(t, s, c)
	h := s.histories[":"+c.ID]
	if err := h.db.Update(func(tx *bolt.Tx) error { return tx.DeleteBucket([]byte(c.ID)) }); err != nil {
		t.Fatal(err)
	}
	s = closeAndReopenHistory(t, s)
	messages, err := s.AgentMessages(t.Context(), "", c.ID)
	if err != nil || len(messages) != 2 || messages[1].Content != "One Pod is not ready." {
		t.Fatal(messages, err)
	}
	sources, err := s.TurnEvidence(t.Context(), "", c.ID, turn.Request.ID)
	if err != nil || len(sources) != 1 || sources[0].Ref != saved.Ref || sources[0].Projection == nil {
		t.Fatal(sources, err)
	}
}

func TestSourcesRetainEveryProjectedRead(t *testing.T) {
	s, c := newRecordStore(t)
	turn := requireHistoryTurn(t, s, c, "Read saved observations")
	var refs []string
	for range 96 {
		r, err := appendObservationFixture(s, "", c.ID, ReasoningEntry{MessageID: turn.Request.ID, Content: "saved observation"})
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, r.Ref)
	}
	requireHistoryAnswer(t, s, c, turn, "completed")
	s = closeAndReopenHistory(t, s)
	records, err := s.TurnEvidence(t.Context(), "", c.ID, turn.Request.ID)
	if err != nil || len(records) != len(refs) {
		t.Fatal(len(records), err)
	}
	for i, r := range records {
		if r.Ref != refs[i] || r.Projection == nil {
			t.Fatal("lost projected source", i, r)
		}
	}
}

func TestConversationChunksPreserveUTF8(t *testing.T) {
	original := strings.Repeat("観測🙂\r\n", 10)
	var rebuilt strings.Builder
	for offset := 0; offset < len(original); {
		chunk, err := TextChunk("saved", original, offset, 5)
		if err != nil || chunk.NextOffset <= offset || !utf8.ValidString(chunk.Text) {
			t.Fatal(chunk, err)
		}
		rebuilt.WriteString(chunk.Text)
		offset = chunk.NextOffset
	}
	if rebuilt.String() != original {
		t.Fatal("changed conversation text")
	}
	for _, offset := range []int{-1, 1, len(original) + 1} {
		if _, err := TextChunk("saved", original, offset, 5); err == nil {
			t.Fatal("invalid boundary accepted", offset)
		}
	}
}
