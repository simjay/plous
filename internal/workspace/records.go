package workspace

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

const MaxRecordBytes = 2 << 20

type RecordBoundary struct {
	Conversation Conversation `json:"conversation"`
	Messages     int64        `json:"messages"`
	Reasoning    int64        `json:"reasoning"`
}

type RecordLine struct {
	Offset int64           `json:"offset"`
	Data   json.RawMessage `json:"data"`
}

// RecordBoundaries fixes the complete JSONL prefixes visible to an investigation.
func (s *Store) RecordBoundaries(w, current string) ([]RecordBoundary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return nil, ErrClosed
	}
	if _, err := s.conversationDir(w, current); err != nil {
		return nil, err
	}
	conversations, err := s.conversations(w)
	if err != nil {
		return nil, err
	}
	if len(conversations) > 10000 {
		return nil, errors.New("conversation history exceeds the current search index limit")
	}
	var boundaries []RecordBoundary
	for _, c := range conversations {
		if c.ID > current || w == "" && c.ID != current {
			continue
		}
		dir, err := s.conversationDir(w, c.ID)
		if err != nil {
			return nil, err
		}
		b := RecordBoundary{Conversation: c}
		for source, target := range map[string]*int64{"conversation": &b.Messages, "reasoning": &b.Reasoning} {
			f, err := openRecordFile(filepath.Join(dir, source+".jsonl"), false)
			if err != nil {
				return nil, err
			}
			*target, err = completedBoundary(f)
			f.Close()
			if err != nil {
				return nil, err
			}
		}
		boundaries = append(boundaries, b)
	}
	return boundaries, nil
}

func (s *Store) ReadRecords(ctx context.Context, w, id, source string, offset, boundary int64, limit, maxBytes int) ([]RecordLine, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return nil, offset, ErrClosed
	}
	if source != "conversation" && source != "reasoning" {
		return nil, offset, errors.New("unsupported record source")
	}
	if offset < 0 || boundary < offset || limit < 1 || limit > 100 || maxBytes < 1 || maxBytes > 4*MaxRecordBytes {
		return nil, offset, errors.New("invalid record read limits")
	}
	dir, err := s.conversationDir(w, id)
	if err != nil {
		return nil, offset, err
	}
	f, err := openRecordFile(filepath.Join(dir, source+".jsonl"), false)
	if err != nil {
		return nil, offset, err
	}
	defer f.Close()
	size, err := completedBoundary(f)
	if err != nil {
		return nil, offset, err
	}
	if boundary > size {
		return nil, offset, errors.New("record boundary exceeds saved data")
	}
	return readRecordLines(ctx, f, offset, boundary, limit, maxBytes)
}

func openRecordFile(path string, writable bool) (*os.File, error) {
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW
	if writable {
		flags = unix.O_RDWR | unix.O_CLOEXEC | unix.O_NOFOLLOW
	}
	fd, err := unix.Open(path, flags, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, errors.New("conversation records must be regular files")
	}
	if writable {
		if err := f.Chmod(0600); err != nil {
			f.Close()
			return nil, err
		}
	}
	return f, nil
}

func completedBoundary(f *os.File) (int64, error) {
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	size := info.Size()
	if size == 0 {
		return 0, nil
	}
	var final [1]byte
	if _, err := f.ReadAt(final[:], size-1); err != nil {
		return 0, err
	}
	if final[0] == '\n' {
		return size, nil
	}
	start := max(int64(0), size-MaxRecordBytes-1)
	tail := make([]byte, size-start)
	if _, err := f.ReadAt(tail, start); err != nil && !errors.Is(err, io.EOF) {
		return 0, err
	}
	if i := bytes.LastIndexByte(tail, '\n'); i >= 0 {
		return start + int64(i) + 1, nil
	}
	if start > 0 {
		return 0, errors.New("incomplete conversation record exceeds size limit")
	}
	return 0, nil
}

func readRecordLines(ctx context.Context, f *os.File, offset, boundary int64, limit, maxBytes int) ([]RecordLine, int64, error) {
	if offset > 0 {
		var previous [1]byte
		if _, err := f.ReadAt(previous[:], offset-1); err != nil || previous[0] != '\n' {
			return nil, offset, errors.New("record cursor must follow a complete line")
		}
	}
	r := bufio.NewReaderSize(io.NewSectionReader(f, offset, boundary-offset), 64<<10)
	var result []RecordLine
	readBytes := 0
	for offset < boundary && len(result) < limit {
		if err := ctx.Err(); err != nil {
			return nil, offset, err
		}
		var line []byte
		for {
			fragment, err := r.ReadSlice('\n')
			if len(line)+len(fragment) > MaxRecordBytes {
				return nil, offset, errors.New("conversation record exceeds size limit")
			}
			line = append(line, fragment...)
			if errors.Is(err, bufio.ErrBufferFull) {
				continue
			}
			if err != nil {
				return nil, offset, errors.New("record boundary must end at a complete line")
			}
			break
		}
		if readBytes+len(line) > maxBytes && len(result) > 0 {
			break
		}
		if !json.Valid(line) {
			return nil, offset, errors.New("invalid JSON in completed conversation record")
		}
		result = append(result, RecordLine{Offset: offset, Data: line[:len(line)-1]})
		offset += int64(len(line))
		readBytes += len(line)
		if readBytes >= maxBytes {
			break
		}
	}
	return result, offset, nil
}

func appendJSONL(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > MaxRecordBytes {
		return errors.New("conversation record exceeds size limit")
	}
	f, err := openRecordFile(path, true)
	if err != nil {
		return err
	}
	defer f.Close()
	boundary, err := completedBoundary(f)
	if err != nil {
		return err
	}
	// An interrupted append has no newline and was never a published record.
	if err := f.Truncate(boundary); err != nil {
		return err
	}
	if _, err := f.Seek(boundary, io.SeekStart); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}

// ActivityReferences captures revision identities without reading every body.
func (s *Store) ActivityReferences(w string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return nil, ErrClosed
	}
	if !idPattern.MatchString(w) {
		return nil, errors.New("invalid workspace ID")
	}
	dir := filepath.Join(s.root, "workspaces", w, "revisions")
	for _, p := range []string{filepath.Dir(dir), dir} {
		info, err := os.Lstat(p)
		if err != nil || !info.IsDir() {
			return nil, errors.New("activity storage must be a directory")
		}
	}
	entries, err := readDirBounded(dir, 100000)
	if err != nil {
		return nil, err
	}
	if len(entries) > 100000 {
		return nil, errors.New("activity exceeds the current search index limit")
	}
	var refs []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".pending-") {
			continue
		}
		if !e.IsDir() || strings.ContainsAny(e.Name(), `/\`) {
			return nil, errors.New("invalid activity entry")
		}
		refs = append(refs, e.Name())
	}
	return refs, nil
}

func (s *Store) ReadActivity(w, ref string) (Change, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return Change{}, ErrClosed
	}
	if !idPattern.MatchString(w) || strings.HasPrefix(ref, ".") || strings.ContainsAny(ref, `/\`) || len(ref) > 128 || ref == "" {
		return Change{}, errors.New("invalid activity reference")
	}
	dir := filepath.Join(s.root, "workspaces", w)
	for _, path := range []string{dir, filepath.Join(dir, "revisions"), filepath.Join(dir, "revisions", ref)} {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() {
			return Change{}, errors.New("activity storage must be a directory")
		}
	}
	f, err := openRecordFile(filepath.Join(dir, "revisions", ref, "change.json"), false)
	if err != nil {
		return Change{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxRecordBytes+1))
	if err != nil || len(data) > MaxRecordBytes {
		return Change{}, errors.New("activity record exceeds size limit")
	}
	var change Change
	if err := json.Unmarshal(data, &change); err != nil {
		return Change{}, err
	}
	if change.WorkspaceID != w || change.After.ID != w || revisionName(change) != ref {
		return Change{}, errors.New("activity identity does not match reference")
	}
	return change, nil
}

func readDirBounded(path string, limit int) ([]os.DirEntry, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	entries, err := f.ReadDir(limit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > limit {
		return nil, errors.New("stored history exceeds the current search index limit")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}
