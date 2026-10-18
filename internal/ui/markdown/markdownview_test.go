package markdown

import (
	lipgloss "charm.land/lipgloss/v2"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	textview "github.com/simjay/plous/internal/ui/text"
)

func TestRenderedMarkdownAndExactSourceCopy(t *testing.T) {
	source := "# Argo CD Chart\n\nThis is **community maintained** and *readable*. Set `replicas` below.\n\n- First item\n- Second item\n\n> A warning\n\n[Documentation](https://example.com/docs)\n\n```yaml\n# Scale the workload\nreplicas: 2\n```\n\n| Name | Value |\n| --- | --- |\n| Ready | yes |\n"
	m := New(source)
	view := m.View(90, 50, textview.DefaultTheme())
	plain := ansi.Strip(view)
	t.Log("\n" + view)
	for _, want := range []string{"Argo CD Chart", "community maintained", "First item", "A warning", "Documentation", "https://example.com/docs", "replicas: 2", "Ready"} {
		if !strings.Contains(plain, want) {
			t.Fatal("missing", want, plain)
		}
	}
	for _, raw := range []string{"# Argo", "**community", "```yaml", "[Documentation]("} {
		if strings.Contains(plain, raw) {
			t.Fatal("raw markup rendered", raw)
		}
	}
	if !strings.Contains(view, "\x1b[") {
		t.Fatal("missing styles")
	}
	// Separate viewers and cached documents must follow their supplied palette.
	custom := textview.DefaultTheme()
	custom.Added = custom.Added.Foreground(lipgloss.Color("#123456"))
	custom.Muted = custom.Muted.Foreground(lipgloss.Color("#ABCDEF"))
	for _, theme := range []textview.Theme{custom, textview.DefaultTheme()} {
		for _, viewer := range []*Model{m, New(source)} {
			viewer.View(90, 50, theme)
			for _, sample := range []struct {
				text  string
				style lipgloss.Style
			}{{"replicas: 2", theme.Added}, {"# Scale the workload", theme.Muted}} {
				r, g, b, _ := sample.style.GetForeground().RGBA()
				color := fmt.Sprintf("38;2;%d;%d;%d", r>>8, g>>8, b>>8)
				found := false
				for _, line := range viewer.Reader.Lines {
					if strings.Contains(line.Text, sample.text) && strings.Contains(line.Styled, color) {
						found = true
					}
				}
				if !found {
					t.Fatalf("code block kept a different palette for %q", sample.text)
				}
			}
		}
	}
	_, copied := m.Key("Y", "")
	if copied != source {
		t.Fatal("copy source changed content", copied)
	}
	m.Key("R", "")
	if !strings.Contains(ansi.Strip(m.View(90, 50, textview.DefaultTheme())), "# Argo CD Chart") {
		t.Fatal("source toggle did not show raw Markdown")
	}
}

func TestMarkdownWrapSearchSelectionAndResize(t *testing.T) {
	paragraph := strings.Repeat("Long readable paragraph with Unicode 世界. ", 8)
	m := New("# Title\n\n" + paragraph + "\n\nLast paragraph")
	theme := textview.DefaultTheme()
	m.View(60, 16, theme)
	for i, line := range m.Reader.Lines {
		if strings.Contains(line.Text, "Long readable") {
			m.Reader.Cursor = textview.Point{Row: i}
			break
		}
	}
	_, copied := m.Key("y", "")
	if strings.Contains(copied, "\n") || !strings.Contains(copied, "世界") || len(copied) < len(paragraph)-1 {
		t.Fatal("copy acquired visual wrapping", copied)
	}
	m.Key("/", "")
	m.Reader.Paste("Last")
	m.Key("enter", "")
	if m.Reader.Searching || !strings.Contains(m.Reader.Status(), "1/1") {
		t.Fatal("find failed", m.Reader.Status())
	}
	m.Key("v", "")
	for _, size := range [][2]int{{25, 9}, {110, 30}, {10, 4}} {
		view := m.View(size[0], size[1], theme)
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] || !m.Reader.Selecting {
			t.Fatal("resize lost selection or overflowed", size, view)
		}
	}
	used, _ := m.Key("esc", "")
	if !used || m.Reader.Selecting {
		t.Fatal("Esc did not clear selection")
	}
	used, _ = m.Key("esc", "")
	if !used || m.Reader.Query != "" {
		t.Fatal("Esc did not clear search")
	}
	used, _ = m.Key("esc", "")
	if used {
		t.Fatal("final Esc did not return to caller")
	}
}

func TestMarkdownSearchLiteralKeysAndControlSanitization(t *testing.T) {
	m := New("# Safe\n\n\x1b]52;c;c2VjcmV0\aHello\x1b[2J world")
	m.View(80, 20, textview.DefaultTheme())
	m.Key("/", "")
	m.Key("R", "R")
	m.Key("Y", "Y")
	if m.Source || m.Reader.Query != "RY" {
		t.Fatal("search dispatched actions")
	}
	m.Key("esc", "")
	_, copy := m.Key("Y", "")
	if strings.Contains(copy, "\x1b") || strings.Contains(copy, "c2VjcmV0") {
		t.Fatal("source retained terminal controls", copy)
	}
}
