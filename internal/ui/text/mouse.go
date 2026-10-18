package text

import (
	"net/url"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const mouseURI = "plous-mouse:"

// Mouse marks a control before layout. Hyperlink metadata survives ANSI-aware
// wrapping, clipping and composition, and is removed before terminal output.
func Mouse(text, target string) string {
	open, close := ansi.SetHyperlink(mouseURI+url.QueryEscape(target)), ansi.ResetHyperlink()
	text = strings.ReplaceAll(text, "\x1b]8;;\x1b\\", close)
	text = strings.ReplaceAll(text, close, close+open)
	// Composition interleaves panel lines. Each line must close its own target.
	return open + strings.ReplaceAll(text, "\n", close+"\n"+open) + close
}

// MouseScope supplies an input owner for controls and otherwise empty space.
func MouseScope(text, scope string) string {
	var out strings.Builder
	for len(text) > 0 {
		seq, _, n, _ := ansi.DecodeSequence(text, 0, nil)
		if target, ok := mouseTarget(seq); ok && !strings.Contains(target, "\x1f") {
			seq = ansi.SetHyperlink(mouseURI + url.QueryEscape(scope+"\x1f"+target))
		}
		out.WriteString(seq)
		text = text[n:]
	}
	return Mouse(out.String(), scope+"\x1f")
}

type MouseSpan struct {
	X, Y, Width   int
	Scope, Target string
}

// MouseLayout extracts only explicit control metadata from the final layout.
// Cell widths come from the same ANSI library used by the renderers.
func MouseLayout(text string) (string, []MouseSpan) {
	var out strings.Builder
	var spans []MouseSpan
	x, y := 0, 0
	current, internal := "", false
	for len(text) > 0 {
		seq, width, n, _ := ansi.DecodeSequence(text, 0, nil)
		text = text[n:]
		if target, ok := mouseTarget(seq); ok {
			current, internal = target, true
			continue
		}
		if strings.HasPrefix(seq, "\x1b]8;") {
			wasInternal := internal
			current, internal = "", false
			if wasInternal && (seq == ansi.ResetHyperlink() || seq == "\x1b]8;;\x1b\\") {
				continue
			}
		}
		out.WriteString(seq)
		if seq == "\n" {
			x, y = 0, y+1
			continue
		}
		if width > 0 && current != "" {
			scope, target, _ := strings.Cut(current, "\x1f")
			if len(spans) > 0 {
				last := &spans[len(spans)-1]
				if last.Y == y && last.X+last.Width == x && last.Scope == scope && last.Target == target {
					last.Width += width
				} else {
					spans = append(spans, MouseSpan{x, y, width, scope, target})
				}
			} else {
				spans = append(spans, MouseSpan{x, y, width, scope, target})
			}
		}
		x += width
	}
	return out.String(), spans
}

func mouseTarget(seq string) (string, bool) {
	if !strings.HasPrefix(seq, "\x1b]8;") {
		return "", false
	}
	_, value, ok := strings.Cut(seq[len("\x1b]8;"):], ";")
	if !ok || !strings.HasPrefix(value, mouseURI) {
		return "", false
	}
	value = strings.TrimSuffix(strings.TrimSuffix(value, "\a"), "\x1b\\")
	target, err := url.QueryUnescape(strings.TrimPrefix(value, mouseURI))
	return target, err == nil
}
