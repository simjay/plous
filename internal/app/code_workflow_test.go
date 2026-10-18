package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	appcode "github.com/simjay/plous/internal/app/code"
	"github.com/simjay/plous/internal/source"
)

func TestCodeRenderMapFilterAndContextualReturns(t *testing.T) {
	m, s := codeModel(t)
	openSourceView(t, m, "render")
	s.Rescan = false
	m.updateCode("/")
	s.Input.Text = "plain/service.yaml"
	m.submitCodeInput()
	if s.Preference.Section != 0 || len(s.Entries()) != 1 || m.syncCode() != nil {
		t.Fatal("node filter did not narrow the map without rendering")
	}
	m.updateCode("enter")
	finishCode(t, m)
	if s.Preference.Selected != "plain:plain/service.yaml" || s.Preference.Section != 1 {
		t.Fatal("explicit rendering did not open output")
	}
	resource, _ := s.CurrentResource()
	s.Filter = "api"
	s.Preference.Field = "/metadata/name"
	s.FieldResource = ""
	s.CurrentFields()
	codeAction(t, m, "Field History")
	if s.Preference.Section != 2 || s.ViewID() != "files" {
		t.Fatal("field history became another sidebar view")
	}
	m.updateCode("esc") // The resource filter clears before the local detail closes.
	m.updateCode("esc")
	if s.Preference.Section != 1 || s.Preference.Resource != resource.Key() || s.Preference.Field != "/metadata/name" || s.Filter != "api" {
		t.Fatal("field history lost its return selection", s.Preference)
	}
	s.Filter = ""
	codeAction(t, m, "Graph")
	codeAction(t, m, "View Findings")
	if s.ValidationSource != s.Preference.Selected {
		t.Fatal("entry findings silently widened to the repository")
	}
	m.updateCode("esc")
	if s.Preference.Section != 0 {
		t.Fatal("nested findings did not return to entry details")
	}
	if s.Preference.Resource != resource.Key() {
		t.Fatal("returning to the map lost the rendered resource identity")
	}
	codeAction(t, m, "Repository Findings")
	if s.ValidationSource != "" {
		t.Fatal("repository findings were restricted to one entry")
	}
}

func TestCodeRenderSaveRefreshesChangesWithoutReplacingEditor(t *testing.T) {
	m, s := codeModel(t)
	m.selectScope(m.active, "manifests", m.connection)
	s.Preference.Selected, s.Pending = "plain:plain/service.yaml", "render"
	finishCode(t, m)
	before := s.Builds[s.Preference.Selected]
	codeAction(t, m, "Open Resource Source")
	document := s.Document
	if document == nil || document.ReadOnly || s.File.Path != "plain/service.yaml" {
		t.Fatal("resource source did not open for editing")
	}
	document.Cursor = strings.Index(document.Text(), "80")
	document.Anchor = document.Cursor
	document.Paste("1")
	if s.Pending != "" {
		t.Fatal("unsaved edits scheduled a render")
	}
	if err := os.WriteFile(filepath.Join(s.Config.Path, "added.yaml"), []byte("apiVersion: v1\nkind: ConfigMap\nmetadata: {name: added}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m.saveCodeDocument(false)
	if s.Pending != "render" || s.SavedBaseline != before {
		t.Fatal("save did not capture the previous output")
	}
	if !s.Rescan || !s.Inventory.Discovering || s.Inventory.Entry("plain:added.yaml") != nil {
		t.Fatal("save rediscovered the source instead of queuing background work")
	}
	finishCode(t, m)
	if s.Inventory.Entry("plain:added.yaml") != nil {
		t.Fatal("render rediscovered unrelated configurations")
	}
	if s.Document != document || document.Dirty() || !s.SavedComparison || s.Comparison != before || s.ComparisonBase != s.Builds[s.Preference.Selected] {
		t.Fatal("save comparison replaced the editor or lost its baseline")
	}
	m.codeDocumentCommand("q")
	body := ansi.Strip(m.codeBody(110, 32))
	if !strings.Contains(body, "Before Save") || !strings.Contains(body, "After Save") || !strings.Contains(body, "port: 180") {
		t.Fatal("missing saved edit comparison", body)
	}
	codeAction(t, m, "Show Final YAML")
	if s.ComparisonActive() || !strings.Contains(ansi.Strip(m.codeBody(110, 32)), "port: 180") {
		t.Fatal("complete output is inaccessible")
	}
	codeAction(t, m, "Show Changes")
	if !s.ComparisonActive() {
		t.Fatal("cannot resume saved changes")
	}
	finishCode(t, m)
	if s.Rescan || s.Inventory.Discovering || s.Inventory.Entry("plain:added.yaml") == nil {
		t.Fatal("queued discovery did not refresh the configuration inventory")
	}
}

func TestCodeSavedRenderFailureRetainsOutputAndRetryBaseline(t *testing.T) {
	m, s := codeModel(t)
	m.selectScope(m.active, "manifests", m.connection)
	s.Preference.Selected, s.Pending = "kustomize:prod", "render"
	finishCode(t, m)
	before := s.Builds[s.Preference.Selected]
	s.Preference.File = "prod/kustomization.yaml"
	m.openCodeFile(true)
	s.Document.Paste("# saved edit\n")
	s.Renderer.Kustomize = func(context.Context, string) ([]byte, string, error) {
		return nil, "fixture", errors.New("fixture render failed")
	}
	m.saveCodeDocument(true)
	cmd := m.syncCode()
	applyCodeResult(t, m, cmd().(appcode.CodeResult))
	if s.Builds[s.Preference.Selected] != before || s.SavedBaseline != before || s.Status(s.Preference.Selected) != "failed" {
		t.Fatal("failed save discarded output or baseline")
	}
	body := ansi.Strip(m.codeBody(110, 32))
	if !strings.Contains(body, "Previous output") || !strings.Contains(body, "Couldn't render prod") || !strings.Contains(body, "Technical details") {
		t.Fatal("failure was not explicit", body)
	}
	m.activateCodeDiagnostics()
	if s.Document == nil || !strings.Contains(s.Document.Text(), "fixture render failed") {
		t.Fatal("technical details lost engine failure")
	}
	s.CloseDocument()
	s.Renderer = source.Renderer{}
	m.executeCode("render")
	finishCode(t, m)
	if s.SavedBaseline != nil || s.Comparison != before || !s.SavedComparison {
		t.Fatal("retry lost the pre-edit baseline")
	}
}

func TestCodeRenderDetailsCloseBeforeExpandedLayout(t *testing.T) {
	m, s := codeModel(t)
	m.selectScope(m.active, "manifests", m.connection)
	s.Preference.Selected, s.Pending = "plain:plain/service.yaml", "render"
	finishCode(t, m)
	m.execute("f")
	codeAction(t, m, "Field History")
	press(t, m, "esc")
	if s.Preference.Section != 1 || !m.viewExpanded() {
		t.Fatal("Escape restored layout before returning from field history")
	}
}

func TestCodeSavedSectionsRestoreMapAndExactScope(t *testing.T) {
	for _, section := range []int{0, 1, 2, 3} {
		config := source.Config{Path: t.TempDir()}
		controller := appcode.Controller{Preferences: map[string]appcode.CodePreference{"w/c": {Selected: "kustomize:prod", Section: section, Resource: "resource-key", Field: "/spec/replicas", File: "draft-path", Fingerprint: config.Fingerprint()}}}
		s := controller.StateFor("w", "c", config, nil)
		if s.Preference.Section != 0 || s.Preference.Selected != "kustomize:prod" || s.Preference.Resource != "resource-key" || s.Preference.Field != "/spec/replicas" || s.Preference.File != "draft-path" {
			t.Fatal("legacy selection was not retained", section, s.Preference)
		}
	}
}
