package monitor

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	textview "github.com/simjay/plous/internal/ui/text"
)

func TestOverviewArrowsFollowWideTilesAndColumns(t *testing.T) {
	s, _ := overviewDesignFixture()
	s.Resize(150, 44)
	for tile, pane := range []int{1, 1, 2, 3, 3} {
		s.Pane, s.Cursor[0] = 0, tile
		s.Move("down")
		if s.Pane != pane {
			t.Fatalf("Down from tile %d: pane %d, want %d", tile, s.Pane, pane)
		}
		s.Move("up")
		if s.Pane != 0 || s.Cursor[0] != tile {
			t.Fatalf("Up did not return to tile %d: pane %d, tile %d", tile, s.Pane, s.Cursor[0])
		}
	}
	s.Move("home")
	for tile := 1; tile < 5; tile++ {
		s.Move("right")
		if s.Pane != 0 || s.Cursor[0] != tile {
			t.Fatal("Right left the tile row", s.Pane, s.Cursor[0])
		}
	}
	s.Move("right")
	s.Move("up")
	if s.Pane != 0 || s.Cursor[0] != 4 {
		t.Fatal("arrows wrapped at the Pulse edge")
	}
	s.Move("down")
	s.Move("left")
	s.Move("down")
	if s.Pane != 2 || s.Cursor[2] != 1 {
		t.Fatal("lower column or row navigation failed")
	}
	s.Move("left")
	s.Move("left")
	if s.Pane != 1 {
		t.Fatal("Left wrapped out of Deployments")
	}
	s.Move("right")
	if s.Cursor[2] != 1 {
		t.Fatal("returning to Attention lost its row")
	}
	s.Move("home")
	s.Move("up")
	if s.Pane != 0 || s.Cursor[0] != 2 {
		t.Fatal("Up from Attention did not select the tile above it")
	}
}

func TestOverviewArrowsFollowStackedSections(t *testing.T) {
	s, _ := overviewDesignFixture()
	s.Resize(100, 40)
	for tile, pane := range []int{1, 1, 1, 2, 2} {
		s.Pane, s.Cursor[0] = 0, tile
		s.Move("down")
		if s.Pane != pane {
			t.Fatalf("Down from tile %d: pane %d, want %d", tile, s.Pane, pane)
		}
		s.Move("up")
		if s.Pane != 0 || s.Cursor[0] != tile {
			t.Fatalf("Up did not return to tile %d", tile)
		}
	}
	s.Cursor[0] = 3
	s.Move("down")
	if s.Pane != 2 {
		t.Fatal("Down from Pods did not enter Attention")
	}
	s.Move("end")
	s.Move("down")
	if s.Pane != 3 {
		t.Fatal("Down from the last warning did not enter Snapshot")
	}
	s.Move("up")
	if s.Pane != 2 || s.Cursor[2] != len(s.Filtered(2))-1 {
		t.Fatal("Up from Snapshot lost the warning selection")
	}
	s.Move("home")
	s.Move("up")
	if s.Pane != 0 || s.Cursor[0] != 3 {
		t.Fatal("Up from Attention did not return to Pulse")
	}
	s.Move("home")
	s.Move("down")
	s.Move("right")
	if s.Pane != 2 {
		t.Fatal("Right from the top of Deployments did not enter Attention")
	}
	s.Move("right")
	if s.Pane != 2 {
		t.Fatal("Right moved down into Snapshot")
	}
	s.Move("left")
	s.Move("end")
	s.Move("right")
	if s.Pane != 3 {
		t.Fatal("Right from a low deployment row did not enter Snapshot")
	}
	s.Move("left")
	if s.Pane != 1 || s.Cursor[1] != len(s.Filtered(1))-1 {
		t.Fatal("Left lost the deployment row")
	}
}

func TestOverviewCompactAndExpandedSectionsRemainReachable(t *testing.T) {
	for _, size := range [][2]int{{40, 12}, {76, 18}, {70, 40}, {130, 24}} {
		s, _ := overviewDesignFixture()
		s.Resize(size[0], size[1])
		s.Move("right")
		if s.Cursor[0] != 1 || s.Pane != 0 {
			t.Fatalf("%v: Right did not select Memory", size)
		}
		s.Move("down")
		for pane := 1; pane < 4; pane++ {
			if s.Pane != pane {
				t.Fatalf("%v: cannot reach pane %d", size, pane)
			}
			s.Move("end")
			s.Move("right")
			if s.Pane != pane {
				t.Fatal("Right moved vertically in a single column")
			}
			s.Move("down")
		}
		for pane := 3; pane > 0; pane-- {
			s.Move("home")
			s.Move("up")
			if s.Pane != pane-1 {
				t.Fatalf("%v: Up did not return above pane %d", size, pane)
			}
		}
	}
	s, _ := overviewDesignFixture()
	s.Expanded = true
	s.Resize(80, 40)
	s.Move("right")
	s.Move("down")
	if s.Pane != 0 || s.Cursor[0] != 4 {
		t.Fatal("Down did not follow the expanded Pulse grid")
	}
	s.Move("right")
	if s.Cursor[0] != 4 {
		t.Fatal("Right wrapped the last tile")
	}
	s.Move("down")
	if s.Pane != 1 || !s.Expanded {
		t.Fatal("expanded Pulse trapped navigation")
	}
	s.Move("up")
	if s.Pane != 0 || s.Cursor[0] != 4 || !s.Expanded {
		t.Fatal("expanded section return lost the tile or expansion")
	}
	s.Move("up")
	if s.Cursor[0] != 1 {
		t.Fatal("Up did not follow the expanded Pulse grid")
	}
}

func TestOverviewArrowsHandleEmptyFiltersAndSearchOwnership(t *testing.T) {
	s, _ := overviewDesignFixture()
	s.Resize(100, 40)
	s.Pane, s.Query = 2, "no matches"
	s.Move("down")
	if s.Pane != 3 {
		t.Fatal("empty Attention trapped Down")
	}
	s.Move("up")
	s.Move("up")
	if s.Pane != 0 {
		t.Fatal("empty sections trapped Up")
	}
	s.Pane = 2
	input := textview.NewField("no matches")
	s.Input = &input
	s.Move("up")
	s.Move("down")
	if s.Pane != 2 {
		t.Fatal("search arrows left the searched list")
	}
	s.Input, s.Query = nil, ""
	s.Move("j")
	if s.Cursor[2] != 1 {
		t.Fatal("j no longer matches Down")
	}
	s.Move("k")
	s.Move("k")
	if s.Pane != 0 {
		t.Fatal("k no longer matches Up")
	}
}

func TestOverviewNavigationSurvivesResizeAndVisitsSnapshotCounters(t *testing.T) {
	s, _ := overviewDesignFixture()
	s.Resize(150, 44)
	s.Cursor[0] = 3
	s.Move("down")
	s.Move("down")
	if s.Pane != 3 || s.Counter != 1 {
		t.Fatal("Down did not select the first Snapshot counter")
	}
	s.Resize(100, 40)
	s.Move("up")
	if s.Pane != 3 || s.Counter != 0 {
		t.Fatal("Up did not return to the integration heading")
	}
	s.Move("up")
	if s.Pane != 2 {
		t.Fatal("Up used the old three-column layout after resize")
	}
	s.Move("end")
	s.Move("down")
	for range len(s.Filtered(3)[0].Counters) {
		s.Move("down")
	}
	s.Move("down")
	if s.Cursor[3] != 1 || s.Counter != 0 {
		t.Fatal("Down did not advance to the next integration after its counters")
	}
	s.Move("up")
	if s.Cursor[3] != 0 || s.Counter != len(s.Filtered(3)[0].Counters) {
		t.Fatal("Up did not return to the preceding integration's last counter")
	}
	s.Resize(40, 12)
	s.Pane = 2
	if hint := s.NavigationHint(); hint != "↑↓ Rows/sections" {
		t.Fatal("compact list hint advertises horizontal movement", hint)
	}
	view := ansi.Strip(s.Render("fixture", 40, 12, true, time.Now()))
	if !strings.Contains(view, "↑ Deployments · ↓ Snapshot") || strings.Contains(view, "←→") {
		t.Fatal("compact hint does not match section movement", view)
	}
}
