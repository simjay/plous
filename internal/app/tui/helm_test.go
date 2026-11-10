package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	helmcommon "github.com/simjay/plous/internal/integrations/helm/common"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
)

func helmModel(t *testing.T) (*model, *monitor.HelmState) {
	t.Helper()
	m, _ := testModel(t)
	createWorkspace(t, m, "Commerce")
	path := contextFile(t, "staging", "production")
	w := *m.current()
	w.Connections = []workspace.Connection{{Tool: "helm", Name: "staging", Settings: map[string]string{"kubeconfig": path, "context": "staging", "namespace": "apps"}}}
	if !m.saveWorkspace(w) {
		t.Fatal(m.status)
	}
	m.selectScope(m.active, "helm", m.current().Connections[0].ID)
	m.mode, m.focus = 1, 1
	m.clusterFor(m.active, *m.savedConnection()).HelmDriver = "secret"
	h := m.helmer()
	client, err := helmtool.New(h.Config)
	if err != nil {
		t.Fatal(err)
	}
	h.Client = client
	h.Loaded = true
	h.Preference.Namespace = "apps"
	return m, h
}

func TestHelmContextSetupAndTwoSections(t *testing.T) {
	m, _ := testModel(t)
	path := contextFile(t, "staging")
	installDefaultKubeconfig(t, path)
	createWorkspace(t, m, "Commerce")
	openTool(t, m, "kubernetes")
	if m.form == nil || m.form.Kind != "kube" || m.toolSetup.Tool != "kubernetes" {
		t.Fatal("Helm did not use shared context picker")
	}
	m.chosen[0] = true
	m.addContexts()
	if len(m.current().Connections) != 1 || m.current().Connections[0].Tool != "kubernetes" {
		t.Fatal("shared cluster connection was not saved")
	}
	c := m.current().Connections[0]
	for _, row := range m.treeRowsExpanded(true) {
		if row.kind == "capability" && row.tool == "helm" {
			t.Fatal("setup pre-populated Helm without discovery")
		}
	}
	m.clusterFor(m.active, c).HelmDriver = "secret"
	m.expanded[m.active] = true
	m.expanded[c.ID+"/view/helm"] = true
	m.expanded[c.ID] = true
	count := 0
	for _, row := range m.treeRows() {
		if row.kind == "helmsection" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("sections %d", count)
	}
	if m.connection != "" {
		t.Fatal("saving setup selected a live connection")
	}
}

func TestHelmChartGroupingAndReleaseIdentity(t *testing.T) {
	m, h := helmModel(t)
	h.Releases = []helmtool.Release{{Name: "one", Namespace: "apps", Revision: 3, Chart: "demo", Version: "1.0.0", Status: "failed"}, {Name: "two", Namespace: "other", Revision: 1, Chart: "demo", Version: "1.0.0"}, {Name: "third", Namespace: "apps", Revision: 1, Chart: "demo", Version: "2.0.0"}}
	m.chooseHelmSection("Charts")
	if len(h.ChartGroups()) != 1 {
		t.Fatal(h.ChartGroups())
	}
	if len(h.ReleaseRows()) != 3 {
		t.Fatal("group lost releases")
	}
	h.Cursor = 2
	m.updateHelm("enter")
	if h.Preference.Selected != "other/two" || h.Pending != "detail" {
		t.Fatal("selected wrong release")
	}
}

func TestHelmRetiredDiscoverRestoresReleaseScope(t *testing.T) {
	m, h := helmModel(t)
	h.Preference.Section, h.Preference.Filter, h.Preference.Selected = "Discover", "shop", "apps/shop"
	m.rememberHelm(h)
	m.helmStates = nil
	restored := m.helmer()
	if restored.Preference.Section != "Releases" || restored.Preference.Namespace != "apps" || restored.Preference.Filter != "shop" || restored.Preference.Selected != "apps/shop" {
		t.Fatal("retired section lost the saved release scope", restored.Preference)
	}
	if m.chooseHelmSection("Discover") || m.executeHelm("install") {
		t.Fatal("retired Discover or installation controls remain reachable")
	}
}

func TestHelmDraftLiteralKeysAndHelpPreserveDocument(t *testing.T) {
	m, h := helmModel(t)
	m.beginHelmDraft(h)
	for _, field := range h.Draft.Form.Fields {
		if field.Key == "create_namespace" {
			t.Fatal("preview offered namespace creation")
		}
	}
	h.Draft.Form.Index = len(h.Draft.Form.Fields) + 1
	m.updateHelm("enter")
	d := h.Document
	typeText(m, "i")
	m.Update(tea.PasteMsg{Content: "# bb ii ss cc hh :wq\n"})
	if m.help != nil || m.page != "" || d.Mode != docedit.Insert || !strings.Contains(d.Text(), "bb ii") {
		t.Fatal("draft input dispatched navigation")
	}
	press(t, m, "esc")
	m.execute("help")
	press(t, m, "esc")
	if h.Document != d || !d.Dirty() {
		t.Fatal("help lost draft")
	}
	m.execute("w")
	if d.Dirty() || !strings.Contains(h.Draft.Values, "bb ii") || d.SavedStatus != "Saved to draft" {
		t.Fatal("did not save to draft")
	}
	if h.Pending != "" {
		t.Fatal("draft save scheduled a write")
	}
}

func TestHelmDraftDoesNotLeakIntoSessionOrContext(t *testing.T) {
	m, h := helmModel(t)
	m.beginHelmDraft(h)
	h.Draft.Values = "password: do-not-persist\n"
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	data, err := m.store.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "do-not-persist") || strings.Contains(m.contextText(), "do-not-persist") {
		t.Fatal("secret leaked")
	}
	var s session
	if err = json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	if s.Helm[m.active+"/"+m.connection].Section != "Releases" {
		t.Fatal("section was not persisted")
	}
}

func TestHelmRetainsScopeAndRejectsStaleResults(t *testing.T) {
	m, h := helmModel(t)
	h.Busy = true
	h.Run = 10
	h.Running = "detail"
	cancelled := false
	h.Cancel = func() { cancelled = true }
	m.chooseHelmSection("Charts")
	m.updateHelmResult(monitor.HelmResult{State: h, Run: 10, Kind: "detail", Snapshot: &helmtool.Snapshot{Name: "wrong"}})
	if !cancelled || h.Detail != nil || h.Preference.Section != "Charts" {
		t.Fatal("stale read overwrote navigation")
	}
}

func TestHelmRenderFitsTerminalSizes(t *testing.T) {
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 50}} {
		m, h := helmModel(t)
		m.width, m.height = size[0], size[1]
		for _, section := range monitor.HelmSections {
			h.Preference.Section = section
			view := m.render()
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("%s overflow %dx%d", section, lipgloss.Width(view), lipgloss.Height(view))
			}
		}
	}
}

func TestHelmActivityPreservesConfigurationHistory(t *testing.T) {
	m, h := helmModel(t)
	m.mode = 2
	completeActivity(t, m)
	before := len(m.activityUI.Changes)
	op, err := m.store.RecordOperation(workspace.Operation{WorkspaceID: m.active, ConnectionID: h.Connection, Tool: "helm", Context: "staging", Namespace: "apps", Release: "demo", Kind: "install", Status: "submitted"})
	if err != nil {
		t.Fatal(err)
	}
	m.loadHistory()
	completeActivity(t, m)
	m.mode = 2
	if len(m.activityUI.Changes) != before || len(m.activityEntries()) != before+1 {
		t.Fatal("operation altered config history")
	}
	m.openActivityEntry(m.activityEntries()[0])
	if !strings.Contains(m.detail, "Outcome unknown") {
		t.Fatal(m.detail)
	}
	op.Status = "completed"
	op.ObservedAt = op.ObservedAt.Add(1)
	if _, err = m.store.RecordOperation(op); err != nil {
		t.Fatal(err)
	}
	m.loadHistory()
	completeActivity(t, m)
	if len(m.activityUI.Operations) != 1 || m.activityUI.Operations[0].Status != "completed" {
		t.Fatal("outcome was not reconciled")
	}
}

func TestHelmValuesEditorDiscardAndReviewCannotExecute(t *testing.T) {
	m, h := helmModel(t)
	m.beginHelmDraft(h)
	h.Document = docedit.New(h.Draft.Values)
	h.Document.Paste("# changed\n")
	m.execute("review")
	if h.Comparison == nil {
		t.Fatal("review missing")
	}
	press(t, m, "enter")
	if h.Pending != "" {
		t.Fatal("review executed")
	}
	m.execute("q")
	if !h.Discard {
		t.Fatal("dirty close did not confirm")
	}
	press(t, m, "esc")
	if h.Document == nil {
		t.Fatal("cancel discarded draft")
	}
	m.execute("q!")
	if h.Document != nil {
		t.Fatal("explicit discard failed")
	}
}

func TestHelmErrorDoesNotMarkSuccess(t *testing.T) {
	m, h := helmModel(t)
	h.Busy = true
	h.Run = 1
	m.updateHelmResult(monitor.HelmResult{State: h, Run: 1, Kind: "execute", Outcome: "unknown", Err: context.DeadlineExceeded})
	if !strings.Contains(h.Notice, "unknown") || strings.Contains(h.Notice, "completed") {
		t.Fatal(h.Notice)
	}
	if strings.Contains(ansi.Strip(m.helmBody(80, 25)), "operation completed") {
		t.Fatal("rendered false success")
	}
}

func TestHelmConfirmationKeepsBothActionsVisible(t *testing.T) {
	m, h := helmModel(t)
	h.BeginWorkflow()
	h.Plan = &helmtool.Plan{Operation: "rollback", Context: strings.Repeat("long-context-", 4), Namespace: "apps", Name: "demo", CurrentRevision: 7, TargetRevision: 2}
	m.confirmHelm(h)
	for _, offset := range []int{0, 8, 100} {
		h.ConfirmScroll = offset
		view := m.helmBody(28, 10)
		if !strings.Contains(ansi.Strip(view), "Cancel") || !strings.Contains(ansi.Strip(view), "Rollback") || lipgloss.Height(view) > 10 || lipgloss.Width(view) > 28 {
			t.Fatalf("confirmation lost its actions: %q", view)
		}
	}
	m.updateHelm("enter")
	if h.Pending != "" {
		t.Fatal("default confirmation executed")
	}
	m.updateHelm("I")
	if h.Confirm {
		t.Fatal("install shortcut opened a rollback confirmation")
	}
	m.confirmHelm(h)
	m.updateHelm("right")
	m.updateHelm("enter")
	if h.Pending != "execute" {
		t.Fatal("explicit confirmation did not execute")
	}
}

func TestHelmActivityDoesNotRetargetEditedConnection(t *testing.T) {
	m, h := helmModel(t)
	op := &workspace.Operation{WorkspaceID: m.active, ConnectionID: h.Connection, ConnectionFingerprint: h.Config.Fingerprint(), Tool: "helm", Context: "staging", Namespace: "apps", Release: "demo", Kind: "revision", Revision: 2}
	m.operationDetail = op
	m.mode = 2
	m.currentConnection().Settings["kubeconfig"] = "/different/kubeconfig"
	m.openOperationRelease()
	if m.mode != 2 || !strings.Contains(m.status, "has changed") {
		t.Fatal("Activity opened through changed connection", m.status)
	}
	m.currentConnection().Settings["kubeconfig"] = h.Config.Kubeconfig
	m.openOperationRelease()
	if m.mode != 1 || h.RequestedRelease == nil || h.RequestedRelease.Revision != 2 {
		t.Fatal("Activity did not restore its original release identity")
	}
}

func TestHelmConnectionEditsPreserveUnknownSettings(t *testing.T) {
	m, _ := helmModel(t)
	c := m.currentConnection()
	c.Settings["older-setting"] = "keep-me"
	c.Settings["repository_cache"] = "/old/cache"
	m.beginConnection("helm", c)
	for i := range m.form.Fields {
		if m.form.Fields[i].Key == "repository_cache" {
			m.form.Fields[i].Value = textview.NewField("")
		}
	}
	m.submitForm()
	if m.currentConnection().Settings["older-setting"] != "keep-me" || m.currentConnection().Settings["repository_cache"] != "" {
		t.Fatal("connection edit lost older settings or retained a cleared field")
	}
}

func TestHelmAssistantCapturesReviewedPlanNamespace(t *testing.T) {
	m, h := helmModel(t)
	h.BeginWorkflow()
	h.Plan = &helmtool.Plan{Name: "demo", Namespace: "sandbox", Context: h.Config.Context, CurrentRevision: 4, Values: "password: hidden"}
	scope := m.assistantPageContext()
	page := m.assistantPageContext()
	if scope.Namespace != "sandbox" || page.Name != "demo" || page.Revision != 4 {
		t.Fatal("assistant lost the visible plan target", scope, page)
	}
	h.Plan.Namespace = "changed"
	if scope.Namespace != "sandbox" {
		t.Fatal("scope changed after capture")
	}
}

func TestHelmRetainedPlanDoesNotReplaceChartActions(t *testing.T) {
	m, h := helmModel(t)
	h.BeginWorkflow()
	h.Plan = &helmtool.Plan{Operation: "preview"}
	m.chooseHelmSection("Charts")
	m.helmActions(h)
	if len(h.Menu) == 0 || h.Menu[0].Action != monitor.HelmRefreshReleases {
		t.Fatal("an existing plan replaced Chart actions", h.Menu)
	}
	m.resumeHelmWorkflow(h)
	m.helmActions(h)
	for _, action := range h.Menu {
		if strings.HasPrefix(action.Label, "Execute ") {
			t.Fatal("chart-change preview offered an unsupported write", h.Menu)
		}
	}
}

func inspectorFixture() *helmtool.Snapshot {
	return &helmtool.Snapshot{Name: "shop", Namespace: "apps", Version: 2,
		Chart:  helmcommon.Chart{Metadata: map[string]any{"name": "demo"}, Values: map[string]any{"replicas": 1}},
		Config: map[string]any{"replicas": 2}, Effective: map[string]any{"replicas": 2, "password": "private-value"},
		Manifest: "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: web}\nspec:\n  replicas: 2\n  template:\n    spec:\n      containers:\n      - name: web\n        image: example/web:v2\n        ports: [{containerPort: 8080}]\n        env:\n        - name: CONFIG\n          valueFrom: {configMapKeyRef: {name: settings, key: options}}\n---\napiVersion: v1\nkind: Service\nmetadata: {name: web}\nspec: {ports: [{port: 80, targetPort: 8080}]}\n",
		History:  []helmtool.Revision{{Revision: 1, Status: "superseded"}, {Revision: 2, Status: "deployed"}},
	}
}

func TestHelmTimelinePagingKeepsActionIdentityVisible(t *testing.T) {
	m, h := helmModel(t)
	h.OpenTimeline(inspectorFixture())
	h.Timeline.Entries[2] = monitor.HelmTimelineEntry{Changes: &helmtool.RevisionChanges{Added: 12, Changes: []helmtool.RevisionChange{{Group: "resources", Label: strings.Repeat("long-resource-name-", 5), Action: "changed"}}}}
	h.Timeline.Entries[1] = monitor.HelmTimelineEntry{Changes: &helmtool.RevisionChanges{Added: 1}}
	for _, width := range []int{28, 50, 100} {
		for _, height := range []int{5, 9, 16} {
			for i := 0; i < 8; i++ {
				m.helmTimelineKey(h, "pgdown")
				view := ansi.Strip(monitor.HelmTimelineBody(h, width, height))
				if !strings.Contains(view, "› Revision 2") || lipgloss.Width(view) > width || lipgloss.Height(view) > height {
					t.Fatalf("lost target at %dx%d: %s", width, height, view)
				}
			}
			m.helmTimelineKey(h, "enter")
			if h.JobRevision != 2 || h.RequestedRelease.Revision != 2 {
				t.Fatal("inspect differs from visible selection")
			}
			h.Pending = ""
		}
	}
	m.helmTimelineKey(h, "down")
	m.helmTimelineKey(h, "pgdown")
	_ = monitor.HelmTimelineBody(h, 50, 8)
	m.rollbackHelmSelection(h)
	if h.JobRevision != 1 || h.Pending != "rollback" {
		t.Fatal("rollback differs from visible selection")
	}
}

func TestHelmRefreshPreservesSectionResourceAndDocumentPosition(t *testing.T) {
	m, h := helmModel(t)
	h.ApplyInspectorSnapshot(inspectorFixture())
	h.Subcursor = 1
	m.updateHelm("right")
	h.Subcursor = 9
	m.updateHelm("right")
	h.Subcursor = 4
	refreshed := inspectorFixture()
	refreshed.Manifest = "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: aaa}\n---\n" + refreshed.Manifest
	m.updateHelmResult(monitor.HelmResult{State: h, Run: h.Run, Kind: "detail", Snapshot: refreshed})
	if h.Tab != 1 || h.Subcursor != 4 {
		t.Fatal("refresh reset current document")
	}
	m.updateHelm("left")
	if h.Subcursor != 9 {
		t.Fatal("values scroll lost")
	}
	m.updateHelm("left")
	resources, _ := monitor.HelmResources(h.Detail)
	if resources[h.Subcursor].Kind != "Service" {
		t.Fatal("refresh followed old index instead of resource identity")
	}
	m.updateHelm("enter")
	if h.Document == nil || h.Menu != nil || !strings.Contains(h.Document.Text(), "targetPort: 8080") {
		t.Fatal("Enter did not open complete resource directly")
	}
	h.Document = nil
	m.updateHelm("right")
	m.updateHelm("enter")
	m.updateHelm("down")
	m.updateHelm("enter")
	if h.ValuesVariant != 1 || strings.Contains(monitor.HelmDetailText(h), "private-value") {
		t.Fatal("wrong values source or masking")
	}
	h.Subcursor = 2
	h.ChooseInspectorDocument("values", 0)
	if h.Subcursor != 9 {
		t.Fatal("variant reading position lost")
	}
}

func TestHelmInspectorPickerSearchIsLiteralAndMapsExactIdentity(t *testing.T) {
	m, h := helmModel(t)
	h.ApplyInspectorSnapshot(inspectorFixture())
	h.Releases = []helmtool.Release{{Name: "shop", Namespace: "apps", Chart: "demo"}, {Name: "worker", Namespace: "other", Chart: "demo", Revision: 8}}
	typeText(m, "ff")
	if !m.viewExpanded() || h.Menu != nil || h.Input != nil {
		t.Fatal("fullscreen opened the release picker")
	}
	mainCommand(t, m, "f")
	if m.viewExpanded() || h.Menu != nil {
		t.Fatal("fullscreen command did not restore the inspector")
	}
	mainCommand(t, m, "J")
	labels, _ := h.MenuChoices()
	if !strings.Contains(strings.Join(labels, "\n"), "apps/shop") || !strings.Contains(h.Menu[0].Label, "✓") {
		t.Fatal("current release not marked")
	}
	typeText(m, "worker")
	rows, _ := h.MenuChoices()
	if len(rows) != 1 {
		t.Fatal(rows)
	}
	m.helmKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if h.RequestedRelease.Key() != "other/worker" || h.RequestedRelease.Revision != 0 {
		t.Fatal("filtered picker selected wrong release")
	}
	h.Pending = ""
	h.RequestedRelease = nil
	typeText(m, "F")
	m.Update(tea.PasteMsg{Content: "bb :quit"})
	if m.leftHidden || m.command != nil || h.Input.Text != "bb :quit" {
		t.Fatal("picker input executed actions")
	}
	m.helmKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if h.Input == nil || h.Pending != "" {
		t.Fatal("empty filtered results selected a revision")
	}
	press(t, m, "esc")
	typeText(m, "F")
	typeText(m, "superseded")
	m.helmKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if h.JobRevision != 1 {
		t.Fatal("filtered revision picker selected wrong revision")
	}
}

func TestHelmFindStaysInPlaceAndSearchesLoadedDiffs(t *testing.T) {
	m, h := helmModel(t)
	h.OpenTimeline(inspectorFixture())
	h.Timeline.Entries[1] = monitor.HelmTimelineEntry{Changes: &helmtool.RevisionChanges{Diff: "+ unique-setting: true\n"}}
	typeText(m, "/")
	typeText(m, "unique-setting")
	if h.Timeline.Selected() != 1 || h.Document != nil {
		t.Fatal("timeline find did not select the matching revision")
	}
	press(t, m, "enter")
	typeText(m, "n")
	if h.Namespaces != nil {
		t.Fatal("find-next opened namespaces")
	}
	if !strings.Contains(ansi.Strip(m.helmBody(100, 26)), "unique-setting") {
		t.Fatal("matching diff not visible")
	}
	press(t, m, "esc")
	if h.Find != nil || h.Timeline.Selected() != 1 {
		t.Fatal("clearing search lost current revision")
	}
	h.ApplyInspectorSnapshot(inspectorFixture())
	typeText(m, "/")
	typeText(m, "Service")
	resources, _ := monitor.HelmResources(h.Detail)
	if resources[h.Subcursor].Kind != "Service" || h.Document != nil {
		t.Fatal("resource search left the inventory")
	}
	press(t, m, "esc")
	press(t, m, "right")
	typeText(m, "/")
	typeText(m, "replicas")
	if h.Document != nil || h.Find == nil {
		t.Fatal("values find opened a separate document")
	}
	view := ansi.Strip(m.helmBody(80, 24))
	if !strings.Contains(view, "matches") || strings.Contains(view, "private-value") {
		t.Fatal("missing match or leaked value", view)
	}
}

func TestHelmCompactFactsAndActionsFit(t *testing.T) {
	m, h := helmModel(t)
	h.ApplyInspectorSnapshot(inspectorFixture())
	resources, _ := monitor.HelmResources(h.Detail)
	facts := ansi.Strip(monitor.HelmResourceSummary(resources[0], "apps", 100))
	for _, want := range []string{"Images", "example/web:v2", "8080", "settings"} {
		if !strings.Contains(facts, want) {
			t.Fatal("missing fact", want, facts)
		}
	}
	if strings.Contains(facts, "valueFrom:") {
		t.Fatal("preview still dumps containers")
	}
	for _, tab := range []int{5, 2, 1} {
		h.Tab = tab
		m.helmActions(h)
		if len(h.Menu) > 7 {
			t.Fatal("oversized context menu", h.Menu)
		}
		h.Menu = nil
		for _, size := range [][2]int{{28, 10}, {50, 16}, {100, 26}} {
			view := m.helmBody(size[0], size[1])
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatal(fmt.Sprint(size), ansi.Strip(view))
			}
		}
	}
}

func TestHelmRefreshAndPausedWorkflowPreserveReadingState(t *testing.T) {
	m, h := helmModel(t)
	h.OpenTimeline(inspectorFixture())
	h.Timeline.Scroll = 4
	h.OpenTimeline(inspectorFixture())
	_ = monitor.HelmTimelineBody(h, 50, 8)
	if h.Timeline.Scroll != 4 {
		t.Fatal("loading refresh discarded timeline position")
	}
	h.ApplyInspectorSnapshot(inspectorFixture())
	h.Tab, h.ValuesVariant, h.Subcursor = 2, 2, 8
	h.SaveInspectorPosition()
	h.BeginWorkflow()
	m.leaveHelmWorkflow(h, false)
	other := inspectorFixture()
	other.Name = "another"
	h.ApplyInspectorSnapshot(other)
	m.resumeHelmWorkflow(h)
	m.leaveHelmWorkflow(h, true)
	if h.Detail.Name != "shop" || h.Tab != 2 || h.ValuesVariant != 2 || h.Subcursor != 8 {
		t.Fatal("paused action returned to wrong document or position")
	}
}
