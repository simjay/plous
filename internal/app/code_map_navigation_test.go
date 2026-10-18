package app

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	appcode "github.com/simjay/plous/internal/app/code"
	"github.com/simjay/plous/internal/source"
)

func openConfigurationMap(t *testing.T, m *model) *appcode.CodeState {
	t.Helper()
	if !m.openCodeView(treeRow{workspace: m.active, connection: m.connection, tool: "manifests", resource: "render"}) {
		t.Fatal("Render did not open")
	}
	s := m.coder()
	if s.Preference.Section != 0 || s.Menu != "" || s.InspectorArea != 0 {
		t.Fatal("Render did not open the visible configuration map")
	}
	for i := 0; i < 3; i++ {
		cmd := m.syncCode()
		if cmd == nil {
			return s
		}
		result := cmd().(appcode.CodeResult)
		if result.Kind != "load" || result.Err != nil {
			t.Fatal("opening the map rendered configurations", result.Kind, result.Err)
		}
		applyCodeResult(t, m, result)
	}
	t.Fatal("map discovery did not finish")
	return nil
}

func TestConfigurationMapSelectsBeforeExplicitRender(t *testing.T) {
	m, _ := codeModel(t)
	s := openConfigurationMap(t, m)
	if body := ansi.Strip(m.codeBody(110, 30)); !strings.Contains(body, "Configuration Map") || !strings.Contains(body, "Render Selected") {
		t.Fatal("map lacks its visible render action", body)
	}
	m.updateCode("end")
	if s.Preference.Section != 0 || len(s.Builds) != 0 || m.syncCode() != nil {
		t.Fatal("keyboard node selection rendered or left the map")
	}
	index := s.SourceIndex("plain:plain/service.yaml")
	if index < 0 {
		t.Fatal("fixture entry missing")
	}
	m.mouseItem("code-row", strconv.Itoa(index), false)
	m.mouseItem("code-row", strconv.Itoa(index), true)
	selected := s.Preference.Selected
	if selected != "plain:plain/service.yaml" || s.Preference.Section != 0 || s.Pending != "" || m.syncCode() != nil {
		t.Fatal("node clicking rendered or left the map")
	}
	m.mouseControl("enter")
	if s.Preference.Section != 1 || s.Pending != "render" || len(s.Details) != 1 || s.Details[0].Selected != selected {
		t.Fatal("explicit render action did not retain its map origin")
	}
	finishCode(t, m)
	if s.Builds[selected] == nil || s.Actions()[0].Actions[0].Label != "Configuration Map" {
		t.Fatal("rendered output lacks its map return action")
	}
	m.updateCode("esc")
	if s.Preference.Section != 0 || s.Preference.Selected != selected || s.Entries()[s.Cursor].ID != selected || m.syncCode() != nil {
		t.Fatal("returning from output lost its selected node or rendered again")
	}
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	restored, err := newModel(m.store, m.history, "test")
	if err != nil {
		t.Fatal(err)
	}
	restored.selectScope(m.active, "manifests", m.connection)
	if got := restored.coder(); got.Preference.Section != 0 || got.Preference.Selected != selected {
		t.Fatal("restart did not restore the map and selected node")
	}
}

func TestConfigurationMapRetainsNestedDetailReturns(t *testing.T) {
	m, _ := codeModel(t)
	s := openConfigurationMap(t, m)
	s.Preference.Selected = "plain:plain/service.yaml"
	s.RestoreCursor()
	m.updateCode("enter")
	finishCode(t, m)
	resource, ok := s.CurrentResource()
	if !ok {
		t.Fatal("rendered fixture resource is missing")
	}
	m.updateCode("enter")
	if s.Preference.Section != 2 {
		t.Fatal("output did not open field history")
	}
	s.OpenFindings(false)
	for _, section := range []int{2, 1, 0} {
		m.updateCode("esc")
		if s.Preference.Section != section || s.Preference.Selected != "plain:plain/service.yaml" {
			t.Fatal("detail return skipped its origin", section, s.Preference)
		}
		if section == 1 && s.Preference.Resource != resource.Key() {
			t.Fatal("field history lost its originating resource")
		}
	}
}

func TestConfigurationMapKeyboardFollowsHierarchy(t *testing.T) {
	m, _ := codeModel(t)
	s := openConfigurationMap(t, m)
	order := s.GraphOrder()
	if len(order) < 3 {
		t.Fatal("fixture needs multiple configuration nodes")
	}
	m.updateCode("home")
	for _, id := range order {
		if s.Preference.Selected != id || s.Preference.Section != 0 || s.Pending != "" {
			t.Fatal("keyboard selection left the displayed hierarchy order", id, s.Preference)
		}
		m.updateCode("down")
	}
	for i := len(order) - 1; i >= 0; i-- {
		if s.Preference.Selected != order[i] {
			t.Fatal("reverse navigation skipped a configuration node", order[i], s.Preference)
		}
		m.updateCode("up")
	}
	m.updateCode("end")
	if s.Preference.Selected != order[len(order)-1] || m.syncCode() != nil {
		t.Fatal("hierarchy navigation rendered or lost the last node")
	}
}

func TestConfigurationMapCompactReturnRestoresNodeFilter(t *testing.T) {
	m, _ := codeModel(t)
	s := openConfigurationMap(t, m)
	s.Filter = "plain"
	s.Preference.Selected = "plain:plain/service.yaml"
	s.RestoreCursor()
	m.updateCode("enter")
	finishCode(t, m)
	if s.Filter != "" {
		t.Fatal("map filter leaked into output")
	}
	if body := ansi.Strip(m.codeBody(44, 18)); !strings.Contains(body, "[Esc] Configuration Map") {
		t.Fatal("compact output lost its visible map return action", body)
	}
	m.width, m.height = 80, 24
	clickMouse(t, m, "main", "action/Esc")
	if s.Preference.Section != 0 || s.Filter != "plain" || s.Preference.Selected != "plain:plain/service.yaml" || m.syncCode() != nil {
		t.Fatal("map return control lost the node/filter or started another render")
	}
	s.Prompt("Filter", "no-matching-configuration")
	m.submitCodeInput()
	m.executeCode("render")
	if s.Preference.Section != 0 || s.Pending != "" || m.syncCode() != nil {
		t.Fatal("empty map filter rendered an invisible selection")
	}
}

func TestConfigurationMapRelationshipLinkRevealsTargetBeforeRender(t *testing.T) {
	for _, filter := range []string{"base", "kustomize"} {
		t.Run(filter, func(t *testing.T) {
			m, _ := codeModel(t)
			s := openConfigurationMap(t, m)
			s.Filter = filter
			s.Preference.Selected = "kustomize:base"
			s.RestoreCursor()
			m.width, m.height, m.leftHidden, m.rightHidden = 140, 50, true, true
			target := "kustomize:prod"
			index := slices.IndexFunc(s.NavigationEntries(), func(entry source.Entry) bool { return entry.ID == target })
			if index < 0 {
				t.Fatal("fixture target is missing")
			}
			clickMouse(t, m, "main", "code-source/"+strconv.Itoa(index))
			if s.Preference.Selected != target || s.Entries()[s.Cursor].ID != target || s.Preference.Section != 0 || s.Pending != "" {
				t.Fatal("relationship link did not select its visible target", s.Preference, s.Filter)
			}
			if filter == "kustomize" && s.Filter != filter {
				t.Fatal("relationship link cleared a filter that includes its target")
			}
			m.updateCode("enter")
			finishCode(t, m)
			if s.Builds[target] == nil || s.Builds["kustomize:base"] != nil || s.Preference.Selected != target {
				t.Fatal("Enter rendered a different node than the clicked relationship")
			}
			m.updateCode("esc")
			if s.Preference.Section != 0 || s.Preference.Selected != target || s.Entries()[s.Cursor].ID != target {
				t.Fatal("output return hid or changed the relationship target")
			}
		})
	}
}

func TestConfigurationMapPreservesFileDraftAndLateOutput(t *testing.T) {
	m, s := codeModel(t)
	selectCodeFile(t, s, "README.md")
	m.openCodeFile(true)
	document := s.Document
	document.Paste("unsaved map navigation\n")
	draft := document.Text()
	s = openConfigurationMap(t, m)
	if s.Document != nil {
		t.Fatal("Files editor obscured the map")
	}
	s.Preference.Selected = "plain:plain/service.yaml"
	s.RestoreCursor()
	m.updateCode("enter")
	cmd := m.syncCode()
	if cmd == nil {
		t.Fatal("explicit render did not start")
	}
	if !m.openCodeView(treeRow{workspace: m.active, connection: m.connection, tool: "manifests", resource: "render"}) {
		t.Fatal("map navigation failed during rendering")
	}
	applyCodeResult(t, m, cmd().(appcode.CodeResult))
	if s.Preference.Section != 0 || s.Builds[s.Preference.Selected] == nil {
		t.Fatal("late output replaced the map or was lost")
	}
	if !m.openCodeView(treeRow{workspace: m.active, connection: m.connection, tool: "files", resource: "files"}) {
		t.Fatal("Files did not reopen")
	}
	if s.Document != document || document.Text() != draft || !document.Dirty() {
		t.Fatal("map or output navigation discarded the source draft")
	}
}
