package app

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
)

func treeDisclosure(row treeRow) string {
	if row.isBranch() {
		if row.open {
			return "▾ "
		}
		return "▸ "
	}
	if row.isAddAction() {
		return "+ "
	}
	return "  "
}

func treeHasSibling(rows []treeRow, index int) bool {
	row := rows[index]
	for _, next := range rows[index+1:] {
		if next.depth < row.depth {
			return false
		}
		if next.depth == row.depth {
			return next.parent == row.parent
		}
	}
	return false
}

func treeGuides(rows []treeRow, index int, continuation bool) string {
	row := rows[index]
	guides := ""
	for depth := 1; depth < row.depth; depth++ {
		ancestor := index - 1
		for ancestor >= 0 && rows[ancestor].depth != depth {
			ancestor--
		}
		if ancestor >= 0 && treeHasSibling(rows, ancestor) {
			guides += "│ "
		} else {
			guides += "  "
		}
	}
	if row.depth > 0 {
		switch {
		case continuation && treeHasSibling(rows, index):
			guides += "│ "
		case continuation:
			guides += "  "
		case treeHasSibling(rows, index):
			guides += "├─"
		default:
			guides += "└─"
		}
	}
	return guides
}

func (m *model) treeRowState(row treeRow) (openResource, ancestor bool) {
	if row.kind == "current" {
		return false, m.currentRoot
	}
	if row.workspace != m.active {
		return false, false
	}
	switch row.kind {
	case "workspace", "current":
		ancestor = true
	case "area":
		ancestor = row.tool == connectionArea(m.tool)
	case "capability":
		openResource = row.connection == m.connection && row.tool == m.tool
	case "gitopssection":
		if row.connection == m.connection && row.tool == m.tool {
			if view := m.gitOpsCurrent(); view != nil {
				openResource = row.resource == string(view.State.Section)
			}
		}
	case "codeview":
		if row.connection == m.connection && m.codeActive() {
			openResource = row.resource == m.coder().ViewID()
		}
	case "tool":
		ancestor = row.tool == m.tool
	case "connection":
		ancestor = row.connection == m.connection
	case "helmsection":
		if m.helmActive() && row.connection == m.connection {
			openResource = row.resource == m.helmer().Preference.Section
		}
	case "category", "resource":
		if m.exploreActive() && m.explore != nil && row.connection == m.connection {
			openResource = row.kind == "resource" && row.resource == m.explore.Preference.Resource
			if resource, ok := kubernetes.Find(m.explore.Resources, m.explore.Preference.Resource); ok {
				ancestor = row.kind == "category" && row.key == categoryTreeKey(m.connection, kubernetes.Category(resource))
			}
		}
	}
	return
}

func (m *model) renderTreeRow(rows []treeRow, index, width int) []string {
	row := rows[index]
	openResource, ancestor := m.treeRowState(row)
	cursor := m.focus == 0 && index == m.treeCursor
	style := lipgloss.NewStyle().Width(width)
	labelStyle := lipgloss.NewStyle().Foreground(ui.Foreground)
	mutedStyle := ui.Muted
	guideStyle := lipgloss.NewStyle().Foreground(ui.BorderColor)
	if row.kind == "workspace" || row.kind == "current" || row.kind == "savedgroup" || row.kind == "area" || row.kind == "category" {
		labelStyle = labelStyle.Bold(true)
	}
	if ancestor {
		labelStyle = labelStyle.Foreground(ui.Accent)
	}
	if openResource {
		labelStyle = labelStyle.Foreground(ui.Accent).Bold(true)
	}
	if cursor {
		style = style.Background(ui.Highlight)
		labelStyle = ui.ShortcutStyle.Background(ui.Highlight)
		mutedStyle = mutedStyle.Background(ui.Highlight)
		guideStyle = guideStyle.Background(ui.Highlight)
	}
	labelWidth := max(1, width-1-row.depth*2-2)
	label := ui.OneLine(row.label)
	if row.isAddAction() {
		label = strings.TrimPrefix(label, "+ ")
	}
	parts := strings.Split(ansi.Hardwrap(label, labelWidth, true), "\n")
	badge := ""
	if row.badge != "" {
		badge = " (" + row.badge + ")"
	}
	if row.kind == "connection" {
		nameWidth := max(0, labelWidth-ansi.StringWidth(badge))
		parts = []string{ansi.Truncate(label, nameWidth, "...")}
	}
	lines := make([]string, 0, len(parts))
	for i, part := range parts {
		indicator, disclosure := " ", "  "
		if i == 0 {
			disclosure = treeDisclosure(row)
			if openResource {
				indicator = "▎"
			}
			if cursor {
				indicator = "›"
			}
		}
		guide := guideStyle.Render(treeGuides(rows, index, i > 0))
		fold := mutedStyle.Render(disclosure)
		if i == 0 && row.isBranch() {
			fold = ui.MouseTarget(fold, "fold/"+row.key)
		}
		renderedPart := labelStyle.Render(part)
		if i == 0 && badge != "" {
			renderedPart += mutedStyle.Render(badge)
		}
		line := mutedStyle.Render(indicator) + guide + fold + renderedPart
		lines = append(lines, ui.MouseTarget(style.Render(ui.Clipped(line, width)), "tree/"+row.key))
	}
	return lines
}

func (m *model) treeBody(width, height int) string {
	rows := m.treeRows()
	lines := []string{}
	heading := ""
	if m.searches[0] != nil {
		heading = m.searchBar(0, width, "Search tree") + "\n"
		height = max(0, height-ui.SearchBarHeight)
		if len(rows) == 1 {
			lines = append(lines, ui.Muted.Render("No matches."))
		}
	}
	selectedRow, selectedHeight := 0, 1
	for i, row := range rows {
		if row.kind == "settings" {
			continue
		}
		if row.kind == "new" && len(m.workspaces) > 0 {
			lines = append(lines, "")
		}
		if row.kind == "workspace" && i > 0 && rows[i-1].kind != "savedgroup" {
			lines = append(lines, "")
		}
		rendered := m.renderTreeRow(rows, i, width)
		if i == m.treeCursor {
			selectedRow = len(lines)
			selectedHeight = len(rendered)
		}
		lines = append(lines, rendered...)
	}
	topHeight := max(0, height-4)
	start := max(0, selectedRow-topHeight+min(topHeight, max(3, selectedHeight)))
	start = min(start, max(0, len(lines)-topHeight))
	visible := lines[start:min(len(lines), start+topHeight)]
	if m.searches[0] == nil {
		visible = append(visible, treeLighthouse(width, topHeight-len(lines))...)
	}
	top := strings.Join(visible, "\n")
	top = lipgloss.NewStyle().Width(width).Height(topHeight).MaxHeight(topHeight).Render(top)
	bottom := []string{lipgloss.NewStyle().Foreground(ui.BorderColor).Render(strings.Repeat("─", width)), ui.Muted.Render("Application"), ""}
	bottom = append(bottom, m.renderTreeRow(rows, len(rows)-1, width)...)
	return heading + top + "\n" + strings.Join(bottom, "\n")
}

func treeLighthouse(width, spareRows int) []string {
	art := []string{
		"         ⡇",
		"       ⡠⠔⠑⠤⡀",
		"      ⠈⡏⢉⣍⠉⡏",
		"      ⡤⠧⠤⠥⠤⠧⡄",
		"      ⠓⢲⠒⠒⢲⠒⠃",
		"       ⡎⢰ ⠉⡆",
		"      ⢠⠋  ⠒⢣",
		"      ⡼⠄ ⠇ ⠘⡄",
		" ⢀   ⢀⠇ ⠠⠄ ⠉⢇",
		"⠈⠁⠑⡠⠤⠺⢖⠚⠓⠒⠒⢒⡺⠤⣀",
		" ⠐⠊⠢⠂  ⠒⠑⠔⠂  ⣀⢄⢁⡀",
		"⠤⠢⡠⠄ ⠒⠑⠔  ⣀⢄  ⢀⡡⡀⡀",
	}
	artWidth := lipgloss.Width(strings.Join(art, "\n"))
	// Leave two rows above the artwork and one above the pinned Application block.
	if width < artWidth+2 || spareRows < len(art)+3 {
		return nil
	}
	lines := make([]string, spareRows-1)
	indent := strings.Repeat(" ", (width-artWidth)/2)
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#647B8D"))
	for i, line := range art {
		lines[len(lines)-len(art)+i] = indent + style.Render(line)
	}
	return lines
}
