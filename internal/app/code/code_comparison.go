package code

import (
	"fmt"
	"github.com/pmezard/go-difflib/difflib"
	"github.com/simjay/plous/internal/source"

	manifest "github.com/simjay/plous/internal/integrations/manifest"
	"github.com/simjay/plous/internal/ui"
	"slices"
	"strings"
)

func (s *CodeState) ComparisonPreview(current manifest.Resource, width, height int) string {
	if !s.ComparisonCurrent() {
		return "Comparison uses previous output.\na Show Final YAML opens the latest output."
	}
	changes := s.ComparisonChanges()
	if s.Cursor < 0 || s.Cursor >= len(changes) {
		return "Select a comparison resource."
	}
	change := changes[s.Cursor]
	before, after := change.Before, change.After
	leftLabel, rightLabel := s.Comparison.Path, s.ComparisonBase.Path
	if s.SavedComparison {
		leftLabel, rightLabel = "Before Save", "After Save"
	}
	if s.ComparisonSwapped {
		before, after = after, before
		leftLabel, rightLabel = rightLabel, leftLabel
	}
	old, newText := manifest.MaskedComparison(before.Object, after.Object)
	if before.Name == "" {
		old = ""
	}
	if after.Name == "" {
		newText = ""
	}
	title := "Compare " + leftLabel + " → " + rightLabel
	rows := CodeComparisonRows(old, newText)
	if s.ComparisonChanged {
		filtered := rows[:0]
		for _, row := range rows {
			if row.Changed {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}
	identity := change.Before.Key() + "|" + change.After.Key()
	if identity != s.ComparisonResource {
		s.ComparisonOffset, s.ComparisonResource = 0, identity
	}
	selectedRow := min(s.ComparisonOffset, max(0, len(rows)-1))
	if len(rows) == 0 {
		return "No changed lines in this resource."
	}
	status := change.Status
	if s.ComparisonSwapped {
		if status == "Added" {
			status = "Removed"
		} else if status == "Removed" {
			status = "Added"
		}
	}
	header := ui.Muted.Render(ui.Clipped(title, width)) + "\n" + ui.Muted.Render(ui.Clipped(status+" · "+change.Match, width)) + "\n"
	if s.ComparisonUnified || width < 72 {
		lines := []string{}
		cursor := 0
		for n, row := range rows {
			if n == selectedRow {
				cursor = len(lines)
			}
			if row.Changed {
				if row.HasBefore {
					lines = append(lines, ui.RemovedStyle.Render(ui.Clipped("- "+row.Before, width)))
				}
				if row.HasAfter {
					lines = append(lines, ui.AddedStyle.Render(ui.Clipped("+ "+row.After, width)))
				}
			} else {
				lines = append(lines, ui.Clipped("  "+row.After, width))
			}
		}
		return header + ui.ScrollSelection(lines, cursor, max(1, height-2))
	}
	lw := (width - 3) / 2
	rw := width - lw - 3
	left, right := []string{}, []string{}
	for _, row := range rows {
		a, b := ui.Clipped(row.Before, lw), ui.Clipped(row.After, rw)
		if row.Changed {
			a, b = ui.RemovedStyle.Render(a), ui.AddedStyle.Render(b)
		}
		left, right = append(left, a), append(right, b)
	}
	inner := max(1, height-2)
	return header + CodeColumns(CodePane(leftLabel, ui.ScrollSelection(left, selectedRow, max(1, inner-2)), lw, inner, false), CodePane(rightLabel, ui.ScrollSelection(right, selectedRow, max(1, inner-2)), rw, inner, true), lw, rw, inner)
}

func (s *CodeState) ComparisonCurrent() bool {
	built := s.Builds[s.Preference.Selected]
	return s.ComparisonBase != nil && built != nil && s.Inventory != nil && built.OperationID == s.ComparisonBase.OperationID && built.SourceIdentity == s.Inventory.Source.Identity && built.InputIdentity == s.Inputs[s.Preference.Selected].Identity()
}

type CodeComparisonRow struct {
	Before, After                string
	Line                         int
	HasBefore, HasAfter, Changed bool
}

// Align insertions and removals so later equal lines remain opposite each other.
func CodeComparisonRows(before, after string) []CodeComparisonRow {
	lines := func(text string) []string {
		if text == "" {
			return nil
		}
		return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	}
	a, b := lines(before), lines(after)
	rows := []CodeComparisonRow{}
	for _, op := range difflib.NewMatcher(a, b).GetOpCodes() {
		for n := 0; n < max(op.I2-op.I1, op.J2-op.J1); n++ {
			row := CodeComparisonRow{Changed: op.Tag != 'e'}
			if op.I1+n < op.I2 {
				row.Before, row.HasBefore = a[op.I1+n], true
			}
			if op.J1+n < op.J2 {
				row.After, row.HasAfter, row.Line = b[op.J1+n], true, op.J1+n+1
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func (s *CodeState) ComparisonActive() bool {
	return s.Preference.Section == 1 && s.Stage == 0 && !s.ComparisonHidden && s.Comparison != nil && s.ComparisonBase != nil && s.ComparisonScope == s.Preference.Selected
}

func (s *CodeState) ComparisonChanges() []source.ResourceChange {
	if !s.ComparisonActive() {
		return nil
	}
	var changes []source.ResourceChange
	for _, change := range s.ComparedResources() {
		r := change.Resource()
		if ui.SearchContains(r.Kind+" "+r.Namespace+" "+r.Name+" "+change.Before.Name, s.Filter) && (!s.ComparisonChanged || change.Status != "Unchanged") {
			changes = append(changes, change)
		}
	}
	slices.SortStableFunc(changes, func(a, b source.ResourceChange) int {
		x, y := a.Resource(), b.Resource()
		return strings.Compare(x.Kind+"/"+x.Namespace+"/"+x.Name, y.Kind+"/"+y.Namespace+"/"+y.Name)
	})
	return changes
}

func (s *CodeState) ComparisonLabel() string {
	if s.Comparison == nil || s.ComparisonBase == nil {
		return ""
	}
	counts := map[string]int{}
	for _, change := range s.ComparedResources() {
		counts[change.Status]++
	}
	added, removed := counts["Added"], counts["Removed"]
	if s.ComparisonSwapped {
		added, removed = removed, added
	}
	return fmt.Sprintf("%d changed · %d added · %d removed", counts["Changed"], added, removed)
}

func (s *CodeState) ComparedResources() []source.ResourceChange {
	if s.Comparison == nil || s.ComparisonBase == nil {
		return nil
	}
	identity := s.Comparison.ID + "/" + s.Comparison.OperationID + "/" + s.ComparisonBase.ID + "/" + s.ComparisonBase.OperationID
	if s.ComparisonCache == nil || identity != s.ComparisonIdentity {
		s.ComparisonCache, s.ComparisonIdentity = source.Compare(s.Comparison, s.ComparisonBase), identity
	}
	return s.ComparisonCache
}

func (s *CodeState) ComparisonRowCount() int {
	changes := s.ComparisonChanges()
	if s.Cursor < 0 || s.Cursor >= len(changes) {
		return 0
	}
	change := changes[s.Cursor]
	before, after := change.Before, change.After
	if s.ComparisonSwapped {
		before, after = after, before
	}
	a, b := manifest.MaskedComparison(before.Object, after.Object)
	if before.Name == "" {
		a = ""
	}
	if after.Name == "" {
		b = ""
	}
	count := 0
	for _, row := range CodeComparisonRows(a, b) {
		if !s.ComparisonChanged || row.Changed {
			count++
		}
	}
	return count
}
