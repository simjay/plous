package history

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

const HistoryFormat = 2
const HistoryTurnBytes = 64 << 20
const HistoryAnswerReserve = 8 << 20
const HistoryClosureReserve = 16 << 10
const HistoryShardBytes = 256 << 20

var errMissingTurn = errors.New("conversation turn does not exist")

var ErrHistoryBudget = errors.New("investigation storage limit reached; finish with the saved evidence")
var historyStreams = []string{"conversation", "reasoning", "execution"}

type historyDB struct{ *bolt.DB }
type historyShard struct {
	Number int               `json:"number"`
	Sealed bool              `json:"sealed"`
	Sizes  historyOffsets    `json:"sizes"`
	Hashes map[string]string `json:"hashes,omitempty"`
	First  uint64            `json:"first_sequence,string"`
	Last   uint64            `json:"last_sequence,string"`
}
type historyManifest struct {
	Version      int                       `json:"version"`
	Conversation string                    `json:"conversation_id"`
	Log          string                    `json:"log_id"`
	Streams      map[string][]historyShard `json:"streams,omitempty"`
}
type historyMeta struct {
	Log       string         `json:"log_id"`
	Sequence  uint64         `json:"sequence,string"`
	Offsets   historyOffsets `json:"offsets"`
	Active    string         `json:"active,omitempty"`
	Positions map[string]int `json:"positions,omitempty"`
}
type historyState struct {
	dir         string
	manifest    historyManifest
	db          *historyDB
	meta        historyMeta
	active      *Turn
	poisoned    error
	shardTarget int64
	openAppend  func(string) (historyAppendFile, error)
}

type historyAppendFile interface {
	recordFileReader
	WriteAt([]byte, int64) (int, error)
	Truncate(int64) error
	Sync() error
	Close() error
}

func (h *historyState) appendFile(path string) (historyAppendFile, error) {
	if h.openAppend != nil {
		return h.openAppend(path)
	}
	return openRecordFile(path, true)
}

type recordLocation struct {
	ID       string `json:"id"`
	TurnID   string `json:"turn_id"`
	Stream   string `json:"stream"`
	Shard    int    `json:"shard"`
	Offset   int64  `json:"offset,string"`
	Length   int64  `json:"length,string"`
	Sequence uint64 `json:"sequence,string"`
	Digest   string `json:"digest"`
}
type turnLocation struct {
	ID          string `json:"id"`
	Sequence    uint64 `json:"sequence,string"`
	EndSequence uint64 `json:"end_sequence,string"`
	RequestID   string `json:"request_id"`
	UserID      string `json:"user_id"`
	AnswerID    string `json:"answer_id"`
	Status      string `json:"status"`
	Bytes       int64  `json:"bytes"`
}
type historyEnvelope struct {
	Version      int            `json:"version"`
	Log          string         `json:"log_id"`
	Conversation string         `json:"conversation_id"`
	Sequence     uint64         `json:"sequence,string"`
	Stream       string         `json:"stream"`
	ID           string         `json:"id"`
	TurnID       string         `json:"turn_id"`
	Kind         string         `json:"kind,omitempty"`
	Role         string         `json:"role,omitempty"`
	Status       string         `json:"status"`
	AnswerID     string         `json:"answer_id,omitempty"`
	Boundaries   historyOffsets `json:"boundaries,omitempty"`
}

func historyKey(n uint64) []byte                     { b := make([]byte, 8); binary.BigEndian.PutUint64(b, n); return b }
func stableRecordRef(conversation, id string) string { return "record:" + conversation + ":" + id }
func shardName(n int) string                         { return fmt.Sprintf("%06d.jsonl", n) }
func emptyOffsets() map[string]int64 {
	return map[string]int64{"conversation": 0, "reasoning": 0, "execution": 0}
}
func createHistoryFiles(dir, id string) error {
	m := historyManifest{Version: HistoryFormat, Conversation: id, Log: newID(), Streams: map[string][]historyShard{}}
	for _, stream := range []string{"conversation", "reasoning", "execution"} {
		m.Streams[stream] = []historyShard{{Number: 1, Sizes: historyOffsets{stream: 0}, First: 1}}
		if err := privateDir(filepath.Join(dir, stream)); err != nil {
			return err
		}
		if err := writeFile(filepath.Join(dir, stream, shardName(1)), nil); err != nil {
			return err
		}
		if err := syncDir(filepath.Join(dir, stream)); err != nil {
			return err
		}
	}
	return atomicHistoryJSON(filepath.Join(dir, "manifest.json"), m)
}
func atomicHistoryJSON(path string, value any) error {
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return errors.New("history metadata must be a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".pending-history-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(append(data, '\n')); err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}
func (s *Store) historyRoot(w string) (string, error) {
	dir, err := s.chatDir(w)
	if err != nil {
		return "", err
	}
	return filepath.Dir(dir), nil
}
func (s *Store) HistoryRoot(w string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return "", ErrClosed
	}
	return s.historyRoot(w)
}
func (s *Store) historyDB(w string) (*historyDB, error) {
	root, err := s.historyRoot(w)
	if err != nil {
		return nil, err
	}
	if db := s.historyDBs[root]; db != nil {
		return db, nil
	}
	path := filepath.Join(root, "history-lookup.db")
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return nil, errors.New("history lookup must be a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, fmt.Errorf("open history lookup; repair is required: %w", err)
	}
	if s.historyDBs == nil {
		s.historyDBs = map[string]*historyDB{}
	}
	result := &historyDB{db}
	s.historyDBs[root] = result
	return result, nil
}
func historyBucket(tx *bolt.Tx, id string) *bolt.Bucket { return tx.Bucket([]byte(id)) }
func initHistoryBucket(tx *bolt.Tx, id string) (*bolt.Bucket, error) {
	b, err := tx.CreateBucketIfNotExists([]byte(id))
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"records", "sequences", "turns", "order", "completed_order", "turn_records"} {
		if _, err = b.CreateBucketIfNotExists([]byte(name)); err != nil {
			return nil, err
		}
	}
	return b, nil
}
func putHistoryJSON(b *bolt.Bucket, key []byte, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return b.Put(key, data)
}
func (s *Store) conversationHistory(w, id string) (*historyState, error) {
	if s.closed {
		return nil, ErrClosed
	}
	key := w + ":" + id
	if h := s.histories[key]; h != nil {
		return h, h.poisoned
	}
	dir, err := s.conversationDir(w, id)
	if err != nil {
		return nil, err
	}
	h := &historyState{dir: dir, shardTarget: HistoryShardBytes}
	if err = readJSON(filepath.Join(dir, "manifest.json"), &h.manifest); err != nil {
		return nil, err
	}
	if h.manifest.Version != HistoryFormat || h.manifest.Conversation != id || !idPattern.MatchString(h.manifest.Log) || len(h.manifest.Streams) != len(historyStreams) {
		return nil, errors.New("unsupported conversation storage format")
	}
	for _, stream := range historyStreams {
		if len(h.manifest.Streams[stream]) == 0 {
			return nil, errors.New("missing history stream")
		}
	}
	for stream, shards := range h.manifest.Streams {
		if len(shards) == 0 {
			return nil, errors.New("history stream has no shards")
		}
		for i, g := range shards {
			if g.Number != i+1 || g.Sealed != (i < len(shards)-1) {
				return nil, errors.New("invalid conversation stream shard membership")
			}
			if _, err = h.path(stream, g.Number); err != nil {
				return nil, err
			}
		}
	}
	h.db, err = s.historyDB(w)
	if err != nil {
		return nil, err
	}
	err = h.db.Update(func(tx *bolt.Tx) error {
		if existing := historyBucket(tx, id); existing != nil && existing.Get([]byte("meta")) != nil {
			for _, name := range []string{"records", "sequences", "turns", "order", "completed_order", "turn_records"} {
				if existing.Bucket([]byte(name)) == nil {
					return errors.New("history lookup is incomplete; repair is required")
				}
			}
		}
		b, e := initHistoryBucket(tx, id)
		if e != nil {
			return e
		}
		if data := b.Get([]byte("meta")); data != nil {
			if e = json.Unmarshal(data, &h.meta); e != nil {
				return e
			}
		}
		if h.meta.Log != h.manifest.Log {
			if e = tx.DeleteBucket([]byte(id)); e != nil {
				return e
			}
			if _, e = initHistoryBucket(tx, id); e != nil {
				return e
			}
			h.meta = historyMeta{Log: h.manifest.Log, Offsets: emptyOffsets(), Positions: map[string]int{}}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if s.histories == nil {
		s.histories = map[string]*historyState{}
	}
	s.histories[key] = h
	if err = h.recover(); err != nil {
		h.poisoned = err
		return nil, err
	}
	return h, nil
}
func (h *historyState) path(stream string, shard int) (string, error) {
	if stream != "conversation" && stream != "reasoning" && stream != "execution" {
		return "", errors.New("invalid history stream")
	}
	count := len(h.manifest.Streams[stream])
	if shard < 1 || shard > count {
		return "", errors.New("invalid history shard")
	}
	dir := filepath.Join(h.dir, stream)
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		return "", errors.New("history stream must be a directory")
	}
	return filepath.Join(dir, shardName(shard)), nil
}
func (h *historyState) turn(id string) (turnLocation, error) {
	var t turnLocation
	err := h.db.View(func(tx *bolt.Tx) error {
		data := historyBucket(tx, h.manifest.Conversation).Bucket([]byte("turns")).Get([]byte(id))
		if data == nil {
			return errMissingTurn
		}
		return json.Unmarshal(data, &t)
	})
	return t, err
}
func (h *historyState) apply(data []byte, loc recordLocation) error {
	var e historyEnvelope
	if err := json.Unmarshal(data, &e); err != nil {
		return err
	}
	if e.Version != HistoryFormat || e.Conversation != h.manifest.Conversation || e.Log != h.manifest.Log || e.Sequence != h.meta.Sequence+1 || e.Stream != loc.Stream || !idPattern.MatchString(e.ID) || !idPattern.MatchString(e.TurnID) {
		return errors.New("invalid history identity or append sequence")
	}
	var timestamp struct {
		Time time.Time `json:"time"`
	}
	if err := json.Unmarshal(data, &timestamp); err != nil || timestamp.Time.IsZero() {
		return errors.New("history entry has no timestamp")
	}
	if e.Stream == "conversation" {
		if (e.Role != "user" && e.Role != "assistant" && e.Kind != "turn_end") || (e.Kind == "turn_end" && e.Role != "") {
			return errors.New("invalid conversation entry")
		}
		if e.Role == "user" {
			t, err := h.turn(e.TurnID)
			if err != nil {
				return err
			}
			saved, err := h.readID(context.Background(), t.RequestID)
			if err != nil {
				return err
			}
			var request ReasoningEntry
			var message ConversationRecord
			if err = json.Unmarshal(saved.Data, &request); err != nil {
				return err
			}
			if err = json.Unmarshal(data, &message); err != nil {
				return err
			}
			if request.Request == nil || request.Request.ID != e.TurnID || request.Request.Prompt != message.Text {
				return errors.New("question does not match its saved request")
			}
		}
	} else {
		var entry ReasoningEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			return err
		}
		if entry.MessageID != e.TurnID || entry.Kind == "" || entry.Status == "" {
			return errors.New("execution entry has an invalid turn association")
		}
		if e.Stream == "reasoning" {
			if entry.Kind != "tool" || entry.Projection == nil || entry.Tool == "" || entry.Content != "" || len(entry.Messages) != 0 || entry.Request != nil {
				return errors.New("reasoning requires projected observations")
			}
			if err := entry.Projection.Validate(); err != nil {
				return err
			}
			if entry.Status != entry.Projection.Outcome {
				return errors.New("observation status does not match its outcome")
			}
		}
		if entry.Kind == "request" && (entry.Request == nil || entry.Request.ID != e.TurnID) {
			return errors.New("invalid saved request")
		}
	}
	loc.ID, loc.TurnID, loc.Sequence = e.ID, e.TurnID, e.Sequence
	sum := sha256.Sum256(data)
	loc.Digest = hex.EncodeToString(sum[:])
	meta := h.meta
	meta.Sequence = e.Sequence
	meta.Positions = map[string]int{}
	for k, v := range h.meta.Positions {
		meta.Positions[k] = v
	}
	meta.Positions[e.Stream] = loc.Shard
	meta.Offsets = map[string]int64{}
	for k, v := range h.meta.Offsets {
		meta.Offsets[k] = v
	}
	meta.Offsets[e.Stream] = loc.Offset + loc.Length
	err := h.db.Update(func(tx *bolt.Tx) error {
		b := historyBucket(tx, e.Conversation)
		records := b.Bucket([]byte("records"))
		if records.Get([]byte(e.ID)) != nil {
			return errors.New("duplicate history record ID")
		}
		turns := b.Bucket([]byte("turns"))
		var t turnLocation
		if d := turns.Get([]byte(e.TurnID)); d != nil {
			if err := json.Unmarshal(d, &t); err != nil {
				return err
			}
		}
		if t.ID == "" {
			if e.Kind != "request" {
				return errors.New("history request is missing")
			}
			if meta.Active != "" {
				return errors.New("overlapping history turns")
			}
			t = turnLocation{ID: e.TurnID, RequestID: e.ID, Status: "streaming"}
			meta.Active = e.TurnID
		}
		if t.EndSequence != 0 {
			return errors.New("history event follows turn closure")
		}
		t.Bytes += loc.Length
		switch {
		case e.Kind == "request":
			if t.RequestID != e.ID {
				return errors.New("duplicate turn request")
			}
		case e.Role == "user":
			if t.UserID != "" {
				return errors.New("duplicate user message")
			}
			t.UserID = e.ID
			t.Sequence = e.Sequence
			if err := b.Bucket([]byte("order")).Put(historyKey(e.Sequence), []byte(e.TurnID)); err != nil {
				return err
			}
		case e.Kind == "turn_end":
			if !validResponseStatus(e.Status) || e.Status == "streaming" || e.AnswerID == "" || e.AnswerID != t.AnswerID {
				return errors.New("invalid turn closing entry")
			}
			for _, stream := range []string{"reasoning", "execution"} {
				boundary, present := e.Boundaries[stream]
				if !present || boundary != h.meta.Offsets[stream] {
					return errors.New("turn closing boundary does not match durable history")
				}
			}
			for stream := range e.Boundaries {
				if stream != "reasoning" && stream != "execution" {
					return errors.New("invalid turn closing stream")
				}
			}
			t.EndSequence = e.Sequence
			t.Status = e.Status
			if e.Status == "completed" {
				if err := b.Bucket([]byte("completed_order")).Put(historyKey(t.Sequence), []byte(e.TurnID)); err != nil {
					return err
				}
			}
			meta.Active = ""
		case e.Role == "assistant":
			var r ConversationRecord
			if err := json.Unmarshal(data, &r); err != nil {
				return err
			}
			if !validResponseStatus(r.Status) || r.Delta && r.Status != "streaming" || t.AnswerID != "" {
				return errors.New("invalid assistant response sequence")
			}
			if !r.Delta && r.Status != "streaming" {
				t.AnswerID = e.ID
			}

		}
		if err := putHistoryJSON(records, []byte(e.ID), loc); err != nil {
			return err
		}
		if err := b.Bucket([]byte("sequences")).Put(historyKey(e.Sequence), []byte(e.ID)); err != nil {
			return err
		}
		if err := b.Bucket([]byte("turn_records")).Put(append(append([]byte(e.TurnID), 0), historyKey(e.Sequence)...), []byte(e.ID)); err != nil {
			return err
		}
		if err := putHistoryJSON(turns, []byte(e.TurnID), t); err != nil {
			return err
		}
		return putHistoryJSON(b, []byte("meta"), meta)
	})
	if err == nil {
		h.meta = meta
	}
	return err
}
func (h *historyState) append(stream string, value any, turn string, class int) (recordLocation, error) {
	var loc recordLocation
	if h.poisoned != nil {
		return loc, h.poisoned
	}
	data, err := json.Marshal(value)
	if err != nil {
		return loc, err
	}
	var obj map[string]json.RawMessage
	if err = json.Unmarshal(data, &obj); err != nil {
		return loc, err
	}
	for k, v := range map[string]any{"version": HistoryFormat, "log_id": h.manifest.Log, "conversation_id": h.manifest.Conversation, "sequence": fmt.Sprint(h.meta.Sequence + 1), "stream": stream, "turn_id": turn} {
		obj[k], _ = json.Marshal(v)
	}
	data, err = json.Marshal(obj)
	if err != nil {
		return loc, err
	}
	if len(data)+1 > MaxRecordBytes {
		return loc, errors.New("history entry exceeds 8 MiB")
	}
	used := int64(0)
	if t, e := h.turn(turn); e == nil {
		used = t.Bytes
	} else if !errors.Is(e, errMissingTurn) {
		return loc, e
	}
	cap := int64(HistoryTurnBytes - HistoryAnswerReserve - HistoryClosureReserve)
	if class == 1 {
		cap = HistoryTurnBytes - HistoryClosureReserve
	}
	if class == 2 {
		cap = HistoryTurnBytes
	}
	if used+int64(len(data)+1) > cap {
		return loc, ErrHistoryBudget
	}
	if err := h.rotateStream(stream); err != nil {
		h.poisoned = fmt.Errorf("history rotation requires recovery: %w", err)
		return loc, h.poisoned
	}
	shard := len(h.manifest.Streams[stream])
	path, err := h.path(stream, shard)
	if err != nil {
		return loc, err
	}
	f, err := h.appendFile(path)
	if err != nil {
		return loc, err
	}
	offset := h.meta.Offsets[stream]
	info, err := f.Stat()
	if err == nil && info.Size() != offset {
		err = errors.New("history file changed outside its writer")
	}
	if err == nil {
		var n int
		n, err = f.WriteAt(append(data, '\n'), offset)
		if err == nil && n != len(data)+1 {
			err = io.ErrShortWrite
		}
	}
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		h.poisoned = fmt.Errorf("history write requires recovery: %w", err)
		return loc, h.poisoned
	}
	loc = recordLocation{Stream: stream, Shard: shard, Offset: offset, Length: int64(len(data) + 1)}
	if err = h.apply(data, loc); err != nil {
		h.poisoned = fmt.Errorf("history saved but lookup requires recovery: %w", err)
		return loc, h.poisoned
	}
	var e historyEnvelope
	_ = json.Unmarshal(data, &e)
	loc.ID, loc.TurnID, loc.Sequence = e.ID, e.TurnID, e.Sequence
	return loc, nil
}
func (h *historyState) recover() error { return h.recoverStreams() }
func (h *historyState) rotate() error {
	for stream := range h.manifest.Streams {
		if err := h.rotateStream(stream); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) StartTurn(w, id string, r Request) (Turn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.signalHistoryChange(w)
	if s.closing {
		return Turn{}, ErrClosed
	}
	h, err := s.conversationHistory(w, id)
	if err != nil {
		return Turn{}, err
	}
	if h.meta.Active != "" {
		return Turn{}, errors.New("the previous response is still running or closing")
	}
	r.ID, r.Time = chronologicalID(), time.Now().UTC()
	t := Turn{Request: r, Response: Response{Status: "streaming", Updated: r.Time}}
	entry := ReasoningEntry{ID: chronologicalID(), MessageID: r.ID, Kind: "request", Status: "completed", Time: r.Time, Model: r.Agent, Request: &r}
	messagePreview := ConversationRecord{ID: r.ID, TurnID: r.ID, Role: "user", Time: r.Time, Response: Response{Status: "completed", Text: r.Prompt, Updated: r.Time}}
	for _, value := range []any{entry, messagePreview} {
		data, err := json.Marshal(value)
		if err != nil {
			return Turn{}, err
		}
		if len(data)+1024 > MaxRecordBytes {
			return Turn{}, errors.New("question or captured settings exceed the saved entry limit")
		}
	}
	if _, err = h.append("execution", entry, r.ID, 0); err != nil {
		return t, err
	}
	record := ConversationRecord{ID: r.ID, TurnID: r.ID, Role: "user", Time: r.Time, Response: Response{Status: "completed", Text: r.Prompt, Updated: r.Time}}
	loc, err := h.append("conversation", record, r.ID, 0)
	if err != nil {
		return t, err
	}
	t.Sequence = loc.Sequence
	t.UserRef = stableRecordRef(id, loc.ID)
	h.active = &t
	return t, nil
}
func (s *Store) SaveResponse(w, id, turn string, r Response) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.signalHistoryChange(w)
	if s.closing {
		return ErrClosed
	}
	if !validResponseStatus(r.Status) || !idPattern.MatchString(turn) {
		return errors.New("invalid response identity or status")
	}
	h, err := s.conversationHistory(w, id)
	if err != nil {
		return err
	}
	if h.active == nil || h.active.Request.ID != turn {
		t, err := h.loadTurn(turn)
		if err != nil {
			return err
		}
		comparison := r
		comparison.Updated = t.Response.Updated
		if reflect.DeepEqual(t.Response, comparison) {
			return nil
		}
		return errors.New("a finished response cannot be overwritten")
	}
	if len(r.Text) > 1<<20 {
		return errors.New("answer exceeds its 1 MiB text limit")
	}
	if len(r.Error) > 16<<10 {
		r.Error = r.Error[:16<<10]
	}
	if r.Status != "streaming" {
		return h.finish(r)
	}
	comparison := r
	comparison.Updated = h.active.Response.Updated
	if reflect.DeepEqual(h.active.Response, comparison) {
		return nil
	}
	r.Updated = time.Now().UTC()
	record := ConversationRecord{ID: chronologicalID(), TurnID: turn, Role: "assistant", Time: r.Updated, Response: r}
	if strings.HasPrefix(r.Text, h.active.Response.Text) {
		record.Delta = true
		record.Text = strings.TrimPrefix(r.Text, h.active.Response.Text)
	}
	if _, err = h.append("conversation", record, turn, 0); err == nil {
		h.active.Response = r
	}
	return err
}
func (h *historyState) finish(r Response) error {
	turn := h.meta.Active
	if turn == "" {
		return errors.New("there is no active response")
	}
	t, err := h.turn(turn)
	if err != nil {
		return err
	}
	r.Updated = time.Now().UTC()
	if t.AnswerID == "" {
		answer := ConversationRecord{ID: chronologicalID(), TurnID: turn, Role: "assistant", Time: r.Updated, Response: r}
		loc, err := h.append("conversation", answer, turn, 1)
		if err != nil {
			return err
		}
		t.AnswerID = loc.ID
	}
	// Auxiliary entries were synced before publishing their lookup positions.
	// No producer can append while this closure holds the store lock.
	end := ConversationRecord{ID: chronologicalID(), Kind: "turn_end", TurnID: turn, AnswerID: t.AnswerID, Time: r.Updated, Response: Response{Status: r.Status, Updated: r.Updated}, Boundaries: h.completionBoundaries()}
	if _, err = h.append("conversation", end, turn, 2); err != nil {
		return err
	}
	h.active = nil
	if err = h.rotate(); err != nil {
		h.poisoned = err
	}
	return err
}
func (s *Store) AppendReasoning(w, id string, entry ReasoningEntry) (ReasoningEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.signalHistoryChange(w)
	if s.closing {
		return entry, ErrClosed
	}
	if strings.TrimSpace(entry.Kind) == "" || strings.TrimSpace(entry.Status) == "" || entry.Kind == "request" || entry.Request != nil || !idPattern.MatchString(entry.MessageID) {
		return entry, errors.New("invalid execution entry or turn")
	}
	h, err := s.conversationHistory(w, id)
	if err != nil {
		return entry, err
	}
	if h.meta.Active != entry.MessageID {
		return entry, errors.New("execution event does not belong to the active turn")
	}
	if entry.Projection == nil || entry.Kind != "tool" || entry.Tool == "" || entry.Content != "" || len(entry.Messages) != 0 {
		return entry, errors.New("reasoning accepts projected tool observations only")
	}
	if err := entry.Projection.Validate(); err != nil {
		return entry, err
	}
	if entry.Status != entry.Projection.Outcome {
		return entry, errors.New("observation status does not match its outcome")
	}
	entry.ID, entry.Time = chronologicalID(), time.Now().UTC()
	entry.TurnID = entry.MessageID
	loc, err := h.append("reasoning", entry, entry.MessageID, 0)
	if err == nil {
		entry.Ref = stableRecordRef(id, entry.ID)
		entry.Sequence = loc.Sequence
	}
	return entry, err
}

// These helpers keep lookup cursor buffers private to their bbolt transaction.
func (h *historyState) locations(bucket string, start, prefix []byte, limit int) ([]recordLocation, error) {
	var result []recordLocation
	err := h.db.View(func(tx *bolt.Tx) error {
		b := historyBucket(tx, h.manifest.Conversation)
		c := b.Bucket([]byte(bucket)).Cursor()
		k, v := c.Seek(start)
		for k != nil && len(result) < limit {
			if len(prefix) > 0 && !bytes.HasPrefix(k, prefix) {
				break
			}
			var loc recordLocation
			if err := json.Unmarshal(b.Bucket([]byte("records")).Get(v), &loc); err != nil {
				return err
			}
			result = append(result, loc)
			k, v = c.Next()
		}
		return nil
	})
	return result, err
}

type historyOffsets map[string]int64

func (o historyOffsets) MarshalJSON() ([]byte, error) {
	values := map[string]string{}
	for k, v := range o {
		values[k] = strconv.FormatInt(v, 10)
	}
	return json.Marshal(values)
}
func (o *historyOffsets) UnmarshalJSON(data []byte) error {
	var values map[string]string
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	result := historyOffsets{}
	for k, v := range values {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			return errors.New("invalid history offset")
		}
		result[k] = n
	}
	*o = result
	return nil
}

// Each subscriber receives coalesced commit notifications until the store closes.
func (s *Store) HistoryChanges(w string) (<-chan struct{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.closing {
		return nil, ErrClosed
	}
	if _, err := s.historyRoot(w); err != nil {
		return nil, err
	}
	if s.historyChanges == nil {
		s.historyChanges = map[string][]chan struct{}{}
	}
	changed := make(chan struct{}, 1)
	s.historyChanges[w] = append(s.historyChanges[w], changed)
	return changed, nil
}
func (s *Store) signalHistoryChange(w string) {
	if s.closing {
		return
	}
	for _, ch := range s.historyChanges[w] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
