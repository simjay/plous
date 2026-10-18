package history

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/simjay/plous/internal/integrations/observation"
)

// Access is the entire model-facing history boundary. It never returns journal serialization.
type Access struct {
	store     *Store
	workspace string
	bounds    map[string]RecordBoundary
	index     *SearchIndex
	mu        sync.Mutex
}

const MaxPageResults = 100
const DefaultPageResults = 20

type HistoryRequest struct {
	Operation string             `json:"operation"`
	Query     string             `json:"query,omitempty"`
	Ref       string             `json:"ref,omitempty"`
	Other     string             `json:"other,omitempty"`
	Offset    int                `json:"offset,omitempty"`
	Limit     int                `json:"limit,omitempty"`
	Field     string             `json:"field,omitempty"`
	Equals    *observation.Value `json:"equals,omitempty"`
	Fields    []string           `json:"fields"`
}
type Evidence struct {
	observation.Delivery
	Conversation   string    `json:"conversation"`
	Turn           string    `json:"turn"`
	Sequence       uint64    `json:"sequence"`
	Role           string    `json:"role"`
	Status         string    `json:"status"`
	Time           time.Time `json:"time"`
	Text           string    `json:"text,omitempty"`
	NextTextOffset *int      `json:"next_text_offset,omitempty"`
}
type HistoryReply struct {
	NextWindow  int            `json:"next_window"`
	Evidence    []Evidence     `json:"evidence,omitempty"`
	Search      *SearchResults `json:"search,omitempty"`
	Calculation *Calculation   `json:"calculation,omitempty"`
}
type Calculation struct {
	Refs        []string `json:"refs"`
	Complete    bool     `json:"complete"`
	Observed    int      `json:"observed"`
	Matched     int      `json:"matched"`
	Unknown     int      `json:"unknown"`
	Shared      int      `json:"shared"`
	Added       int      `json:"added"`
	Removed     int      `json:"removed"`
	Limitations []string `json:"limitations,omitempty"`
}

func NewAccess(store *Store, w, id string, index *SearchIndex) (*Access, error) {
	bounds, err := store.RecordBoundaries(w, id)
	if err != nil {
		return nil, err
	}
	a := &Access{store: store, workspace: w, index: index, bounds: map[string]RecordBoundary{}}
	for _, b := range bounds {
		a.bounds[b.Conversation.ID] = b
	}
	return a, nil
}

func (a *Access) PermitSaved(conversation string, sequence uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	b, ok := a.bounds[conversation]
	if ok && sequence > b.Sequence {
		b.Sequence = sequence
		a.bounds[conversation] = b
	}
}
func (a *Access) evidence(ctx context.Context, ref string) (Evidence, error) {
	parts := strings.SplitN(ref, ":", 3)
	if len(parts) != 3 || parts[0] != "record" && parts[0] != "turn" || parts[1] == "" || parts[2] == "" {
		return Evidence{}, invalidQuery("invalid saved reference; use an exact reference returned by history search or a saved tool result")
	}
	a.mu.Lock()
	bound, ok := a.bounds[parts[1]]
	a.mu.Unlock()
	if !ok {
		return Evidence{}, errors.New("reference is outside captured history")
	}
	r, err := a.store.ResolveRecord(ctx, a.workspace, parts[1], ref, bound)
	if err != nil {
		return Evidence{}, err
	}
	return a.store.projectRecord(ctx, a.workspace, parts[1], r, bound.Sequence)
}
func (s *Store) projectRecord(ctx context.Context, w, id string, r StoredRecord, bound uint64) (Evidence, error) {
	e := Evidence{Delivery: observation.Delivery{Ref: r.Ref}, Conversation: id, Turn: r.TurnID, Sequence: r.Sequence}
	switch r.Stream {
	case "reasoning":
		var entry ReasoningEntry
		if json.Unmarshal(r.Data, &entry) != nil || entry.Projection == nil {
			return e, errors.New("record has no projected evidence")
		}
		if err := entry.Projection.Validate(); err != nil {
			return e, err
		}
		e.Role, e.Status, e.Time, e.Observation = "tool", entry.Status, entry.Projection.ObservedAt, entry.Projection
	case "conversation":
		var entry ConversationRecord
		if err := json.Unmarshal(r.Data, &entry); err != nil {
			return e, err
		}
		if entry.Role == "user" {
			e.Role, e.Text, e.Time, e.Status = "user", entry.Text, entry.Time, entry.Status
			return e, nil
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		h, err := s.conversationHistory(w, id)
		if err != nil {
			return e, err
		}
		turn, err := h.loadTurn(r.TurnID)
		if err != nil {
			return e, err
		}
		if turn.EndSequence == 0 || turn.EndSequence > bound {
			return e, errors.New("assistant response has not settled within captured history")
		}
		e.Role, e.Text, e.Time, e.Status, e.Ref, e.Sequence = "assistant", turn.Response.Text, turn.Response.Updated, turn.Response.Status, turn.AnswerRef, turn.EndSequence
	default:
		return e, errors.New("record is not searchable evidence")
	}
	return e, nil
}
func (a *Access) Call(ctx context.Context, q HistoryRequest, maxBytes int) (HistoryReply, error) {
	if err := ctx.Err(); err != nil {
		return HistoryReply{}, err
	}
	if err := q.validate(); err != nil {
		return HistoryReply{}, err
	}
	if (q.Operation == "search" || q.Operation == "window") && q.Limit == 0 {
		q.Limit = DefaultPageResults
	}
	if q.Limit < 0 || q.Offset < 0 || maxBytes < 1 || (q.Operation == "search" || q.Operation == "window") && q.Limit > MaxPageResults {
		return HistoryReply{}, invalidQuery("invalid history page")
	}
	out := HistoryReply{NextWindow: -1}
	switch q.Operation {
	case "search":
		if a.index == nil {
			out.Search = &SearchResults{Next: -1, Semantic: "unavailable", Notice: "Search index is unavailable. Empty results do not establish absence. Known saved references remain readable."}
			return out, nil
		}
		a.mu.Lock()
		bounds := make(map[string]RecordBoundary, len(a.bounds))
		for id, b := range a.bounds {
			bounds[id] = b
		}
		a.mu.Unlock()
		hits, err := a.index.Search(ctx, q.Query, q.Offset, q.Limit, bounds)
		if err != nil {
			return out, err
		}
		out.Search = &hits
	case "read":
		e, err := a.evidence(ctx, q.Ref)
		if err != nil {
			return out, err
		}
		if e.Observation != nil {
			whole := e.Observation
			e.Observation = nil
			wrapper, _ := json.Marshal(HistoryReply{NextWindow: -1, Evidence: []Evidence{e}})
			base, _ := json.Marshal(e.Delivery)
			page, err := whole.Deliver(e.Ref, observation.Selection{Offset: q.Offset, Limit: q.Limit, Fields: q.Fields}, maxBytes-(len(wrapper)-len(base)))
			if err != nil {
				return out, invalidQuery(err.Error())
			}
			e.Delivery = page
		} else {
			chunk, err := TextChunk(e.Ref, e.Text, q.Offset, MaxReadBytes)
			if err != nil {
				return out, err
			}
			e.Text = chunk.Text
			if chunk.Truncated {
				e.NextTextOffset = &chunk.NextOffset
			}
		}
		out.Evidence = []Evidence{e}
	case "window":
		e, err := a.evidence(ctx, q.Ref)
		if err != nil {
			return out, err
		}
		a.mu.Lock()
		bound := a.bounds[e.Conversation]
		a.mu.Unlock()
		offset := uint64(q.Offset)
		if offset == 0 {
			a.store.mu.Lock()
			h, err := a.store.conversationHistory(a.workspace, e.Conversation)
			if err == nil {
				var t Turn
				t, err = h.loadTurn(e.Turn)
				if err == nil {
					offset = t.Sequence - 1
				}
			}
			a.store.mu.Unlock()
			if err != nil {
				return out, err
			}
		}
		page, err := a.store.TurnPageAfter(ctx, a.workspace, e.Conversation, offset, min(q.Limit, 4), 65536)
		if err != nil {
			return out, err
		}
		for _, t := range page.Turns {
			if t.Sequence > bound.Sequence || t.EndSequence == 0 || t.EndSequence > bound.Sequence {
				break
			}
			for _, ref := range []string{t.UserRef, t.AnswerRef} {
				if ref == "" {
					continue
				}
				v, err := a.evidence(ctx, ref)
				if err != nil {
					return out, err
				}
				chunk, _ := TextChunk(v.Ref, v.Text, 0, MaxReadBytes)
				v.Text = chunk.Text
				if chunk.Truncated {
					v.NextTextOffset = &chunk.NextOffset
				}
				out.Evidence = append(out.Evidence, v)
			}
		}
		if page.HasNewer && page.After < bound.Sequence {
			out.NextWindow = int(page.After)
		}
	case "count", "compare":
		e, err := a.evidence(ctx, q.Ref)
		if err != nil {
			return out, err
		}
		if e.Observation == nil {
			return out, invalidQuery("calculation requires a projected observation")
		}
		r := e.Observation
		if (q.Field == "") != (q.Equals == nil) || q.Equals != nil && q.Equals.Unknown {
			return out, invalidQuery("field count requires a typed equals value")
		}
		c := Calculation{Refs: []string{e.Ref}, Complete: r.Coverage.Complete, Limitations: append([]string(nil), r.Coverage.Limitations...)}
		if q.Equals != nil {
			probe := observation.New("filter.v1", "validate", time.Now())
			probe.Facts["equals"] = *q.Equals
			if probe.Validate() != nil {
				return out, invalidQuery("equals must contain exactly one typed value")
			}
		}
		keys, unknown := matchingRows(r.Rows, q.Field, q.Equals)
		c.Observed, c.Unknown = len(keys), unknown
		for _, state := range keys {
			if state == 1 {
				c.Matched++
			}
		}
		c.Complete = c.Complete && unknown == 0

		if q.Operation == "compare" {
			other, err := a.evidence(ctx, q.Other)
			if err != nil {
				return out, err
			}
			if other.Observation == nil {
				return out, invalidQuery("comparison requires two observations")
			}
			right := other.Observation
			if r.Schema != right.Schema || r.Connection != right.Connection || r.Namespace != right.Namespace || r.Operation != right.Operation || len(r.Selection) == 0 || !maps.Equal(r.Selection, right.Selection) {
				return out, invalidQuery("comparison requires matching schema and scope")
			}
			c.Refs = append(c.Refs, other.Ref)
			c.Complete = c.Complete && right.Coverage.Complete
			seen, unknown := matchingRows(right.Rows, q.Field, q.Equals)
			c.Unknown += unknown
			c.Complete = c.Complete && unknown == 0
			for key, state := range seen {
				if state != 1 {
					continue
				}
				switch keys[key] {
				case 1:
					c.Shared++
				case 0:
					c.Added++
				}
			}
			for key, state := range keys {
				if state == 1 && seen[key] == 0 {
					c.Removed++
				}
			}

			c.Limitations = append(c.Limitations, right.Coverage.Limitations...)
		}
		if !c.Complete {
			c.Limitations = append(c.Limitations, "counts and differences cover observed rows only")
		}
		out.Calculation = &c
	default:
		return out, invalidQuery(fmt.Sprintf("unsupported history operation %q", q.Operation))
	}
	return out, nil
}

// A repeated identity is one subject. Conflicting or missing retained values
// cannot establish whether that subject matches the requested predicate.
func matchingRows(rows []observation.Row, field string, equals *observation.Value) (map[string]int, int) {
	states := map[string]int{}
	for _, row := range rows {
		state := 1
		if field != "" {
			value, found := row.Facts[field]
			state = 0
			if !found || value.Unknown {
				state = -1
			} else {
				left, _ := json.Marshal(value)
				right, _ := json.Marshal(equals)
				if string(left) == string(right) {
					state = 1
				}
			}
		}
		key := row.Identity.Key()
		if old, seen := states[key]; seen && old != state {
			state = -1
		}
		states[key] = state
	}
	unknown := 0
	for _, state := range states {
		if state == -1 {
			unknown++
		}
	}
	return states, unknown
}
