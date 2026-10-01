package app

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/kubernetes"
)

func treeDisclosure(row treeRow) string {
	if row.isBranch() {
		if row.open {
			return "▾ "
		}
		return "▸ "
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
	if row.workspace != m.active {
		return false, false
	}
	switch row.kind {
	case "workspace":
		ancestor = true
	case "tool":
		ancestor = row.tool == m.tool
	case "connection":
		ancestor = row.connection == m.connection
	case "category", "resource":
		if m.mode == 1 && m.explore != nil && row.connection == m.connection {
			openResource = row.kind == "resource" && row.resource == m.explore.preference.Resource
			if resource, ok := kubernetes.Find(m.explore.resources, m.explore.preference.Resource); ok {
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
	labelStyle := lipgloss.NewStyle().Foreground(foreground)
	if row.kind == "workspace" || row.kind == "category" {
		labelStyle = labelStyle.Bold(true)
	}
	if ancestor {
		labelStyle = labelStyle.Foreground(accent)
	}
	if openResource {
		labelStyle = labelStyle.Foreground(accent).Bold(true)
	}
	if cursor {
		style = style.Background(highlight)
		labelStyle = labelStyle.Bold(true).Foreground(foreground)
	}
	labelWidth := max(1, width-1-row.depth*2-2)
	parts := strings.Split(ansi.Hardwrap(oneLine(row.label), labelWidth, true), "\n")
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
		guide := lipgloss.NewStyle().Foreground(borderColor).Render(treeGuides(rows, index, i > 0))
		line := muted.Render(indicator) + guide + muted.Render(disclosure) + labelStyle.Render(part)
		lines = append(lines, style.Render(clipped(line, width)))
	}
	return lines
}

func (m *model) treeBody(width, height int) string {
	rows := m.treeRows()
	lines := []string{}
	heading := ""
	if m.searches[0] != nil {
		heading = m.searchBar(0, width, "Search tree") + "\n"
		height = max(0, height-searchBarHeight)
		if len(rows) == 1 {
			lines = append(lines, muted.Render("No matches."))
		}
	}
	selectedRow, selectedHeight := 0, 1
	if len(m.workspaces) == 0 {
		lines = append(lines, muted.Render(clipped("No workspaces yet", width)), "")
	}
	for i, row := range rows {
		if row.kind == "settings" {
			continue
		}
		if row.kind == "new" && len(m.workspaces) > 0 {
			lines = append(lines, "")
		}
		if row.kind == "workspace" && i > 0 {
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
	top := strings.Join(lines[start:min(len(lines), start+topHeight)], "\n")
	top = lipgloss.NewStyle().Width(width).Height(topHeight).MaxHeight(topHeight).Render(top)
	bottom := []string{lipgloss.NewStyle().Foreground(borderColor).Render(strings.Repeat("─", width)), muted.Render("Application"), ""}
	bottom = append(bottom, m.renderTreeRow(rows, len(rows)-1, width)...)
	return heading + top + "\n" + strings.Join(bottom, "\n")
}
