package code

import (
	"fmt"
	"slices"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/source"
	"github.com/simjay/plous/internal/ui"
)

// Keep the location's distinguishing tail instead of clipping every sibling to
// the same repository prefix. The inspector retains the complete file path.
func hierarchyTail(text string, width int) string {
	text = ui.OneLine(text)
	if width < 1 {
		return ""
	}
	cells := ansi.StringWidth(text)
	if cells <= width {
		return text
	}
	if width == 1 {
		return "…"
	}
	start := cells - width + 1
	tail := ansi.Cut(text, start, cells)
	for ansi.StringWidth(tail) > width-1 {
		start++
		tail = ansi.Cut(text, start, cells)
	}
	return "…" + tail
}

func hierarchyRole(roles []string) string {
	labels := map[string]string{"base": "Base", "overlay": "Overlay", "shared": "Shared", "component": "Component", "entry": "Source entry"}
	var result []string
	for _, role := range roles {
		if label := labels[role]; label != "" {
			result = append(result, label)
		}
	}
	return strings.Join(result, " · ")
}

func hierarchyPathLines(text string, width int) []string {
	parts := strings.Split(ui.OneLine(text), "/")
	var lines []string
	line := ""
	for index, part := range parts {
		if index < len(parts)-1 {
			part += "/"
		}
		if line != "" && ansi.StringWidth(line+part) > width {
			lines, line = append(lines, line), ""
		}
		wrapped := strings.Split(ansi.Hardwrap(part, width, false), "\n")
		if len(wrapped) > 1 {
			lines = append(lines, wrapped[:len(wrapped)-1]...)
		}
		line += wrapped[len(wrapped)-1]
	}
	lines = append(lines, line)
	if len(lines) > 2 {
		lines = lines[len(lines)-2:]
		lines[0] = "…" + hierarchyTail(lines[0], width-1)
	}
	for len(lines) < 2 {
		lines = append(lines, "")
	}
	return lines
}

func hierarchyWalk(start string, next func(string) []string) map[string]bool {
	seen := map[string]bool{}
	pending := []string{start}
	for len(pending) > 0 {
		id := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if seen[id] {
			continue
		}
		seen[id] = true
		pending = append(pending, next(id)...)
	}
	return seen
}

func (s *CodeState) hierarchyConsumers() map[string]bool {
	return hierarchyWalk(s.Preference.Selected, s.Inventory.EntryConsumers)
}

func (s *CodeState) hierarchyChain() map[string]bool {
	chain := hierarchyWalk(s.Preference.Selected, s.Dependencies)
	for id := range s.hierarchyConsumers() {
		chain[id] = true
	}
	return chain
}

func (s *CodeState) hierarchyBadge(e source.Entry) string {
	charts := s.Inventory.EntryHelmCharts(e.ID)
	if len(charts) == 0 {
		if e.Component {
			return "Included input"
		}
		return s.SourceStatus(e)
	}
	label := "Helm " + charts[0].Name
	if charts[0].Version != "" {
		label += " " + charts[0].Version
	}
	if len(charts) > 1 {
		label += fmt.Sprintf(" +%d", len(charts)-1)
	}
	return label
}

// GraphOrder is the map's stable target-to-dependency navigation order.
// Shared sources occur once even when several incoming edges reference them.
func (s *CodeState) GraphOrder() []string {
	entries := s.Entries()
	visible, visited := map[string]bool{}, map[string]bool{}
	for _, entry := range entries {
		visible[entry.ID] = true
	}
	var order []string
	var walk func(string)
	walk = func(id string) {
		if visited[id] || !visible[id] {
			return
		}
		visited[id] = true
		order = append(order, id)
		for _, dependency := range s.Dependencies(id) {
			walk(dependency)
		}
	}
	for _, entry := range entries {
		if len(s.Inventory.EntryConsumers(entry.ID)) == 0 {
			walk(entry.ID)
		}
	}
	for _, entry := range entries {
		walk(entry.ID)
	}
	return order
}

type hierarchyPosition struct{ x, y int }

type hierarchyCell struct {
	edges   uint8
	routes  map[string]uint8
	lit     bool
	arrow   bool
	forward bool
	dash    bool
}

const (
	hierarchyLeft uint8 = 1 << iota
	hierarchyRight
	hierarchyUp
	hierarchyDown
)

func (c hierarchyCell) crossing() bool {
	for owner, edges := range c.routes {
		if edges&(hierarchyUp|hierarchyDown) == 0 {
			continue
		}
		for other, crossing := range c.routes {
			if owner != other && crossing&(hierarchyLeft|hierarchyRight) != 0 {
				return true
			}
		}
	}
	return false
}

func (c hierarchyCell) glyph() string {
	if c.arrow {
		if c.forward {
			if c.dash {
				return "⇢"
			}
			return "▶"
		}
		if c.dash {
			return "⇠"
		}
		return "◀"
	}
	if c.crossing() {
		return "╳"
	}
	switch c.edges {
	case 0:
		return " "
	case hierarchyLeft, hierarchyRight, hierarchyLeft | hierarchyRight:
		if c.dash {
			return "╌"
		}
		return "─"
	case hierarchyUp, hierarchyDown, hierarchyUp | hierarchyDown:
		return "│"
	case hierarchyLeft | hierarchyUp:
		return "╯"
	case hierarchyLeft | hierarchyDown:
		return "╮"
	case hierarchyRight | hierarchyUp:
		return "╰"
	case hierarchyRight | hierarchyDown:
		return "╭"
	case hierarchyLeft | hierarchyRight | hierarchyUp:
		return "┴"
	case hierarchyLeft | hierarchyRight | hierarchyDown:
		return "┬"
	case hierarchyLeft | hierarchyUp | hierarchyDown:
		return "┤"
	case hierarchyRight | hierarchyUp | hierarchyDown:
		return "├"
	default:
		return "┼"
	}
}

// sourceGraph adapts the shared layout to a browser group or a render scope.
type sourceGraph struct {
	entries  []source.Entry
	selected string
	target   func(string) string
	label    func(source.Entry) string
	forward  bool
}

func (s *CodeState) renderGraph() sourceGraph {
	return sourceGraph{
		entries: s.Entries(), selected: s.Preference.Selected,
		target: func(id string) string { return fmt.Sprintf("code-row/%d", s.SourceIndex(id)) },
		label:  func(e source.Entry) string { return e.Path },
	}
}

func (g sourceGraph) dependencies(s *CodeState, id string) []string {
	dependencies := s.Dependencies(id)
	if g.forward {
		dependencies = slices.DeleteFunc(dependencies, func(id string) bool {
			return !slices.ContainsFunc(g.entries, func(e source.Entry) bool { return e.ID == id })
		})
	}
	return dependencies
}

func (g sourceGraph) chain(s *CodeState) map[string]bool {
	if !g.forward {
		return s.hierarchyChain()
	}
	roles := s.Inventory.EntryRoles(g.selected)
	if slices.Contains(roles, "overlay") || !slices.Contains(roles, "base") && !slices.Contains(roles, "component") {
		return hierarchyWalk(g.selected, s.Dependencies)
	}
	return hierarchyWalk(g.selected, s.Inventory.EntryConsumers)
}

func (s *CodeState) SourceGraph(width, height int) string {
	return s.sourceGraph(width, height, s.renderGraph())
}

// Connectors occupy dedicated gutters. Labels are placed by terminal cell width,
// so wide Unicode paths cannot shift a neighboring node or overwrite its border.
func (s *CodeState) sourceGraph(width, height int, graph sourceGraph) string {
	if width < 1 || height < 1 {
		return ""
	}
	entries := graph.entries
	levels, visiting := map[string]int{}, map[string]bool{}
	cycle := false
	var depth func(string) int
	depth = func(id string) int {
		if level, ok := levels[id]; ok {
			return level
		}
		if visiting[id] {
			cycle = true
			return 0
		}
		visiting[id] = true
		level := 0
		for _, dep := range graph.dependencies(s, id) {
			level = max(level, depth(dep)+1)
		}
		delete(visiting, id)
		levels[id] = level
		return level
	}
	maxLevel := 0
	for _, entry := range entries {
		maxLevel = max(maxLevel, depth(entry.ID))
	}
	columns := max(1, maxLevel+1)
	fallback := ""
	if cycle {
		fallback = "Selected-node graph for cyclic references"
	}
	for _, entry := range entries {
		for _, dep := range graph.dependencies(s, entry.ID) {
			if levels[entry.ID]-levels[dep] > 1 {
				fallback = "Selected-node graph for references across layers"
			}
		}
	}
	if fallback != "" {
		if graph.forward && height > 1 {
			return ui.ChangedStyle.Render(ui.Clipped(fallback, width)) + "\n" + s.compactGraph(width, height-1, graph)
		}
		return s.compactGraph(width, height, graph)
	}
	chain := graph.chain(s)
	groups := make([][]source.Entry, columns)
	for _, entry := range entries {
		groups[levels[entry.ID]] = append(groups[levels[entry.ID]], entry)
	}
	cardHeight := 6
	if graph.forward {
		cardHeight = 4
	}
	stride := cardHeight + 1
	largestColumn := 0
	for _, nodes := range groups {
		largestColumn = max(largestColumn, len(nodes))
	}
	if largestColumn > (height-2)/stride && largestColumn <= (height-2)/cardHeight {
		stride = cardHeight
	}
	capacity := max(1, (height-2)/stride)
	// Each consumer owns one vertical lane. Its branches may join each other;
	// another consumer can only cross that lane, never share its trunk.
	gaps := make([]int, max(0, columns-1))
	gapWidth := 0
	for col := range gaps {
		gaps[col] = min(capacity, len(groups[col+1])) + 2
		gapWidth += gaps[col]
	}
	nodeWidth := (width - gapWidth) / columns
	if nodeWidth < 18 || height < 9 {
		return s.compactGraph(width, height, graph)
	}
	columnX := make([]int, columns)
	for col := 1; col < columns; col++ {
		columnX[col] = columnX[col-1] + nodeWidth + gaps[col-1]
	}
	if graph.forward {
		for col := range columnX {
			columnX[col] = width - columnX[col] - nodeWidth
		}
	}
	positions := map[string]hierarchyPosition{}
	lanes := map[string]int{}
	var visible []source.Entry
	hidden := 0
	for col, nodes := range groups {
		slices.SortStableFunc(nodes, func(a, b source.Entry) int {
			return strings.Compare(a.Path, b.Path)
		})
		start := 0
		for index, entry := range nodes {
			if entry.ID == graph.selected {
				start = index / capacity * capacity
			}
		}
		end := min(start+capacity, len(nodes))
		for n, entry := range nodes[start:end] {
			positions[entry.ID] = hierarchyPosition{columnX[col], 2 + n*stride}
			lanes[entry.ID] = n
			visible = append(visible, entry)
		}
		hidden += len(nodes) - (end - start)
	}
	grid := make([][]hierarchyCell, height)
	for y := range grid {
		grid[y] = make([]hierarchyCell, width)
	}
	link := func(a, b hierarchyPosition, owner string, lit, dash bool) {
		if a.x < 0 || b.x < 0 || a.x >= width || b.x >= width || a.y < 0 || b.y < 0 || a.y >= height || b.y >= height {
			return
		}
		var from, to uint8
		switch {
		case a.x < b.x:
			from, to = hierarchyRight, hierarchyLeft
		case a.x > b.x:
			from, to = hierarchyLeft, hierarchyRight
		case a.y < b.y:
			from, to = hierarchyDown, hierarchyUp
		case a.y > b.y:
			from, to = hierarchyUp, hierarchyDown
		}
		for _, endpoint := range []struct {
			p    hierarchyPosition
			edge uint8
		}{{a, from}, {b, to}} {
			cell := &grid[endpoint.p.y][endpoint.p.x]
			if cell.routes == nil {
				cell.routes = map[string]uint8{}
			}
			cell.routes[owner] |= endpoint.edge
			cell.dash = dash && (cell.edges == 0 || cell.dash)
			cell.edges |= endpoint.edge
			cell.lit = cell.lit || lit
		}
	}
	for _, entry := range visible {
		from := positions[entry.ID]
		for _, id := range graph.dependencies(s, entry.ID) {
			to, ok := positions[id]
			if !ok || !graph.forward && from.x <= to.x || graph.forward && from.x >= to.x {
				continue
			}
			lit := chain[entry.ID] && chain[id]
			dash := false
			if e := s.Inventory.Entry(id); e != nil {
				dash = e.Component
			}
			lane := lanes[entry.ID]
			// Each consumer has a separate gutter lane. Crossings between lanes
			// retain their route owners so the glyph cannot imply a shared trunk.
			ports := [...]int{1, 3, 4}
			y1, y2, mid := from.y+2, to.y+ports[lane%len(ports)], to.x+nodeWidth+1+lane
			startX, endX := from.x, to.x+nodeWidth
			if graph.forward {
				y1, y2 = from.y+1, to.y+1+lane%2
				startX, endX, mid = from.x+nodeWidth, to.x-1, from.x+nodeWidth+1+lane
			}
			for x := min(mid, startX); x < max(mid, startX); x++ {
				link(hierarchyPosition{x, y1}, hierarchyPosition{x + 1, y1}, entry.ID, lit, dash)
			}
			for y := min(y1, y2); y < max(y1, y2); y++ {
				link(hierarchyPosition{mid, y}, hierarchyPosition{mid, y + 1}, entry.ID, lit, dash)
			}
			for x := min(endX, mid); x < max(endX, mid); x++ {
				link(hierarchyPosition{x, y2}, hierarchyPosition{x + 1, y2}, entry.ID, lit, dash)
			}
			grid[y2][endX].arrow = true
			grid[y2][endX].forward = graph.forward
			grid[y2][endX].lit = lit || grid[y2][endX].lit
			grid[y2][endX].dash = dash
		}
	}
	lines := make([]string, height)
	hasCrossing := false
	for y := range grid {
		var line strings.Builder
		for _, cell := range grid[y] {
			hasCrossing = hasCrossing || !cell.arrow && cell.crossing()
			style := ui.Muted
			if cell.lit {
				style = ui.ShortcutStyle
			}
			line.WriteString(style.Render(cell.glyph()))
		}
		lines[y] = line.String()
	}
	replace := func(x, y int, text string) {
		if y >= 0 && y < height {
			lines[y] = ansi.Cut(lines[y], 0, x) + text + ansi.Cut(lines[y], x+ansi.StringWidth(text), width)
		}
	}
	if hasCrossing && !graph.forward {
		replace(0, 1, ui.Muted.Render(ui.Clipped("╳ crossing, no join", width)))
	}
	for col := range groups {
		label := "Sources"
		if graph.forward {
			label = "Bases / components"
		}
		if columns == 1 {
			label = "Source nodes"
		} else if col == columns-1 {
			label = "Target candidates"
			if graph.forward {
				label = "Overlays / entries"
			}
		} else if col > 0 {
			label = "Layers"
		}
		replace(columnX[col], 0, ui.Muted.Render(ui.Clipped(label, nodeWidth)))
	}
	for _, entry := range visible {
		p := positions[entry.ID]
		border, content := ui.Muted, ui.Muted
		if chain[entry.ID] {
			border, content = ui.ShortcutStyle, ui.TitleStyle
		}
		selected := entry.ID == graph.selected
		if selected {
			content = ui.ShortcutStyle.Background(ui.Highlight)
		}
		inner := nodeWidth - 2
		marker := "  "
		if selected {
			marker = "› "
		}
		pathWidth := inner - 2
		pathLines := hierarchyPathLines(graph.label(entry), pathWidth)
		facts := hierarchyRole(s.Inventory.EntryRoles(entry.ID))
		card := []string{marker + pathLines[0], "  " + pathLines[1], " " + facts, " " + s.hierarchyBadge(entry)}
		if graph.forward {
			card = []string{marker + hierarchyTail(graph.label(entry), pathWidth), " " + facts}
		}
		dash := "─"
		if entry.Component {
			dash = "╌"
		}
		box := []string{border.Render("╭" + strings.Repeat(dash, inner) + "╮")}
		for _, row := range card {
			row = ui.Clipped(row, inner)
			row += strings.Repeat(" ", max(0, inner-ansi.StringWidth(row)))
			box = append(box, border.Render("│")+content.Render(row)+border.Render("│"))
		}
		box = append(box, border.Render("╰"+strings.Repeat(dash, inner)+"╯"))
		for n, row := range box[:cardHeight] {
			replace(p.x, p.y+n, ui.MouseTarget(row, graph.target(entry.ID)))
		}
	}
	if hidden > 0 {
		lines[height-1] = ui.Muted.Render(ui.Clipped(fmt.Sprintf("+%d nodes · ↑↓ selects every source", hidden), width))
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxWidth(width).MaxHeight(height).Render(strings.Join(lines, "\n"))
}
