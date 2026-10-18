package history

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	aimodel "github.com/simjay/plous/internal/ai/model"
	"github.com/simjay/plous/internal/integrations/observation"
)

func projectedFixture(t *testing.T, s *Store, c Conversation) (Turn, ReasoningEntry) {
	t.Helper()
	turn, err := s.StartTurn(c.WorkspaceID, c.ID, Request{Prompt: "Why is payments unavailable?"})
	if err != nil {
		t.Fatal(err)
	}
	r := observation.New("kubernetes.pods.v1", "get", time.Now().UTC())
	r.Connection = "cluster"
	r.Namespace = "payments"
	r.Selection = map[string]string{"resource": "pods"}
	r.Rows = []observation.Row{{Identity: observation.Identity{API: "v1", Kind: "Pod", Name: "api", UID: "pod-one"}, Facts: map[string]observation.Value{"ready": observation.Boolean(false), "reason": observation.Text("OOMKilled")}}}
	r.Coverage.Observed = 1
	r.Complete()
	r.Facts["unready"] = observation.Number(1)
	e, err := s.SaveObservation(c.WorkspaceID, c.ID, turn.Request.ID, "kubernetes_resources_read", json.RawMessage(`{"operation":"get"}`), r)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveMessages(c.WorkspaceID, c.ID, turn.Request.ID, []aimodel.RequestMessage{{Role: "user", Content: turn.Request.Prompt}, {Role: "assistant", Content: "One Pod is not ready."}}); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveResponse(c.WorkspaceID, c.ID, turn.Request.ID, Response{Status: "completed", Text: "One Pod is not ready."}); err != nil {
		t.Fatal(err)
	}
	return turn, e
}
func TestProjectedHistoryRestartIndexAndScope(t *testing.T) {
	s, c := newRecordStore(t)
	turn, e := projectedFixture(t, s, c)
	index, err := s.SearchIndex("", c.ID, SearchOptions{})
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
	hits, err := access.Call(t.Context(), HistoryRequest{Operation: "search", Query: "OOMKilled"}, 65536)
	if err != nil || len(hits.Search.Hits) != 1 || !hits.Search.LexicalComplete {
		t.Fatal(hits, err)
	}
	if hits.Search.Hits[0].Ref != e.Ref {
		t.Fatal(hits)
	}
	for _, ref := range []string{"kubernetes:pods:payments:checkout", "record::missing", "turn:" + c.ID + ":"} {
		_, err := access.Call(t.Context(), HistoryRequest{Operation: "window", Ref: ref}, 65536)
		var invalid *QueryError
		if !errors.As(err, &invalid) {
			t.Fatal("malformed reference must receive corrective feedback", ref, err)
		}
	}
	counted, err := access.Call(t.Context(), HistoryRequest{Operation: "count", Ref: e.Ref, Field: "ready", Equals: func() *observation.Value { v := observation.Boolean(false); return &v }()}, 65536)
	if err != nil || counted.Calculation.Matched != 1 || !counted.Calculation.Complete {
		t.Fatal(counted, err)
	}
	other, err := s.NewConversation("", "other")
	if err != nil {
		t.Fatal(err)
	}
	_, foreign := projectedFixture(t, s, other)
	var invalid *QueryError
	if _, err = access.Call(t.Context(), HistoryRequest{Operation: "read", Ref: foreign.Ref}, 65536); err == nil || errors.As(err, &invalid) {
		t.Fatal("cross-conversation General history escape")
	}
	s = closeAndReopenHistory(t, s)
	m, err := s.AgentMessages(t.Context(), "", c.ID)
	if err != nil || len(m) != 2 || m[0].Content != turn.Request.Prompt {
		t.Fatal(m, err)
	}
	sources, err := s.TurnEvidence(t.Context(), "", c.ID, turn.Request.ID)
	if err != nil || len(sources) != 1 || sources[0].Projection == nil {
		t.Fatal(sources, err)
	}
	index, err = s.SearchIndex("", c.ID, SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err = index.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}
	access, err = NewAccess(s, "", c.ID, index)
	if err != nil {
		t.Fatal(err)
	}
	hits, err = access.Call(t.Context(), HistoryRequest{Operation: "search", Query: "OOMKilled"}, 65536)
	if err != nil || len(hits.Search.Hits) != 1 {
		t.Fatal("replay duplicated evidence", hits, err)
	}
}
func TestCodeNeverUsesEmbeddings(t *testing.T) {
	s, c := newRecordStore(t)
	projectedFixture(t, s, c)
	var calls atomic.Int32
	index, err := s.SearchIndex("", c.ID, SearchOptions{EmbeddingID: "test", Embed: func(context.Context, []string) ([][]float64, int64, error) {
		calls.Add(1)
		return nil, 0, errors.New("not allowed")
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err = index.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = index.embedPending(t.Context()); err != nil {
		t.Fatal(err)
	}
	access, err := NewAccess(s, "", c.ID, index)
	if err != nil {
		t.Fatal(err)
	}
	_, err = access.Call(t.Context(), HistoryRequest{Operation: "search", Query: "payments"}, 65536)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("Code called embedding provider")
	}
}
func TestMonitorSemanticSearchAndUnavailableCoverage(t *testing.T) {
	s, c := newRecordStore(t)
	_, e := projectedFixture(t, s, c)
	embed := func(ctx context.Context, texts []string) ([][]float64, int64, error) {
		vectors := make([][]float64, len(texts))
		for i, text := range texts {
			if strings.Contains(text, "OOMKilled") || text == "memory exhaustion" {
				vectors[i] = []float64{1, 0}
			} else {
				vectors[i] = []float64{0, 1}
			}
		}
		return vectors, 10, nil
	}
	index, err := s.SearchIndex("", c.ID, SearchOptions{Monitor: true, EmbeddingID: "test-vector-v1", Embed: embed})
	if err != nil {
		t.Fatal(err)
	}
	if err = index.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = index.embedPending(t.Context()); err != nil {
		t.Fatal(err)
	}
	access, err := NewAccess(s, "", c.ID, index)
	if err != nil {
		t.Fatal(err)
	}
	hits, err := access.Call(t.Context(), HistoryRequest{Operation: "search", Query: "memory exhaustion"}, 65536)
	if err != nil || len(hits.Search.Hits) == 0 || hits.Search.Hits[0].Ref != e.Ref {
		t.Fatal(hits, err)
	}
}
func TestFilteredComparisonPreservesUnknownsAndDeduplicates(t *testing.T) {
	s, c := newRecordStore(t)
	_, first := projectedFixture(t, s, c)
	for _, unknown := range []bool{false, true} {
		turn, err := s.StartTurn("", c.ID, Request{Prompt: "Compare current readiness"})
		if err != nil {
			t.Fatal(err)
		}
		r := *first.Projection
		r.Rows = []observation.Row{
			{Identity: observation.Identity{API: "v1", Kind: "Pod", Name: "api", UID: "pod-one"}, Facts: map[string]observation.Value{"ready": observation.Boolean(true)}},
			{Identity: observation.Identity{API: "v1", Kind: "Pod", Name: "new", UID: "pod-two"}, Facts: map[string]observation.Value{"ready": observation.Boolean(false)}},
		}
		if unknown {
			r.Rows[0].Facts["ready"] = observation.Unknown()
		}
		r.Rows = append(r.Rows, r.Rows[1])
		r.Coverage.Observed = 3
		r.Complete()
		second, err := s.SaveObservation("", c.ID, turn.Request.ID, "kubernetes_resources_read", nil, r)
		if err != nil {
			t.Fatal(err)
		}
		a, err := NewAccess(s, "", c.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		value := observation.Boolean(false)
		reply, err := a.Call(t.Context(), HistoryRequest{Operation: "compare", Ref: first.Ref, Other: second.Ref, Field: "ready", Equals: &value}, 65536)
		if err != nil {
			t.Fatal(err)
		}
		got := reply.Calculation
		if got.Shared != 0 || got.Added != 1 || got.Complete == unknown {
			t.Fatal(got)
		}
		if unknown && (got.Removed != 0 || got.Unknown != 1) || !unknown && got.Removed != 1 {
			t.Fatal(got)
		}
		if err = s.SaveResponse("", c.ID, turn.Request.ID, Response{Status: "completed", Text: "Compared"}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDerivedSearchRebuildKeepsOriginalReferences(t *testing.T) {
	s, c := newRecordStore(t)
	_, saved := projectedFixture(t, s, c)
	x, err := s.SearchIndex("", c.ID, SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err = x.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = x.Rebuild(t.Context()); err != nil {
		t.Fatal(err)
	}
	a, err := NewAccess(s, "", c.ID, x)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := a.Call(t.Context(), HistoryRequest{Operation: "search", Query: "OOMKilled"}, 65536)
	if err != nil || len(reply.Search.Hits) != 1 || reply.Search.Hits[0].Ref != saved.Ref {
		t.Fatal(reply, err)
	}
}

func TestLexicalIdentifiersIgnoreSentencePunctuation(t *testing.T) {
	got := terms("Use release-11. Open config.yaml. ...")
	if got["release-11"] != 1 || got["config.yaml"] != 1 || got[""] != 0 || got["release-11."] != 0 {
		t.Fatal(got)
	}
}

func TestRecallSourcesResolveSavedOriginalsAfterRestart(t *testing.T) {
	s, c := newRecordStore(t)
	_, original := projectedFixture(t, s, c)
	page, err := s.TurnPageAfter(t.Context(), "", c.ID, 0, 1, MaxRecordBytes)
	if err != nil {
		t.Fatal(err)
	}
	answerRef := page.Turns[0].AnswerRef
	turn, err := s.StartTurn("", c.ID, Request{Prompt: "What did we find?"})
	if err != nil {
		t.Fatal(err)
	}
	r := observation.New("history.read.v1", "history", time.Now().UTC())
	for _, ref := range []string{original.Ref, answerRef, original.Ref} {
		r.Rows = append(r.Rows, observation.Row{Identity: observation.Identity{Kind: "SavedEvidence", Name: ref}, Facts: map[string]observation.Value{"role": observation.Text("history citation")}})
	}
	r.Coverage.Observed = len(r.Rows)
	r.Partial("selected evidence")
	_, err = s.SaveObservation("", c.ID, turn.Request.ID, "history", nil, r)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveResponse("", c.ID, turn.Request.ID, Response{Status: "completed", Text: "One Pod was not ready."}); err != nil {
		t.Fatal(err)
	}
	s = closeAndReopenHistory(t, s)
	evidence, err := s.TurnEvidence(t.Context(), "", c.ID, turn.Request.ID)
	if err != nil || len(evidence) != 3 {
		t.Fatal(evidence, err)
	}
	found := map[string]ReasoningEntry{}
	for _, e := range evidence {
		found[e.Ref] = e
	}
	if !strings.Contains(found[original.Ref].Content, "OOMKilled") || !strings.Contains(found[answerRef].Content, "One Pod is not ready.") || !strings.Contains(found[answerRef].Content, `"role":"assistant"`) {
		t.Fatal(evidence)
	}
}

func TestComparisonsRequireIntegrationSelection(t *testing.T) {
	s, c := newRecordStore(t)
	_, first := projectedFixture(t, s, c)
	turn, err := s.StartTurn("", c.ID, Request{Prompt: "compare"})
	if err != nil {
		t.Fatal(err)
	}
	for _, selection := range []map[string]string{nil, {"resource": "pods", "name": "api"}, {"resource": "pods", "label_selector": "app=other"}} {
		r := *first.Projection
		r.Selection = selection
		second, err := s.SaveObservation("", c.ID, turn.Request.ID, "kubernetes_resources_read", nil, r)
		if err != nil {
			t.Fatal(err)
		}
		a, err := NewAccess(s, "", c.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = a.Call(t.Context(), HistoryRequest{Operation: "compare", Ref: first.Ref, Other: second.Ref}, 65536); err == nil {
			t.Fatal("compared different or unrecorded selections", selection)
		}
	}
}

func TestLexicalIdentifierAcrossFragmentBoundary(t *testing.T) {
	s, c := newRecordStore(t)
	identifier := "deployment-payments-east-731"
	prompt := strings.Repeat("a ", 2045) + identifier + " end"
	turn, err := s.StartTurn("", c.ID, Request{Prompt: prompt})
	if err != nil {
		t.Fatal(err)
	}
	index, err := s.SearchIndex("", c.ID, SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err = index.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}
	a, err := NewAccess(s, "", c.ID, index)
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Call(t.Context(), HistoryRequest{Operation: "search", Query: identifier}, 65536)
	if err != nil || len(got.Search.Hits) != 1 || !strings.Contains(got.Search.Hits[0].Text, identifier) {
		t.Fatal(got, err)
	}
	hit := got.Search.Hits[0]
	if prompt[hit.Fragment:hit.Fragment+len(hit.Text)] != hit.Text {
		t.Fatal("fragment lost original byte positions")
	}
	if err = s.SaveResponse("", c.ID, turn.Request.ID, Response{Status: "completed", Text: "done"}); err != nil {
		t.Fatal(err)
	}
}

func TestReasoningWriterRejectsRawAndMixedRecords(t *testing.T) {
	s, c := newRecordStore(t)
	turn, err := s.StartTurn("", c.ID, Request{Prompt: "question"})
	if err != nil {
		t.Fatal(err)
	}
	r := observation.New("fixture", "get", time.Now().UTC())
	r.Complete()
	for _, entry := range []ReasoningEntry{
		{MessageID: turn.Request.ID, Kind: "tool", Tool: "kubernetes_resources_read", Status: "completed", Content: "raw"},
		{MessageID: turn.Request.ID, Kind: "tool", Tool: "kubernetes_resources_read", Status: "completed", Projection: &r, Content: "hidden raw"},
		{MessageID: turn.Request.ID, Kind: "messages", Tool: "kubernetes_resources_read", Status: "completed", Projection: &r},
	} {
		if _, err := s.AppendReasoning("", c.ID, entry); err == nil {
			t.Fatal("raw or mixed record accepted")
		}
	}
	if _, err := s.SaveObservation("", c.ID, turn.Request.ID, "kubernetes_resources_read", nil, r); err != nil {
		t.Fatal(err)
	}
}

func TestCheckpointSurvivesManyInterruptedFollowups(t *testing.T) {
	s, c := newRecordStore(t)
	original, _ := projectedFixture(t, s, c)
	for range 105 {
		turn, err := s.StartTurn("", c.ID, Request{Prompt: "interrupted followup"})
		if err != nil {
			t.Fatal(err)
		}
		if err = s.SaveResponse("", c.ID, turn.Request.ID, Response{Status: "interrupted", Text: "partial"}); err != nil {
			t.Fatal(err)
		}
	}
	messages, err := s.AgentMessages(t.Context(), "", c.ID)
	if err != nil || len(messages) != 2 || messages[0].Content != original.Request.Prompt {
		t.Fatal("lost last completed checkpoint", messages, err)
	}
}

func TestCalculationRejectsIncompleteFilter(t *testing.T) {
	s, c := newRecordStore(t)
	_, e := projectedFixture(t, s, c)
	a, err := NewAccess(s, "", c.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	value := observation.Boolean(false)
	for _, q := range []HistoryRequest{{Operation: "count", Ref: e.Ref, Field: "ready"}, {Operation: "count", Ref: e.Ref, Equals: &value}} {
		if _, err = a.Call(t.Context(), q, 65536); err == nil {
			t.Fatal("incomplete filter silently became an unfiltered count")
		}
	}
}

func TestHistoryAccessKeepsCapturedWatermark(t *testing.T) {
	s, c := newRecordStore(t)
	_, old := projectedFixture(t, s, c)
	access, err := NewAccess(s, "", c.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = access.Call(t.Context(), HistoryRequest{Operation: "read", Ref: old.Ref}, 65536); err != nil {
		t.Fatal(err)
	}
	_, later := projectedFixture(t, s, c)
	if _, err = access.Call(t.Context(), HistoryRequest{Operation: "read", Ref: later.Ref}, 65536); err == nil {
		t.Fatal("read beyond captured watermark")
	}
	access.PermitSaved(c.ID, later.Sequence)
	if _, err = access.Call(t.Context(), HistoryRequest{Operation: "read", Ref: later.Ref}, 65536); err != nil {
		t.Fatal(err)
	}
}
