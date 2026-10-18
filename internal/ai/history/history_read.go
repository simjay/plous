package history

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	bolt "go.etcd.io/bbolt"
)

type StoredRecord struct {
	ID       string          `json:"id"`
	TurnID   string          `json:"turn_id"`
	Stream   string          `json:"stream"`
	Ref      string          `json:"ref"`
	Sequence uint64          `json:"sequence,string"`
	Data     json.RawMessage `json:"data"`
}
type HistoryPage struct {
	Turns    []Turn
	Before   uint64
	After    uint64
	HasOlder bool
	HasNewer bool
}

type TurnOutcome struct {
	Status      string
	Closed      bool
	AnswerRef   string
	EndSequence uint64
}

// TurnOutcome reports acknowledged durable closure, even when the writer later
// failed after syncing its closure. An unconfirmed append is never closed.
func (s *Store) TurnOutcome(w, id, turn string) (TurnOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return TurnOutcome{}, ErrClosed
	}
	h, err := s.conversationHistory(w, id)
	if h == nil {
		return TurnOutcome{}, err
	}
	if err != nil && h.poisoned == nil {
		return TurnOutcome{}, err
	}
	t, err := h.turn(turn)
	if err != nil {
		return TurnOutcome{}, err
	}
	result := TurnOutcome{Status: t.Status, Closed: t.EndSequence != 0, EndSequence: t.EndSequence}
	if t.AnswerID != "" {
		result.AnswerRef = stableRecordRef(id, t.AnswerID)
	}
	return result, nil
}

func (h *historyState) location(id string) (recordLocation, error) {
	var loc recordLocation
	err := h.db.View(func(tx *bolt.Tx) error {
		b := historyBucket(tx, h.manifest.Conversation)
		data := b.Bucket([]byte("records")).Get([]byte(id))
		if data == nil {
			return errors.New("saved record does not exist")
		}
		return json.Unmarshal(data, &loc)
	})
	return loc, err
}
func (h *historyState) readLocation(ctx context.Context, loc recordLocation) (StoredRecord, error) {
	var result StoredRecord
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if loc.Length < 2 || loc.Length > MaxRecordBytes || loc.Offset < 0 {
		return result, errors.New("invalid saved record location")
	}
	path, err := h.path(loc.Stream, loc.Shard)
	if err != nil {
		return result, err
	}
	f, err := openRecordFile(path, false)
	if err != nil {
		return result, err
	}
	defer f.Close()
	data := make([]byte, loc.Length)
	if _, err = f.ReadAt(data, loc.Offset); err != nil {
		return result, err
	}
	if data[len(data)-1] != '\n' {
		return result, errors.New("saved record is incomplete")
	}
	data = data[:len(data)-1]
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != loc.Digest {
		return result, errors.New("saved record checksum does not match its lookup")
	}
	var e historyEnvelope
	if err = json.Unmarshal(data, &e); err != nil {
		return result, err
	}
	if e.ID != loc.ID || e.TurnID != loc.TurnID || e.Sequence != loc.Sequence || e.Stream != loc.Stream || e.Log != h.manifest.Log || e.Conversation != h.manifest.Conversation {
		return result, errors.New("saved record identity does not match its lookup")
	}
	return StoredRecord{ID: e.ID, TurnID: e.TurnID, Stream: e.Stream, Ref: stableRecordRef(e.Conversation, e.ID), Sequence: e.Sequence, Data: data}, nil
}
func (h *historyState) readID(ctx context.Context, id string) (StoredRecord, error) {
	loc, err := h.location(id)
	if err != nil {
		return StoredRecord{}, err
	}
	return h.readLocation(ctx, loc)
}
func (s *Store) ResolveRecord(ctx context.Context, w, id, ref string, boundary RecordBoundary) (StoredRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, err := s.conversationHistory(w, id)
	if err != nil {
		return StoredRecord{}, err
	}
	var recordID string
	parts := strings.SplitN(ref, ":", 3)
	if len(parts) != 3 || parts[1] != id {
		return StoredRecord{}, errors.New("saved reference is outside the conversation")
	}
	switch parts[0] {
	case "record":
		recordID = parts[2]
	case "turn":
		t, err := h.turn(parts[2])
		if err != nil {
			return StoredRecord{}, err
		}
		recordID = t.UserID
	default:
		return StoredRecord{}, errors.New("invalid saved reference")
	}
	if recordID == "" {
		return StoredRecord{}, errors.New("saved reference does not exist")
	}
	loc, err := h.location(recordID)
	if err != nil {
		return StoredRecord{}, err
	}
	if loc.Sequence > boundary.Sequence {
		return StoredRecord{}, errors.New("saved reference exceeds captured history")
	}
	return h.readLocation(ctx, loc)
}
func (h *historyState) turnRecords(ctx context.Context, turn string, after, boundary uint64, limit, maxBytes int) ([]StoredRecord, uint64, error) {
	if limit < 1 || limit > 100 || maxBytes < 1 || maxBytes > 4*MaxRecordBytes {
		return nil, after, errors.New("invalid turn read limits")
	}
	if err := ctx.Err(); err != nil {
		return nil, after, err
	}
	prefix := append([]byte(turn), 0)
	locations, err := h.locations("turn_records", append(append([]byte(nil), prefix...), historyKey(after+1)...), prefix, limit+1)
	if err != nil {
		return nil, after, err
	}
	var result []StoredRecord
	size := 0
	next := after
	for _, loc := range locations {
		if loc.Sequence > boundary {
			break
		}
		if len(result) == limit || len(result) > 0 && size+int(loc.Length) > maxBytes {
			return result, next, nil
		}
		r, err := h.readLocation(ctx, loc)
		if err != nil {
			return nil, after, err
		}
		result = append(result, r)
		size += int(loc.Length)
		next = loc.Sequence
	}
	return result, boundary, nil
}
func (s *Store) ReadTurnRecords(ctx context.Context, w, id, turn string, after, maxSequence uint64, limit, maxBytes int) ([]StoredRecord, uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, err := s.conversationHistory(w, id)
	if err != nil {
		return nil, after, err
	}
	return h.turnRecords(ctx, turn, after, min(maxSequence, h.meta.Sequence), limit, maxBytes)
}

func (h *historyState) loadTurn(id string) (Turn, error) {
	t, err := h.turn(id)
	if err != nil {
		return Turn{}, err
	}
	r, err := h.readID(context.Background(), t.RequestID)
	if err != nil {
		return Turn{}, err
	}
	var request ReasoningEntry
	if err = json.Unmarshal(r.Data, &request); err != nil || request.Request == nil {
		return Turn{}, errors.New("saved request is missing")
	}
	result := Turn{Request: *request.Request, Sequence: t.Sequence, EndSequence: t.EndSequence, UserRef: stableRecordRef(h.manifest.Conversation, t.UserID), Response: Response{Status: "streaming", Updated: request.Time}}
	if t.AnswerID != "" {
		r, err := h.readID(context.Background(), t.AnswerID)
		if err != nil {
			return Turn{}, err
		}
		var message ConversationRecord
		if err = json.Unmarshal(r.Data, &message); err != nil {
			return Turn{}, err
		}
		result.Response = message.Response
		result.AnswerRef = r.Ref
	} else if h.active != nil && h.active.Request.ID == id {
		result.Response = h.active.Response
	} else {
		var after uint64
		for after < h.meta.Sequence {
			records, next, err := h.turnRecords(context.Background(), id, after, h.meta.Sequence, 100, 4*MaxRecordBytes)
			if err != nil {
				return Turn{}, err
			}
			for _, r := range records {
				if r.Stream != "conversation" {
					continue
				}
				var message ConversationRecord
				if err = json.Unmarshal(r.Data, &message); err != nil {
					return Turn{}, err
				}
				if message.Role == "assistant" {
					if message.Delta {
						message.Text = result.Response.Text + message.Text
					}
					result.Response = message.Response
				}
			}
			after = next
		}
	}
	result.Response.Status = t.Status
	return result, nil
}
func (h *historyState) orderedTurns(anchor uint64, reverse bool, limit int) ([]turnLocation, error) {
	return h.orderedTurnIndex("order", anchor, reverse, limit)
}
func (h *historyState) orderedTurnIndex(index string, anchor uint64, reverse bool, limit int) ([]turnLocation, error) {
	var result []turnLocation
	err := h.db.View(func(tx *bolt.Tx) error {
		b := historyBucket(tx, h.manifest.Conversation)
		c := b.Bucket([]byte(index)).Cursor()
		var k, v []byte
		if reverse {
			if anchor == 0 {
				k, v = c.Last()
			} else {
				k, _ = c.Seek(historyKey(anchor))
				if k == nil {
					k, v = c.Last()
				} else {
					k, v = c.Prev()
				}
			}
		} else {
			k, v = c.Seek(historyKey(anchor + 1))
		}
		for k != nil && len(result) < limit {
			var t turnLocation
			if err := json.Unmarshal(b.Bucket([]byte("turns")).Get(v), &t); err != nil {
				return err
			}
			result = append(result, t)
			if reverse {
				k, v = c.Prev()
			} else {
				k, v = c.Next()
			}
		}
		return nil
	})
	return result, err
}
func (h *historyState) page(ctx context.Context, anchor uint64, reverse bool, limit, maxBytes int) (HistoryPage, error) {
	var page HistoryPage
	if limit < 1 || limit > 100 || maxBytes < 1 || maxBytes > 8*MaxRecordBytes {
		return page, errors.New("invalid conversation read limits")
	}
	locations, err := h.orderedTurns(anchor, reverse, limit+1)
	if err != nil {
		return page, err
	}
	size := 0
	for _, location := range locations {
		if err := ctx.Err(); err != nil {
			return page, err
		}
		if len(page.Turns) >= limit {
			break
		}
		turn, err := h.loadTurn(location.ID)
		if err != nil {
			return page, err
		}
		n := TurnBytes(turn)
		if size+n > maxBytes && len(page.Turns) > 0 {
			break
		}

		page.Turns = append(page.Turns, turn)
		size += n
	}
	sort.Slice(page.Turns, func(i, j int) bool { return page.Turns[i].Sequence < page.Turns[j].Sequence })
	if len(page.Turns) > 0 {
		page.Before = page.Turns[0].Sequence
		page.After = page.Turns[len(page.Turns)-1].Sequence
		older, e := h.orderedTurns(page.Before, true, 1)
		if e != nil {
			return page, e
		}
		newer, e := h.orderedTurns(page.After, false, 1)
		if e != nil {
			return page, e
		}
		page.HasOlder = len(older) > 0
		page.HasNewer = len(newer) > 0
	}
	return page, nil
}
func (s *Store) TurnPage(ctx context.Context, w, id string, before uint64, limit, maxBytes int) (HistoryPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, err := s.conversationHistory(w, id)
	if err != nil {
		return HistoryPage{}, err
	}
	return h.page(ctx, before, true, limit, maxBytes)
}
func (s *Store) TurnPageAfter(ctx context.Context, w, id string, after uint64, limit, maxBytes int) (HistoryPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, err := s.conversationHistory(w, id)
	if err != nil {
		return HistoryPage{}, err
	}
	return h.page(ctx, after, false, limit, maxBytes)
}
func (s *Store) Turns(w, id string) ([]Turn, error) {
	var result []Turn
	var after uint64
	for {
		p, err := s.TurnPageAfter(context.Background(), w, id, after, 100, 8*MaxRecordBytes)
		if err != nil {
			return nil, err
		}
		result = append(result, p.Turns...)
		if !p.HasNewer {
			return result, nil
		}
		after = p.After
	}
}
func (s *Store) RecordBoundaries(w, current string) ([]RecordBoundary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrClosed
	}
	conversations, err := s.conversations(w)
	if err != nil {
		return nil, err
	}
	var currentConversation Conversation
	for _, c := range conversations {
		if c.ID == current {
			currentConversation = c
		}
	}
	if currentConversation.ID == "" {
		return nil, errors.New("conversation does not exist")
	}
	var result []RecordBoundary
	for _, c := range conversations {
		if w == "" && c.ID != current {
			continue
		}
		h, err := s.conversationHistory(w, c.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, RecordBoundary{Conversation: c, Sequence: h.meta.Sequence})
	}
	return result, nil
}

// ReadRecords uses exclusive sequence cursors, not byte positions. Exhaustion advances to boundary even for a sparse stream.
func (s *Store) ReadRecords(ctx context.Context, w, id, source string, offset, boundary int64, limit, maxBytes int) ([]RecordLine, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if source != "conversation" && source != "reasoning" && source != "both" {
		return nil, offset, errors.New("unsupported record source")
	}
	if offset < 0 || boundary < offset || limit < 1 || limit > 100 || maxBytes < 1 || maxBytes > 4*MaxRecordBytes {
		return nil, offset, errors.New("invalid record read limits")
	}
	if err := ctx.Err(); err != nil {
		return nil, offset, err
	}
	h, err := s.conversationHistory(w, id)
	if err != nil {
		return nil, offset, err
	}
	if uint64(boundary) > h.meta.Sequence {
		return nil, offset, errors.New("record boundary exceeds saved data")
	}
	var result []RecordLine
	size := 0
	next := uint64(offset)
	for next < uint64(boundary) {
		if err := ctx.Err(); err != nil {
			return nil, offset, err
		}
		locations, err := h.locations("sequences", historyKey(next+1), nil, 100)
		if err != nil {
			return nil, offset, err
		}
		if len(locations) == 0 {
			break
		}
		for _, loc := range locations {
			if loc.Sequence > uint64(boundary) {
				return result, boundary, nil
			}
			if loc.Stream != source && !(source == "both" && (loc.Stream == "conversation" || loc.Stream == "reasoning")) {
				next = loc.Sequence
				continue
			}
			if len(result) >= limit || len(result) > 0 && size+int(loc.Length) > maxBytes {
				return result, int64(next), nil
			}
			r, err := h.readLocation(ctx, loc)
			if err != nil {
				return nil, offset, err
			}
			result = append(result, RecordLine{Offset: int64(loc.Sequence), Sequence: loc.Sequence, ID: loc.ID, Ref: r.Ref, Data: r.Data})
			size += int(loc.Length)
			next = loc.Sequence
		}
	}
	return result, boundary, nil
}

// RepairHistoryLookups reconstructs locations from the immutable source files. It does not rewrite messages.
func (s *Store) RepairHistoryLookups(w string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	prefix := w + ":"
	for key, h := range s.histories {
		if strings.HasPrefix(key, prefix) && h.meta.Active != "" {
			return errors.New("finish the running response before repairing history")
		}
	}
	root, err := s.historyRoot(w)
	if err != nil {
		return err
	}
	conversations, err := s.conversations(w)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(root, ".pending-lookup-")
	if err != nil {
		return err
	}
	temporary := f.Name()
	if err = f.Close(); err != nil {
		return err
	}
	defer os.Remove(temporary)
	db, err := bolt.Open(temporary, 0600, nil)
	if err != nil {
		return err
	}
	candidate := &historyDB{db}
	old := s.historyDBs[root]
	if s.historyDBs == nil {
		s.historyDBs = map[string]*historyDB{}
	}
	s.historyDBs[root] = candidate
	saved := map[string]*historyState{}
	for key, h := range s.histories {
		if strings.HasPrefix(key, prefix) {
			saved[key] = h
			delete(s.histories, key)
		}
	}
	published := false
	defer func() {
		if !published {
			candidate.Close()
			if old == nil {
				delete(s.historyDBs, root)
			} else {
				s.historyDBs[root] = old
			}
			for key := range s.histories {
				if strings.HasPrefix(key, prefix) {
					delete(s.histories, key)
				}
			}
			for key, h := range saved {
				s.histories[key] = h
			}
		}
	}()
	// The replacement is unpublished derived data. A failed repair leaves the old lookup available.
	candidate.NoSync = true
	for _, c := range conversations {
		if _, err = s.conversationHistory(w, c.ID); err != nil {
			return err
		}
	}
	candidate.NoSync = false
	if err = candidate.Sync(); err != nil {
		return err
	}
	path := filepath.Join(root, "history-lookup.db")
	if err = os.Rename(temporary, path); err != nil {
		return err
	}
	if err = syncDir(root); err != nil {
		return err
	}
	published = true
	if old != nil {
		return old.Close()
	}
	return nil
}

func TurnBytes(turn Turn) int { data, _ := json.Marshal(turn); return len(data) }

func (s *Store) TurnStatus(w, id, turn string, maxSequence uint64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, err := s.conversationHistory(w, id)
	if err != nil {
		return "", err
	}
	t, err := h.turn(turn)
	if err != nil {
		return "", err
	}
	if t.Sequence > maxSequence {
		return "", errors.New("turn exceeds captured history")
	}
	if t.EndSequence == 0 || t.EndSequence > maxSequence {
		return "streaming", nil
	}
	return t.Status, nil
}
