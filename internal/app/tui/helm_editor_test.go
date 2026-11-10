package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestHelmNamespaceFailureCannotDisplayOldScope(t *testing.T) {
	m, h := helmModel(t)
	h.Releases = []helmtool.Release{{Name: "old-release", Namespace: "apps"}}
	m.openHelmNamespacePicker(h)
	h.Namespaces.Manual = true
	*h.Input = textview.NewField("production")
	press(t, m, "enter")
	m.updateHelmResult(monitor.HelmResult{State: h, Run: h.Run, Kind: "list", Err: errors.New("access denied")})
	if len(h.Releases) != 0 || strings.Contains(ansi.Strip(m.helmBody(60, 20)), "old-release") {
		t.Fatal("failed namespace change displayed the previous inventory")
	}
}

func TestHelmNamespaceEditorDoesNotRetryPreviousFailedScope(t *testing.T) {
	m, h := helmModel(t)
	h.Loaded = false
	h.Err = "access denied"
	h.Preference.Namespace = "blocked"
	m.executeHelm("namespace")
	m.syncHelm()
	if h.Busy || h.Pending != "" {
		t.Fatal("opening the namespace editor started another read of the failed scope")
	}
	press(t, m, "esc")
	m.syncHelm()
	if h.Busy || h.Pending != "" || h.Err != "access denied" {
		t.Fatal("cancelling the namespace editor retried its failed read")
	}
}

func TestHelmRollbackPickerSelectsAnExplicitRevision(t *testing.T) {
	m, h := helmModel(t)
	h.Detail = &helmtool.Snapshot{Name: "shop", Namespace: "apps", Version: 2, History: []helmtool.Revision{{Revision: 1, Status: "superseded"}, {Revision: 2, Status: "deployed"}}}
	m.executeHelm("rollback")
	if h.MenuKind != "revision" || h.Choice != 0 || h.Pending != "" {
		t.Fatal("latest revision offered as rollback target")
	}
	m.updateHelm("down")
	m.updateHelm("enter")
	if h.Pending != "detail" || h.JobRevision != 1 {
		t.Fatal("picker did not request exact revision")
	}
	h.Pending = ""
	h.Detail.Version = 1
	m.executeHelm("rollback")
	if h.Pending != "rollback" || h.JobRevision != 1 || h.Confirm {
		t.Fatal("wrong revision was prepared")
	}
}

func TestHelmActivityLinkCancelsEarlierRead(t *testing.T) {
	m, h := helmModel(t)
	m.mode = 2
	m.operationDetail = &workspace.Operation{WorkspaceID: m.active, ConnectionID: h.Connection, Context: "staging", Namespace: "apps", Release: "wanted", Kind: "revision", Revision: 2}
	h.Busy, h.Running, h.Run = true, "detail", 5
	cancelled := false
	h.Cancel = func() { cancelled = true }
	m.openOperationRelease()
	m.updateHelmResult(monitor.HelmResult{State: h, Run: 5, Kind: "detail", Snapshot: &helmtool.Snapshot{Name: "wrong"}})
	if !cancelled || h.Detail != nil || h.RequestedRelease == nil || h.RequestedRelease.Name != "wanted" || h.Pending != "detail" {
		t.Fatal("an earlier read overrode the Activity target")
	}
}

func TestHelmCollectionPositionsSurviveSections(t *testing.T) {
	m, h := helmModel(t)
	h.Releases = []helmtool.Release{{Name: "one", Chart: "alpha", Version: "1"}, {Name: "two", Chart: "beta", Version: "1"}, {Name: "three", Chart: "gamma", Version: "1"}}
	h.Cursor = 2
	m.chooseHelmSection("Charts")
	h.Cursor = 1
	m.updateHelm("enter")
	m.updateHelm("esc")
	if h.Cursor != 1 {
		t.Fatal("return from chart inspector lost its card")
	}
	m.chooseHelmSection("Releases")
	if h.Cursor != 2 {
		t.Fatal("return to Releases lost its row")
	}
}

func TestHelmCompactViewsKeepSelectionAndContentVisible(t *testing.T) {
	m, h := helmModel(t)
	h.Preference.Section = "Releases"
	h.Detail = &helmtool.Snapshot{Name: "shop", Namespace: "apps", Manifest: "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: queuecfg}\n"}
	h.Tab = 5
	h.Notice = "Loaded release"
	if view := ansi.Strip(m.helmBody(28, 10)); !strings.Contains(view, "Resources") || !strings.Contains(view, "queuecfg") {
		t.Fatal(view)
	}
	h.Menu = monitor.HelmMenuItems([]string{"One", "Two", "Chosen action"})
	h.Choice = 2
	view := m.helmBody(28, 10)
	if !strings.Contains(ansi.Strip(view), "Chosen action") || lipgloss.Height(view) > 10 || lipgloss.Width(view) > 28 {
		t.Fatal("compact menu lost its selected action", view)
	}
}

func TestHelmDraftChecksRequiredInputsBeforeAcquisition(t *testing.T) {
	m, h := helmModel(t)
	m.beginHelmDraft(h)
	h.Draft.Form.Index = len(h.Draft.Form.Fields)
	m.updateHelm("enter")
	if h.Pending != "" || h.Draft.Form.Index != 0 || !strings.Contains(h.Err, "required") {
		t.Fatal("empty chart inputs reached acquisition")
	}
}

func TestHelmRevealedResourceStaysOutOfAssistantContext(t *testing.T) {
	m, h := helmModel(t)
	h.Detail = &helmtool.Snapshot{Name: "shop", Namespace: "apps", Manifest: "apiVersion: v1\nkind: Secret\nmetadata: {name: demo}\ndata: {password: private-content}\n"}
	h.Tab = 5
	m.updateHelm("a")
	h.Choice = 1
	m.updateHelm("enter")
	if h.Document == nil || !strings.Contains(h.Document.Text(), "private-content") {
		t.Fatal("explicit reveal did not expose the selected resource to the user")
	}
	if strings.Contains(m.contextText(), "private-content") {
		t.Fatal("revealed resource leaked into automatic assistant context")
	}
}

func TestHelmDraftReviewCannotSaveChangedDocument(t *testing.T) {
	m, h := helmModel(t)
	m.beginHelmDraft(h)
	h.Document = docedit.New(h.Draft.Values)
	h.Document.Paste("# reviewed\n")
	m.helmDocumentCommand("v")
	h.Document.Paste("# later edit\n")
	m.helmDocumentCommand("w")
	if h.Comparison == nil || !h.Comparison.Stale || strings.Contains(h.Draft.Values, "later edit") {
		t.Fatal("review accepted a different document")
	}
	h.Comparison = nil
	m.helmDocumentCommand("w")
	if !strings.Contains(h.Draft.Values, "later edit") || h.Document.Dirty() {
		t.Fatal("direct save could not recover after stale review")
	}
}

func TestHelmDetailSearchUsesMaskedInlineText(t *testing.T) {
	m, h := helmModel(t)
	h.Detail = &helmtool.Snapshot{Name: "shop", Effective: map[string]any{"password": "hidden-content", "replicas": 3}}
	h.Tab = 2
	m.updateHelm("/")
	if h.Document != nil || h.Find == nil || !h.Find.Reader.Searching || strings.Contains(monitor.HelmDetailText(h), "hidden-content") {
		t.Fatal("detail search did not use masked inline text")
	}
}

func valueModel(t *testing.T) (*model, *monitor.ResourceActionState) {
	t.Helper()
	m := exploreModel(t)
	m.openExploreSelection(0)
	m.explore.DetailValid = true
	m.openResourceActions("")
	a := m.explore.Action
	a.Pending = ""
	a.Resource = kubecommon.Resource{Name: "secrets", Kind: "Secret"}
	a.Object = &unstructured.Unstructured{Object: map[string]any{
		"metadata":   map[string]any{"name": "fixture", "namespace": "default"},
		"stringData": map[string]any{"config.yaml": "password: fixture-private\n" + strings.Repeat("entry: example\n", 70) + "last: value\n"},
	}}
	m.startResourceAction(monitor.ResourceAction{Id: "value"})
	press(t, m, "enter")
	return m, a
}

func TestValueDocumentUsesReadOnlyEditorAndClearsOnHide(t *testing.T) {
	m, a := valueModel(t)
	d := a.ValueDocument
	if !d.ReadOnly || d.Syntax != "yaml" || !strings.Contains(m.render(), "Plous Editor") {
		t.Fatal("value did not use the shared read-only Editor")
	}
	original := d.Text()
	m.Update(tea.PasteMsg{Content: "replacement"})
	typeText(m, "i")
	if d.Text() != original || d.Dirty() || d.Mode == docedit.Insert {
		t.Fatal("value inspection changed the document")
	}
	press(t, m, "esc")
	typeText(m, "/fixture-private")
	m.Update(tea.PasteMsg{Content: ":q bb"})
	if d.Query != "fixture-private:q bb" || m.command != nil || m.leftHidden {
		t.Fatal("value search dispatched literal input")
	}
	press(t, m, "esc")
	if a.ValueDocument != d || d.Query != "" {
		t.Fatal("closing search hid the value")
	}
	press(t, m, "end")
	if !strings.Contains(ansi.Strip(m.valueDocumentBody(80, 18)), "last: value") {
		t.Fatal("value did not share document scrolling")
	}
	typeText(m, "v")
	press(t, m, "up")
	if cmd := press(t, m, "enter"); cmd != nil || a.Stage != "valueview" {
		t.Fatal("reading Enter executed an action")
	}
	typeText(m, "y")
	if !strings.Contains(m.status, "clipboard") {
		t.Fatal("selected value text did not copy")
	}
	typeText(m, "v")
	press(t, m, "esc")
	press(t, m, "esc")
	if a.Stage != "values" || a.ValueDocument != nil || d.Text() != "" || d.Query != "" || d.SelectedText() != "" {
		t.Fatal("hiding a Secret left text in its Editor buffer")
	}
}

func TestValueDocumentExpansionHelpFocusAndLiteralPaste(t *testing.T) {
	m, a := valueModel(t)
	d := a.ValueDocument
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 50}} {
		m.width, m.height = size[0], size[1]
		view := m.render()
		if len(strings.Split(view, "\n")) > size[1] {
			t.Fatal("value frame height overflow")
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("value frame width overflow")
			}
		}
	}
	mainCommand(t, m, "f")
	if !m.viewExpanded() || m.leftHidden || m.rightHidden {
		t.Fatal("value expansion changed saved panel visibility")
	}
	mainCommand(t, m, "L")
	if d.LineNumbers {
		t.Fatal("value did not share line number controls")
	}
	typeText(m, "hh")
	if m.help == nil || !strings.Contains(m.panelHelpText(1), "Value (read only)") {
		t.Fatal("value help was not contextual")
	}
	press(t, m, "tab")
	press(t, m, "esc")
	if !m.viewExpanded() || a.ValueDocument != d {
		t.Fatal("Help lost the expanded document")
	}
	press(t, m, "alt+right")
	if m.focus != 2 || m.viewExpanded() || a.ValueDocument != d {
		t.Fatal("Tab did not preserve the value and restore the layout")
	}
	mainCommand(t, m, "c")
	if !m.chatEditing || a.ValueDocument != d {
		t.Fatal("value prevented chat entry")
	}
	m.stopResourceAction()
	if d.Text() != "" {
		t.Fatal("closing the action retained the value buffer")
	}
}
