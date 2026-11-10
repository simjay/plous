package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	logview "github.com/simjay/plous/internal/ui/logs"
	textview "github.com/simjay/plous/internal/ui/text"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestLogsEscapeRestoresEachOriginSectionBeforeTheList(t *testing.T) {
	for tab := range 4 {
		for _, key := range []string{"l", "p"} {
			if key == "p" && tab != 0 {
				continue
			}
			t.Run(fmt.Sprintf("section%d/%s", tab, key), func(t *testing.T) {
				m := exploreModel(t)
				e := m.explore
				e.Preference.Filter, e.Preference.Selector, e.Preference.Sort = "checkout", "app=checkout", 2
				e.Preference.Descending = true
				m.openExploreSelection(tab)
				e.Scroll, e.RelatedCursor = 17, 3
				preference, uid := e.Preference, e.Detail.GetUID()
				m.updateExplore(key)
				if e.Tab != 4 || e.LogOrigin == nil {
					t.Fatal("Logs did not capture the detail origin")
				}
				ctx, cancel := context.WithCancel(context.Background())
				e.Log = &monitor.ExploreLogRun{Ctx: ctx, Cancel: cancel}
				e.Scroll = 500
				m.updateExplore("esc")
				if ctx.Err() == nil || e.Log != nil || e.LogOrigin != nil {
					t.Fatal("leaving Logs kept the stream or its return state alive")
				}
				if e.Detail == nil || e.Detail.GetUID() != uid || e.Tab != tab || e.Scroll != 17 || e.RelatedCursor != 3 || e.Preference != preference || m.focus != 1 {
					t.Fatalf("Esc did not restore the exact originating detail: %#v", e)
				}
				m.updateExplore("esc")
				if e.Detail != nil || e.Preference != preference || m.focus != 1 {
					t.Fatal("second Esc did not return to the preserved resource list")
				}
			})
		}
	}
}

func TestWorkloadLogActionsReturnDirectlyToOriginalDetail(t *testing.T) {
	for _, entry := range []string{"l", "logs", "actions"} {
		t.Run(entry, func(t *testing.T) {
			m := exploreModel(t)
			e := m.explore
			pod := e.Collection.Objects[0].DeepCopy()
			workload := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]any{"name": "checkout", "namespace": "shop", "uid": "deployment-uid"}}}
			e.Resource, e.Preference.Resource = e.Resources[1], e.Resources[1].Key()
			e.Detail, e.DetailValid, e.Tab, e.Scroll, e.RelatedCursor = workload, true, 3, 12, 2
			e.Selected = kubernetes.Identity(*workload)
			e.Preference.Filter = "checkout"
			origin := e.Location()
			if entry == "l" {
				m.updateExplore(entry)
			} else {
				m.runExploreCommand(entry, true)
			}
			a := e.Action
			if a == nil {
				t.Fatal("Logs entry did not open target selection")
			}
			a.Pending, a.Operation, a.Pod, a.Container, a.TargetChecked = "", "logs", pod, "app", true
			m.chooseActionContainer()
			if e.Resource.Key() != "pods" || e.Detail.GetUID() != pod.GetUID() || e.Tab != 4 || e.Action != nil || len(e.Trail) != 0 {
				t.Fatal("workload Logs did not open as one action subview")
			}
			m.updateExplore("esc")
			if e.Resource.Key() != origin.Resource.Key() || e.Detail.GetUID() != workload.GetUID() || e.Tab != origin.Tab || e.Scroll != origin.Scroll || e.RelatedCursor != origin.RelatedCursor || e.Preference != origin.Preference || e.Selected != origin.Selected {
				t.Fatal("Logs Esc inserted a Pod Describe or lost workload browsing state")
			}
			m.updateExplore("esc")
			if e.Detail != nil || e.Resource.Key() != "deployments.apps" {
				t.Fatal("second Esc did not reach the workload list")
			}
		})
	}
}

func TestPodLogActionAndContainerSwitchKeepTheOriginalSection(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	m.openExploreSelection(2)
	e.DetailValid, e.Scroll = true, 13
	m.runExploreCommand("logs", true)
	a := e.Action
	if a == nil {
		t.Fatal("Logs command did not open the action")
	}
	a.Pending, a.Operation, a.Pod, a.Container, a.TargetChecked = "", "logs", e.Detail.DeepCopy(), "app", true
	m.chooseActionContainer()
	m.openPodLogs(e.Resource, e.Detail, "sidecar")
	m.updateExplore("esc")
	if e.Detail == nil || e.Tab != 2 || e.Scroll != 13 || e.Container != "app" || len(e.Trail) != 0 {
		t.Fatal("Pod log action or container switch replaced its origin")
	}
}

func TestPodAllContainersKeepsSelectionAcrossRefreshAndRestoresOrigin(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	m.openExploreSelection(2)
	_ = unstructured.SetNestedSlice(e.Detail.Object, []any{map[string]any{"name": "app"}, map[string]any{"name": "proxy"}}, "spec", "containers")
	e.DetailValid, e.Scroll = true, 13
	m.openResourceActions("logs")
	a := e.Action
	a.Pending, a.Operation, a.Pod, a.TargetChecked = "", "logs", e.Detail.DeepCopy(), true
	m.chooseActionPod()
	if a.Stage != "containers" || a.Containers[0] != monitor.AllApplicationContainers {
		t.Fatal("Pod log chooser has no All application containers option")
	}
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.updateResourceActionResult(monitor.ResourceActionResult{Action: a, Serial: a.Serial, Job: "targetaccess"})
	if !e.MergedLogs || e.Container != "" || e.Tab != 4 || len(m.logContainers()) != 3 {
		t.Fatal("All containers did not open the Pod's merged log view")
	}
	m.syncExploreDetail()
	run := e.DetailRun
	run.Cancel()
	m.updateExploreMessage(monitor.ExploreDetailResult{Run: run, Object: e.Detail.DeepCopy()})
	if e.Container != "" {
		t.Fatal("detail refresh replaced All containers with an individual container")
	}
	m.syncExploreDetail()
	logs := e.Log
	m.updateExplore("o")
	m.updateExploreInput(tea.KeyPressMsg{Code: tea.KeyDown})
	m.updateExploreInput(tea.KeyPressMsg{Code: tea.KeyDown})
	m.updateExploreInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.syncExploreDetail()
	if e.MergedLogs || e.Container != "proxy" || logs.Ctx.Err() == nil || e.Log == logs {
		t.Fatal("container switch kept merged sources alive or chose the wrong container")
	}
	m.updateExplore("o")
	m.updateExploreInput(tea.KeyPressMsg{Code: tea.KeyUp})
	m.updateExploreInput(tea.KeyPressMsg{Code: tea.KeyUp})
	m.updateExploreInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !e.MergedLogs || e.Container != "" {
		t.Fatal("could not return to All containers")
	}
	m.updateExplore("esc")
	if e.Tab != 2 || e.Scroll != 13 || e.Container != "app" || e.MergedLogs {
		t.Fatal("container selection lost the original detail or selected container")
	}
}

func TestLogControlsAndSiblingSectionsDoNotAccumulateHistory(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	m.openExploreSelection(1)
	e.Scroll = 9
	m.updateExplore("l")
	for _, key := range []string{"l", "p", "p", "w", "t", "space", "space"} {
		m.updateExplore(key)
	}
	m.updateExplore("esc")
	if e.Tab != 1 || e.Scroll != 9 || len(e.Trail) != 0 {
		t.Fatal("log controls replaced or accumulated the return origin")
	}
	for range 3 {
		for _, key := range []string{"d", "y", "e", "r", "]", "["} {
			m.updateExplore(key)
			if e.Tab == 4 || e.LogOrigin != nil || len(e.Trail) != 0 {
				t.Fatal("sibling navigation created action history or entered Logs")
			}
		}
	}
	m.updateExplore("l")
	m.updateExplore("y")
	if e.Tab != 1 || e.LogOrigin != nil {
		t.Fatal("explicit sibling navigation did not leave Logs")
	}
	m.updateExplore("esc")
	if e.Detail == nil || e.Tab != 3 {
		t.Fatal("YAML did not return to Related after leaving Logs")
	}
	m.updateExplore("esc")
	if e.Detail != nil {
		t.Fatal("Related did not return to the list")
	}
}

func TestLogsBackPreservesRelatedTrailAndCapturedIdentity(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	m.openExploreSelection(3)
	parent := e.Location()
	e.Trail = []monitor.ExploreLocation{parent}
	e.Detail = e.Collection.Objects[1].DeepCopy()
	e.Tab, e.Scroll = 2, 8
	childUID := e.Detail.GetUID()
	m.updateExplore("l")
	e.Detail.SetUID("replacement")
	m.updateExplore("esc")
	if e.Detail.GetUID() != childUID || e.Tab != 2 || e.Scroll != 8 || len(e.Trail) != 1 {
		t.Fatal("Logs lost the captured child or consumed its Related parent")
	}
	m.updateExplore("esc")
	if e.Detail.GetUID() != parent.Object.GetUID() || e.Tab != parent.Tab || len(e.Trail) != 0 {
		t.Fatal("next Esc did not return to the Related parent")
	}
}

func TestLogsEscapeRespectsTransientLayersAndHiddenSidebar(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	m.openExploreSelection(0)
	e.Scroll = 11
	m.updateExplore("l")
	typeText(m, "b")
	press(t, m, "esc")
	if e.Tab != 4 || e.LogOrigin == nil || m.shortcutPrefix != "" {
		t.Fatal("prefix Esc also left Logs")
	}
	typeText(m, ":")
	press(t, m, "esc")
	if e.Tab != 4 || e.LogOrigin == nil || m.command != nil {
		t.Fatal("command Esc also left Logs")
	}
	typeText(m, "/")
	press(t, m, "esc")
	if e.Tab != 4 || e.LogOrigin == nil || m.searches[1] != nil {
		t.Fatal("search Esc also left Logs")
	}
	m.updateExplore("o")
	press(t, m, "esc")
	if e.Tab != 4 || e.LogOrigin == nil || e.Picker != "" {
		t.Fatal("container picker Esc also left Logs")
	}
	m.leftHidden = true
	press(t, m, "esc")
	if e.Detail == nil || e.Tab != 0 || e.Scroll != 11 || m.focus != 1 {
		t.Fatal("Logs Esc did not restore detail while the sidebar was hidden")
	}
	press(t, m, "esc")
	if e.Detail != nil || m.focus != 1 || !m.leftHidden {
		t.Fatal("detail Esc did not return to the table alone")
	}
	press(t, m, "esc")
	if m.focus != 0 || m.leftHidden {
		t.Fatal("root Esc did not reveal the resource sidebar")
	}
}

func TestCancellingResourceActionRetainsOriginSection(t *testing.T) {
	for _, stage := range []string{"menu", "done", "confirm", "scale", "edit", "pods", "containers", "execcommand"} {
		t.Run(stage, func(t *testing.T) {
			m := exploreModel(t)
			e := m.explore
			m.openExploreSelection(2)
			e.DetailValid, e.Scroll = true, 14
			m.openResourceActions("")
			e.Action.Stage, e.Action.Pending = stage, ""
			for range 4 {
				if e.Action != nil {
					m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEscape})
				}
			}
			if e.Action != nil || e.Detail == nil || e.Tab != 2 || e.Scroll != 14 || e.Detail.GetUID() != "one" {
				t.Fatal("cancelling an action lost its originating detail")
			}
		})
	}
}

func logBufferModel(t *testing.T) *model {
	t.Helper()
	m := exploreModel(t)
	m.width, m.height = 160, 40
	m.openExploreSelection(4)
	m.syncExploreDetail()
	run := m.explore.DetailRun
	run.Cancel()
	m.updateExploreMessage(monitor.ExploreDetailResult{Run: run, Object: m.explore.Detail.DeepCopy()})
	m.syncExploreDetail()
	return m
}

func TestLogsReadingKeepsStreamAndResumeKeepsBuffer(t *testing.T) {
	m := logBufferModel(t)
	e := m.explore
	run, key := e.Log, e.LogKey
	m.updateExploreMessage(monitor.ExploreLogEvent{Run: run, Text: strings.Repeat("record\n", 100)})
	m.render()
	press(t, m, "space")
	v := m.podLogViewer()
	offset := v.Reader.Top
	if v.Following || offset >= 100 || m.syncExploreDetail() != nil || run.Ctx.Err() != nil {
		t.Fatal("pause stopped collection or lost viewport")
	}
	m.updateExploreMessage(monitor.ExploreLogEvent{Run: run, Text: "new record 1\nnew record 2\n"})
	m.render()
	if v.Reader.Top != offset || v.Unseen != 2 || !strings.Contains(v.Text, "new record 2") {
		t.Fatal("append lost reading position or output")
	}
	text := v.Text
	press(t, m, "space")
	if !v.Following || v.Unseen != 0 || v.Text != text || e.LogKey != key || e.Log != run || v.Reader.Cursor.Row < 100 {
		t.Fatal("resume replaced stream or lost output")
	}
	if m.syncExploreDetail() != nil || run.Ctx.Err() != nil {
		t.Fatal("resume restarted collection")
	}
}

func TestLogsEndHoldsLatestPositionUntilFollowingResumes(t *testing.T) {
	m := logBufferModel(t)
	m.appendExploreLogs(strings.Repeat("earlier record\n", 100))
	press(t, m, "home")
	press(t, m, "end")
	m.render()
	v := m.podLogViewer()
	offset, cursor := v.Reader.Top, v.Reader.Cursor
	if v.Following || cursor.Row != len(v.Reader.Lines)-1 {
		t.Fatal("End failed to hold tail")
	}
	m.appendExploreLogs(strings.Repeat("new record\n", 10))
	m.render()
	if v.Reader.Top != offset || v.Reader.Cursor != cursor {
		t.Fatal("new output moved held tail")
	}
}

func TestLogsTrimPreservesRetainedReadingPosition(t *testing.T) {
	for _, wrap := range []bool{true, false} {
		t.Run(fmt.Sprintf("wrap=%t", wrap), func(t *testing.T) {
			m := logBufferModel(t)
			v := m.podLogViewer()
			v.Reader.Wrap = wrap
			var initial strings.Builder
			for i := range 600 {
				fmt.Fprintf(&initial, "record %04d %s\n", i, strings.Repeat("word ", 35))
			}
			m.appendExploreLogs(initial.String())
			v.Pause()
			v.Reader.Cursor = textview.Point{Row: 400}
			m.render()
			anchor := v.Reader.SelectedText()
			m.appendExploreLogs(strings.Repeat("new record "+strings.Repeat("word ", 35)+"\n", 250))
			m.render()
			if len(v.Text) > monitor.ExploreLogBufferLimit || v.Dropped == 0 || v.Unseen != 250 {
				t.Fatal("retention bounds lost")
			}
			if v.Reader.SelectedText() != anchor {
				t.Fatal("eviction moved retained selection")
			}
		})
	}
}

func TestLogsSearchKeepsSelectedMatchAsBufferChanges(t *testing.T) {
	m := logBufferModel(t)
	v := m.podLogViewer()
	var initial strings.Builder
	for i := range 600 {
		fmt.Fprintf(&initial, "needle %04d %s\n", i, strings.Repeat("word ", 35))
	}
	m.appendExploreLogs(initial.String())
	typeText(m, "/needle 0400")
	press(t, m, "enter")
	m.render()
	before := v.Reader.Top
	anchor := v.Reader.SelectedText()
	m.appendExploreLogs("needle appended\n")
	m.render()
	if v.Reader.Top != before || v.Following {
		t.Fatal("append moved search result")
	}
	m.appendExploreLogs(strings.Repeat("needle "+strings.Repeat("word ", 35)+"\n", 250))
	m.render()
	if v.Reader.SelectedText() != anchor || v.Reader.Status() != "1/1 matches" || v.Dropped == 0 || m.explore.Log.Ctx.Err() != nil {
		t.Fatal("eviction lost match or stopped collection")
	}
	if m.searches[1] != nil || m.beginPanelSearch() {
		t.Fatal("logs created a second search controller")
	}
}

func TestLogsSearchNearTailDoesNotRecenterForNewOutput(t *testing.T) {
	m := logBufferModel(t)
	m.appendExploreLogs(strings.Repeat("older record\n", 20) + "needle\n")
	typeText(m, "/needle")
	press(t, m, "enter")
	m.render()
	v := m.podLogViewer()
	offset := v.Reader.Top
	m.appendExploreLogs(strings.Repeat("new record\n", 20))
	m.render()
	if v.Reader.Top != offset {
		t.Fatal("new output recentered match")
	}
}

func TestLogsConnectionStateAndSourceReset(t *testing.T) {
	m := logBufferModel(t)
	e := m.explore
	run := e.Log
	if !m.loadingVisible() {
		t.Fatal("initial connection must animate")
	}
	m.updateExploreMessage(monitor.ExploreLogEvent{Run: run, Connected: true})
	if m.loadingVisible() || !e.LogConnected {
		t.Fatal("connected stream must stop animation")
	}
	v := m.podLogViewer()
	v.Append("retained\n")
	v.Dropped, v.Unseen = 3, 4
	v.Reader.Wrap = false
	m.updateExplore("p")
	m.syncExploreDetail()
	if run.Ctx.Err() == nil || e.Log == run || v.Text != "" || v.Unseen != 0 || v.Dropped != 0 || e.LogConnected || v.Reader.Wrap {
		t.Fatal("source reset lost wrap preference or kept old stream")
	}
	m.updateExploreMessage(monitor.ExploreLogEvent{Run: run, Text: "stale\n", Connected: true})
	if v.Text != "" || e.LogConnected {
		t.Fatal("stale output reached replacement")
	}
	current := e.Log
	m.updateExploreMessage(monitor.ExploreLogEvent{Run: current, Text: "previous\n", Done: true})
	if e.Log != nil || m.syncExploreDetail() != nil || v.Text != "previous\n" {
		t.Fatal("ended stream lost output or restarted")
	}
	m.updateExplore("g")
	if e.DetailKey != "" || e.LogKey != "" {
		t.Fatal("reconnect did not invalidate request")
	}
}

func TestLogsEscCancelsCollectionAndRestoresDetail(t *testing.T) {
	m := logBufferModel(t)
	run := m.explore.Log
	press(t, m, "up")
	press(t, m, "esc")
	if run.Ctx.Err() != context.Canceled || m.explore.Tab != 0 || m.explore.Detail == nil {
		t.Fatal("leaving logs did not restore origin")
	}
}

func TestLogHeaderKeepsSourceReadingAndConnectionDistinct(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(4)
	e := m.explore
	e.Detail.SetNamespace("payments")
	e.Detail.SetName("checkout")
	e.Container, e.LogKey = "app", "loaded"
	e.Log = &monitor.ExploreLogRun{Cancel: func() {}}
	e.LogConnected, e.Previous = true, true
	m.podLogViewer().Pause()
	m.podLogViewer().Unseen, m.podLogViewer().Dropped = 23, 12
	text := ansi.Strip(m.exploreLogBody(110, 30))
	for _, want := range []string{"payments / checkout / app", "Previous instance · Latest 200 lines", "Reading", "Loading", "23 new lines", "12 older lines removed", "Stream", "Source", "Display", "[Space] Jump live", "[o] Container", "[u] Time range", "[p] Previous instance On", "[t] Timestamps Off", "[w] Wrap lines On", "[g] Reload logs"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in log controls: %s", want, text)
		}
	}
	e.Log = nil
	text = ansi.Strip(m.exploreLogBody(110, 30))
	if !strings.Contains(text, "Ended") || !strings.Contains(text, "Previous") || !strings.Contains(text, "Reading") {
		t.Fatal("stream completion hid the source or reading position")
	}
}

func TestMergedLogSourceFormattingPreservesCopySearchAndTimestamps(t *testing.T) {
	prefix := "[frontend-podinfo-6db9bf595b-fgl9m/podinfo] "
	stamp := "2026-09-30T20:35:10.608Z "
	message := `{"msg":"Starting podinfo","literal":"[other/source]"}`
	raw := prefix + stamp + message
	v := logview.New(4096, 100)
	v.SetFormatter(monitor.PodLogFormatter(false, true))
	v.Append(raw)
	row := v.Reader.Lines[0]
	if row.Text != prefix+message || ansi.Strip(row.Styled) != row.Text || row.ContinuationIndent != len(prefix) {
		t.Fatalf("source or message changed: %#v", row)
	}
	v.Key("/", "")
	v.Reader.Paste("fgl9m")
	v.Key("enter", "")
	_, copied := v.Key("y", "")
	if copied != prefix+message || v.Reader.Status() != "1/1 matches" || v.Text != raw {
		t.Fatal("source prefix is not searchable/copyable, or raw output changed")
	}
	v.SetFormatter(monitor.PodLogFormatter(true, true))
	if v.Reader.SelectedText() != raw || ansi.Strip(v.Reader.Lines[0].Styled) != raw {
		t.Fatal("timestamp toggle lost source identity or timestamp")
	}
	if got := monitor.PodLogFormatter(false, false)(raw); got.Text != raw || got.ContinuationIndent != 0 {
		t.Fatal("ordinary log text was mistaken for generated metadata")
	}
	first := monitor.PodLogSourceStyle("frontend-podinfo-6db9bf595b-fgl9m").GetForeground()
	second := monitor.PodLogSourceStyle("frontend-podinfo-6db9bf595b-znl86").GetForeground()
	if first == second {
		t.Fatal("sample Pods are not visually distinguishable")
	}
	for _, container := range []string{"app", "proxy"} {
		line := "[same-pod/" + container + "] hello"
		formatted := monitor.PodLogFormatter(false, true)(line)
		if !strings.HasPrefix(formatted.Styled, monitor.PodLogSourceStyle("same-pod").Render("[same-pod/"+container+"]")) {
			t.Fatal("container changed its Pod color")
		}
	}
}

func TestLogViewerFitsCompactAndWidePanelsWithoutLosingOutput(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(4)
	e := m.explore
	e.DetailValid = true
	e.Container, e.LogKey = "app", "loaded"
	m.podLogViewer().Append(strings.Repeat("earlier line\n", 30) + "latest evidence")
	for _, size := range [][2]int{{28, 6}, {48, 12}, {84, 26}, {196, 40}} {
		text := m.exploreLogBody(size[0], size[1])
		lines := strings.Split(text, "\n")
		if len(lines) > size[1] {
			t.Fatalf("%v log view used %d rows", size, len(lines))
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("%v log view overflowed: %q", size, ansi.Strip(line))
			}
		}
		if !strings.Contains(ansi.Strip(text), "latest evidence") {
			t.Fatalf("%v controls crowded all log output out of the view", size)
		}
	}
}

func TestLogHeaderDoesNotGrowWhenReadingOrReceivingMoreLines(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(4)
	m.explore.DetailValid = true
	m.podLogViewer().Append(strings.Repeat("retained record\n", 100))
	outputRows := func(view string) string {
		rows := []string{}
		for i, row := range strings.Split(ansi.Strip(view), "\n") {
			if strings.Contains(row, "retained record") {
				rows = append(rows, fmt.Sprintf("%d:%s", i, row))
			}
		}
		return strings.Join(rows, "\n")
	}
	for _, width := range []int{28, 48, 84, 196} {
		m.podLogViewer().Follow()
		m.podLogViewer().Dropped = 0
		before := outputRows(m.exploreLogBody(width, 30))
		m.podLogViewer().Pause()
		m.podLogViewer().Unseen, m.podLogViewer().Dropped = 123456, 987654
		if after := outputRows(m.exploreLogBody(width, 30)); before == "" || before != after {
			t.Fatalf("new-line notice moved the retained output at width %d", width)
		}
	}
}

func TestStyledLogTimestampsPreserveOriginalText(t *testing.T) {
	text := "2026-09-30T10:00:00Z ready\nnot-a-time message\n2026-09-30T10:00:01.123Z error\n"
	if got := ansi.Strip(monitor.StyledLogTimestamps(text)); got != text {
		t.Fatalf("timestamp styling changed evidence: %q", got)
	}
}
