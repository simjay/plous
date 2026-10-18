package app

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	appcode "github.com/simjay/plous/internal/app/code"
	"github.com/simjay/plous/internal/source"
)

func kustomizeBrowserModel(t *testing.T) (*model, *appcode.CodeState) {
	t.Helper()
	m, s := codeModel(t)
	fixture, err := source.Load(t.Context(), source.Config{Path: "../../tests/fixtures/code/inspector"})
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.Copy(filepath.Join(s.Config.Path, "inspector")); err != nil {
		t.Fatal(err)
	}
	s.Pending = "load"
	finishCode(t, m)
	openSourceView(t, m, "kustomize")
	s.Rescan = false
	if !s.SelectKustomizeNode("kustomize:inspector/clusters/prod-us-east") {
		t.Fatal("missing graph target")
	}
	return m, s
}

func TestKustomizeBrowserRenderKeepsExactOverlayAndReturnsToGraph(t *testing.T) {
	m, s := kustomizeBrowserModel(t)
	selected := s.Preference.BrowserItem
	for _, key := range []string{"right", "left", "enter", "esc"} {
		m.updateCode(key)
	}
	if len(s.Builds) != 0 || s.Pending != "" {
		t.Fatal("graph navigation rendered an entry")
	}
	s.SelectKustomizeNode(selected)
	s.Filter = "clusters/prod-us-east"
	s.RestoreCursor()
	m.executeCode("render")
	if s.RenderRoot != selected || s.Preference.Selected != selected || s.Menu != "" || s.Pending != "render" {
		t.Fatal("render retargeted the selection")
	}
	finishCode(t, m)
	build := s.Builds[selected]
	if build == nil || len(build.Resources) != 4 || len(s.Builds) != 1 {
		t.Fatal("did not render only the complete overlay", s.Err, s.Builds)
	}
	for _, resource := range build.Resources {
		if resource.Namespace != "production-us" || !strings.HasPrefix(resource.Name, "prod-") {
			t.Fatal("overlay was not applied", resource.Key())
		}
	}
	if !strings.Contains(ansi.Strip(m.codeBody(120, 40)), "Rendered: inspector/clusters/prod-us-east") {
		t.Fatal("output lost target label")
	}
	m.codeBrowserControl("back")
	if !s.BrowserDetail {
		t.Fatal("output did not return to overview")
	}
	m.updateCode("esc")
	if s.View != "code-kustomize" || s.Preference.BrowserItem != selected || s.Filter != "clusters/prod-us-east" {
		t.Fatal("back lost graph selection or filter")
	}
	if text := ansi.Strip(m.codeBody(120, 40)); !strings.Contains(text, "inspector") || !strings.Contains(text, "api") {
		t.Fatal("filtered graph lost dependencies", text)
	}
}

func TestKustomizeBrowserComponentAndBaseDoNotRenderImplicitly(t *testing.T) {
	m, s := kustomizeBrowserModel(t)
	s.SelectKustomizeNode("kustomize:inspector/components/tls")
	m.executeCode("render")
	if s.Pending != "" || len(s.RenderCandidates()) != 0 || s.Menu != "" {
		t.Fatal("component rendered a nearby source")
	}
	s.SelectKustomizeNode("kustomize:inspector/apps/cache/base")
	m.executeCode("render")
	if s.Pending != "" || !strings.Contains(s.Err, "Render Base Only") {
		t.Fatal("base rendered without explicit base intent")
	}
	m.updateCode("enter")
	m.executeCode("actions")
	s.Choice = slices.Index(s.MenuItems(), "Render Base Only")
	if s.Choice < 0 {
		t.Fatal("base-only action unavailable")
	}
	m.updateCode("enter")
	finishCode(t, m)
	build := s.Builds["kustomize:inspector/apps/cache/base"]
	if build == nil || len(build.Resources) != 1 || build.Resources[0].Namespace == "production-us" {
		t.Fatal("base-only action applied an overlay")
	}
}

func TestKustomizeBrowserGroupsMouseAndRestore(t *testing.T) {
	m, s := kustomizeBrowserModel(t)
	openSourceView(t, m, "kustomize")
	s.Rescan = false
	if !s.KustomizeDirectory() || s.Menu != "" {
		t.Fatal("sidebar did not open directory")
	}
	node := s.Preference.BrowserItem
	m.updateCode("down")
	if s.Preference.BrowserItem != node {
		t.Fatal("group navigation overwrote node selection")
	}
	m.executeCode("render")
	if s.Pending != "" || s.Busy || !s.KustomizeDirectory() {
		t.Fatal("directory rendered a hidden selection")
	}
	m.width, m.height = 180, 50
	m.execute("fullscreen")
	clickMouse(t, m, "main", "code-kustomize-group/inspector")
	item, _ := s.SelectedCatalogItem()
	if item.Group != "inspector" || s.Pending != "" || s.BrowserDetail {
		t.Fatal("choosing a group changed source operation")
	}
	clickMouse(t, m, "main", "code-kustomize/kustomize:inspector/components/tls")
	item, _ = s.SelectedCatalogItem()
	if item.EntryID != "kustomize:inspector/components/tls" || s.Pending != "" {
		t.Fatal("mouse node did not select component")
	}
	m.updateCode("esc")
	m.updateCode("esc")
	if !s.KustomizeDirectory() || s.KustomizeGroupItems()[s.Cursor].Path != "inspector" {
		t.Fatal("Back did not restore directory selection")
	}
	pref := s.Preference
	controller := appcode.Controller{Preferences: map[string]appcode.CodePreference{s.Workspace + "/" + s.Connection: pref}}
	restored := controller.StateFor(s.Workspace, s.Connection, s.Config, nil)
	restored.View, restored.Inventory = "code-kustomize", s.Inventory
	restored.RestoreCursor()
	if !restored.KustomizeDirectory() || restored.KustomizeGroupItems()[restored.Cursor].Path != "inspector" {
		t.Fatal("restart did not restore group selection in directory")
	}
	restored.OpenKustomizeGroup("inspector")
	item, _ = restored.SelectedCatalogItem()
	if item.ID != pref.BrowserItem {
		t.Fatal("restart lost selected graph node")
	}
}

func TestKustomizeNodeExplorerNavigationAndDirectOutput(t *testing.T) {
	m, s := kustomizeBrowserModel(t)
	m.width, m.height = 180, 50
	m.execute("fullscreen")
	id := "kustomize:inspector/clusters/prod-eu-west"
	clickMouse(t, m, "main", "code-kustomize/"+id)
	if !s.BrowserDetail || s.NodeGraph != "" || s.Busy || s.Pending != "" || s.Preference.Selected != id {
		t.Fatal("click did not open only overview")
	}
	overview := ansi.Strip(m.codeBody(120, 30))
	for _, want := range []string{"Overview", "Sets namespace to production-eu", "Graph", "Render", "Open source"} {
		if !strings.Contains(overview, want) {
			t.Fatal("missing overview", want, overview)
		}
	}
	m.execute("G")
	if s.NodeGraph != id || s.BrowserDetail || s.Busy {
		t.Fatal("Graph did not remain inspection-only")
	}
	graph := ansi.Strip(m.codeBody(120, 36))
	for _, bad := range []string{"Selected ·", "THIS NODE ADDS", "Last render", "crossing, no join", "Final output"} {
		if strings.Contains(graph, bad) {
			t.Fatal("graph contains prose", bad)
		}
	}
	child := "kustomize:inspector/apps/api/base"
	clickMouse(t, m, "main", "code-kustomize/"+child)
	if !s.BrowserDetail || s.Preference.Selected != child {
		t.Fatal("graph child did not open overview")
	}
	press(t, m, "esc")
	if s.NodeGraph != id || s.BrowserDetail {
		t.Fatal("Back lost anchored graph")
	}
	press(t, m, "esc")
	if !s.BrowserDetail || s.Preference.Selected != id {
		t.Fatal("Back lost original overview")
	}
	m.executeCode("describe")
	if s.Document == nil || s.Document.ReadOnly || s.DocumentPath != "inspector/clusters/prod-eu-west/kustomization.yaml" {
		t.Fatal("source action lost original file")
	}
	m.codeDocumentCommand("q")
	if s.Document != nil || !s.BrowserDetail || s.Preference.Selected != id {
		t.Fatal("editor did not return to overview")
	}
	m.execute("r")
	if s.View != "manifests" || s.Preference.Section != 1 || s.Pending != "render" {
		t.Fatal("render entered intermediate graph")
	}
	finishCode(t, m)
	if len(s.Builds[id].Resources) != 4 {
		t.Fatal("overlay output incomplete")
	}
	output := ansi.Strip(m.codeBody(160, 40))
	for _, want := range []string{"Kind", "Name", "Namespace", "production-eu", "Deployment"} {
		if !strings.Contains(output, want) {
			t.Fatal("unlabeled output", want, output)
		}
	}
	for _, bad := range []string{"[ Graph ]", "[ Output ]", "0 errors"} {
		if strings.Contains(output, bad) {
			t.Fatal("redundant output chrome", bad)
		}
	}
	codeAction(t, m, "Node overview")
	if !s.BrowserDetail || s.View != "code-kustomize" || s.Preference.Selected != id {
		t.Fatal("output Back lost overview")
	}
	m.updateCode("esc")
	if s.BrowserDetail || s.NodeGraph != "" || s.Preference.BrowserItem != id {
		t.Fatal("overview Back lost group selection")
	}
}

func TestKustomizeNodeFailuresAndLateCompletionStayWithTarget(t *testing.T) {
	m, s := kustomizeBrowserModel(t)
	failed := "kustomize:inspector/clusters/prod-ap-south"
	s.OpenKustomizeOverview(failed)
	m.executeCode("render")
	failedResult := m.syncCode()().(appcode.CodeResult)
	if failedResult.Err == nil {
		t.Fatal("fixture should fail rendering")
	}
	applyCodeResult(t, m, failedResult)
	if s.PreviewFailure(failed) == "" {
		t.Fatal("fixture should fail")
	}
	text := ansi.Strip(m.codeBody(110, 35))
	if !strings.Contains(text, "A patch targets Deployment api-v1") || strings.Contains(text, "[noNs]") {
		t.Fatal("unreadable failure", text)
	}
	m.updateCode("left")
	m.updateCode("down")
	if s.InspectorArea != 2 || s.DetailOffset == 0 {
		t.Fatal("failure navigation focused a missing resource list")
	}
	m.updateCode("esc")
	if !s.BrowserDetail || !strings.Contains(ansi.Strip(m.codeBody(110, 35)), "api-v1") {
		t.Fatal("failure disappeared from its overview")
	}
	m.updateCode("esc")
	good := "kustomize:inspector/clusters/prod-eu-west"
	s.OpenKustomizeOverview(good)
	if strings.Contains(ansi.Strip(m.codeBody(110, 35)), "api-v1") {
		t.Fatal("failure leaked to another node")
	}
	m.executeCode("render")
	command := m.syncCode()
	result := command().(appcode.CodeResult)
	m.updateCode("esc")
	if s.View != "code-kustomize" || !s.BrowserDetail || s.Busy {
		t.Fatal("cancel did not return to overview")
	}
	applyCodeResult(t, m, result)
	if s.View != "code-kustomize" || !s.BrowserDetail || s.Builds[good] != nil {
		t.Fatal("late completion changed navigation or accepted cancelled result")
	}
	m.executeCode("render")
	finishCode(t, m)
	if s.PreviewFailure(good) != "" || s.Builds[good] == nil || s.PreviewFailure(failed) == "" {
		t.Fatal("retry changed another target's result")
	}
}

func TestKustomizeSavedOutputDoesNotRenderOnOpen(t *testing.T) {
	m, s := kustomizeBrowserModel(t)
	id := s.Preference.BrowserItem
	s.OpenKustomizeOverview(id)
	m.executeCode("render")
	finishCode(t, m)
	build := s.Builds[id]
	m.updateCode("esc")
	entry := s.Inventory.Entry(id)
	data := append([]byte("# changed after render\n"), []byte(sourceTextFixture(t, s.Inventory.Source, entry.File))...)
	s.Inventory = source.Discover(changedSourceFixture(t, s.Inventory.Source, entry.File, data))
	m.codeBrowserControl("saved-output")
	if s.View != "manifests" || s.Preference.Section != 1 || s.Pending != "" || s.Busy {
		t.Fatal("opening saved output scheduled rendering")
	}
	if cmd := m.syncCode(); cmd != nil || s.Builds[id] != build || s.Status(id) != "stale" {
		t.Fatalf("saved output implicitly refreshed: command=%t same build=%t status=%s return=%s pending=%s busy=%t", cmd != nil, s.Builds[id] == build, s.Status(id), s.ReturnView, s.Pending, s.Busy)
	}
	if text := ansi.Strip(m.codeBody(120, 30)); !strings.Contains(text, "Previous output · stale") {
		t.Fatal("stale output not labeled", text)
	}
	m.updateCode("esc")
	if !s.BrowserDetail || s.Preference.Selected != id {
		t.Fatal("saved output lost its overview")
	}
}

func TestKustomizeOverviewSearchKeepsNodeAndBack(t *testing.T) {
	m, s := kustomizeBrowserModel(t)
	id := s.Preference.BrowserItem
	s.OpenKustomizeOverview(id)
	typeText(m, "/production-us")
	if s.Input != nil || m.searches[1] == nil || s.Preference.BrowserItem != id {
		t.Fatal("overview search changed the graph selection")
	}
	if body := ansi.Strip(m.codeBody(90, 25)); !strings.Contains(body, "Find · 1/1") {
		t.Fatal("overview did not find its text", body)
	}
	press(t, m, "enter")
	press(t, m, "esc")
	if m.searches[1] != nil || !s.BrowserDetail || s.Preference.BrowserItem != id {
		t.Fatal("closing find left the overview")
	}
	press(t, m, "esc")
	if s.BrowserDetail || s.Preference.BrowserItem != id {
		t.Fatal("Back did not restore the graph")
	}
}

func TestKustomizeFindingOpensOverviewAndRestoresFindings(t *testing.T) {
	m, s := kustomizeBrowserModel(t)
	id := s.Preference.BrowserItem
	m.executeCode("render")
	finishCode(t, m)
	s.OpenFindings(false)
	findings := s.Findings()
	s.Cursor = slices.IndexFunc(findings, func(f source.Finding) bool { return f.Target == id })
	if s.Cursor < 0 {
		t.Fatal("fixture has no target finding")
	}
	cursor := s.Cursor
	m.codeFindingDestination("sources")
	if s.View != "code-kustomize" || !s.BrowserDetail || s.Preference.Selected != id || s.Pending != "" {
		t.Fatal("finding bypassed node overview")
	}
	m.updateCode("esc")
	if s.View != "manifests" || s.Preference.Section != 3 || s.Cursor != cursor || s.ValidationSource != id {
		t.Fatal("overview lost the originating findings")
	}
}

func TestKustomizeSingleActionKeysRespectFocusAndTyping(t *testing.T) {
	m, s := kustomizeBrowserModel(t)
	id := s.Preference.BrowserItem
	s.OpenKustomizeOverview(id)
	m.focus = 0
	typeText(m, "rG")
	if s.Pending != "" || s.NodeGraph != "" || m.shortcutPrefix != "" {
		t.Fatal("local actions escaped main-panel focus")
	}
	m.focus = 1
	typeText(m, "/rG")
	if s.Pending != "" || s.NodeGraph != "" || m.searches[1] == nil {
		t.Fatal("search text activated a node action")
	}
	press(t, m, "esc")
	typeText(m, "G")
	if s.NodeGraph != id || s.Pending != "" || m.shortcutPrefix != "" {
		t.Fatal("single G did not open graph without rendering")
	}
	press(t, m, "esc")
	typeText(m, "r")
	if s.RenderRoot != id || !s.Busy || s.Running != "render" || s.RunningEntry != id || m.shortcutPrefix != "" {
		t.Fatal("single r did not capture the exact render target")
	}
	press(t, m, "esc")
	typeText(m, "d")
	if s.Document == nil || s.DocumentPath != "inspector/clusters/prod-us-east/kustomization.yaml" {
		t.Fatal("single d did not open source")
	}
	typeText(m, "irG")
	if !strings.Contains(s.Document.Text(), "rG") || s.Pending != "" || s.NodeGraph != "" {
		t.Fatal("document text activated source actions")
	}
}
