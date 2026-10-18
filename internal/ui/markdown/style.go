package markdown

import (
	glamour "charm.land/glamour/v2"
	"fmt"
	chroma "github.com/alecthomas/chroma/v2"
	"image/color"
	"sync"

	glamansi "charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"

	chromastyles "github.com/alecthomas/chroma/v2/styles"
	textview "github.com/simjay/plous/internal/ui/text"
)

type markdownPalette struct {
	text, muted, accent, link, value, removed, codeBackground string
}

func hex(c color.Color) string {
	if c == nil {
		return "#DDDFE4"
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02X%02X%02X", r>>8, g>>8, b>>8)
}

func paletteFor(theme textview.Theme) markdownPalette {
	return markdownPalette{
		text: hex(theme.Text.GetForeground()), muted: hex(theme.Muted.GetForeground()),
		accent: hex(theme.Accent.GetForeground()), link: hex(theme.Keycap.GetForeground()),
		value: hex(theme.Added.GetForeground()), removed: hex(theme.Removed.GetForeground()),
		codeBackground: hex(theme.Selection.GetBackground()),
	}
}

var renderMu sync.Mutex

func renderMarkdown(source string, p markdownPalette) (string, error) {
	// Glamour's inline Chroma configuration keeps the first palette globally.
	// Register ours explicitly and serialize registry changes with rendering.
	renderMu.Lock()
	defer renderMu.Unlock()
	chromastyles.Register(chroma.MustNewStyle("plous", chroma.StyleEntries{
		chroma.Text: p.text, chroma.Error: p.text,
		chroma.Comment: p.muted, chroma.Keyword: p.accent,
		chroma.Name: p.text, chroma.NameTag: p.accent, chroma.NameAttribute: p.accent,
		chroma.NameBuiltin: p.accent, chroma.NameFunction: p.link,
		chroma.Literal: p.value, chroma.Operator: p.text, chroma.Punctuation: p.text,
		chroma.GenericDeleted: p.removed, chroma.GenericInserted: p.value,
		chroma.GenericHeading: p.text + " bold", chroma.GenericSubheading: p.text,
		chroma.GenericEmph: "italic", chroma.GenericStrong: "bold",
	}))

	style := styles.DarkStyleConfig
	style.Document.Margin = ptr(uint(0))
	style.Document.Color = &p.text
	style.Document.BlockPrefix, style.Document.BlockSuffix = "", ""
	style.Heading.Color = &p.text
	for _, heading := range []*glamansi.StyleBlock{&style.H1, &style.H2, &style.H3, &style.H4, &style.H5, &style.H6} {
		*heading = glamansi.StyleBlock{}
	}
	style.H1.Underline = ptr(true)
	style.Strong.Color = &p.text
	style.BlockQuote.Color = &p.muted
	style.HorizontalRule.Color = &p.muted
	style.Link.Color, style.LinkText.Color = &p.link, &p.link
	style.Image.Color, style.ImageText.Color = &p.link, &p.muted
	style.Code.Color, style.Code.BackgroundColor = &p.value, &p.codeBackground
	style.Code.Prefix, style.Code.Suffix = "", ""
	style.CodeBlock = glamansi.StyleCodeBlock{
		StyleBlock: glamansi.StyleBlock{StylePrimitive: glamansi.StylePrimitive{Color: &p.text}, Margin: ptr(uint(0))},
		Theme:      "plous",
	}
	// Keep logical paragraphs for copying; the shared viewer handles wrapping.
	renderer, err := glamour.NewTermRenderer(glamour.WithStyles(style), glamour.WithWordWrap(0), glamour.WithPreservedNewLines(), glamour.WithTableWrap(false), glamour.WithChromaFormatter("terminal16m"))
	if err != nil {
		return "", err
	}
	return renderer.Render(source)
}
