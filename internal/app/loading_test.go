package app

import (
	"strings"
	"testing"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	appsettings "github.com/simjay/plous/internal/app/settings"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

func TestLoadingTicksOnlyAnimateAndStopWhenFinished(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	e.Listed = false
	e.Collection = kubernetes.Collection{}
	stream := e.Stream
	m.dirty, m.shortcutPrefix = false, "b"
	if m.syncLoading() == nil || m.syncLoading() != nil {
		t.Fatal("loading must schedule exactly one animation timer")
	}
	before := m.render()
	_, next := m.Update(loadingTickMsg{})
	after := m.render()
	if next == nil || ansi.Strip(before) == ansi.Strip(after) {
		t.Fatal("a pending request must animate without keyboard input")
	}
	if w, h := lipgloss.Size(before); w != lipgloss.Width(after) || h != lipgloss.Height(after) {
		t.Fatal("animation changed the layout size")
	}
	if m.dirty || m.shortcutPrefix != "b" || e.Stream != stream || e.DetailRun != nil || m.focus != 1 {
		t.Fatal("animation changed navigation, storage, or network request state")
	}
	for range len(loadingFrames) {
		label := ansi.Strip(m.loading("Loading Pods…"))
		if ansi.StringWidth(strings.TrimSuffix(label, " Loading Pods…")) != 5 {
			t.Fatal("dot sweep moved the loading label", label)
		}
		m.Update(loadingTickMsg{})
	}
	m.Update(monitor.ExploreUpdate{Stream: stream, Snapshot: kubernetes.Snapshot{Collection: &kubernetes.Collection{}, Status: "Live"}})
	_, next = m.Update(loadingTickMsg{})
	if next != nil || m.loadingTickPending {
		t.Fatal("completed requests must stop animating, including empty results")
	}
	for _, frame := range loadingFrames {
		if strings.Contains(ansi.Strip(m.render()), frame) {
			t.Fatal("completed request retained a loading indicator")
		}
	}
}

func TestLoadingPausesBehindOverlaysAndDoesNotAnimateErrors(t *testing.T) {
	m := exploreModel(t)
	m.explore.Listed = false
	for _, state := range []string{"help", "settings", "small", "error", "exit"} {
		switch state {
		case "help":
			m.help = &helpState{}
		case "settings":
			m.settingsOverlay = &appsettings.SettingsOverlay{}
		case "small":
			m.width = 60
		case "error":
			m.explore.Err = "Access denied"
		case "exit":
			m.exploreClosed = true
		}
		_, next := m.Update(loadingTickMsg{})
		if next != nil || m.loadingTickPending {
			t.Fatalf("%s kept the loading timer running", state)
		}
		m.help, m.settingsOverlay, m.width, m.explore.Err, m.exploreClosed = nil, nil, 120, "", false
	}
	if m.syncLoading() == nil {
		t.Fatal("visible unfinished work must resume animation")
	}
}
