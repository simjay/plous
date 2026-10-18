package history

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/simjay/plous/internal/integrations/observation"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func historyBenchmarkFixture(b testing.TB, count int, bodyBytes int) (*Store, Conversation) {
	b.Helper()
	s, err := openTestHistory(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { s.Close() })
	c, err := s.NewConversation("", "Benchmark")
	if err != nil {
		b.Fatal(err)
	}
	h, err := s.conversationHistory("", c.ID)
	if err != nil {
		b.Fatal(err)
	}
	var files = map[string]*os.File{}
	var writers = map[string]*bufio.Writer{}
	var offsets = emptyOffsets()
	var sequence uint64
	for _, stream := range historyStreams {
		path, _ := h.path(stream, 1)
		f, err := os.OpenFile(path, os.O_WRONLY, 0600)
		if err != nil {
			b.Fatal(err)
		}
		files[stream] = f
		writers[stream] = bufio.NewWriterSize(f, 1<<20)
	}
	write := func(stream, turn string, value any) {
		data, _ := json.Marshal(value)
		var object map[string]json.RawMessage
		_ = json.Unmarshal(data, &object)
		sequence++
		for k, v := range map[string]any{"version": HistoryFormat, "log_id": h.manifest.Log, "conversation_id": c.ID, "sequence": fmt.Sprint(sequence), "stream": stream, "turn_id": turn} {
			object[k], _ = json.Marshal(v)
		}
		data, _ = json.Marshal(object)
		data = append(data, '\n')
		if _, err := writers[stream].Write(data); err != nil {
			b.Fatal(err)
		}
		offsets[stream] += int64(len(data))
	}
	body := make([]byte, bodyBytes)
	for i := range body {
		body[i] = 'x'
	}
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("turn-%08d", i)
		now := time.Unix(int64(i+1), 0).UTC()
		prompt := fmt.Sprintf("Archive observation %d", i)
		if i == 0 || i == count/2 || i == count-1 {
			prompt = fmt.Sprintf("needle-%08d release-%08d", i, i)
		}
		request := Request{ID: id, Time: now, Prompt: prompt}
		write("execution", id, ReasoningEntry{ID: "request-" + id, MessageID: id, Kind: "request", Status: "completed", Time: now, Request: &request})
		write("conversation", id, ConversationRecord{ID: id, TurnID: id, Role: "user", Time: now, Response: Response{Status: "completed", Text: request.Prompt, Updated: now}})
		if bodyBytes > 0 {
			r := observation.New("fixture.text.v1", "get", now)
			r.Facts["text"] = observation.Text(string(body))
			write("reasoning", id, ReasoningEntry{ID: "tool-" + id, MessageID: id, Kind: "tool", Tool: "kubernetes_resources_read", Status: r.Outcome, Time: now, Projection: &r})
		}
		answer := "answer-" + id
		write("conversation", id, ConversationRecord{ID: answer, TurnID: id, Role: "assistant", Time: now, Response: Response{Status: "completed", Text: prompt, Updated: now}})
		write("conversation", id, ConversationRecord{ID: "end-" + id, Kind: "turn_end", TurnID: id, AnswerID: answer, Time: now, Response: Response{Status: "completed", Updated: now}, Boundaries: historyOffsets{"reasoning": offsets["reasoning"], "execution": offsets["execution"]}})
	}
	for stream, w := range writers {
		if err := w.Flush(); err != nil {
			b.Fatal(err)
		}
		if err := files[stream].Sync(); err != nil {
			b.Fatal(err)
		}
		files[stream].Close()
	}
	// Fixture construction bypasses per-entry syncing only in this test helper.
	h.db.NoSync = true
	if err := h.recover(); err != nil {
		b.Fatal(err)
	}
	h.db.NoSync = false
	if err := h.db.Sync(); err != nil {
		b.Fatal(err)
	}
	return s, c
}
func BenchmarkHistoryKnownSource10000(b *testing.B) {
	s, c := historyBenchmarkFixture(b, 10000, 0)
	h := s.histories[":"+c.ID]
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.ResolveRecord(context.Background(), "", c.ID, "record:"+c.ID+":answer-turn-00005000", RecordBoundary{Sequence: h.meta.Sequence}); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkHistoryRecentReopen10000(b *testing.B) {
	s, c := historyBenchmarkFixture(b, 10000, 0)
	root := s.root
	s.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reopened, err := openTestHistory(root)
		if err != nil {
			b.Fatal(err)
		}
		p, err := reopened.TurnPage(context.Background(), "", c.ID, 0, 48, 4<<20)
		if err != nil || len(p.Turns) != 48 {
			b.Fatal(len(p.Turns), err)
		}
		reopened.Close()
	}
}
func BenchmarkHistorySaveWith10000Turns(b *testing.B) {
	s, c := historyBenchmarkFixture(b, 10000, 0)
	var samples []time.Duration
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		turn, err := s.StartTurn("", c.ID, Request{Prompt: "new question"})
		if err != nil {
			b.Fatal(err)
		}
		start := time.Now()
		if err = s.SaveResponse("", c.ID, turn.Request.ID, Response{Status: "completed", Text: "answer"}); err != nil {
			b.Fatal(err)
		}
		samples = append(samples, time.Since(start))
	}
	b.StopTimer()
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	b.ReportMetric(float64(samples[(len(samples)-1)*95/100].Nanoseconds()), "save-p95-ns")
}
func BenchmarkHistoryNearFullActiveReopen(b *testing.B) {
	s, c := historyBenchmarkFixture(b, 240, 1<<20)
	root := s.root
	s.Close()
	info, err := os.Stat(filepath.Join(root, "conversations", c.ID, "reasoning", "000001.jsonl"))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestHistoryReopenProcess$")
		cmd.Env = append(os.Environ(), "PLOUS_HISTORY_BENCH_ROOT="+root, "PLOUS_HISTORY_BENCH_CONVERSATION="+c.ID)
		if output, err := cmd.CombinedOutput(); err != nil {
			b.Fatalf("subprocess reopen: %v: %s", err, output)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(info.Size()), "active-bytes")
}
func TestHistoryReopenProcess(t *testing.T) {
	root := os.Getenv("PLOUS_HISTORY_BENCH_ROOT")
	if root == "" {
		t.Skip("benchmark subprocess only")
	}
	s, err := openTestHistory(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, err := s.TurnPage(context.Background(), "", os.Getenv("PLOUS_HISTORY_BENCH_CONVERSATION"), 0, 48, 4<<20)
	if err != nil || len(p.Turns) != 48 {
		t.Fatal(len(p.Turns), err)
	}
}
