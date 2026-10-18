package text

import (
	lipgloss "charm.land/lipgloss/v2"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestMouseLayoutFollowsClippingWrappingAndComposition(t *testing.T) {
	control := Mouse("界é abcdefgh", "open/item")
	left := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Width(8).Render(ansi.Hardwrap(control, 6, true))
	right := lipgloss.NewStyle().Width(8).Render(Mouse("Save", "save"))
	rendered := lipgloss.JoinHorizontal(lipgloss.Top, MouseScope(left, "main"), MouseScope(right, "chat"))
	clean, spans := MouseLayout(rendered)
	if ansi.Strip(clean) != ansi.Strip(rendered) || strings.Contains(clean, mouseURI) {
		t.Fatal("annotations changed text or escaped into output")
	}
	var foundOpen, foundSave bool
	for _, span := range spans {
		if span.Target == "open/item" {
			foundOpen = true
			if span.Scope != "main" || span.X < 1 || span.X+span.Width > 7 {
				t.Fatalf("wrong wrapped bounds: %+v", span)
			}
		}
		if span.Target == "save" {
			foundSave = true
			if span.Scope != "chat" || span.X != 8 || span.Width != 4 {
				t.Fatalf("wrong composed bounds: %+v", span)
			}
		}
	}
	if !foundOpen || !foundSave {
		t.Fatalf("missing targets: %+v", spans)
	}
	clipped := ansi.Truncate(Mouse("abcdef", "clipped"), 3, "") + "tail"
	_, clippedSpans := MouseLayout(MouseScope(clipped, "main"))
	for _, span := range clippedSpans {
		if span.Target == "clipped" && span.X+span.Width > 3 {
			t.Fatalf("clipped control leaked: %+v", clippedSpans)
		}
	}
}

func TestMouseNestedControlsRestoreOuterTarget(t *testing.T) {
	text := MouseScope(Mouse("row "+Mouse("arrow", "fold")+" name", "row"), "tree")
	clean, spans := MouseLayout(text)
	if clean != "row arrow name" {
		t.Fatalf("output %q", clean)
	}
	if len(spans) != 3 || spans[0].Target != "row" || spans[1].Target != "fold" || spans[2].Target != "row" {
		t.Fatalf("targets: %+v", spans)
	}
}

func TestMouseOverlayRemovesBackgroundTargets(t *testing.T) {
	base := MouseScope(Mouse("Background", "open"), "tree")
	overlay := MouseScope(Mouse("Save", "save"), "main")
	composite := lipgloss.NewCompositor(lipgloss.NewLayer(ansi.Strip(base)), lipgloss.NewLayer(overlay).X(2).Z(1)).Render()
	clean, spans := MouseLayout(composite)
	if strings.Contains(clean, mouseURI) {
		t.Fatal("internal URI exposed")
	}
	for _, span := range spans {
		if span.Scope != "main" || span.Target != "save" || span.X != 2 {
			t.Fatalf("background target: %+v", spans)
		}
	}
	if len(spans) != 1 {
		t.Fatalf("missing overlay: %+v", spans)
	}
}
