package app

import (
	"context"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/simjay/plous/internal/kubernetes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func actionIDs(r kubernetes.Resource) []string {
	object := &unstructured.Unstructured{Object: map[string]any{}}
	var ids []string
	for _, item := range actionsFor(r, object) {
		ids = append(ids, item.id)
	}
	return ids
}

func TestActionsRespectFullResourceIdentityAndNativeSemantics(t *testing.T) {
	for _, r := range []kubernetes.Resource{{Name: "services"}, {Name: "ingresses", Group: "networking.k8s.io"}, {Name: "pods", Group: "example.io"}} {
		for _, id := range []string{"logs", "exec", "scale", "restart"} {
			if slices.Contains(actionIDs(r), id) {
				t.Fatalf("%s gained %s", r.Key(), id)
			}
		}
	}
	if slices.Contains(actionIDs(kubernetes.Resource{Name: "pods"}), "restart") || slices.Contains(actionIDs(kubernetes.Resource{Name: "daemonsets", Group: "apps"}), "scale") {
		t.Fatal("invented Pod restart or DaemonSet replica scale")
	}
	for _, key := range []string{"deployments", "statefulsets"} {
		ids := actionIDs(kubernetes.Resource{Name: key, Group: "apps"})
		for _, id := range []string{"logs", "exec", "scale", "restart", "edit", "delete"} {
			if !slices.Contains(ids, id) {
				t.Fatalf("%s missing %s", key, id)
			}
		}
	}
	if slices.Contains(actionIDs(kubernetes.Resource{Name: "events", Group: "events.k8s.io"}), "edit") {
		t.Fatal("events gained unsupported editor")
	}
	custom := kubernetes.Resource{Name: "widgets", Group: "example.io", Subresources: map[string][]string{"scale": {"get", "update"}}}
	if !slices.Contains(actionIDs(custom), "scale") {
		t.Fatal("served custom scale is unavailable")
	}
}

func TestActionTargetSnapshotSurvivesWatchAndListSelection(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	m.openExploreSelection(0)
	e.detailValid = true
	m.openResourceActions("")
	a := e.action
	captured := a.object.GetUID()
	e.selected = "two"
	e.detail = e.collection.Objects[1].DeepCopy()
	if a.object.GetUID() != captured || a.object.GetUID() == e.detail.GetUID() {
		t.Fatal("open action was retargeted by a new selection")
	}
	m.stopResourceAction()
	m.updateResourceActionResult(resourceActionResult{action: a, serial: a.serial, job: "prepare", object: e.detail.DeepCopy()})
	if e.action != nil {
		t.Fatal("stale action completion reopened a cancelled form")
	}
}

func TestActionFooterPreservesPendingShortcutAndBlockedCommand(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(0)
	m.explore.detailValid = true
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
	e.detailValid = true
	m.openResourceActions("")
	a := e.action
	a.stage = "confirm"
	a.operation = "delete"
	a.pending = ""
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if a.pending != "" || a.stage != "menu" {
		t.Fatal("default confirmation mutated a resource")
	}
	a.stage = "confirm"
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyRight})
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if a.pending != "delete" {
		t.Fatal("explicit confirmation did not queue delete")
	}
}

func TestActionOverlayFitsTerminalAndKeepsTargetVisible(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	m.openExploreSelection(0)
	e.detailValid = true
	m.openResourceActions("")
	e.action.pending = ""
	for _, size := range [][2]int{{80, 24}, {120, 32}, {160, 40}, {200, 50}} {
		m.width, m.height = size[0], size[1]
		for _, stage := range []string{"menu", "confirm", "edit"} {
			e.action.stage = stage
			e.action.operation = "delete"
			e.action.input = newEditor("apiVersion: v1\nkind: Pod\n")
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

func TestExecArgumentsAreParsedWithoutLocalShellExpansion(t *testing.T) {
	args, err := splitExecCommand(`/bin/sh -c 'echo "$HOME"; printf "hello world"'`)
	if err != nil || len(args) != 3 || args[2] != `echo "$HOME"; printf "hello world"` {
		t.Fatalf("arguments lost quoting: %#v %v", args, err)
	}
	args, err = splitExecCommand(`echo $(touch /tmp/never)`)
	if err != nil || strings.Join(args, " ") != `echo $(touch /tmp/never)` {
		t.Fatal("command substitution was interpreted locally")
	}
	for _, input := range []string{"", `sh 'unterminated`, `sh trailing\`} {
		if _, err := splitExecCommand(input); err == nil {
			t.Fatalf("invalid command accepted: %q", input)
		}
	}
}

func TestExecCapturedEscTabAndColonRemainRemote(t *testing.T) {
	m := exploreModel(t)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	tm := &resourceTerminal{emulator: vt.NewSafeEmulator(40, 10), cancel: cancel, captured: true}
	m.explore.terminal = tm
	output := make(chan string, 1)
	go func() {
		buf := make([]byte, 32)
		var text strings.Builder
		for text.Len() < 3 {
			n, err := tm.emulator.Read(buf)
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
	if !tm.captured || m.focus != 1 || m.command != nil {
		t.Fatal("remote keys executed Plous navigation")
	}
	m.Update(tea.KeyPressMsg{Code: ']', Mod: tea.ModCtrl})
	if tm.captured {
		t.Fatal("explicit terminal escape did not release input")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.explore.terminal != nil {
		t.Fatal("terminal controls did not return to details")
	}
}

func TestTerminalBufferCancellationUnblocksRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	input := &terminalInput{ctx: ctx, chunks: make(chan []byte, 2)}
	_, _ = input.Write([]byte("hello"))
	buf := make([]byte, 2)
	if n, _ := input.Read(buf); n != 2 || string(buf) != "he" {
		t.Fatal("input lost partial read")
	}
	buf = make([]byte, 4)
	if n, _ := input.Read(buf); string(buf[:n]) != "llo" {
		t.Fatal("input remainder lost")
	}
	cancel()
	if _, err := input.Read(buf); err != io.EOF {
		t.Fatal("cancelled terminal input still blocked")
	}
}

func TestRelatedListAndChildDetailHaveSeparateBackSteps(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	original := e.collection.Objects[0].DeepCopy()
	e.trail = []exploreLocation{{preference: e.preference, selected: e.selected, object: original, tab: 3}}
	m.openExploreSelection(0)
	if len(e.trail) != 2 {
		t.Fatal("opening a listed child lost the parent list")
	}
	m.updateExplore("esc")
	if e.detail != nil || len(e.trail) != 1 || m.focus != 1 {
		t.Fatal("child Esc skipped its list")
	}
	m.updateExplore("esc")
	if e.detail == nil || e.detail.GetUID() != original.GetUID() || e.tab != 3 {
		t.Fatal("list Esc did not restore original Related detail")
	}
}

func TestRelatedSelectionKeepsIdentityAndWrappedConditionsVisible(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(3)
	e := m.explore
	e.detailValid = true
	e.references = []kubernetes.Reference{{Resource: kubernetes.Resource{Kind: "HTTPRoute"}, Name: "checkout", Namespace: "shop", Relation: "Parent reference · https · Accepted=False · NotAllowedByListeners"}}
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
	e.detailValid = true
	m.openResourceActions("")
	a := e.action
	a.pending = ""
	a.resource = kubernetes.Resource{Name: "secrets"}
	a.object = &unstructured.Unstructured{Object: map[string]any{"data": map[string]any{"token": "c2VjcmV0LXZhbHVl"}}}
	m.startResourceAction(resourceAction{id: "value"})
	if len(a.values) != 1 || a.values[0].text != "secret-value" {
		t.Fatal("value not decoded")
	}
	if cmd := m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil || a.stage != "valueview" {
		t.Fatal("revealing a value copied it")
	}
	if cmd := m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil || a.stage != "values" {
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
	m.explore.detailValid = true
	m.openResourceActions("")
	a := m.explore.action
	a.pending = ""
	a.resource = kubernetes.Resource{Name: "certificatesigningrequests", Group: "certificates.k8s.io"}
	request := "-----BEGIN CERTIFICATE REQUEST-----\nfixture\n-----END CERTIFICATE REQUEST-----\n"
	a.object = &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "request-one"},
		"spec":     map[string]any{"request": base64.StdEncoding.EncodeToString([]byte(request))},
	}}
	m.startResourceAction(resourceAction{id: "value"})
	if len(a.values) != 1 || a.values[0].text != request {
		t.Fatal("request was not decoded or an absent issued certificate was invented")
	}
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if a.stage != "export" || a.input.text != "request-one-request.pem" {
		t.Fatal("certificate export did not open its path editor")
	}
	path := filepath.Join(t.TempDir(), "request.pem")
	a.input = newEditor(path)
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	cmd := m.syncResourceAction()
	if cmd == nil {
		t.Fatal("explicit export did not submit")
	}
	m.updateResourceActionResult(cmd().(resourceActionResult))
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != request || a.stage != "done" {
		t.Fatalf("PEM export failed: %q %v", contents, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("export permissions are not private")
	}
	a.pending, a.exportText = "export", "changed"
	m.updateResourceActionResult(m.syncResourceAction()().(resourceActionResult))
	contents, _ = os.ReadFile(path)
	if a.err == "" || string(contents) != request {
		t.Fatal("certificate export overwrote an existing file")
	}
}

func TestRevisionValueCopiesOnlyImmutableSnapshot(t *testing.T) {
	object := unstructured.Unstructured{Object: map[string]any{"metadata": map[string]any{"name": "revision"}, "data": map[string]any{"spec": map[string]any{"image": "checkout:v1"}}}}
	values := resourceValues(kubernetes.Resource{Name: "controllerrevisions", Group: "apps"}, object)
	if len(values) != 1 || !strings.Contains(values[0].text, "checkout:v1") || strings.Contains(values[0].text, "metadata") {
		t.Fatalf("unexpected revision snapshot: %#v", values)
	}
}

func TestLogTimestampTogglePreservesPausedBuffer(t *testing.T) {
	text := "2026-09-30T03:00:00.123Z line one\nnot a timestamp\n"
	if got := displayLogText(text, false); got != "line one\nnot a timestamp\n" {
		t.Fatalf("unexpected hidden timestamps %q", got)
	}
	m := exploreModel(t)
	e := m.explore
	m.openExploreSelection(4)
	m.podLogViewer().Append(text)
	e.logKey = "retained"
	m.podLogViewer().Pause()
	m.updateExplore("t")
	if m.podLogViewer().Following || e.logKey != "retained" || m.podLogViewer().Text != text || !e.logTimestamps {
		t.Fatal("timestamp display change restarted or erased paused logs")
	}
}
