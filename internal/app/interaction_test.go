package app

import (
	"reflect"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

func TestFinishedFindAndChartButtonsHaveSameInputOwner(t *testing.T) {
	for _, mouse := range []bool{false, true} {
		m, run := chartModel(t)
		m.beginPanelSearch()
		typeText(m, "app")
		m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		search := m.searches[1]
		if mouse {
			clickMouse(t, m, "main", "action/I")
		} else {
			typeText(m, "I")
		}
		if m.activeInteraction().owner != interactionCharts || m.activePanelSearch(1) != nil {
			t.Fatal("finished search intercepted chart action", mouse)
		}
		before := *search
		m.pasteText("literal paste")
		m.mainHelp()
		m.hint()
		m.clipboardSurface()
		if !reflect.DeepEqual(before, *search) {
			t.Fatal("inactive search received input", mouse)
		}
		m.openHelp()
		if m.activeInteraction().owner != interactionHelp {
			t.Fatal("Help lost ownership")
		}
		selected := run.Charts.Selected
		m.pasteText("I W P")
		if run.Charts.Selected != selected || !reflect.DeepEqual(before, *search) {
			t.Fatal("Help input reached underlying state")
		}
		m.closeHelp()
		if mouse {
			clickMouse(t, m, "main", "action/I")
		} else {
			typeText(m, "I")
		}
		if m.activePanelSearch(1) != search || search.input.Text != "app" {
			t.Fatal("return lost preserved search")
		}
	}
}

func TestUsageSortKeepsPrecisionHiddenByLabels(t *testing.T) {
	m := exploreModel(t)
	run := enableTestUsage(m)
	now := time.Now()
	run.Samples = map[string]kubernetes.Usage{
		"shop/checkout": {CPU: .000011, Memory: 1048580, Timestamp: now},
		"shop/catalog":  {CPU: .000019, Memory: 1048590, Timestamp: now},
	}
	columns, rows := m.exploreRows()
	for _, metric := range []string{"CPU", "Memory"} {
		col := slices.Index(columns, metric)
		if rows[0].Cells[col] != rows[1].Cells[col] {
			t.Fatal("fixture does not exercise rounding", rows)
		}
		m.explore.Preference.Sort = col
		_, sorted := m.exploreRows()
		if sorted[0].Object.GetName() != "checkout" {
			t.Fatal("rounded labels changed numeric order", metric)
		}
	}
}
