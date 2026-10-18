package app

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	appcode "github.com/simjay/plous/internal/app/code"
	"github.com/simjay/plous/internal/app/monitor"
	appsettings "github.com/simjay/plous/internal/app/settings"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	diffview "github.com/simjay/plous/internal/ui/diff"
	docedit "github.com/simjay/plous/internal/ui/editor"
	helpview "github.com/simjay/plous/internal/ui/help"
	textview "github.com/simjay/plous/internal/ui/text"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func helpEntries(s helpview.Section) map[string]helpview.Entry {
	entries := map[string]helpview.Entry{}
	for _, g := range s.Groups {
		for _, e := range g.Entries {
			entries[e.ID] = e
		}
	}
	return entries
}

func TestContextualHelpResourceActionsMatchRegistry(t *testing.T) {
	for _, resource := range []kubecommon.Resource{
		{Name: "pods", Kind: "Pod"}, {Group: "apps", Name: "deployments", Kind: "Deployment"},
		{Name: "namespaces", Kind: "Namespace"}, {Name: "secrets", Kind: "Secret"}, {Name: "configmaps", Kind: "ConfigMap"},
		{Name: "events", Kind: "Event"}, {Group: "apps", Name: "controllerrevisions", Kind: "ControllerRevision"},
		{Group: "example.test", Name: "widgets", Kind: "Widget"},
	} {
		t.Run(resource.Key(), func(t *testing.T) {
			e := &monitor.ExploreState{Resource: resource, Detail: &unstructured.Unstructured{Object: map[string]any{"kind": resource.Kind}}}
			expected := map[string]monitor.ResourceAction{}
			for _, action := range monitor.ActionsFor(resource, e.Detail) {
				expected[action.Command] = action
			}
			entries := helpEntries(e.Help())
			for _, action := range monitor.ResourceActionDefinitions {
				got, present := entries[action.Command]
				want, applicable := expected[action.Command]
				if present != applicable || present && (got.Key != want.Alias || got.Label != want.Label) {
					t.Fatalf("%s help does not match resource action: %+v, applicable %t", action.Command, got, applicable)
				}
			}
			e.Tab = 1
			_, editable := helpEntries(e.Help())["edit"]
			_, wantEditable := expected["edit"]
			if editable != wantEditable {
				t.Fatal("YAML bypassed resource edit applicability")
			}
			e.Detail = nil
			entries = helpEntries(e.Help())
			if _, present := entries["scale"]; present {
				t.Fatal("table advertises detail-only Scale")
			}
			if entries["sort"].Key != "S" {
				t.Fatal("table lost Sort")
			}
		})
	}
}

func TestContextualHelpSnapshotAndPurity(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(0)
	m.focus = 1
	c := m.selectedConnection()
	c.Target, c.TargetCaptured = nil, false
	expanded := maps.Clone(m.expanded)
	originalFocus, originalTool, originalConnection := m.focus, m.tool, m.connection
	e := m.explore
	action := &monitor.ResourceActionState{Stage: "menu", Items: monitor.ActionsFor(e.Resource, e.Detail), Reasons: map[string]string{"delete": "Access denied"}}
	e.Action = action
	m.openHelp()
	if c.TargetCaptured || c.Target != nil || !reflect.DeepEqual(expanded, m.expanded) || m.focus != originalFocus || m.tool != originalTool || m.connection != originalConnection {
		t.Fatalf("Help changed state: target captured=%v, target present=%v, expanded changed=%v, focus=%d/%d, tool=%s/%s, connection=%s/%s", c.TargetCaptured, c.Target != nil, !reflect.DeepEqual(expanded, m.expanded), m.focus, originalFocus, m.tool, originalTool, m.connection, originalConnection)
	}
	if got := helpEntries(m.help.sections[1])["delete"].Reason; got != "Access denied" {
		t.Fatalf("lost known denial: %q", got)
	}
	before := m.help.sections[1].Text()
	action.Reasons["delete"] = "changed during background refresh"
	e.Action = nil
	m.help.scroll[1] = 2
	m.updateHelp("right")
	m.openHelp()
	if m.help.sections[1].Text() != before || m.help.scroll[1] != 2 || m.focus != originalFocus {
		t.Fatal("Help reference or originating focus changed while reading")
	}
	m.closeHelp()
	m.openHelp()
	if m.help.sections[1].Text() == before {
		t.Fatal("reopening Help did not refresh context")
	}
	m.closeHelp()
	m.explore = nil
	m.helpSections()
	if m.explore != nil {
		t.Fatal("Help initialized a lazy Kubernetes view")
	}
}

func TestContextualHelpPanelAndOverlayPrecedence(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(0)
	title := m.mainHelp().Title
	rows := m.treeRowsRead()
	for i, row := range rows {
		if row.kind == "settings" {
			m.treeCursor = i
			break
		}
	}
	if m.mainHelp().Title != title || !strings.Contains(m.workspaceHelp().Title, "Settings") {
		t.Fatal("tree highlight redirected main Help")
	}
	m.form = &appsettings.Form{Title: "Unfinished setup", Kind: "connection"}
	if m.mainHelp().Title != "Unfinished setup" {
		t.Fatal("resource overrode setup Help")
	}
	m.form = nil
	m.mode = 0
	if !strings.HasPrefix(m.mainHelp().Title, "Overview") {
		t.Fatal("resource overrode Overview")
	}
	m.mode = 2
	if !strings.Contains(m.mainHelp().Title, "Activity") {
		t.Fatal("resource overrode Activity")
	}
}

func TestContextualHelpHelmAndGitOpsRouting(t *testing.T) {
	m, h := helmModel(t)
	h.Preference.Section = "Charts"
	if m.mainHelp().Title != "Helm · Charts" {
		t.Fatal(m.mainHelp().Title)
	}
	h.Document = docedit.New("replicas: 2\n")
	h.Comparison = diffview.New("replicas: 1\n", h.Document.Text(), "draft", 0, false)
	entries := helpEntries(m.mainHelp())
	if entries["save"].Label != "Save to draft values" {
		t.Fatal("Helm values review lost its save target")
	}
	h.Document = nil
	if _, save := helpEntries(m.mainHelp())["save"]; save {
		t.Fatal("read-only Helm comparison offers Save")
	}
	h.Comparison = nil
	h.Timeline = &monitor.HelmTimeline{Release: helmtool.Release{Revision: 2}, Revisions: []helmtool.Revision{{Revision: 2}, {Revision: 1}}}
	if _, rollback := helpEntries(m.mainHelp())["rollback"]; rollback {
		t.Fatal("latest revision offers rollback key")
	}
	if strings.Contains(m.mainHelp().Text(), "Preview rollback") {
		t.Fatal("latest revision offers a rollback menu route")
	}
	h.Timeline.Cursor = 1
	if _, rollback := helpEntries(m.mainHelp())["rollback"]; !rollback {
		t.Fatal("older revision lost rollback")
	}
	g, v := gitOpsModel(t)
	if !strings.Contains(g.mainHelp().Title, "Argo") {
		t.Fatal(g.mainHelp().Title)
	}
	v.State.ApplyDetail(v.State.Snapshot.Items[0])
	if _, linked := helpEntries(g.mainHelp())["link-Kubernetes resource"]; !linked {
		t.Fatal("Argo resource link missing")
	}
	g.tool = string(gitops.Flux)
	v = g.gitOpsCurrent()
	if !strings.Contains(g.mainHelp().Title, "Flux") || strings.Contains(g.mainHelp().Text(), "rollback") {
		t.Fatal(g.mainHelp().Text())
	}
	v.Namespaces = &monitor.NamespacePicker{}
	if !strings.Contains(g.mainHelp().Title, "Namespace") {
		t.Fatal("Flux namespace picker lost precedence")
	}
}

func TestContextualHelpCodeCoverageAndCachePurity(t *testing.T) {
	m, s := codeModel(t)
	for _, view := range []string{"files", "code-helm", "code-kustomize", "code-argo", "code-flux", "manifests"} {
		for section := range appcode.CodeSections {
			s.View, m.tool, s.Preference.Section = view, view, section
			s.FieldResource, s.FieldCursor = "untouched", 27
			s.ComparisonCache, s.ComparisonIdentity = nil, "untouched"
			help := m.mainHelp()
			if !strings.HasPrefix(help.Title, "Code") {
				t.Fatalf("%s: %s", view, help.Title)
			}
			if s.FieldResource != "untouched" || s.FieldCursor != 27 || s.ComparisonIdentity != "untouched" || s.ComparisonCache != nil {
				t.Fatal("Help populated view caches")
			}
			if strings.Contains(help.Text(), "Preview rollback") || strings.Contains(help.Text(), "Save to Kubernetes") {
				t.Fatal("Code inherited Monitor controls")
			}
		}
	}
	s.View, m.tool = "files", "files"
	s.Menu = "actions"
	h := m.mainHelp().Text()
	scratch := *s
	for _, label := range scratch.MenuItems() {
		if !strings.Contains(h, label) {
			t.Fatalf("Help dropped menu item %q", label)
		}
	}
	s.Input = &textview.Field{}
	s.InputKind = "filter"
	if strings.Contains(m.mainHelp().Text(), "Actions menu") {
		t.Fatal("typing Help includes navigation actions")
	}
}

func TestContextualHelpCommandsAndScreenFrames(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(0)
	for _, section := range m.helpSections() {
		for _, e := range helpEntries(section) {
			if e.Command == "" {
				continue
			}
			found := false
			for _, binding := range append([]string{e.Key}, e.Alternatives...) {
				if !strings.HasPrefix(binding, ":") {
					continue
				}
				command := canonicalCommand(strings.TrimPrefix(binding, ":"))
				if command == e.Command || actionCommand(command) == e.Command {
					found = true
				}
			}
			if !found {
				t.Fatalf("%s has no registered command spelling: %+v", e.ID, e)
			}
		}
	}
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 60}} {
		m.width, m.height = size[0], size[1]
		m.execute("help")
		view := ansi.Strip(m.render())
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) != size[0] {
				t.Fatalf("Help overflow at %v", size)
			}
		}
		if strings.Contains(view, "Current Workspace\nLaunch") || strings.Contains(view, "Click controls to activate") {
			t.Fatal("Help still contains introductory essays")
		}
		if dir := os.Getenv("PLOUS_REVIEW_DIR"); dir != "" {
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("help-%d.txt", size[0])), []byte(view), 0600); err != nil {
				t.Fatal(err)
			}
		}
		m.updateHelp("end")
		widths, height, _ := m.helpSizes()
		if m.help.scroll[1] != max(0, len(helpLines(m.help.sections[1], widths[1]-4))-m.helpBodyHeight(1, height)) {
			t.Fatal("last Help controls are unreachable")
		}
		m.closeHelp()
	}
}

func TestContextualHelpInteractionLayers(t *testing.T) {
	m, a := documentModel(t)
	a.Document.Paste("draft\n")
	a.Document.Key("v", "v")
	a.Document.Key("right", "")
	before := a.Document.Text()
	m.openHelp()
	if !strings.Contains(m.panelHelpText(1), "Save to Kubernetes") {
		t.Fatal("wrong save target")
	}
	m.closeHelp()
	if a.Document.Text() != before || a.Document.Mode != docedit.Select {
		t.Fatal("Help altered document selection")
	}
	a.Diagnostic = &textview.Viewer{}
	if m.mainHelp().Title != "Error details" {
		t.Fatal("editor overrode diagnostic")
	}
	a.Diagnostic.Searching = true
	if _, save := helpEntries(m.mainHelp())["save"]; save {
		t.Fatal("search inherited Save")
	}
	a.Diagnostic = nil
	a.Discard = true
	if m.mainHelp().Title != "Discard draft" {
		t.Fatal("editor overrode discard confirmation")
	}
	a.Discard, a.Busy = false, true
	if _, save := helpEntries(m.mainHelp())["save"]; save {
		t.Fatal("pending write advertises another Save")
	}
	a.Busy, a.Stage = false, "scale"
	if strings.Contains(m.mainHelp().Text(), "PgUp") {
		t.Fatal("replica input advertises list keys")
	}
	c := m.chat()
	m.chats[m.conversationScope()] = c
	c.Models = true
	if m.chatHelp().Title != "AI chat · Models" {
		t.Fatal("model picker lost precedence")
	}
	c.Models, c.History = false, true
	if m.chatHelp().Title != "AI chat · History" {
		t.Fatal("chat history lost precedence")
	}
	c.History = false
	m.chatEditing = true
	entries := helpEntries(m.chatHelp())
	if entries["send"].Key != "Enter" || len(entries["newline"].Alternatives) != 1 {
		t.Fatal("composer lost newline exception")
	}
	if _, stop := entries["stopchat"]; stop {
		t.Fatal("idle chat advertises Stop")
	}
}

func TestContextualHelpPickersDescribeControlsWithoutRepeatingData(t *testing.T) {
	e := &monitor.ExploreState{Picker: "sort"}
	text := e.Help().Text()
	if !strings.Contains(text, "reverse current sort") || strings.Contains(text, "Type to") {
		t.Fatal(text)
	}
	h := &monitor.HelmState{MenuKind: "release", Menu: []string{"private-release-name"}}
	if text := h.Help().Text(); strings.Contains(text, "private-release-name") || !strings.Contains(text, "PgUp") {
		t.Fatal(text)
	}
	h.MenuKind = "actions"
	h.Menu = []string{"Preview chart changes"}
	if !strings.Contains(h.Help().Text(), h.Menu[0]) {
		t.Fatal("action menu lost its commands")
	}
}

func (m *model) panelHelpText(panel int) string { return m.panelHelpSection(panel).Text() }

func (m *model) viewHelp() string { s := m.mainHelp(); m.resolveHelpCommands(&s); return s.Text() }
