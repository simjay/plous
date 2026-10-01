package app

import (
	"cmp"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/kubernetes"
)

var relatedGroups = []string{"Owner", "Managed resources", "Referenced resources", "Selector matches"}

func relatedGroup(ref kubernetes.Reference) int {
	if ref.Relation == "Owner" {
		return 0
	}
	if strings.Contains(strings.ToLower(ref.Relation), "selector match") {
		return 3
	}
	// A claim or node used by an owned Pod is a reference, not an owned object.
	if strings.HasPrefix(ref.Relation, "Owned ") && !strings.HasPrefix(ref.Relation, "Owned Pod ") {
		return 1
	}
	return 2
}

func sortedReferences(refs []kubernetes.Reference) []kubernetes.Reference {
	refs = slices.Clone(refs)
	slices.SortStableFunc(refs, func(a, b kubernetes.Reference) int {
		if n := cmp.Compare(relatedGroup(a), relatedGroup(b)); n != 0 {
			return n
		}
		return cmp.Compare(a.Resource.Kind+"/"+a.Namespace+"/"+a.Name, b.Resource.Kind+"/"+b.Namespace+"/"+b.Name)
	})
	return refs
}

func (m *model) relatedBody(width, height int) string {
	e := m.explorer()
	if len(e.references) == 0 {
		text := "No related resources found."
		if e.detailRun != nil && !e.detailLoaded {
			text = m.loading("Loading relationships…")
		}
		if e.detailText != "" {
			text += "\n\n" + e.detailText
		}
		if m.searches[1] != nil {
			return text
		}
		return styledTextView(text, width, height, e.scroll)
	}
	columns := []string{"Kind", "Name", "Namespace", "Relationship"}
	section := kubernetes.DetailSection{Columns: columns}
	for _, ref := range e.references {
		ns, name := ref.Namespace, ref.Name
		if ns == "" {
			ns = "cluster"
			if ref.Resource.Namespaced {
				ns = "all namespaces"
			}
		}
		if ref.List {
			name = "Open list"
		}
		section.Rows = append(section.Rows, []string{ref.Resource.Kind, name, ns, ref.Relation})
	}
	widths, tabular := detailTableWidths(section, width)
	tabular = tabular && width >= 72
	// Keep relationship conditions readable when four columns become too narrow.
	tabular = tabular && width >= 64
	var lines []string
	selectedLine, selectedEnd, group := 0, 0, -1
	for i, ref := range e.references {
		next := relatedGroup(ref)
		if next != group {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, summaryHeading(relatedGroups[next], width))
			if next == 3 {
				lines = append(lines, muted.Render(clipped("Label matches do not establish ownership.", width)))
			}
			if tabular {
				lines = append(lines, renderDetailTableHeader(columns, widths)...)
			}
			group = next
		}
		active := i == e.relatedCursor
		if active {
			selectedLine = len(lines)
		}
		if tabular {
			lines = append(lines, renderDetailTableRow(columns, widths, section.Rows[i], false, active)...)
		} else {
			row := section.Rows[i]
			prefix := "  "
			if active {
				prefix = "› "
			}
			text := prefix + row[0] + " / " + row[1] + "\n  " + row[2] + " · " + row[3]
			text = ansi.Wrap(text, max(1, width), "")
			if active {
				text = selected.Render(text)
			}
			lines = append(lines, strings.Split(text, "\n")...)
		}
		if active {
			selectedEnd = len(lines)
		}
	}
	if e.detailText != "" {
		lines = append(lines, "", summaryHeading("Unavailable relationships", width), ansi.Wrap(plain(e.detailText), max(1, width), ""))
	}
	text := strings.Join(lines, "\n")
	if m.searches[1] != nil {
		return text
	}
	offset := max(0, selectedEnd-max(1, height-1))
	offset = max(min(offset, selectedLine), e.scroll)
	hint := "↑↓ Select · Enter Open resource · / Find"
	if e.detailText != "" {
		hint = "↑↓ Select · Enter Open · End Read notes · / Find"
	}
	return styledTextView(text, width, max(0, height-1), offset) + "\n" + muted.Render(clipped(hint, width))
}
