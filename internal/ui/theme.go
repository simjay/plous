package ui

import (
	lipgloss "charm.land/lipgloss/v2"
	"github.com/simjay/plous/internal/ui/palette"
	textview "github.com/simjay/plous/internal/ui/text"
	"strings"
)

var AddedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(palette.Success))

var RemovedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(palette.Error))

var ChangedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(palette.Warning))

func WorkspaceTheme() textview.Theme {
	t := textview.DefaultTheme()
	t.Text, t.Muted, t.Accent = CanvasStyle, Muted, lipgloss.NewStyle().Foreground(Accent).Bold(true)
	t.Added, t.Removed = AddedStyle, RemovedStyle
	t.Selection = lipgloss.NewStyle().Background(Highlight)
	t.Keycap = t.Keycap.Foreground(ShortcutStyle.GetForeground())
	return t
}

var Background = lipgloss.Color(palette.Background)

var Foreground = lipgloss.Color(palette.Foreground)

var DashboardAccent = lipgloss.Color("#49D6E8")

var BrandColor = lipgloss.Color(palette.Brand)

var Accent = lipgloss.Color(palette.Accent)

var BorderColor = lipgloss.Color(palette.Border)

var FocusedBorder = BrandColor

var Surface = Background

var Highlight = lipgloss.Color(palette.Selection)

var CanvasStyle = lipgloss.NewStyle().Foreground(Foreground).Background(Background)

var Muted = lipgloss.NewStyle().Foreground(lipgloss.Color(palette.Muted))

var TitleStyle = lipgloss.NewStyle().Foreground(Foreground).Bold(true)

var BrandStyle = lipgloss.NewStyle().Foreground(BrandColor)

var Selected = lipgloss.NewStyle().Foreground(Foreground).Bold(true)

var OverlayTitleStyle = TitleStyle.Foreground(Accent)

var InfoStyle = lipgloss.NewStyle().Foreground(Accent)

var ShortcutStyle = InfoStyle.Bold(true)

var SectionHeadingStyle = TitleStyle

var PrimaryActionStyle = ShortcutStyle

var ChartCPU = lipgloss.NewStyle().Foreground(lipgloss.Color(palette.CPU))
var ChartMemory = lipgloss.NewStyle().Foreground(lipgloss.Color(palette.Memory))
var ChartCPUFill = lipgloss.Color(palette.CPUFill)
var ChartMemoryFill = lipgloss.Color(palette.MemoryFill)

func Heading(text string) string {
	return SectionHeadingStyle.Render(OneLine(text))
}

func FormAction(label string, primary, active bool, width int) string {
	prefix := "  "
	rowStyle := CanvasStyle.Width(width)
	buttonStyle := lipgloss.NewStyle().Foreground(Foreground)
	if primary {
		buttonStyle = PrimaryActionStyle
	} else if strings.HasPrefix(label, "Remove") {
		buttonStyle = RemovedStyle
	}
	if active {
		prefix = "› "
		rowStyle = ShortcutStyle.Background(Highlight).Width(width)
		buttonStyle = rowStyle.UnsetWidth()
		if strings.HasPrefix(label, "Remove") {
			buttonStyle = buttonStyle.Foreground(RemovedStyle.GetForeground())
		}
	}
	return rowStyle.Render(Clipped(prefix+buttonStyle.Render("[ "+OneLine(label)+" ]"), width))
}

func ResourceStatusStyle(value string) lipgloss.Style {
	switch value {
	case "Running", "Ready", "Active", "Bound", "Available", "Succeeded", "Complete", "Normal", "Live":
		return AddedStyle
	case "Pending", "ContainerCreating", "PodInitializing", "Terminating", "Unknown", "Released", "Warning", "Stale":
		return ChangedStyle
	case "Failed", "Error", "NotReady", "Lost", "CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull", "CreateContainerConfigError", "CreateContainerError", "RunContainerError", "InvalidImageName", "OOMKilled", "Unavailable":
		return RemovedStyle
	default:
		return lipgloss.NewStyle()
	}
}
