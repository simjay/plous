package history

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const MaxReadBytes = 16 << 10

type Chunk struct {
	Ref        string `json:"ref"`
	Text       string `json:"text"`
	NextOffset int    `json:"next_offset"`
	TotalBytes int    `json:"total_bytes"`
	Truncated  bool   `json:"truncated"`
}

func TextChunk(ref, text string, offset, limit int) (Chunk, error) {
	if limit == 0 {
		limit = MaxReadBytes
	}
	if offset < 0 || offset > len(text) || limit < 1 || limit > MaxReadBytes || offset < len(text) && !utf8.RuneStart(text[offset]) {
		return Chunk{}, errors.New("invalid history chunk bounds")
	}
	end := min(offset+limit, len(text))
	for end > offset && end < len(text) && !utf8.RuneStart(text[end]) {
		end--
	}
	if end == offset && end < len(text) {
		return Chunk{}, errors.New("history byte limit is too small for the next character")
	}
	return Chunk{Ref: ref, Text: strings.Clone(text[offset:end]), NextOffset: end, TotalBytes: len(text), Truncated: end < len(text)}, nil
}
