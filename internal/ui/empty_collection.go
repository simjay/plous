package ui

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	textview "github.com/simjay/plous/internal/ui/text"
)

const emptyCollectionArt = `  .------.
 /      /|
+------+ |
| .  . | /
|  __  |/
'------'`

// EmptyCollection keeps recovery controls visible before spending space on decoration.
func EmptyCollection(title, description string, actions []textview.Action, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	contentWidth := min(width, 56)
	heading := ansi.Wrap(OverlayTitleStyle.Render(OneLine(title)), contentWidth, "")
	message := ansi.Wrap(CanvasStyle.Render(OneLine(description)), contentWidth, "")
	var controls []string
	for _, action := range actions {
		controls = append(controls, textview.ActionChip(action, contentWidth, WorkspaceTheme()))
	}
	// A short panel still shows its first recovery action after the heading.
	if len(controls) > 0 {
		heading = StyledView(heading, contentWidth, max(1, height-1))
		controls = controls[:min(len(controls), max(0, height-lipgloss.Height(heading)))]
	}
	content := heading
	room := height - lipgloss.Height(heading) - len(controls)
	if description != "" && room >= lipgloss.Height(message)+2 {
		content += "\n\n" + message + "\n"
	} else if room > 0 && len(controls) > 0 {
		content += "\n"
	}
	if len(controls) > 0 {
		content += "\n" + strings.Join(controls, "\n")
	}
	if width >= 20 && height-lipgloss.Height(content) >= lipgloss.Height(emptyCollectionArt)+2 {
		content = OverlayTitleStyle.Render(emptyCollectionArt) + "\n\n" + content
	}
	content = StyledView(content, contentWidth, height)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Position(0.35), content)
}
