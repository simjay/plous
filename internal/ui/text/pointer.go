package text

import (
	"fmt"
	"hash/fnv"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// Text targets describe source positions, not decorated text or screen rows.
// Their fingerprint rejects input against a frame whose source has changed.
type TextTarget struct {
	Owner       string
	Row, Offset int
	Hash        uint64
	Cells       bool
}

func TextHash(source string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(source))
	return h.Sum64()
}

func (t TextTarget) String() string {
	unit := "r"
	if t.Cells {
		unit = "c"
	}
	return fmt.Sprintf("text/%s/%s/%d/%d/%x", url.QueryEscape(t.Owner), unit, t.Row, t.Offset, t.Hash)
}

func ParseTextTarget(target string) (TextTarget, bool) {
	p := strings.Split(target, "/")
	if len(p) != 6 || p[0] != "text" || p[2] != "c" && p[2] != "r" {
		return TextTarget{}, false
	}
	owner, err := url.QueryUnescape(p[1])
	row, e1 := strconv.Atoi(p[3])
	offset, e2 := strconv.Atoi(p[4])
	hash, e3 := strconv.ParseUint(p[5], 16, 64)
	return TextTarget{owner, row, offset, hash, p[2] == "c"}, err == nil && e1 == nil && e2 == nil && e3 == nil && row >= 0
}

func TextRow(rendered, source, owner string, row, cell int) string {
	return Mouse(rendered, (TextTarget{Owner: owner, Row: row, Offset: cell, Hash: TextHash(source), Cells: true}).String())
}

// ColumnAt maps expanded tabs and whole grapheme cells back to rune offsets.
func ColumnAt(text string, cell int) int {
	col, cells := 0, 0
	for text != "" {
		cluster, _ := ansi.FirstGraphemeCluster(text, ansi.GraphemeWidth)
		width := Cells(cluster)
		if cells+width > cell {
			return col
		}
		cells += width
		col += utf8.RuneCountInString(cluster)
		text = text[len(cluster):]
	}
	return col
}

// WordBounds returns an exclusive range of complete graphemes on one line.
func WordBounds(text string, col int) (int, int) {
	if text == "" {
		return 0, 0
	}
	col = max(0, min(col, utf8.RuneCountInString(text)-1))
	class := func(r rune) int {
		if unicode.IsSpace(r) {
			return 0
		}
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' {
			return 1
		}
		return 2
	}
	start, offset, kind, found := 0, 0, -1, false
	for text != "" {
		cluster, _ := ansi.FirstGraphemeCluster(text, ansi.GraphemeWidth)
		r, _ := utf8.DecodeRuneInString(cluster)
		nextKind := class(r)
		if nextKind != kind {
			if found {
				return start, offset
			}
			start, kind = offset, nextKind
		}
		offset += utf8.RuneCountInString(cluster)
		found = found || offset > col
		text = text[len(cluster):]
	}
	return start, offset
}

func (v *Viewer) Pointer(point Point, anchor *Point, word bool) {
	v.Cursor = point
	v.clamp()
	v.Selecting, v.Searching, v.Caret = false, false, true
	v.Notice = ""
	// A pointer gesture must not scroll the pressed text out from under itself.
	v.mouseScrolled = true
	if anchor != nil {
		v.Anchor, v.Selecting = *anchor, true
	} else if word && len(v.Lines) > 0 {
		line := v.Lines[v.Cursor.Row].Text
		a, b := WordBounds(line, v.Cursor.Col)
		v.Anchor = Point{v.Cursor.Row, a}
		v.Cursor.Col = Previous(line, b)
		v.Selecting = b > a
	}
}
