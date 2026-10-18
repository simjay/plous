package app

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
