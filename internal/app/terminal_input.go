package app

import (
	"io"
	"os"
	"unicode/utf8"
)

// Keep split UTF-8 characters out of Ultraviolet's escape-sequence timeout.
// Embedding File preserves terminal detection and cancellable reads.
type utf8TerminalInput struct {
	*os.File
	pending []byte
}

func (r *utf8TerminalInput) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	// The terminal reader uses a 4096-byte buffer. A complete rune must fit.
	if len(p) < utf8.UTFMax {
		return 0, io.ErrShortBuffer
	}
	carried := copy(p, r.pending)
	r.pending = r.pending[:0]
	n, err := r.File.Read(p[carried:])
	n += carried
	if n == 0 || err != nil {
		return n, err
	}
	start := n - 1
	for start > 0 && !utf8.RuneStart(p[start]) {
		start--
	}
	if !utf8.FullRune(p[start:n]) {
		r.pending = append(r.pending, p[start:n]...)
		return start, nil
	}
	return n, nil
}
