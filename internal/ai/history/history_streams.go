package history

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	bolt "go.etcd.io/bbolt"
)

func (h *historyState) normalizeShards(sequence uint64) {
	for stream, shards := range h.manifest.Streams {
		position := max(1, h.meta.Positions[stream])
		for position < len(shards) && shards[position].First <= sequence {
			position++
			h.meta.Offsets[stream] = 0
		}
		h.meta.Positions[stream] = position
	}
}

func (h *historyState) rotateStream(stream string) error {
	shards := h.manifest.Streams[stream]
	if len(shards) == 0 {
		return errors.New("history stream is not writable")
	}
	if h.meta.Offsets[stream] < h.shardTarget {
		return nil
	}
	path, err := h.path(stream, len(shards))
	if err != nil {
		return err
	}
	f, err := openRecordFile(path, false)
	if err != nil {
		return err
	}
	hash := sha256.New()
	_, err = io.Copy(hash, f)
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	next := h.manifest
	next.Streams = map[string][]historyShard{}
	for name, existing := range h.manifest.Streams {
		next.Streams[name] = append([]historyShard(nil), existing...)
	}
	sealed := &next.Streams[stream][len(shards)-1]
	sealed.Sealed, sealed.Last = true, h.meta.Sequence
	sealed.Sizes = historyOffsets{stream: h.meta.Offsets[stream]}
	sealed.Hashes = map[string]string{stream: hex.EncodeToString(hash.Sum(nil))}
	path = filepath.Join(h.dir, stream, shardName(len(shards)+1))
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Size() != 0 {
			return errors.New("unpublished successor shard requires repair")
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err = writeFile(path, nil); err != nil {
			return err
		}
	} else {
		return err
	}
	if err = syncDir(filepath.Dir(path)); err != nil {
		return err
	}
	next.Streams[stream] = append(next.Streams[stream], historyShard{Number: len(shards) + 1, Sizes: historyOffsets{stream: 0}, First: h.meta.Sequence + 1})
	if err = atomicHistoryJSON(filepath.Join(h.dir, "manifest.json"), next); err != nil {
		return err
	}
	h.manifest = next
	h.meta.Offsets[stream] = 0
	h.meta.Positions[stream] = len(shards) + 1
	return h.db.Update(func(tx *bolt.Tx) error {
		return putHistoryJSON(historyBucket(tx, h.manifest.Conversation), []byte("meta"), h.meta)
	})
}

type streamRecovery struct {
	stream   string
	shard    int
	offset   int64
	boundary int64
	file     historyAppendFile
	head     *RecordLine
	length   int64
	sequence uint64
}

func (h *historyState) recoveryHead(c *streamRecovery) error {
	shards := h.manifest.Streams[c.stream]
	for c.shard <= len(shards) {
		if c.file == nil {
			path, err := h.path(c.stream, c.shard)
			if err != nil {
				return err
			}
			if shards[c.shard-1].Sealed {
				c.file, err = openRecordFile(path, false)
			} else {
				c.file, err = h.appendFile(path)
			}
			if err != nil {
				return err
			}
			c.boundary, err = completedBoundary(c.file)
			if err != nil {
				return err
			}
			info, err := c.file.Stat()
			if err != nil {
				return err
			}
			g := shards[c.shard-1]
			if g.Sealed {
				if info.Size() != g.Sizes[c.stream] || c.boundary != info.Size() {
					return errors.New("sealed history size mismatch")
				}
				if c.offset == 0 {
					hash := sha256.New()
					if _, err = io.Copy(hash, io.NewSectionReader(c.file, 0, info.Size())); err != nil {
						return err
					}
					if hex.EncodeToString(hash.Sum(nil)) != g.Hashes[c.stream] {
						return errors.New("sealed history checksum mismatch")
					}
				}
			} else if info.Size() != c.boundary {
				if err = c.file.Truncate(c.boundary); err == nil {
					err = c.file.Sync()
				}
				if err != nil {
					return err
				}
			} else if c.boundary > c.offset {
				if err = c.file.Sync(); err != nil {
					return err
				}
			}
			if c.offset > c.boundary {
				return errors.New("history lookup exceeds saved bytes")
			}
		}
		if c.offset < c.boundary {
			lines, next, err := readRecordLines(context.Background(), c.file, c.offset, c.boundary, 1, MaxRecordBytes)
			if err != nil {
				return err
			}
			var envelope historyEnvelope
			if err = json.Unmarshal(lines[0].Data, &envelope); err != nil {
				return err
			}
			c.head, c.length, c.sequence = &lines[0], next-c.offset, envelope.Sequence
			return nil
		}
		if err := c.file.Close(); err != nil {
			return err
		}
		c.file = nil
		c.shard++
		c.offset = 0
	}
	return nil
}

func (h *historyState) recoverStreams() (resultErr error) {
	defer func() {
		if resultErr != nil {
			h.poisoned = resultErr
		}
	}()
	var cursors []*streamRecovery
	for stream := range h.manifest.Streams {
		if len(h.manifest.Streams[stream]) == 0 {
			continue
		}
		cursors = append(cursors, &streamRecovery{stream: stream, shard: max(1, h.meta.Positions[stream]), offset: h.meta.Offsets[stream]})
	}
	defer func() {
		for _, c := range cursors {
			if c.file != nil {
				_ = c.file.Close()
			}
		}
	}()
	for {
		var chosen *streamRecovery
		for _, c := range cursors {
			if c.head == nil {
				if err := h.recoveryHead(c); err != nil {
					return err
				}
			}
			if c.head != nil && (chosen == nil || c.sequence < chosen.sequence) {
				chosen = c
			}
		}
		if chosen == nil {
			break
		}
		h.normalizeShards(chosen.sequence)
		if err := h.apply(chosen.head.Data, recordLocation{Stream: chosen.stream, Shard: chosen.shard, Offset: chosen.offset, Length: chosen.length}); err != nil {
			return err
		}
		chosen.offset += chosen.length
		chosen.head = nil
	}
	h.normalizeShards(h.meta.Sequence + 1)
	if err := h.db.Update(func(tx *bolt.Tx) error {
		return putHistoryJSON(historyBucket(tx, h.manifest.Conversation), []byte("meta"), h.meta)
	}); err != nil {
		return err
	}
	if h.meta.Active != "" {
		location, err := h.turn(h.meta.Active)
		if err != nil {
			return err
		}
		if location.UserID == "" {
			record, err := h.readID(context.Background(), location.RequestID)
			if err != nil {
				return err
			}
			var entry ReasoningEntry
			if err = json.Unmarshal(record.Data, &entry); err != nil || entry.Request == nil {
				return errors.New("unfinished request is unreadable")
			}
			r := entry.Request
			user := ConversationRecord{ID: r.ID, TurnID: r.ID, Role: "user", Time: r.Time, Response: Response{Status: "completed", Text: r.Prompt, Updated: r.Time}}
			if _, err = h.append("conversation", user, r.ID, 0); err != nil {
				return err
			}
		}
		turn, err := h.loadTurn(h.meta.Active)
		if err != nil {
			return err
		}
		h.active = &turn
		response := turn.Response
		response.Status = "interrupted"
		if response.Error == "" {
			response.Error = "Response interrupted before its closing entry was saved."
		}
		if err = h.finish(response); err != nil {
			return err
		}
	}
	return h.rotate()
}
