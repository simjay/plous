package tui

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/simjay/plous/internal/app/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	textview "github.com/simjay/plous/internal/ui/text"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func actionIDs(r kubecommon.Resource) []string {
	object := &unstructured.Unstructured{Object: map[string]any{}}
	var ids []string
	for _, item := range monitor.ActionsFor(r, object) {
		ids = append(ids, item.Id)
	}
	return ids
}

func TestActionsRespectFullResourceIdentityAndNativeSemantics(t *testing.T) {
	for _, r := range []kubecommon.Resource{{Name: "services"}, {Name: "ingresses", Group: "networking.k8s.io"}, {Name: "pods", Group: "example.io"}} {
		for _, id := range []string{"logs", "exec", "scale", "restart"} {
			if slices.Contains(actionIDs(r), id) {
				t.Fatalf("%s gained %s", r.Key(), id)
			}
		}
	}
	if slices.Contains(actionIDs(kubecommon.Resource{Name: "pods"}), "restart") || slices.Contains(actionIDs(kubecommon.Resource{Name: "daemonsets", Group: "apps"}), "scale") {
		t.Fatal("invented Pod restart or DaemonSet replica scale")
	}
	for _, key := range []string{"deployments", "statefulsets"} {
		ids := actionIDs(kubecommon.Resource{Name: key, Group: "apps"})
		for _, id := range []string{"logs", "exec", "scale", "restart", "edit", "delete"} {
			if !slices.Contains(ids, id) {
				t.Fatalf("%s missing %s", key, id)
			}
		}
	}
	if slices.Contains(actionIDs(kubecommon.Resource{Name: "events", Group: "events.k8s.io"}), "edit") {
		t.Fatal("events gained unsupported editor")
	}
	custom := kubecommon.Resource{Name: "widgets", Group: "example.io", Subresources: map[string][]string{"scale": {"get", "update"}}}
	if !slices.Contains(actionIDs(custom), "scale") {
		t.Fatal("served custom scale is unavailable")
	}
}

func TestActionTargetSnapshotSurvivesWatchAndListSelection(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	m.openExploreSelection(0)
	e.DetailValid = true
	m.openResourceActions("")
	a := e.Action
	captured := a.Object.GetUID()
	e.Selected = "two"
	e.Detail = e.Collection.Objects[1].DeepCopy()
	if a.Object.GetUID() != captured || a.Object.GetUID() == e.Detail.GetUID() {
		t.Fatal("open action was retargeted by a new selection")
	}
	m.stopResourceAction()
	m.updateResourceActionResult(monitor.ResourceActionResult{Action: a, Serial: a.Serial, Job: "prepare", Object: e.Detail.DeepCopy()})
	if e.Action != nil {
		t.Fatal("stale action completion reopened a cancelled form")
	}
}

func TestActionFooterPreservesPendingShortcutAndBlockedCommand(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(0)
	m.explore.DetailValid = true
	m.openResourceActions("")
	m.shortcutPrefix = "h"
	if !strings.Contains(ansi.Strip(m.footer()), "h → h") {
		t.Fatal("action footer hid the pending navigation shortcut")
	}
	m.shortcutPrefix = ""
	m.status = "Finish or cancel the open action first."
	if !strings.Contains(ansi.Strip(m.footer()), m.status) {
		t.Fatal("action footer hid the blocked-command explanation")
	}
}

func TestActionConfirmationDefaultsToBackAndRequiresExplicitApply(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	m.openExploreSelection(0)
	e.DetailValid = true
	m.openResourceActions("")
	a := e.Action
	a.Stage = "confirm"
	a.Operation = "delete"
	a.Pending = ""
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if a.Pending != "" || a.Stage != "menu" {
		t.Fatal("default confirmation mutated a resource")
	}
	a.Stage = "confirm"
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyRight})
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if a.Pending != "delete" {
		t.Fatal("explicit confirmation did not queue delete")
	}
}

func TestActionOverlayFitsTerminalAndKeepsTargetVisible(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	m.openExploreSelection(0)
	e.DetailValid = true
	m.openResourceActions("")
	e.Action.Pending = ""
	for _, size := range [][2]int{{80, 24}, {120, 32}, {160, 40}, {200, 50}} {
		m.width, m.height = size[0], size[1]
		for _, stage := range []string{"menu", "confirm", "edit"} {
			e.Action.Stage = stage
			e.Action.Operation = "delete"
			e.Action.Input = textview.NewField("apiVersion: v1\nkind: Pod\n")
			view := ansi.Strip(m.render())
			if !strings.Contains(view, "UID one") {
				t.Fatalf("%v %s missing captured UID", size, stage)
			}
			if len(strings.Split(view, "\n")) > m.height {
				t.Fatalf("%v %s height overflow", size, stage)
			}
			for _, line := range strings.Split(view, "\n") {
				if ansi.StringWidth(line) > m.width {
					t.Fatalf("%v %s width overflow", size, stage)
				}
			}
		}
	}
}

func TestExecCapturedEscTabAndColonRemainRemote(t *testing.T) {
	m := exploreModel(t)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	tm := &monitor.ResourceTerminal{Emulator: vt.NewSafeEmulator(40, 10), Cancel: cancel, Captured: true}
	m.explore.Terminal = tm
	if m.View().MouseMode != tea.MouseModeNone {
		t.Fatal("captured Exec enabled mouse reporting")
	}
	output := make(chan string, 1)
	go func() {
		buf := make([]byte, 32)
		var text strings.Builder
		for text.Len() < 3 {
			n, err := tm.Emulator.Read(buf)
			text.Write(buf[:n])
			if err != nil {
				break
			}
		}
		output <- text.String()
	}()
	for _, msg := range []tea.KeyPressMsg{{Code: tea.KeyEscape}, {Code: tea.KeyTab}, {Code: ':', Text: ":"}} {
		m.Update(msg)
	}
	select {
	case got := <-output:
		if got != "\x1b\t:" {
			t.Fatalf("remote keys changed: %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("captured input blocked")
	}
	if !tm.Captured || m.focus != 1 || m.command != nil {
		t.Fatal("remote keys executed Plous navigation")
	}
	m.Update(tea.KeyPressMsg{Code: ']', Mod: tea.ModCtrl})
	if tm.Captured {
		t.Fatal("explicit terminal escape did not release input")
	}
	if m.View().MouseMode != tea.MouseModeCellMotion {
		t.Fatal("Exec review did not restore mouse reporting")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.explore.Terminal != nil {
		t.Fatal("terminal controls did not return to details")
	}
}

func TestRelatedListAndChildDetailHaveSeparateBackSteps(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	original := e.Collection.Objects[0].DeepCopy()
	e.Trail = []monitor.ExploreLocation{{Preference: e.Preference, Selected: e.Selected, Object: original, Tab: 3}}
	m.openExploreSelection(0)
	if len(e.Trail) != 2 {
		t.Fatal("opening a listed child lost the parent list")
	}
	m.updateExplore("esc")
	if e.Detail != nil || len(e.Trail) != 1 || m.focus != 1 {
		t.Fatal("child Esc skipped its list")
	}
	m.updateExplore("esc")
	if e.Detail == nil || e.Detail.GetUID() != original.GetUID() || e.Tab != 3 {
		t.Fatal("list Esc did not restore original Related detail")
	}
}

func TestRelatedSelectionKeepsIdentityAndWrappedConditionsVisible(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(3)
	e := m.explore
	e.DetailValid = true
	e.References = []kubernetes.Reference{{Resource: kubecommon.Resource{Kind: "HTTPRoute"}, Name: "checkout", Namespace: "shop", Relation: "Parent reference · https · Accepted=False · NotAllowedByListeners"}}
	text := ansi.Strip(m.exploreDetailBody(52, 24))
	for _, want := range []string{"HTTPRoute / checkout", "Accepted=False", "NotAllowedByListeners"} {
		if !strings.Contains(text, want) {
			t.Fatalf("related view hid %q: %s", want, text)
		}
	}
}

func TestSecretValueRevealDoesNotCopyUntilConfirmed(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	m.openExploreSelection(0)
	e.DetailValid = true
	m.openResourceActions("")
	a := e.Action
	a.Pending = ""
	a.Resource = kubecommon.Resource{Name: "secrets"}
	a.Object = &unstructured.Unstructured{Object: map[string]any{"data": map[string]any{"token": "c2VjcmV0LXZhbHVl"}}}
	m.startResourceAction(monitor.ResourceAction{Id: "value"})
	if len(a.Values) != 1 || a.Values[0].Text != "secret-value" {
		t.Fatal("value not decoded")
	}
	if cmd := m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil || a.Stage != "valueview" {
		t.Fatal("revealing a value copied it")
	}
	if cmd := m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil || a.Stage != "values" {
		t.Fatal("default Back control copied a value")
	}
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyRight})
	if cmd := m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd == nil {
		t.Fatal("explicit copy did not produce clipboard command")
	}
}

func TestCertificateValuesExportDecodedPEMWithoutOverwriting(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(0)
	m.explore.DetailValid = true
	m.openResourceActions("")
	a := m.explore.Action
	a.Pending = ""
	a.Resource = kubecommon.Resource{Name: "certificatesigningrequests", Group: "certificates.k8s.io"}
	request := "-----BEGIN CERTIFICATE REQUEST-----\nfixture\n-----END CERTIFICATE REQUEST-----\n"
	a.Object = &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "request-one"},
		"spec":     map[string]any{"request": base64.StdEncoding.EncodeToString([]byte(request))},
	}}
	m.startResourceAction(monitor.ResourceAction{Id: "value"})
	if len(a.Values) != 1 || a.Values[0].Text != request {
		t.Fatal("request was not decoded or an absent issued certificate was invented")
	}
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if a.Stage != "export" || a.Input.Text != "request-one-request.pem" {
		t.Fatal("certificate export did not open its path editor")
	}
	path := filepath.Join(t.TempDir(), "request.pem")
	a.Input = textview.NewField(path)
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	cmd := m.syncResourceAction()
	if cmd == nil {
		t.Fatal("explicit export did not submit")
	}
	m.updateResourceActionResult(cmd().(monitor.ResourceActionResult))
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != request || a.Stage != "done" {
		t.Fatalf("PEM export failed: %q %v", contents, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("export permissions are not private")
	}
	a.Pending, a.ExportText = "export", "changed"
	m.updateResourceActionResult(m.syncResourceAction()().(monitor.ResourceActionResult))
	contents, _ = os.ReadFile(path)
	if a.Err == "" || string(contents) != request {
		t.Fatal("certificate export overwrote an existing file")
	}
}

func TestLogTimestampTogglePreservesPausedBuffer(t *testing.T) {
	text := "2026-09-30T03:00:00.123Z line one\nnot a timestamp\n"
	if got := monitor.DisplayLogText(text, false); got != "line one\nnot a timestamp\n" {
		t.Fatalf("unexpected hidden timestamps %q", got)
	}
	m := exploreModel(t)
	e := m.explore
	m.openExploreSelection(4)
	m.podLogViewer().Append(text)
	e.LogKey = "retained"
	m.podLogViewer().Pause()
	m.updateExplore("t")
	if m.podLogViewer().Following || e.LogKey != "retained" || m.podLogViewer().Text != text || !e.LogTimestamps {
		t.Fatal("timestamp display change restarted or erased paused logs")
	}
}

func TestDetailActionKeysPreserveSectionsAndGlobalPrefixes(t *testing.T) {
	for key, operation := range map[string]string{"x": "exec", "S": "scale", "R": "restart", "E": "edit", "D": "delete"} {
		t.Run(key, func(t *testing.T) {
			m := exploreModel(t)
			m.openExploreSelection(0)
			m.explore.Resource = kubecommon.Resource{Name: "deployments", Group: "apps"}
			m.explore.DetailValid = true
			m.Update(tea.KeyPressMsg{Code: rune(key[0]), Text: key})
			a := m.explore.Action
			if a == nil || !a.Requested || a.Items[a.Cursor].Id != operation {
				t.Fatalf("%s did not open %s", key, operation)
			}
		})
	}
	m := exploreModel(t)
	m.openExploreSelection(0)
	for key, tab := range map[string]int{"d": 0, "y": 1, "e": 2, "r": 3} {
		typeText(m, key)
		if m.explore.Tab != tab || m.explore.Action != nil {
			t.Fatalf("%s lost its detail-section binding", key)
		}
	}
	typeText(m, "s")
	if m.shortcutPrefix != "s" || m.explore.Action != nil {
		t.Fatal("resource actions consumed the Settings prefix")
	}
}

func TestSingleLetterActionCommandsKeepOnlyCurrentAliases(t *testing.T) {
	for _, item := range monitor.ResourceActionDefinitions {
		if actionCommand(item.Alias) != item.Id || actionCommand(item.Command) != item.Id || len(item.Alias) != 1 {
			t.Fatalf("invalid short action binding: %#v", item)
		}
		if global, _ := navigationShortcut(item.Alias); global != "" || canonicalCommand(item.Alias) != item.Alias {
			t.Fatalf("action alias collides with global: %s", item.Alias)
		}
	}
	for _, old := range []string{"ac", "lg", "ex", "sl", "rr", "ed", "dl", "ci", "cv", "xy", "un"} {
		if actionCommand(old) != "" {
			t.Fatalf("retired command %s is still active", old)
		}
	}
}

func TestDirectActionEscapeReturnsToOriginWithoutAnExtraMenu(t *testing.T) {
	for _, stage := range []string{"scale", "export", "pods", "confirm"} {
		t.Run(stage, func(t *testing.T) {
			m := exploreModel(t)
			m.openExploreSelection(1)
			m.explore.DetailValid = true
			m.explore.Scroll = 12
			m.openResourceActions("delete")
			a := m.explore.Action
			a.Pending, a.Stage, a.Operation = "", stage, "delete"
			m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEscape})
			if m.explore.Action != nil || m.explore.Detail == nil || m.explore.Tab != 1 || m.explore.Scroll != 12 {
				t.Fatal("direct action Escape lost the originating detail")
			}
		})
	}
}

func TestDetailActionBarShowsCommonOperationsAndFitsNarrowViews(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(0)
	m.explore.Resource = kubecommon.Resource{Name: "deployments", Group: "apps"}
	text := ansi.Strip(m.resourceActionBar(110))
	for _, label := range []string{"[l] Logs", "[x] Exec", "[S] Scale", "[R] Restart", "[E] Edit", "[D] Delete", "[a] More"} {
		if !strings.Contains(text, label) {
			t.Fatalf("common action hidden: %s in %s", label, text)
		}
	}
	for _, width := range []int{20, 32, 52, 80, 110} {
		text := m.resourceActionBar(width)
		if len(strings.Split(text, "\n")) > 2 || !strings.Contains(ansi.Strip(text), "More") {
			t.Fatalf("invalid narrow action bar %d: %s", width, text)
		}
		for _, line := range strings.Split(text, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("action bar overflows %d columns", width)
			}
		}
	}
	m.explore.Resource = kubecommon.Resource{Name: "services"}
	text = ansi.Strip(m.resourceActionBar(110))
	for _, label := range []string{"Logs", "Exec", "Scale", "Restart"} {
		if strings.Contains(text, label) {
			t.Fatalf("Service gained unsupported %s", label)
		}
	}
}

func TestActionErrorsStayVisibleWithControlsAtEveryTerminalSize(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(0)
	m.explore.DetailValid = true
	m.openResourceActions("")
	a := m.explore.Action
	a.Pending = ""
	a.Busy = false
	a.Operation = "delete"
	a.Input = textview.NewField(strings.Repeat("metadata:\n  name: example\n", 40))
	a.Proposed = a.Object.DeepCopy()
	a.Scale = &unstructured.Unstructured{Object: map[string]any{"spec": map[string]any{"replicas": int64(1)}}}
	a.Err = "The resource changed since it was opened. Return to its details and review the current resource before trying again."
	for _, size := range [][2]int{{80, 24}, {120, 32}, {160, 40}, {200, 50}} {
		m.width, m.height = size[0], size[1]
		for _, stage := range []string{"confirm", "scale", "export"} {
			a.Stage = stage
			view := ansi.Strip(m.render())
			for _, text := range []string{"resource changed since it was opened", "trying again.", "UID one", "Esc"} {
				if !strings.Contains(view, text) {
					t.Fatalf("%v %s clipped %q: %s", size, stage, text, view)
				}
			}
			if stage == "confirm" && !strings.Contains(view, "Confirm delete") {
				t.Fatalf("%v confirmation controls disappeared", size)
			}
			if stage == "review" && !strings.Contains(view, "Save to Kubernetes") {
				t.Fatalf("%v review controls disappeared", size)
			}
			lines := strings.Split(view, "\n")
			if len(lines) > m.height {
				t.Fatalf("%v %s error caused height overflow", size, stage)
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > m.width {
					t.Fatalf("%v %s error caused width overflow", size, stage)
				}
			}
		}
	}
}

func TestAuditTerminalCloseUnblocksPendingTerminalReply(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tm := &monitor.ResourceTerminal{Emulator: vt.NewSafeEmulator(80, 24), Cancel: cancel}
	t.Cleanup(tm.Close)
	finished := make(chan struct{})
	go func() {
		// A cursor query sends a reply through the input pipe. No reader is
		// attached, which exercises closing while the output writer is blocked.
		_, _ = tm.Emulator.Write([]byte("\x1b[6n"))
		close(finished)
	}()
	select {
	case <-finished:
		t.Fatal("cursor reply unexpectedly had an input consumer")
	case <-time.After(10 * time.Millisecond):
	}
	tm.Close()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("closing the terminal deadlocked a pending reply")
	}
	if ctx.Err() == nil {
		t.Fatal("closing the terminal left its network context active")
	}
}

func TestAuditActionButtonsAcceptHelpAndRestoreTheirState(t *testing.T) {
	for _, stage := range []string{"menu", "confirm", "review", "done"} {
		t.Run(stage, func(t *testing.T) {
			m := exploreModel(t)
			m.openExploreSelection(0)
			m.explore.DetailValid = true
			m.openResourceActions("")
			a := m.explore.Action
			a.Pending, a.Stage = "", stage
			m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
			m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
			if m.help == nil {
				t.Fatal("Help shortcut was swallowed by a resource action's navigation controls")
			}
			press(t, m, "esc")
			if m.help != nil || m.explore.Action != a || a.Stage != stage {
				t.Fatal("Help did not restore the retained action state")
			}
		})
	}
}

func TestAuditExecDoubleQuotedArgumentPreservesLiteralBackslash(t *testing.T) {
	args, err := monitor.SplitExecCommand(`/bin/sh -c "printf '\n'"`)
	if err != nil || len(args) != 3 || args[2] != `printf '\n'` {
		t.Fatalf("quoted command was changed: %#v %v", args, err)
	}
}

func progressModel(t *testing.T) (*model, *monitor.ResourceActionState) {
	t.Helper()
	m := exploreModel(t)
	m.openExploreSelection(0)
	m.explore.DetailValid = true
	m.openResourceActions("")
	a := m.explore.Action
	a.Pending = ""
	a.Stage = "done"
	a.Progress = &monitor.ResourceProgress{Status: kubernetes.OperationProgress{State: "waiting", Summary: "Deletion in progress", Details: "Waiting on finalizers: test.example/hold"}, Deadline: time.Now().Add(time.Minute)}
	a.Note = "Deletion requested."
	return m, a
}

func TestObservationEscapeReturnsToDetailsAndIgnoresLateResults(t *testing.T) {
	m, a := progressModel(t)
	ctx, cancel := context.WithCancel(context.Background())
	a.Cancel = cancel
	a.Progress.Checking = true
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if ctx.Err() == nil || m.explore.Action != nil || m.explore.Detail == nil {
		t.Fatal("Esc did not stop observation and retain details")
	}
	m.updateResourceProgress(monitor.ResourceProgressResult{Action: a, Progress: a.Progress, Status: kubernetes.OperationProgress{State: "complete"}})
	if m.explore.Action != nil {
		t.Fatal("late progress reopened action")
	}
}

func TestObservationTimeoutAndRecheckNeverResubmit(t *testing.T) {
	m, a := progressModel(t)
	a.Progress.Deadline = time.Now().Add(-time.Second)
	if cmd := m.syncResourceProgress(a); cmd != nil || a.Progress.Status.Watching() || !strings.Contains(a.Progress.Status.Details, "does not mean") {
		t.Fatal("timeout misreported operation failure")
	}
	m.updateResourceActionKey(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if !a.Progress.Status.Watching() || a.Pending != "" || a.Progress.Deadline.Before(time.Now()) {
		t.Fatal("recheck did not restart only observation")
	}
	old := a.Progress
	a.Progress = &monitor.ResourceProgress{Status: kubernetes.OperationProgress{State: "waiting"}}
	m.updateResourceProgress(monitor.ResourceProgressResult{Action: a, Progress: old, Status: kubernetes.OperationProgress{State: "complete"}})
	if !a.Progress.Status.Watching() {
		t.Fatal("stale observer changed new result")
	}
}

func TestProgressOverlayRetainsTargetStatusAndControlsAcrossSizes(t *testing.T) {
	m, a := progressModel(t)
	for _, size := range [][2]int{{80, 24}, {120, 32}, {220, 50}} {
		m.width, m.height = size[0], size[1]
		view := ansi.Strip(m.render())
		for _, want := range []string{"UID one", "Deletion requested.", "Deletion in progress", "test.example/hold", "Enter/Esc details"} {
			if !strings.Contains(view, want) {
				t.Fatalf("%v missing %q: %s", size, want, view)
			}
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > m.width {
				t.Fatal("width overflow")
			}
		}
		if len(strings.Split(view, "\n")) > m.height {
			t.Fatal("height overflow")
		}
	}
	a.Progress.Status = kubernetes.OperationProgress{State: "unavailable", Summary: "Status unavailable"}
	if !strings.Contains(ansi.Strip(m.render()), "g check again") {
		t.Fatal("missing recovery control")
	}
}

func TestObservationCancelsPendingReadAndDoesNotScheduleDuplicates(t *testing.T) {
	m, a := progressModel(t)
	a.Progress.Next = time.Now().Add(time.Hour)
	cmd := m.syncResourceProgress(a)
	if cmd == nil || m.syncResourceProgress(a) != nil {
		t.Fatal("observer was missing or scheduled twice")
	}
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if msg := cmd(); msg != nil {
		t.Fatal("closed observer performed a read")
	}
}

func TestProgressScrollClampsAndStripsServerControlSequences(t *testing.T) {
	m, a := progressModel(t)
	a.Progress.Status.Details = strings.Repeat("long controller message\n", 50) + "last line\x1b[2J"
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEnd})
	view := monitor.RenderOperationProgress(a, 60, 8)
	if !strings.Contains(ansi.Strip(view), "last line") || strings.Contains(view, "\x1b[2J") {
		t.Fatal("end or terminal sanitization failed")
	}
	end := a.Scroll
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyUp})
	_ = monitor.RenderOperationProgress(a, 60, 8)
	if a.Scroll != end-1 {
		t.Fatal("scrolling up remained stuck at the end")
	}
}
