package app

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var background = lipgloss.Color("#161719")
var foreground = lipgloss.Color("#DDDFE4")
var accent = lipgloss.Color("#BCA4EC")
var borderColor = lipgloss.Color("#363A43")
var focusedBorder = lipgloss.Color("#9A76E5")
var surface = background
var highlight = lipgloss.Color("#2B2735")
var canvasStyle = lipgloss.NewStyle().Foreground(foreground).Background(background)
var muted = lipgloss.NewStyle().Foreground(lipgloss.Color("#90949E"))
var titleStyle = lipgloss.NewStyle().Foreground(foreground).Bold(true)
var selected = lipgloss.NewStyle().Foreground(foreground).Bold(true)
var overlayTitleStyle = titleStyle.Foreground(accent)
var shortcutStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#8DBBEB")).Bold(true)
var sectionHeadingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#D9B879")).Bold(true)
var primaryActionStyle = shortcutStyle

func gradientHeading(text string) string {
	return gradientText(text, lipgloss.Color("#F474B7"), lipgloss.Color("#EEA087"), lipgloss.Color("#EBDD8B"))
}

// Keep each grapheme intact so color changes cannot split accents or emoji.
func gradientText(text string, stops ...color.Color) string {
	text = oneLine(text)
	width := ansi.StringWidth(text)
	if width == 0 {
		return text
	}
	colors := lipgloss.Blend1D(width, stops...)
	var rendered strings.Builder
	column := 0
	for text != "" {
		cluster, cells := ansi.FirstGraphemeCluster(text, ansi.GraphemeWidth)
		rendered.WriteString(lipgloss.NewStyle().Foreground(colors[min(column, width-1)]).Render(cluster))
		column += cells
		text = text[len(cluster):]
	}
	return rendered.String()
}

func formAction(label string, primary, active bool, width int) string {
	prefix := "  "
	rowStyle := canvasStyle.Width(width)
	buttonStyle := lipgloss.NewStyle().Foreground(foreground)
	if primary {
		buttonStyle = primaryActionStyle
	} else if strings.HasPrefix(label, "Remove") {
		buttonStyle = removedStyle
	}
	if active {
		prefix = "› "
		rowStyle = shortcutStyle.Background(highlight).Width(width)
		buttonStyle = rowStyle.UnsetWidth()
		if strings.HasPrefix(label, "Remove") {
			buttonStyle = buttonStyle.Foreground(removedStyle.GetForeground())
		}
	}
	return rowStyle.Render(clipped(prefix+buttonStyle.Render("[ "+oneLine(label)+" ]"), width))
}

func resourceStatusStyle(value string) lipgloss.Style {
	switch value {
	case "Running", "Ready", "Active", "Bound", "Available", "Succeeded", "Complete", "Normal", "Live":
		return addedStyle
	case "Pending", "ContainerCreating", "PodInitializing", "Terminating", "Unknown", "Released", "Warning", "Stale":
		return changedStyle
	case "Failed", "Error", "NotReady", "Lost", "CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull", "CreateContainerConfigError", "CreateContainerError", "RunContainerError", "InvalidImageName", "OOMKilled", "Unavailable":
		return removedStyle
	default:
		return lipgloss.NewStyle()
	}
}
