package code

import (
	"fmt"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/integrations/catalog"
	"github.com/simjay/plous/internal/ui"
	docedit "github.com/simjay/plous/internal/ui/editor"
)

func technologyStyle(technology string) lipgloss.Style {
	switch technology {
	case "helm", "H":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#8DBBEB")).Bold(true)
	case "kustomize", "K":
		return ui.OverlayTitleStyle
	case "argo", "A":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#D9B879")).Bold(true)
	default:
		return ui.AddedStyle.Bold(true)
	}
}

func technologyBadge(letter string) string {
	return technologyStyle(letter).Background(ui.Highlight).Render(" " + letter + " ")
}

func technologyLegend() string {
	var parts []string
	for _, item := range [][2]string{{"H", "Helm"}, {"K", "Kustomize"}, {"A", "Argo"}, {"F", "Flux"}} {
		parts = append(parts, technologyBadge(item[0])+" "+ui.Muted.Render(item[1]))
	}
	return strings.Join(parts, "   ")
}

func fileTreeLine(file CodeFileRow, active bool, width int) string {
	var badges []string
	if file.Directory {
		for _, letter := range strings.Fields(file.Tags) {
			badges = append(badges, technologyBadge(letter))
		}
	}
	tags := strings.Join(badges, " ")
	label := ui.OneLine(file.Label)
	if tags != "" {
		label = ui.Clipped(label, max(1, width-ansi.StringWidth(tags)-4)) + "  " + tags
	}
	prefix, style := "  ", ui.CanvasStyle
	if active {
		prefix, style = "› ", ui.ShortcutStyle.Background(ui.Highlight)
	}
	return style.Width(width).Render(ui.Clipped(prefix+label, width))
}

func catalogField(label, value string, style lipgloss.Style, width int) string {
	return ansi.Wrap(ui.Muted.Render(label+": ")+style.Render(ui.OneLine(value)), max(1, width), "")
}

func catalogCard(item catalog.Item, active bool, width int) string {
	inner := max(1, width-4)
	title := item.Name
	border := ui.BorderColor
	if active {
		title = "› " + title
		border = ui.FocusedBorder
	}
	lines := []string{
		ui.OverlayTitleStyle.Render(ansi.Wrap(ui.OneLine(title), inner, "")),
		technologyStyle(item.Technology).Render(ui.OneLine(item.Kind)),
		catalogField("File", item.File, ui.ShortcutStyle.Bold(false), inner),
	}
	if item.Version != "" {
		lines = append(lines, catalogField("Version", item.Version, ui.TitleStyle, inner))
	}
	if item.Detail != "" {
		lines = append(lines, ansi.Wrap(ui.OneLine(item.Detail), inner, ""))
	}
	if item.External {
		lines = append(lines, ui.ChangedStyle.Render("External chart declaration"))
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).
		Padding(0, 1).Width(width).MaxWidth(width).Render(strings.Join(lines, "\n"))
}

func (s *CodeState) CatalogDetail(item catalog.Item, width int) string {
	lines := []string{ui.OverlayTitleStyle.Render(ansi.Wrap(ui.OneLine(item.Name), width, "")), technologyStyle(item.Technology).Render(item.Kind), ""}
	field := func(label, value string, style lipgloss.Style) {
		if value != "" {
			lines = append(lines, catalogField(label, value, style, width))
		}
	}
	section := func(title string) {
		lines = append(lines, "", ui.SectionHeadingStyle.Render(title), ui.Muted.Render(strings.Repeat("─", width)))
	}
	field("File", item.File, ui.ShortcutStyle.Bold(false))
	field("Version", item.Version, ui.TitleStyle)
	field("Source", item.Location, ui.ShortcutStyle.Bold(false))
	if item.Detail != "" {
		lines = append(lines, "", ansi.Wrap(item.Detail, width, ""))
	}
	if item.External {
		lines = append(lines, "", ui.ChangedStyle.Render(ansi.Wrap("External chart declaration. Chart contents are not captured here.", width, "")))
	}
	if e := s.Inventory.Entry(item.EntryID); e != nil {
		section("Includes")
		if len(e.References) == 0 {
			lines = append(lines, ui.Muted.Render("No declared repository references."))
		}
		for _, ref := range e.References {
			lines = append(lines, ansi.Wrap("• "+ui.Muted.Render(ref.Relation+" → ")+ui.ShortcutStyle.Bold(false).Render(ref.Path), width, ""))
			if ref.Error != "" {
				lines = append(lines, ui.ChangedStyle.Render(ansi.Wrap(ref.Error, width, "")))
			}
		}
		if s.Inventory.Kustomize != nil {
			if k := s.Inventory.Kustomize.Entry(e.Path); k != nil && strings.HasPrefix(e.ID, "kustomize:") {
				section("Changes here")
				for _, line := range strings.Split(k.Summary(), "\n") {
					lines = append(lines, ansi.Wrap(docedit.YAMLLine(line, ui.WorkspaceTheme()), width, ""))
				}
			}
		}
		section("Used by")
		consumers := s.Inventory.Consumers(e.ID)
		for _, consumer := range consumers {
			if entry := s.Inventory.Entry(consumer); entry != nil {
				consumer = entry.Path
			}
			lines = append(lines, ui.ShortcutStyle.Bold(false).Render(ansi.Wrap("• "+consumer, width, "")))
		}
		if len(consumers) == 0 {
			lines = append(lines, ui.Muted.Render("No consumers in this loaded source."))
		}
		field("Affected consumers", fmt.Sprint(len(s.Inventory.Affected(e.ID))), ui.TitleStyle)
	}
	section("Render")
	targets := s.RenderCandidates()
	if len(targets) == 0 {
		message := "No local render target. Open in Files to inspect the declaration. Argo and Flux controller execution is not available."
		if s.View == "code-kustomize" {
			message = "Select an overlay to render its declared composition. Components are included through their consumers."
			if s.KustomizeBaseOnly() {
				message = "Select a consuming overlay, or use Actions → Render Base Only to render this base without its overlays."
			}
		}
		lines = append(lines, ui.Muted.Render(ansi.Wrap(message, width, "")))
	} else {
		for _, target := range targets {
			field(CodeEntryKind(target.Kind), target.Path, ui.ShortcutStyle.Bold(false))
		}
		if item.Technology == "argo" || item.Technology == "flux" {
			lines = append(lines, ui.ChangedStyle.Render(ansi.Wrap("Renders the containing configuration, including this declaration. It does not execute the controller or render its remote workload.", width, "")))
		}
		lines = append(lines, ui.Muted.Render(ansi.Wrap(":r renders saved source into Kubernetes YAML. Inspect Graph and Output; nothing is deployed.", width, "")))
	}
	return strings.Join(lines, "\n")
}
