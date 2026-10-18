package app

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	textview "github.com/simjay/plous/internal/ui/text"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCodeUXFrames(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 32}, {180, 42}, {240, 50}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			m, _ := codeModel(t)
			m.width, m.height = size[0], size[1]
			m.expanded[m.active], m.expanded[m.connection] = true, true
			capture := func(scene string) {
				t.Helper()
				view := m.View().Content
				plain := ansi.Strip(view)
				if scene == "validate" && (!strings.Contains(plain, "omitted") || !strings.Contains(plain, "Validate All")) {
					t.Fatal("compact Validate hides coverage or its primary action", plain)
				}
				if scene == "files" && !strings.Contains(plain, "Actions") {
					t.Fatal("Files hides its action menu")
				}
				if lines := strings.Split(view, "\n"); len(lines) != m.height {
					t.Fatalf("%s: %d rows, want %d", scene, len(lines), m.height)
				} else {
					for _, line := range lines {
						if ansi.StringWidth(line) != m.width {
							t.Fatalf("%s: row exceeds layout: %d != %d", scene, ansi.StringWidth(line), m.width)
						}
					}
				}
				if dir := os.Getenv("PLOUS_REVIEW_DIR"); dir != "" {
					if err := os.MkdirAll(dir, 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("code-%s-%d.txt", scene, m.width)), []byte(ansi.Strip(view)), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			capture("files")
			m.updateCode("right")
			capture("file-tree")
			m.executeCode("actions")
			capture("file-actions")
			m.updateCode("esc")
			m.selectScope(m.active, "tfcode", m.connection)
			capture("terraform")
			m.selectScope(m.active, "manifests", m.connection)
			s := m.coder()
			s.Preference.Selected = "plain:plain/service.yaml"
			s.RestoreCursor()
			capture("sources")
			s.Pending = "render"
			finishCode(t, m)
			capture("render")
			s.ChooseSection(2)
			m.openCodeTrace()
			capture("trace")
			m.codeDocumentCommand("q")
			s.ChooseSection(3)
			capture("validate")
			s.Preference.File = "README.md"
			m.openCodeFile(true)
			s.Document.Paste("reviewed edit\n")
			capture("editor")
			m.codeDocumentCommand("v")
			capture("review")
		})
	}
}

func TestCodeUXEscapeClearsSearchBeforeClosingDocument(t *testing.T) {
	m, s := codeModel(t)
	s.Filter = "README.md"
	m.openCodeFile(false)
	d := s.Document
	typeText(m, "/fixture")
	press(t, m, "enter")
	press(t, m, "esc")
	if s.Document != d || d.Query != "" {
		t.Fatal("Escape closed the document instead of clearing its search")
	}
	press(t, m, "esc")
	if s.Document != nil {
		t.Fatal("Escape did not return to Files")
	}
}

func TestCodeUXMenusFollowCurrentCapability(t *testing.T) {
	m, s := codeModel(t)
	m.executeCode("actions")
	for _, item := range s.MenuItems() {
		if strings.Contains(item, "Helm") || strings.Contains(item, "Git Revision") {
			t.Fatalf("local Files offers unrelated action %q", item)
		}
	}
	m.updateCode("esc")
	m.selectScope(m.active, "manifests", m.connection)
	s = m.coder()
	s.Preference.Selected = "kustomize:prod"
	m.executeCode("actions")
	if strings.Contains(strings.Join(s.MenuItems(), "\n"), "Helm release") {
		t.Fatal("Kustomize offers standalone Helm release inputs")
	}
	s.Preference.Selected = "helm:chart"
	if !slices.Contains(s.MenuItems(), "Helm Inputs") {
		t.Fatal("standalone Helm Inputs are unavailable")
	}
}

func TestCodeUXFilterAppliesToVisibleListAndEscBeforeFullscreen(t *testing.T) {
	m, _ := codeModel(t)
	m.selectScope(m.active, "manifests", m.connection)
	s := m.coder()
	s.Preference.Selected = "plain:plain/service.yaml"
	s.Pending = "render"
	finishCode(t, m)
	m.updateCode("/")
	s.Input.Text = "no-resource-matches"
	m.submitCodeInput()
	if m.codeCount() != 0 {
		t.Fatal("Render filter does not filter rendered resources")
	}
	m.execute("f")
	press(t, m, "esc")
	if s.Filter != "" || !m.viewExpanded() {
		t.Fatal("Escape must clear the filter before restoring layout")
	}
	m.updateCode("/")
	s.Input.Text = "api"
	m.submitCodeInput()
	s.ChooseSection(0)
	if s.Filter != "" {
		t.Fatal("resource filter leaked into Sources")
	}
}

func TestCodeUXRenderedResourceNeverOffersUnrelatedFileEdit(t *testing.T) {
	m, _ := codeModel(t)
	m.selectScope(m.active, "manifests", m.connection)
	s := m.coder()
	s.Preference.File = "README.md"
	s.Preference.Selected = "plain:plain/service.yaml"
	s.Pending = "render"
	finishCode(t, m)
	m.openCodeResource()
	m.codeDocumentCommand("a")
	if slices.Contains(s.MenuItems(), "Edit Selected File") {
		t.Fatal("rendered document offered an edit using an unrelated previous file selection")
	}
	m.updateCode("esc")
	m.codeDocumentCommand("d")
	if s.Document == nil || s.Document.Name != "plain/service.yaml" {
		t.Fatal("Open Source did not use the rendered resource's verified origin")
	}
}

func TestCodeUXHelpDescribesSettingsAboveRepository(t *testing.T) {
	m, _ := codeModel(t)
	m.execute("settings")
	if !strings.HasPrefix(m.viewHelp(), "Settings") {
		t.Fatal("Help describes the covered Code view instead of Settings")
	}
}

func TestCodeUXHelpUsesTheActualSaveTarget(t *testing.T) {
	m, s := codeModel(t)
	s.Filter = "README.md"
	m.openCodeFile(true)
	if help := m.viewHelp(); !strings.Contains(help, "Save to local file") || strings.Contains(help, "Save to Kubernetes") {
		t.Fatal("file editor help describes the wrong save target", help)
	}
	m.codeDocumentCommand("v")
	if help := m.viewHelp(); !strings.Contains(help, "Optional file review") || strings.Contains(help, "Save to Kubernetes") {
		t.Fatal("file review help describes the wrong save target", help)
	}
	s.Review, s.File = nil, nil
	s.Document.ReadOnly = true
	if help := m.viewHelp(); strings.Contains(help, ":w ") || !strings.Contains(help, "read only") {
		t.Fatal("read-only preview advertises saving", help)
	}
}

func TestCodeUXEditorActionsSaveOnlyOnIntent(t *testing.T) {
	m, s := codeModel(t)
	m.width, m.height = 200, 45
	s.Filter = "README.md"
	m.openCodeFile(true)
	d := s.Document
	before := d.Text()
	typeText(m, "yyp")
	if d.Text() == before || !d.Dirty() {
		t.Fatal("local yank/paste did not edit the document")
	}
	press(t, m, "esc")
	if !d.Controls {
		t.Fatal("Escape did not focus editor actions")
	}
	press(t, m, "right")
	saved, _ := os.ReadFile(filepath.Join(s.Config.Path, s.File.Path))
	if string(saved) != before {
		t.Fatal("selecting Save wrote the file")
	}
	press(t, m, "enter")
	if d.Dirty() || d.SavedStatus != "Saved to local file" {
		t.Fatal("Save action failed", d.Diagnostic)
	}
	clickMouse(t, m, "main", "codedocumentaction/0")
	typeText(m, "mouse edit ")
	press(t, m, "esc")
	clickMouse(t, m, "main", "codedocumentaction/1")
	if d.Dirty() {
		t.Fatal("explicit mouse Save did not save", d.Diagnostic)
	}
}

func TestCodeUXSourceAndErrorDetailsReturnToExactDocument(t *testing.T) {
	m, s := codeModel(t)
	m.selectScope(m.active, "manifests", m.connection)
	s.Preference.Selected, s.Pending = "plain:plain/service.yaml", "render"
	finishCode(t, m)
	m.openCodeResource()
	rendered := s.Document
	m.codeDocumentCommand("d")
	if s.DocumentPath != "plain/service.yaml" {
		t.Fatal("source identity was lost")
	}
	m.openCodeFile(true)
	draft := s.Document
	draft.Paste("broken: [\n")
	m.codeDocumentCommand("w")
	if draft.Diagnostic == "" || !draft.Dirty() {
		t.Fatal("invalid draft was saved")
	}
	typeText(m, "!")
	if s.Document == draft || !strings.Contains(s.Document.Text(), "yaml:") {
		t.Fatal("full validation error is unavailable")
	}
	press(t, m, "esc")
	if s.Document != draft || !draft.Dirty() {
		t.Fatal("error details lost the draft")
	}
	m.codeDocumentCommand("q!")
	if s.Document != rendered {
		t.Fatal("source preview did not return to the original rendered document")
	}
	m.codeDocumentCommand("d")
	m.openCodeFile(true)
	s.Document.Paste("# saved change\n")
	m.codeDocumentCommand("wq")
	if s.Document != rendered || s.DocumentPath != "" || !strings.Contains(rendered.Target, "stale") {
		t.Fatal("old rendered output offered a source link after that source changed")
	}
}

func TestCodeUXOmittedSourceDoesNotAppearFullyChecked(t *testing.T) {
	m, s := codeModel(t)
	m.selectScope(m.active, "manifests", m.connection)
	s.Inventory.Entries, s.Inventory.Findings = nil, nil
	s.Inventory.Source.Gaps = []string{"omitted-submodule"}
	s.ChooseSection(3)
	view := ansi.Strip(m.codeBody(70, 24))
	if !strings.Contains(view, "Coverage is incomplete") || strings.Contains(view, "No findings") {
		t.Fatal("omitted inputs appeared fully checked", view)
	}
}

func TestCodeUXMouseMenuAndInputOwnWheel(t *testing.T) {
	m, s := codeModel(t)
	m.width, m.height = 200, 45
	s.Filter = "README.md"
	m.openCodeFile(false)
	d := s.Document
	m.codeDocumentCommand("a")
	choice := s.Choice
	m.mouseWheel(textview.MouseSpan{Scope: "main"}, tea.MouseWheelDown)
	if s.Choice == choice || d.Cursor != 0 {
		t.Fatal("wheel did not select the visible menu")
	}
	m.updateCode("esc")
	m.codeDocumentCommand("q")
	m.selectScope(m.active, "manifests", m.connection)
	s.Preference.Selected = "helm:chart"
	m.executeCode("actions")
	s.Menu = "helm-inputs"
	index := slices.Index(s.MenuItems(), "Helm Release Name")
	s.Choice = index
	m.activateCodeMenu()
	value := s.Input.Text
	m.mouseWheel(textview.MouseSpan{Scope: "main"}, tea.MouseWheelDown)
	if s.Input.Text != value {
		t.Fatal("wheel changed text input")
	}
	press(t, m, "esc")
	if s.Menu != "helm-inputs" || s.Choice != index {
		t.Fatal("cancelled input did not return to its menu selection")
	}
	m.updateCode("esc")
	m.expanded[m.connection] = true
	clickMouse(t, m, "tree", "tree/"+m.connection+"/helm")
	if s.View != "code-helm" {
		t.Fatal("sidebar view click failed")
	}
}

func TestCodeUXTerraformOpensFilteredFiles(t *testing.T) {
	m, _ := codeModel(t)
	m.selectScope(m.active, "tfcode", m.connection)
	s := m.coder()
	directory := s.TerraformDirectories()[0]
	m.updateCode("enter")
	if m.tool != "files" || s.FileRoot != directory || s.Filter != "" {
		t.Fatal("Terraform entry did not open its files", m.tool, s.Filter)
	}
	m.updateCode("right")
	m.updateCode("enter")
	if s.Document == nil || !strings.HasPrefix(s.Document.Name, directory+"/") {
		t.Fatal("Terraform opened a file from another directory")
	}
}
