package chat

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ai/agent"
)

func TestNativeScrollbackHoldsOutputAcrossWritesAndResize(t *testing.T) {
	n, _ := NewNative(t.Context(), agent.ClaudeCode, "scope", "label", 30, 5)
	t.Cleanup(n.Close)
	for i := range 30 {
		_, _ = fmt.Fprintf(n, "\x1b[32mReply %02d\x1b[0m\r\n", i)
	}
	if handled, changed := n.Scroll(-3); !handled || !changed || !n.Reading() {
		t.Fatal("normal screen did not enter scrollback")
	}
	before := n.Render()
	if !strings.Contains(ansi.Strip(before), "Reply 23") || !strings.Contains(before, "\x1b[") {
		t.Fatalf("scrollback lost text or styling: %q", before)
	}
	for range 5010 {
		_, _ = n.Write([]byte("new output\r\n"))
	}
	if n.Render() != before {
		t.Fatal("new output or buffer eviction moved the reading position")
	}
	if n.Screen.ScrollbackLen() != 5000 {
		t.Fatal("terminal history is not bounded")
	}
	n.Scroll(-10000)
	if !strings.HasPrefix(ansi.Strip(n.Render()), "Reply 00\n") {
		t.Fatal("history eviction changed rows that had not been rendered yet")
	}
	n.Scroll(23)
	n.Resize(24, 8)
	if !strings.HasPrefix(n.Render(), strings.Split(before, "\n")[0]) {
		t.Fatal("resize lost the first visible row")
	}
	n.Scroll(10000)
	if n.Reading() || !strings.Contains(n.Render(), "new output") {
		t.Fatal("scrolling to the bottom did not follow current output")
	}
}

func TestNativeAlternateScreenDoesNotExposeNormalHistory(t *testing.T) {
	n, _ := NewNative(t.Context(), agent.Codex, "scope", "label", 30, 5)
	t.Cleanup(n.Close)
	_, _ = n.Write([]byte(strings.Repeat("normal history\r\n", 10)))
	_, _ = n.Write([]byte("\x1b[?1049hfullscreen CLI"))
	if handled, changed := n.Scroll(-3); handled || changed || n.Reading() || strings.Contains(n.Render(), "normal history") {
		t.Fatal("alternate screen showed unrelated terminal history")
	}
	_, _ = n.Write([]byte("\x1b[?1049l"))
	if handled, changed := n.Scroll(-3); !handled || !changed || !n.Reading() || !strings.Contains(n.Render(), "normal history") {
		t.Fatal("normal-screen history was lost")
	}
}

func TestNativeScrollbackConcurrentOutput(t *testing.T) {
	n, _ := NewNative(t.Context(), agent.ClaudeCode, "scope", "label", 30, 5)
	t.Cleanup(n.Close)
	_, _ = n.Write([]byte(strings.Repeat("initial output\r\n", 10)))
	var writes sync.WaitGroup
	writes.Go(func() {
		for range 100 {
			_, _ = n.Write([]byte("streaming\r\n"))
		}
	})
	for i := range 100 {
		n.Scroll(-3)
		n.Resize(30, 5+i%2)
		_ = n.Render()
		n.Follow()
	}
	writes.Wait()
}
