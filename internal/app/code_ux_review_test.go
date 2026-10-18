package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/source"
)

func TestCodeReviewSavedComparisonStaysUseful(t *testing.T) {
	m, s := codeModel(t)
	m.selectScope(m.active, "manifests", m.connection)
	s.Preference.Selected, s.Pending = "plain:plain/service.yaml", "render"
	finishCode(t, m)
	codeAction(t, m, "Open Resource Source")
	s.Document.Paste("# comment only\n")
	m.saveCodeDocument(true)
	finishCode(t, m)
	for _, width := range []int{32, 70, 110} {
		body := ansi.Strip(m.codeBody(width, 26))
		if !strings.Contains(body, "No manifest changes.") || strings.Contains(body, "match this filter") {
			t.Fatal("unchanged save looks like a filter failure", body)
		}
	}
	m.executeCode("render")
	finishCode(t, m)
	if !s.ComparisonCurrent() {
		t.Fatal("rendering again invalidated a saved comparison")
	}
	codeAction(t, m, "All Lines")
	s.InspectorArea = 1
	if !strings.Contains(ansi.Strip(m.codeBody(110, 26)), "Inspect Changes") {
		t.Fatal("resource list advertises the YAML action for comparison focus")
	}
	s.InspectorArea = 2
	m.updateCode("enter")
	if s.Document == nil || s.DocumentKind != "resource" || s.Preference.Section != 1 {
		t.Fatal("comparison Enter opened unrelated field history")
	}
	m.codeDocumentCommand("q")
	codeAction(t, m, "Show Final YAML")
	if s.ComparisonActive() || !strings.Contains(ansi.Strip(m.codeBody(110, 26)), "kind: Service") {
		t.Fatal("unchanged comparison trapped the complete YAML")
	}
}

func TestCodeReviewNestedDetailsRestoreFindingScope(t *testing.T) {
	m, s := codeModel(t)
	m.selectScope(m.active, "manifests", m.connection)
	s.Preference.Selected, s.Pending = "kustomize:prod", "render"
	finishCode(t, m)
	s.OpenFindings(true)
	s.SelectValidationView("Images")
	s.ValidationCategory, s.ValidationSource = "Images", "kustomize:prod"
	s.Filter, s.DetailOffset = "image", 2
	if len(s.Findings()) == 0 {
		t.Fatal("missing image fixture")
	}
	m.codeFindingDestination("sources")
	s.OpenFindings(false)
	s.CloseDetail()
	s.CloseDetail()
	if s.Preference.Section != 3 || s.ValidationView != "Images" || s.ValidationCategory != "Images" || s.ValidationSource != "kustomize:prod" || s.Filter != "image" || s.DetailOffset != 2 {
		t.Fatal("Back changed the findings scope or position", s.ValidationView, s.ValidationSource, s.Filter, s.DetailOffset)
	}
}

func TestCodeReviewFieldNavigationUsesVisibleAreas(t *testing.T) {
	m, s := codeModel(t)
	m.selectScope(m.active, "manifests", m.connection)
	s.Preference.Selected, s.Pending = "kustomize:prod", "render"
	finishCode(t, m)
	s.InspectorArea = 2
	s.CurrentFields()
	field, resource := s.FieldCursor, s.Cursor
	m.updateCode("j")
	if s.FieldCursor != field+1 || s.Cursor != resource {
		t.Fatal("j changed resources instead of the focused YAML field")
	}
	m.updateCode("k")
	if s.FieldCursor != field {
		t.Fatal("k did not restore the field")
	}
	m.openCodeTrace()
	s.CaptureTraceReference()
	reference := s.TraceReference
	if reference == nil {
		t.Fatal("missing trace fixture")
	}
	s.InspectorArea = 0
	s.OpenFindings(false)
	s.CloseDetail()
	if s.TraceReference != reference || s.InspectorArea != 0 {
		t.Fatal("Back lost cross-overlay field context")
	}
	m.updateCode("enter")
	if s.InspectorArea != 2 {
		t.Fatal("Enter focused an invisible resource area")
	}
	s.TraceUnavailable = "No matching resource in this entry."
	if body := ansi.Strip(m.codeBody(70, 26)); strings.Count(body, "[a]") != 1 {
		t.Fatal("compact history repeats the action control", body)
	}
}

func TestCodeReviewPrimaryActionsAndEmptyStates(t *testing.T) {
	m, s := codeModel(t)
	m.selectScope(m.active, "manifests", m.connection)
	s.ChooseSection(1)
	s.Preference.Selected = ""
	m.updateCode("enter")
	if s.Preference.Section != 0 || s.Menu != "" {
		t.Fatal("empty output has no working configuration map action")
	}
	s.Menu = ""
	s.Preference.Selected, s.Pending = "plain:plain/service.yaml", "render"
	finishCode(t, m)
	s.Menu = "actions"
	if len(s.MenuItems()) > 9 || !slices.Contains(s.MenuItems(), "Graph") || !slices.Contains(s.MenuItems(), "More Tools") || slices.Contains(s.MenuItems(), "Validate All Entry Points") {
		t.Fatal("primary actions are still overloaded", s.MenuItems())
	}
	codeAction(t, m, "More Tools")
	m.updateCode("esc")
	if s.Menu != "actions" || s.MenuItems()[s.Choice] != "More Tools" {
		t.Fatal("submenu Escape did not restore its parent")
	}
	s.Menu = ""
	s.Inventory.Findings = slices.DeleteFunc(s.Inventory.Findings, func(f source.Finding) bool { return f.Target == s.Preference.Selected })
	s.OpenFindings(false)
	// A clean entry must not inherit findings from other entries.
	body := ansi.Strip(m.codeBody(110, 30))
	if !strings.Contains(body, "No findings for this entry") || strings.Contains(body, "No findings match this filter") {
		t.Fatal("clean selected entry is described as a filter failure", body)
	}
}

func TestCodeReviewWorkflowFrames(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 32}, {180, 42}} {
		m, s := codeModel(t)
		m.width, m.height = size[0], size[1]
		m.expanded[m.active], m.expanded[m.connection] = true, true
		m.selectScope(m.active, "manifests", m.connection)
		s.Preference.Selected, s.Pending = "plain:plain/service.yaml", "render"
		finishCode(t, m)
		capture := func(scene string) {
			t.Helper()
			frame := m.View().Content
			rows := strings.Split(frame, "\n")
			if len(rows) != m.height {
				t.Fatal("review frame exceeds terminal height", scene)
			}
			for _, row := range rows {
				if ansi.StringWidth(row) != m.width {
					t.Fatal("review frame exceeds terminal width", scene)
				}
			}
			if dir := os.Getenv("PLOUS_REVIEW_DIR"); dir != "" {
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("review-%s-%d.txt", scene, m.width)), []byte(ansi.Strip(frame)), 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
		capture("output")
		codeAction(t, m, "Open Resource Source")
		s.Document.Paste("# comment only\n")
		m.saveCodeDocument(true)
		finishCode(t, m)
		capture("unchanged")
		m.executeCode("actions")
		capture("actions")
		codeAction(t, m, "Show Final YAML")
		m.openCodeTrace()
		capture("history")
		s.OpenFindings(false)
		capture("findings")
	}
}

func TestCodeReviewCapabilityHintsMatchEntry(t *testing.T) {
	m, s := codeModel(t)
	m.selectScope(m.active, "manifests", m.connection)
	s.Preference.Selected = "terraform:infra"
	for _, section := range []int{1, 0} {
		s.ChooseSection(section)
		body := ansi.Strip(m.codeBody(110, 26))
		if !strings.Contains(body, "Open Files") || strings.Contains(body, "Render Again") || strings.Contains(body, "Preparing preview") || strings.Contains(body, "View Resources") {
			t.Fatal("unsupported entry advertises rendering", body)
		}
	}
	s.ChooseSection(1)
	m.updateCode("enter")
	if s.View != "files" || s.FileRoot != "infra" {
		t.Fatal("Open Files did not open the selected configuration")
	}
}

func TestCodeReviewSingleEntryComparisonHasHonestEmptyState(t *testing.T) {
	m, s := codeModel(t)
	m.selectScope(m.active, "manifests", m.connection)
	m.coder()
	s.Preference.Selected = "plain:plain/service.yaml"
	entry := *s.Inventory.Entry(s.Preference.Selected)
	s.Inventory.Entries = []source.Entry{entry, {ID: "kustomize:component", Kind: "kustomize", Component: true}}
	s.Menu = "more-tools"
	if slices.Contains(s.MenuItems(), "Compare Entry Points") {
		t.Fatal("comparison is offered without another renderable entry")
	}
	m.executeCode("review")
	body := ansi.Strip(m.codeBody(70, 26))
	if !strings.Contains(body, "No other renderable entries") || strings.Contains(body, "/ changes the entry filter") {
		t.Fatal("empty comparison gives unusable filtering instructions", body)
	}
	m.updateCode("esc")
	s.OpenConfigurationMap()
	s.Filter = "does-not-exist"
	if body := ansi.Strip(m.codeBody(70, 26)); !strings.Contains(body, "No entries match") {
		t.Fatal("configuration map lost its actionable filter guidance", body)
	}
}

func TestCodeReviewCompactFailureKeepsRetryVisible(t *testing.T) {
	m, s := codeModel(t)
	m.selectScope(m.active, "manifests", m.connection)
	s.Preference.Selected, s.Pending = "plain:plain/service.yaml", "render"
	finishCode(t, m)
	s.Failures[s.Preference.Selected] = "Saved source could not be rendered."
	for _, width := range []int{32, 70, 110} {
		body := ansi.Strip(m.codeBody(width, 26))
		if !strings.Contains(body, "Retry") || !strings.Contains(body, "Previous output") {
			t.Fatal("failed render hides retry while showing old output", body)
		}
	}
	m.executeCode("render")
	if s.Pending != "render" {
		t.Fatal("visible Retry does not render again")
	}
}

func TestCodeReviewCompactHistoryIdentifiesResource(t *testing.T) {
	m, s := codeModel(t)
	m.selectScope(m.active, "manifests", m.connection)
	s.Preference.Selected, s.Pending = "plain:plain/service.yaml", "render"
	finishCode(t, m)
	m.openCodeTrace()
	for _, width := range []int{28, 70} {
		body := ansi.Strip(m.codeBody(width, 26))
		if !strings.Contains(body, "Service/api") || !strings.Contains(body, "/apiVersion") || !strings.Contains(body, "Final value: v1") {
			t.Fatal("compact field history lost its resource or field context", body)
		}
	}
}

func TestCodeReviewDetailScrollingKeepsSelection(t *testing.T) {
	m, s := codeModel(t)
	m.selectScope(m.active, "manifests", m.connection)
	s.Preference.Selected = "kustomize:prod"
	s.MarkPreviewAttempt(s.Preference.Selected)
	s.Failures[s.Preference.Selected] = "First diagnostic. " + strings.Repeat("diagnostic ", 180) + "Last diagnostic."
	s.OpenFindings(true)
	s.InspectorArea = 1
	cursor, selected := s.Cursor, s.Preference.Selected
	m.updateCode("end")
	body := ansi.Strip(m.codeBody(90, 28))
	if s.Cursor != cursor || s.Preference.Selected != selected || !strings.Contains(body, "Last diagnostic") {
		t.Fatal("End changed the finding instead of scrolling its details", body)
	}
	offset := s.DetailOffset
	m.updateCode("up")
	m.codeBody(90, 28)
	if s.DetailOffset != offset-1 {
		t.Fatal("scrolling back from End is stuck beyond the last line")
	}
	m.updateCode("home")
	if s.DetailOffset != 0 || s.Cursor != cursor || s.Preference.Selected != selected {
		t.Fatal("Home changed the finding instead of returning to its first line")
	}
}
