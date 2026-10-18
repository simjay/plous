package app

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestFooterKeepsCompleteContextualControls(t *testing.T) {
	m := exploreModel(t)
	m.status = ""
	for _, width := range []int{80, 120, 240} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m.width = width
			text := ansi.Strip(m.footer())
			if strings.Contains(text, "\n") || ansi.StringWidth(text) != width {
				t.Fatalf("footer exceeds its single row: %q", text)
			}
			for _, want := range []string{"Pods", "Enter Open", "/ Filter", "Esc Connections", "hh Help"} {
				if !strings.Contains(text, want) {
					t.Errorf("footer missing %q: %s", want, text)
				}
			}
			for _, absent := range []string{"[Main]", "Ctrl+", ":new", ":kinds", "…"} {
				if strings.Contains(text, absent) {
					t.Errorf("footer contains %q: %s", absent, text)
				}
			}
			if width >= 120 && !strings.Contains(text, ":f Fullscreen") {
				t.Fatal("wide footer omitted main expansion")
			}
			t.Log(strings.TrimSpace(text))
		})
	}
	typeText(m, "bb")
	m.status = ""
	text := ansi.Strip(m.footer())
	if !strings.Contains(text, "Esc Browse resources") || strings.Contains(text, "Esc Connections") {
		t.Fatalf("footer advertises a hidden panel as the Esc target: %s", text)
	}
	typeText(m, "b")
	if text := ansi.Strip(m.footer()); !strings.Contains(text, "b → b") || !strings.Contains(text, "Esc cancel") {
		t.Fatalf("pending shortcut lost its instructions: %s", text)
	}
}

func TestFooterFollowsLogAndEditorInputStates(t *testing.T) {
	t.Run("logs", func(t *testing.T) {
		m := exploreModel(t)
		m.openExploreSelection(4)
		m.status = ""
		if !strings.Contains(ansi.Strip(m.footer()), "Space Pause") {
			t.Fatal("following logs should offer Pause")
		}
		press(t, m, "space")
		if !strings.Contains(ansi.Strip(m.footer()), "Space Jump live") {
			t.Fatal("paused logs should offer Jump live")
		}
		typeText(m, "/needle")
		text := ansi.Strip(m.footer())
		if !strings.Contains(text, "Enter Finish search") || strings.Contains(text, "hh Help") || strings.Contains(text, "Space Pause") {
			t.Fatalf("log search advertises navigation keys: %s", text)
		}
		press(t, m, "enter")
		if !strings.Contains(ansi.Strip(m.footer()), "Esc Clear search") {
			t.Fatal("finished log search should clear before leaving")
		}
	})
	t.Run("editor", func(t *testing.T) {
		m, a := documentModel(t)
		m.status = ""
		if text := ansi.Strip(m.footer()); !strings.Contains(text, "Editor NORMAL") || !strings.Contains(text, ":w Save") || !strings.Contains(text, ":h Help") || strings.Contains(text, "hh Help") {
			t.Fatalf("editor should use its own controls: %s", text)
		}
		typeText(m, "i")
		if text := ansi.Strip(m.footer()); !strings.Contains(text, "Editor INSERT") || !strings.Contains(text, "Esc Normal mode") || strings.Contains(text, ":w") || strings.Contains(text, "Help") {
			t.Fatalf("Insert footer advertises commands that insert text: %s", text)
		}
		press(t, m, "esc")
		typeText(m, "/kind")
		if text := ansi.Strip(m.footer()); !strings.Contains(text, "Enter Finish search") || strings.Contains(text, ":w") {
			t.Fatalf("editor search advertises Save: %s", text)
		}
		press(t, m, "esc")
		a.Document.Paste("# unsaved\n")
		m.closeDocument()
		if text := ansi.Strip(m.footer()); !strings.Contains(text, "Esc Keep Editing") || strings.Contains(text, ":w") {
			t.Fatalf("discard prompt footer is misleading: %s", text)
		}
	})
}

func TestFooterExpansionReturnsBeforeClosingAndSearchKeepsLocalKeys(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(1)
	m.viewCommand("fullscreen")
	if !strings.Contains(ansi.Strip(m.footer()), "Esc Restore layout") {
		t.Fatal("expanded YAML does not explain the next Esc")
	}
	typeText(m, "/kind")
	text := ansi.Strip(m.footer())
	if !strings.Contains(text, "Esc Cancel search") || strings.Contains(text, "Restore layout") || strings.Contains(text, "hh Help") {
		t.Fatalf("search should retain literal input: %s", text)
	}
	press(t, m, "esc")
	if !strings.Contains(ansi.Strip(m.footer()), "Esc Restore layout") {
		t.Fatal("closing search did not restore the expansion hint")
	}
}
