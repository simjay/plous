package app

import (
	"errors"
	"strings"
	"testing"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
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
	h.Menu = []string{"One", "Two", "Chosen action"}
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
