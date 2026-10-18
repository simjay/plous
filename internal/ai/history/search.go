package history

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	bolt "go.etcd.io/bbolt"
)

type EmbedFunc func(context.Context, []string) ([][]float64, int64, error)
type SearchOptions struct {
	Monitor     bool
	EmbeddingID string
	Embed       EmbedFunc
}
type SearchIndex struct {
	store              *Store
	workspace, current string
	options            SearchOptions
	db                 *bolt.DB
	cancel             context.CancelFunc
	done               chan struct{}
	mu                 sync.Mutex
	update             sync.Mutex
	embedding          sync.Mutex
	err, embeddingErr  error
}
type SearchHit struct {
	Ref      string    `json:"ref"`
	Fragment int       `json:"fragment"`
	Role     string    `json:"role"`
	Status   string    `json:"status"`
	Time     time.Time `json:"time"`
	Text     string    `json:"text"`
	Score    float64   `json:"score"`
}
type SearchResults struct {
	Hits            []SearchHit `json:"hits"`
	LexicalComplete bool        `json:"lexical_complete"`
	Semantic        string      `json:"semantic"`
	Next            int         `json:"next"`
	Notice          string      `json:"notice"`
}
type fragment struct {
	SearchHit
	Conversation string    `json:"conversation"`
	Sequence     uint64    `json:"sequence"`
	Length       int       `json:"length"`
	Vector       []float64 `json:"vector,omitempty"`
}

func termRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.'
}
func terms(text string) map[string]int {
	result := map[string]int{}
	for _, term := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !termRune(r)
	}) {
		term = strings.Trim(term, ".")
		if len(term) > 0 && len(term) <= 256 {
			result[term]++
		}
	}
	return result
}
func (s *Store) SearchIndex(w, current string, o SearchOptions) (*SearchIndex, error) {
	s.searchMu.Lock()
	defer s.searchMu.Unlock()
	// Lexical readers share one index regardless of the current live capabilities.
	if o.Embed == nil {
		o.Monitor, o.EmbeddingID = false, ""
	}
	key := w + "\x00" + o.EmbeddingID + fmt.Sprint(o.Monitor)
	if w == "" {
		key += "\x00" + current
	}
	if x := s.searches[key]; x != nil {
		x.mu.Lock()
		x.options = o
		x.mu.Unlock()
		return x, nil
	}
	root, err := s.HistoryRoot(w)
	if err != nil {
		return nil, err
	}
	changes, err := s.HistoryChanges(w)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, "search")
	if err = privateDir(dir); err != nil {
		return nil, err
	}
	hash := sha256.Sum256([]byte("projected-search-v3\x00" + key))
	db, err := bolt.Open(filepath.Join(dir, fmt.Sprintf("%x.db", hash[:16])), 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, err
	}
	if err = db.Update(func(tx *bolt.Tx) error {
		for _, name := range []string{"fragments", "terms", "progress", "usage"} {
			if _, e := tx.CreateBucketIfNotExists([]byte(name)); e != nil {
				return e
			}
		}
		return nil
	}); err != nil {
		db.Close()
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	x := &SearchIndex{store: s, workspace: w, current: current, options: o, db: db, cancel: cancel, done: make(chan struct{})}
	if err = s.AddCloser(func() error { cancel(); <-x.done; return db.Close() }); err != nil {
		cancel()
		db.Close()
		return nil, err
	}
	if s.searches == nil {
		s.searches = map[string]*SearchIndex{}
	}
	s.searches[key] = x
	go x.background(ctx, changes)
	return x, nil
}
func (x *SearchIndex) background(ctx context.Context, changes <-chan struct{}) {
	defer close(x.done)
	embeddingDone := make(chan struct{})
	wake := make(chan struct{}, 1)
	go func() {
		defer close(embeddingDone)
		for {
			select {
			case <-ctx.Done():
				return
			case <-wake:
				err := x.embedPending(ctx)
				x.mu.Lock()
				x.embeddingErr = err
				x.mu.Unlock()
			}
		}
	}()
	defer func() { x.cancel(); <-embeddingDone }()
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		err := x.Sync(ctx)
		x.mu.Lock()
		x.err = err
		x.mu.Unlock()
		select {
		case wake <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			return
		case <-changes:
		case <-tick.C:
		}
	}
}

// Sync advances a durable replay cursor only after all eligible fragments commit.
func (x *SearchIndex) Sync(ctx context.Context) error {
	x.update.Lock()
	defer x.update.Unlock()
	bounds, err := x.store.RecordBoundaries(x.workspace, x.current)
	if err != nil {
		return err
	}
	for _, bound := range bounds {
		var after int64
		if err = x.db.View(func(tx *bolt.Tx) error {
			after, _ = strconv.ParseInt(string(tx.Bucket([]byte("progress")).Get([]byte(bound.Conversation.ID))), 10, 64)
			return nil
		}); err != nil {
			return err
		}
		for after < int64(bound.Sequence) {
			rows, next, err := x.store.ReadRecords(ctx, x.workspace, bound.Conversation.ID, "both", after, int64(bound.Sequence), 100, MaxRecordBytes)
			if err != nil {
				return err
			}
			var docs []Evidence
			for _, row := range rows {
				var entry struct {
					Kind, Role string
					Projection json.RawMessage
				}
				if err = json.Unmarshal(row.Data, &entry); err != nil {
					return err
				}
				stream := "conversation"
				if len(entry.Projection) > 0 && string(entry.Projection) != "null" {
					stream = "reasoning"
				} else if entry.Role != "user" && entry.Kind != "turn_end" {
					continue
				}
				record := StoredRecord{Ref: row.Ref, ID: row.ID, Sequence: row.Sequence, Stream: stream, Data: row.Data}
				var turn struct {
					TurnID string `json:"turn_id"`
				}
				_ = json.Unmarshal(row.Data, &turn)
				record.TurnID = turn.TurnID
				e, err := x.store.projectRecord(ctx, x.workspace, bound.Conversation.ID, record, bound.Sequence)
				if err != nil {
					return err
				}
				docs = append(docs, e)
			}
			err = x.db.Update(func(tx *bolt.Tx) error {
				for _, e := range docs {
					if err := indexEvidence(tx, e); err != nil {
						return err
					}
				}
				return tx.Bucket([]byte("progress")).Put([]byte(bound.Conversation.ID), []byte(strconv.FormatInt(next, 10)))
			})
			if err != nil {
				return err
			}
			after = next
		}
	}
	return nil
}
func indexEvidence(tx *bolt.Tx, e Evidence) error {
	text := e.Text
	if e.Observation != nil && e.Observation.Schema == "history.read.v1" {
		return nil
	}
	if e.Observation != nil {
		text = e.Observation.SearchText()
	}
	if text == "" {
		return nil
	}
	fragments := tx.Bucket([]byte("fragments"))
	index := tx.Bucket([]byte("terms"))
	for start := 0; start < len(text); {
		end := min(len(text), start+4096)
		for end < len(text) && !utf8.RuneStart(text[end]) {
			end--
		}
		// Keep searchable identifiers intact at fragment boundaries. Tokens longer
		// than 256 bytes are not indexed; they need not expand a fragment.
		if end < len(text) {
			next, _ := utf8.DecodeRuneInString(text[end:])
			if termRune(next) {
				boundary := end
				for boundary > start && end-boundary <= 256 {
					r, n := utf8.DecodeLastRuneInString(text[start:boundary])
					if !termRune(r) {
						break
					}
					boundary -= n
				}
				if boundary > start && end-boundary <= 256 {
					end = boundary
				}
			}
		}
		f := fragment{SearchHit: SearchHit{Ref: e.Ref, Fragment: start, Role: e.Role, Status: e.Status, Time: e.Time, Text: text[start:end]}, Conversation: e.Conversation, Sequence: e.Sequence}
		key := []byte(fmt.Sprintf("%s#%010d", e.Ref, start))
		if fragments.Get(key) != nil {
			start = end
			continue
		}
		words := terms(f.Text)
		for _, n := range words {
			f.Length += n
		}
		data, err := json.Marshal(f)
		if err != nil {
			return err
		}
		if err = fragments.Put(key, data); err != nil {
			return err
		}
		for word, count := range words {
			bucket, err := index.CreateBucketIfNotExists([]byte(word))
			if err != nil {
				return err
			}
			if err = bucket.Put(key, []byte(strconv.Itoa(count))); err != nil {
				return err
			}
		}
		start = end
	}
	return nil
}
func (x *SearchIndex) configuredOptions() SearchOptions {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.options
}

func (x *SearchIndex) embedPending(ctx context.Context) error {
	options := x.configuredOptions()
	x.embedding.Lock()
	defer x.embedding.Unlock()
	if !options.Monitor || options.Embed == nil {
		return nil
	}
	for {
		var keys []string
		var docs []fragment
		err := x.db.View(func(tx *bolt.Tx) error {
			return tx.Bucket([]byte("fragments")).ForEach(func(k, v []byte) error {
				if len(keys) >= 32 {
					return nil
				}
				var f fragment
				if err := json.Unmarshal(v, &f); err != nil {
					return err
				}
				if len(f.Vector) == 0 {
					keys = append(keys, string(k))
					docs = append(docs, f)
				}
				return ctx.Err()
			})
		})
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			return nil
		}
		texts := make([]string, len(docs))
		for i, d := range docs {
			texts[i] = d.Text
		}
		vectors, usage, err := options.Embed(ctx, texts)
		if err != nil {
			return err
		}
		if len(vectors) != len(keys) {
			return errors.New("embedding provider returned an invalid batch")
		}
		for i := range vectors {
			if !validVector(vectors[i]) {
				return errors.New("embedding provider returned an invalid vector")
			}
		}
		err = x.db.Update(func(tx *bolt.Tx) error {
			for i, key := range keys {
				docs[i].Vector = vectors[i]
				data, err := json.Marshal(docs[i])
				if err != nil {
					return err
				}
				if err = tx.Bucket([]byte("fragments")).Put([]byte(key), data); err != nil {
					return err
				}
			}
			b := tx.Bucket([]byte("usage"))
			n, _ := strconv.ParseInt(string(b.Get([]byte("tokens"))), 10, 64)
			return b.Put([]byte("tokens"), []byte(strconv.FormatInt(n+usage, 10)))
		})
		if err != nil {
			return err
		}
	}
}
func validVector(v []float64) bool {
	if len(v) == 0 || len(v) > 65536 {
		return false
	}
	norm := 0.
	for _, n := range v {
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return false
		}
		norm += n * n
	}
	return norm > 0 && !math.IsInf(norm, 0)
}
func cosine(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	dot, aa, bb := 0., 0., 0.
	for i := range a {
		dot += a[i] * b[i]
		aa += a[i] * a[i]
		bb += b[i] * b[i]
	}
	if aa == 0 || bb == 0 {
		return 0
	}
	return dot / math.Sqrt(aa*bb)
}
func (x *SearchIndex) Search(ctx context.Context, query string, offset, limit int, bounds map[string]RecordBoundary) (SearchResults, error) {
	out := SearchResults{Next: -1, LexicalComplete: true, Semantic: "not configured", Notice: "Search matches are not inventory counts or proof of absence."}
	if strings.TrimSpace(query) == "" || len(query) > 4096 || offset < 0 || limit < 1 || limit > 100 {
		return out, invalidQuery("invalid history search")
	}
	var vector []float64
	options := x.configuredOptions()
	if options.Monitor && options.Embed != nil {
		out.Semantic = "complete"
		vectors, usage, err := options.Embed(ctx, []string{query})
		if usage > 0 {
			if err := x.addUsage(usage); err != nil {
				return out, err
			}
		}
		if err != nil || len(vectors) != 1 || !validVector(vectors[0]) {
			out.Semantic = "unavailable"
		} else {
			vector = vectors[0]
		}
	}
	scores := map[string]float64{}
	docs := map[string]fragment{}
	semantic := map[string]float64{}
	err := x.db.View(func(tx *bolt.Tx) error {
		progress := tx.Bucket([]byte("progress"))
		for id, b := range bounds {
			n, _ := strconv.ParseUint(string(progress.Get([]byte(id))), 10, 64)
			if n < b.Sequence {
				out.LexicalComplete = false
			}
		}
		fragments := tx.Bucket([]byte("fragments"))
		N := float64(fragments.Stats().KeyN)
		load := func(key string) (fragment, bool) {
			if f, ok := docs[key]; ok {
				return f, true
			}
			var f fragment
			if json.Unmarshal(fragments.Get([]byte(key)), &f) != nil {
				out.LexicalComplete = false
				out.Semantic = "unavailable"
				return f, false
			}
			b, ok := bounds[f.Conversation]
			if !ok || f.Sequence > b.Sequence {
				return f, false
			}
			docs[key] = f
			return f, true
		}
		for term := range terms(query) {
			bucket := tx.Bucket([]byte("terms")).Bucket([]byte(term))
			if bucket == nil {
				continue
			}
			df := float64(bucket.Stats().KeyN)
			idf := math.Log(1 + (N-df+0.5)/(df+0.5))
			err := bucket.ForEach(func(k, v []byte) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				f, ok := load(string(k))
				if !ok {
					return nil
				}
				tf, _ := strconv.ParseFloat(string(v), 64)
				scores[string(k)] += idf * tf * 2.2 / (tf + 1.2*(0.25+0.75*float64(f.Length)/400))
				return nil
			})
			if err != nil {
				return err
			}
		}
		if len(vector) > 0 {
			return fragments.ForEach(func(k, v []byte) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				f, ok := load(string(k))
				if !ok {
					return nil
				}
				if len(f.Vector) == 0 {
					out.Semantic = "pending"
					return nil
				}
				if len(f.Vector) != len(vector) {
					out.Semantic = "incompatible"
					return nil
				}
				score := cosine(vector, f.Vector)
				if score > 0 {
					semantic[string(k)] = score
				}
				return nil
			})
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	// Reciprocal-rank fusion keeps lexical and semantic scales independent.
	rank := func(values map[string]float64) []string {
		keys := make([]string, 0, len(values))
		for k := range values {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if values[keys[i]] == values[keys[j]] {
				return keys[i] < keys[j]
			}
			return values[keys[i]] > values[keys[j]]
		})
		return keys
	}
	fused := map[string]float64{}
	for _, values := range []map[string]float64{scores, semantic} {
		for i, k := range rank(values) {
			fused[k] += 1 / float64(60+i+1)
		}
	}
	keys := rank(fused)
	if offset > len(keys) {
		return out, invalidQuery("search offset exceeds matches")
	}
	end := min(len(keys), offset+limit)
	bytes := 0
	for i, key := range keys[offset:end] {
		hit := docs[key].SearchHit
		hit.Score = fused[key]
		encoded, _ := json.Marshal(hit)
		if bytes+len(encoded) > 60000 {
			end = offset + i
			break
		}
		bytes += len(encoded)
		out.Hits = append(out.Hits, hit)
	}
	if !out.LexicalComplete && out.Semantic == "complete" {
		out.Semantic = "pending"
	}
	if end < len(keys) {
		out.Next = end
	}
	x.mu.Lock()
	if x.err != nil {
		out.LexicalComplete = false
	}
	if x.embeddingErr != nil && out.Semantic == "complete" {
		out.Semantic = "unavailable"
	}
	x.mu.Unlock()
	return out, nil
}

func (x *SearchIndex) addUsage(tokens int64) error {
	return x.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("usage"))
		n, _ := strconv.ParseInt(string(b.Get([]byte("tokens"))), 10, 64)
		return b.Put([]byte("tokens"), []byte(strconv.FormatInt(n+tokens, 10)))
	})
}

// Rebuild discards only derived entries, then replays committed evidence.
// It never acquires live data or changes original conversation records.
func (x *SearchIndex) Rebuild(ctx context.Context) error {
	x.update.Lock()
	x.embedding.Lock()
	err := x.db.Update(func(tx *bolt.Tx) error {
		for _, name := range []string{"fragments", "terms", "progress"} {
			if err := tx.DeleteBucket([]byte(name)); err != nil {
				return err
			}
			if _, err := tx.CreateBucket([]byte(name)); err != nil {
				return err
			}
		}
		return nil
	})
	x.embedding.Unlock()
	x.update.Unlock()
	if err != nil {
		return err
	}
	return x.Sync(ctx)
}
