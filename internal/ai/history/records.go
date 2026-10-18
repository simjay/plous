package history

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sort"

	"golang.org/x/sys/unix"
)

const MaxRecordBytes = 8 << 20

type RecordBoundary struct {
	Sequence     uint64       `json:"sequence,string"`
	Conversation Conversation `json:"conversation"`
}

type RecordLine struct {
	Sequence uint64          `json:"sequence,string"`
	ID       string          `json:"id,omitempty"`
	Ref      string          `json:"ref,omitempty"`
	Offset   int64           `json:"offset"`
	Data     json.RawMessage `json:"data"`
}

func openRecordFile(path string, writable bool) (*os.File, error) {
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
	if writable {
		flags = unix.O_RDWR | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
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
	if writable && info.Mode().Perm() != 0600 {
		if err := f.Chmod(0600); err != nil {
			f.Close()
			return nil, err
		}
	}
	return f, nil
}

type recordFileReader interface {
	io.ReaderAt
	Stat() (os.FileInfo, error)
}

func completedBoundary(f recordFileReader) (int64, error) {
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

func readRecordLines(ctx context.Context, f io.ReaderAt, offset, boundary int64, limit, maxBytes int) ([]RecordLine, int64, error) {
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
